package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"miniDocker/state"
)

type inspectOutput struct {
	ID         string     `json:"ID"`
	Image      string     `json:"Image"`
	Command    []string   `json:"Command"`
	State      string     `json:"State"`
	PID        int        `json:"PID"`
	ExitCode   int        `json:"ExitCode"`
	Error      string     `json:"Error,omitempty"`
	Uptime     string     `json:"Uptime"`
	CreatedAt  time.Time  `json:"CreatedAt"`
	StartedAt  *time.Time `json:"StartedAt,omitempty"`
	FinishedAt *time.Time `json:"FinishedAt,omitempty"`
	Resources  struct {
		Memory     string `json:"Memory,omitempty"`
		CPU        string `json:"CPU,omitempty"`
		PIDs       int    `json:"PIDs,omitempty"`
		Swap       string `json:"Swap,omitempty"`
		CgroupPath string `json:"CgroupPath,omitempty"`
	} `json:"Resources"`
	Network struct {
		IPAddress string `json:"IPAddress,omitempty"`
		Gateway   string `json:"Gateway,omitempty"`
		Bridge    string `json:"Bridge,omitempty"`
	} `json:"Network"`
	Paths struct {
		RootFS    string `json:"RootFS"`
		StateDir  string `json:"StateDir"`
		StdoutLog string `json:"StdoutLog"`
		StderrLog string `json:"StderrLog"`
	} `json:"Paths"`
}

func Inspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	pretty := fs.Bool("pretty", true, "Pretty-print JSON (default true)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	ids := fs.Args()
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: miniDocker inspect [--pretty=false] <container-id> [...]")
		return fmt.Errorf("at least one container ID required")
	}

	lm, err := getLifecycleManager()
	if err != nil {
		return err
	}

	var outputs []inspectOutput
	var hadErr bool

	for _, id := range ids {
		cs, err := loadContainer(lm, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			hadErr = true
			continue
		}
		outputs = append(outputs, buildInspect(cs))
	}

	if len(outputs) == 0 {
		if hadErr {
			return fmt.Errorf("no containers found")
		}
		return nil
	}

	enc := json.NewEncoder(os.Stdout)
	if *pretty {
		enc.SetIndent("", "  ")
	}

	if len(outputs) == 1 {
		return enc.Encode(outputs[0])
	}
	return enc.Encode(outputs)
}

func buildInspect(cs *state.ContainerState) inspectOutput {
	var out inspectOutput
	out.ID = cs.ID
	out.Image = cs.Image
	out.Command = cs.Command
	out.State = string(cs.Status)
	out.PID = cs.Pid
	out.ExitCode = cs.ExitCode
	out.Error = cs.Error
	out.Uptime = formatUptime(cs)
	out.CreatedAt = cs.CreatedAt

	if !cs.StartedAt.IsZero() {
		t := cs.StartedAt
		out.StartedAt = &t
	}
	if !cs.FinishedAt.IsZero() {
		t := cs.FinishedAt
		out.FinishedAt = &t
	}

	out.Resources.Memory = cs.Memory
	out.Resources.CPU = cs.CPU
	out.Resources.PIDs = cs.PIDs
	out.Resources.Swap = cs.Swap
	out.Resources.CgroupPath = cs.CgroupPath

	out.Network.IPAddress = cs.IPAddress
	out.Network.Gateway = cs.Gateway
	out.Network.Bridge = cs.Bridge

	out.Paths.RootFS = cs.RootFS
	out.Paths.StateDir = cs.StateDir
	out.Paths.StdoutLog = cs.StdoutPath
	out.Paths.StderrLog = cs.StderrPath

	return out
}
