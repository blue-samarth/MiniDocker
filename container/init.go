//go:build linux

package container

import (
	"fmt"
	"log"
	"miniDocker/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func RunContainerInitProcess(args []string) error {
	log.Printf("[init] starting inside container namespaces, args: %v", args)

	// Validate arguments before performing any side effects
	if len(args) == 0 {
		log.Printf("[init] no command provided")
		return fmt.Errorf("no command provided")
	}

	containerID := os.Getenv("CONTAINER_ID")
	imagePath := os.Getenv("CONTAINER_IMAGE")
	if containerID == "" {
		return fmt.Errorf("missing CONTAINER_ID")
	}
	if imagePath == "" {
		return fmt.Errorf("missing CONTAINER_IMAGE")
	}

	containerDir := filepath.Join(containersBaseDir, containerID)
	upperDir := os.Getenv("CONTAINER_UPPER")
	workDir := os.Getenv("CONTAINER_WORK")
	mergedDir := os.Getenv("CONTAINER_MERGED")

	if upperDir == "" {
		upperDir = filepath.Join(containerDir, "upper")
	}
	if workDir == "" {
		workDir = filepath.Join(containerDir, "work")
	}
	if mergedDir == "" {
		mergedDir = filepath.Join(containerDir, "merged")
	}

	if err := fs.MountOverlay(fs.OverlayConfig{
		Lower:  imagePath,
		Upper:  upperDir,
		Work:   workDir,
		Merged: mergedDir,
	}); err != nil {
		return fmt.Errorf("failed to mount overlay: %w", err)
	}

	if err := fs.PivotRoot(mergedDir); err != nil {
		return fmt.Errorf("failed to pivot root: %w", err)
	}

	if err := fs.MountEssentials(); err != nil {
		return fmt.Errorf("failed to mount essentials: %w", err)
	}

	// Set hostname after validation
	hostname := "container"
	log.Printf("[init] setting hostname to %q", hostname)
	if err := unix.Sethostname([]byte(hostname)); err != nil {
		log.Printf("[init] failed to set hostname: %v", err)
		return err
	}

	cmd := args[0]
	cmdArgs := args[1:]

	log.Printf("[init] executing command: %s with args: %v", cmd, cmdArgs)
	if err := unix.Exec(cmd, append([]string{cmd}, cmdArgs...), os.Environ()); err != nil {
		log.Printf("[init] exec failed: %v", err)
		return fmt.Errorf("exec failed: %v", err)
	}

	// Never reached if exec succeeds
	return nil
}
