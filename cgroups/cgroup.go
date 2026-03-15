package cgroups

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	cgroupV2Root     = "/sys/fs/cgroup"
	maxCPU           = 128.0
	minMemBytes      = 4 * 1024 * 1024
	defaultCPUPeriod = int64(100000)
)

type CgroupConfig struct {
	ContainerID string
	Memory      string
	SwapMemory  string
	CPU         string
	CPUWeight   int
	PIDs        int
}

// Validate checks the config for invalid values before Setup is called.
func (cfg CgroupConfig) Validate() error {
	if cfg.ContainerID == "" {
		return errors.New("ContainerID must not be empty")
	}

	if cfg.Memory != "" {
		mem, err := parseMemory(cfg.Memory)
		if err != nil {
			return fmt.Errorf("invalid Memory: %w", err)
		}
		if mem < minMemBytes {
			return fmt.Errorf("memory limit %d bytes is below minimum %d bytes (4MiB)", mem, minMemBytes)
		}
	}

	if cfg.SwapMemory != "" {
		swap, err := parseMemory(cfg.SwapMemory)
		if err != nil {
			return fmt.Errorf("invalid SwapMemory: %w", err)
		}
		if cfg.Memory != "" {
			mem, _ := parseMemory(cfg.Memory)
			if swap < mem {
				return fmt.Errorf("swap limit (%d) must be >= memory limit (%d)", swap, mem)
			}
		}
		if swap < minMemBytes {
			return fmt.Errorf("swap limit %d bytes is below minimum %d bytes (4MiB)", swap, minMemBytes)
		}
	}

	if cfg.CPU != "" {
		cpuVal, err := strconv.ParseFloat(strings.TrimSpace(cfg.CPU), 64)
		if err != nil {
			return fmt.Errorf("invalid CPU value %q: %w", cfg.CPU, err)
		}
		if cpuVal <= 0 {
			return fmt.Errorf("CPU must be positive, got %f", cpuVal)
		}
		if cpuVal > maxCPU {
			return fmt.Errorf("CPU value %f exceeds maximum of %g", cpuVal, maxCPU)
		}
	}

	if cfg.CPUWeight < 0 || cfg.CPUWeight > 10000 {
		return fmt.Errorf("CPUWeight must be between 0 and 10000, got %d", cfg.CPUWeight)
	}

	if cfg.PIDs < 0 {
		return fmt.Errorf("PIDs must be non-negative, got %d", cfg.PIDs)
	}

	return nil
}

// Path returns the cgroup directory for this container.
func (cfg CgroupConfig) Path() string {
	return filepath.Join(cgroupV2Root, "miniDocker", cfg.ContainerID)
}

// Setup validates the config, enables controllers, applies limits, and
// attaches pid to the cgroup. Returns the cgroup path on success.
func (cfg CgroupConfig) Setup(pid int) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", fmt.Errorf("invalid cgroup config: %w", err)
	}

	if err := enableControllers(); err != nil {
		return "", err
	}

	cgroupPath := cfg.Path()
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create cgroup directory %q: %w", cgroupPath, err)
	}

	if err := cfg.applyLimits(cgroupPath); err != nil {
		os.RemoveAll(cgroupPath)
		return "", err
	}

	log.Printf("[cgroup] attaching PID %d to %s", pid, cgroupPath)
	if err := writeFile(filepath.Join(cgroupPath, "cgroup.procs"), fmt.Sprintf("%d", pid)); err != nil {
		os.RemoveAll(cgroupPath)
		return "", fmt.Errorf("failed to attach process to cgroup: %w", err)
	}

	return cgroupPath, nil
}

func (cfg CgroupConfig) applyLimits(cgroupPath string) error {
	// Memory limit
	if cfg.Memory != "" {
		memBytes, _ := parseMemory(cfg.Memory) // already validated
		log.Printf("[cgroup] setting memory.max = %d bytes", memBytes)
		if err := writeFile(filepath.Join(cgroupPath, "memory.max"), fmt.Sprintf("%d", memBytes)); err != nil {
			return fmt.Errorf("failed to set memory limit: %w", err)
		}
	}

	// Swap limit
	if cfg.SwapMemory != "" {
		swapBytes, _ := parseMemory(cfg.SwapMemory)
		log.Printf("[cgroup] setting memory.swap.max = %d bytes", swapBytes)
		if err := writeFile(filepath.Join(cgroupPath, "memory.swap.max"), fmt.Sprintf("%d", swapBytes)); err != nil {
			return fmt.Errorf("failed to set swap limit: %w", err)
		}
	}

	// CPU quota
	if cfg.CPU != "" {
		cpuMax, _ := parseCPU(cfg.CPU)
		log.Printf("[cgroup] setting cpu.max = %s", cpuMax)
		if err := writeFile(filepath.Join(cgroupPath, "cpu.max"), cpuMax); err != nil {
			return fmt.Errorf("failed to set CPU limit: %w", err)
		}
	}

	// CPU weight (alternative to quota — relative share)
	if cfg.CPUWeight > 0 {
		log.Printf("[cgroup] setting cpu.weight = %d", cfg.CPUWeight)
		if err := writeFile(filepath.Join(cgroupPath, "cpu.weight"), fmt.Sprintf("%d", cfg.CPUWeight)); err != nil {
			return fmt.Errorf("failed to set CPU weight: %w", err)
		}
	}

	// PID limit
	if cfg.PIDs > 0 {
		log.Printf("[cgroup] setting pids.max = %d", cfg.PIDs)
		if err := writeFile(filepath.Join(cgroupPath, "pids.max"), fmt.Sprintf("%d", cfg.PIDs)); err != nil {
			return fmt.Errorf("failed to set PIDs limit: %w", err)
		}
	}

	return nil
}

