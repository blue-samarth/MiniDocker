package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"miniDocker/fs"

	"golang.org/x/sys/unix"
)

// TestPivotRootRemovesPutOld runs fs.PivotRoot in a child process with its own
// mount namespace and checks the old root is detached from /put_old afterwards.
func TestPivotRootRemovesPutOld(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root privileges - run with: sudo go test")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "marker"), []byte("new-root"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestPivotRootHelper$")
	cmd.Env = append(os.Environ(), "MINIDOCKER_PIVOT_HELPER_ROOT="+root)
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNS}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper failed: %v\noutput: %s", err, out)
	}

	if !strings.Contains(string(out), "pivot ok") {
		t.Errorf("unexpected helper output: %s", out)
	}
}

// TestPivotRootHelper runs only as the child of TestPivotRootRemovesPutOld
func TestPivotRootHelper(t *testing.T) {
	root := os.Getenv("MINIDOCKER_PIVOT_HELPER_ROOT")
	if root == "" {
		return
	}

	fail := func(format string, args ...any) {
		fmt.Printf(format+"\n", args...)
		os.Exit(1)
	}

	// pivot_root rejects shared mounts; keep this namespace's changes private
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		fail("make / private: %v", err)
	}

	if err := fs.PivotRoot(root); err != nil {
		fail("PivotRoot: %v", err)
	}

	if _, err := os.Stat("/marker"); err != nil {
		fail("marker missing after pivot: %v", err)
	}
	if _, err := os.Stat("/put_old"); !os.IsNotExist(err) {
		fail("/put_old still present after pivot: %v", err)
	}

	fmt.Println("pivot ok")
	os.Exit(0)
}
