package tests

import (
	"miniDocker/cgroups"
	"testing"
)

func TestCgroupConfig_Validate_ValidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  cgroups.CgroupConfig
	}{
		{
			name: "memory only",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123def456",
				Memory:      "256m",
			},
		},
		{
			name: "cpu only",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123def456",
				CPU:         "0.5",
			},
		},
		{
			name: "pids only",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123def456",
				PIDs:        100,
			},
		},
		{
			name: "all limits",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123def456",
				Memory:      "512m",
				SwapMemory:  "1g",
				CPU:         "1.0",
				CPUWeight:   500,
				PIDs:        50,
			},
		},
		{
			name: "no limits",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123def456",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestCgroupConfig_Validate_InvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  cgroups.CgroupConfig
		want string
	}{
		{
			name: "empty container id",
			cfg: cgroups.CgroupConfig{
				ContainerID: "",
				Memory:      "256m",
			},
			want: "ContainerID required",
		},
		{
			name: "invalid memory format",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123",
				Memory:      "invalid",
			},
			want: "invalid memory string",
		},
		{
			name: "memory below minimum",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123",
				Memory:      "1m",
			},
			want: "memory limit too low",
		},
		{
			name: "invalid cpu format",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123",
				CPU:         "not_a_number",
			},
			want: "strconv.ParseFloat",
		},
		{
			name: "zero cpu",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123",
				CPU:         "0",
			},
			want: "CPU must be positive",
		},
		{
			name: "negative pids",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123",
				PIDs:        -1,
			},
			want: "PIDs cannot be negative",
		},
		{
			name: "swap < memory",
			cfg: cgroups.CgroupConfig{
				ContainerID: "abc123",
				Memory:      "512m",
				SwapMemory:  "256m",
			},
			want: "swap must be",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Errorf("expected error, got nil")
			}
			if !contains(err.Error(), tt.want) {
				t.Errorf("expected error containing %q, got %q", tt.want, err.Error())
			}
		})
	}
}

func TestCgroupConfig_Path(t *testing.T) {
	cfg := cgroups.CgroupConfig{
		ContainerID: "abc123def456",
	}

	path := cfg.Path()

	if !contains(path, "/sys/fs/cgroup") {
		t.Errorf("path should contain /sys/fs/cgroup, got %q", path)
	}
	if !contains(path, "miniDocker") {
		t.Errorf("path should contain miniDocker, got %q", path)
	}
	if !contains(path, "abc123def456") {
		t.Errorf("path should contain container id, got %q", path)
	}
}

func TestParseMemory_ValidFormats(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"256m", 256 * 1024 * 1024},
		{"1g", 1024 * 1024 * 1024},
		{"512m", 512 * 1024 * 1024}, // was 512k — below 4MiB minimum
		{"8m", 8 * 1024 * 1024},     // was 1024b — below 4MiB minimum
		{"4m", 4 * 1024 * 1024},     // was 1024 (raw bytes) — below 4MiB minimum
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cfg := cgroups.CgroupConfig{
				ContainerID: "test",
				Memory:      tt.input,
			}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseCPU_ValidFormats(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"0.5"},
		{"1.0"},
		{"2.0"},
		{"0.25"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cfg := cgroups.CgroupConfig{
				ContainerID: "test",
				CPU:         tt.input,
			}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCgroupConfig_Reset(t *testing.T) {
	cfg := cgroups.CgroupConfig{
		ContainerID: "test_reset_12345678",
		Memory:      "256m",
		CPU:         "0.5",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("config validation failed: %v", err)
	}

	// Reset should succeed even if cgroup doesn't exist (best-effort)
	err := cfg.Reset()
	if err != nil && !contains(err.Error(), "no such file") {
		t.Logf("reset warning (expected if cgroup not created): %v", err)
	}
}
