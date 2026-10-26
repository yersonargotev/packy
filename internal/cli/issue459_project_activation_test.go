package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue459ProjectActivationRequiresAnInstalledPack(t *testing.T) {
	pack := testsupport.PortableAllSurfaces("project-activation-absent")
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
	opts, home := fixture.options, fixture.home
	packID := pack.Manifest().ID
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	beforeProject, beforeHome := snapshotTree(t, project), snapshotTree(t, home)
	_, err := executeCommand(t, NewRootCommand(opts), "activate", packID, "--surface", "agents", "--project")
	if err == nil || !strings.Contains(err.Error(), "project installation") {
		t.Fatalf("activation without installation error = %v", err)
	}
	if snapshotTree(t, project) != beforeProject || snapshotTree(t, home) != beforeHome {
		t.Fatal("failed project activation created an implicit installation or personal state")
	}
}

func TestIssue459DeclarativeProjectActivationIsNotRequired(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	pack := testsupport.PortableAllSurfaces("project-declarative")
	fixture := newSyntheticCLIFixture(t, terminal, pack)
	opts, home := fixture.options, fixture.home
	packID := pack.Manifest().ID
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	if out, err := executeCommand(t, NewRootCommand(opts), "install", packID, "--surface", "agents"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	terminal.calls = 0
	out, err := executeCommand(t, NewRootCommand(opts), "activate", packID, "--surface", "agents", "--project")
	if err != nil || !strings.Contains(out, "Runtime activation: not-required") {
		t.Fatalf("declarative activation: %v\n%s", err, out)
	}
	if terminal.calls != 0 {
		t.Fatalf("declarative activation requested %d approvals", terminal.calls)
	}
	entries, readErr := os.ReadDir(filepath.Join(home, ".packy", "projects"))
	if readErr == nil && len(entries) != 0 {
		t.Fatalf("declarative activation persisted personal state: %v", entries)
	}
}

func TestIssue459ProjectUsableEnforcementRequiresPersonalActivation(t *testing.T) {
	pack := testsupport.PortableAllSurfaces("project-usable")
	opts := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack).options
	packID := pack.Manifest().ID
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "install", packID, "--surface", "agents"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", "agents", "--project", "--require", "usable")
	if err == nil || !strings.Contains(out, "configured=true, authorized=unknown, usable=unknown") {
		t.Fatalf("declarative project usability must fail closed without runtime evidence: %v\n%s", err, out)
	}
}
