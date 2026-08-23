package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func issue457RichInstallArgs(packID, surface string) []string {
	return []string{"install", packID, "--surface", surface, "--resource", "skill:workflow", "--resource", "agent:reviewer"}
}

func TestIssue457ClaudeProjectInstallUsesNativeDeclarativeSurfaces(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	pack := testsupport.CapabilityRich("project-native")
	opts := newSyntheticCLIFixture(t, terminal, pack).options
	packID := pack.Manifest().ID
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	if err := os.WriteFile(filepath.Join(project, "CLAUDE.md"), []byte("# Team instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".mcp.json"), []byte("{\n  \"foreign\": true\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := executeCommand(t, NewRootCommand(opts), issue457RichInstallArgs(packID, "claude")...); err != nil {
		t.Fatalf("install synthetic Pack for Claude: %v\n%s", err, out)
	}
	for _, relative := range []string{
		".claude/skills/helper/SKILL.md",
		".claude/skills/workflow/SKILL.md",
		".claude/agents/reviewer.md",
		".claude/assets/reference/RESOURCE",
	} {
		if _, err := os.Stat(filepath.Join(project, relative)); err != nil {
			t.Fatalf("missing Claude project projection %s: %v", relative, err)
		}
	}

	out, err := executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", "claude", "--project", "--require", "installed")
	if err != nil {
		t.Fatalf("Claude project status for synthetic Pack: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), issue457RichInstallArgs(packID, "opencode")...); err != nil {
		t.Fatalf("add OpenCode project surface: %v\n%s", err, out)
	}
	missingSkill := filepath.Join(project, ".claude", "skills", "helper")
	if err := os.RemoveAll(missingSkill); err != nil {
		t.Fatal(err)
	}
	missingOpenCodeAgent := filepath.Join(project, ".opencode", "agents", "reviewer.md")
	if err := os.Remove(missingOpenCodeAgent); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), issue457RichInstallArgs(packID, "claude")...); err != nil {
		t.Fatalf("reconcile Claude receipt: %v\n%s", err, out)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), issue457RichInstallArgs(packID, "opencode")...); err != nil {
		t.Fatalf("reconcile OpenCode receipt: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(missingSkill, "SKILL.md")); err != nil {
		t.Fatalf("Claude project reconcile did not restore the locked skill: %v", err)
	}
	if _, err := os.Stat(missingOpenCodeAgent); err != nil {
		t.Fatalf("OpenCode receipt reconcile did not restore its projection: %v", err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "uninstall", packID, "--surface", "claude"); err != nil {
		t.Fatalf("uninstall synthetic Pack for Claude: %v\n%s", err, out)
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
	pack := testsupport.CapabilityRich("project-claude-preview")
	fixture := newSyntheticCLIFixture(t, terminal, pack)
	opts, home := fixture.options, fixture.home
	packID := pack.Manifest().ID
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	before := snapshotTree(t, home)

	out, err := executeCommand(t, NewRootCommand(opts), "install", packID, "--surface", "claude", "--resource", "skill:helper", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("preview Claude project install: %v\n%s", err, out)
	}
	var preview capabilitypack.JSONProjectInstallPreview
	if err := json.Unmarshal([]byte(out), &preview); err != nil || preview.Surface != capabilitypack.SurfaceClaude {
		t.Fatalf("Claude preview = %#v, err=%v", preview, err)
	}
	if snapshotTree(t, home) != before || exists(filepath.Join(home, ".claude", "skills", "helper")) {
		t.Fatal("Claude project dry-run changed global Claude activation state")
	}
	globalOut, globalErr := executeCommand(t, NewRootCommand(opts), "activate", packID, "--surface", "claude", "--resource", "skill:helper", "--dry-run")
	if globalErr != nil || !strings.Contains(globalOut, "kind=claude-skill-link") || strings.Contains(globalOut, "claude-project") {
		t.Fatalf("global Claude activation behavior changed: %v\n%s", globalErr, globalOut)
	}
}
