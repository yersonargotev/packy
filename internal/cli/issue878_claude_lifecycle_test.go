package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogstore"
)

func TestIssue878ClaudeRequiredRoleLifecycleAtBothScopes(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			pack := testsupport.CapabilityRich("reviewed-claude").WithSurfaces(testsupport.SurfaceClaude)
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
			issue878PrepareCatalog(t, fixture.catalogRoot, pack.ID(), []string{"global", "project"}, "initial")
			source := &catalogSourceFixture{release: issue878Release(t, fixture.catalogRoot, pack, strings.Repeat("a", 40))}
			fixture.options.catalogRootOverride = ""
			fixture.options.CatalogSource = source
			project, _ := fixture.options.Getwd()
			root, install, remove := fixture.home, "activate", "deactivate"
			if scope == "project" {
				root, install, remove = project, "install", "uninstall"
			}
			instruction := filepath.Join(root, "CLAUDE.md")
			if scope == "global" {
				instruction = filepath.Join(root, ".claude", "CLAUDE.md")
			}
			foreign := map[string]string{
				instruction: "User instructions.\n",
				filepath.Join(root, ".claude", "settings.json"): "{\"hooks\":{\"SessionStart\":[{\"command\":\"user-hook\"}]},\"permissions\":{\"allow\":[\"Read\"]}}\n",
				filepath.Join(root, ".claude.json"):             "{\"mcpServers\":{\"personal\":{\"command\":\"user-server\"}}}\n",
				filepath.Join(root, ".mcp.json"):                "{\"mcpServers\":{\"personal\":{\"command\":\"user-server\"}}}\n",
			}
			for path, body := range foreign {
				writeAuthoringFile(t, path, body)
			}
			run := func(args ...string) string {
				t.Helper()
				out, err := executeCommand(t, NewRootCommand(fixture.options), args...)
				if err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
				return out
			}
			run("init")
			selected := []string{install, pack.ID(), "--surface", "claude", "--resource", "skill:workflow", "--resource", "instruction:guidance"}
			preview := run(append(append([]string{}, selected...), "--dry-run", "--json")...)
			if !strings.Contains(preview, "reviewer") || !strings.Contains(preview, "claude") || !strings.Contains(preview, scope) {
				t.Fatalf("required role selection explanation missing: %s", preview)
			}
			run(selected...)
			skill := filepath.Join(root, ".claude", "skills", "helper")
			workflow := filepath.Join(root, ".claude", "skills", "workflow")
			role := filepath.Join(root, ".claude", "agents", "reviewer.md")
			for path, expected := range map[string]string{
				filepath.Join(skill, "SKILL.md"):                                 "reviewed helper workflow",
				filepath.Join(skill, "references", "guide.md"):                   "copied-tree reference",
				filepath.Join(skill, "agents", "openai.yaml"):                    "initial",
				filepath.Join(workflow, "references", pack.ID()+"-reference.md"): "Synthetic reference asset",
				role:        "initial role",
				instruction: "Synthetic capability-rich guidance",
			} {
				issue878RequireContent(t, path, expected)
			}
			if info, err := os.Stat(filepath.Join(skill, "scripts", "run.sh")); err != nil || info.Mode().Perm() != 0755 {
				t.Fatalf("skill executable mode: %v %v", info, err)
			}
			issue878RequireContent(t, role, "helper")
			if scope == "project" {
				issue878RequireContent(t, filepath.Join(root, "PACKY-NOTICES.md"), "Apache License 2.0")
				issue878RequireContent(t, filepath.Join(root, "PACKY-NOTICES.md"), "Packy Fixture Authors")
			} else {
				retainedSkill, err := filepath.EvalSymlinks(skill)
				if err != nil {
					t.Fatal(err)
				}
				issue878RequireContent(t, filepath.Join(retainedSkill, "..", "..", "notices", pack.ID()+"-apache"), "Apache License 2.0")
				data, err := os.ReadFile(filepath.Join(root, ".packy", "packs.json"))
				if err != nil {
					t.Fatal(err)
				}
				var state struct {
					Receipts []struct{ Resources []struct{ Kind, ID string } }
				}
				if err := json.Unmarshal(data, &state); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, receipt := range state.Receipts {
					for _, resource := range receipt.Resources {
						found = found || resource.Kind == "notice" && resource.ID == "apache"
					}
				}
				if !found {
					t.Fatal("global receipt omitted required agent legal closure")
				}
			}
			statusArgs := []string{"status", pack.ID(), "--surface", "claude", "--json"}
			if scope == "project" {
				statusArgs = append(statusArgs, "--project")
			}
			status := run(statusArgs...)
			if strings.Contains(status, `"usable":"true"`) {
				t.Fatalf("role file claimed observed runtime usability: %s", status)
			}
			original, err := os.ReadFile(role)
			if err != nil {
				t.Fatal(err)
			}
			writeAuthoringFile(t, role, "User edited role.\n")
			before := snapshotTree(t, root)
			if out, err := executeCommand(t, NewRootCommand(fixture.options), remove, pack.ID(), "--surface", "claude"); err == nil || before != snapshotTree(t, root) {
				t.Fatalf("role drift changed state: %v\n%s", err, out)
			}
			if err := os.WriteFile(role, original, 0644); err != nil {
				t.Fatal(err)
			}
			candidate := t.TempDir()
			if err := pack.Candidate().WriteCatalog(candidate); err != nil {
				t.Fatal(err)
			}
			issue878PrepareCatalog(t, candidate, pack.ID(), []string{"global", "project"}, "updated")
			source.release = issue878Release(t, candidate, pack.Candidate(), strings.Repeat("b", 40))
			run("catalog", "refresh")
			update := []string{"update", pack.ID(), "--surface", "claude"}
			if scope == "project" {
				update = append(update, "--project")
			}
			run(update...)
			issue878RequireContent(t, role, "updated role")
			issue878RequireContent(t, filepath.Join(skill, "agents", "openai.yaml"), "updated")
			run(remove, pack.ID(), "--surface", "claude")
			for _, path := range []string{skill, workflow, role, filepath.Join(root, ".claude", "assets", "reference", "RESOURCE")} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("retired projection remains %s: %v", path, err)
				}
			}
			for path, expected := range foreign {
				got, err := os.ReadFile(path)
				if err != nil || strings.TrimSpace(string(got)) != strings.TrimSpace(expected) {
					t.Fatalf("complete foreign document changed %s: %q %v", path, got, err)
				}
			}
		})
	}
}

