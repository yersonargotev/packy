//go:build linux && amd64

package offlinevalidation

import "golang.org/x/sys/unix"

const workerAuditArch = uint32(unix.AUDIT_ARCH_X86_64)

var workerProcessSyscalls = []uint32{
	unix.SYS_FORK,
	unix.SYS_VFORK,
	unix.SYS_CLONE3,
	unix.SYS_EXECVE,
	unix.SYS_EXECVEAT,
}
