package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/workstation"
)

func TestIssue458ProjectUpdateTargetsOneExactVersionAcrossEveryInstalledSurface(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	for _, surface := range []string{"claude", "opencode"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", surface); err != nil {
			t.Fatalf("install Matty for %s: %v\n%s", surface, err, out)
		}
	}

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--project", "--version", "3.0.0", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("preview exact project update: %v\n%s", err, out)
	}
	var preview capabilitypack.JSONProjectInstallPreview
	if err := json.Unmarshal([]byte(out), &preview); err != nil {
		t.Fatalf("decode project update preview: %v\n%s", err, out)
	}
	if preview.Pack.ID != "matty" || preview.Pack.Version != "3.0.0" || preview.Surface != "" || preview.Disposition != capabilitypack.ProjectInstallPreviewable {
		t.Fatalf("project update preview = %#v", preview)
	}
	if got := preview.Pack.Surfaces; len(got) != 2 || got[0] != capabilitypack.SurfaceClaude || got[1] != capabilitypack.SurfaceOpenCode {
		t.Fatalf("affected surfaces = %v, want [claude opencode]", got)
	}
	out, err = executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--project", "--version", "3.0.0")
	if err != nil {
		t.Fatalf("apply exact project update: %v\n%s", err, out)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if installation.Manifest.Packs[0].Version != "3.0.0" || installation.Lock.Source.PackVersion != "3.0.0" || installation.Lock.Packs[0].Version != "3.0.0" {
		t.Fatalf("updated exact identities = %#v", installation)
	}
	if got, want := installation.Manifest.Packs[0].SurfaceIntents, preview.Pack.SurfaceIntents; !reflect.DeepEqual(got, want) {
		t.Fatalf("surface intents changed during update: got %#v want %#v", got, want)
	}
	for _, relative := range []string{".claude/skills/ask-matt/SKILL.md", ".agents/skills/ask-matt/SKILL.md", "CLAUDE.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(project, relative)); err != nil {
			t.Fatalf("host-specific updated projection %s is missing: %v", relative, err)
		}
	}

	out, err = executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--project", "--version", "4.0.0")
	if err != nil {
		t.Fatalf("apply exact catalog-current project update: %v\n%s", err, out)
	}
	installation, err = capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	if installation.Manifest.Packs[0].Version != "4.0.0" || len(installation.Lock.ResourceGraph.Resources) != 23 {
		t.Fatalf("catalog-current project closure = %#v", installation)
	}

	out, err = executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--project", "--version", "3.0.0", "--surface", "opencode", "--dry-run")
	if err == nil || !strings.Contains(out+err.Error(), "--surface is not accepted for project update") {
		t.Fatalf("project update accepted a surface selector: %v\n%s", err, out)
	}
}

func TestIssue458ProjectUpdateDoesNotRewriteAnExactSharedProjection(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	for _, surface := range []string{"codex", "opencode"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", surface); err != nil {
			t.Fatalf("install Matty for %s: %v\n%s", surface, err, out)
		}
	}
	shared := filepath.Join(project, ".agents", "skills", "ask-matt", "SKILL.md")
	before, err := os.Stat(shared)
	if err != nil {
		t.Fatal(err)
	}
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "matty", "--project", "--version", "4.0.0")
	if err != nil || !strings.Contains(out, "Verified no-op") {
		t.Fatalf("exact shared update: %v\n%s", err, out)
	}
	after, err := os.Stat(shared)
	if err != nil || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("exact shared projection was rewritten: before=%v after=%v err=%v", before.ModTime(), after.ModTime(), err)
	}
	installation, err := capabilitypack.LoadProjectInstallation(project)
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range installation.Lock.Projections {
		if projection.Resource.Kind == "skill" && (!containsString(projection.Contributors, "surface:codex:pack:matty") || !containsString(projection.Contributors, "surface:opencode:pack:matty")) {
			t.Fatalf("shared projection %s lost contributors: %v", projection.Resource, projection.Contributors)
		}
	}
}

func TestIssue458ProjectUpdateRejectsAStaleMultiSurfacePlanBeforeEffects(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	for _, surface := range []string{"claude", "opencode"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", surface); err != nil {
			t.Fatalf("install Matty for %s: %v\n%s", surface, err, out)
		}
	}

	configured := opts.withDefaults()
	resolver := newWorkstationResolver(configured)
	snapshot, err := resolver.Resolve(workstation.Options{})
	if err != nil {
		t.Fatal(err)
	}
	composition, err := resolvePackComposition(configured, resolver)
	if err != nil {
		t.Fatal(err)
	}
	facade := capabilitypack.NewFacade(composition.catalog)
	adapter := projectInstallAdapter("", composition.bundleRoot, composition.skills.Root(), composition.codex.PromptFile(), composition.codex.ConfigFile(), composition.openCode.ConfigFile(), composition.openCode.PromptFile())
	preview, err := facade.PreviewProjectUpdate(context.Background(), capabilitypack.ProjectUpdateRequest{PackID: "matty", Version: "3.0.0", ProjectRoot: project}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(project, ".agents", "skills", "ask-matt", "SKILL.md")
	if err := os.WriteFile(skill, []byte("changed after preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = facade.ApplyProjectInstall(context.Background(), capabilitypack.ProjectInstallApplyRequest{Preview: preview, PackyHome: snapshot.PackyHome(), Adapter: adapter})
	if !errors.Is(err, capabilitypack.ErrStalePlan) {
		t.Fatalf("stale project update error = %v", err)
	}
	installation, loadErr := capabilitypack.LoadProjectInstallation(project)
	if loadErr != nil || installation.Manifest.Packs[0].Version != "4.0.0" {
		t.Fatalf("stale plan changed durable intent: version=%s err=%v", installation.Manifest.Packs[0].Version, loadErr)
	}
}

func TestIssue458ProjectUpdateBlocksDriftAndUnavailableExactVersionsWithoutMutation(t *testing.T) {
	terminal := &fakeTerminal{interactive: true, approve: true}
	opts, _, _ := packActivationOptions(t, terminal)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	if out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "addy", "--surface", "opencode"); err != nil {
		t.Fatalf("install Addy for OpenCode: %v\n%s", err, out)
	}

	before := snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(opts), "pack", "update", "addy", "--project", "--version", "9.9.9", "--dry-run")
	if err == nil || !strings.Contains(out+err.Error(), "unavailable exact version") || snapshotTree(t, project) != before {
		t.Fatalf("unavailable exact update was not safely rejected: %v\n%s", err, out)
	}

	skill := filepath.Join(project, ".agents", "skills", "using-agent-skills", "SKILL.md")
	if err := os.WriteFile(skill, []byte("manual edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before = snapshotTree(t, project)
	out, err = executeCommand(t, NewRootCommand(opts), "pack", "update", "addy", "--project", "--version", "1.0.0", "--dry-run")
	if err == nil || !strings.Contains(out+err.Error(), "owned_drift") || snapshotTree(t, project) != before {
		t.Fatalf("drifted update was not safely blocked: %v\n%s", err, out)
	}
}
