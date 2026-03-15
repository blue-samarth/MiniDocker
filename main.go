package main

import (
	"fmt"
	"log"
	"os"

	"miniDocker/cmd"
	"miniDocker/container"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		if err := container.RunContainerInitProcess(os.Args[2:]); err != nil {
			log.Fatalf("[init] error: %v", err)
		}
	case "run":
		if err := cmd.Run(os.Args[2:]); err != nil {
			log.Fatalf("[run] error: %v", err)
		}
	case "ps":
		if err := cmd.Ps(os.Args[2:]); err != nil {
			log.Fatalf("[ps] error: %v", err)
		}
	case "logs":
		if err := cmd.Logs(os.Args[2:]); err != nil {
			log.Fatalf("[logs] error: %v", err)
		}
	case "stop":
		if err := cmd.Stop(os.Args[2:]); err != nil {
			log.Fatalf("[stop] error: %v", err)
		}
	case "rm":
		if err := cmd.Remove(os.Args[2:]); err != nil {
			log.Fatalf("[rm] error: %v", err)
		}
	case "inspect":
		if err := cmd.Inspect(os.Args[2:]); err != nil {
			log.Fatalf("[inspect] error: %v", err)
		}
	case "stats":
		if err := cmd.Stats(os.Args[2:]); err != nil {
			log.Fatalf("[stats] error: %v", err)
		}
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q — run 'miniDocker help' for usage\n", os.Args[1])
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Fprint(os.Stderr, `Usage: miniDocker <command> [options]

Container lifecycle:
  run     [options] <image-path> <cmd> [args...]   Start a new container
  stop    [-t <sec>] [-f] <id> [...]               Stop a running container
  rm      [-f] <id> [...]                          Remove a container

Inspection:
  ps      [-a] [-q] [--format table|json|ids]      List containers
  logs    [-f] [--tail N] [--timestamps] <id>      Fetch container logs
  inspect [--pretty=false] <id> [...]              Show container metadata
  stats   [-a] [--no-stream] [--interval ms] <id>  Display resource usage

run options:
  --memory  <limit>    Memory limit (e.g. 256m, 1g)
  --swap    <limit>    Swap limit (must be >= memory)
  --cpu     <frac>     CPU fraction (e.g. 0.5, 2.0)
  --cpu-weight <n>     CPU scheduling weight (1–10000)
  --pids    <n>        Max PIDs

Other:
  help / --help / -h                               Show this message
`)
}
