package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"miniDocker/state"
)

func Ps(args []string) error {
	fs := flag.NewFlagSet("ps", flag.ContinueOnError)
	all := fs.Bool("a", false, "Show all containers (default: only running)")
	quiet := fs.Bool("q", false, "Only display container IDs")
	format := fs.String("format", "table", "Output format: table, json, ids")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *quiet {
		*format = "ids"
	}

	lm, err := getLifecycleManager()
	if err != nil {
		return err
	}

	containers, err := lm.ListContainers()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if !*all {
		var running []*state.ContainerState
		for _, c := range containers {
			if c.Status == state.StatusRunning {
				running = append(running, c)
			}
		}
		containers = running
	}

	// Newest first
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].CreatedAt.After(containers[j].CreatedAt)
	})

	switch strings.ToLower(*format) {
	case "json":
		return psJSON(containers)
	case "ids":
		return psIDs(containers)
	default:
		return psTable(containers)
	}
}

func psTable(containers []*state.ContainerState) error {
	const (
		wID      = 14
		wImage   = 22
		wCmd     = 22
		wStatus  = 10
		wPID     = 8
		wCreated = 12
	)

	fmt.Printf("%s%s%s%s%s%s%s\n",
		padRight("CONTAINER ID", wID),
		padRight("IMAGE", wImage),
		padRight("COMMAND", wCmd),
		padRight("STATUS", wStatus),
		padRight("PID", wPID),
		padRight("CREATED", wCreated),
		"UPTIME",
	)

	for _, c := range containers {
		// Show last path segment for image brevity
		img := c.Image
		if idx := strings.LastIndex(img, "/"); idx >= 0 {
			img = img[idx+1:]
		}
		fmt.Printf("%s%s%s%s%s%s%s\n",
			padRight(truncateID(c.ID, wID-2), wID),
			padRight(truncateStr(img, wImage-2), wImage),
			padRight(commandStr(c, wCmd-2), wCmd),
			padRight(string(c.Status), wStatus),
			padRight(pidStr(c), wPID),
			padRight(formatAge(c.CreatedAt), wCreated),
			formatUptime(c),
		)
	}

	if len(containers) == 0 {
		fmt.Println("(no containers)")
	}
	return nil
}

func psJSON(containers []*state.ContainerState) error {
	type row struct {
		ID      string `json:"id"`
		Image   string `json:"image"`
		Command string `json:"command"`
		Status  string `json:"status"`
		PID     string `json:"pid"`
		Created string `json:"created"`
		Uptime  string `json:"uptime"`
	}
	rows := make([]row, len(containers))
	for i, c := range containers {
		rows[i] = row{
			ID:      c.ID,
			Image:   c.Image,
			Command: commandStr(c, 200),
			Status:  string(c.Status),
			PID:     pidStr(c),
			Created: formatAge(c.CreatedAt),
			Uptime:  formatUptime(c),
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func psIDs(containers []*state.ContainerState) error {
	for _, c := range containers {
		fmt.Println(c.ID)
	}
	return nil
}
