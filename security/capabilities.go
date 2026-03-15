package security

import (
	"log"

	"golang.org/x/sys/unix"
)

// dangerousCapabilities are dropped from bounding, effective, permitted, and inheritable sets.
var dangerousCapabilities = []uintptr{
	unix.CAP_SYS_ADMIN,
	unix.CAP_SYS_PTRACE,
	unix.CAP_SYS_MODULE,
	unix.CAP_SYS_RAWIO,
	unix.CAP_SYS_BOOT,
	unix.CAP_SYS_NICE,
	unix.CAP_SYS_RESOURCE,
	unix.CAP_SYS_TIME,
	unix.CAP_SYS_TTY_CONFIG,
	unix.CAP_NET_ADMIN,
	unix.CAP_NET_RAW,
	unix.CAP_NET_BROADCAST,
	unix.CAP_NET_BIND_SERVICE, // often kept, but dropped here for max restriction
	unix.CAP_IPC_LOCK,
	unix.CAP_IPC_OWNER,
	unix.CAP_LINUX_IMMUTABLE,
	unix.CAP_MAC_ADMIN,
	unix.CAP_MAC_OVERRIDE,
	unix.CAP_AUDIT_CONTROL,
	unix.CAP_AUDIT_READ,
	unix.CAP_AUDIT_WRITE,
	unix.CAP_SYSLOG,
	unix.CAP_WAKE_ALARM,
	unix.CAP_BLOCK_SUSPEND,
	unix.CAP_PERFMON,
	unix.CAP_BPF,
	unix.CAP_CHECKPOINT_RESTORE,
}

func DropCapabilities() error {
	log.Printf("[security] dropping dangerous capabilities")

	// Drop from bounding set (prevents re-acquire via exec)
	for _, cap := range dangerousCapabilities {
		_ = unix.Prctl(unix.PR_CAPBSET_DROP, cap, 0, 0, 0) // best-effort
	}

	// Get current capability sets
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3, Pid: 0}
	var data [2]unix.CapUserData
	if err := unix.Capget(&hdr, &data[0]); err != nil {
		return err
	}

	// Clear dangerous caps from effective, permitted, inheritable
	for _, cap := range dangerousCapabilities {
		word := cap / 32
		bit := uint32(1) << (cap % 32)
		if word < 2 {
			data[word].Effective &^= bit
			data[word].Permitted &^= bit
			data[word].Inheritable &^= bit
		}
	}

	if err := unix.Capset(&hdr, &data[0]); err != nil {
		return err
	}

	// Lock down privilege escalation via setuid/setgid
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}

	return nil
}
