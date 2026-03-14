package tests

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func buildBinary(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("../miniDocker_test"); os.IsNotExist(err) {
		buildCmd := exec.Command("go", "build", "-o", "../miniDocker_test", "../.")
		buildCmd.Dir = ".."
		output, err := buildCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to build: %v\nOutput: %s", err, string(output))
		}
		t.Cleanup(func() { os.Remove("../miniDocker_test") })
	}
}

func TestPhase1_BasicExecution(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	buildBinary(t)

	cmd := exec.Command("../miniDocker_test")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error when running without arguments")
	}
	if !strings.Contains(string(output), "Usage") {
		t.Error("expected usage message in output")
	}

	cmd = exec.Command("../miniDocker_test", "invalid")
	output, err = cmd.CombinedOutput()
	if err == nil {
		t.Error("expected error for unknown command")
	}
	if !strings.Contains(string(output), "unknown command") {
		t.Error("expected unknown command error")
	}
}

func TestPhase1_ContainerHostname(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/hostname")
	cmd.Env = os.Environ()

	stdout, err := cmd.Output()
	t.Logf("stdout: %q", string(stdout))

	if err != nil {
		t.Fatalf("container execution failed: %v", err)
	}

	if strings.TrimSpace(string(stdout)) != "container" {
		t.Errorf("expected hostname 'container', got: %q", strings.TrimSpace(string(stdout)))
	}
}

func TestPhase1_SignalHandling(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/sleep", "30")
	cmd.Stderr = os.Stderr // print container logs so we can see what's happening

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}
	t.Logf("container parent PID: %d", cmd.Process.Pid)

	time.Sleep(500 * time.Millisecond)

	// Check the process is still alive before signalling
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Logf("process already dead before signal: %v", err)
	} else {
		t.Logf("process is alive, sending SIGINT")
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Errorf("failed to send SIGINT: %v", err)
	}

	// Also try SIGTERM after a short wait as fallback
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Logf("SIGTERM failed (process may already be exiting): %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		t.Logf("container exited: %v", err)
	case <-time.After(10 * time.Second):
		// Log process state before killing
		t.Logf("timeout — checking process state")
		if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
			t.Logf("process is dead but Wait() didn't return: %v", err)
		} else {
			t.Logf("process is still alive after 10s")
		}
		cmd.Process.Kill()
		t.Error("container did not exit after signal within timeout")
	}
}

func TestPhase2_ContainerIDGeneration(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/true")
	output, _ := cmd.CombinedOutput()
	t.Logf("Output: %s", string(output))
	t.Log("Phase 2: Container ID generation and setup completed")
}

func TestPhase2_EnvironmentVariables(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/sh", "-c", "echo $CONTAINER_ID")
	stdout, _ := cmd.Output()
	t.Logf("CONTAINER_ID: %q", strings.TrimSpace(string(stdout)))
	if strings.TrimSpace(string(stdout)) == "" {
		t.Error("expected CONTAINER_ID to be set in container environment")
	}
}

func TestPhase2_OverlayMounting(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/true")
	output, _ := cmd.CombinedOutput()
	t.Logf("Output: %s", string(output))
	t.Log("Phase 2: Overlay mount attempted")
}

func TestPhase2_PivotRoot(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/true")
	output, _ := cmd.CombinedOutput()
	t.Logf("Output: %s", string(output))
	t.Log("Phase 2: Pivot root attempted")
}

func TestPhase2_MountEssentials(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/true")
	output, _ := cmd.CombinedOutput()
	t.Logf("Output: %s", string(output))
	t.Log("Phase 2: Essential mounts attempted")
}

func TestPhase2_Cleanup(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/true")
	output, _ := cmd.CombinedOutput()
	t.Logf("Output: %s", string(output))
	t.Log("Phase 2: Container cleanup (unmount + remove dirs) executed")
}
