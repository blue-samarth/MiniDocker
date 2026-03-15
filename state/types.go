package state

import "fmt"

// ContainerStatus represents the lifecycle state of a container.
type ContainerStatus string

const (
	StatusCreated ContainerStatus = "created"
	StatusRunning ContainerStatus = "running"
	StatusExited  ContainerStatus = "exited"
	StatusError   ContainerStatus = "error"
)

// ValidTransitions defines which state transitions are allowed.
var ValidTransitions = map[ContainerStatus][]ContainerStatus{
	StatusCreated: {StatusRunning, StatusError},
	StatusRunning: {StatusExited, StatusError},
	StatusExited:  {},
	StatusError:   {},
}

// IsTerminal returns true if the status is a final state.
func (s ContainerStatus) IsTerminal() bool {
	return s == StatusExited || s == StatusError
}

// ContainerConfig holds the configuration used to create a container.
type ContainerConfig struct {
	Image    string   // Root filesystem path
	Command  []string // Command to execute
	Args     []string // Command arguments
	Memory   string   // Memory limit (e.g., "256m")
	CPU      string   // CPU limit (e.g., "0.5")
	PIDs     int      // Max PIDs
	Swap     string   // Swap limit
	Hostname string   // Container hostname
}

// ErrContainerNotFound is returned when a container ID is not found.
type ErrContainerNotFound struct {
	ID string
}

func (e *ErrContainerNotFound) Error() string {
	return fmt.Sprintf("container %q not found", e.ID)
}

// ErrInvalidStateTransition is returned when a transition is not allowed.
type ErrInvalidStateTransition struct {
	From ContainerStatus
	To   ContainerStatus
}

func (e *ErrInvalidStateTransition) Error() string {
	return fmt.Sprintf("invalid state transition: %s → %s", e.From, e.To)
}