// Reset clears all resource limits on the cgroup back to kernel defaults.
func (cfg CgroupConfig) Reset() error {
	cgroupPath := cfg.Path()
	log.Printf("[cgroup] resetting all limits for %s", cgroupPath)

	type resetSpec struct {
		file  string
		value string
	}

	resets := []resetSpec{
		{"memory.max", "max"},
		{"memory.swap.max", "max"},
		{"cpu.max", "max 100000"},
		{"cpu.weight", "100"},
		{"pids.max", "max"},
	}

	var errs []string
	for _, r := range resets {
		path := filepath.Join(cgroupPath, r.file)
		if _, err := os.Stat(path); err == nil {
			if err := writeFile(path, r.value); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", r.file, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors resetting cgroup limits: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ResetMemory clears only the memory limits back to defaults.
func (cfg CgroupConfig) ResetMemory() error {
	cgroupPath := cfg.Path()
	log.Printf("[cgroup] resetting memory limits for %s", cgroupPath)

	for _, f := range []string{"memory.max", "memory.swap.max"} {
		path := filepath.Join(cgroupPath, f)
		if _, err := os.Stat(path); err == nil {
			if err := writeFile(path, "max"); err != nil {
				return fmt.Errorf("failed to reset %s: %w", f, err)
			}
		}
	}
	return nil
}

// Cleanup removes the cgroup directory. Should be called after the container exits.
func (cfg CgroupConfig) Cleanup() error {
	cgroupPath := cfg.Path()
	log.Printf("[cgroup] removing cgroup %s", cgroupPath)
	if err := os.RemoveAll(cgroupPath); err != nil {
		return fmt.Errorf("failed to remove cgroup %q: %w", cgroupPath, err)
	}
	return nil
}

// enableControllers ensures memory, cpu, and pids controllers are available
// in the miniDocker parent cgroup.
func enableControllers() error {
	parentPath := filepath.Join(cgroupV2Root, "miniDocker")
	if err := os.MkdirAll(parentPath, 0755); err != nil {
		return fmt.Errorf("failed to create parent cgroup dir: %w", err)
	}

	controllers := "+memory +cpu +pids"

	// Best-effort at root — may fail without CAP_SYS_ADMIN, that's OK.
	if err := writeFile(filepath.Join(cgroupV2Root, "cgroup.subtree_control"), controllers); err != nil {
		log.Printf("[cgroup] warning: could not enable controllers at root (may need elevated privileges): %v", err)
	}

	// Must succeed at the miniDocker level.
	if err := writeFile(filepath.Join(parentPath, "cgroup.subtree_control"), controllers); err != nil {
		return fmt.Errorf("failed to enable cgroup controllers: %w", err)
	}

	return nil
}

// parseMemory converts a human-readable memory string to bytes.
// Returns 0 for empty string (no limit).
func parseMemory(memStr string) (int64, error) {
	if memStr == "" {
		return 0, nil
	}

	memStr = strings.TrimSpace(memStr)
	multipliers := map[string]int64{
		"b": 1,
		"k": 1024,
		"m": 1024 * 1024,
		"g": 1024 * 1024 * 1024,
	}

	for unit, mult := range multipliers {
		if strings.HasSuffix(strings.ToLower(memStr), unit) {
			numStr := strings.TrimSuffix(strings.ToLower(memStr), unit)
			val, err := strconv.ParseInt(numStr, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid memory value: %s", memStr)
			}
			if val <= 0 {
				return 0, fmt.Errorf("memory value must be positive: %s", memStr)
			}
			return val * mult, nil
		}
	}

	val, err := strconv.ParseInt(memStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value: %s", memStr)
	}
	if val <= 0 {
		return 0, fmt.Errorf("memory value must be positive: %s", memStr)
	}
	return val, nil
}

// parseCPU converts a CPU fraction string to the "quota period" format
// used by cpu.max.
func parseCPU(cpuStr string) (string, error) {
	if cpuStr == "" {
		return "", nil
	}

	cpuVal, err := strconv.ParseFloat(strings.TrimSpace(cpuStr), 64)
	if err != nil {
		return "", fmt.Errorf("invalid CPU value: %s", cpuStr)
	}

	quota := int64(float64(defaultCPUPeriod) * cpuVal)
	return fmt.Sprintf("%d %d", quota, defaultCPUPeriod), nil
}

func writeFile(path string, data string) error {
	return os.WriteFile(path, []byte(data), 0644)
}
