package fs

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

type OverlayConfig struct {
	Lower  string // read-only image layer (must already exist)
	Upper  string // writable layer
	Work   string // kernel scratch space (must be same filesystem as Upper)
	Merged string // unified mount point presented to container
}

func (cfg OverlayConfig) validate() error {
	fields := map[string]string{
		"Lower":  cfg.Lower,
		"Upper":  cfg.Upper,
		"Work":   cfg.Work,
		"Merged": cfg.Merged,
	}
	for name, path := range fields {
		if path == "" {
			return fmt.Errorf("%s path must not be empty", name)
		}
		if strings.ContainsRune(path, ',') {
			return fmt.Errorf("%s path %q contains a comma, which is invalid in overlay mount options", name, path)
		}
	}
	return nil
}

func MountOverlay(cfg OverlayConfig) error {
	if err := cfg.validate(); err != nil {
		return fmt.Errorf("invalid overlay config: %w", err)
	}

	if _, err := os.Stat(cfg.Lower); err != nil {
		return fmt.Errorf("lower dir %q must exist: %w", cfg.Lower, err)
	}

	var created []string
	for _, dir := range []string{cfg.Upper, cfg.Work, cfg.Merged} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				for _, d := range created {
					os.RemoveAll(d)
				}
				return fmt.Errorf("failed to create overlay dir %q: %w", dir, err)
			}
			created = append(created, dir)
		}
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", cfg.Lower, cfg.Upper, cfg.Work)
	flags := uintptr(0)

	if err := unix.Mount("overlay", cfg.Merged, "overlay", flags, opts); err != nil {
		for _, d := range created {
			os.RemoveAll(d)
		}
		return fmt.Errorf("failed to mount overlay at %q: %w", cfg.Merged, err)
	}
	return nil
}
