package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func installIssue453Project(t *testing.T) (Options, string) {
	t.Helper()
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("seed project install: %v\n%s", err, out)
	}
	return opts, project
}

func TestIssue453ProjectStatusReportsIndependentAxesOffline(t *testing.T) {
	_, resources := checkedInMattyFacts(t)
	opts, project := installIssue453Project(t)
	opts.Env = MapEnv{
		"HOME": opts.Env.Getenv("HOME"), "XDG_CONFIG_HOME": opts.Env.Getenv("XDG_CONFIG_HOME"),
		"PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(t.TempDir(), "missing-catalog"),
	}

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--require", "installed", "--json")
	if err != nil {
		t.Fatalf("offline project status: %v\n%s", err, out)
	}
	var report capabilitypack.JSONProjectStatusReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode project status: %v\n%s", err, out)
	}
	if report.Report != "project-status" || report.ProjectRoot != "<project-root>" || len(report.Packs) != 1 {
		t.Fatalf("project status report = %#v", report)
	}
	status := report.Packs[0]
	if status.Pack.ID != "matty" || status.Surface != capabilitypack.SurfaceCodex || status.Installation != capabilitypack.ProjectInstallationInstalled || status.Runtime != capabilitypack.ProjectRuntimeNotRequired || !status.RequirementSatisfied {
		t.Fatalf("project status axes = %#v", status)
	}
	if status.Projections == nil || status.Blockers == nil || len(status.Projections) != resources+1 {
		t.Fatalf("project status omitted portable evidence: %#v", status)
	}
	if _, err := os.Stat(filepath.Join(project, "packy.lock.json")); err != nil {
		t.Fatal(err)
	}
}

func TestIssue453ProjectStatusReportsAbsentInstallationSeparately(t *testing.T) {
	terminal := &fakeTerminal{interactive: false}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--json")
	if err != nil {
		t.Fatalf("absent project status: %v\n%s", err, out)
	}
	var report capabilitypack.JSONProjectStatusReport
	if json.Unmarshal([]byte(out), &report) != nil || len(report.Packs) != 1 || report.Packs[0].Installation != capabilitypack.ProjectInstallationAbsent || report.Packs[0].Runtime != capabilitypack.ProjectRuntimePending {
		t.Fatalf("absent project axes = %#v\n%s", report, out)
	}
	_, err = executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--require", "installed")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("absent installed gate = %v", err)
	}
	_, err = executeCommand(t, NewRootCommand(opts), "pack", "status", "--project", "--require", "installed")
	if err == nil || !strings.Contains(err.Error(), "no installed capability packs") {
		t.Fatalf("absent whole-project installed gate = %v", err)
	}
}

func TestIssue453InstalledEnforcementDetectsDriftWithoutMutation(t *testing.T) {
	opts, project := installIssue453Project(t)
	drift := filepath.Join(project, ".agents", "skills", "ask-matt", "SKILL.md")
	if err := os.WriteFile(drift, []byte("drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--require", "installed", "--json")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("installed enforcement error = %v\n%s", err, out)
	}
	var report capabilitypack.JSONProjectStatusReport
	if json.Unmarshal(firstJSONDocument(t, out), &report) != nil || len(report.Packs) != 1 || report.Packs[0].Installation != capabilitypack.ProjectInstallationDrifted || report.Packs[0].RequirementSatisfied {
		t.Fatalf("drift status = %#v\n%s", report, out)
	}
	if snapshotTree(t, project) != before {
		t.Fatal("read-only project status mutated files")
	}
}

func TestIssue453NoArgumentInstallRestoresAuthorizedMissingBytes(t *testing.T) {
	opts, project := installIssue453Project(t)
	missing := filepath.Join(project, ".agents", "skills", "ask-matt")
	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}
	terminal := opts.Terminal.(*fakeTerminal)
	terminal.calls = 0
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "install")
	if err != nil {
		t.Fatalf("reconcile project: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Verified project installation") || terminal.calls != 1 {
		t.Fatalf("reconcile result approvals=%d\n%s", terminal.calls, out)
	}
	if _, err := os.Stat(filepath.Join(missing, "SKILL.md")); err != nil {
		t.Fatalf("missing exact bytes were not restored: %v", err)
	}
}

