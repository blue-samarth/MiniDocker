package security

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/sys/unix"
)

// sensitivePaths are masked by bind-mounting /dev/null over them,
// preventing containers from reading or writing kernel internals.
var sensitivePaths = []string{
	"/proc/kcore",          // raw kernel memory
	"/proc/kmem",           // kernel memory
	"/proc/mem",            // physical memory
	"/proc/sysrq-trigger",  // trigger sysrq actions
	"/proc/acpi",           // ACPI interface
	"/proc/timer_list",     // kernel timer internals
	"/proc/timer_stats",    // timer statistics
	"/proc/sched_debug",    // scheduler internals
	"/proc/irq",            // IRQ configuration
	"/proc/bus",            // hardware bus info
	"/sys/firmware",        // firmware interface
	"/sys/kernel/security", // LSM configuration
	"/sys/kernel/debug",    // kernel debug interface
}

// readonlyPaths are remounted read-only inside the container.
var readonlyPaths = []string{
	"/proc/sys",
	"/proc/sysrq-trigger",
	"/proc/latency_stats",
	"/proc/fs",
	"/proc/tty/ldiscs",
	"/sys/fs/cgroup",
}

// MaskPaths binds /dev/null over sensitive kernel paths and remounts
// others as read-only. Must be called inside the container namespace,
// after MountEssentials and before exec.
func MaskPaths() error {
	log.Printf("[security] masking sensitive paths")

	for _, path := range sensitivePaths {
		if err := maskPath(path); err != nil {
			// Non-fatal — path may not exist in all rootfs images
			log.Printf("[security] warning: could not mask %s: %v", path, err)
		}
	}

	log.Printf("[security] remounting paths read-only")
	for _, path := range readonlyPaths {
		if err := remountReadOnly(path); err != nil {
			log.Printf("[security] warning: could not remount %s read-only: %v", path, err)
		}
	}

	log.Printf("[security] path masking complete")
	return nil
}

// maskPath bind-mounts /dev/null over the target path, making it
// inaccessible without removing it from the filesystem view.
func maskPath(path string) error {
	// Check if path exists — skip silently if not present in this rootfs
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	if err := unix.Mount("/dev/null", path, "", unix.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind mount /dev/null over %s: %w", path, err)
	}

	log.Printf("[security] masked %s", path)
	return nil
}

// remountReadOnly remounts an existing mount point as read-only.
func remountReadOnly(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	if err := unix.Mount("", path, "", unix.MS_REMOUNT|unix.MS_RDONLY, ""); err != nil {
		return fmt.Errorf("remount %s read-only: %w", path, err)
	}

	log.Printf("[security] remounted %s read-only", path)
	return nil
}
