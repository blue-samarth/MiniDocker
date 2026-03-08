package container

import (
	"fmt"
	"log"
	"os"
	"syscall"
)

func RunContainerInitProcess(args []string) error {
	log.Printf("[init] starting inside container namespaces, args: %v", args)

	// Validate arguments before performing any side effects
	if len(args) == 0 {
		log.Printf("[init] no command provided")
		return fmt.Errorf("no command provided")
	}

	// Set hostname after validation
	hostname := "container"
	log.Printf("[init] setting hostname to %q", hostname)
	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		log.Printf("[init] failed to set hostname: %v", err)
		return err
	}

	cmd := args[0]
	cmdArgs := args[1:]

	log.Printf("[init] executing command: %s with args: %v", cmd, cmdArgs)
	if err := syscall.Exec(cmd, append([]string{cmd}, cmdArgs...), os.Environ()); err != nil {
		log.Printf("[init] exec failed: %v", err)
		return fmt.Errorf("exec failed: %v", err)
	}

	// Never reached if exec succeeds
	return nil
}
