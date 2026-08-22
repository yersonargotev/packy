//go:build linux && amd64

package offlinevalidation

import "golang.org/x/sys/unix"

const (
	workerAuditArch            = uint32(unix.AUDIT_ARCH_X86_64)
	workerForbiddenSyscallMask = uint32(1 << 30) // Linux __X32_SYSCALL_BIT.
)

var workerProcessSyscalls = []uint32{
	unix.SYS_FORK,
	unix.SYS_VFORK,
	unix.SYS_EXECVE,
	unix.SYS_EXECVEAT,
}
