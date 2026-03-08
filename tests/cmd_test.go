package tests

import (
	"miniDocker/cmd"
	"testing"
)

func TestCmdRun_InsufficientArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"empty args", []string{}},
		{"only image", []string{"/some/image"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cmd.Run(tt.args)
			if err == nil {
				t.Errorf("expected error with args %v", tt.args)
			}
		})
	}
}
