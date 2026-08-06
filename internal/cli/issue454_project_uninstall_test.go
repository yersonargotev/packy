package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssue454PackWideUninstallPreviewsEveryRemovalWithoutMutation(t *testing.T) {
	version, _ := checkedInMattyFacts(t)
	opts, project := installIssue453Project(t)
	before := snapshotTree(t, project)
	terminal := opts.Terminal.(*fakeTerminal)
	terminal.calls = 0

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty", "--dry-run")
	if err != nil {
		t.Fatalf("pack-wide uninstall dry-run: %v\n%s", err, out)
	}
	for _, want := range []string{
		"COMPLETE PROJECT PACK UNINSTALL DRY-RUN",
		"Pack: matty " + version,
		"Scope: complete pack",
		"packy.json",
		"packy.lock.json",
		"PACKY-NOTICES.md",
		"skill:ask-matt -> .agents/skills/ask-matt",
		"Disposition: previewable",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("uninstall preview missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != 0 {
		t.Fatalf("dry-run approvals = %d, want 0", terminal.calls)
	}
	if got := snapshotTree(t, project); got != before {
		t.Fatal("uninstall dry-run mutated the project")
	}
}

func TestIssue454PackWideUninstallRemovesOnlyExactOwnedContent(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	agentsPath := filepath.Join(project, "AGENTS.md")
	noticesPath := filepath.Join(project, "PACKY-NOTICES.md")
	unrelatedPath := filepath.Join(project, "unrelated.txt")
	if err := os.WriteFile(agentsPath, []byte("foreign agents preamble\n\nforeign agents epilogue\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noticesPath, []byte("foreign notices preamble\n\nforeign notices epilogue\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelatedPath, []byte("unrelated dirty content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitBefore := snapshotTree(t, filepath.Join(project, ".git"))
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("seed install: %v\n%s", err, out)
	}
	terminal.calls = 0

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty")
	if err != nil {
		t.Fatalf("pack-wide uninstall: %v\n%s", err, out)
	}
	for _, want := range []string{"COMPLETE PROJECT PACK UNINSTALL PREVIEW", "Approve complete project pack uninstall", "Verified project uninstall"} {
		if !strings.Contains(out, want) {
			t.Fatalf("uninstall output missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != 1 {
		t.Fatalf("uninstall approvals = %d, want 1", terminal.calls)
	}
	for _, target := range []string{"packy.json", "packy.lock.json"} {
		if _, err := os.Lstat(filepath.Join(project, target)); !os.IsNotExist(err) {
			t.Fatalf("owned contract target %s remains: %v", target, err)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(project, ".agents", "skills")); err != nil || len(entries) != 0 {
		t.Fatalf("owned skill projections remain: entries=%v err=%v", entries, err)
	}
	if got := readFileString(t, agentsPath); !strings.Contains(got, "foreign agents preamble") || !strings.Contains(got, "foreign agents epilogue") || strings.Contains(got, "packy:project:matty:codex") {
		t.Fatalf("foreign AGENTS.md content was not preserved exactly around the removed contribution: %q", got)
	}
	if got := readFileString(t, noticesPath); !strings.Contains(got, "foreign notices preamble") || !strings.Contains(got, "foreign notices epilogue") || strings.Contains(got, "packy:project:matty:notices") {
		t.Fatalf("foreign notices content was not preserved exactly around the removed contribution: %q", got)
	}
	if got := readFileString(t, unrelatedPath); got != "unrelated dirty content\n" {
		t.Fatalf("unrelated content changed: %q", got)
	}
	if got := snapshotTree(t, filepath.Join(project, ".git")); got != gitBefore {
		t.Fatal("uninstall mutated Git metadata")
	}
	if _, err := os.Stat(filepath.Join(opts.Env.Getenv("HOME"), ".packy", "projects")); !os.IsNotExist(err) {
		t.Fatalf("uninstall retained recovery state: %v", err)
	}
}

func TestIssue454SurfaceUninstallRemovesOnlyTheSelectedContributor(t *testing.T) {
	opts, project := installIssue453Project(t)
	terminal := opts.Terminal.(*fakeTerminal)
	terminal.calls = 0

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty", "--surface", "codex")
	if err != nil {
		t.Fatalf("surface uninstall: %v\n%s", err, out)
	}
	for _, want := range []string{"PROJECT SURFACE UNINSTALL PREVIEW", "Scope: surface codex", "Approve project surface uninstall", "Verified project uninstall"} {
		if !strings.Contains(out, want) {
			t.Fatalf("surface uninstall output missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != 1 {
		t.Fatalf("surface uninstall approvals = %d, want 1", terminal.calls)
	}
	if _, err := os.Stat(filepath.Join(project, "packy.lock.json")); !os.IsNotExist(err) {
		t.Fatalf("last selected surface did not remove the now-empty pack contract: %v", err)
	}
}

func TestIssue454UninstallPreservesDriftAndForeignMatchingContent(t *testing.T) {
	opts, project := installIssue453Project(t)
	drift := filepath.Join(project, ".agents", "skills", "ask-matt", "SKILL.md")
	if err := os.WriteFile(drift, []byte("owned drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, project)
	terminal := opts.Terminal.(*fakeTerminal)
	terminal.calls = 0
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty")
	if err == nil || !strings.Contains(out, "project_drift") {
		t.Fatalf("drifted uninstall = err:%v\n%s", err, out)
	}
	if terminal.calls != 0 || snapshotTree(t, project) != before {
		t.Fatal("blocked drifted uninstall approved or mutated content")
	}

	if err := os.Remove(filepath.Join(project, "packy.lock.json")); err != nil {
		t.Fatal(err)
	}
	before = snapshotTree(t, project)
	_, err = executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty")
	if err == nil || snapshotTree(t, project) != before {
		t.Fatalf("matching bytes without lock were adopted or removed: %v", err)
	}
}

func TestIssue454UninstallRejectsConcurrentChangeAfterApproval(t *testing.T) {
	opts, project := installIssue453Project(t)
	terminal := opts.Terminal.(*fakeTerminal)
	target := filepath.Join(project, "AGENTS.md")
	terminal.onApprove = func() {
		file, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0)
		if err == nil {
			_, err = file.WriteString("concurrent foreign content\n")
			_ = file.Close()
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "uninstall", "matty")
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("concurrent uninstall = err:%v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(project, "packy.lock.json")); err != nil {
		t.Fatal("stale uninstall removed the project contract")
	}
}
