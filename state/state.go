package state

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const stateBaseDir = "/var/run/miniDocker/containers"

// ContainerState holds all persistent metadata for a container.
type ContainerState struct {
	ID         string          `json:"id"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at"`
	Status     ContainerStatus `json:"status"`
	Pid        int             `json:"pid"`
	ExitCode   int             `json:"exit_code"`
	Error      string          `json:"error,omitempty"`

	// Configuration
	Image   string   `json:"image"`
	Command []string `json:"command"`
	Args    []string `json:"args"`

	// Resource limits
	Memory string `json:"memory,omitempty"`
	CPU    string `json:"cpu,omitempty"`
	PIDs   int    `json:"pids,omitempty"`
	Swap   string `json:"swap,omitempty"`

	// Network
	IPAddress string `json:"ip_address,omitempty"`
	Gateway   string `json:"gateway,omitempty"`
	Bridge    string `json:"bridge,omitempty"`

	// Paths
	RootFS     string `json:"root_fs"`
	CgroupPath string `json:"cgroup_path,omitempty"`
	StateDir   string `json:"state_dir"`
	StdoutPath string `json:"stdout_path"`
	StderrPath string `json:"stderr_path"`
}

// StateManager handles all container state operations with thread-safe access.
type StateManager struct {
	stateDir string
	states   map[string]*ContainerState
	mu       sync.RWMutex
}

// NewStateManager creates a StateManager using the default state directory,
// loading any existing state from disk.
func NewStateManager() (*StateManager, error) {
	return NewStateManagerWithDir(stateBaseDir)
}

// NewStateManagerWithDir creates a StateManager using a custom directory.
// Useful for testing without requiring root.
func NewStateManagerWithDir(dir string) (*StateManager, error) {
	sm := &StateManager{
		stateDir: dir,
		states:   make(map[string]*ContainerState),
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state dir %q: %w", dir, err)
	}
	if err := sm.loadAll(); err != nil {
		return nil, fmt.Errorf("failed to load existing state: %w", err)
	}
	return sm, nil
}

// CreateContainer initialises a new container state record.
func (sm *StateManager) CreateContainer(id string, cfg *ContainerConfig) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	containerDir := filepath.Join(sm.stateDir, id)
	logsDir := filepath.Join(containerDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create container state dir: %w", err)
	}

	cs := &ContainerState{
		ID:         id,
		CreatedAt:  time.Now(),
		Status:     StatusCreated,
		Image:      cfg.Image,
		Command:    cfg.Command,
		Args:       cfg.Args,
		Memory:     cfg.Memory,
		CPU:        cfg.CPU,
		PIDs:       cfg.PIDs,
		Swap:       cfg.Swap,
		RootFS:     cfg.Image,
		StateDir:   containerDir,
		StdoutPath: filepath.Join(logsDir, "stdout"),
		StderrPath: filepath.Join(logsDir, "stderr"),
	}

	sm.states[id] = cs
	return sm.persist(id, cs)
}

// UpdateStatus transitions a container to a new status.
func (sm *StateManager) UpdateStatus(id string, status ContainerStatus) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cs, err := sm.get(id)
	if err != nil {
		return err
	}

	// Validate transition
	allowed := ValidTransitions[cs.Status]
	valid := false
	for _, s := range allowed {
		if s == status {
			valid = true
			break
		}
	}
	if !valid {
		return &ErrInvalidStateTransition{From: cs.Status, To: status}
	}

	cs.Status = status
	if status == StatusRunning {
		cs.StartedAt = time.Now()
	}
	return sm.persist(id, cs)
}

// SetPid records the host PID of the container process.
func (sm *StateManager) SetPid(id string, pid int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cs, err := sm.get(id)
	if err != nil {
		return err
	}

	cs.Pid = pid

	// Also write a plain pid file for external tooling
	pidFile := filepath.Join(sm.stateDir, id, "pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", pid)), 0644); err != nil {
		log.Printf("[state] warning: could not write pid file: %v", err)
	}

	return sm.persist(id, cs)
}

// SetExit records the exit code and marks the container as exited.
func (sm *StateManager) SetExit(id string, exitCode int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cs, err := sm.get(id)
	if err != nil {
		return err
	}

	cs.ExitCode = exitCode
	cs.FinishedAt = time.Now()
	cs.Status = StatusExited
	cs.Pid = 0

	return sm.persist(id, cs)
}

// SetError records an error message and marks the container as errored.
func (sm *StateManager) SetError(id string, errMsg string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cs, err := sm.get(id)
	if err != nil {
		return err
	}

	cs.Error = errMsg
	cs.Status = StatusError
	cs.FinishedAt = time.Now()
	cs.Pid = 0

	return sm.persist(id, cs)
}

// SetNetwork records the network configuration for a container.
func (sm *StateManager) SetNetwork(id, ip, gateway, bridge string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cs, err := sm.get(id)
	if err != nil {
		return err
	}

	cs.IPAddress = ip
	cs.Gateway = gateway
	cs.Bridge = bridge

	return sm.persist(id, cs)
}

// SetCgroupPath records the cgroup path for a container.
func (sm *StateManager) SetCgroupPath(id, cgroupPath string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cs, err := sm.get(id)
	if err != nil {
		return err
	}

	cs.CgroupPath = cgroupPath
	return sm.persist(id, cs)
}

// GetState returns a copy of the container state.
func (sm *StateManager) GetState(id string) (*ContainerState, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	cs, err := sm.get(id)
	if err != nil {
		return nil, err
	}

	// Return a copy to prevent external mutation
	copy := *cs
	return &copy, nil
}

// ListContainers returns copies of all known container states.
func (sm *StateManager) ListContainers() ([]*ContainerState, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]*ContainerState, 0, len(sm.states))
	for _, cs := range sm.states {
		copy := *cs
		result = append(result, &copy)
	}
	return result, nil
}

// RemoveContainer deletes the container state from memory and disk.
func (sm *StateManager) RemoveContainer(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.states[id]; !ok {
		return &ErrContainerNotFound{ID: id}
	}

	delete(sm.states, id)

	containerDir := filepath.Join(sm.stateDir, id)
	if err := os.RemoveAll(containerDir); err != nil {
		return fmt.Errorf("failed to remove container state dir: %w", err)
	}

	return nil
}

// GetStateFile returns the path to a container's state.json file.
func (sm *StateManager) GetStateFile(id string) string {
	return filepath.Join(sm.stateDir, id, "state.json")
}

// get returns the in-memory state for id (must be called with lock held).
func (sm *StateManager) get(id string) (*ContainerState, error) {
	cs, ok := sm.states[id]
	if !ok {
		return nil, &ErrContainerNotFound{ID: id}
	}
	return cs, nil
}

// persist atomically writes the container state to disk.
func (sm *StateManager) persist(id string, cs *ContainerState) error {
	stateFile := sm.GetStateFile(id)
	tmp := stateFile + ".tmp"

	data, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("failed to write state temp file: %w", err)
	}

	if err := os.Rename(tmp, stateFile); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// loadAll reads all container state files from disk into memory.
func (sm *StateManager) loadAll() error {
	entries, err := os.ReadDir(sm.stateDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read state dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		id := entry.Name()
		stateFile := filepath.Join(sm.stateDir, id, "state.json")

		data, err := os.ReadFile(stateFile)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			log.Printf("[state] warning: could not read state file for %s: %v", id, err)
			continue
		}

		var cs ContainerState
		if err := json.Unmarshal(data, &cs); err != nil {
			log.Printf("[state] warning: could not parse state file for %s: %v", id, err)
			continue
		}

		sm.states[id] = &cs
	}

	return nil
}
