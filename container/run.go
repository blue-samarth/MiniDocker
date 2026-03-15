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

	"miniDocker/cgroups"
	"miniDocker/network"
	"miniDocker/state"

	"golang.org/x/sys/unix"
)

const containersBaseDir = "/var/lib/miniDocker/containers"

var ipam = network.NewIPAM(network.Subnet)

func generateContainerID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate container id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func RunContainer(args []string, cgroupCfg ...cgroups.CgroupConfig) error {
	if len(args) < 2 {
		log.Printf("[run] usage: run <image-path> <command> [args...]")
		return fmt.Errorf("insufficient arguments: %v", args)
	}

	imagePath := args[0]
	containerCmd := args[1:]
	log.Printf("[run] image path: %q", imagePath)
	log.Printf("[run] container command: %v", containerCmd)

	containerID, err := generateContainerID()
	if err != nil {
		return err
	}

	// Initialise state manager and lifecycle manager
	sm, err := state.NewStateManager()
	if err != nil {
		log.Printf("[run] warning: failed to init state manager: %v", err)
	}
	var lm *state.LifecycleManager
	if sm != nil {
		lm = state.NewLifecycleManager(sm)
		if err := lm.InitContainer(containerID, &state.ContainerConfig{
			Image:   imagePath,
			Command: containerCmd,
		}); err != nil {
			log.Printf("[run] warning: failed to init container state: %v", err)
		}
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

	// Resolve optional cgroup config
	var cgCfg *cgroups.CgroupConfig
	if len(cgroupCfg) > 0 {
		c := cgroupCfg[0]
		c.ContainerID = containerID
		cgCfg = &c
	}

	// Ensure bridge exists (idempotent)
	if err := network.CreateBridge(); err != nil {
		log.Printf("[run] warning: failed to create bridge: %v", err)
	}

	// Allocate an IP for this container
	containerIP, err := ipam.Allocate(containerID)
	if err != nil {
		if lm != nil {
			lm.MarkError(containerID, err.Error())
		}
		return fmt.Errorf("failed to allocate container IP: %w", err)
	}
	log.Printf("[run] allocated IP %s for container %s", containerIP, containerID)

	if lm != nil {
		lm.RecordNetwork(containerID, containerIP, network.GatewayIP, network.BridgeName)
	}

	hostVethName := fmt.Sprintf("veth-%s", containerID[:8])
	containerVethName := "eth0"

	defer func() {
		if err := unix.Unmount(mergedDir, unix.MNT_DETACH); err != nil &&
			err != unix.EINVAL && err != unix.ENOENT {
			log.Printf("[run] warning: failed to unmount merged dir %q: %v", mergedDir, err)
		}
		if err := os.RemoveAll(containerDir); err != nil {
			log.Printf("[run] warning: failed to remove container dir %q: %v", containerDir, err)
		}
		if cgCfg != nil {
			if err := cgCfg.Cleanup(); err != nil {
				log.Printf("[run] warning: failed to cleanup cgroups: %v", err)
			}
		}
		if err := ipam.Release(containerIP); err != nil {
			log.Printf("[run] warning: failed to release IP %s: %v", containerIP, err)
		}
	}()

	cmd := exec.Command("/proc/self/exe", append([]string{"init"}, containerCmd...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"CONTAINER_IMAGE="+imagePath,
		"CONTAINER_ID="+containerID,
		"CONTAINER_UPPER="+upperDir,
		"CONTAINER_WORK="+workDir,
		"CONTAINER_MERGED="+mergedDir,
		"CONTAINER_IP="+containerIP,
		"CONTAINER_GATEWAY="+network.GatewayIP,
		"CONTAINER_VETH="+containerVethName,
	)

	cmd.SysProcAttr = &unix.SysProcAttr{
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
		Setpgid:                    true,
	}

	// Set up signal forwarding BEFORE starting the container
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
			if err := unix.Kill(-cmd.Process.Pid, sig.(unix.Signal)); err != nil {
				log.Printf("[run] process group signal failed, falling back: %v", err)
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
		if lm != nil {
			lm.MarkError(containerID, err.Error())
		}
		return err
	}
	log.Printf("[run] container PID on host: %d", cmd.Process.Pid)

	if lm != nil {
		if err := lm.MarkRunning(containerID, cmd.Process.Pid); err != nil {
			log.Printf("[run] warning: failed to mark container running: %v", err)
		}
	}

	// Setup veth pair now that we have the container PID
	if err := network.SetupVeth(cmd.Process.Pid, containerIP, hostVethName, containerVethName); err != nil {
		log.Printf("[run] warning: failed to setup veth: %v", err)
	}

	// Setup cgroups after process starts
	if cgCfg != nil && (cgCfg.Memory != "" || cgCfg.CPU != "" || cgCfg.PIDs > 0 || cgCfg.CPUWeight > 0 || cgCfg.SwapMemory != "") {
		if cgroupPath, err := cgCfg.Setup(cmd.Process.Pid); err != nil {
			log.Printf("[run] warning: failed to setup cgroups: %v", err)
		} else if lm != nil {
			lm.RecordCgroupPath(containerID, cgroupPath)
		}
	}

	waitErr := cmd.Wait()

	signal.Stop(sigCh)
	close(sigCh)
	<-done

	// Determine exit code
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	if lm != nil {
		if err := lm.MarkExited(containerID, exitCode); err != nil {
			log.Printf("[run] warning: failed to mark container exited: %v", err)
		}
	}

	if waitErr != nil {
		log.Printf("[run] container exited with error: %v", waitErr)
		return waitErr
	}

	log.Printf("[run] container exited cleanly")
	return nil
}
