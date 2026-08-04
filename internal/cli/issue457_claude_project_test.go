package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue457ClaudeProjectInstallUsesNativeDeclarativeSurfaces(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	if err := os.WriteFile(filepath.Join(project, "CLAUDE.md"), []byte("# Team instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".mcp.json"), []byte("{\n  \"foreign\": true\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "addy", "--surface", "claude"); err != nil {
		t.Fatalf("install Addy for Claude: %v\n%s", err, out)
	}
	for _, relative := range []string{
		".claude/skills/using-agent-skills/SKILL.md",
		".claude/skills/build/SKILL.md",
		".claude/agents/code-reviewer.md",
		".claude/assets/definition-of-done/definition-of-done.md",
	} {
		if _, err := os.Stat(filepath.Join(project, relative)); err != nil {
			t.Fatalf("missing Claude project projection %s: %v", relative, err)
		}
	}

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "addy", "--surface", "claude", "--project", "--require", "installed")
	if err != nil {
		t.Fatalf("Claude project status for Addy: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "addy", "--surface", "claude"); err != nil {
		t.Fatalf("uninstall Addy for Claude: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(project, "CLAUDE.md")); err != nil {
		t.Fatalf("Claude uninstall removed foreign instructions: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); err != nil {
		t.Fatalf("Claude uninstall removed foreign MCP content: %v", err)
	}
}

func TestIssue457ClaudeProjectDryRunDoesNotChangeGlobalActivation(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	before := snapshotTree(t, home)

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "claude", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("preview Claude project install: %v\n%s", err, out)
	}
	var preview capabilitypack.JSONProjectInstallPreview
	if err := json.Unmarshal([]byte(out), &preview); err != nil || preview.Surface != capabilitypack.SurfaceClaude {
		t.Fatalf("Claude preview = %#v, err=%v", preview, err)
	}
	if snapshotTree(t, home) != before || exists(filepath.Join(home, ".claude", "skills", "ask-matt")) {
		t.Fatal("Claude project dry-run changed global Claude activation state")
	}
}
