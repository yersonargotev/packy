package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/claudecode"
	"github.com/yersonargotev/packy/internal/testprocess"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue823AllKindsGlobalAndProjectVariantLifecycle(t *testing.T) {
	for _, surface := range []string{"claude", "codex", "opencode"} {
		for _, scope := range []string{"global", "project"} {
			t.Run(surface+"/"+scope, func(t *testing.T) {
				pack := testsupport.AllKindVariants("all-kinds")
				fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
				opts := fixture.options
				source := &catalogSourceFixture{release: catalogReleaseFixture(t, strings.Repeat("a", 40), pack)}
				opts.catalogRootOverride = ""
				opts.CatalogSource = source
				opts.ClaudeLookPath = func(string) (string, error) { return "fixture-claude", nil }
				opts.ClaudeRunner = issue823ClaudeRunner{home: fixture.home}
				root := fixture.home
				apply, remove := "activate", "deactivate"
				selection := []string{}
				if scope == "project" {
					root = t.TempDir()
					writeTestGitWorktree(t, root)
					opts.Getwd = func() (string, error) { return root, nil }
					apply, remove = "install", "uninstall"
					// Codex currently offers project-native skills, instructions and MCP only.
					if surface == "codex" {
						selection = []string{"--resource", "skill:sample-skill", "--resource", "instruction:sample-instruction", "--resource", "mcp_server:sample-mcp-server"}
					}
				}
				run := func(args ...string) string {
					t.Helper()
					out, err := executeCommand(t, NewRootCommand(opts), args...)
					if err != nil {
						t.Fatalf("%v: %v\n%s", args, err, out)
					}
					return out
				}
				run("init")
				args := append([]string{apply, pack.ID(), "--surface", surface}, selection...)
				run(append(args, "--dry-run", "--json")...)
				if scope == "project" {
					run(append(args, "--json")...)
				} else {
					run(args...)
				}
				receiptPath := filepath.Join(fixture.home, ".packy", "packs.json")
				if scope == "project" {
					receiptPath = filepath.Join(root, "packy.lock.json")
					run("verify", "--json")
				}
				receipt, err := os.ReadFile(receiptPath)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(receipt), "sample-skill") || strings.Contains(string(receipt), "sample-skill-"+surface) {
					t.Fatalf("logical receipt identities changed: %s", receipt)
				}
				// Read actual owned projections rather than the retained catalog trees.
				var state struct {
					Receipts []struct {
						Projections []struct {
							Target string `json:"target"`
						}
					} `json:"receipts"`
				}
				if err := json.Unmarshal(receipt, &state); err != nil {
					t.Fatal(err)
				}
				targets := []string{}
				for _, r := range state.Receipts {
					for _, p := range r.Projections {
						path := p.Target
						if !filepath.IsAbs(path) {
							path = filepath.Join(root, path)
						}
						targets = append(targets, path)
					}
				}
				if surface == "claude" && scope == "global" {
					targets = append(targets, filepath.Join(fixture.home, ".claude.json"))
				}
				content := issue823ProjectedContent(t, targets)
				for _, kind := range []string{"skill", "instruction", "mcp_server"} {
					want := "Reviewed " + surface + " " + kind
					if kind == "mcp_server" {
						want = surface + "-argument"
					}
					if !strings.Contains(content, want) {
						t.Fatalf("missing %s in projected files:\n%s\nTargets %v", want, content, targets)
					}
				}
				if scope == "global" || surface != "codex" {
					for _, kind := range []string{"agent", "command", "asset"} {
						if !strings.Contains(content, "Reviewed "+surface+" "+kind) {
							t.Fatalf("missing %s variant in projections:\n%s", kind, content)
						}
					}
				}
				if scope == "project" {
					if !strings.Contains(content, "Reviewed "+surface+" notice") || !strings.Contains(content, "Original MIT notice and attribution.") {
						t.Fatalf("selected notice missing: %s", content)
					}
				}
				if surface == "claude" && !strings.Contains(content, "reviewed-hook") {
					t.Fatalf("typed lifecycle hook missing: %s", content)
				}
				status := []string{"status", pack.ID(), "--surface", surface, "--json"}
				if scope == "project" {
					status = append(status, "--project")
				}
				run(status...)
				updated := pack.WithAllKindVariantRevision(testsupport.Surface(surface))
				source.release = catalogReleaseFixture(t, strings.Repeat("b", 40), updated)
				run("catalog", "refresh")
				update := []string{"update", pack.ID(), "--surface", surface}
				if scope == "project" {
					update = append(update, "--project")
				}
				run(update...)
				updatedContent := issue823ProjectedContent(t, targets)
				if !strings.Contains(updatedContent, "Updated reviewed body") || !strings.Contains(updatedContent, "updated") {
					t.Fatalf("update omitted selected definitions: %s", updatedContent)
				}
				if scope == "project" {
					run("verify", "--json")
				}
				// Drift in an owned projection must block retirement for this host.
				driftChecked := false
				for _, target := range targets {
					info, err := os.Stat(target)
					if err != nil || !info.Mode().IsRegular() {
						continue
					}
					original, err := os.ReadFile(target)
					if err != nil {
						t.Fatal(err)
					}
					changed := []byte(strings.Replace(string(original), "Reviewed "+surface, "Unreviewed "+surface, 1))
					if string(changed) == string(original) {
						continue
					}
					if err := os.WriteFile(target, changed, info.Mode().Perm()); err != nil {
						t.Fatal(err)
					}
					if out, err := executeCommand(t, NewRootCommand(opts), remove, pack.ID(), "--surface", surface); err == nil {
						t.Fatalf("drifted projection retired: %s", out)
					}
					if err := os.WriteFile(target, original, info.Mode().Perm()); err != nil {
						t.Fatal(err)
					}
					driftChecked = true
					break
				}
				if !driftChecked {
					t.Fatal("fixture has no owned file contribution for drift check")
				}
				run(remove, pack.ID(), "--surface", surface)
			})
		}
	}
}

