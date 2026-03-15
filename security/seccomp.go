package security

import (
	"fmt"
	"log"
	"unsafe"

	"golang.org/x/sys/unix"
)

// allowedSyscalls is the whitelist of syscalls containers are permitted to use.
// Everything not listed here returns EPERM.
var allowedSyscalls = []uint32{
	// Process
	unix.SYS_READ,
	unix.SYS_WRITE,
	unix.SYS_OPEN,
	unix.SYS_OPENAT,
	unix.SYS_CLOSE,
	unix.SYS_STAT,
	unix.SYS_FSTAT,
	unix.SYS_LSTAT,
	unix.SYS_POLL,
	unix.SYS_LSEEK,
	unix.SYS_MMAP,
	unix.SYS_MPROTECT,
	unix.SYS_MUNMAP,
	unix.SYS_BRK,
	unix.SYS_RT_SIGACTION,
	unix.SYS_RT_SIGPROCMASK,
	unix.SYS_RT_SIGRETURN,
	unix.SYS_IOCTL,
	unix.SYS_PREAD64,
	unix.SYS_PWRITE64,
	unix.SYS_READV,
	unix.SYS_WRITEV,
	unix.SYS_ACCESS,
	unix.SYS_PIPE,
	unix.SYS_SELECT,
	unix.SYS_SCHED_YIELD,
	unix.SYS_MREMAP,
	unix.SYS_MSYNC,
	unix.SYS_MINCORE,
	unix.SYS_MADVISE,
	unix.SYS_DUP,
	unix.SYS_DUP2,
	unix.SYS_PAUSE,
	unix.SYS_NANOSLEEP,
	unix.SYS_GETITIMER,
	unix.SYS_ALARM,
	unix.SYS_SETITIMER,
	unix.SYS_GETPID,
	unix.SYS_SENDFILE,

	// File system
	unix.SYS_SOCKET,
	unix.SYS_CONNECT,
	unix.SYS_ACCEPT,
	unix.SYS_SENDTO,
	unix.SYS_RECVFROM,
	unix.SYS_SENDMSG,
	unix.SYS_RECVMSG,
	unix.SYS_SHUTDOWN,
	unix.SYS_BIND,
	unix.SYS_LISTEN,
	unix.SYS_GETSOCKNAME,
	unix.SYS_GETPEERNAME,
	unix.SYS_SOCKETPAIR,
	unix.SYS_SETSOCKOPT,
	unix.SYS_GETSOCKOPT,
	unix.SYS_CLONE,
	unix.SYS_FORK,
	unix.SYS_VFORK,
	unix.SYS_EXECVE,
	unix.SYS_EXIT,
	unix.SYS_WAIT4,
	unix.SYS_KILL,
	unix.SYS_UNAME,
	unix.SYS_FCNTL,
	unix.SYS_FLOCK,
	unix.SYS_FSYNC,
	unix.SYS_FDATASYNC,
	unix.SYS_TRUNCATE,
	unix.SYS_FTRUNCATE,
	unix.SYS_GETDENTS,
	unix.SYS_GETCWD,
	unix.SYS_CHDIR,
	unix.SYS_FCHDIR,
	unix.SYS_RENAME,
	unix.SYS_MKDIR,
	unix.SYS_RMDIR,
	unix.SYS_CREAT,
	unix.SYS_LINK,
	unix.SYS_UNLINK,
	unix.SYS_SYMLINK,
	unix.SYS_READLINK,
	unix.SYS_CHMOD,
	unix.SYS_FCHMOD,
	unix.SYS_CHOWN,
	unix.SYS_FCHOWN,
	unix.SYS_LCHOWN,
	unix.SYS_UMASK,
	unix.SYS_GETTIMEOFDAY,
	unix.SYS_GETRLIMIT,
	unix.SYS_GETRUSAGE,
	unix.SYS_SYSINFO,
	unix.SYS_TIMES,
	unix.SYS_GETUID,
	unix.SYS_GETGID,
	unix.SYS_GETEUID,
	unix.SYS_GETEGID,
	unix.SYS_SETPGID,
	unix.SYS_GETPPID,
	unix.SYS_GETPGRP,
	unix.SYS_SETSID,
	unix.SYS_SETREUID,
	unix.SYS_SETREGID,
	unix.SYS_GETGROUPS,
	unix.SYS_SETGROUPS,
	unix.SYS_SETRESUID,
	unix.SYS_GETRESUID,
	unix.SYS_SETRESGID,
	unix.SYS_GETRESGID,
	unix.SYS_GETPGID,
	unix.SYS_SETFSUID,
	unix.SYS_SETFSGID,
	unix.SYS_GETSID,
	unix.SYS_CAPGET,
	unix.SYS_CAPSET,
	unix.SYS_RT_SIGPENDING,
	unix.SYS_RT_SIGTIMEDWAIT,
	unix.SYS_RT_SIGQUEUEINFO,
	unix.SYS_RT_SIGSUSPEND,
	unix.SYS_SIGALTSTACK,
	unix.SYS_UTIME,
	unix.SYS_MKNOD,
	unix.SYS_PERSONALITY,
	unix.SYS_USTAT,
	unix.SYS_STATFS,
	unix.SYS_FSTATFS,
	unix.SYS_GETPRIORITY,
	unix.SYS_SETPRIORITY,
	unix.SYS_SCHED_SETPARAM,
	unix.SYS_SCHED_GETPARAM,
	unix.SYS_SCHED_SETSCHEDULER,
	unix.SYS_SCHED_GETSCHEDULER,
	unix.SYS_SCHED_GET_PRIORITY_MAX,
	unix.SYS_SCHED_GET_PRIORITY_MIN,
	unix.SYS_SCHED_RR_GET_INTERVAL,
	unix.SYS_MLOCK,
	unix.SYS_MUNLOCK,
	unix.SYS_MLOCKALL,
	unix.SYS_MUNLOCKALL,
	unix.SYS_VHANGUP,
	unix.SYS_PRCTL,
	unix.SYS_ARCH_PRCTL,
	unix.SYS_SETRLIMIT,
	unix.SYS_SYNC,
	unix.SYS_ACCT,
	unix.SYS_SETTIMEOFDAY,
	unix.SYS_CHROOT,
	unix.SYS_SYNC_FILE_RANGE,
	unix.SYS_GETTID,
	unix.SYS_FUTEX,
	unix.SYS_SCHED_SETAFFINITY,
	unix.SYS_SCHED_GETAFFINITY,
	unix.SYS_SET_THREAD_AREA,
	unix.SYS_GET_THREAD_AREA,
	unix.SYS_IO_SETUP,
	unix.SYS_IO_DESTROY,
	unix.SYS_IO_GETEVENTS,
	unix.SYS_IO_SUBMIT,
	unix.SYS_IO_CANCEL,
	unix.SYS_LOOKUP_DCOOKIE,
	unix.SYS_EPOLL_CREATE,
	unix.SYS_EPOLL_CTL,
	unix.SYS_EPOLL_WAIT,
	unix.SYS_REMAP_FILE_PAGES,
	unix.SYS_GETDENTS64,
	unix.SYS_SET_TID_ADDRESS,
	unix.SYS_SEMTIMEDOP,
	unix.SYS_FADVISE64,
	unix.SYS_TIMER_CREATE,
	unix.SYS_TIMER_SETTIME,
	unix.SYS_TIMER_GETTIME,
	unix.SYS_TIMER_GETOVERRUN,
	unix.SYS_TIMER_DELETE,
	unix.SYS_CLOCK_SETTIME,
	unix.SYS_CLOCK_GETTIME,
	unix.SYS_CLOCK_GETRES,
	unix.SYS_CLOCK_NANOSLEEP,
	unix.SYS_EXIT_GROUP,
	unix.SYS_EPOLL_PWAIT,
	unix.SYS_TGKILL,
	unix.SYS_UTIMES,
	unix.SYS_WAITID,
	unix.SYS_INOTIFY_INIT,
	unix.SYS_INOTIFY_ADD_WATCH,
	unix.SYS_INOTIFY_RM_WATCH,
	unix.SYS_OPENAT,
	unix.SYS_MKDIRAT,
	unix.SYS_MKNODAT,
	unix.SYS_FCHOWNAT,
	unix.SYS_FUTIMESAT,
	unix.SYS_NEWFSTATAT,
	unix.SYS_UNLINKAT,
	unix.SYS_RENAMEAT,
	unix.SYS_LINKAT,
	unix.SYS_SYMLINKAT,
	unix.SYS_READLINKAT,
	unix.SYS_FCHMODAT,
	unix.SYS_FACCESSAT,
	unix.SYS_PSELECT6,
	unix.SYS_PPOLL,
	unix.SYS_SPLICE,
	unix.SYS_TEE,
	unix.SYS_SYNC_FILE_RANGE,
	unix.SYS_VMSPLICE,
	unix.SYS_UTIMENSAT,
	unix.SYS_EPOLL_PWAIT,
	unix.SYS_SIGNALFD,
	unix.SYS_TIMERFD_CREATE,
	unix.SYS_EVENTFD,
	unix.SYS_FALLOCATE,
	unix.SYS_TIMERFD_SETTIME,
	unix.SYS_TIMERFD_GETTIME,
	unix.SYS_ACCEPT4,
	unix.SYS_SIGNALFD4,
	unix.SYS_EVENTFD2,
	unix.SYS_EPOLL_CREATE1,
	unix.SYS_DUP3,
	unix.SYS_PIPE2,
	unix.SYS_INOTIFY_INIT1,
	unix.SYS_PREADV,
	unix.SYS_PWRITEV,
	unix.SYS_RT_TGSIGQUEUEINFO,
	unix.SYS_RECVMMSG,
	unix.SYS_SENDMMSG,
	unix.SYS_GETCPU,
	unix.SYS_GETRANDOM,
	unix.SYS_MEMFD_CREATE,
	unix.SYS_EXECVEAT,
	unix.SYS_COPY_FILE_RANGE,
	unix.SYS_PREADV2,
	unix.SYS_PWRITEV2,
	unix.SYS_STATX,
	unix.SYS_PIDFD_OPEN,
	unix.SYS_CLONE3,
	unix.SYS_CLOSE_RANGE,
	unix.SYS_OPENAT2,
	unix.SYS_FACCESSAT2,
}

