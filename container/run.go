package container

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func RunContainer(args []string) error {
	if len(args) < 2 {
		log.Printf("[run] usage: run <image-path> <command> [args...]")
		return fmt.Errorf("insufficient arguments: %v", args)
	}
	imagePath := args[0]
	log.Printf("[run] image path: %q", imagePath)
	log.Printf("[run] container command: %v", args[1:])

	cmd := exec.Command("/proc/self/exe", append([]string{"init"}, args[1:]...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "CONTAINER_IMAGE="+imagePath)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWUSER,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
	}

	// Set up signal forwarding BEFORE starting the container to avoid dropping signals
	sigCh := make(chan os.Signal, 8)
	signal.Notify(sigCh,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGQUIT,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
		syscall.SIGWINCH,
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for sig := range sigCh {
			if cmd.Process != nil {
				log.Printf("[run] forwarding signal %v to container", sig)
				if err := cmd.Process.Signal(sig); err != nil {
					log.Printf("[run] failed to forward signal %v: %v", sig, err)
				}
			}
		}
	}()

	log.Printf("[run] starting container process")
	if err := cmd.Start(); err != nil {
		log.Printf("[run] failed to start container process: %v", err)
		// Clean up signal handling on start failure
		signal.Stop(sigCh)
		close(sigCh)
		<-done
		return err
	}
	log.Printf("[run] container PID on host: %d", cmd.Process.Pid)

	waitErr := cmd.Wait()

	// Stop notify before closing: guarantees the runtime won't send to sigCh
	// after close, avoiding a panic on send-to-closed-channel.
	signal.Stop(sigCh)
	close(sigCh)
	<-done // wait for goroutine to drain and exit

	if waitErr != nil {
		log.Printf("[run] container exited with error: %v", waitErr)
		return waitErr
	}

	log.Printf("[run] container exited cleanly")
	return nil
}