func issue823ProjectedContent(t *testing.T, targets []string) string {
	t.Helper()
	var result strings.Builder
	for _, target := range targets {
		info, err := os.Stat(target)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			result.Write(data)
			continue
		}
		resolved, resolveErr := filepath.EvalSymlinks(target)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		err = filepath.WalkDir(resolved, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				result.Write(data)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return result.String()
}

func TestIssue823VariantRefreshPreservesMaintainedBodies(t *testing.T) {
	for _, approve := range []bool{false, true} {
		t.Run(map[bool]string{false: "decline", true: "reconcile"}[approve], func(t *testing.T) {
			fixture := writeUpstreamRefreshFixture(t, "exact-copy")
			manifest := readUpstreamRefreshManifest(t, fixture.project)
			source := "skills/codex"
			manifest.Resources[1].Variants = capabilitypack.ResourceVariants{{Surface: capabilitypack.SurfaceCodex, Source: &source, Origin: &capabilitypack.ResourceOrigin{ID: "upstream", Path: "skills/seed", Relationship: "adapted"}}}
			writeAuthoringFile(t, filepath.Join(fixture.project, "packs/seed/skills/codex/SKILL.md"), "Maintained Codex adaptation\n")
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			writeAuthoringFile(t, filepath.Join(fixture.project, "packs/seed/pack.json"), string(data))
			before := snapshotTree(t, filepath.Join(fixture.project, "packs"))
			out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: fixture.resolver, Terminal: &fakeTerminal{interactive: true, approve: approve}}), "catalog", "upstream-refresh", "seed", "--project", fixture.project, "--origin-id", "upstream", "--commit", fixture.newCommit, "--version", "1.1.0")
			if !approve {
				if err == nil || snapshotTree(t, filepath.Join(fixture.project, "packs")) != before {
					t.Fatalf("declined reconciliation mutated pack: %v %s", err, out)
				}
				return
			}
			if err != nil {
				t.Fatalf("refresh: %v\n%s", err, out)
			}
			if !strings.Contains(out, "skill:seed (codex variant)") || !strings.Contains(out, "-# Old upstream") || !strings.Contains(out, "+# New upstream") {
				t.Fatalf("variant-specific upstream review missing: %s", out)
			}
			body, err := os.ReadFile(filepath.Join(fixture.project, "packs/seed/skills/codex/SKILL.md"))
			if err != nil || string(body) != "Maintained Codex adaptation\n" {
				t.Fatalf("adaptation overwritten: %s %v", body, err)
			}
			original, err := os.ReadFile(filepath.Join(fixture.project, "packs/seed/skills/seed/SKILL.md"))
			if err != nil || string(original) != "# New upstream\n" {
				t.Fatalf("original not refreshed: %s %v", original, err)
			}
		})
	}
}

