package tests

import (
	"miniDocker/container"
	"testing"
)

func TestRunContainer_InsufficientArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no args", []string{}},
		{"one arg", []string{"image"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := container.RunContainer(tt.args)
			if err == nil {
				t.Fatal("expected error with insufficient arguments")
			}
		})
	}
}
