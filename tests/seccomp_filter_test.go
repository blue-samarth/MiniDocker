package tests

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"miniDocker/security"

	"golang.org/x/sys/unix"
)

const bpfMaxInsns = 4096 // kernel BPF_MAXINSNS

// evalSeccomp runs a classic BPF seccomp program against a synthetic
// seccomp_data (nr at offset 0, arch at offset 4) and returns the action.
// It supports only the opcodes BuildSeccompFilter emits.
func evalSeccomp(t *testing.T, prog []unix.SockFilter, nr, arch uint32) uint32 {
	t.Helper()

	data := make([]byte, 64) // sizeof(struct seccomp_data)
	binary.LittleEndian.PutUint32(data[0:], nr)
	binary.LittleEndian.PutUint32(data[4:], arch)

	var acc uint32
	for pc := 0; pc < len(prog); pc++ {
		ins := prog[pc]
		switch ins.Code {
		case unix.BPF_LD | unix.BPF_W | unix.BPF_ABS:
			acc = binary.LittleEndian.Uint32(data[ins.K:])
		case unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K:
			if acc == ins.K {
				pc += int(ins.Jt)
			} else {
				pc += int(ins.Jf)
			}
		case unix.BPF_RET | unix.BPF_K:
			return ins.K
		default:
			t.Fatalf("unsupported BPF opcode 0x%x at pc %d", ins.Code, pc)
		}
	}

	t.Fatalf("program fell off the end without returning")
	return 0
}

func errnoAction(errno unix.Errno) uint32 {
	return unix.SECCOMP_RET_ERRNO | (uint32(errno) & 0x0000ffff)
}

// TestSeccompFilterActions verifies the action the filter returns per syscall
func TestSeccompFilterActions(t *testing.T) {
	prog := security.BuildSeccompFilter()

	tests := []struct {
		name string
		nr   uint32
		arch uint32
		want uint32
	}{
		// clone3 must look absent (ENOSYS) so glibc falls back to clone()
		{"clone3 denied as ENOSYS", unix.SYS_CLONE3, unix.AUDIT_ARCH_X86_64, errnoAction(unix.ENOSYS)},

		{"clone allowed", unix.SYS_CLONE, unix.AUDIT_ARCH_X86_64, unix.SECCOMP_RET_ALLOW},
		{"read allowed", unix.SYS_READ, unix.AUDIT_ARCH_X86_64, unix.SECCOMP_RET_ALLOW},
		{"execve allowed", unix.SYS_EXECVE, unix.AUDIT_ARCH_X86_64, unix.SECCOMP_RET_ALLOW},

		{"ptrace denied as EPERM", unix.SYS_PTRACE, unix.AUDIT_ARCH_X86_64, errnoAction(unix.EPERM)},
		{"prctl denied as EPERM", unix.SYS_PRCTL, unix.AUDIT_ARCH_X86_64, errnoAction(unix.EPERM)},
		{"ioctl denied as EPERM", unix.SYS_IOCTL, unix.AUDIT_ARCH_X86_64, errnoAction(unix.EPERM)},
		{"seccomp denied as EPERM", unix.SYS_SECCOMP, unix.AUDIT_ARCH_X86_64, errnoAction(unix.EPERM)},
		{"mount denied as EPERM", unix.SYS_MOUNT, unix.AUDIT_ARCH_X86_64, errnoAction(unix.EPERM)},
		{"init_module denied as EPERM", unix.SYS_INIT_MODULE, unix.AUDIT_ARCH_X86_64, errnoAction(unix.EPERM)},

		{"foreign arch killed", unix.SYS_READ, unix.AUDIT_ARCH_I386, unix.SECCOMP_RET_KILL_PROCESS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evalSeccomp(t, prog, tt.nr, tt.arch)
			if got != tt.want {
				t.Errorf("action: got 0x%08x, want 0x%08x", got, tt.want)
			}
		})
	}
}

// TestSeccompFilterLength verifies the program fits the kernel's instruction limit
func TestSeccompFilterLength(t *testing.T) {
	prog := security.BuildSeccompFilter()
	if len(prog) == 0 || len(prog) > bpfMaxInsns {
		t.Errorf("filter length %d outside 1..%d", len(prog), bpfMaxInsns)
	}
}

// TestSeccompFilterKernel loads the real filter in a child process and checks
// the errno the kernel actually returns. No root needed: NO_NEW_PRIVS suffices.
func TestSeccompFilterKernel(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("seccomp filter is x86-64 only")
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestSeccompFilterKernelHelper$")
	cmd.Env = append(os.Environ(), "MINIDOCKER_SECCOMP_HELPER=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("helper failed: %v\nstdout: %s", err, out)
	}

	want := fmt.Sprintf("clone3=%d ptrace=%d prctl=%d getpid=0",
		int(unix.ENOSYS), int(unix.EPERM), int(unix.EPERM))
	if !strings.Contains(string(out), want) {
		t.Errorf("got %q, want it to contain %q", strings.TrimSpace(string(out)), want)
	}
}

// TestSeccompFilterKernelHelper runs only as the child of TestSeccompFilterKernel
func TestSeccompFilterKernelHelper(t *testing.T) {
	if os.Getenv("MINIDOCKER_SECCOMP_HELPER") != "1" {
		return
	}

	// Seccomp filters installed via prctl apply to the calling thread only
	runtime.LockOSThread()

	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		fmt.Printf("no_new_privs: %v\n", err)
		os.Exit(1)
	}
	if err := security.ApplySeccompFilter(); err != nil {
		fmt.Printf("apply: %v\n", err)
		os.Exit(1)
	}

	// Arguments are deliberately invalid so nothing happens if a call slips through
	_, _, clone3 := unix.RawSyscall(unix.SYS_CLONE3, 0, 0, 0)
	_, _, ptrace := unix.RawSyscall(unix.SYS_PTRACE, 0xffff, 0, 0)
	_, _, prctl := unix.RawSyscall(unix.SYS_PRCTL, unix.PR_GET_DUMPABLE, 0, 0)
	_, _, getpid := unix.RawSyscall(unix.SYS_GETPID, 0, 0, 0)

	fmt.Printf("clone3=%d ptrace=%d prctl=%d getpid=%d\n",
		int(clone3), int(ptrace), int(prctl), int(getpid))
	os.Exit(0)
}
