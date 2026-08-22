//go:build darwin || linux

package offlinevalidation

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestInstallResourceLimitsNeverRaisesAnExistingLimit(t *testing.T) {
	var applied []unix.Rlimit
	current := unix.Rlimit{Cur: 8, Max: 32}
	system := rlimitSystem{
		get: func(_ int, result *unix.Rlimit) error {
			*result = current
			return nil
		},
		set: func(_ int, limit *unix.Rlimit) error {
			applied = append(applied, *limit)
			current = *limit
			return nil
		},
	}

	if err := installResourceLimits([]resourceLimit{{name: "fixture", resource: 1, maximum: 16}}, system); err != nil {
		t.Fatal(err)
	}
	want := []unix.Rlimit{{Cur: 8, Max: 32}, {Cur: 8, Max: 8}}
	if len(applied) != len(want) || applied[0] != want[0] || applied[1] != want[1] {
		t.Fatalf("applied limits = %#v, want %#v", applied, want)
	}
}

func TestInstallResourceLimitsFailsClosedOnSetupErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		system rlimitSystem
		want   string
	}{
		{
			name: "read",
			system: rlimitSystem{
				get: func(int, *unix.Rlimit) error { return errors.New("get failed") },
				set: func(int, *unix.Rlimit) error { return nil },
			},
			want: "read fixture limit: get failed",
		},
		{
			name: "set",
			system: rlimitSystem{
				get: func(_ int, current *unix.Rlimit) error {
					*current = unix.Rlimit{Cur: 32, Max: 32}
					return nil
				},
				set: func(int, *unix.Rlimit) error { return errors.New("set failed") },
			},
			want: "set fixture soft limit: set failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := installResourceLimits([]resourceLimit{{name: "fixture", resource: 1, maximum: 16}}, test.system)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("installResourceLimits() error = %v, want %q", err, test.want)
			}
		})
	}
}
