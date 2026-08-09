package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue451MattyCodexProjectInstallPreviewIsCompleteAndEffectFree(t *testing.T) {
	version, resources := checkedInMattyFacts(t)
	opts, home, repoRoot := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	runner := opts.Runner.(*fakeRunner)
	project := filepath.Join(t.TempDir(), "project")
	nested := filepath.Join(project, "one", "two")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return nested, nil }
	beforeProject := snapshotTree(t, project)
	beforeHome := snapshotTree(t, home)
	beforeBundle := snapshotTree(t, filepath.Join(repoRoot, "bundle"))

	human, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex", "--dry-run")
	if err != nil {
		t.Fatalf("human preview: %v\n%s", err, human)
	}
	for _, want := range []string{
		"Project install dry-run", "Project root: <project-root>", "Pack: matty " + version, "Surface: codex",
		fmt.Sprintf("Selection: all (%d resources)", resources), "Manifest: packy.json", "Lock: packy.lock.json",
		"Notices: PACKY-NOTICES.md (0 contributions)", filepath.Join(".agents", "skills", "ask-matt"),
		"Reviewed Pack: matty@" + version, fmt.Sprintf("Lock receipt: %d resources, %d projections", resources, resources+1),
		"Requirements: none", "Expected readiness: configured=true, authorized=unknown, usable=unknown", "Readiness condition:", "Blockers: none", "Disposition: previewable",
	} {
		if !strings.Contains(human, want) {
			t.Fatalf("human preview missing %q:\n%s", want, human)
		}
	}

	structured, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("JSON preview: %v\n%s", err, structured)
	}
	var report capabilitypack.JSONProjectInstallPreview
	if err := json.Unmarshal([]byte(structured), &report); err != nil {
		t.Fatalf("decode JSON preview: %v\n%s", err, structured)
	}
	if report.SchemaVersion != capabilitypack.ProjectInstallPreviewSchemaVersion || report.Report != "project-install-preview" || !report.DryRun || report.ProjectRoot != "<project-root>" || report.Pack.ID != "matty" || report.Pack.Version != version || report.Surface != capabilitypack.SurfaceCodex || report.Selection.Mode != capabilitypack.SelectionAll || len(report.Selection.Resources) != resources || len(report.Projections) != resources+1 || report.Manifest.Path != "packy.json" || report.Lock.SchemaVersion != 1 || len(report.Lock.Receipts) != 1 || report.Notices.Path != "PACKY-NOTICES.md" || report.Notices.Contributions == nil || len(report.Blockers) != 0 || report.Disposition != capabilitypack.ProjectInstallPreviewable || report.ExpectedReadiness.Configured != capabilitypack.ReadinessTrue || report.ExpectedReadiness.Usable != capabilitypack.ReadinessUnknown || len(report.Conditions) == 0 {
		t.Fatalf("incomplete JSON preview: %#v", report)
	}
	if again, repeatErr := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex", "--dry-run", "--json"); repeatErr != nil || again != structured {
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
	if _, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex", "--dry-run"); err == nil || !strings.Contains(err.Error(), "Git worktree") {
		t.Fatalf("outside-worktree error = %v", err)
	}
	writeTestGitWorktree(t, outside)
	if _, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex"); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("non-interactive mutation error = %v", err)
	}
}

func TestIssue451UnrepresentableProjectResourceIsNonActionable(t *testing.T) {
	opts, home, _ := packActivationOptions(t, &fakeTerminal{})
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	beforeProject, beforeHome := snapshotTree(t, project), snapshotTree(t, home)
	human, humanErr := executeCommand(t, NewRootCommand(opts), "install", "addy", "--surface", "codex", "--dry-run")
	if humanErr == nil || !strings.Contains(human, "Legal contribution: notice:mit") || !strings.Contains(human, "license=MIT") || !strings.Contains(human, "attribution=") {
		t.Fatalf("human legal disclosure: err=%v\n%s", humanErr, human)
	}
	out, err := executeCommand(t, NewRootCommand(opts), "install", "addy", "--surface", "codex", "--dry-run", "--json")
	if err == nil || !strings.Contains(err.Error(), "not actionable") {
		t.Fatalf("unrepresentable preview error = %v\n%s", err, out)
	}
	var report capabilitypack.JSONProjectInstallPreview
	if decodeErr := json.Unmarshal(firstJSONDocument(t, out), &report); decodeErr != nil || report.Disposition != capabilitypack.ProjectInstallBlocked || len(report.Blockers) == 0 || report.Blockers[0].Code != "unrepresentable_resource" {
		t.Fatalf("blocked preview = %#v; decode=%v\n%s", report, decodeErr, out)
	}
	if snapshotTree(t, project) != beforeProject || snapshotTree(t, home) != beforeHome || len(opts.Runner.(*fakeRunner).calls) != 0 {
		t.Fatal("blocked preview caused effects")
	}
}

func writeTestGitWorktree(t *testing.T, root string) {
	t.Helper()
	gitDirectory := filepath.Join(root, ".git")
	for _, path := range []string{filepath.Join(gitDirectory, "objects"), filepath.Join(gitDirectory, "refs")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(gitDirectory, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
