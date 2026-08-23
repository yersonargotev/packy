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

type installedSyntheticProject struct {
	options Options
	project string
	pack    testsupport.Fixture
}

func installIssue453Project(t *testing.T) installedSyntheticProject {
	t.Helper()
	terminal := &fakeTerminal{interactive: true, approve: true}
	pack := testsupport.PortableAllSurfaces("project-portable")
	fixture := newSyntheticCLIFixture(t, terminal, pack)
	opts := fixture.options
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "install", pack.Manifest().ID, "--surface", "codex"); err != nil {
		t.Fatalf("seed project install: %v\n%s", err, out)
	}
	return installedSyntheticProject{options: opts, project: project, pack: pack}
}

func syntheticProjectTarget(t *testing.T, installation installedSyntheticProject, resource capabilitypack.ResourceIdentity) string {
	t.Helper()
	contract, err := capabilitypack.LoadProjectInstallation(installation.project)
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range contract.Lock.Projections {
		if projection.Resource == resource {
			return filepath.Join(installation.project, filepath.FromSlash(projection.Target))
		}
	}
	t.Fatalf("installed synthetic project has no projection for %s", resource)
	return ""
}

func TestIssue453ProjectStatusReportsIndependentAxesOffline(t *testing.T) {
	installation := installIssue453Project(t)
	opts, project := installation.options, installation.project
	packID := installation.pack.Manifest().ID
	opts.Env = MapEnv{
		"HOME": opts.Env.Getenv("HOME"), "XDG_CONFIG_HOME": opts.Env.Getenv("XDG_CONFIG_HOME"),
		"PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(t.TempDir(), "missing-catalog"),
	}

	out, err := executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", "codex", "--project", "--require", "installed", "--json")
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
	if status.Pack.ID != packID || status.Surface != capabilitypack.SurfaceCodex || status.Installation != capabilitypack.ProjectInstallationInstalled || status.Runtime != capabilitypack.ProjectRuntimeNotRequired || !status.RequirementSatisfied {
		t.Fatalf("project status axes = %#v", status)
	}
	contract, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if status.Projections == nil || status.Blockers == nil || len(status.Projections) != len(contract.Lock.Projections) {
		t.Fatalf("project status omitted portable evidence: %#v", status)
	}
	if _, err := os.Stat(filepath.Join(project, "packy.lock.json")); err != nil {
		t.Fatal(err)
	}
}

func TestIssue453ProjectStatusReportsAbsentInstallationSeparately(t *testing.T) {
	terminal := &fakeTerminal{interactive: false}
	pack := testsupport.PortableAllSurfaces("project-absent")
	opts := newSyntheticCLIFixture(t, terminal, pack).options
	packID := pack.Manifest().ID
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	out, err := executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", "codex", "--project", "--json")
	if err != nil {
		t.Fatalf("absent project status: %v\n%s", err, out)
	}
	var report capabilitypack.JSONProjectStatusReport
	if json.Unmarshal([]byte(out), &report) != nil || len(report.Packs) != 1 || report.Packs[0].Installation != capabilitypack.ProjectInstallationAbsent || report.Packs[0].Runtime != capabilitypack.ProjectRuntimePending {
		t.Fatalf("absent project axes = %#v\n%s", report, out)
	}
	_, err = executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", "codex", "--project", "--require", "installed")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("absent installed gate = %v", err)
	}
	_, err = executeCommand(t, NewRootCommand(opts), "status", "--project", "--require", "installed")
	if err == nil || !strings.Contains(err.Error(), "no installed capability packs") {
		t.Fatalf("absent whole-project installed gate = %v", err)
	}
}

func TestIssue453InstalledEnforcementDetectsDriftWithoutMutation(t *testing.T) {
	installation := installIssue453Project(t)
	opts, project := installation.options, installation.project
	packID := installation.pack.Manifest().ID
	drift := syntheticProjectTarget(t, installation, capabilitypack.ResourceIdentity{Kind: "instruction", ID: "guidance"})
	if err := os.WriteFile(drift, []byte("drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", "codex", "--project", "--require", "installed", "--json")
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

func TestIssue453NamedInstallRestoresAuthorizedMissingBytes(t *testing.T) {
	installation := installIssue453Project(t)
	opts := installation.options
	packID := installation.pack.Manifest().ID
	missing := syntheticProjectTarget(t, installation, capabilitypack.ResourceIdentity{Kind: "instruction", ID: "guidance"})
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	terminal := opts.Terminal.(*fakeTerminal)
	terminal.calls = 0
	out, err := executeCommand(t, NewRootCommand(opts), "install", packID, "--surface", "codex")
	if err != nil {
		t.Fatalf("reconcile project: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Verified project installation") || terminal.calls != 1 {
		t.Fatalf("reconcile result approvals=%d\n%s", terminal.calls, out)
	}
	if _, err := os.Stat(missing); err != nil {
		t.Fatalf("missing exact bytes were not restored: %v", err)
	}
}

func TestIssue453NamedInstallNeverInventsMissingBytesFromDigest(t *testing.T) {
	installation := installIssue453Project(t)
	opts, project := installation.options, installation.project
	packID := installation.pack.Manifest().ID
	missing := syntheticProjectTarget(t, installation, capabilitypack.ResourceIdentity{Kind: "instruction", ID: "guidance"})
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	opts.Env = MapEnv{
		"HOME": opts.Env.Getenv("HOME"), "XDG_CONFIG_HOME": opts.Env.Getenv("XDG_CONFIG_HOME"),
		"PATH": "", "PACKY_SKILLS_SOURCE": filepath.Join(t.TempDir(), "missing-catalog"),
	}
	before := snapshotTree(t, project)
	_, err := executeCommand(t, NewRootCommand(opts), "install", packID, "--surface", "codex")
	if err == nil {
		t.Fatal("reconcile invented missing bytes without an exact local source")
	}
	if snapshotTree(t, project) != before {
		t.Fatal("failed offline acquisition mutated the project")
	}
}

func TestIssue453StatusFailsClosedOnUnsupportedContract(t *testing.T) {
	installation := installIssue453Project(t)
	opts, project := installation.options, installation.project
	packID := installation.pack.Manifest().ID
	manifestPath := filepath.Join(project, "packy.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	unsupported := strings.Replace(string(data), `"schema_version": 1`, `"schema_version": 99`, 1)
	if err := os.WriteFile(manifestPath, []byte(unsupported), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, project)
	_, err = executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", "codex", "--project")
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
		{name: "manifest-to-lock identity", mutate: func(lock *capabilitypack.ProjectLockProposal) { lock.Receipts[0].Pack.Version = "0.0.0" }},
		{name: "closure", mutate: func(lock *capabilitypack.ProjectLockProposal) { lock.Receipts[0].Resources = nil }},
		{name: "target", mutate: func(lock *capabilitypack.ProjectLockProposal) { lock.Receipts[0].Projections[0].Target = "../escape" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			installation := installIssue453Project(t)
			opts, project := installation.options, installation.project
			packID := installation.pack.Manifest().ID
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
			_, err = executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", "codex", "--project", "--require", "installed", "--json")
			if err == nil {
				t.Fatalf("invalid %s evidence passed installed enforcement", test.name)
			}
			if snapshotTree(t, project) != before {
				t.Fatalf("invalid %s status mutated project", test.name)
			}
		})
	}
}
