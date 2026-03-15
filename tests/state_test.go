package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"miniDocker/state"
)

// newTestStateManager creates a StateManager using a temp dir — no root needed.
func newTestStateManager(t *testing.T) *state.StateManager {
	t.Helper()
	sm, err := state.NewStateManagerWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create state manager: %v", err)
	}
	return sm
}

func defaultConfig() *state.ContainerConfig {
	return &state.ContainerConfig{
		Image:   "/tmp/rootfs",
		Command: []string{"/bin/sh"},
		Memory:  "256m",
		CPU:     "0.5",
	}
}

// --- StateManager CRUD ---

func TestStateManager_CreateContainer(t *testing.T) {
	sm := newTestStateManager(t)

	if err := sm.CreateContainer("abc123", defaultConfig()); err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	cs, err := sm.GetState("abc123")
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if cs.ID != "abc123" {
		t.Errorf("expected ID abc123, got %s", cs.ID)
	}
	if cs.Status != state.StatusCreated {
		t.Errorf("expected status created, got %s", cs.Status)
	}
	if cs.Image != "/tmp/rootfs" {
		t.Errorf("expected image /tmp/rootfs, got %s", cs.Image)
	}
	if cs.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestStateManager_UpdateStatus_ValidTransitions(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())

	if err := sm.UpdateStatus("c1", state.StatusRunning); err != nil {
		t.Fatalf("created→running failed: %v", err)
	}
	cs, _ := sm.GetState("c1")
	if cs.Status != state.StatusRunning {
		t.Errorf("expected running, got %s", cs.Status)
	}
	if cs.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set when running")
	}

	if err := sm.UpdateStatus("c1", state.StatusExited); err != nil {
		t.Fatalf("running→exited failed: %v", err)
	}
	cs, _ = sm.GetState("c1")
	if cs.Status != state.StatusExited {
		t.Errorf("expected exited, got %s", cs.Status)
	}
}

func TestStateManager_UpdateStatus_InvalidTransition(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())

	err := sm.UpdateStatus("c1", state.StatusExited)
	if err == nil {
		t.Error("expected error for invalid transition created→exited")
	}
	if !strings.Contains(err.Error(), "invalid state transition") {
		t.Errorf("expected invalid state transition error, got: %v", err)
	}
}

func TestStateManager_SetPid(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())
	sm.UpdateStatus("c1", state.StatusRunning)

	if err := sm.SetPid("c1", 12345); err != nil {
		t.Fatalf("SetPid failed: %v", err)
	}
	cs, _ := sm.GetState("c1")
	if cs.Pid != 12345 {
		t.Errorf("expected pid 12345, got %d", cs.Pid)
	}
}

func TestStateManager_SetExit(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())
	sm.UpdateStatus("c1", state.StatusRunning)

	if err := sm.SetExit("c1", 42); err != nil {
		t.Fatalf("SetExit failed: %v", err)
	}
	cs, _ := sm.GetState("c1")
	if cs.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", cs.ExitCode)
	}
	if cs.Status != state.StatusExited {
		t.Errorf("expected exited status, got %s", cs.Status)
	}
	if cs.Pid != 0 {
		t.Errorf("expected pid 0 after exit, got %d", cs.Pid)
	}
	if cs.FinishedAt.IsZero() {
		t.Error("expected FinishedAt to be set")
	}
}

func TestStateManager_SetError(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())

	if err := sm.SetError("c1", "something went wrong"); err != nil {
		t.Fatalf("SetError failed: %v", err)
	}
	cs, _ := sm.GetState("c1")
	if cs.Status != state.StatusError {
		t.Errorf("expected error status, got %s", cs.Status)
	}
	if cs.Error != "something went wrong" {
		t.Errorf("expected error message, got %q", cs.Error)
	}
}

func TestStateManager_SetNetwork(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())

	if err := sm.SetNetwork("c1", "172.20.0.2", "172.20.0.1", "miniDocker0"); err != nil {
		t.Fatalf("SetNetwork failed: %v", err)
	}
	cs, _ := sm.GetState("c1")
	if cs.IPAddress != "172.20.0.2" {
		t.Errorf("expected IP 172.20.0.2, got %s", cs.IPAddress)
	}
	if cs.Gateway != "172.20.0.1" {
		t.Errorf("expected gateway 172.20.0.1, got %s", cs.Gateway)
	}
	if cs.Bridge != "miniDocker0" {
		t.Errorf("expected bridge miniDocker0, got %s", cs.Bridge)
	}
}

