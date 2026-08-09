package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

func TestIssue452MattyCodexProjectInstallMutatesRecoverablyAndRepeatsAsNoOp(t *testing.T) {
	version, resources := checkedInMattyFacts(t)
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, repoRoot := packActivationOptions(t, terminal)
	project := filepath.Join(t.TempDir(), "project")
	nested := filepath.Join(project, "one", "two")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return nested, nil }
	dirtyPath := filepath.Join(project, "unrelated.txt")
	agentsPath := filepath.Join(project, "AGENTS.md")
	noticesPath := filepath.Join(project, "PACKY-NOTICES.md")
	if err := os.WriteFile(dirtyPath, []byte("unrelated dirty content\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentsPath, []byte("foreign preamble\n\nforeign epilogue\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noticesPath, []byte("foreign notice preamble\n\nforeign notice epilogue\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	gitBefore := snapshotTree(t, filepath.Join(project, ".git"))

	out, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex")
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	for _, want := range []string{"Project install preview", "Approve project installation", "Verified project installation"} {
		if !strings.Contains(out, want) {
			t.Fatalf("install output missing %q:\n%s", want, out)
		}
	}
	if terminal.calls != 1 {
		t.Fatalf("approval calls = %d, want 1", terminal.calls)
	}

	manifestData, err := os.ReadFile(filepath.Join(project, "packy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SchemaVersion          int    `json:"schema_version"`
		MinimumPackyCapability string `json:"minimum_packy_capability"`
		Packs                  []struct {
			ID             string                                `json:"id"`
			SurfaceIntents []capabilitypack.ProjectSurfaceIntent `json:"surface_intents"`
		} `json:"packs"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode manifest: %v\n%s", err, manifestData)
	}
	if manifest.SchemaVersion != 1 || manifest.MinimumPackyCapability != "" || len(manifest.Packs) != 1 || manifest.Packs[0].ID != "matty" || len(manifest.Packs[0].SurfaceIntents) != 1 || manifest.Packs[0].SurfaceIntents[0].Version != version || manifest.Packs[0].SurfaceIntents[0].Surface != capabilitypack.SurfaceCodex {
		t.Fatalf("manifest = %#v", manifest)
	}

	lockData, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock capabilitypack.ProjectLockProposal
	if err := json.Unmarshal(lockData, &lock); err != nil {
		t.Fatalf("decode lock: %v\n%s", err, lockData)
	}
	if lock.SchemaVersion != 1 || len(lock.Receipts) != 1 || len(lock.Receipts[0].Resources) != resources+1 || len(lock.Receipts[0].Projections) != resources+2 {
		t.Fatalf("lock = %#v", lock)
	}

	notices, err := os.ReadFile(noticesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(notices), "foreign notice preamble") || !strings.Contains(string(notices), "Packy project notices") || !strings.Contains(string(notices), "Reviewed Pack content is authoritative") || !strings.Contains(string(notices), "foreign notice epilogue") {
		t.Fatalf("notices = %q", notices)
	}
	if info, err := os.Stat(noticesPath); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("PACKY-NOTICES.md mode = %v, %v; want 0640", info, err)
	}
	agents, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"foreign preamble", "<!-- packy:project:instruction:matty-codex-project:start -->", ".agents/skills", "<!-- packy:project:instruction:matty-codex-project:end -->", "foreign epilogue"} {
		if !strings.Contains(string(agents), want) {
			t.Fatalf("AGENTS.md missing %q:\n%s", want, agents)
		}
	}
	if info, err := os.Stat(agentsPath); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("AGENTS.md mode = %v, %v; want 0640", info, err)
	}

	for _, projection := range lock.Projections {
		if projection.Resource.Kind != "skill" {
			continue
		}
		target := filepath.Join(project, filepath.FromSlash(projection.Target))
		info, err := os.Lstat(target)
		if err != nil {
			t.Fatalf("stat %s: %v", projection.Resource, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s was not installed as a copied tree: %v", projection.Resource, info.Mode())
		}
		fingerprint, err := localprojection.FingerprintExactTree(target)
		if err != nil || fingerprint != projection.DesiredFingerprint {
			t.Fatalf("%s fingerprint = %q, %v; want %q", projection.Resource, fingerprint, err, projection.DesiredFingerprint)
		}
	}
	if got := readFileString(t, dirtyPath); got != "unrelated dirty content\n" {
		t.Fatalf("dirty content changed: %q", got)
	}
	if snapshotTree(t, filepath.Join(project, ".git")) != gitBefore || exists(filepath.Join(home, ".packy", "projects")) || len(opts.Runner.(*fakeRunner).calls) != 0 {
		t.Fatalf("install caused Git, Packy Home, or process effects: calls=%v", opts.Runner.(*fakeRunner).calls)
	}

	if err := os.WriteFile(agentsPath, []byte(strings.Replace(string(agents), "foreign epilogue", "changed foreign epilogue", 1)), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(noticesPath, []byte(strings.Replace(string(notices), "foreign notice epilogue", "changed foreign notice epilogue", 1)), 0o640); err != nil {
		t.Fatal(err)
	}
	beforeRepeat := snapshotTree(t, project)
	terminal.calls = 0
	repeated, repeatErr := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex")
	if repeatErr != nil {
		t.Fatalf("repeat install: %v\n%s", repeatErr, repeated)
	}
	if !strings.Contains(repeated, "Verified no-op") || terminal.calls != 0 || snapshotTree(t, project) != beforeRepeat {
		t.Fatalf("repeat was not an exact no-op: approvals=%d\n%s", terminal.calls, repeated)
	}
	_ = repoRoot
}

func TestIssue452ProjectInstallBlocksForeignTargetsAndAmbiguousMarkersBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, project string)
		want  string
	}{
		{name: "foreign non-composable target", setup: func(t *testing.T, project string) {
			t.Helper()
			target := filepath.Join(project, ".agents", "skills", "ask-matt")
			if err := os.MkdirAll(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("foreign\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, want: "foreign_target"},
		{name: "ambiguous Packy markers", setup: func(t *testing.T, project string) {
			t.Helper()
			block := "<!-- packy:project:instruction:matty-codex-project:start -->\nchanged\n<!-- packy:project:instruction:matty-codex-project:end -->\n"
			if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte(block+block), 0o600); err != nil {
				t.Fatal(err)
			}
		}, want: "foreign_target"},
		{name: "symlinked project target ancestor", setup: func(t *testing.T, project string) {
			t.Helper()
			if err := os.Symlink(t.TempDir(), filepath.Join(project, ".agents")); err != nil {
				t.Fatal(err)
			}
		}, want: "unsafe_path"},
		{name: "symlinked composable target", setup: func(t *testing.T, project string) {
			t.Helper()
			outside := filepath.Join(t.TempDir(), "outside.md")
			if err := os.WriteFile(outside, []byte("foreign outside content\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(project, "AGENTS.md")); err != nil {
				t.Fatal(err)
			}
		}, want: "unsafe_path"},
	} {
		t.Run(test.name, func(t *testing.T) {
			terminal := &fakeTerminal{interactive: true, approve: true}
			opts, _, _ := packActivationOptions(t, terminal)
			project := t.TempDir()
			writeTestGitWorktree(t, project)
			test.setup(t, project)
			opts.Getwd = func() (string, error) { return project, nil }
			before := snapshotTree(t, project)
			out, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex")
			if err == nil || !strings.Contains(out, test.want) || terminal.calls != 0 {
				t.Fatalf("blocked install = err:%v approvals:%d\n%s", err, terminal.calls, out)
			}
			if snapshotTree(t, project) != before {
				t.Fatal("blocked install changed the project")
			}
		})
	}
}

func TestIssue452ProjectInstallRejectsConcurrentTargetChangeAfterApproval(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	agents := filepath.Join(project, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("before approval\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts.Getwd = func() (string, error) { return project, nil }
	terminal.onApprove = func() {
		if err := os.WriteFile(agents, []byte("concurrent foreign change\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex")
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale install = %v\n%s", err, out)
	}
	if got := readFileString(t, agents); got != "concurrent foreign change\n" {
		t.Fatalf("concurrent content changed: %q", got)
	}
	for _, target := range []string{"packy.json", "packy.lock.json", "PACKY-NOTICES.md", ".agents"} {
		if _, err := os.Lstat(filepath.Join(project, target)); !os.IsNotExist(err) {
			t.Fatalf("stale install created %s: %v", target, err)
		}
	}
}

func TestIssue452ProjectInstallPreservesOwnedDriftAndRefusesRepeat(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("seed install: %v\n%s", err, out)
	}
	drift := filepath.Join(project, ".agents", "skills", "ask-matt", "SKILL.md")
	if err := os.WriteFile(drift, []byte("owned drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	terminal.calls = 0
	out, err := executeCommand(t, NewRootCommand(opts), "install", "matty", "--surface", "codex")
	if err == nil || !strings.Contains(out, "owned_drift") || terminal.calls != 0 {
		t.Fatalf("drifted repeat = err:%v approvals:%d\n%s", err, terminal.calls, out)
	}
	if got := readFileString(t, drift); got != "owned drift\n" {
		t.Fatalf("owned drift was overwritten: %q", got)
	}
}
