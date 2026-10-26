package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogstore"
)

func TestIssue851ReviewedSkillTemplatesCoexistThroughCompleteLifecycle(t *testing.T) {
	for _, surface := range []string{"claude", "codex"} {
		for _, scope := range []string{"global", "project"} {
			for _, reverse := range []bool{false, true} {
				order := "legacy-first"
				if reverse {
					order = "template-first"
				}
				t.Run(surface+"/"+scope+"/"+order, func(t *testing.T) {
					legacy := testsupport.CapabilityRich("legacy-flow")
					templated := testsupport.CapabilityRich("pstack-flow")
					fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, legacy, templated)
					issue851RewriteCatalogPack(t, fixture.catalogRoot, legacy.ID(), "1.0.0", false, "v1")
					issue851RewriteCatalogPack(t, fixture.catalogRoot, templated.ID(), "1.0.0", true, "v1")

					root := fixture.home
					apply, remove := "activate", "deactivate"
					opts := fixture.options
					catalogRoot := fixture.catalogRoot
					var source *catalogSourceFixture
					if scope == "project" {
						root = t.TempDir()
						writeTestGitWorktree(t, root)
						opts.Getwd = func() (string, error) { return root, nil }
						apply, remove = "install", "uninstall"
					} else {
						commit := strings.Repeat("a", 40)
						source = &catalogSourceFixture{release: issue851CatalogRelease(t, commit, "1.0.0", "v1")}
						opts.catalogRootOverride = ""
						opts.CatalogSource = source
						catalogRoot = filepath.Join(catalogstore.DefaultDataRoot(fixture.home), "catalog", "snapshots", commit)
					}
					run := func(args ...string) string {
						t.Helper()
						out, err := executeCommand(t, NewRootCommand(opts), args...)
						if err != nil {
							t.Fatalf("%v: %v\n%s", args, err, out)
						}
						return out
					}
					if scope == "global" {
						run("init")
					}
					initialCatalogRoot := catalogRoot
					legacySource := snapshotTree(t, filepath.Join(catalogRoot, "packs", legacy.ID()))
					templateSource := snapshotTree(t, filepath.Join(catalogRoot, "packs", templated.ID()))

					first, second := legacy.ID(), templated.ID()
					if reverse {
						first, second = second, first
					}
					for _, packID := range []string{first, second} {
						preview := run(apply, packID, "--surface", surface, "--dry-run", "--json")
						if packID == templated.ID() {
							for _, want := range []string{templated.ID() + "-tdd", templated.ID() + "-teach"} {
								if !strings.Contains(preview, want) {
									t.Fatalf("template preview omitted effective name %q: %s", want, preview)
								}
							}
						}
						run(apply, packID, "--surface", surface)
					}
					if scope == "project" {
						run("verify", "--json")
					} else {
						run("status", legacy.ID(), "--surface", surface, "--json")
						run("status", templated.ID(), "--surface", surface, "--json")
					}
					issue851AssertInstalled(t, root, scope, surface, legacy.ID(), templated.ID(), "v1", "")
					beforeConvergedHome := snapshotTree(t, root)
					converged := run(apply, templated.ID(), "--surface", surface, "--dry-run", "--json")
					if !strings.Contains(strings.ToLower(converged), "converged") {
						t.Fatalf("installed template Pack did not produce a converged preview: %s", converged)
					}
					if snapshotTree(t, root) != beforeConvergedHome {
						t.Fatal("converged preview changed installed files")
					}
					if got := snapshotTree(t, filepath.Join(catalogRoot, "packs", legacy.ID())); got != legacySource {
						t.Fatal("installation mutated the legacy Catalog Snapshot source")
					}
					if got := snapshotTree(t, filepath.Join(catalogRoot, "packs", templated.ID())); got != templateSource {
						t.Fatal("rendering mutated the template Catalog Snapshot source")
					}

					if scope == "project" {
						issue851RewriteCatalogPack(t, fixture.catalogRoot, templated.ID(), "1.1.0", true, "v2")
					} else {
						commit := strings.Repeat("b", 40)
						source.release = issue851CatalogRelease(t, commit, "1.1.0", "v2")
						run("catalog", "refresh")
						catalogRoot = filepath.Join(catalogstore.DefaultDataRoot(fixture.home), "catalog", "snapshots", commit)
					}
					updatedSource := snapshotTree(t, filepath.Join(catalogRoot, "packs", templated.ID()))
					update := []string{"update", templated.ID(), "--surface", surface}
					if scope == "project" {
						update = append(update, "--project")
					}
					run(update...)
					if scope == "project" {
						run("verify", "--json")
					}
					issue851AssertInstalled(t, root, scope, surface, legacy.ID(), templated.ID(), "v2", "")
					if got := snapshotTree(t, filepath.Join(catalogRoot, "packs", templated.ID())); got != updatedSource {
						t.Fatal("update mutated the revised Catalog Snapshot source")
					}
					if scope == "global" {
						if got := snapshotTree(t, filepath.Join(initialCatalogRoot, "packs", legacy.ID())); got != legacySource {
							t.Fatal("update mutated the retained legacy Catalog Snapshot source")
						}
						if got := snapshotTree(t, filepath.Join(initialCatalogRoot, "packs", templated.ID())); got != templateSource {
							t.Fatal("update mutated the retained template Catalog Snapshot source")
						}
					}

					templateTDD := issue851SkillPath(root, scope, surface, templated.ID()+"-tdd")
					skillFile := filepath.Join(templateTDD, "SKILL.md")
					original, err := os.ReadFile(skillFile)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(skillFile, append(append([]byte(nil), original...), []byte("operator drift\n")...), 0o644); err != nil {
						t.Fatal(err)
					}
					out, err := executeCommand(t, NewRootCommand(opts), remove, templated.ID(), "--surface", surface)
					if err == nil || !strings.Contains(strings.ToLower(out+err.Error()), "drift") {
						t.Fatalf("drifted rendered skill was removed: %v\n%s", err, out)
					}
					if err := os.WriteFile(skillFile, original, 0o644); err != nil {
						t.Fatal(err)
					}
					run(remove, templated.ID(), "--surface", surface)
					if _, err := os.Stat(templateTDD); !os.IsNotExist(err) {
						t.Fatalf("template-owned skill remains after removal: %v", err)
					}
					legacyTDD := issue851SkillPath(root, scope, surface, "tdd")
					if body := readFileString(t, filepath.Join(legacyTDD, "SKILL.md")); !strings.Contains(body, "legacy-flow tdd v1") {
						t.Fatalf("removing the template Pack changed the legacy definition:\n%s", body)
					}
					run(remove, legacy.ID(), "--surface", surface)
					if got := snapshotTree(t, filepath.Join(catalogRoot, "packs", templated.ID())); got != updatedSource {
						t.Fatal("removal mutated the revised Catalog Snapshot source")
					}
				})
			}
		}
	}
}