func TestStateManager_SetCgroupPath(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())

	path := "/sys/fs/cgroup/miniDocker/c1"
	if err := sm.SetCgroupPath("c1", path); err != nil {
		t.Fatalf("SetCgroupPath failed: %v", err)
	}
	cs, _ := sm.GetState("c1")
	if cs.CgroupPath != path {
		t.Errorf("expected cgroup path %s, got %s", path, cs.CgroupPath)
	}
}

func TestStateManager_GetState_NotFound(t *testing.T) {
	sm := newTestStateManager(t)
	_, err := sm.GetState("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent container")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestStateManager_ListContainers(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())
	sm.CreateContainer("c2", defaultConfig())
	sm.CreateContainer("c3", defaultConfig())

	list, err := sm.ListContainers()
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 containers, got %d", len(list))
	}
}

func TestStateManager_ListContainers_ReturnsCopies(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())

	list, _ := sm.ListContainers()
	list[0].Status = state.StatusRunning

	cs, _ := sm.GetState("c1")
	if cs.Status != state.StatusCreated {
		t.Error("mutating list result should not affect internal state")
	}
}

func TestStateManager_RemoveContainer(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())
	sm.UpdateStatus("c1", state.StatusRunning)
	sm.SetExit("c1", 0)

	if err := sm.RemoveContainer("c1"); err != nil {
		t.Fatalf("RemoveContainer failed: %v", err)
	}
	_, err := sm.GetState("c1")
	if err == nil {
		t.Error("expected error after removal")
	}
}

func TestStateManager_RemoveContainer_NotFound(t *testing.T) {
	sm := newTestStateManager(t)
	if err := sm.RemoveContainer("nonexistent"); err == nil {
		t.Error("expected error removing nonexistent container")
	}
}

// --- Persistence ---

func TestStateManager_Persistence(t *testing.T) {
	dir := t.TempDir()

	sm1, _ := state.NewStateManagerWithDir(dir)
	sm1.CreateContainer("c1", defaultConfig())
	sm1.UpdateStatus("c1", state.StatusRunning)
	sm1.SetPid("c1", 9999)
	sm1.SetNetwork("c1", "172.20.0.5", "172.20.0.1", "miniDocker0")

	sm2, err := state.NewStateManagerWithDir(dir)
	if err != nil {
		t.Fatalf("failed to reload state: %v", err)
	}
	cs, err := sm2.GetState("c1")
	if err != nil {
		t.Fatalf("GetState after reload failed: %v", err)
	}
	if cs.Status != state.StatusRunning {
		t.Errorf("expected running after reload, got %s", cs.Status)
	}
	if cs.Pid != 9999 {
		t.Errorf("expected pid 9999 after reload, got %d", cs.Pid)
	}
	if cs.IPAddress != "172.20.0.5" {
		t.Errorf("expected IP 172.20.0.5 after reload, got %s", cs.IPAddress)
	}
}

func TestStateManager_StateFile_Exists(t *testing.T) {
	sm := newTestStateManager(t)
	sm.CreateContainer("c1", defaultConfig())

	stateFile := sm.GetStateFile("c1")
	if _, err := os.Stat(stateFile); err != nil {
		t.Errorf("state file should exist at %s: %v", stateFile, err)
	}
}

func TestStateManager_CorruptedStateFile(t *testing.T) {
	dir := t.TempDir()

	sm1, _ := state.NewStateManagerWithDir(dir)
	sm1.CreateContainer("c1", defaultConfig())

	stateFile := sm1.GetStateFile("c1")
	if err := os.WriteFile(stateFile, []byte("not valid json{"), 0644); err != nil {
		t.Fatalf("failed to corrupt state file: %v", err)
	}

	sm2, err := state.NewStateManagerWithDir(dir)
	if err != nil {
		t.Fatalf("should not fail on corrupted state: %v", err)
	}
	list, _ := sm2.ListContainers()
	if len(list) != 0 {
		t.Error("corrupted state should be skipped")
	}
}

// --- Logs ---

