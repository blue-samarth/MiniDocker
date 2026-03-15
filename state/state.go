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

type StateManager struct {
	stateDir string
	states   map[string]*ContainerState
	mu       sync.RWMutex
}

func NewStateManager() (*StateManager, error) {
	return NewStateManagerWithDir(stateBaseDir)
}

func NewStateManagerWithDir(dir string) (*StateManager, error) {
	sm := &StateManager{
		stateDir: dir,
		states:   make(map[string]*ContainerState),
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	if err := sm.loadAll(); err != nil {
		return nil, err
	}
	return sm, nil
}

func (sm *StateManager) CreateContainer(id string, cfg *ContainerConfig) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	containerDir := filepath.Join(sm.stateDir, id)
	logsDir := filepath.Join(containerDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return err
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

func (sm *StateManager) UpdateStatus(id string, status ContainerStatus) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cs, err := sm.get(id)
	if err != nil {
		return err
	}

	if !isValidTransition(cs.Status, status) {
		return &ErrInvalidStateTransition{From: cs.Status, To: status}
	}

	cs.Status = status
	if status == StatusRunning {
		cs.StartedAt = time.Now()
	}
	return sm.persist(id, cs)
}

func (sm *StateManager) SetPid(id string, pid int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cs, err := sm.get(id)
	if err != nil {
		return err
	}

	cs.Pid = pid

	pidFile := filepath.Join(sm.stateDir, id, "pid")
	_ = os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", pid)), 0644) // best effort

	return sm.persist(id, cs)
}

func (sm *StateManager) SetExit(id string, exitCode int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cs, err := sm.get(id)
	if err != nil {
		return err
	}

	if !isValidTransition(cs.Status, StatusExited) {
		return &ErrInvalidStateTransition{From: cs.Status, To: StatusExited}
	}

	cs.ExitCode = exitCode
	cs.FinishedAt = time.Now()
	cs.Status = StatusExited
	cs.Pid = 0

	return sm.persist(id, cs)
}

func (sm *StateManager) SetError(id string, errMsg string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	cs, err := sm.get(id)
	if err != nil {
		return err
	}

	if !isValidTransition(cs.Status, StatusError) {
		return &ErrInvalidStateTransition{From: cs.Status, To: StatusError}
	}

	cs.Error = errMsg
	cs.Status = StatusError
	cs.FinishedAt = time.Now()
	cs.Pid = 0

	return sm.persist(id, cs)
}

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

func (sm *StateManager) GetState(id string) (*ContainerState, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	cs, err := sm.get(id)
	if err != nil {
		return nil, err
	}

	copy := *cs
	return &copy, nil
}

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

func (sm *StateManager) RemoveContainer(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.states[id]; !ok {
		return &ErrContainerNotFound{ID: id}
	}

	delete(sm.states, id)

	containerDir := filepath.Join(sm.stateDir, id)
	if err := os.RemoveAll(containerDir); err != nil {
		return err
	}
	return nil
}

func (sm *StateManager) GetStateFile(id string) string {
	return filepath.Join(sm.stateDir, id, "state.json")
}

// ─── internal helpers ─────────────────────────────────────────────

func (sm *StateManager) get(id string) (*ContainerState, error) {
	cs, ok := sm.states[id]
	if !ok {
		return nil, &ErrContainerNotFound{ID: id}
	}
	return cs, nil
}

func (sm *StateManager) persist(id string, cs *ContainerState) error {
	stateFile := sm.GetStateFile(id)
	tmp := stateFile + ".tmp"

	data, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmp, stateFile); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (sm *StateManager) loadAll() error {
	entries, err := os.ReadDir(sm.stateDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
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
			log.Printf("could not read state file %s: %v", id, err)
			continue
		}

		var cs ContainerState
		if err := json.Unmarshal(data, &cs); err != nil {
			log.Printf("could not parse state file %s: %v", id, err)
			continue
		}

		sm.states[id] = &cs
	}
	return nil
}

func isValidTransition(from, to ContainerStatus) bool {
	allowed, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
