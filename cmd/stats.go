package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"miniDocker/state"
)

type statsSnapshot struct {
	ContainerID string  `json:"container_id"`
	ShortID     string  `json:"short_id"`
	Status      string  `json:"status"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemUsage    int64   `json:"mem_usage_bytes"`
	MemLimit    int64   `json:"mem_limit_bytes"`
	MemPercent  float64 `json:"mem_percent"`
	PIDs        int     `json:"pids"`
}

func Stats(args []string) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	all := fs.Bool("a", false, "Show all containers (default: running only)")
	noStream := fs.Bool("no-stream", false, "Print a single snapshot then exit")
	interval := fs.Int("interval", 1000, "Update interval in milliseconds")
	format := fs.String("format", "table", "Output format: table, json")

	if err := fs.Parse(args); err != nil {
		return err
	}

	lm, err := getLifecycleManager()
	if err != nil {
		return err
	}

	// Explicit IDs take precedence; otherwise pull from state.
	ids := fs.Args()
	if len(ids) == 0 {
		containers, err := lm.ListContainers()
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}
		for _, c := range containers {
			if *all || c.Status == state.StatusRunning {
				ids = append(ids, c.ID)
			}
		}
	}

	if len(ids) == 0 {
		fmt.Println("No containers found.")
		return nil
	}

	snapshot := func() {
		snaps := collectSnapshots(lm, ids)
		switch strings.ToLower(*format) {
		case "json":
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(snaps)
		default:
			printStatsTable(snaps)
		}
	}

	if *noStream {
		snapshot()
		return nil
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, unix.SIGINT, unix.SIGTERM)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(time.Duration(*interval) * time.Millisecond)
	defer ticker.Stop()

	for {
		fmt.Print("\033[H\033[2J") // clear screen
		snapshot()
		select {
		case <-sigCh:
			return nil
		case <-ticker.C:
		}
	}
}

func collectSnapshots(lm *state.LifecycleManager, ids []string) []statsSnapshot {
	snaps := make([]statsSnapshot, 0, len(ids))
	for _, id := range ids {
		cs, err := lm.GetState(id)
		if err != nil {
			continue
		}
		snaps = append(snaps, buildSnapshot(cs))
	}
	return snaps
}

func buildSnapshot(cs *state.ContainerState) statsSnapshot {
	snap := statsSnapshot{
		ContainerID: cs.ID,
		ShortID:     truncateID(cs.ID, 12),
		Status:      string(cs.Status),
	}

	if cs.CgroupPath == "" || cs.Status != state.StatusRunning {
		return snap
	}

	cg := cs.CgroupPath

	if v, _ := readCgroupFile(cg, "memory.current"); v != "" && v != "max" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			snap.MemUsage = n
		}
	}

	if v, _ := readCgroupFile(cg, "memory.max"); v != "" && v != "max" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			snap.MemLimit = n
		}
	}

	if snap.MemLimit > 0 {
		snap.MemPercent = roundFloat(float64(snap.MemUsage)/float64(snap.MemLimit)*100, 2)
	}

	snap.CPUPercent = cpuPercent(cg)

	if v, _ := readCgroupFile(cg, "pids.current"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			snap.PIDs = n
		}
	}

	return snap
}

// cpuPercent samples cpu.stat usage_usec 100ms apart and returns utilisation %.
func cpuPercent(cgroupPath string) float64 {
	t1 := readUsageUsec(cgroupPath)
	start := time.Now()
	time.Sleep(100 * time.Millisecond)
	t2 := readUsageUsec(cgroupPath)
	elapsed := time.Since(start).Microseconds()

	if t1 < 0 || t2 <= t1 || elapsed <= 0 {
		return 0
	}
	return roundFloat(float64(t2-t1)/float64(elapsed)*100, 2)
}

func readUsageUsec(cgroupPath string) int64 {
	v, err := readCgroupFile(cgroupPath, "cpu.stat")
	if err != nil || v == "" {
		return -1
	}
	for _, line := range strings.Split(v, "\n") {
		if strings.HasPrefix(line, "usage_usec ") {
			if n, err := strconv.ParseInt(strings.Fields(line)[1], 10, 64); err == nil {
				return n
			}
		}
	}
	return -1
}

func printStatsTable(snaps []statsSnapshot) {
	const (
		wID     = 14
		wStatus = 10
		wCPU    = 10
		wMem    = 24
		wMemPct = 10
	)

	fmt.Printf("%s%s%s%s%s%s\n",
		padRight("CONTAINER ID", wID),
		padRight("STATUS", wStatus),
		padRight("CPU %", wCPU),
		padRight("MEM USAGE / LIMIT", wMem),
		padRight("MEM %", wMemPct),
		"PIDS",
	)

	for _, s := range snaps {
		memStr := "-"
		if s.MemLimit > 0 {
			memStr = fmt.Sprintf("%s / %s", formatBytes(s.MemUsage), formatBytes(s.MemLimit))
		} else if s.MemUsage > 0 {
			memStr = fmt.Sprintf("%s / -", formatBytes(s.MemUsage))
		}

		cpuStr := "-"
		pidStr := "-"
		memPctStr := "-"
		if s.Status == string(state.StatusRunning) {
			cpuStr = fmt.Sprintf("%.2f%%", s.CPUPercent)
			if s.PIDs > 0 {
				pidStr = strconv.Itoa(s.PIDs)
			}
			if s.MemLimit > 0 {
				memPctStr = fmt.Sprintf("%.2f%%", s.MemPercent)
			}
		}

		fmt.Printf("%s%s%s%s%s%s\n",
			padRight(s.ShortID, wID),
			padRight(s.Status, wStatus),
			padRight(cpuStr, wCPU),
			padRight(memStr, wMem),
			padRight(memPctStr, wMemPct),
			pidStr,
		)
	}

	if len(snaps) == 0 {
		fmt.Println("(no containers)")
	}
}
