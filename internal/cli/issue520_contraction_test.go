package cli

import (
	"strings"
	"testing"
)

func TestIssue520RecoveryOrientedReconcileCommandIsRemoved(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{})
	root := NewRootCommand(opts)
	pack, _, err := root.Find([]string{"pack"})
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range pack.Commands() {
		if command.Name() == "reconcile" {
			t.Fatal("reconcile command remained available")
		}
	}
}

func TestIssue520CurrentLifecycleAcrossSupportedAdapters(t *testing.T) {
	for _, surface := range []string{"codex", "opencode", "claude"} {
		t.Run(surface, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, _, _ := currentPackActivationOptions(t, terminal)
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", surface); err != nil || !strings.Contains(out, "Verified plan") {
				t.Fatalf("activate: %v\n%s", err, out)
			}
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", surface); err != nil || !strings.Contains(out, "active") {
				t.Fatalf("status: %v\n%s", err, out)
			}
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--surface", surface); err != nil || !strings.Contains(out, "Already converged") {
				t.Fatalf("update: %v\n%s", err, out)
			}
			if out, err := executeCommand(t, NewRootCommand(opts), "pack", "deactivate", "matty", "--surface", surface); err != nil || !strings.Contains(out, "Verified plan") {
				t.Fatalf("deactivate: %v\n%s", err, out)
			}
		})
	}
}
