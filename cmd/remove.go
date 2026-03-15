package cmd

import (
	"flag"
	"fmt"
	"os"

	"miniDocker/state"
)

func Remove(args []string) error {
	fs := flag.NewFlagSet("rm", flag.ContinueOnError)
	force := fs.Bool("f", false, "Force removal of a running container (stops it first)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	ids := fs.Args()
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: miniDocker rm [-f] <container-id> [...]")
		return fmt.Errorf("container ID required")
	}

	lm, err := getLifecycleManager()
	if err != nil {
		return err
	}

	var lastErr error
	for _, id := range ids {
		if err := removeOne(lm, id, *force); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing %s: %v\n", id, err)
			lastErr = err
		}
	}
	return lastErr
}

func removeOne(lm *state.LifecycleManager, id string, force bool) error {
	cs, err := loadContainer(lm, id)
	if err != nil {
		return err
	}

	short := truncateID(id, 12)

	if cs.Status == state.StatusRunning {
		if !force {
			return fmt.Errorf("container %s is running — stop it first or use -f", short)
		}
		fmt.Printf("Stopping %s before removal...\n", short)
		if err := stopOne(lm, id, 10, false); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: stop failed: %v\n", err)
		}
	}

	if err := lm.Cleanup(id); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	fmt.Printf("Removed %s\n", short)
	return nil
}
