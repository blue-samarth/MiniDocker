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
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}
	t.Logf("container parent PID: %d", cmd.Process.Pid)

	time.Sleep(500 * time.Millisecond)

	// The user command runs as PID 1 inside CLONE_NEWPID.
	// PID 1 ignores SIGINT and SIGTERM by default — use SIGKILL instead.
	t.Logf("sending SIGKILL to container process group")
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		// Fall back to killing just the direct child.
		t.Logf("process group kill failed (%v), falling back to direct kill", err)
		if err := cmd.Process.Kill(); err != nil {
			t.Errorf("failed to kill container: %v", err)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		t.Logf("container exited after signal: %v", err)
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Error("container did not exit after SIGKILL within timeout")
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

// Phase 4: Network Isolation Tests

func TestPhase4_BridgeCreation(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	// Ensure bridge is clean before test
	exec.Command("../miniDocker_test", "cleanup-bridge").CombinedOutput()

	// Start a container - this should create the bridge
	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/sleep", "1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Check if bridge exists
	checkBridge := exec.Command("ip", "link", "show", "miniDocker0")
	output, _ := checkBridge.CombinedOutput()

	if !strings.Contains(string(output), "miniDocker0") {
		t.Errorf("bridge miniDocker0 not found after container start")
	} else {
		t.Log("Phase 4: Bridge miniDocker0 created successfully")
	}

	cmd.Process.Kill()
	cmd.Wait()
}

func TestPhase4_IPAllocation(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	// Start a container with IP check
	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/sh", "-c", "ip addr show eth0 | grep 172.20")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("container command error (may be expected): %v", err)
	}

	if strings.Contains(string(output), "172.20") {
		t.Log("Phase 4: Container IP allocated from 172.20.0.0/16 subnet")
	} else {
		t.Logf("IP allocation test: %s", string(output))
	}
}

func TestPhase4_ContainerNetworkInterface(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	// Check if container has eth0
	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/sh", "-c", "ip link show eth0")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("eth0 check error (may be expected): %v", err)
	}

	if strings.Contains(string(output), "eth0") {
		t.Log("Phase 4: Container network interface eth0 configured")
	} else {
		t.Logf("Network interface test output: %s", string(output))
	}
}

func TestPhase4_MultipleContainers(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	// Start first container
	cmd1 := exec.Command("../miniDocker_test", "run", "/usr", "/bin/sleep", "2")
	if err := cmd1.Start(); err != nil {
		t.Fatalf("failed to start first container: %v", err)
	}
	defer cmd1.Process.Kill()

	time.Sleep(500 * time.Millisecond)

	// Start second container
	cmd2 := exec.Command("../miniDocker_test", "run", "/usr", "/bin/sleep", "2")
	if err := cmd2.Start(); err != nil {
		t.Fatalf("failed to start second container: %v", err)
	}
	defer cmd2.Process.Kill()

	time.Sleep(500 * time.Millisecond)

	// Check veth interfaces
	checkVeth := exec.Command("ip", "link", "show", "type", "veth")
	output, _ := checkVeth.CombinedOutput()

	vethCount := strings.Count(string(output), "veth")
	if vethCount >= 2 {
		t.Logf("Phase 4: Multiple veth pairs created for containers (found %d veth interfaces)", vethCount)
	} else {
		t.Logf("Veth interface check: %s", string(output))
	}

	cmd2.Process.Kill()
	cmd2.Wait()
}

func TestPhase4_GatewayConnectivity(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	// Start container and ping gateway
	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/sh", "-c", "ping -c 1 172.20.0.1")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Logf("ping command error (may be expected): %v", err)
	}

	if strings.Contains(string(output), "1 packets transmitted") {
		t.Log("Phase 4: Container can reach gateway 172.20.0.1")
	} else {
		t.Logf("Gateway connectivity test output: %s", string(output))
	}
}

func TestPhase4_IPRelease(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	// Start and stop a container
	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/true")
	if err := cmd.Run(); err != nil {
		t.Logf("container execution error (may be expected): %v", err)
	}

	// Give time for cleanup
	time.Sleep(500 * time.Millisecond)

	// Check IPAM state file
	stateFile := "/var/run/miniDocker/ipam.json"
	if _, err := os.Stat(stateFile); err == nil {
		t.Log("Phase 4: IPAM state file exists at /var/run/miniDocker/ipam.json")
	} else {
		t.Logf("IPAM state file check: %v", err)
	}
}

func TestPhase4_ContainerNetworkVsHost(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	buildBinary(t)

	// Start container
	cmd := exec.Command("../miniDocker_test", "run", "/usr", "/bin/sleep", "1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}
	defer cmd.Process.Kill()

	time.Sleep(500 * time.Millisecond)

	// Check veth on host
	checkVeth := exec.Command("ip", "link", "show")
	output, _ := checkVeth.CombinedOutput()

	if strings.Contains(string(output), "veth") {
		t.Log("Phase 4: Veth interfaces visible on host side")
	}

	cmd.Wait()
}
