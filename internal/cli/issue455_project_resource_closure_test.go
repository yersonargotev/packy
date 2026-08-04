package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestIssue455ProjectInstallPersistsExplicitRootsAndAliases(t *testing.T) {
	opts, _, _ := packActivationOptions(t, &fakeTerminal{interactive: true, approve: true})
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }

	out, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex",
		"--resource", "skill:ask-matt", "--alias", "skill:ask-matt=project-matt", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("preview selected project resources: %v\n%s", err, out)
	}
	var report capabilitypack.JSONProjectInstallPreview
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode preview: %v\n%s", err, out)
	}
	wantRoot := capabilitypack.ResourceIdentity{Kind: "skill", ID: "ask-matt"}
	if report.Selection.Mode != capabilitypack.SelectionCustom || len(report.Pack.Selection.Roots) != 1 || report.Pack.Selection.Roots[0] != wantRoot {
		t.Fatalf("selection = %#v; pack = %#v", report.Selection, report.Pack)
	}
	if len(report.Selection.Resources) != 1 || report.Selection.Resources[0].Resource != wantRoot || report.Selection.Resources[0].Role != capabilitypack.ResourceRoleRoot {
		t.Fatalf("resolved closure = %#v", report.Selection.Resources)
	}
	if len(report.Pack.Aliases) != 1 || report.Pack.Aliases[0].Name != "project-matt" {
		t.Fatalf("manifest aliases = %#v", report.Pack.Aliases)
	}
	if len(report.Projections) != 2 {
		t.Fatalf("projections = %#v", report.Projections)
	}
	foundAlias := false
	for _, projection := range report.Projections {
		if projection.Resource == wantRoot && filepath.ToSlash(projection.Target) == ".agents/skills/project-matt" {
			foundAlias = true
		}
	}
	if !foundAlias {
		t.Fatalf("aliased project projection missing: %#v", report.Projections)
	}

	applyOut, err := executeCommand(t, NewRootCommand(opts), "pack", "install", "matty", "--surface", "codex",
		"--resource", "skill:ask-matt", "--alias", "skill:ask-matt=project-matt")
	if err != nil {
		t.Fatalf("install selected project resources: %v\n%s", err, applyOut)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(project, "packy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Packs []capabilitypack.ProjectManifestPack `json:"packs"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil || len(manifest.Packs) != 1 {
		t.Fatalf("manifest: %v %#v", err, manifest)
	}
	if manifest.Packs[0].Selection.Mode != capabilitypack.SelectionCustom || len(manifest.Packs[0].Aliases) != 1 {
		t.Fatalf("persisted project intent = %#v", manifest.Packs[0])
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "project-matt", "SKILL.md")); err != nil {
		t.Fatalf("aliased selected skill was not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "code-review")); !os.IsNotExist(err) {
		t.Fatalf("unselected skill was materialized: %v", err)
	}
}

func TestIssue455ProjectManifestRejectsMovingAndMachineSpecificAuthority(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(string) string
	}{
		{name: "moving version", mutate: func(value string) string {
			return strings.Replace(value, `"version": "4.0.0"`, `"version": "latest"`, 1)
		}},
		{name: "arbitrary source URL", mutate: func(value string) string {
			return strings.Replace(value, `"version": "4.0.0",`, `"version": "4.0.0", "source_url": "https://example.invalid/pack",`, 1)
		}},
		{name: "machine source override", mutate: func(value string) string {
			return strings.Replace(value, `"version": "4.0.0",`, `"version": "4.0.0", "source_override": "/tmp/local-pack",`, 1)
		}},
		{name: "literal secret", mutate: func(value string) string {
			return strings.Replace(value, `"version": "4.0.0",`, `"version": "4.0.0", "token": "literal-secret",`, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			opts, project := installIssue453Project(t)
			manifestPath := filepath.Join(project, "packy.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, []byte(test.mutate(string(data))), 0o644); err != nil {
				t.Fatal(err)
			}
			before := snapshotTree(t, project)
			_, err = executeCommand(t, NewRootCommand(opts), "pack", "status", "matty", "--surface", "codex", "--project")
			if err == nil {
				t.Fatal("invalid project authority was accepted")
			}
			if snapshotTree(t, project) != before {
				t.Fatal("read-only status mutated the rejected project contract")
			}
		})
	}
}