func TestIssue453NoArgumentInstallRegeneratesStaleLockWithoutFloatingVersion(t *testing.T) {
	version, _ := checkedInMattyFacts(t)
	opts, project := installIssue453Project(t)
	lockPath := filepath.Join(project, "packy.lock.json")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	var lock capabilitypack.ProjectLockProposal
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	lock.ManifestSHA256 = strings.Repeat("0", 64)
	stale, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, append(stale, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "install")
	if err != nil {
		t.Fatalf("regenerate stale lock: %v\n%s", err, out)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if installation.Manifest.Packs[0].Version != version || installation.Lock.Source.PackVersion != version || installation.Lock.ManifestSHA256 == strings.Repeat("0", 64) {
		t.Fatalf("reconciliation floated or retained stale identity: %#v", installation)
	}
}

func TestIssue453NoArgumentInstallNeverInventsMissingBytesFromDigest(t *testing.T) {
	opts, project := installIssue453Project(t)
	missing := filepath.Join(project, ".agents", "skills", "ask-matt")
	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}
	opts.Env = MapEnv{
		"HOME": opts.Env.Getenv("HOME"), "XDG_CONFIG_HOME": opts.Env.Getenv("XDG_CONFIG_HOME"),
		"PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(t.TempDir(), "missing-catalog"),
	}
	before := snapshotTree(t, project)
	_, err := executeCommand(t, NewRootCommand(opts), "pack", "install")
	if err == nil {
		t.Fatal("reconcile invented missing bytes without an exact local source")
	}
	if snapshotTree(t, project) != before {
		t.Fatal("failed offline acquisition mutated the project")
	}
}

func TestIssue453StatusFailsClosedOnUnsupportedContract(t *testing.T) {
	opts, project := installIssue453Project(t)
	manifestPath := filepath.Join(project, "packy.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	unsupported := strings.Replace(string(data), `"schema_version": 3`, `"schema_version": 99`, 1)
	if err := os.WriteFile(manifestPath, []byte(unsupported), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, project)
	_, err = executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported project status error = %v", err)
	}
	if snapshotTree(t, project) != before {
		t.Fatal("status migrated an unsupported project contract")
	}
}

func TestIssue453InstalledEnforcementCoversThePortableContract(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*capabilitypack.ProjectLockProposal)
	}{
		{name: "manifest-to-lock identity", mutate: func(lock *capabilitypack.ProjectLockProposal) { lock.ManifestSHA256 = strings.Repeat("0", 64) }},
		{name: "closure", mutate: func(lock *capabilitypack.ProjectLockProposal) { lock.ResourceGraph.Resources = nil }},
		{name: "target", mutate: func(lock *capabilitypack.ProjectLockProposal) { lock.Projections[0].Target = "../escape" }},
		{name: "mode", mutate: func(lock *capabilitypack.ProjectLockProposal) { lock.Projections[0].Mode = "invented" }},
		{name: "contributor", mutate: func(lock *capabilitypack.ProjectLockProposal) { lock.Projections[0].Contributor = "" }},
		{name: "notices", mutate: func(lock *capabilitypack.ProjectLockProposal) { lock.NoticesSHA256 = strings.Repeat("0", 64) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			opts, project := installIssue453Project(t)
			lockPath := filepath.Join(project, "packy.lock.json")
			data, err := os.ReadFile(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			var lock capabilitypack.ProjectLockProposal
			if err := json.Unmarshal(data, &lock); err != nil {
				t.Fatal(err)
			}
			test.mutate(&lock)
			changed, err := json.MarshalIndent(lock, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(lockPath, append(changed, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
			before := snapshotTree(t, project)
			_, err = executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--require", "installed", "--json")
			if err == nil {
				t.Fatalf("invalid %s evidence passed installed enforcement", test.name)
			}
			if snapshotTree(t, project) != before {
				t.Fatalf("invalid %s status mutated project", test.name)
			}
		})
	}
}
