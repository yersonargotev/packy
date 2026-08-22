//go:build darwin

package offlinevalidation

import (
	"context"
	"strings"
	"testing"
)

func TestDarwinWorkerCommandFailsClosedWithoutTheSandboxLauncher(t *testing.T) {
	_, err := darwinWorkerCommand(context.Background(), processInvocation{
		Executable: "/bin/worker",
		Args:       []string{ModeArgument},
	}, "/definitely-missing/packy-sandbox-exec")
	if err == nil || !strings.Contains(err.Error(), "inspect macOS sandbox launcher") {
		t.Fatalf("darwinWorkerCommand() error = %v", err)
	}
}
