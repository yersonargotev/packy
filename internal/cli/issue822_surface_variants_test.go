package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue822SkillVariantsThroughAcquiredCatalogAndGlobalLifecycle(t *testing.T) {
	pack := testsupport.SkillVariants("variants", testsupport.SurfaceClaude, testsupport.SurfaceCodex, testsupport.SurfaceOpenCode)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, strings.Repeat("a", 40), pack)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
	opts := fixture.options
	opts.catalogRootOverride = ""
	opts.CatalogSource = source
	run := func(args ...string) string {
		t.Helper()
		out, err := executeCommand(t, NewRootCommand(opts), args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	run("init")
	for _, surface := range []string{"codex", "claude", "opencode", "codex"} {
		preview := run("activate", pack.ID(), "--surface", surface, "--dry-run", "--json")
		if !strings.Contains(preview, `"definition":"surface_variant"`) || !strings.Contains(preview, `"relationship":"adapted"`) {
			t.Fatalf("preview does not explain selected variant provenance: %s", preview)
		}
		run("activate", pack.ID(), "--surface", surface)
		relative := ".agents/skills/guide"
		if surface == "claude" {
			relative = ".claude/skills/guide"
		}
		if surface == "opencode" {
			relative = "xdg/opencode/skills/guide"
		}
		target := filepath.Join(fixture.home, relative)
		for _, file := range []string{"SKILL.md", "references/detail.md", "agents/openai.yaml"} {
			data, err := os.ReadFile(filepath.Join(target, file))
			if err != nil || !strings.Contains(string(data), "reviewed "+surface) {
				t.Fatalf("%s %s = %s, %v", surface, file, data, err)
			}
		}
		run("status", pack.ID(), "--surface", surface, "--json")
		if surface == "codex" {
			before := snapshotTree(t, target)
			if out, err := executeCommand(t, NewRootCommand(opts), "activate", pack.ID(), "--surface", "opencode"); err == nil {
				t.Fatalf("unverified divergent discovery accepted: %s", out)
			}
			if snapshotTree(t, target) != before {
				t.Fatal("collision modified installed tree")
			}
		}
		aux := filepath.Join(target, "references/detail.md")
		original, err := os.ReadFile(aux)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(aux, []byte("local auxiliary change\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := executeCommand(t, NewRootCommand(opts), "deactivate", pack.ID(), "--surface", surface); err == nil {
			t.Fatalf("auxiliary drift allowed removal: %s", out)
		}
		if err := os.WriteFile(aux, original, 0644); err != nil {
			t.Fatal(err)
		}
		run("deactivate", pack.ID(), "--surface", surface)
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("skill remains after removal: %v", err)
		}
	}
	run("activate", pack.ID(), "--surface", "claude")
	updated := pack.WithVersion("2.0.0").WithVariantBytes(testsupport.SurfaceClaude, "references/detail.md", []byte("reviewed claude updated\n"))
	source.release = catalogReleaseFixture(t, strings.Repeat("b", 40), updated)
	run("catalog", "refresh")
	run("update", pack.ID(), "--surface", "claude")
	data, err := os.ReadFile(filepath.Join(fixture.home, ".claude/skills/guide/references/detail.md"))
	if err != nil || string(data) != "reviewed claude updated\n" {
		t.Fatalf("updated tree: %s %v", data, err)
	}
	run("deactivate", pack.ID(), "--surface", "claude")
}

func TestIssue822SkillVariantsProjectLifecycleAndCommonInheritance(t *testing.T) {
	pack := testsupport.SkillVariants("variants", testsupport.SurfaceCodex, testsupport.SurfaceOpenCode)
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
	opts := fixture.options
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts.Getwd = func() (string, error) { return project, nil }
	run := func(args ...string) string {
		t.Helper()
		out, err := executeCommand(t, NewRootCommand(opts), args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	for _, surface := range []string{"codex", "claude", "opencode"} {
		run("install", pack.ID(), "--surface", surface, "--dry-run", "--json")
		run("install", pack.ID(), "--surface", surface)
		relative := ".agents/skills/guide"
		body := surface
		if surface == "claude" {
			relative = ".claude/skills/guide"
			body = "common"
		}
		if surface == "opencode" {
			relative = ".opencode/skills/guide"
		}
		target := filepath.Join(project, relative)
		data, err := os.ReadFile(filepath.Join(target, "references/detail.md"))
		if err != nil || !strings.Contains(string(data), "reviewed "+body) {
			t.Fatalf("project %s: %s %v", surface, data, err)
		}
		lock, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
		if err != nil || !strings.Contains(string(lock), `"guide"`) {
			t.Fatalf("logical receipt identity: %s %v", lock, err)
		}
		script := filepath.Join(target, "scripts/run.sh")
		info, err := os.Stat(script)
		if err != nil {
			t.Fatal(err)
		}
		expectedMode := os.FileMode(0755)
		if body == "common" {
			expectedMode = 0644
		}
		if info.Mode().Perm() != expectedMode {
			t.Fatalf("script mode %o, want %o", info.Mode().Perm(), expectedMode)
		}
		run("verify")
		aux := filepath.Join(target, "agents/openai.yaml")
		original, err := os.ReadFile(aux)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(aux, []byte("changed metadata"), 0644); err != nil {
			t.Fatal(err)
		}
		if out, err := executeCommand(t, NewRootCommand(opts), "verify"); err == nil {
			t.Fatalf("verify accepted changed metadata: %s", out)
		}
		if err := os.WriteFile(aux, original, 0644); err != nil {
			t.Fatal(err)
		}
		if surface == "codex" {
			updated := pack.WithVersion("2.0.0").WithVariantBytes(testsupport.SurfaceCodex, "references/detail.md", []byte("reviewed codex updated\n"))
			if err := updated.WriteCatalog(fixture.catalogRoot); err != nil {
				t.Fatal(err)
			}
			run("update", pack.ID(), "--surface", surface, "--project", "--dry-run", "--json")
			run("update", pack.ID(), "--surface", surface, "--project")
			data, err := os.ReadFile(filepath.Join(target, "references/detail.md"))
			if err != nil || string(data) != "reviewed codex updated\n" {
				t.Fatalf("project update %s %v", data, err)
			}
			run("verify")
		}
		run("uninstall", pack.ID(), "--surface", surface)
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("project tree remains: %v", err)
		}
	}
}

func TestIssue822VariantDependencyClosureUsesSelectedSurfaceInGlobalAndProjectCLI(t *testing.T) {
	pack := testsupport.SkillVariantDependencies("dependencies")
	for _, projectScope := range []bool{false, true} {
		fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
		opts := fixture.options
		root := fixture.home
		apply, remove := "activate", "deactivate"
		if projectScope {
			root = t.TempDir()
			writeTestGitWorktree(t, root)
			opts.Getwd = func() (string, error) { return root, nil }
			apply, remove = "install", "uninstall"
		}
		for _, surface := range []string{"codex", "claude"} {
			if out, err := executeCommand(t, NewRootCommand(opts), apply, pack.ID(), "--surface", surface, "--resource", "skill:guide"); err != nil {
				t.Fatalf("%s %s: %v\n%s", apply, surface, err, out)
			}
			hostRoot := ".agents"
			if surface == "claude" {
				hostRoot = ".claude"
			}
			_, err := os.Stat(filepath.Join(root, hostRoot, "skills/helper"))
			if surface == "codex" && err != nil {
				t.Fatalf("effective dependency missing: %v", err)
			}
			if surface == "claude" && !os.IsNotExist(err) {
				t.Fatalf("Codex dependency leaked to Claude: %v", err)
			}
			if out, err := executeCommand(t, NewRootCommand(opts), remove, pack.ID(), "--surface", surface); err != nil {
				t.Fatalf("%s %s: %v\n%s", remove, surface, err, out)
			}
		}
	}
}
