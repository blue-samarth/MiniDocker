package fs

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func PivotRoot(newRoot string) error {
	if newRoot == "" {
		return fmt.Errorf("new root path required")
	}

	putOld := filepath.Join(newRoot, "put_old") // inside newRoot, before pivot

	// Bind-mount newRoot onto itself (required before pivot)
	if err := unix.Mount(newRoot, newRoot, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return err
	}

	if err := os.MkdirAll(putOld, 0700); err != nil {
		_ = unix.Unmount(newRoot, unix.MNT_DETACH)
		return err
	}

	if err := unix.PivotRoot(newRoot, putOld); err != nil {
		_ = unix.Unmount(newRoot, unix.MNT_DETACH)
		_ = os.Remove(putOld)
		return err
	}

	if err := os.Chdir("/"); err != nil {
		return err
	}

	if err := unix.Unmount(putOld, unix.MNT_DETACH); err != nil {
		return err
	}

	return os.Remove(putOld)
}

type Mount struct {
	Source     string
	Target     string
	FSType     string
	Flags      uintptr
	Data       string
	BestEffort bool
}

func MountEssentials() error {
	mounts := []Mount{
		{Source: "proc", Target: "/proc", FSType: "proc",
			Flags: unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NODEV, BestEffort: true},
		{Source: "tmpfs", Target: "/dev", FSType: "tmpfs",
			Flags: unix.MS_NOSUID | unix.MS_STRICTATIME, Data: "mode=755,size=64m"},
		{Source: "devpts", Target: "/dev/pts", FSType: "devpts",
			Flags: unix.MS_NOSUID | unix.MS_NOEXEC, Data: "newinstance,ptmxmode=0666"},
		{Source: "tmpfs", Target: "/dev/shm", FSType: "tmpfs",
			Flags: unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NODEV, Data: "mode=1777,size=64m"},
		{Source: "sysfs", Target: "/sys", FSType: "sysfs",
			Flags: unix.MS_RDONLY | unix.MS_NOEXEC | unix.MS_NOSUID | unix.MS_NODEV, BestEffort: true},
	}

	var mounted []string

	for _, m := range mounts {
		if err := os.MkdirAll(m.Target, 0755); err != nil {
			unmountReverse(mounted)
			return err
		}

		err := unix.Mount(m.Source, m.Target, m.FSType, m.Flags, m.Data)
		if err != nil {
			if m.BestEffort {
				fmt.Fprintf(os.Stderr, "[fs] skipping mount %s → %s: %v\n", m.FSType, m.Target, err)
				continue
			}
			unmountReverse(mounted)
			return err
		}

		mounted = append(mounted, m.Target)
	}

	return nil
}

func unmountReverse(targets []string) {
	for i := len(targets) - 1; i >= 0; i-- {
		_ = unix.Unmount(targets[i], unix.MNT_DETACH)
	}
}