func TestIssue823VariantImportKeepsOriginalAndLogicalIdentity(t *testing.T) {
	fixture := writeUpstreamRefreshFixture(t, "exact-copy")
	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: fixture.resolver}), "catalog", "import", "seed", "--project", fixture.project, "--version", "1.1.0", "--repository", "example/upstream", "--commit", fixture.oldCommit, "--origin-id", "upstream", "--origin-path", "skills/seed", "--destination", "skills/codex", "--relationship", "adapted", "--kind", "skill", "--resource-id", "seed", "--description", "Reviewed Codex adaptation", "--notice", "notice:seed", "--variant-surface", "codex")
	if err != nil {
		t.Fatalf("variant import: %v\n%s", err, out)
	}
	manifest := readUpstreamRefreshManifest(t, fixture.project)
	if len(manifest.Resources) != 2 || len(manifest.Resources[1].Variants) != 1 || manifest.Resources[1].ID != "seed" {
		t.Fatalf("import fragmented identity: %#v", manifest.Resources)
	}
	for _, source := range []string{"seed", "codex"} {
		body, err := os.ReadFile(filepath.Join(fixture.project, "packs/seed/skills", source, "SKILL.md"))
		if err != nil || string(body) != "# Old upstream\n" {
			t.Fatalf("import did not preserve original: %s %v", body, err)
		}
	}
}

// The host transport is simulated; projection ownership and static verification
// still use the production Claude adapter and actual sandbox files.
type issue823ClaudeRunner struct{ home string }

func (r issue823ClaudeRunner) Run(_ context.Context, command claudecode.Command) claudecode.Result {
	if len(command.Args) == 1 && command.Args[0] == "--version" {
		return claudecode.Result{Stdout: "2.1.0"}
	}
	if len(command.Args) < 3 || command.Args[0] != "mcp" {
		return claudecode.Result{}
	}
	servers := map[string]any{}
	if command.Args[1] == "add" {
		for i, arg := range command.Args {
			if arg == "--" && i+1 < len(command.Args) {
				servers[command.Args[2]] = map[string]any{"type": "stdio", "command": command.Args[i+1], "args": command.Args[i+2:]}
				break
			}
		}
	}
	data, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err == nil {
		err = os.WriteFile(filepath.Join(r.home, ".claude.json"), data, 0600)
	}
	return claudecode.Result{Err: err}
}

func TestIssue823BuiltCLIConsumesAllKindVariantSnapshot(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "packy")
	build := exec.Command("go", "build", "-o", binary, "./cmd/packy")
	build.Dir = filepath.Join("..", "..")
	build.Env = testprocess.GoOfflineEnv(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	for _, surface := range []string{"claude", "codex", "opencode"} {
		t.Run(surface, func(t *testing.T) {
			environment := testprocess.Env(t)
			env := adoptionEnvironmentMap(environment)
			project := t.TempDir()
			writeTestGitWorktree(t, project)
			pack := testsupport.AllKindVariants("binary-variants")
			opts := Options{Env: env, Runner: &fakeRunner{}, Terminal: &fakeTerminal{interactive: true, approve: true}, Getwd: func() (string, error) { return project, nil }, CatalogSource: &catalogSourceFixture{release: catalogReleaseFixture(t, strings.Repeat("c", 40), pack)}}
			for _, args := range [][]string{{"init"}, {"install", pack.ID(), "--surface", surface, "--json"}} {
				if args[0] == "install" && surface == "codex" {
					args = append(args, "--resource", "skill:sample-skill", "--resource", "instruction:sample-instruction", "--resource", "mcp_server:sample-mcp-server")
				}
				if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
					t.Fatalf("prepare %v: %v\n%s", args, err, out)
				}
			}
			for _, args := range [][]string{{"activate", pack.ID(), "--surface", surface, "--dry-run", "--json"}, {"verify", "--json"}, {"status", pack.ID(), "--surface", surface, "--project", "--json"}} {
				command := exec.Command(binary, args...)
				command.Dir = project
				if args[0] == "activate" {
					// Global selection is independent of the project installation's
					// same-name generated skills and their discovery policy.
					command.Dir = t.TempDir()
					writeTestGitWorktree(t, command.Dir)
				}
				command.Env = append([]string{}, environment...)
				output, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("real CLI %v: %v\n%s", args, err, output)
				}
				if args[0] == "activate" {
					for _, kind := range []string{"agent", "asset", "command", "instruction", "lifecycle", "mcp_server", "notice", "skill"} {
						if !strings.Contains(string(output), `"kind":"`+kind+`"`) {
							t.Fatalf("built CLI omitted %s: %s", kind, output)
						}
					}
				}
			}
		})
	}
}

