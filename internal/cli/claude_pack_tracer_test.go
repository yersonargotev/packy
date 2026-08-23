package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

// TestClaudeMattyTracerActivatesStatusesAndDeactivatesInSandbox is the one
// real-catalog CLI integration smoke: it protects Matty's reviewed Claude
// projection and cleanup contract end to end.
func TestClaudeMattyTracerActivatesStatusesAndDeactivatesInSandbox(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte("operator-owned guidance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shown, err := executeCommand(t, NewRootCommand(opts), "show", "matty")
	if err != nil || !strings.Contains(shown, "Surface contract: claude") || !strings.Contains(shown, "Compatibility: complete") || !strings.Contains(shown, "Binding: skill:ask-matt") {
		t.Fatalf("Claude tracer show: err=%v\n%s", err, shown)
	}
	beforePreview := snapshotTree(t, home)

	preview, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "claude", "--dry-run")
	facts := checkedInMattyFacts(t)
	if err != nil || !strings.Contains(preview, "skill:ask-matt") || !strings.Contains(preview, fmt.Sprintf("Logical resources: %d skill, 0 instruction", facts.Skills)) || !strings.Contains(preview, "Expected readiness: configured=true, authorized=unknown, usable=unknown") || !strings.Contains(preview, "Pending evidence:") {
		t.Fatalf("Claude tracer preview: err=%v\n%s", err, preview)
	}
	for _, retired := range []string{"matty-guidance", "matty-workflow-conventions"} {
		if strings.Contains(preview, retired) {
			t.Fatalf("retired instruction %q entered Claude preview:\n%s", retired, preview)
		}
	}
	if snapshotTree(t, home) != beforePreview {
		t.Fatal("Claude dry-run mutated the sandbox")
	}
	activated, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "claude")
	if err != nil {
		t.Fatalf("Claude tracer activate: %v\n%s", err, activated)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "ask-matt")); err != nil {
		t.Fatalf("Claude workflow skill was not projected: %v", err)
	}
	instructions, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil || string(instructions) != "operator-owned guidance\n" {
		t.Fatalf("Claude activation changed foreign instructions: %v\n%s", err, instructions)
	}
	status, err := executeCommand(t, NewRootCommand(opts), "status", "matty", "--surface", "claude")
	if err != nil || !strings.Contains(status, "configured=true") || !strings.Contains(status, "authorized=unknown") {
		t.Fatalf("Claude tracer status: err=%v\n%s", err, status)
	}
	if !strings.Contains(status, "Compatibility: complete") {
		t.Fatalf("Claude tracer compatibility missing from status:\n%s", status)
	}
	updated, err := executeCommand(t, NewRootCommand(opts), "update", "matty", "--surface", "claude")
	if err != nil || !strings.Contains(updated, "Already converged") {
		t.Fatalf("Claude tracer update: err=%v\n%s", err, updated)
	}
	deactivated, err := executeCommand(t, NewRootCommand(opts), "deactivate", "matty", "--surface", "claude")
	if err != nil {
		t.Fatalf("Claude tracer deactivate: %v\n%s", err, deactivated)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "ask-matt")); !os.IsNotExist(err) {
		t.Fatal("Claude workflow skill survived deactivation")
	}
	instructions, err = os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
	if err != nil || string(instructions) != "operator-owned guidance\n" {
		t.Fatalf("Claude last-contributor cleanup changed foreign instructions: err=%v\n%s", err, instructions)
	}
}

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
