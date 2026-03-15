package cmd

import (
	"flag"
	"fmt"
	"log"
	"miniDocker/cgroups"
	"miniDocker/container"
)

func Run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)

	memory := fs.String("memory", "", "memory limit (e.g., 256m, 1g)")
	swap := fs.String("swap", "", "swap memory limit (e.g., 512m, must be >= memory)")
	cpu := fs.String("cpu", "", "CPU quota as fraction of one core (e.g., 0.5, 2.0)")
	cpuWeight := fs.Int("cpu-weight", 0, "CPU weight for relative scheduling (1-10000, default 100)")
	pids := fs.Int("pids", 0, "max PIDs (0 = unlimited)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) < 2 {
		log.Printf("[cmd] usage: run [--memory <limit>] [--swap <limit>] [--cpu <limit>] [--cpu-weight <n>] [--pids <n>] <image-path> <command> [args...]")
		return fmt.Errorf("insufficient arguments")
	}

	cgroupCfg := cgroups.CgroupConfig{
		Memory:     *memory,
		SwapMemory: *swap,
		CPU:        *cpu,
		CPUWeight:  *cpuWeight,
		PIDs:       *pids,
	}

	return container.RunContainer(remaining, cgroupCfg)
}
