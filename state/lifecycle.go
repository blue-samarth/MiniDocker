package state

import (
	"errors"
	"log"
	"os"
	"path/filepath"
)

// LifecycleManager coordinates container state transitions.
type LifecycleManager struct {
	sm *StateManager
}

func NewLifecycleManager(sm *StateManager) *LifecycleManager {
	return &LifecycleManager{sm: sm}
}

func (lm *LifecycleManager) InitContainer(id string, cfg *ContainerConfig) error {
	if _, err := lm.sm.GetState(id); err == nil {
		return errors.New("container already exists")
	}

	log.Printf("initializing container %s", id)
	return lm.sm.CreateContainer(id, cfg)
}

func (lm *LifecycleManager) MarkRunning(id string, pid int) error {
	log.Printf("container %s running (pid %d)", id, pid)

	if err := lm.sm.UpdateStatus(id, StatusRunning); err != nil {
		return err
	}
	return lm.sm.SetPid(id, pid)
}

func (lm *LifecycleManager) MarkExited(id string, exitCode int) error {
	log.Printf("container %s exited with code %d", id, exitCode)
	return lm.sm.SetExit(id, exitCode)
}

func (lm *LifecycleManager) MarkError(id string, msg string) error {
	log.Printf("container %s error: %s", id, msg)
	return lm.sm.SetError(id, msg)
}

func (lm *LifecycleManager) RecordNetwork(id, ip, gateway, bridge string) error {
	// optional: log only on debug level or when troubleshooting networking
	// log.Printf("container %s network: ip=%s gw=%s bridge=%s", id, ip, gateway, bridge)
	return lm.sm.SetNetwork(id, ip, gateway, bridge)
}

func (lm *LifecycleManager) RecordCgroupPath(id, path string) error {
	// usually not worth logging unless debugging cgroup issues
	// log.Printf("container %s cgroup path: %s", id, path)
	return lm.sm.SetCgroupPath(id, path)
}

func (lm *LifecycleManager) GetLogDir(id string) (string, error) {
	cs, err := lm.sm.GetState(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(cs.StateDir, "logs"), nil
}

func (lm *LifecycleManager) Cleanup(id string) error {
	cs, err := lm.sm.GetState(id)
	if err != nil {
		return err
	}

	if !cs.Status.IsTerminal() {
		return errors.New("cannot cleanup non-terminal container")
	}

	log.Printf("cleaning up container %s (state dir: %s)", id, cs.StateDir)

	if err := os.RemoveAll(cs.StateDir); err != nil {
		log.Printf("warning: failed to remove state dir %s: %v", cs.StateDir, err)
		// still continue — we want to remove the DB record anyway
	}

	return lm.sm.RemoveContainer(id)
}

func (lm *LifecycleManager) GetState(id string) (*ContainerState, error) {
	return lm.sm.GetState(id)
}

func (lm *LifecycleManager) ListContainers() ([]*ContainerState, error) {
	return lm.sm.ListContainers()
}
