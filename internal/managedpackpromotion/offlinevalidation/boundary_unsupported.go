//go:build (!darwin && !linux) || (linux && !amd64 && !arm64)

package offlinevalidation

import (
	"context"
	"errors"
	"os/exec"
)

func workerCommand(context.Context, processInvocation) (*exec.Cmd, error) {
	return nil, errors.New("offline validation has no enforceable worker boundary on this platform")
}

func installWorkerBoundary() error {
	return errors.New("offline validation has no enforceable worker boundary on this platform")
}
