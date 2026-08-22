//go:build linux && (amd64 || arm64)

package offlinevalidation

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestInstallLinuxNetworkBoundaryFailsClosedOnSetupErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		system linuxNetworkSystem
		want   string
	}{
		{
			name: "no new privileges",
			system: linuxNetworkSystem{
				noNewPrivileges: func() error { return errors.New("prctl failed") },
				installSeccomp: func(*unix.SockFprog) (uintptr, error) {
					return 0, nil
				},
			},
			want: "set no-new-privileges: prctl failed",
		},
		{
			name: "seccomp",
			system: linuxNetworkSystem{
				noNewPrivileges: func() error { return nil },
				installSeccomp: func(*unix.SockFprog) (uintptr, error) {
					return 0, errors.New("seccomp failed")
				},
			},
			want: "install seccomp network filter: seccomp failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := installLinuxNetworkBoundaryWith(test.system)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("installLinuxNetworkBoundaryWith() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLinuxNetworkFilterChecksArchitectureAndDeniesSocket(t *testing.T) {
	filters := linuxNetworkFilters()
	if len(filters) < 7 {
		t.Fatalf("filter length = %d", len(filters))
	}
	if filters[0].K != 4 || filters[1].K != workerAuditArch || filters[2].K != unix.SECCOMP_RET_KILL_PROCESS || filters[3].K != 0 {
		t.Fatalf("filter does not check architecture before syscall number: %#v", filters[:4])
	}
	foundSocketDeny := false
	for index := 4; index+1 < len(filters); index++ {
		if filters[index].K == unix.SYS_SOCKET && filters[index+1].K == unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM) {
			foundSocketDeny = true
			break
		}
	}
	if !foundSocketDeny {
		t.Fatal("filter does not deny socket with EPERM")
	}
}

func TestLinuxNetworkFilterFallsBackFromClone3AndAllowsOnlyThreadClone(t *testing.T) {
	filters := linuxNetworkFilters()
	tests := []struct {
		name    string
		syscall uint32
		flags   uint64
		want    uint32
	}{
		{name: "clone3 fallback", syscall: unix.SYS_CLONE3, want: unix.SECCOMP_RET_ERRNO | uint32(unix.ENOSYS)},
		{name: "thread clone", syscall: unix.SYS_CLONE, flags: unix.CLONE_THREAD, want: unix.SECCOMP_RET_ALLOW},
		{name: "process clone", syscall: unix.SYS_CLONE, want: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)},
		{name: "exec", syscall: unix.SYS_EXECVE, want: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := evaluateLinuxFilter(t, filters, test.syscall, test.flags)
			if got != test.want {
				t.Fatalf("seccomp action = %#x, want %#x", got, test.want)
			}
		})
	}
}

func TestLinuxNetworkFilterRejectsAlternateABISyscallsBeforeTheDenylist(t *testing.T) {
	if workerForbiddenSyscallMask == 0 {
		t.Skip("architecture has no alternate syscall ABI mask")
	}
	filters := linuxNetworkFilters()
	for name, syscall := range map[string]uint32{
		"clone":  unix.SYS_CLONE,
		"socket": unix.SYS_SOCKET,
		"exec":   unix.SYS_EXECVE,
	} {
		t.Run(name, func(t *testing.T) {
			got := evaluateLinuxFilter(t, filters, syscall|workerForbiddenSyscallMask, unix.CLONE_THREAD)
			if got != unix.SECCOMP_RET_KILL_PROCESS {
				t.Fatalf("alternate-ABI seccomp action = %#x, want KILL_PROCESS", got)
			}
		})
	}
}

func evaluateLinuxFilter(t *testing.T, filters []unix.SockFilter, syscall uint32, flags uint64) uint32 {
	t.Helper()
	var accumulator uint32
	for index := 0; index < len(filters); {
		filter := filters[index]
		switch filter.Code {
		case unix.BPF_LD | unix.BPF_W | unix.BPF_ABS:
			switch filter.K {
			case 0:
				accumulator = syscall
			case 4:
				accumulator = workerAuditArch
			case 16:
				accumulator = uint32(flags)
			default:
				t.Fatalf("unsupported seccomp data offset %d", filter.K)
			}
			index++
		case unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K:
			if accumulator == filter.K {
				index += int(filter.Jt) + 1
			} else {
				index += int(filter.Jf) + 1
			}
		case unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K:
			if accumulator&filter.K != 0 {
				index += int(filter.Jt) + 1
			} else {
				index += int(filter.Jf) + 1
			}
		case unix.BPF_RET | unix.BPF_K:
			return filter.K
		default:
			t.Fatalf("unsupported BPF instruction %#x", filter.Code)
		}
	}
	t.Fatal("seccomp filter did not return an action")
	return 0
}
