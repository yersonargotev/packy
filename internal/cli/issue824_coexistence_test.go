package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue876RetiredDiscoveryRootsDoNotBlockRemainingSurfaces(t *testing.T) {
	for _, surface := range []string{"agents", "claude"} {
		for _, scope := range []string{"global", "project"} {
			t.Run(surface+"-"+scope, func(t *testing.T) {
				pack := testsupport.SkillVariants("coexist", testsupport.SurfaceAgents, testsupport.SurfaceClaude)
				fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
				project := t.TempDir()
				writeTestGitWorktree(t, project)
				opts := fixture.options
				opts.Getwd = func() (string, error) { return project, nil }
				retired := []string{filepath.Join(fixture.home, "xdg/opencode/skills"), filepath.Join(fixture.home, "xdg/opencode/skill"), filepath.Join(project, ".opencode/skills"), filepath.Join(project, ".opencode/skill")}
				before := map[string]string{}
				for _, root := range retired {
					file := filepath.Join(root, "foreign-directory", "SKILL.md")
					issue851WriteFile(t, file, []byte("---\nname: guide\ndescription: retired unmanaged definition\n---\nKeep this content.\n"))
					before[root] = snapshotTree(t, root)
				}
				install, remove := "activate", "deactivate"
				if scope == "project" {
					install, remove = "install", "uninstall"
				}
				for _, command := range []string{install, "status", remove} {
					args := []string{command, pack.ID(), "--surface", surface}
					if command == "status" && scope == "project" {
						args = append(args, "--project")
					}
					if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
						t.Fatalf("%v: %v\n%s", args, err, out)
					}
				}
				for _, root := range retired {
					if snapshotTree(t, root) != before[root] {
						t.Fatalf("lifecycle changed retired unmanaged root %s", root)
					}
				}
			})
		}
	}
}

func TestIssue824SameSurfaceDiscoveryInvalidatesStaleGlobalPlan(t *testing.T) {
	pack := testsupport.SkillVariants("coexist", testsupport.SurfaceAgents)
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	opts := fixture.options.withDefaults()
	opts.Getwd = func() (string, error) { return project, nil }
	facade, err := activationFacade(context.Background(), opts, newWorkstationResolver(opts))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := facade.Preview(context.Background(), capabilitypack.ActivationRequest{PackID: pack.ID(), Surface: capabilitypack.SurfaceAgents})
	if err != nil {
		t.Fatal(err)
	}
	unmanaged := filepath.Join(fixture.home, ".agents/skills/foreign-directory")
	if err := os.MkdirAll(unmanaged, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unmanaged, "SKILL.md"), []byte("---\nname: guide\ndescription: foreign\n---\nUnmanaged definition\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, fixture.home)
	_, err = facade.Apply(context.Background(), capabilitypack.ApplyRequest{Plan: plan, Interactive: true, Approvals: []capabilitypack.ApprovalReceipt{facade.Approve(plan, capabilitypack.ConsentReversibleLocal)}})
	if !errors.Is(err, capabilitypack.ErrStalePlan) {
		t.Fatalf("new discovery did not invalidate plan: %v", err)
	}
	if before != snapshotTree(t, fixture.home) {
		t.Fatal("stale plan mutated user content")
	}
	out, err := executeCommand(t, NewRootCommand(opts), "activate", pack.ID(), "--surface", "agents")
	if err == nil || !strings.Contains(out, "foreign-directory") {
		t.Fatalf("frontmatter name collision was missed: %v\n%s", err, out)
	}
}

func TestIssue824AgentsClaudeVariantsCoexistAcrossUpdateAndRemoval(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			pack := testsupport.SkillVariants("coexist", testsupport.SurfaceAgents, testsupport.SurfaceClaude)
			source := &catalogSourceFixture{release: catalogReleaseFixture(t, strings.Repeat("c", 40), pack)}
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
			opts := fixture.options
			opts.catalogRootOverride = ""
			opts.CatalogSource = source
			project := t.TempDir()
			writeTestGitWorktree(t, project)
			opts.Getwd = func() (string, error) { return project, nil }
			run := func(args ...string) {
				t.Helper()
				if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
			}
			run("init")
			command, remove, root := "activate", "deactivate", fixture.home
			if scope == "project" {
				command, remove, root = "install", "uninstall", project
			}
			run(command, pack.ID(), "--surface", "agents")
			run(command, pack.ID(), "--surface", "claude")
			claude := filepath.Join(root, ".claude/skills/guide")
			before := snapshotTree(t, claude)
			updated := pack.WithVersion("2.0.0").WithVariantBytes(testsupport.SurfaceAgents, "references/detail.md", []byte("reviewed agents changed\n"))
			source.release = catalogReleaseFixture(t, strings.Repeat("d", 40), updated)
			run("catalog", "refresh")
			args := []string{"update", pack.ID(), "--surface", "agents"}
			if scope == "project" {
				args = append(args, "--project")
			}
			run(args...)
			if snapshotTree(t, claude) != before {
				t.Fatal("Agents update changed Claude tree")
			}
			run(remove, pack.ID(), "--surface", "agents")
			if snapshotTree(t, claude) != before {
				t.Fatal("Agents removal changed Claude tree")
			}
			run(remove, pack.ID(), "--surface", "claude")
		})
	}
}

func TestIssue824StatusReportsDiscoveryWithoutClaimingForeignOwnership(t *testing.T) {
	schemaRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			pack := testsupport.SkillVariants("coexist", testsupport.SurfaceAgents)
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
			opts := fixture.options
			project := t.TempDir()
			writeTestGitWorktree(t, project)
			opts.Getwd = func() (string, error) { return project, nil }
			install, remove, root := "activate", "deactivate", fixture.home
			if scope == "project" {
				install, remove, root = "install", "uninstall", project
			}
			if out, err := executeCommand(t, NewRootCommand(opts), install, pack.ID(), "--surface", "agents"); err != nil {
				t.Fatalf("install: %v %s", err, out)
			}
			foreign := filepath.Join(root, ".agents/skills/foreign-directory")
			if err := os.MkdirAll(foreign, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(foreign, "SKILL.md"), []byte("---\nname: guide\ndescription: unmanaged\n---\nforeign body\n"), 0600); err != nil {
				t.Fatal(err)
			}
			before := snapshotTree(t, foreign)
			args := []string{"status", pack.ID(), "--surface", "agents", "--json"}
			if scope == "project" {
				args = append(args, "--project")
			}
			out, err := executeCommand(t, NewRootCommand(opts), args...)
			if err != nil || !strings.Contains(out, "deterministic coexistence is not verified") || !strings.Contains(out, `"usable":"unknown"`) {
				t.Fatalf("status: %v %s", err, out)
			}
			if scope == "project" {
				assertProjectStructuredOutput(t, schemaRoot, "project-status.schema.json", out)
			} else {
				assertStructuredOutput(t, schemaRoot, "pack-status.schema.json", out)
			}
			if out, err := executeCommand(t, NewRootCommand(opts), remove, pack.ID(), "--surface", "agents"); err != nil {
				t.Fatalf("remove: %v %s", err, out)
			}
			if snapshotTree(t, foreign) != before {
				t.Fatal("removal claimed foreign discovery content")
			}
		})
	}
}
