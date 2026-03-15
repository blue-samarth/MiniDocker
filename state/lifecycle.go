package state

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// LifecycleManager coordinates container state with resource managers.
type LifecycleManager struct {
	sm *StateManager
}

// NewLifecycleManager creates a LifecycleManager backed by the given StateManager.
func NewLifecycleManager(sm *StateManager) *LifecycleManager {
	return &LifecycleManager{sm: sm}
}

// InitContainer creates the state record for a new container before it starts.
// Called from container/run.go after generating the container ID.
// Returns error if container already exists.
func (lm *LifecycleManager) InitContainer(id string, cfg *ContainerConfig) error {
	log.Printf("[lifecycle] initialising container %s", id)

	// Check if container already exists
	if _, err := lm.sm.GetState(id); err == nil {
		return fmt.Errorf("container %s already exists", id)
	}

	if err := lm.sm.CreateContainer(id, cfg); err != nil {
		return fmt.Errorf("failed to create container state: %w", err)
	}
	return nil
}

// MarkRunning transitions the container to running and records its host PID.
// Called from container/run.go after cmd.Start() succeeds.
func (lm *LifecycleManager) MarkRunning(id string, pid int) error {
	log.Printf("[lifecycle] container %s running with PID %d", id, pid)

	if err := lm.sm.UpdateStatus(id, StatusRunning); err != nil {
		return fmt.Errorf("failed to mark container running: %w", err)
	}
	if err := lm.sm.SetPid(id, pid); err != nil {
		return fmt.Errorf("failed to record container PID: %w", err)
	}
	return nil
}

// MarkExited transitions the container to exited and records the exit code.
// Called from container/run.go after cmd.Wait() returns.
func (lm *LifecycleManager) MarkExited(id string, exitCode int) error {
	log.Printf("[lifecycle] container %s exited with code %d", id, exitCode)
	return lm.sm.SetExit(id, exitCode)
}

// MarkError transitions the container to the error state.
func (lm *LifecycleManager) MarkError(id, errMsg string) error {
	log.Printf("[lifecycle] container %s error: %s", id, errMsg)
	return lm.sm.SetError(id, errMsg)
}

// RecordNetwork stores the network configuration in the container state.
func (lm *LifecycleManager) RecordNetwork(id, ip, gateway, bridge string) error {
	return lm.sm.SetNetwork(id, ip, gateway, bridge)
}

// RecordCgroupPath stores the cgroup path in the container state.
func (lm *LifecycleManager) RecordCgroupPath(id, cgroupPath string) error {
	return lm.sm.SetCgroupPath(id, cgroupPath)
}

// GetLogDir returns the log directory path for a container.
func (lm *LifecycleManager) GetLogDir(id string) (string, error) {
	cs, err := lm.sm.GetState(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(cs.StateDir, "logs"), nil
}

// Cleanup removes the container state directory.
// Call this when the container is fully torn down and the caller
// no longer needs state or logs.
func (lm *LifecycleManager) Cleanup(id string) error {
	cs, err := lm.sm.GetState(id)
	if err != nil {
		return err
	}

	if !cs.Status.IsTerminal() {
		return fmt.Errorf("cannot cleanup container %s in non-terminal state %s", id, cs.Status)
	}

	log.Printf("[lifecycle] cleaning up container %s state dir %s", id, cs.StateDir)
	if err := os.RemoveAll(cs.StateDir); err != nil {
		log.Printf("[lifecycle] warning: failed to remove state dir: %v", err)
	}

	return lm.sm.RemoveContainer(id)
}

// GetState returns the current state of a container.
func (lm *LifecycleManager) GetState(id string) (*ContainerState, error) {
	return lm.sm.GetState(id)
}

// ListContainers returns all known container states.
func (lm *LifecycleManager) ListContainers() ([]*ContainerState, error) {
	return lm.sm.ListContainers()
}
