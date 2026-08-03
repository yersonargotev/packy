package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue451MattyCodexProjectInstallPreviewIsCompleteAndEffectFree(t *testing.T) {
	opts, home, repoRoot := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	runner := opts.Runner.(*fakeRunner)
	project := filepath.Join(t.TempDir(), "project")
	nested := filepath.Join(project, "one", "two")
	for _, path := range []string{filepath.Join(project, ".git"), nested} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	opts.Getwd = func() (string, error) { return nested, nil }
	beforeProject := snapshotTree(t, project)
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	human, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatalf("human preview: %v\n%s", err, human)
	}
	for _, want := range []string{
		"Project install dry-run", "Project root: <project-root>", "Pack: matty 4.0.0", "Surface: codex",
		"Selection: all (23 resources)", "Manifest: packy.json", "Lock: packy.lock.json",
		"Notices: PACKY-NOTICES.md (0 contributions)", filepath.Join(".agents", "skills", "ask-matt"),
		"Requirements: none", "Blockers: none", "Disposition: previewable",
	} {
		if !strings.Contains(human, want) {
			t.Fatalf("human preview missing %q:\n%s", want, human)
		}
	}

	structured, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("JSON preview: %v\n%s", err, structured)
	}
	var report capabilitypack.JSONProjectInstallPreview
	if err := json.Unmarshal([]byte(structured), &report); err != nil {
		t.Fatalf("decode JSON preview: %v\n%s", err, structured)
	}
	if report.SchemaVersion != capabilitypack.ProjectInstallPreviewSchemaVersion || report.Report != "project-install-preview" || !report.DryRun || report.ProjectRoot != "<project-root>" || report.Pack.ID != "matty" || report.Pack.Version != "4.0.0" || report.Surface != capabilitypack.SurfaceCodex || report.Selection.Mode != capabilitypack.SelectionAll || len(report.Selection.Resources) != 23 || len(report.Projections) != 23 || report.Manifest.Path != "packy.json" || report.Lock.Path != "packy.lock.json" || report.Notices.Path != "PACKY-NOTICES.md" || report.Notices.Contributions == nil || len(report.Blockers) != 0 || report.Disposition != capabilitypack.ProjectInstallPreviewable {
		t.Fatalf("incomplete JSON preview: %#v", report)
	}
	if again, repeatErr := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--dry-run", "--json"); repeatErr != nil || again != structured {
		t.Fatalf("preview is not deterministic: err=%v\nfirst=%s\nsecond=%s", repeatErr, structured, again)
	}
	if strings.Contains(human, project) || strings.Contains(structured, project) || strings.Contains(structured, home) || strings.Contains(structured, repoRoot) {
		t.Fatalf("preview disclosed workstation paths: %s", structured)
	}
	if len(runner.calls) != 0 || snapshotTree(t, project) != beforeProject || snapshotTree(t, home) != beforeHome || snapshotTree(t, filepath.Join(repoRoot, "bundle")) != beforeBundle {
		t.Fatalf("preview caused effects: calls=%v", runner.calls)
	}
}

func TestIssue451ProjectInstallRequiresGitWorktreeAndDryRun(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{})
	outside := t.TempDir()
	opts.Getwd = func() (string, error) { return outside, nil }
	if _, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex", "--dry-run"); err == nil || !strings.Contains(err.Error(), "Git worktree") {
		t.Fatalf("outside-worktree error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(outside, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex"); err == nil || !strings.Contains(err.Error(), "--dry-run") {
		t.Fatalf("mutation guard error = %v", err)
	}
}

func TestIssue451UnrepresentableProjectResourceIsNonActionable(t *testing.T) {
	opts, home, _ := packActivationOptions(t, &fakeTerminal{})
	project := t.TempDir()
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	opts.Getwd = func() (string, error) { return project, nil }
	beforeProject, beforeHome := snapshotTree(t, project), snapshotTree(t, home)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "engram", "--surface", "codex", "--dry-run", "--json")
	if err == nil || !strings.Contains(err.Error(), "not actionable") {
		t.Fatalf("unrepresentable preview error = %v\n%s", err, out)
	}
	var report capabilitypack.JSONProjectInstallPreview
	if decodeErr := json.Unmarshal([]byte(out), &report); decodeErr != nil || report.Disposition != capabilitypack.ProjectInstallBlocked || len(report.Blockers) == 0 || report.Blockers[0].Code != "unrepresentable_resource" {
		t.Fatalf("blocked preview = %#v; decode=%v\n%s", report, decodeErr, out)
	}
	if snapshotTree(t, project) != beforeProject || snapshotTree(t, home) != beforeHome || len(opts.Runner.(*fakeRunner).calls) != 0 {
		t.Fatal("blocked preview caused effects")
	}
}