// BPF instruction opcodes
const (
	bpfLD  = 0x00
	bpfJMP = 0x05
	bpfRET = 0x06
	bpfW   = 0x00 // word (32-bit)
	bpfABS = 0x20
	bpfJEQ = 0x10
	bpfK   = 0x00
	bpfJGE = 0x30

	syscallNr = 0 // offset of syscall number in seccomp_data
)

// seccompData mirrors the kernel's struct seccomp_data layout.
// arch is at offset 4, nr (syscall number) is at offset 0.
const (
	seccompDataOffNr   = 0
	seccompDataOffArch = 4
)

// ApplySeccompFilter installs a whitelist BPF seccomp filter.
// Must be called after PR_SET_NO_NEW_PRIVS (set in DropCapabilities).
func ApplySeccompFilter() error {
	log.Printf("[security] applying seccomp whitelist filter (%d allowed syscalls)", len(allowedSyscalls))

	// Each allowed syscall needs 2 instructions: load nr + jeq.
	// Plus: 2 for arch check, 1 for default deny, 1 for allow ret = 4 overhead.
	filter := make([]unix.SockFilter, 0, len(allowedSyscalls)*2+4)

	// 1. Validate architecture (x86_64 = AUDIT_ARCH_X86_64 = 0xc000003e)
	filter = append(filter,
		// Load arch field
		bpfStmt(bpfLD|bpfW|bpfABS, seccompDataOffArch),
		// If arch != AUDIT_ARCH_X86_64, kill process
		bpfJump(bpfJMP|bpfJEQ|bpfK, unix.AUDIT_ARCH_X86_64,
			1, // true: skip kill
			0, // false: fall through to kill
		),
		bpfStmt(bpfRET|bpfK, unix.SECCOMP_RET_KILL_PROCESS),
	)

	// 2. Load syscall number once
	filter = append(filter, bpfStmt(bpfLD|bpfW|bpfABS, seccompDataOffNr))

	// 3. For each allowed syscall: if nr == syscall, return ALLOW
	for _, nr := range allowedSyscalls {
		filter = append(filter,
			bpfJump(bpfJMP|bpfJEQ|bpfK, nr,
				0, // true: fall through to next instruction (allow)
				1, // false: skip allow instruction
			),
			bpfStmt(bpfRET|bpfK, unix.SECCOMP_RET_ALLOW),
		)
	}

	// 4. Default: return EPERM for anything not whitelisted
	filter = append(filter,
		bpfStmt(bpfRET|bpfK, unix.SECCOMP_RET_ERRNO|(unix.EPERM&unix.SECCOMP_RET_DATA)),
	)

	prog := unix.SockFprog{
		Len:    uint16(len(filter)),
		Filter: &filter[0],
	}

	if err := unix.Prctl(unix.PR_SET_SECCOMP,
		unix.SECCOMP_MODE_FILTER,
		uintptr(unsafe.Pointer(&prog)),
		0, 0,
	); err != nil {
		return fmt.Errorf("failed to apply seccomp filter: %w", err)
	}

	log.Printf("[security] seccomp filter applied successfully")
	return nil
}

func bpfStmt(code uint16, k uint32) unix.SockFilter {
	return unix.SockFilter{Code: code, K: k}
}

func bpfJump(code uint16, k uint32, jt, jf uint8) unix.SockFilter {
	return unix.SockFilter{Code: code, Jt: jt, Jf: jf, K: k}
}