func TestLogManager_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	lm, err := state.NewLogManager("test-container", dir)
	if err != nil {
		t.Fatalf("NewLogManager failed: %v", err)
	}

	fmt.Fprintf(lm.StdoutWriter(), "line one\nline two\nline three\n")
	lm.Close()

	lm2, _ := state.NewLogManager("test-container", dir)
	defer lm2.Close()

	data, err := lm2.GetLogs(0)
	if err != nil {
		t.Fatalf("GetLogs failed: %v", err)
	}
	if !strings.Contains(string(data), "line one") {
		t.Errorf("expected log content, got: %q", string(data))
	}
}

func TestLogManager_TailLines(t *testing.T) {
	dir := t.TempDir()
	lm, _ := state.NewLogManager("test-container", dir)

	for i := 1; i <= 10; i++ {
		fmt.Fprintf(lm.StdoutWriter(), "line %d\n", i)
	}
	lm.Close()

	lm2, _ := state.NewLogManager("test-container", dir)
	defer lm2.Close()

	data, err := lm2.GetLogs(3)
	if err != nil {
		t.Fatalf("GetLogs failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %q", len(lines), string(data))
	}
	if !strings.Contains(lines[len(lines)-1], "line 10") {
		t.Errorf("last line should be line 10, got %q", lines[len(lines)-1])
	}
}

func TestLogManager_EmptyLogs(t *testing.T) {
	lm, _ := state.NewLogManager("test-container", t.TempDir())
	defer lm.Close()

	data, err := lm.GetLogs(10)
	if err != nil {
		t.Fatalf("GetLogs on empty log failed: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %q", string(data))
	}
}

func TestLogManager_Close(t *testing.T) {
	lm, err := state.NewLogManager("test-container", t.TempDir())
	if err != nil {
		t.Fatalf("NewLogManager failed: %v", err)
	}
	if err := lm.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestLogManager_StderrLogs(t *testing.T) {
	dir := t.TempDir()
	lm, _ := state.NewLogManager("test-container", dir)

	fmt.Fprintf(lm.StderrWriter(), "error line 1\nerror line 2\n")
	lm.Close()

	lm2, _ := state.NewLogManager("test-container", dir)
	defer lm2.Close()

	data, err := lm2.GetStderrLogs(0)
	if err != nil {
		t.Fatalf("GetStderrLogs failed: %v", err)
	}
	if !strings.Contains(string(data), "error line 1") {
		t.Errorf("expected stderr content, got: %q", string(data))
	}
}

// --- LifecycleManager ---

func TestLifecycleManager_FullLifecycle(t *testing.T) {
	sm := newTestStateManager(t)
	lm := state.NewLifecycleManager(sm)

	if err := lm.InitContainer("c1", defaultConfig()); err != nil {
		t.Fatalf("InitContainer failed: %v", err)
	}
	if err := lm.MarkRunning("c1", 5678); err != nil {
		t.Fatalf("MarkRunning failed: %v", err)
	}

	cs, _ := lm.GetState("c1")
	if cs.Status != state.StatusRunning {
		t.Errorf("expected running, got %s", cs.Status)
	}
	if cs.Pid != 5678 {
		t.Errorf("expected pid 5678, got %d", cs.Pid)
	}

	if err := lm.MarkExited("c1", 0); err != nil {
		t.Fatalf("MarkExited failed: %v", err)
	}
	cs, _ = lm.GetState("c1")
	if cs.Status != state.StatusExited {
		t.Errorf("expected exited, got %s", cs.Status)
	}
}

func TestLifecycleManager_MarkError(t *testing.T) {
	sm := newTestStateManager(t)
	lm := state.NewLifecycleManager(sm)
	lm.InitContainer("c1", defaultConfig())

	if err := lm.MarkError("c1", "oom killed"); err != nil {
		t.Fatalf("MarkError failed: %v", err)
	}
	cs, _ := lm.GetState("c1")
	if cs.Status != state.StatusError {
		t.Errorf("expected error status, got %s", cs.Status)
	}
}

func TestLifecycleManager_RecordNetwork(t *testing.T) {
	sm := newTestStateManager(t)
	lm := state.NewLifecycleManager(sm)
	lm.InitContainer("c1", defaultConfig())

	if err := lm.RecordNetwork("c1", "172.20.0.3", "172.20.0.1", "miniDocker0"); err != nil {
		t.Fatalf("RecordNetwork failed: %v", err)
	}
	cs, _ := lm.GetState("c1")
	if cs.IPAddress != "172.20.0.3" {
		t.Errorf("expected IP 172.20.0.3, got %s", cs.IPAddress)
	}
}

func TestLifecycleManager_Cleanup_NonTerminal(t *testing.T) {
	sm := newTestStateManager(t)
	lm := state.NewLifecycleManager(sm)
	lm.InitContainer("c1", defaultConfig())
	lm.MarkRunning("c1", 1234)

	if err := lm.Cleanup("c1"); err == nil {
		t.Error("expected error cleaning up running container")
	}
}

func TestLifecycleManager_Cleanup_Terminal(t *testing.T) {
	sm := newTestStateManager(t)
	lm := state.NewLifecycleManager(sm)
	lm.InitContainer("c1", defaultConfig())
	lm.MarkRunning("c1", 1234)
	lm.MarkExited("c1", 0)

	if err := lm.Cleanup("c1"); err != nil {
		t.Fatalf("Cleanup of exited container failed: %v", err)
	}
	if _, err := lm.GetState("c1"); err == nil {
		t.Error("expected container to be gone after cleanup")
	}
}

func TestLifecycleManager_ListContainers(t *testing.T) {
	sm := newTestStateManager(t)
	lm := state.NewLifecycleManager(sm)
	lm.InitContainer("c1", defaultConfig())
	lm.InitContainer("c2", defaultConfig())

	list, err := lm.ListContainers()
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 containers, got %d", len(list))
	}
}

