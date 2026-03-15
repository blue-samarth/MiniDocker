package fs

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

type OverlayConfig struct {
	Lower  string // read-only lower layer (must exist)
	Upper  string // writable upper layer
	Work   string // workdir (must be on same fs as Upper)
	Merged string // mount point for the unified view
}

func (cfg OverlayConfig) validate() error {
	for name, path := range map[string]string{
		"Lower":  cfg.Lower,
		"Upper":  cfg.Upper,
		"Work":   cfg.Work,
		"Merged": cfg.Merged,
	} {
		if path == "" {
			return errors.New(name + " path required")
		}
		if strings.ContainsRune(path, ',') {
			return errors.New(name + " path contains invalid comma")
		}
	}
	return nil
}

func MountOverlay(cfg OverlayConfig) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	if _, err := os.Stat(cfg.Lower); err != nil {
		return err
	}

	// Create upper/work/merged if missing (MkdirAll is idempotent)
	for _, dir := range []string{cfg.Upper, cfg.Work, cfg.Merged} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	opts := "lowerdir=" + cfg.Lower +
		",upperdir=" + cfg.Upper +
		",workdir=" + cfg.Work

	if err := unix.Mount("overlay", cfg.Merged, "overlay", 0, opts); err != nil {
		return err
	}

	return nil
}
