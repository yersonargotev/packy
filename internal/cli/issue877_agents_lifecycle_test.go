package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue877AgentsCompleteSkillLifecycle(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			pack := testsupport.SkillVariants("shared-guide", testsupport.SurfaceAgents)
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
			project := t.TempDir()
			writeTestGitWorktree(t, project)
			opts := fixture.options
			opts.Getwd = func() (string, error) { return project, nil }
			command, remove, target := "activate", "deactivate", filepath.Join(fixture.home, ".agents", "skills", "guide")
			if scope == "project" {
				command, remove, target = "install", "uninstall", filepath.Join(project, ".agents", "skills", "guide")
			}
			run := func(args ...string) string {
				t.Helper()
				out, err := executeCommand(t, NewRootCommand(opts), args...)
				if err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
				return out
			}
			run(command, pack.ID(), "--surface", "agents")
			for _, file := range []string{"SKILL.md", "references/detail.md", "agents/openai.yaml", "scripts/run.sh"} {
				got, err := os.ReadFile(filepath.Join(target, file))
				if err != nil || !strings.Contains(string(got), "reviewed agents") {
					t.Fatalf("complete tree %s: %q %v", file, got, err)
				}
			}
			if info, err := os.Stat(filepath.Join(target, "scripts/run.sh")); err != nil || info.Mode().Perm() != 0755 {
				t.Fatalf("executable mode: %v %v", info, err)
			}
			for _, root := range []string{fixture.home, project} {
				if _, err := os.Stat(filepath.Join(root, ".codex")); !os.IsNotExist(err) {
					t.Fatalf("shared skill wrote Codex state: %v", err)
				}
			}
			metadata := filepath.Join(target, "agents/openai.yaml")
			original, err := os.ReadFile(metadata)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(metadata, []byte("user edit\n"), 0644); err != nil {
				t.Fatal(err)
			}
			before := snapshotTree(t, target)
			out, err := executeCommand(t, NewRootCommand(opts), remove, pack.ID(), "--surface", "agents")
			if err == nil || snapshotTree(t, target) != before {
				t.Fatalf("drift removal: %v %s", err, out)
			}
			if err := os.WriteFile(metadata, original, 0644); err != nil {
				t.Fatal(err)
			}
			candidate := pack.Candidate().WithVariantBytes(testsupport.SurfaceAgents, "agents/openai.yaml", []byte("updated metadata\n"))
			if err := candidate.WriteCatalog(fixture.catalogRoot); err != nil {
				t.Fatal(err)
			}
			update := []string{"update", pack.ID(), "--surface", "agents"}
			if scope == "project" {
				update = append(update, "--project")
			}
			run(update...)
			if got, err := os.ReadFile(metadata); err != nil || string(got) != "updated metadata\n" {
				t.Fatalf("metadata update: %q %v", got, err)
			}
			run(remove, pack.ID(), "--surface", "agents")
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatalf("retired tree remains: %v", err)
			}
		})
	}
}

func TestIssue877RequiredCodexRoleIsExplicitOwnedAndRemovable(t *testing.T) {
	pack := testsupport.CapabilityRich("role-workflow")
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts := fixture.options
	opts.Getwd = func() (string, error) { return project, nil }
	requireIssue877Role(t, fixture.catalogRoot, pack.ID())
	base := []string{"activate", pack.ID(), "--surface", "agents", "--resource", "skill:workflow"}
	for _, args := range [][]string{base, append(append([]string{}, base...), "--client", "other"), {"install", pack.ID(), "--surface", "agents", "--client", "codex", "--resource", "skill:workflow"}, {"install", pack.ID(), "--surface", "claude", "--client", "codex", "--resource", "skill:helper"}} {
		beforeHome, beforeProject := snapshotTree(t, fixture.home), snapshotTree(t, project)
		out, err := executeCommand(t, NewRootCommand(opts), args...)
		if err == nil || snapshotTree(t, fixture.home) != beforeHome || snapshotTree(t, project) != beforeProject {
			t.Fatalf("unsupported client/scope mutated: %v %v %s", args, err, out)
		}
	}
	args := append(append([]string{}, base...), "--client", "codex")
	preview, err := executeCommand(t, NewRootCommand(opts), append(append([]string{}, args...), "--dry-run", "--json")...)
	if err != nil || !strings.Contains(preview, `"client":"codex"`) || !strings.Contains(preview, "global") {
		t.Fatalf("concrete preview: %v %s", err, preview)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
		t.Fatalf("activate: %v %s", err, out)
	}
	role := filepath.Join(fixture.home, ".codex", "agents", "reviewer.toml")
	original, err := os.ReadFile(role)
	if err != nil || !strings.Contains(string(original), "developer_instructions") {
		t.Fatalf("native role: %q %v", original, err)
	}
	if _, err := os.Stat(filepath.Join(fixture.home, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("unrelated runtime configuration: %v", err)
	}
	status, err := executeCommand(t, NewRootCommand(opts), "status", pack.ID(), "--surface", "agents", "--json")
	if err != nil || !strings.Contains(status, `"usable":"unknown"`) {
		t.Fatalf("configured is not delegation proof: %v %s", err, status)
	}
	candidate := pack.Candidate().WithAdaptedBytes("agent:reviewer", ".", []byte("Review the updated contract.\n"))
	candidateRoot := t.TempDir()
	if err := candidate.WriteCatalog(candidateRoot); err != nil {
		t.Fatal(err)
	}
	requireIssue877Role(t, candidateRoot, pack.ID())
	opts.catalogRootOverride = candidateRoot
	if out, err := executeCommand(t, NewRootCommand(opts), "update", pack.ID(), "--surface", "agents"); err != nil {
		t.Fatalf("role update lost client: %v %s", err, out)
	}
	original, err = os.ReadFile(role)
	if err != nil || !strings.Contains(string(original), "updated contract") {
		t.Fatalf("role update: %q %v", original, err)
	}
	collision := testsupport.CapabilityRich("other-role")
	if err := collision.WriteCatalog(candidateRoot); err != nil {
		t.Fatal(err)
	}
	beforeCollision := snapshotTree(t, fixture.home)
	if out, err := executeCommand(t, NewRootCommand(opts), "activate", collision.ID(), "--surface", "agents", "--client", "codex", "--resource", "agent:reviewer"); err == nil || snapshotTree(t, fixture.home) != beforeCollision {
		t.Fatalf("role collision overwrote ownership: %v %s", err, out)
	}
	if err := os.WriteFile(role, []byte("user role\n"), 0644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, fixture.home)
	if out, err := executeCommand(t, NewRootCommand(opts), "deactivate", pack.ID(), "--surface", "agents"); err == nil || snapshotTree(t, fixture.home) != before {
		t.Fatalf("role drift: %v %s", err, out)
	}
	if err := os.WriteFile(role, original, 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := executeCommand(t, NewRootCommand(opts), "deactivate", pack.ID(), "--surface", "agents"); err != nil {
		t.Fatalf("remove: %v %s", err, out)
	}
	for _, path := range []string{role, filepath.Join(fixture.home, ".agents", "skills", "helper"), filepath.Join(fixture.home, ".agents", "skills", "workflow")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("retired role closure remains %s: %v", path, err)
		}
	}
}

func requireIssue877Role(t *testing.T, root, packID string) {
	t.Helper()
	path := filepath.Join(root, "packs", packID, "pack.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, raw := range manifest["resources"].([]any) {
		r := raw.(map[string]any)
		if r["id"] == "workflow" {
			r["requires"] = []string{"agent:reviewer", "asset:reference", "skill:helper"}
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
