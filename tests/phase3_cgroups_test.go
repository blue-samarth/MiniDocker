package tests

import (
	"miniDocker/cgroups"
	"strings"
	"testing"
)

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func randHex(n int) string {
	const chars = "0123456789abcdef"
	s := make([]byte, n)
	for i := 0; i < n; i++ {
		s[i] = chars[i%len(chars)]
	}
	return string(s)
}

func TestPhase3_CgroupValidation_Memory(t *testing.T) {
	tests := []struct {
		name    string
		memory  string
		wantErr bool
	}{
		{"256 MB", "256m", false},
		{"1 GB", "1g", false},
		{"512 MB", "512m", false}, // was "512k" — 512 KiB is below the 4MiB minimum
		{"invalid", "invalid", true},
		{"below minimum", "1m", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cgroups.CgroupConfig{
				ContainerID: "test",
				Memory:      tt.memory,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPhase3_CgroupValidation_CPU(t *testing.T) {
	tests := []struct {
		name    string
		cpu     string
		wantErr bool
	}{
		{"half core", "0.5", false},
		{"one core", "1.0", false},
		{"two cores", "2.0", false},
		{"invalid", "not_a_number", true},
		{"zero", "0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cgroups.CgroupConfig{
				ContainerID: "test",
				CPU:         tt.cpu,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPhase3_CgroupValidation_PIDs(t *testing.T) {
	tests := []struct {
		name    string
		pids    int
		wantErr bool
	}{
		{"50 PIDs", 50, false},
		{"1000 PIDs", 1000, false},
		{"0 PIDs (unlimited)", 0, false},
		{"negative PIDs", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cgroups.CgroupConfig{
				ContainerID: "test",
				PIDs:        tt.pids,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPhase3_CgroupValidation_Swap(t *testing.T) {
	tests := []struct {
		name       string
		memory     string
		swap       string
		wantErr    bool
		errorCheck string
	}{
		{
			name:    "swap >= memory",
			memory:  "256m",
			swap:    "512m",
			wantErr: false,
		},
		{
			name:       "swap < memory",
			memory:     "512m",
			swap:       "256m",
			wantErr:    true,
			errorCheck: "swap limit",
		},
		{
			name:    "swap == memory",
			memory:  "256m",
			swap:    "256m",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cgroups.CgroupConfig{
				ContainerID: "test",
				Memory:      tt.memory,
				SwapMemory:  tt.swap,
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errorCheck != "" && err != nil {
				if !contains(err.Error(), tt.errorCheck) {
					t.Errorf("expected error containing %q, got %q", tt.errorCheck, err.Error())
				}
			}
		})
	}
}

func TestPhase3_CgroupConfig_Path(t *testing.T) {
	containerID := "abc123def456"
	cfg := cgroups.CgroupConfig{
		ContainerID: containerID,
	}

	path := cfg.Path()

	if !contains(path, "/sys/fs/cgroup") {
		t.Errorf("path should contain /sys/fs/cgroup, got %q", path)
	}
	if !contains(path, "miniDocker") {
		t.Errorf("path should contain miniDocker, got %q", path)
	}
	if !contains(path, containerID) {
		t.Errorf("path should contain container ID %q, got %q", containerID, path)
	}
}

func TestPhase3_CgroupConfig_EmptyConfig(t *testing.T) {
	cfg := cgroups.CgroupConfig{
		ContainerID: "test",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("empty config should be valid, got error: %v", err)
	}
}

func TestPhase3_CgroupConfig_AllLimits(t *testing.T) {
	cfg := cgroups.CgroupConfig{
		ContainerID: "test",
		Memory:      "512m",
		SwapMemory:  "1g",
		CPU:         "1.0",
		CPUWeight:   500,
		PIDs:        100,
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("config with all limits should be valid, got error: %v", err)
	}
}