func TestIssue823RefreshFindsVariantOnlyProvenance(t *testing.T) {
	fixture := writeUpstreamRefreshFixture(t, "exact-copy")
	manifest := readUpstreamRefreshManifest(t, fixture.project)
	manifest.Resources[1].Origin = nil
	source := "skills/codex"
	manifest.Resources[1].Variants = capabilitypack.ResourceVariants{{Surface: capabilitypack.SurfaceCodex, Source: &source, Origin: &capabilitypack.ResourceOrigin{ID: "upstream", Path: "skills/seed", Relationship: "exact-copy"}}}
	writeAuthoringFile(t, filepath.Join(fixture.project, "packs/seed/skills/codex/SKILL.md"), "# Old upstream\n")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeAuthoringFile(t, filepath.Join(fixture.project, "packs/seed/pack.json"), string(data))
	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: fixture.resolver}), "catalog", "upstream-refresh", "seed", "--project", fixture.project, "--origin-id", "upstream", "--commit", fixture.newCommit, "--version", "1.1.0")
	if err != nil {
		t.Fatalf("variant-only exact refresh: %v\n%s", err, out)
	}
	for source, want := range map[string]string{"seed": "# Old upstream\n", "codex": "# New upstream\n"} {
		data, err := os.ReadFile(filepath.Join(fixture.project, "packs/seed/skills", source, "SKILL.md"))
		if err != nil || string(data) != want {
			t.Fatalf("%s body = %s %v", source, data, err)
		}
	}
}

func TestIssue823VariantMCPUpdatePreservesUnmanagedAndDriftedEntries(t *testing.T) {
	for _, surface := range []string{"opencode", "claude"} {
		for _, owned := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/owned=%t", surface, owned), func(t *testing.T) {
				pack := testsupport.AllKindVariants("guarded")
				fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
				opts := fixture.options
				root := t.TempDir()
				writeTestGitWorktree(t, root)
				opts.Getwd = func() (string, error) { return root, nil }
				path := filepath.Join(root, "opencode.json")
				if surface == "claude" {
					path = filepath.Join(root, ".mcp.json")
				}
				if owned {
					if out, err := executeCommand(t, NewRootCommand(opts), "install", pack.ID(), "--surface", surface, "--resource", "mcp_server:sample-mcp-server", "--json"); err != nil {
						t.Fatalf("install: %v\n%s", err, out)
					}
				}
				content := `{"mcp":{"sample-mcp-server":{"type":"local","command":["unmanaged"],"enabled":true}}}`
				if surface == "claude" {
					content = `{"mcpServers":{"sample-mcp-server":{"type":"stdio","command":"unmanaged","args":[]}}}`
				}
				writeAuthoringFile(t, path, content)
				before := snapshotTree(t, root)
				if out, err := executeCommand(t, NewRootCommand(opts), "install", pack.ID(), "--surface", surface, "--resource", "mcp_server:sample-mcp-server", "--json"); err == nil {
					t.Fatalf("changed or unowned MCP overwritten: %s", out)
				}
				if snapshotTree(t, root) != before {
					t.Fatal("blocked MCP plan changed project")
				}
			})
		}
	}
}

