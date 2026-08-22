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
