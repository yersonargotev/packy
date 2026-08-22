//go:build darwin || linux

package offlinevalidation

import (
	"fmt"

	"golang.org/x/sys/unix"
)

const (
	workerCPUSeconds = uint64(60)
	workerFileBytes  = uint64(20 << 20)
	workerOpenFiles  = uint64(64)
	workerStackBytes = uint64(64 << 20)
)

type resourceLimit struct {
	name     string
	resource int
	maximum  uint64
}

type rlimitSystem struct {
	get func(int, *unix.Rlimit) error
	set func(int, *unix.Rlimit) error
}

func installResourceLimits(limits []resourceLimit, system rlimitSystem) error {
	if system.get == nil || system.set == nil {
		return fmt.Errorf("resource-limit system calls are unavailable")
	}
	for _, limit := range limits {
		if limit.name == "" {
			return fmt.Errorf("resource limit has no name")
		}
		var current unix.Rlimit
		if err := system.get(limit.resource, &current); err != nil {
			return fmt.Errorf("read %s limit: %w", limit.name, err)
		}
		target := limit.maximum
		if current.Cur < target {
			target = current.Cur
		}
		if current.Max < target {
			target = current.Max
		}
		softRestricted := unix.Rlimit{Cur: target, Max: current.Max}
		if err := system.set(limit.resource, &softRestricted); err != nil {
			return fmt.Errorf("set %s soft limit: %w", limit.name, err)
		}
		restricted := unix.Rlimit{Cur: target, Max: target}
		if err := system.set(limit.resource, &restricted); err != nil {
			return fmt.Errorf("set %s hard limit: %w", limit.name, err)
		}
		var verified unix.Rlimit
		if err := system.get(limit.resource, &verified); err != nil {
			return fmt.Errorf("verify %s limit: %w", limit.name, err)
		}
		if verified != restricted {
			return fmt.Errorf("verify %s limit: got cur=%d max=%d, want cur=%d max=%d", limit.name, verified.Cur, verified.Max, restricted.Cur, restricted.Max)
		}
	}
	return nil
}

func nativeRlimitSystem() rlimitSystem {
	return rlimitSystem{get: unix.Getrlimit, set: unix.Setrlimit}
}

func baseWorkerResourceLimits() []resourceLimit {
	return []resourceLimit{
		{name: "CPU", resource: unix.RLIMIT_CPU, maximum: workerCPUSeconds},
		{name: "core file", resource: unix.RLIMIT_CORE, maximum: 0},
		{name: "output file", resource: unix.RLIMIT_FSIZE, maximum: workerFileBytes},
		{name: "open file", resource: unix.RLIMIT_NOFILE, maximum: workerOpenFiles},
		{name: "stack", resource: unix.RLIMIT_STACK, maximum: workerStackBytes},
	}
}