func TestIssue823AuthoringRoundTripRetainsAllKindsAndVariants(t *testing.T) {
	pack := testsupport.AllKindVariants("round-trip")
	root := t.TempDir()
	if err := pack.WriteCatalog(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	upstream := t.TempDir()
	writeAuthoringFile(t, filepath.Join(upstream, "LICENSE"), "Upstream original legal text\n")
	commit := strings.Repeat("a", 40)
	before, err := os.ReadFile(filepath.Join(root, "packs/round-trip/pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: authoringOriginResolver{"example/upstream@" + commit: upstream}}), "catalog", "import", pack.ID(), "--project", root, "--version", "1.1.0", "--repository", "example/upstream", "--commit", commit, "--origin-id", "upstream", "--origin-path", "LICENSE", "--destination", "notices/imported", "--relationship", "exact-copy", "--kind", "notice", "--resource-id", "imported", "--description", "Imported legal text", "--license", "MIT", "--attribution", "Original Authors")
	if err != nil {
		t.Fatalf("import into all-kind Pack: %v\n%s", err, out)
	}
	after, err := os.ReadFile(filepath.Join(root, "packs/round-trip/pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oldManifest, newManifest cataloglayout.Manifest
	if err := json.Unmarshal(before, &oldManifest); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &newManifest); err != nil {
		t.Fatal(err)
	}
	byID := map[string]cataloglayout.Resource{}
	for _, r := range newManifest.Resources {
		byID[r.Kind+":"+r.ID] = r
	}
	for _, r := range oldManifest.Resources {
		got := byID[r.Kind+":"+r.ID]
		if !reflect.DeepEqual(r, got) {
			t.Fatalf("authoring lost common/variant typed fields for %s:%s\nbefore=%#v\nafter=%#v", r.Kind, r.ID, r, got)
		}
	}
}

func TestIssue823RefreshBlocksOverlappingMaintainedVariant(t *testing.T) {
	fixture := writeUpstreamRefreshFixture(t, "exact-copy")
	manifest := readUpstreamRefreshManifest(t, fixture.project)
	source := "skills/seed/maintained"
	manifest.Resources[1].Variants = capabilitypack.ResourceVariants{{Surface: capabilitypack.SurfaceCodex, Source: &source, Origin: &capabilitypack.ResourceOrigin{ID: "upstream", Path: "skills/seed", Relationship: "adapted"}}}
	writeAuthoringFile(t, filepath.Join(fixture.project, "packs/seed", source, "SKILL.md"), "Maintained variant\n")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeAuthoringFile(t, filepath.Join(fixture.project, "packs/seed/pack.json"), string(data))
	before := snapshotTree(t, filepath.Join(fixture.project, "packs"))
	out, err := executeCommand(t, NewRootCommand(Options{CatalogOriginResolver: fixture.resolver, Terminal: &fakeTerminal{interactive: true, approve: true}}), "catalog", "upstream-refresh", "seed", "--project", fixture.project, "--origin-id", "upstream", "--commit", fixture.newCommit, "--version", "1.1.0")
	if err == nil || !strings.Contains(err.Error(), "overlaps maintained source") {
		t.Fatalf("overlapping refresh accepted: %v %s", err, out)
	}
	if snapshotTree(t, filepath.Join(fixture.project, "packs")) != before {
		t.Fatal("overlapping refresh modified maintained tree")
	}
}

func TestIssue823LifecycleVariantResolvesDependencyWithoutExecutingAuthoringCode(t *testing.T) {
	pack := testsupport.AllKindVariants("lifecycle-dependency")
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
	for _, surface := range []string{"claude", "codex", "opencode"} {
		out, err := executeCommand(t, NewRootCommand(fixture.options), "activate", pack.ID(), "--surface", surface, "--resource", "lifecycle:sample-lifecycle", "--dry-run", "--json")
		if err != nil {
			t.Fatalf("lifecycle %s: %v\n%s", surface, err, out)
		}
		for _, want := range []string{`"kind":"skill","id":"sample-skill"`, `"role":"dependency"`, `"definition":"surface_variant"`} {
			if !strings.Contains(out, want) {
				t.Fatalf("lifecycle variant closure omitted %s: %s", want, out)
			}
		}
	}
	if len(fixture.options.Runner.(*fakeRunner).calls) != 0 {
		t.Fatal("preview executed a child process")
	}
}
