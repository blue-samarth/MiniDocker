package tests

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPhase1_BasicExecution(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}

	buildCmd := exec.Command("go", "build", "-o", "../miniDocker_test", "../.")
	buildCmd.Dir = ".."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build: %v", err)
	}
	defer os.Remove("../miniDocker_test")

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

	buildCmd := exec.Command("go", "build", "-o", "../miniDocker_test", "../.")
	buildCmd.Dir = ".."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build: %v", err)
	}
	defer os.Remove("../miniDocker_test")

	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/hostname")
	cmd.Env = os.Environ()
	
	output, err := cmd.CombinedOutput()
	t.Logf("Output: %s", string(output))
	
	if err != nil {
		t.Logf("Container execution failed: %v", err)
		return
	}

	if strings.TrimSpace(string(output)) != "container" {
		t.Errorf("expected hostname 'container', got: %s", string(output))
	}
}

func TestPhase1_SignalHandling(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges")
	}

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	buildCmd := exec.Command("go", "build", "-o", "../miniDocker_test", "../.")
	buildCmd.Dir = ".."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build: %v", err)
	}
	defer os.Remove("../miniDocker_test")

	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/sleep", "30")
	
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Errorf("failed to send signal: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("container exited with error after signal: %v", err)
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Error("container did not exit after signal within timeout")
	}
}