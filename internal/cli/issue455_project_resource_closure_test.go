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

func TestIssue455ProjectInstallPersistsExplicitRootsAndAliases(t *testing.T) {
	pack := testsupport.CapabilityRich("project-closure")
	opts := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack).options
	packID := pack.Manifest().ID
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	out, err := executeCommand(t, NewRootCommand(opts), "install", packID, "--surface", "codex",
		"--resource", "skill:helper", "--alias", "skill:helper=project-helper", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("preview selected project resources: %v\n%s", err, out)
	}
	var report capabilitypack.JSONProjectInstallPreview
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode preview: %v\n%s", err, out)
	}
	wantRoot := capabilitypack.ResourceIdentity{Kind: "skill", ID: "helper"}
	if report.Selection.Mode != capabilitypack.SelectionCustom || len(report.Pack.Selection.Roots) != 1 || report.Pack.Selection.Roots[0] != wantRoot {
		t.Fatalf("selection = %#v; pack = %#v", report.Selection, report.Pack)
	}
	wantClosure := map[capabilitypack.ResourceIdentity]capabilitypack.ResourceRole{
		wantRoot:                    capabilitypack.ResourceRoleRoot,
		{Kind: "notice", ID: "mit"}: capabilitypack.ResourceRoleNotice,
	}
	if len(report.Selection.Resources) != len(wantClosure) {
		t.Fatalf("resolved closure = %#v", report.Selection.Resources)
	}
	for _, resource := range report.Selection.Resources {
		if wantRole, ok := wantClosure[resource.Resource]; !ok || resource.Role != wantRole {
			t.Fatalf("unexpected closure member = %#v; want %#v", resource, wantClosure)
		}
	}
	if len(report.Pack.Aliases) != 1 || report.Pack.Aliases[0].Name != "project-helper" {
		t.Fatalf("manifest aliases = %#v", report.Pack.Aliases)
	}
	if len(report.Projections) == 0 {
		t.Fatalf("projections = %#v", report.Projections)
	}
	foundAlias := false
	for _, projection := range report.Projections {
		if projection.Resource == wantRoot && filepath.ToSlash(projection.Target) == ".agents/skills/project-helper" {
			foundAlias = true
		}
	}
	if !foundAlias {
		t.Fatalf("aliased project projection missing: %#v", report.Projections)
	}

	applyOut, err := executeCommand(t, NewRootCommand(opts), "install", packID, "--surface", "codex",
		"--resource", "skill:helper", "--alias", "skill:helper=project-helper")
	if err != nil {
		t.Fatalf("install selected project resources: %v\n%s", err, applyOut)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(project, "packy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Packs []struct {
			SurfaceIntents []capabilitypack.ProjectSurfaceIntent `json:"surface_intents"`
		} `json:"packs"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil || len(manifest.Packs) != 1 {
		t.Fatalf("manifest: %v %#v", err, manifest)
	}
	if len(manifest.Packs[0].SurfaceIntents) != 1 || manifest.Packs[0].SurfaceIntents[0].Selection.Mode != capabilitypack.SelectionCustom || len(manifest.Packs[0].SurfaceIntents[0].Aliases) != 1 {
		t.Fatalf("persisted project intent = %#v", manifest.Packs[0])
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "project-helper", "SKILL.md")); err != nil {
		t.Fatalf("aliased selected skill was not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "workflow")); !os.IsNotExist(err) {
		t.Fatalf("unselected workflow skill was materialized: %v", err)
	}
}

func TestIssue455ProjectManifestRejectsMovingAndMachineSpecificAuthority(t *testing.T) {
	version := testsupport.PortableAllSurfaces("project-portable").CurrentVersion()
	versionField := `"version": "` + version + `"`
	for _, test := range []struct {
		name   string
		mutate func(string) string
	}{
		{name: "moving version", mutate: func(value string) string {
			return strings.Replace(value, versionField, `"version": "latest"`, 1)
		}},
		{name: "arbitrary source URL", mutate: func(value string) string {
			return strings.Replace(value, versionField+`,`, versionField+`, "source_url": "https://example.invalid/pack",`, 1)
		}},
		{name: "machine source override", mutate: func(value string) string {
			return strings.Replace(value, versionField+`,`, versionField+`, "source_override": "/tmp/local-pack",`, 1)
		}},
		{name: "literal secret", mutate: func(value string) string {
			return strings.Replace(value, versionField+`,`, versionField+`, "token": "literal-secret",`, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			installation := installIssue453Project(t)
			opts, project := installation.options, installation.project
			packID := installation.pack.Manifest().ID
			manifestPath := filepath.Join(project, "packy.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, []byte(test.mutate(string(data))), 0o644); err != nil {
				t.Fatal(err)
			}
			before := snapshotTree(t, project)
			_, err = executeCommand(t, NewRootCommand(opts), "status", packID, "--surface", "codex", "--project")
			if err == nil {
				t.Fatal("invalid project authority was accepted")
			}
			if snapshotTree(t, project) != before {
				t.Fatal("read-only status mutated the rejected project contract")
			}
		})
	}
}
