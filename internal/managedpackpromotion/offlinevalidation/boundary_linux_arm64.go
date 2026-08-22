//go:build linux && arm64

package offlinevalidation

import "golang.org/x/sys/unix"

const (
	workerAuditArch            = uint32(unix.AUDIT_ARCH_AARCH64)
	workerForbiddenSyscallMask = uint32(0)
)

var workerProcessSyscalls = []uint32{
	unix.SYS_EXECVE,
	unix.SYS_EXECVEAT,
}
