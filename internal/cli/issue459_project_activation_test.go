package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue459ProjectActivationRequiresAnInstalledPack(t *testing.T) {
	opts, home, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	beforeProject, beforeHome := snapshotTree(t, project), snapshotTree(t, home)
	_, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex", "--project")
	if err == nil || !strings.Contains(err.Error(), "project installation") {
		t.Fatalf("activation without installation error = %v", err)
	}
	if snapshotTree(t, project) != beforeProject || snapshotTree(t, home) != beforeHome {
		t.Fatal("failed project activation created an implicit installation or personal state")
	}
}

func TestIssue459InteractiveInstallCanOfferSeparateActivation(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}

	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	installation.Lock.Sensitive = []capabilitypack.ProjectSensitiveDisclosure{
		{Category: capabilitypack.ProjectActivationMCP, Surface: capabilitypack.SurfaceCodex, Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}, Detail: "mcp_server"},
		{Category: capabilitypack.ProjectActivationTrust, Surface: capabilitypack.SurfaceCodex, Resource: capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}, Detail: "project-trust"},
	}
	lock, err := json.MarshalIndent(installation.Lock, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), append(lock, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	install := capabilitypack.JSONProjectInstallPreview{Pack: installation.Manifest.Packs[0], Surface: capabilitypack.SurfaceCodex}
	facade := capabilitypack.NewFacade(capabilitypack.Catalog{})
	adapter := projectRuntimeAdapter(opts, capabilitypack.SurfaceCodex, home)
	command := &cobra.Command{}
	var output strings.Builder
	command.SetOut(&output)
	command.SetIn(strings.NewReader(""))

	terminal.approve = false
	terminal.calls = 0
	if err := offerProjectActivation(command, opts, facade, install, project, filepath.Join(home, ".packy"), adapter); err != nil {
		t.Fatalf("decline activation offer: %v", err)
	}
	if terminal.calls != 1 || !strings.Contains(output.String(), "activate later") {
		t.Fatalf("declined offer prompts=%d output=%q", terminal.calls, output.String())
	}
	if matches, _ := filepath.Glob(filepath.Join(home, ".packy", "projects", "*", "state.json")); len(matches) != 0 {
		t.Fatalf("declined activation persisted personal state: %v", matches)
	}

	terminal.approve = true
	terminal.calls = 0
	output.Reset()
	if err := offerProjectActivation(command, opts, facade, install, project, filepath.Join(home, ".packy"), adapter); err != nil {
		t.Fatalf("accept activation offer: %v", err)
	}
	if terminal.calls != 3 || !strings.Contains(output.String(), "Verified personal project activation") {
		t.Fatalf("accepted offer prompts=%d output=%q", terminal.calls, output.String())
	}
	trust, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil || !strings.Contains(string(trust), "trust_level = \"trusted\"") || !strings.Contains(string(trust), project) {
		t.Fatalf("personal Codex project trust = %q, %v", trust, err)
	}
	status, err := capabilitypack.InspectProjectStatus(context.Background(), capabilitypack.ProjectStatusRequest{
		ProjectRoot: project, PackID: "matty", Surface: capabilitypack.SurfaceCodex, PackyHome: filepath.Join(home, ".packy"),
		RequireUsable: true,
		Adapters:      map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{capabilitypack.SurfaceCodex: adapter},
	})
	if err != nil || len(status.Packs) != 1 || !status.Packs[0].RequirementSatisfied || !status.Packs[0].Readiness.Authorized || !status.Packs[0].Readiness.Usable {
		t.Fatalf("personally trusted Codex project is not usable: %+v, %v", status, err)
	}
}

func TestIssue459DeclarativeProjectActivationIsNotRequired(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, home, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	terminal.calls = 0
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "activate", "matty", "--surface", "codex", "--project")
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
	opts, _, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex"); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project", "--require", "usable")
	if err != nil {
		t.Fatalf("declarative project should be usable from installation alone: %v\n%s", err, out)
	}
}
