package container

import (
	"fmt"
	"log"
	"miniDocker/fs"
	"miniDocker/network"
	"miniDocker/security"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func RunContainerInitProcess(args []string) error {
	log.Printf("[init] starting inside container namespaces, args: %v", args)

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

	// Step 1: Mount overlay filesystem
	if err := fs.MountOverlay(fs.OverlayConfig{
		Lower:  imagePath,
		Upper:  upperDir,
		Work:   workDir,
		Merged: mergedDir,
	}); err != nil {
		return fmt.Errorf("failed to mount overlay: %w", err)
	}

	// Step 2: Pivot root into container filesystem
	if err := fs.PivotRoot(mergedDir); err != nil {
		return fmt.Errorf("failed to pivot root: %w", err)
	}

	// Step 3: Mount essential filesystems
	if err := fs.MountEssentials(); err != nil {
		return fmt.Errorf("failed to mount essentials: %w", err)
	}

	// Step 4: Set hostname
	hostname := "container"
	log.Printf("[init] setting hostname to %q", hostname)
	if err := unix.Sethostname([]byte(hostname)); err != nil {
		log.Printf("[init] failed to set hostname: %v", err)
		return err
	}

	// Step 5: Configure container networking
	containerIP := os.Getenv("CONTAINER_IP")
	gateway := os.Getenv("CONTAINER_GATEWAY")
	veth := os.Getenv("CONTAINER_VETH")
	if containerIP != "" && gateway != "" && veth != "" {
		log.Printf("[init] configuring network: ip=%s gateway=%s iface=%s", containerIP, gateway, veth)
		if err := network.ConfigureContainerNetwork(containerIP, gateway, veth); err != nil {
			log.Printf("[init] warning: failed to configure network: %v", err)
		}
	} else {
		log.Printf("[init] no network configuration provided, skipping")
	}

	// Step 6: Mask sensitive kernel paths
	if err := security.MaskPaths(); err != nil {
		// Non-fatal — log and continue
		log.Printf("[init] warning: path masking incomplete: %v", err)
	}

	// Step 7: Drop dangerous capabilities
	if err := security.DropCapabilities(); err != nil {
		return fmt.Errorf("failed to drop capabilities: %w", err)
	}

	// Step 8: Apply seccomp whitelist filter
	// Must be after PR_SET_NO_NEW_PRIVS which is set in DropCapabilities.
	if err := security.ApplySeccompFilter(); err != nil {
		return fmt.Errorf("failed to apply seccomp filter: %w", err)
	}

	// Step 9: Exec user command
	cmd := args[0]
	cmdArgs := args[1:]
	log.Printf("[init] executing command: %s with args: %v", cmd, cmdArgs)
	if err := unix.Exec(cmd, append([]string{cmd}, cmdArgs...), os.Environ()); err != nil {
		log.Printf("[init] exec failed: %v", err)
		return fmt.Errorf("exec failed: %v", err)
	}

	return nil
}
