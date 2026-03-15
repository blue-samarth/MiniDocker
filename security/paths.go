package security

import (
	"log"
	"os"

	"golang.org/x/sys/unix"
)

// sensitivePaths are masked with /dev/null (read/write blocked).
var sensitivePaths = []string{
	"/proc/kcore", "/proc/kmem", "/proc/mem",
	"/proc/sysrq-trigger", "/proc/acpi",
	"/proc/timer_list", "/proc/timer_stats",
	"/proc/sched_debug", "/proc/irq", "/proc/bus",
	"/sys/firmware", "/sys/kernel/security", "/sys/kernel/debug",
}

// readonlyPaths are remounted read-only.
var readonlyPaths = []string{
	"/proc/sys", "/proc/sysrq-trigger",
	"/proc/latency_stats", "/proc/fs",
	"/proc/tty/ldiscs", "/sys/fs/cgroup",
}

func MaskPaths() error {
	log.Printf("[security] masking sensitive kernel paths")

	for _, p := range sensitivePaths {
		_ = maskPath(p) // best-effort
	}

	for _, p := range readonlyPaths {
		_ = remountReadOnly(p) // best-effort
	}

	return nil
}

func maskPath(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	if err := unix.Mount("/dev/null", path, "", unix.MS_BIND, ""); err != nil {
		log.Printf("could not mask %s: %v", path, err)
		return nil // non-fatal
	}

	return nil
}

func remountReadOnly(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	if err := unix.Mount("", path, "", unix.MS_REMOUNT|unix.MS_RDONLY, ""); err != nil {
		log.Printf("could not remount %s read-only: %v", path, err)
		return nil // non-fatal
	}

	return nil
}
