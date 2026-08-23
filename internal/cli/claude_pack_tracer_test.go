package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	foreignSkill := filepath.Join(home, ".claude", "skills", "ask-matt")
	if err := os.MkdirAll(foreignSkill, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreignSkill, "FOREIGN.md"), []byte("foreign\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	terminal.calls = 0
	before := snapshotTree(t, home)
	out, err := executeCommand(t, NewRootCommand(opts), "activate", "matty", "--surface", "claude")
	if err == nil || !strings.Contains(out, "Compatibility: blocked") || !strings.Contains(out, "Expected readiness: configured=false") || !strings.Contains(out, "Cannot apply activation: 1 blockers") {
		t.Fatalf("blocked Claude activation: err=%v\n%s", err, out)
	}
	if terminal.calls != 0 || snapshotTree(t, home) != before {
		t.Fatal("blocked Claude activation prompted or executed an effect")
	}
}