func TestIssue878RequiredClaudeRoleRejectsUnsupportedScopeBeforeMutation(t *testing.T) {
	pack := testsupport.CapabilityRich("global-role").WithSurfaces(testsupport.SurfaceClaude)
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
	issue878PrepareCatalog(t, fixture.catalogRoot, pack.ID(), []string{"global"}, "initial")
	project, _ := fixture.options.Getwd()
	homeBefore, projectBefore := snapshotTree(t, fixture.home), snapshotTree(t, project)
	out, err := executeCommand(t, NewRootCommand(fixture.options), "install", pack.ID(), "--surface", "claude", "--resource", "skill:workflow")
	if err == nil || !strings.Contains(out+err.Error(), "scope") {
		t.Fatalf("required unsupported scope accepted: %v %s", err, out)
	}
	if snapshotTree(t, fixture.home) != homeBefore || snapshotTree(t, project) != projectBefore {
		t.Fatal("unsupported required role mutated state")
	}
}

func issue878PrepareCatalog(t *testing.T, root, id string, scopes []string, revision string) {
	t.Helper()
	packRoot := filepath.Join(root, "packs", id)
	path := filepath.Join(packRoot, "pack.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	resources := manifest["resources"].([]any)
	for _, raw := range resources {
		resource := raw.(map[string]any)
		if resource["id"] == "workflow" {
			resource["requires"] = []string{"agent:reviewer", "asset:reference", "skill:helper"}
		}
		if resource["id"] == "reviewer" {
			for _, rawBinding := range resource["bindings"].([]any) {
				rawBinding.(map[string]any)["client_projection"] = map[string]any{"client": "claude", "scopes": scopes}
			}
			resource["origin"].(map[string]any)["relationship"] = "adapted"
			writeAuthoringFile(t, filepath.Join(packRoot, resource["source"].(string)), "---\nname: reviewer\ndescription: Reviewed specialist.\n---\n\nReview the "+revision+" role contract.\n")
		}
		if resource["id"] == "helper" {
			resource["origin"].(map[string]any)["relationship"] = "adapted"
			source := filepath.Join(packRoot, resource["source"].(string))
			writeAuthoringFile(t, filepath.Join(source, "agents", "openai.yaml"), "display_name: "+revision+"\n")
			script := filepath.Join(source, "scripts", "run.sh")
			writeAuthoringFile(t, script, "#!/bin/sh\n# Reviewed inert supporting script.\nexit 0\n")
			if err := os.Chmod(script, 0755); err != nil {
				t.Fatal(err)
			}
		}
	}
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func issue878RequireContent(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), expected) {
		t.Fatalf("%s missing %q: %q %v", path, expected, data, err)
	}
}

func issue878Release(t *testing.T, root string, fixture testsupport.Fixture, commit string) catalogstore.Release {
	t.Helper()
	origins, err := fixture.WriteProject(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	validation, err := cataloglayout.ValidateCatalogProject(context.Background(), root, "", catalogOriginFixture(origins))
	if err != nil {
		t.Fatal(err)
	}
	result, err := cataloglayout.BuildCatalogSnapshot(context.Background(), root, validation, cataloglayout.CatalogSnapshotSource{Repository: "yersonargotev/packy-catalog", Commit: commit, Builder: "yersonargotev/packy@" + strings.Repeat("c", 40)}, filepath.Join(t.TempDir(), "snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	release := catalogstore.Release{Repository: "yersonargotev/packy-catalog", Tag: "catalog-" + commit, Commit: commit, Publisher: "github-actions[bot]", Published: true, Immutable: true, AttestationVerified: true}
	for _, path := range []string{result.ArchivePath, result.ChecksumPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		release.Assets = append(release.Assets, catalogstore.Asset{Name: filepath.Base(path), SHA256: catalogFixtureDigest(data), Data: data})
	}
	return release
}
