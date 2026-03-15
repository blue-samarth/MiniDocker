package cmd

import (
	"flag"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"miniDocker/state"
)

func Stop(args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	timeout := fs.Int("t", 10, "Seconds to wait before SIGKILL")
	force := fs.Bool("f", false, "Skip graceful shutdown, send SIGKILL immediately")

	if err := fs.Parse(args); err != nil {
		return err
	}

	ids := fs.Args()
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: miniDocker stop [-t <sec>] [-f] <container-id> [...]")
		return fmt.Errorf("container ID required")
	}

	lm, err := getLifecycleManager()
	if err != nil {
		return err
	}

	var lastErr error
	for _, id := range ids {
		if err := stopOne(lm, id, *timeout, *force); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping %s: %v\n", id, err)
			lastErr = err
		}
	}
	return lastErr
}

func stopOne(lm *state.LifecycleManager, id string, timeoutSecs int, force bool) error {
	cs, err := loadContainer(lm, id)
	if err != nil {
		return err
	}

	if cs.Status != state.StatusRunning {
		return fmt.Errorf("container %s is not running (status: %s)", truncateID(id, 12), cs.Status)
	}

	// cs.Pid is the host-side PID of the miniDocker run process, not the container's PID 1.
	// Signaling this PID is correct: it terminates the parent, which triggers cleanup.
	parentRunPID := cs.Pid
	if parentRunPID <= 0 {
		return fmt.Errorf("container %s has no valid parent process PID", truncateID(id, 12))
	}

	short := truncateID(id, 12)

	if force {
		fmt.Printf("Force-stopping %s... ", short)
		if err := unix.Kill(parentRunPID, unix.SIGKILL); err != nil && err != unix.ESRCH {
			fmt.Println("FAILED")
			return fmt.Errorf("SIGKILL failed: %w", err)
		}
		fmt.Println("OK")
		return nil
	}

	fmt.Printf("Stopping %s... ", short)

	if err := unix.Kill(parentRunPID, unix.SIGTERM); err != nil {
		if err == unix.ESRCH {
			fmt.Println("(already exited)")
			return nil
		}
		fmt.Println("FAILED")
		return fmt.Errorf("SIGTERM failed: %w", err)
	}

	deadline := time.Now().Add(time.Duration(timeoutSecs) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if err := unix.Kill(parentRunPID, 0); err == unix.ESRCH {
			fmt.Println("OK")
			return nil
		}
	}

	// Grace period elapsed — escalate to SIGKILL.
	fmt.Print("(timeout, sending SIGKILL)... ")
	if err := unix.Kill(parentRunPID, unix.SIGKILL); err != nil && err != unix.ESRCH {
		fmt.Println("FAILED")
		return fmt.Errorf("SIGKILL failed: %w", err)
	}

	killDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(killDeadline) {
		time.Sleep(100 * time.Millisecond)
		if err := unix.Kill(parentRunPID, 0); err == unix.ESRCH {
			break
		}
	}

	fmt.Println("OK")
	return nil
}
