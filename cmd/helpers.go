package cmd

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miniDocker/state"
)

func getLifecycleManager() (*state.LifecycleManager, error) {
	sm, err := state.NewStateManager()
	if err != nil {
		return nil, fmt.Errorf("failed to init state manager: %w", err)
	}
	return state.NewLifecycleManager(sm), nil
}

func loadContainer(lm *state.LifecycleManager, id string) (*state.ContainerState, error) {
	cs, err := lm.GetState(id)
	if err != nil {
		return nil, fmt.Errorf("container %q not found", id)
	}
	return cs, nil
}

func truncateID(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return formatDuration(time.Since(t)) + " ago"
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}

func formatBytes(b int64) string {
	if b < 0 {
		return "-"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatPercent(numerator, denominator float64) string {
	if denominator == 0 {
		return "0.00%"
	}
	return fmt.Sprintf("%.2f%%", (numerator/denominator)*100)
}

func formatUptime(cs *state.ContainerState) string {
	if cs.Status != state.StatusRunning || cs.StartedAt.IsZero() {
		return "-"
	}
	return formatDuration(time.Since(cs.StartedAt))
}

func pidStr(cs *state.ContainerState) string {
	if cs.Status != state.StatusRunning || cs.Pid == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", cs.Pid)
}

func commandStr(cs *state.ContainerState, max int) string {
	if len(cs.Command) == 0 {
		return "-"
	}
	return truncateStr(strings.Join(cs.Command, " "), max)
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

func readCgroupFile(cgroupPath, filename string) (string, error) {
	data, err := os.ReadFile(filepath.Join(cgroupPath, filename))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func roundFloat(f float64, d int) float64 {
	pow := math.Pow(10, float64(d))
	return math.Round(f*pow) / pow
}

// Exported wrappers so package tests can access unexported helpers.
func TruncateID(id string, n int) string         { return truncateID(id, n) }
func TruncateStr(s string, max int) string       { return truncateStr(s, max) }
func FormatBytes(b int64) string                 { return formatBytes(b) }
func FormatDuration(d time.Duration) string      { return formatDuration(d) }
func FormatPercent(n, d float64) string          { return formatPercent(n, d) }
func PadRight(s string, w int) string            { return padRight(s, w) }
func ReadCgroupFile(p, f string) (string, error) { return readCgroupFile(p, f) }