func TestIssue851ExplicitAliasesOverrideGeneratedNamesAndReferences(t *testing.T) {
	for _, surface := range []string{"claude", "codex"} {
		t.Run(surface, func(t *testing.T) {
			legacy := testsupport.CapabilityRich("legacy-alias")
			templated := testsupport.CapabilityRich("pstack-alias")
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, legacy, templated)
			issue851RewriteCatalogPack(t, fixture.catalogRoot, legacy.ID(), "1.0.0", false, "v1")
			issue851RewriteCatalogPack(t, fixture.catalogRoot, templated.ID(), "1.0.0", true, "v1")
			project := t.TempDir()
			writeTestGitWorktree(t, project)
			opts := fixture.options
			opts.Getwd = func() (string, error) { return project, nil }
			run := func(args ...string) {
				t.Helper()
				if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
			}
			run("install", legacy.ID(), "--surface", surface)
			run("install", templated.ID(), "--surface", surface,
				"--alias", "skill:tdd=custom-tdd", "--alias", "skill:teach=custom-teach")
			run("verify", "--json")
			issue851AssertInstalled(t, project, "project", surface, legacy.ID(), templated.ID(), "v1", "custom")
			for _, generated := range []string{templated.ID() + "-tdd", templated.ID() + "-teach"} {
				if _, err := os.Stat(issue851SkillPath(project, "project", surface, generated)); !os.IsNotExist(err) {
					t.Fatalf("generated default %q remained beside its explicit alias: %v", generated, err)
				}
			}
			run("uninstall", templated.ID(), "--surface", surface)
			run("uninstall", legacy.ID(), "--surface", surface)
		})
	}
}

