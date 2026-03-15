package state

// ContainerStatus represents the lifecycle state of a container.
type ContainerStatus string

const (
	StatusCreated ContainerStatus = "created"
	StatusRunning ContainerStatus = "running"
	StatusExited  ContainerStatus = "exited"
	StatusError   ContainerStatus = "error"
)

// ValidTransitions defines allowed state transitions.
var ValidTransitions = map[ContainerStatus][]ContainerStatus{
	StatusCreated: {StatusRunning, StatusError},
	StatusRunning: {StatusExited, StatusError},
	StatusExited:  {},
	StatusError:   {},
}

func (s ContainerStatus) IsTerminal() bool {
	return s == StatusExited || s == StatusError
}

// ContainerConfig holds the configuration used to create a container.
type ContainerConfig struct {
	Image    string
	Command  []string
	Args     []string
	Memory   string // e.g. "256m"
	CPU      string // e.g. "0.5"
	PIDs     int
	Swap     string
	Hostname string
}

// ErrContainerNotFound is returned when a container ID is not found.
type ErrContainerNotFound struct {
	ID string
}

func (e ErrContainerNotFound) Error() string {
	return "container " + e.ID + " not found"
}

// ErrInvalidStateTransition is returned for disallowed state changes.
type ErrInvalidStateTransition struct {
	From ContainerStatus
	To   ContainerStatus
}

func (e ErrInvalidStateTransition) Error() string {
	return "invalid transition: " + string(e.From) + " → " + string(e.To)
}
