package cli

import (
	"strings"
	"testing"
)

func TestIssue520RecoveryOrientedReconcileCommandIsRemoved(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{})
	out, err := executeCommand(t, NewRootCommand(opts), "reconcile")
	if err == nil || !strings.Contains(err.Error(), `unknown command "reconcile"`) {
		t.Fatalf("reconcile route error = %v, output:\n%s", err, out)
	}
}

func TestIssue520CurrentLifecycleAcrossSupportedAdapters(t *testing.T) {
	for _, surface := range []string{"codex", "opencode", "claude"} {
		t.Run(surface, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, _, _ := currentPackActivationOptions(t, terminal)
			if out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", surface); err != nil || !strings.Contains(out, "Verified plan") {
				t.Fatalf("activate: %v\n%s", err, out)
			}
			if out, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", surface); err != nil || !strings.Contains(out, "active") {
				t.Fatalf("status: %v\n%s", err, out)
			}
			if out, err := executeCommand(t, NewRootCommand(opts), "update", "matty", "--surface", surface); err != nil || !strings.Contains(out, "Already converged") {
				t.Fatalf("update: %v\n%s", err, out)
			}
			if out, err := executeCommand(t, NewRootCommand(opts), "deactivate", "matty", "--surface", surface); err != nil || !strings.Contains(out, "Verified plan") {
				t.Fatalf("deactivate: %v\n%s", err, out)
			}
		})
	}
}