func TestIssue851LegacyAliasesStillFailClosedOnDivergentNativeDiscovery(t *testing.T) {
	for _, surface := range []string{"claude", "codex"} {
		t.Run(surface, func(t *testing.T) {
			first := testsupport.CapabilityRich("legacy-one")
			second := testsupport.CapabilityRich("legacy-two")
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, first, second)
			issue851RewriteCatalogPack(t, fixture.catalogRoot, first.ID(), "1.0.0", false, "v1")
			issue851RewriteCatalogPack(t, fixture.catalogRoot, second.ID(), "1.0.0", false, "v1")
			if out, err := executeCommand(t, NewRootCommand(fixture.options), "activate", first.ID(), "--surface", surface); err != nil {
				t.Fatalf("seed legacy Pack: %v\n%s", err, out)
			}
			beforeHome := snapshotTree(t, fixture.home)
			beforeCatalog := snapshotTree(t, fixture.catalogRoot)
			out, err := executeCommand(t, NewRootCommand(fixture.options), "activate", second.ID(), "--surface", surface,
				"--alias", "skill:tdd=other-tdd", "--alias", "skill:teach=other-teach")
			if err == nil || !strings.Contains(out+err.Error(), "deterministic coexistence is not verified") {
				t.Fatalf("legacy alias bypassed divergent native discovery: %v\n%s", err, out)
			}
			if snapshotTree(t, fixture.home) != beforeHome || snapshotTree(t, fixture.catalogRoot) != beforeCatalog {
				t.Fatal("blocked legacy collision mutated HOME or Catalog Snapshot")
			}
		})
	}
}

func TestIssue851ChangedTemplateContractInvalidatesApprovedPreview(t *testing.T) {
	legacy := testsupport.CapabilityRich("legacy-stale")
	templated := testsupport.CapabilityRich("pstack-stale")
	terminal := &fakeTerminal{interactive: true, approve: true}
	fixture := newSyntheticCLIFixture(t, terminal, legacy, templated)
	issue851RewriteCatalogPack(t, fixture.catalogRoot, legacy.ID(), "1.0.0", false, "v1")
	issue851RewriteCatalogPack(t, fixture.catalogRoot, templated.ID(), "1.0.0", true, "v1")
	if out, err := executeCommand(t, NewRootCommand(fixture.options), "activate", legacy.ID(), "--surface", "codex"); err != nil {
		t.Fatalf("seed legacy Pack: %v\n%s", err, out)
	}
	beforeHome := snapshotTree(t, fixture.home)
	beforeSource := snapshotTree(t, filepath.Join(fixture.catalogRoot, "packs", templated.ID()))
	terminal.onApprove = func() {
		issue851ChangeTemplateContract(t, fixture.catalogRoot, templated.ID(), "codex")
	}
	out, err := executeCommand(t, NewRootCommand(fixture.options), "activate", templated.ID(), "--surface", "codex")
	if err == nil || !strings.Contains(strings.ToLower(out+err.Error()), "stale") {
		t.Fatalf("changed source/reference/binding did not stale the approved preview: %v\n%s", err, out)
	}
	if got := snapshotTree(t, fixture.home); got != beforeHome {
		t.Fatal("stale template preview changed the legacy installation or wrote old generated bytes")
	}
	if got := snapshotTree(t, filepath.Join(fixture.catalogRoot, "packs", templated.ID())); got == beforeSource {
		t.Fatal("stale-preview fixture did not change the reviewed template contract")
	}
	legacyBody := readFileString(t, filepath.Join(issue851SkillPath(fixture.home, "global", "codex", "tdd"), "SKILL.md"))
	if !strings.Contains(legacyBody, legacy.ID()+" tdd v1") {
		t.Fatalf("stale preview changed unrelated legacy content:\n%s", legacyBody)
	}
}