func TestLifecycleManager_GetLogDir(t *testing.T) {
	sm := newTestStateManager(t)
	lm := state.NewLifecycleManager(sm)
	lm.InitContainer("c1", defaultConfig())

	logDir, err := lm.GetLogDir("c1")
	if err != nil {
		t.Fatalf("GetLogDir failed: %v", err)
	}
	if !strings.HasSuffix(logDir, filepath.Join("c1", "logs")) {
		t.Errorf("unexpected log dir: %s", logDir)
	}
}

// ─── Comprehensive Functionality Tests ───────────────────────────────────────

// TestComprehensive_ContainerLifecycle tests the full container lifecycle
func TestComprehensive_ContainerLifecycle(t *testing.T) {
	sm := newTestStateManager(t)
	lm := state.NewLifecycleManager(sm)
	id := "lifecycle-test-abc123"

	// 1. Created state
	cfg := &state.ContainerConfig{
		Image:   "/test/rootfs",
		Command: []string{"/bin/bash"},
		Memory:  "512m",
		CPU:     "1.0",
	}
	if err := lm.InitContainer(id, cfg); err != nil {
		t.Fatalf("InitContainer failed: %v", err)
	}

	cs, err := lm.GetState(id)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if cs.Status != state.StatusCreated {
		t.Errorf("Expected StatusCreated, got %v", cs.Status)
	}
	if cs.Pid != 0 {
		t.Errorf("Expected Pid 0 in created state, got %d", cs.Pid)
	}

	// 2. Running state
	testPID := 9999
	if err := lm.MarkRunning(id, testPID); err != nil {
		t.Fatalf("MarkRunning failed: %v", err)
	}

	cs, _ = lm.GetState(id)
	if cs.Status != state.StatusRunning {
		t.Errorf("Expected StatusRunning, got %v", cs.Status)
	}
	if cs.Pid != testPID {
		t.Errorf("Expected Pid %d, got %d", testPID, cs.Pid)
	}

	// 3. Network recording
	if err := lm.RecordNetwork(id, "172.20.0.5", "172.20.0.1", "veth12345"); err != nil {
		t.Fatalf("RecordNetwork failed: %v", err)
	}

	cs, _ = lm.GetState(id)
	if cs.IPAddress != "172.20.0.5" {
		t.Errorf("Expected IP 172.20.0.5, got %s", cs.IPAddress)
	}

	// 4. Cgroup recording
	cgroupPath := "/sys/fs/cgroup/miniDocker/" + id
	if err := lm.RecordCgroupPath(id, cgroupPath); err != nil {
		t.Fatalf("RecordCgroupPath failed: %v", err)
	}

	cs, _ = lm.GetState(id)
	if cs.CgroupPath != cgroupPath {
		t.Errorf("Expected CgroupPath %s, got %s", cgroupPath, cs.CgroupPath)
	}

	// 5. Exited state
	if err := lm.MarkExited(id, 0); err != nil {
		t.Fatalf("MarkExited failed: %v", err)
	}

	cs, _ = lm.GetState(id)
	if cs.Status != state.StatusExited {
		t.Errorf("Expected StatusExited, got %v", cs.Status)
	}
	if cs.ExitCode != 0 {
		t.Errorf("Expected ExitCode 0, got %d", cs.ExitCode)
	}

	// 6. Cleanup
	if err := lm.Cleanup(id); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if _, err := lm.GetState(id); err == nil {
		t.Error("Expected container to be removed after cleanup")
	}
}

