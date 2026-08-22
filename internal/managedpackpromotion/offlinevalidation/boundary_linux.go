//go:build linux && (amd64 || arm64)

package offlinevalidation

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	workerAddressSpaceBytes = uint64(2 << 30)
	workerDataBytes         = uint64(512 << 20)
	workerResidentBytes     = uint64(512 << 20)
)

func workerCommand(ctx context.Context, invocation processInvocation) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, invocation.Executable, invocation.Args...), nil
}

func installWorkerBoundary() error {
	if err := installRuntimeBounds(); err != nil {
		return err
	}
	limits := append(baseWorkerResourceLimits(),
		resourceLimit{name: "address space", resource: unix.RLIMIT_AS, maximum: workerAddressSpaceBytes},
		resourceLimit{name: "data memory", resource: unix.RLIMIT_DATA, maximum: workerDataBytes},
		resourceLimit{name: "resident memory", resource: unix.RLIMIT_RSS, maximum: workerResidentBytes},
	)
	if err := installResourceLimits(limits, nativeRlimitSystem()); err != nil {
		return err
	}
	if err := installLinuxNetworkBoundary(); err != nil {
		return err
	}
	return verifyLinuxNetworkDenied()
}

func installLinuxNetworkBoundary() error {
	return installLinuxNetworkBoundaryWith(linuxNetworkSystem{
		noNewPrivileges: func() error {
			return unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
		},
		installSeccomp: func(program *unix.SockFprog) (uintptr, error) {
			result, _, errno := unix.RawSyscall(
				unix.SYS_SECCOMP,
				uintptr(unix.SECCOMP_SET_MODE_FILTER),
				uintptr(unix.SECCOMP_FILTER_FLAG_TSYNC),
				uintptr(unsafe.Pointer(program)),
			)
			if errno != 0 {
				return result, errno
			}
			return result, nil
		},
	})
}

type linuxNetworkSystem struct {
	noNewPrivileges func() error
	installSeccomp  func(*unix.SockFprog) (uintptr, error)
}

func installLinuxNetworkBoundaryWith(system linuxNetworkSystem) error {
	if system.noNewPrivileges == nil || system.installSeccomp == nil {
		return errors.New("seccomp system calls are unavailable")
	}
	filters := linuxNetworkFilters()
	program := unix.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := system.noNewPrivileges(); err != nil {
		return fmt.Errorf("set no-new-privileges: %w", err)
	}
	result, err := system.installSeccomp(&program)
	runtime.KeepAlive(filters)
	if err != nil {
		return fmt.Errorf("install seccomp network filter: %w", err)
	}
	if result != 0 {
		return fmt.Errorf("install seccomp network filter: thread %d could not be synchronized", result)
	}
	return nil
}

func linuxNetworkFilters() []unix.SockFilter {
	filters := []unix.SockFilter{
		bpfStatement(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 4),
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, workerAuditArch, 1, 0),
		bpfStatement(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_KILL_PROCESS),
		bpfStatement(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 0),
	}
	if workerForbiddenSyscallMask != 0 {
		// AUDIT_ARCH_X86_64 also covers the x32 ABI. Reject its syscall bit
		// before exact syscall-number comparisons can reach the allow action.
		filters = append(filters,
			bpfJump(unix.BPF_JMP|unix.BPF_JSET|unix.BPF_K, workerForbiddenSyscallMask, 0, 1),
			bpfStatement(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_KILL_PROCESS),
		)
	}
	filters = append(filters,
		// glibc falls back to clone when clone3 is unavailable. Returning
		// ENOSYS keeps clone3 arguments out of the trust boundary while the
		// clone rule below can inspect and require CLONE_THREAD.
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.SYS_CLONE3, 0, 1),
		bpfStatement(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ERRNO|uint32(unix.ENOSYS)),
		bpfStatement(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 0),
		// clone is also used by the Go runtime for OS threads. Permit only
		// CLONE_THREAD and reject fork-like clones that create a process.
		bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, unix.SYS_CLONE, 0, 3),
		bpfStatement(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 16),
		bpfJump(unix.BPF_JMP|unix.BPF_JSET|unix.BPF_K, unix.CLONE_THREAD, 1, 0),
		bpfStatement(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM)),
		bpfStatement(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS, 0),
	)
	deniedSyscalls := []uint32{
		unix.SYS_SOCKET, unix.SYS_SOCKETPAIR, unix.SYS_CONNECT, unix.SYS_ACCEPT,
		unix.SYS_ACCEPT4, unix.SYS_BIND, unix.SYS_LISTEN, unix.SYS_SENDTO,
		unix.SYS_SENDMSG, unix.SYS_SENDMMSG, unix.SYS_RECVFROM, unix.SYS_RECVMSG,
		unix.SYS_RECVMMSG, unix.SYS_SHUTDOWN, unix.SYS_GETSOCKNAME,
		unix.SYS_GETPEERNAME, unix.SYS_SETSOCKOPT, unix.SYS_GETSOCKOPT,
		unix.SYS_IO_URING_SETUP,
	}
	deniedSyscalls = append(deniedSyscalls, workerProcessSyscalls...)
	for _, syscall := range deniedSyscalls {
		filters = append(filters,
			bpfJump(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K, syscall, 0, 1),
			bpfStatement(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM)),
		)
	}
	return append(filters, bpfStatement(unix.BPF_RET|unix.BPF_K, unix.SECCOMP_RET_ALLOW))
}

func bpfStatement(code uint16, value uint32) unix.SockFilter {
	return unix.SockFilter{Code: code, K: value}
}

func bpfJump(code uint16, value uint32, jumpTrue, jumpFalse uint8) unix.SockFilter {
	return unix.SockFilter{Code: code, Jt: jumpTrue, Jf: jumpFalse, K: value}
}

func verifyLinuxNetworkDenied() error {
	descriptor, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
		return nil
	}
	if err == nil {
		_ = unix.Close(descriptor)
		return errors.New("seccomp network boundary probe unexpectedly created a socket")
	}
	return fmt.Errorf("seccomp network boundary probe was not denied: %w", err)
}
