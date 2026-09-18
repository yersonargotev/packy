package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestClaudeBlockedSyntheticActivationExecutesZeroEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	pack := testsupport.CapabilityRich("claude-foreign-target")
	fixture := newSyntheticCLIFixture(t, terminal, pack)
	resourceID := pack.OperationalResource()
	resource := syntheticResource(t, pack, resourceID.Kind, resourceID.ID)
	var targetName string
	for _, binding := range resource.Bindings {
		if binding.Surface == testsupport.SurfaceClaude {
			targetName = binding.Name
			break
		}
	}
	if targetName == "" {
		t.Fatalf("synthetic resource %s has no Claude binding", resourceID.String())
	}
	foreignTarget := filepath.Join(fixture.home, ".claude", "skills", targetName)
	if err := os.MkdirAll(foreignTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreignTarget, "FOREIGN.md"), []byte("foreign\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, fixture.home)
	out, err := executeCommand(t, NewRootCommand(fixture.options), "activate", pack.ID(), "--surface", "claude", "--resource", resourceID.String())
	if err == nil || !strings.Contains(out, "Compatibility: blocked") || !strings.Contains(out, "Expected readiness: configured=false") || !strings.Contains(out, "Cannot apply activation: 1 blockers") {
		t.Fatalf("blocked Claude activation: err=%v\n%s", err, out)
	}
	if runner := fixture.options.Runner.(*fakeRunner); terminal.calls != 0 || len(runner.calls) != 0 || snapshotTree(t, fixture.home) != before {
		t.Fatalf("blocked Claude activation caused effects: prompts=%d processes=%v", terminal.calls, runner.calls)
	}
}
