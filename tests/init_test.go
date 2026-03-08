package tests

import (
	"miniDocker/container"
	"testing"
)

func TestInitProcess_NoCommand(t *testing.T) {
	err := container.RunContainerInitProcess([]string{})
	if err == nil {
		t.Fatal("expected error when no command provided")
	}
	if err.Error() != "no command provided" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestInitProcess_InvalidCommand(t *testing.T) {
	err := container.RunContainerInitProcess([]string{"/nonexistent/command"})
	if err == nil {
		t.Fatal("expected error when command doesn't exist")
	}
}
