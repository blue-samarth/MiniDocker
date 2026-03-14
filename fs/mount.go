//go:build linux

package fs

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func PivotRoot(newRoot string) error {
	if newRoot == "" {
		return fmt.Errorf("newRoot must not be empty")
	}

	putOld := filepath.Join(newRoot, "put_old")

	if err := unix.Mount(newRoot, newRoot, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to bind mount new root: %w", err)
	}

	if err := os.MkdirAll(putOld, 0700); err != nil {
		unix.Unmount(newRoot, unix.MNT_DETACH)
		return fmt.Errorf("failed to create put_old dir: %w", err)
	}

	if err := unix.PivotRoot(newRoot, putOld); err != nil {
		unix.Unmount(newRoot, unix.MNT_DETACH)
		os.Remove(putOld)
		return fmt.Errorf("failed to pivot_root: %w", err)
	}

	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("failed to chdir to new root: %w", err)
	}

	// Use absolute paths — don't rely on CWD implicitly.
	if err := unix.Unmount("/put_old", unix.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount old root: %w", err)
	}

	if err := os.Remove("/put_old"); err != nil {
		return fmt.Errorf("failed to remove put_old: %w", err)
	}

	return nil
}

type mountSpec struct {
	source     string
	target     string
	fstype     string
	flags      uintptr
	data       string
	bestEffort bool // if true, log and skip on failure rather than aborting
}

func MountEssentials() error {
	mounts := []mountSpec{
		{
			source: "proc",
			target: "/proc",
			fstype: "proc",
			flags:  unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NODEV,
		},
		{
			// Mount /dev as tmpfs first — subsequent entries below depend on this.
			source: "tmpfs",
			target: "/dev",
			fstype: "tmpfs",
			flags:  unix.MS_NOSUID | unix.MS_STRICTATIME,
			data:   "mode=755,size=65536k",
		},
		{
			// /dev/pts must come after /dev is mounted.
			source: "devpts",
			target: "/dev/pts",
			fstype: "devpts",
			flags:  unix.MS_NOSUID | unix.MS_NOEXEC,
			data:   "newinstance,ptmxmode=0666",
		},
		{
			source: "tmpfs",
			target: "/dev/shm",
			fstype: "tmpfs",
			flags:  unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NODEV,
			data:   "mode=1777,size=65536k",
		},
		{
			// sysfs requires CAP_SYS_ADMIN in the init namespace — likely
			// unavailable inside a user namespace. Best-effort so it doesn't
			// abort container startup on unprivileged hosts.
			source:     "sysfs",
			target:     "/sys",
			fstype:     "sysfs",
			flags:      unix.MS_RDONLY | unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NODEV,
			bestEffort: true,
		},
	}

	var mounted []string

	for _, m := range mounts {
		// For sub-mounts under /dev, the parent tmpfs must be mounted first
		// so MkdirAll operates inside the container's /dev, not the host's.
		if err := os.MkdirAll(m.target, 0755); err != nil {
			unmountAll(mounted)
			return fmt.Errorf("failed to create mount target %q: %w", m.target, err)
		}

		if err := unix.Mount(m.source, m.target, m.fstype, m.flags, m.data); err != nil {
			if m.bestEffort {
				fmt.Fprintf(os.Stderr, "[fs] warning: skipping best-effort mount %q at %q: %v\n", m.fstype, m.target, err)
				continue
			}
			unmountAll(mounted)
			return fmt.Errorf("failed to mount %q at %q: %w", m.fstype, m.target, err)
		}

		mounted = append(mounted, m.target)
	}

	return nil
}

func unmountAll(targets []string) {
	for i := len(targets) - 1; i >= 0; i-- {
		unix.Unmount(targets[i], unix.MNT_DETACH)
	}
}