func TestIssue851UpdateAdoptsTemplateNamingBeforeInstallingLegacyNeighbor(t *testing.T) {
	for _, surface := range []string{"claude", "codex"} {
		for _, scope := range []string{"global", "project"} {
			t.Run(surface+"/"+scope, func(t *testing.T) {
				migrating := testsupport.CapabilityRich("migrating-flow")
				neighbor := testsupport.CapabilityRich("legacy-neighbor")
				fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, migrating, neighbor)
				issue851RewriteCatalogPack(t, fixture.catalogRoot, migrating.ID(), "1.0.0", false, "v1")
				issue851RewriteCatalogPack(t, fixture.catalogRoot, neighbor.ID(), "1.0.0", false, "v1")
				opts := fixture.options
				root := fixture.home
				apply, remove := "activate", "deactivate"
				var source *catalogSourceFixture
				if scope == "project" {
					root = t.TempDir()
					writeTestGitWorktree(t, root)
					opts.Getwd = func() (string, error) { return root, nil }
					apply, remove = "install", "uninstall"
				} else {
					source = &catalogSourceFixture{release: issue851CatalogReleaseFor(t, strings.Repeat("d", 40),
						issue851CatalogPack{ID: migrating.ID(), Version: "1.0.0", Revision: "v1"},
						issue851CatalogPack{ID: neighbor.ID(), Version: "1.0.0", Revision: "v1"},
					)}
					opts.catalogRootOverride = ""
					opts.CatalogSource = source
				}
				run := func(args ...string) string {
					t.Helper()
					out, err := executeCommand(t, NewRootCommand(opts), args...)
					if err != nil {
						t.Fatalf("%v: %v\n%s", args, err, out)
					}
					return out
				}
				if scope == "global" {
					run("init")
				}
				run(apply, migrating.ID(), "--surface", surface)
				if scope == "project" {
					issue851RewriteCatalogPack(t, fixture.catalogRoot, migrating.ID(), "1.1.0", true, "v2")
				} else {
					source.release = issue851CatalogReleaseFor(t, strings.Repeat("e", 40),
						issue851CatalogPack{ID: migrating.ID(), Version: "1.1.0", Revision: "v2", Templated: true},
						issue851CatalogPack{ID: neighbor.ID(), Version: "1.0.0", Revision: "v1"},
					)
					run("catalog", "refresh")
				}
				update := []string{"update", migrating.ID(), "--surface", surface}
				if scope == "project" {
					update = append(update, "--project")
				}
				updateOutput := run(update...)
				for _, canonical := range []string{"tdd", "teach"} {
					if _, err := os.Stat(issue851SkillPath(root, scope, surface, canonical)); !os.IsNotExist(err) {
						t.Fatalf("update retained obsolete canonical %s path: %v\n%s", canonical, err, updateOutput)
					}
				}
				generatedTDD := migrating.ID() + "-tdd"
				body := readFileString(t, filepath.Join(issue851SkillPath(root, scope, surface, generatedTDD), "SKILL.md"))
				if !strings.Contains(body, "name: "+generatedTDD) || !strings.Contains(body, migrating.ID()+" tdd v2") {
					t.Fatalf("update did not install revised generated definition:\n%s", body)
				}
				run(apply, neighbor.ID(), "--surface", surface)
				neighborBody := readFileString(t, filepath.Join(issue851SkillPath(root, scope, surface, "tdd"), "SKILL.md"))
				if !strings.Contains(neighborBody, neighbor.ID()+" tdd v1") {
					t.Fatalf("legacy neighbor did not claim the retired canonical name:\n%s", neighborBody)
				}
				if scope == "project" {
					run("verify", "--json")
				} else {
					run("status", migrating.ID(), "--surface", surface, "--json")
				}
				run(remove, neighbor.ID(), "--surface", surface)
				run(remove, migrating.ID(), "--surface", surface)
			})
		}
	}
}