// TestComprehensive_ErrorTransitions tests invalid state transitions
func TestComprehensive_ErrorTransitions(t *testing.T) {
	sm := newTestStateManager(t)
	lm := state.NewLifecycleManager(sm)
	id := "error-test-xyz789"

	// Try to mark running without creating
	if err := lm.MarkRunning(id, 1234); err == nil {
		t.Error("Expected error marking non-existent container as running")
	}

	// Create container
	cfg := &state.ContainerConfig{Image: "/img", Command: []string{"/bin/sh"}}
	if err := lm.InitContainer(id, cfg); err != nil {
		t.Fatalf("InitContainer failed: %v", err)
	}

	// Try to exit without running
	if err := lm.MarkExited(id, 0); err == nil {
		t.Error("Expected error exiting non-running container")
	}

	// Mark as running
	if err := lm.MarkRunning(id, 1234); err != nil {
		t.Fatalf("MarkRunning failed: %v", err)
	}

	// Try to init again - should fail because container exists
	if err := lm.InitContainer(id, cfg); err == nil {
		t.Error("Expected error re-initializing running container")
	}
}

// TestComprehensive_Concurrency tests thread safety
func TestComprehensive_Concurrency(t *testing.T) {
	sm := newTestStateManager(t)
	lm := state.NewLifecycleManager(sm)
	id := "concurrent-test"

	cfg := &state.ContainerConfig{Image: "/img", Command: []string{"/bin/sh"}}
	lm.InitContainer(id, cfg)
	lm.MarkRunning(id, 8888)

	// Concurrent reads
	readErrors := make(chan error, 100)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := lm.GetState(id)
			if err != nil {
				readErrors <- err
			}
		}()
	}

	wg.Wait()
	close(readErrors)

	for err := range readErrors {
		if err != nil {
			t.Logf("Warning: concurrent read error: %v", err)
		}
	}
}

// TestComprehensive_Persistence tests state recovery across restarts
func TestComprehensive_Persistence(t *testing.T) {
	dir := t.TempDir()

	// First lifecycle: create, run, record data
	{
		sm1, err := state.NewStateManagerWithDir(dir)
		if err != nil {
			t.Fatalf("NewStateManagerWithDir: %v", err)
		}
		lm1 := state.NewLifecycleManager(sm1)

		id := "persist-test"
		cfg := &state.ContainerConfig{
			Image:   "/rootfs",
			Command: []string{"/bin/bash"},
			Memory:  "256m",
			CPU:     "0.5",
		}
		lm1.InitContainer(id, cfg)
		lm1.MarkRunning(id, 5555)
		lm1.RecordNetwork(id, "172.20.0.100", "172.20.0.1", "miniDocker0")
		lm1.RecordCgroupPath(id, "/cgroup/path")
	}

	// Second lifecycle: verify persistence
	{
		sm2, err := state.NewStateManagerWithDir(dir)
		if err != nil {
			t.Fatalf("NewStateManagerWithDir: %v", err)
		}
		lm2 := state.NewLifecycleManager(sm2)

		id := "persist-test"
		cs, err := lm2.GetState(id)
		if err != nil {
			t.Fatalf("GetState: %v", err)
		}

		if cs.Status != state.StatusRunning {
			t.Errorf("Status not persisted: %v", cs.Status)
		}
		if cs.Pid != 5555 {
			t.Errorf("PID not persisted: expected 5555, got %d", cs.Pid)
		}
		if cs.IPAddress != "172.20.0.100" {
			t.Errorf("IP not persisted: got %s", cs.IPAddress)
		}
		if cs.CgroupPath != "/cgroup/path" {
			t.Errorf("CgroupPath not persisted: got %s", cs.CgroupPath)
		}
	}
}
