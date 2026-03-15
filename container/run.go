package container

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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

func generateContainerID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func RunContainer(args []string, cgroupCfg ...cgroups.CgroupConfig) error {
	if len(args) < 2 {
		return errors.New("usage: run <image> <cmd> [args...]")
	}

	imagePath := args[0]
	cmdArgs := args[1:]

	containerID, err := generateContainerID()
	if err != nil {
		return err
	}

	sm, _ := state.NewStateManager() // best effort
	lm := state.NewLifecycleManager(sm)
	_ = lm.InitContainer(containerID, &state.ContainerConfig{
		Image:   imagePath,
		Command: cmdArgs,
	})

	containerDir := filepath.Join(containersBaseDir, containerID)
	upper := filepath.Join(containerDir, "upper")
	work := filepath.Join(containerDir, "work")
	merged := filepath.Join(containerDir, "merged")

	for _, d := range []string{upper, work, merged} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}

	log.Printf("container %s  image=%s  cmd=%v", containerID, imagePath, cmdArgs)

	var cg *cgroups.CgroupConfig
	if len(cgroupCfg) > 0 {
		c := cgroupCfg[0]
		c.ContainerID = containerID
		cg = &c
	}

	_ = network.CreateBridge() // idempotent, ignore error

	ipam := network.NewIPAM(network.Subnet) // fresh instance per run to avoid stale state
	ip, err := ipam.Allocate(containerID)
	if err != nil {
		_ = lm.MarkError(containerID, err.Error())
		return err
	}
	_ = lm.RecordNetwork(containerID, ip, network.GatewayIP, network.BridgeName)

	hostVeth := "veth-" + containerID[:8]
	ctVeth := "eth0"

	defer func() {
		_ = unix.Unmount(merged, unix.MNT_DETACH)
		_ = os.RemoveAll(containerDir)
		if cg != nil {
			_ = cg.Cleanup()
		}
		_ = ipam.Release(ip)
	}()

	cmd := exec.Command("/proc/self/exe", append([]string{"init"}, cmdArgs...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"CONTAINER_IMAGE="+imagePath,
		"CONTAINER_ID="+containerID,
		"CONTAINER_UPPER="+upper,
		"CONTAINER_WORK="+work,
		"CONTAINER_MERGED="+merged,
		"CONTAINER_IP="+ip,
		"CONTAINER_GATEWAY="+network.GatewayIP,
		"CONTAINER_VETH="+ctVeth,
	)

	cmd.SysProcAttr = &unix.SysProcAttr{
		Cloneflags: unix.CLONE_NEWUTS | unix.CLONE_NEWPID |
			unix.CLONE_NEWNS | unix.CLONE_NEWNET |
			unix.CLONE_NEWIPC | unix.CLONE_NEWUSER,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
		GidMappingsEnableSetgroups: false,
		Setpgid:                    true,
	}

	sigCh := make(chan os.Signal, 8)
	signal.Notify(sigCh, unix.SIGINT, unix.SIGTERM, unix.SIGHUP,
		unix.SIGQUIT, unix.SIGUSR1, unix.SIGUSR2, unix.SIGWINCH)
	defer signal.Stop(sigCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for sig := range sigCh {
			if cmd.Process == nil {
				continue
			}
			_ = unix.Kill(-cmd.Process.Pid, sig.(unix.Signal))
		}
	}()

	if err := cmd.Start(); err != nil {
		_ = lm.MarkError(containerID, err.Error())
		return err
	}

	log.Printf("container %s started (host pid %d)", containerID, cmd.Process.Pid)
	_ = lm.MarkRunning(containerID, cmd.Process.Pid)

	_ = network.SetupVeth(cmd.Process.Pid, ip, hostVeth, ctVeth)

	if cg != nil && hasResources(*cg) {
		if path, err := cg.Setup(cmd.Process.Pid); err == nil {
			_ = lm.RecordCgroupPath(containerID, path)
		}
	}

	err = cmd.Wait()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = 1
		}
	}

	_ = lm.MarkExited(containerID, exitCode)

	return err
}

func hasResources(c cgroups.CgroupConfig) bool {
	return c.Memory != "" || c.SwapMemory != "" ||
		c.CPU != "" || c.CPUWeight > 0 || c.PIDs > 0
}