func issue851RewriteCatalogPack(t *testing.T, catalogRoot, packID, version string, templated bool, revision string) {
	t.Helper()
	packRoot := filepath.Join(catalogRoot, "packs", packID)
	manifestPath := filepath.Join(packRoot, "pack.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["version"] = version
	var notice any
	for _, value := range manifest["resources"].([]any) {
		resource := value.(map[string]any)
		if resource["kind"] == "notice" && resource["id"] == "mit" {
			notice = resource
			break
		}
	}
	if notice == nil {
		t.Fatalf("synthetic Pack %q has no MIT notice", packID)
	}
	flowName := packID + "-consumer"
	manifest["resources"] = []any{
		notice,
		issue851SkillResource(packID, flowName, flowName, "reviewer.md", []string{"skill:tdd", "skill:teach"}, templated, "native"),
		issue851SkillResource(packID, "tdd", "tdd", "helper", nil, templated, "pack"),
		issue851SkillResource(packID, "teach", "teach", "workflow", []string{"skill:tdd"}, templated, "pack"),
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(packRoot, "skills")); err != nil {
		t.Fatal(err)
	}
	for _, obsolete := range []string{"agents", "assets", "instructions", filepath.Join("notices", packID+"-apache")} {
		if err := os.RemoveAll(filepath.Join(packRoot, obsolete)); err != nil {
			t.Fatal(err)
		}
	}

	tddBody := fmt.Sprintf("---\nname: tdd\ndescription: %s TDD workflow.\n---\n\n%s tdd %s\n", packID, packID, revision)
	teachBody := fmt.Sprintf("---\nname: teach\ndescription: %s teaching workflow.\n---\n\n%s teach %s\n", packID, packID, revision)
	consumerBody := fmt.Sprintf("---\nname: %s\ndescription: %s consumer workflow.\n---\n\n%s consumer %s\n", flowName, packID, packID, revision)
	if templated {
		teachBody += "Delegate with ${{packy.skill:tdd}}.\n"
		consumerBody += strings.Join([]string{
			"Dollar call: ${{packy.skill:tdd}}",
			"Slash call: /{{packy.skill:teach}}",
			"Skill tool name: `{{packy.skill:tdd}}`",
			"Sibling path: ../{{packy.skill:teach}}/SKILL.md",
			"Unmarked shell example stays $tdd",
			"",
		}, "\n")
	} else {
		teachBody += "Delegate with $tdd.\n"
		consumerBody += "Use $tdd and /teach from this legacy Pack.\n"
	}
	issue851WriteFile(t, filepath.Join(packRoot, "skills", "tdd", "SKILL.md"), []byte(tddBody))
	issue851WriteFile(t, filepath.Join(packRoot, "skills", "teach", "SKILL.md"), []byte(teachBody))
	issue851WriteFile(t, filepath.Join(packRoot, "skills", flowName, "SKILL.md"), []byte(consumerBody))
}

func issue851CatalogRelease(t *testing.T, commit, templateVersion, revision string) catalogstore.Release {
	return issue851CatalogReleaseFor(t, commit,
		issue851CatalogPack{ID: "legacy-flow", Version: "1.0.0", Revision: "v1"},
		issue851CatalogPack{ID: "pstack-flow", Version: templateVersion, Revision: revision, Templated: true},
	)
}

type issue851CatalogPack struct {
	ID        string
	Version   string
	Revision  string
	Templated bool
}

func issue851CatalogReleaseFor(t *testing.T, commit string, packs ...issue851CatalogPack) catalogstore.Release {
	t.Helper()
	projectRoot := t.TempDir()
	origins := catalogOriginFixture{}
	for _, pack := range packs {
		current := testsupport.CapabilityRich(pack.ID).WithVersion(pack.Version)
		if err := current.WriteCatalog(projectRoot); err != nil {
			t.Fatal(err)
		}
		resolved, err := current.WriteProject(t.TempDir(), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		for id, root := range resolved {
			origins[id] = root
		}
		issue851RewriteCatalogPack(t, projectRoot, current.ID(), pack.Version, pack.Templated, pack.Revision)
	}
	validation, err := cataloglayout.ValidateCatalogProject(context.Background(), projectRoot, "", origins)
	if err != nil {
		t.Fatalf("validate issue 851 synthetic catalog: %v", err)
	}
	result, err := cataloglayout.BuildCatalogSnapshot(context.Background(), projectRoot, validation, cataloglayout.CatalogSnapshotSource{
		Repository: "yersonargotev/packy-catalog", Commit: commit, Builder: "yersonargotev/packy@" + strings.Repeat("c", 40),
	}, filepath.Join(t.TempDir(), "snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(result.ArchivePath)
	if err != nil {
		t.Fatal(err)
	}
	checksum, err := os.ReadFile(result.ChecksumPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(data []byte) string {
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	return catalogstore.Release{
		Repository: "yersonargotev/packy-catalog", Tag: "catalog-" + commit, Commit: commit,
		Publisher: "github-actions[bot]", Published: true, Immutable: true, AttestationVerified: true,
		Assets: []catalogstore.Asset{
			{Name: "catalog-snapshot.tar.gz", SHA256: digest(archive), Data: archive},
			{Name: "SHA256SUMS", SHA256: digest(checksum), Data: checksum},
		},
	}
}

func issue851ChangeTemplateContract(t *testing.T, catalogRoot, packID, surface string) {
	t.Helper()
	packRoot := filepath.Join(catalogRoot, "packs", packID)
	manifestPath := filepath.Join(packRoot, "pack.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, value := range manifest["resources"].([]any) {
		resource := value.(map[string]any)
		if resource["kind"] != "skill" || resource["id"] != "tdd" {
			continue
		}
		for _, rawBinding := range resource["bindings"].([]any) {
			binding := rawBinding.(map[string]any)
			if binding["surface"] != surface {
				continue
			}
			binding["name"] = "red-green"
			binding["invocation"] = "$red-green"
		}
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	tddPath := filepath.Join(packRoot, "skills", "tdd", "SKILL.md")
	tdd := readFileString(t, tddPath) + "reviewed source revision after preview\n"
	if err := os.WriteFile(tddPath, []byte(tdd), 0o644); err != nil {
		t.Fatal(err)
	}
	consumerPath := filepath.Join(packRoot, "skills", packID+"-consumer", "SKILL.md")
	consumer := readFileString(t, consumerPath)
	consumer = strings.Replace(consumer, "Sibling path: ../{{packy.skill:teach}}/SKILL.md", "Sibling path: ../{{packy.skill:tdd}}/SKILL.md", 1)
	if err := os.WriteFile(consumerPath, []byte(consumer), 0o644); err != nil {
		t.Fatal(err)
	}
}

func issue851SkillResource(packID, id, name, originPath string, requires []string, templated bool, naming string) map[string]any {
	if requires == nil {
		requires = []string{}
	}
	bindings := make([]any, 0, 3)
	for _, surface := range []string{"claude", "codex"} {
		invocation := name
		switch surface {
		case "claude":
			invocation = "/" + name
		case "codex":
			invocation = "$" + name
		}
		capabilities := []any{}
		if templated {
			capabilities = append(capabilities, map[string]any{
				"type":           "skill-template",
				"skill_template": map[string]any{"naming": naming},
			})
		}
		bindings = append(bindings, map[string]any{
			"surface": surface, "projection": "skill", "name": name, "invocation": invocation,
			"mode": "native", "sharing": "exclusive", "capabilities": capabilities,
		})
	}
	return map[string]any{
		"kind": "skill", "id": id, "source": "skills/" + id,
		"description": fmt.Sprintf("Synthetic %s %s", packID, id),
		"tools":       []string{}, "permissions": []string{}, "requires": requires, "conflicts": []string{},
		"notices": []string{"notice:mit"},
		"origin": map[string]any{
			"id": packID + "-upstream", "path": originPath, "relationship": "adapted",
		},
		"bindings": bindings, "surface_exclusions": []any{},
	}
}

func issue851AssertInstalled(t *testing.T, root, scope, surface, legacyID, templateID, revision, aliasMode string) {
	t.Helper()
	tddName, teachName := templateID+"-tdd", templateID+"-teach"
	if aliasMode == "custom" {
		tddName, teachName = "custom-tdd", "custom-teach"
	}
	legacyTDD := readFileString(t, filepath.Join(issue851SkillPath(root, scope, surface, "tdd"), "SKILL.md"))
	legacyTeach := readFileString(t, filepath.Join(issue851SkillPath(root, scope, surface, "teach"), "SKILL.md"))
	if !strings.Contains(legacyTDD, legacyID+" tdd v1") || !strings.Contains(legacyTeach, legacyID+" teach v1") {
		t.Fatalf("legacy definitions lost their own bodies:\ntdd:\n%s\nteach:\n%s", legacyTDD, legacyTeach)
	}
	tdd := readFileString(t, filepath.Join(issue851SkillPath(root, scope, surface, tddName), "SKILL.md"))
	teach := readFileString(t, filepath.Join(issue851SkillPath(root, scope, surface, teachName), "SKILL.md"))
	consumerPath := issue851SkillPath(root, scope, surface, templateID+"-consumer")
	consumer := readFileString(t, filepath.Join(consumerPath, "SKILL.md"))
	for label, body := range map[string]string{"tdd": tdd, "teach": teach, "consumer": consumer} {
		if strings.Contains(body, "{{packy.skill:") {
			t.Fatalf("%s retained an unresolved template token:\n%s", label, body)
		}
	}
	for _, want := range []string{
		"name: " + tddName,
		templateID + " tdd " + revision,
	} {
		if !strings.Contains(tdd, want) {
			t.Fatalf("rendered tdd missing %q:\n%s", want, tdd)
		}
	}
	for _, want := range []string{
		"name: " + teachName,
		templateID + " teach " + revision,
		"$" + tddName,
	} {
		if !strings.Contains(teach, want) {
			t.Fatalf("rendered teach missing %q:\n%s", want, teach)
		}
	}
	for _, want := range []string{
		"name: " + templateID + "-consumer",
		templateID + " consumer " + revision,
		"Dollar call: $" + tddName,
		"Slash call: /" + teachName,
		"Skill tool name: `" + tddName + "`",
		"Sibling path: ../" + teachName + "/SKILL.md",
		"Unmarked shell example stays $tdd",
	} {
		if !strings.Contains(consumer, want) {
			t.Fatalf("rendered consumer missing %q:\n%s", want, consumer)
		}
	}
	if _, err := os.Stat(filepath.Clean(filepath.Join(consumerPath, "..", teachName, "SKILL.md"))); err != nil {
		t.Fatalf("rendered sibling reference does not address the installed skill: %v", err)
	}
}

func issue851SkillPath(root, scope, surface, name string) string {
	if scope == "project" {
		switch surface {
		case "claude":
			return filepath.Join(root, ".claude", "skills", name)
		default:
			return filepath.Join(root, ".agents", "skills", name)
		}
	}
	switch surface {
	case "claude":
		return filepath.Join(root, ".claude", "skills", name)
	default:
		return filepath.Join(root, ".agents", "skills", name)
	}
}

func issue851WriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
