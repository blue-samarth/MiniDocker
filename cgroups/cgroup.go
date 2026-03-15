package cgroups

import (
	"errors"
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
	defaultCPUPeriod = 100000
)

type CgroupConfig struct {
	ContainerID string
	Memory      string
	SwapMemory  string
	CPU         string
	CPUWeight   int
	PIDs        int
}

func (cfg CgroupConfig) Validate() error {
	if cfg.ContainerID == "" {
		return errors.New("ContainerID required")
	}

	if cfg.Memory != "" {
		mem, err := parseMemory(cfg.Memory)
		if err != nil {
			return err
		}
		if mem < minMemBytes {
			return errors.New("memory limit too low (min 4MiB)")
		}
	}

	if cfg.SwapMemory != "" {
		swap, err := parseMemory(cfg.SwapMemory)
		if err != nil {
			return err
		}
		if cfg.Memory != "" {
			mem, _ := parseMemory(cfg.Memory)
			if swap < mem {
				return errors.New("swap must be ≥ memory")
			}
		}
		if swap < minMemBytes {
			return errors.New("swap limit too low (min 4MiB)")
		}
	}

	if cfg.CPU != "" {
		v, err := strconv.ParseFloat(strings.TrimSpace(cfg.CPU), 64)
		if err != nil {
			return err
		}
		if v <= 0 {
			return errors.New("CPU must be positive")
		}
		if v > maxCPU {
			return errors.New("CPU exceeds maximum")
		}
	}

	if cfg.CPUWeight < 0 || cfg.CPUWeight > 10000 {
		return errors.New("CPUWeight out of range [0–10000]")
	}

	if cfg.PIDs < 0 {
		return errors.New("PIDs cannot be negative")
	}

	return nil
}

func (cfg CgroupConfig) Path() string {
	return filepath.Join(cgroupV2Root, "miniDocker", cfg.ContainerID)
}

func (cfg CgroupConfig) Setup(pid int) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}

	if err := enableControllers(); err != nil {
		return "", err
	}

	path := cfg.Path()
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}

	if err := cfg.applyLimits(path); err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}

	log.Printf("attaching pid %d to cgroup %s", pid, path)
	if err := os.WriteFile(filepath.Join(path, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0644); err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}

	return path, nil
}

func (cfg CgroupConfig) applyLimits(path string) error {
	write := func(file, value string) error {
		return os.WriteFile(filepath.Join(path, file), []byte(value), 0644)
	}

	if cfg.Memory != "" {
		bytes, _ := parseMemory(cfg.Memory)
		log.Printf("memory.max = %d", bytes)
		if err := write("memory.max", strconv.FormatInt(bytes, 10)); err != nil {
			return err
		}
	}

	if cfg.SwapMemory != "" {
		bytes, _ := parseMemory(cfg.SwapMemory)
		log.Printf("memory.swap.max = %d", bytes)
		if err := write("memory.swap.max", strconv.FormatInt(bytes, 10)); err != nil {
			return err
		}
	}

	if cfg.CPU != "" {
		val, _ := parseCPU(cfg.CPU)
		log.Printf("cpu.max = %s", val)
		if err := write("cpu.max", val); err != nil {
			return err
		}
	}

	if cfg.CPUWeight > 0 {
		log.Printf("cpu.weight = %d", cfg.CPUWeight)
		if err := write("cpu.weight", strconv.Itoa(cfg.CPUWeight)); err != nil {
			return err
		}
	}

	if cfg.PIDs > 0 {
		log.Printf("pids.max = %d", cfg.PIDs)
		if err := write("pids.max", strconv.Itoa(cfg.PIDs)); err != nil {
			return err
		}
	}

	return nil
}

func (cfg CgroupConfig) Reset() error {
	path := cfg.Path()
	log.Printf("resetting cgroup %s", path)

	type kv struct{ file, val string }
	for _, r := range []kv{
		{"memory.max", "max"},
		{"memory.swap.max", "max"},
		{"cpu.max", "max 100000"},
		{"cpu.weight", "100"},
		{"pids.max", "max"},
	} {
		f := filepath.Join(path, r.file)
		if _, err := os.Stat(f); err == nil {
			_ = os.WriteFile(f, []byte(r.val), 0644)
		}
	}
	return nil
}

func (cfg CgroupConfig) ResetMemory() error {
	path := cfg.Path()
	log.Printf("resetting memory on %s", path)

	for _, f := range []string{"memory.max", "memory.swap.max"} {
		p := filepath.Join(path, f)
		if _, err := os.Stat(p); err == nil {
			if err := os.WriteFile(p, []byte("max"), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func (cfg CgroupConfig) Cleanup() error {
	path := cfg.Path()
	log.Printf("removing cgroup %s", path)
	return os.RemoveAll(path)
}

func enableControllers() error {
	parent := filepath.Join(cgroupV2Root, "miniDocker")
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}

	controllers := "+memory +cpu +pids"

	// root level — best effort
	_ = os.WriteFile(filepath.Join(cgroupV2Root, "cgroup.subtree_control"), []byte(controllers), 0644)

	// miniDocker level — required
	if err := os.WriteFile(filepath.Join(parent, "cgroup.subtree_control"), []byte(controllers), 0644); err != nil {
		return err
	}
	return nil
}

func parseMemory(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	m := map[string]int64{"b": 1, "k": 1 << 10, "m": 1 << 20, "g": 1 << 30}

	for unit, mult := range m {
		if strings.HasSuffix(strings.ToLower(s), unit) {
			num := strings.TrimSuffix(strings.ToLower(s), unit)
			v, err := strconv.ParseInt(num, 10, 64)
			if err != nil || v <= 0 {
				return 0, errors.New("invalid memory string")
			}
			return v * mult, nil
		}
	}

	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 {
		return 0, errors.New("invalid memory string")
	}
	return v, nil
}

func parseCPU(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return "", err
	}

	quota := int64(float64(defaultCPUPeriod) * f)
	return strconv.FormatInt(quota, 10) + " " + strconv.FormatInt(defaultCPUPeriod, 10), nil
}
