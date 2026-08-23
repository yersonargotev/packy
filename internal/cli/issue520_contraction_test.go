package cli

import (
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
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
			pack := testsupport.PortableAllSurfaces("lifecycle-" + surface)
			fixture := newSyntheticCLIFixture(t, terminal, pack)
			packID := pack.Manifest().ID
			if out, err := executeCommand(t, NewRootCommand(fixture.options), "activate", packID, "--surface", surface); err != nil || !strings.Contains(out, "Verified plan") {
				t.Fatalf("activate: %v\n%s", err, out)
			}
			if out, err := executeCommand(t, NewRootCommand(fixture.options), "status", packID, "--surface", surface); err != nil || !strings.Contains(out, "active") {
				t.Fatalf("status: %v\n%s", err, out)
			}
			if out, err := executeCommand(t, NewRootCommand(fixture.options), "update", packID, "--surface", surface); err != nil || !strings.Contains(out, "Already converged") {
				t.Fatalf("update: %v\n%s", err, out)
			}
			if out, err := executeCommand(t, NewRootCommand(fixture.options), "deactivate", packID, "--surface", surface); err != nil || !strings.Contains(out, "Verified plan") {
				t.Fatalf("deactivate: %v\n%s", err, out)
			}
		})
	}
}
