package container

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

const containersBaseDir = "/var/lib/miniDocker/containers"

func generateContainerID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate container id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func RunContainer(args []string) error {
	if len(args) < 2 {
		log.Printf("[run] usage: run <image-path> <command> [args...]")
		return fmt.Errorf("insufficient arguments: %v", args)
	}

	imagePath := args[0]
	log.Printf("[run] image path: %q", imagePath)
	log.Printf("[run] container command: %v", args[1:])

	containerID, err := generateContainerID()
	if err != nil {
		return err
	}

	containerDir := filepath.Join(containersBaseDir, containerID)
	upperDir := filepath.Join(containerDir, "upper")
	workDir := filepath.Join(containerDir, "work")
	mergedDir := filepath.Join(containerDir, "merged")

	for _, dir := range []string{upperDir, workDir, mergedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create container dir %q: %w", dir, err)
		}
	}

	log.Printf("[run] container id: %s", containerID)
	log.Printf("[run] container root: %s", containerDir)
	defer func() {
		if err := unix.Unmount(mergedDir, unix.MNT_DETACH); err != nil && err != unix.EINVAL && err != unix.ENOENT {
			log.Printf("[run] warning: failed to unmount merged dir %q: %v", mergedDir, err)
		}
		if err := os.RemoveAll(containerDir); err != nil {
			log.Printf("[run] warning: failed to remove container dir %q: %v", containerDir, err)
		}
	}()

	cmd := exec.Command("/proc/self/exe", append([]string{"init"}, args[1:]...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"CONTAINER_IMAGE="+imagePath,
		"CONTAINER_ID="+containerID,
		"CONTAINER_UPPER="+upperDir,
		"CONTAINER_WORK="+workDir,
		"CONTAINER_MERGED="+mergedDir,
	)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: unix.CLONE_NEWUTS |
			unix.CLONE_NEWPID |
			unix.CLONE_NEWNS |
			unix.CLONE_NEWNET |
			unix.CLONE_NEWIPC |
			unix.CLONE_NEWUSER,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
		GidMappingsEnableSetgroups: false,
		// Setpgid puts the child in its own process group (PGID = child PID).
		Setpgid: true,
		// Kill container if parent dies unexpectedly.
		Pdeathsig: unix.SIGKILL,
	}

	// Set up signal forwarding BEFORE starting the container to avoid dropping signals
	sigCh := make(chan os.Signal, 8)
	signal.Notify(sigCh,
		unix.SIGINT,
		unix.SIGTERM,
		unix.SIGHUP,
		unix.SIGQUIT,
		unix.SIGUSR1,
		unix.SIGUSR2,
		unix.SIGWINCH,
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for sig := range sigCh {
			if cmd.Process == nil {
				continue
			}
			log.Printf("[run] forwarding signal %v to container", sig)
			// With Setpgid=true the child's PGID == child's PID,
			// so Kill(-childPID) signals the entire container process group.
			if err := unix.Kill(-cmd.Process.Pid, sig.(unix.Signal)); err != nil {
				log.Printf("[run] process group signal failed, falling back to direct: %v", err)
				if err := cmd.Process.Signal(sig); err != nil {
					log.Printf("[run] failed to forward signal %v: %v", sig, err)
				}
			}
		}
	}()

	log.Printf("[run] starting container process")
	if err := cmd.Start(); err != nil {
		log.Printf("[run] failed to start container process: %v", err)
		signal.Stop(sigCh)
		close(sigCh)
		<-done
		return err
	}
	log.Printf("[run] container PID on host: %d", cmd.Process.Pid)

	waitErr := cmd.Wait()

	signal.Stop(sigCh)
	close(sigCh)
	<-done

	if waitErr != nil {
		log.Printf("[run] container exited with error: %v", waitErr)
		return waitErr
	}

	log.Printf("[run] container exited cleanly")
	return nil
}
