package security

import (
	"fmt"
	"log"

	"golang.org/x/sys/unix"
)

// dangerousCapabilities are dropped from the container's capability sets.
// These allow privilege escalation, kernel module loading, raw network access,
// and other operations that containers should never need.
var dangerousCapabilities = []uintptr{
	unix.CAP_SYS_ADMIN,          // mount, sethostname, many kernel ops
	unix.CAP_SYS_PTRACE,         // ptrace other processes
	unix.CAP_SYS_MODULE,         // load/unload kernel modules
	unix.CAP_SYS_RAWIO,          // raw I/O port access
	unix.CAP_SYS_BOOT,           // reboot/kexec
	unix.CAP_SYS_NICE,           // raise process priorities
	unix.CAP_SYS_RESOURCE,       // override resource limits
	unix.CAP_SYS_TIME,           // set system clock
	unix.CAP_SYS_TTY_CONFIG,     // configure TTY devices
	unix.CAP_NET_ADMIN,          // network configuration
	unix.CAP_NET_RAW,            // raw sockets
	unix.CAP_NET_BROADCAST,      // broadcast/multicast
	unix.CAP_NET_BIND_SERVICE,   // bind to ports < 1024 (keep if needed)
	unix.CAP_IPC_LOCK,           // lock memory
	unix.CAP_IPC_OWNER,          // bypass IPC ownership checks
	unix.CAP_LINUX_IMMUTABLE,    // set immutable file flags
	unix.CAP_MAC_ADMIN,          // MAC configuration
	unix.CAP_MAC_OVERRIDE,       // MAC override
	unix.CAP_AUDIT_CONTROL,      // audit subsystem
	unix.CAP_AUDIT_READ,         // read audit log
	unix.CAP_AUDIT_WRITE,        // write audit log
	unix.CAP_SYSLOG,             // kernel syslog
	unix.CAP_WAKE_ALARM,         // set wake alarms
	unix.CAP_BLOCK_SUSPEND,      // block system suspend
	unix.CAP_PERFMON,            // performance monitoring
	unix.CAP_BPF,                // BPF operations
	unix.CAP_CHECKPOINT_RESTORE, // checkpoint/restore
}

// DropCapabilities drops dangerous capabilities from all sets
// (effective, permitted, inheritable) and sets a restrictive bounding set.
// Must be called inside the container namespace, before exec.
func DropCapabilities() error {
	log.Printf("[security] dropping dangerous capabilities")

	for _, cap := range dangerousCapabilities {
		// Drop from bounding set first — prevents re-acquiring via exec
		if err := unix.Prctl(unix.PR_CAPBSET_DROP, cap, 0, 0, 0); err != nil {
			// Some capabilities may not exist on older kernels — log and continue
			log.Printf("[security] warning: could not drop cap %d from bounding set: %v", cap, err)
		}
	}

	// Build a header + data structure for the remaining allowed caps
	hdr := unix.CapUserHeader{
		Version: unix.LINUX_CAPABILITY_VERSION_3,
		Pid:     0, // 0 = current process
	}

	var data [2]unix.CapUserData
	if err := unix.Capget(&hdr, &data[0]); err != nil {
		return fmt.Errorf("capget failed: %w", err)
	}

	// Clear dangerous caps from effective, permitted, and inheritable sets
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
		return fmt.Errorf("capset failed: %w", err)
	}

	// Prevent privilege escalation via setuid binaries
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("failed to set no_new_privs: %w", err)
	}

	log.Printf("[security] capabilities dropped successfully")
	return nil
}
