package main

import (
	"fmt"
	"log"
	"miniDocker/cmd"
	"miniDocker/container"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <command> [args...]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  run <image-path> <cmd> [args...]   start a new container\n")
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
	default:
		log.Fatalf("unknown command %q — try 'run'", os.Args[1])
	}
}
