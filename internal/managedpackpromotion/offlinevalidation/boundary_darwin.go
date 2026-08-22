//go:build darwin

package offlinevalidation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const (
	darwinSandboxExecutable = "/usr/bin/sandbox-exec"
	darwinSandboxProfile    = `(version 1)(allow default)(deny network*)(deny process-fork)`
	darwinWorkerProcesses   = uint64(1)
)

func workerCommand(ctx context.Context, invocation processInvocation) (*exec.Cmd, error) {
	return darwinWorkerCommand(ctx, invocation, darwinSandboxExecutable)
}

func darwinWorkerCommand(ctx context.Context, invocation processInvocation, launcher string) (*exec.Cmd, error) {
	if !filepath.IsAbs(launcher) || filepath.Clean(launcher) != launcher {
		return nil, errors.New("macOS sandbox launcher must be an absolute clean path")
	}
	info, err := os.Lstat(launcher)
	if err != nil {
		return nil, fmt.Errorf("inspect macOS sandbox launcher: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, errors.New("macOS sandbox launcher must be an executable regular file and not a symlink")
	}
	arguments := []string{"-p", darwinSandboxProfile, invocation.Executable}
	arguments = append(arguments, invocation.Args...)
	return exec.CommandContext(ctx, launcher, arguments...), nil
}

func installWorkerBoundary() error {
	if err := installRuntimeBounds(); err != nil {
		return err
	}
	limits := append(baseWorkerResourceLimits(), resourceLimit{
		name: "process", resource: unix.RLIMIT_NPROC, maximum: darwinWorkerProcesses,
	})
	if err := installResourceLimits(limits, nativeRlimitSystem()); err != nil {
		return err
	}
	return verifyDarwinNetworkDenied()
}

func verifyDarwinNetworkDenied() error {
	descriptor, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
			return nil
		}
		return fmt.Errorf("create network boundary probe socket: %w", err)
	}
	defer unix.Close(descriptor)
	err = unix.Connect(descriptor, &unix.SockaddrInet4{Port: 9, Addr: [4]byte{127, 0, 0, 1}})
	if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
		return nil
	}
	if err == nil {
		return errors.New("network boundary probe unexpectedly connected")
	}
	return fmt.Errorf("network boundary probe was not denied: %w", err)
}
