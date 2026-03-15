package container

import (
	"errors"
	"log"
	"os"
	"path/filepath"

	"miniDocker/fs"
	"miniDocker/network"
	"miniDocker/security"

	"golang.org/x/sys/unix"
)

func RunContainerInitProcess(args []string) error {
	if len(args) == 0 {
		return errors.New("no command provided")
	}

	id := os.Getenv("CONTAINER_ID")
	image := os.Getenv("CONTAINER_IMAGE")
	if id == "" || image == "" {
		return errors.New("missing CONTAINER_ID or CONTAINER_IMAGE")
	}

	upper := os.Getenv("CONTAINER_UPPER")
	work := os.Getenv("CONTAINER_WORK")
	merged := os.Getenv("CONTAINER_MERGED")

	if upper == "" {
		upper = filepath.Join("/var/lib/miniDocker/containers", id, "upper")
	}
	if work == "" {
		work = filepath.Join("/var/lib/miniDocker/containers", id, "work")
	}
	if merged == "" {
		merged = filepath.Join("/var/lib/miniDocker/containers", id, "merged")
	}

	// Mount overlay filesystem
	if err := fs.MountOverlay(fs.OverlayConfig{
		Lower:  image,
		Upper:  upper,
		Work:   work,
		Merged: merged,
	}); err != nil {
		return err
	}

	// Pivot root
	if err := fs.PivotRoot(merged); err != nil {
		return err
	}

	// Mount /proc, /sys, /dev, etc.
	if err := fs.MountEssentials(); err != nil {
		return err
	}

	// Set basic hostname
	_ = unix.Sethostname([]byte("container"))

	// Configure networking if env vars present
	ip := os.Getenv("CONTAINER_IP")
	gw := os.Getenv("CONTAINER_GATEWAY")
	veth := os.Getenv("CONTAINER_VETH")
	if ip != "" && gw != "" && veth != "" {
		log.Printf("configuring network: ip=%s gw=%s iface=%s", ip, gw, veth)
		if err := network.ConfigureContainerNetwork(ip, gw, veth); err != nil {
			log.Printf("network config failed (non-fatal): %v", err)
		}
	}

	// Security hardening
	if err := security.MaskPaths(); err != nil {
		log.Printf("path masking incomplete: %v", err)
	}

	if err := security.DropCapabilities(); err != nil {
		return err
	}

	if err := security.ApplySeccompFilter(); err != nil {
		return err
	}

	// Replace process with user command
	cmd := args[0]
	cmdArgs := args[1:]
	log.Printf("executing: %s %v", cmd, cmdArgs)

	return unix.Exec(cmd, append([]string{cmd}, cmdArgs...), os.Environ())
}
