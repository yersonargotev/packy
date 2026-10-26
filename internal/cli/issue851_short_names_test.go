package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue851ShortNamesPreserveMixedSurfaceDiscoveryAndOwnership(t *testing.T) {
	for _, pair := range [][2]string{{"agents", "claude"}} {
		for _, divergent := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				for _, firstScope := range []string{"global", "project"} {
					for _, secondScope := range []string{"global", "project"} {
						first, second := pair[0], pair[1]
						if reverse {
							first, second = second, first
						}
						name := fmt.Sprintf("%s-%s/then-%s-%s/divergent-%t", first, firstScope, second, secondScope, divergent)
						t.Run(name, func(t *testing.T) {
							pack := testsupport.SkillVariants("short-names")
							if divergent {
								pack = testsupport.SkillVariants("short-names", testsupport.SurfaceAgents, testsupport.SurfaceClaude)
							}
							fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
							issue851EnablePackNaming(t, fixture.catalogRoot, pack.ID())
							project := t.TempDir()
							writeTestGitWorktree(t, project)
							opts := fixture.options
							opts.Getwd = func() (string, error) { return project, nil }
							command := func(scope string) string {
								if scope == "project" {
									return "install"
								}
								return "activate"
							}
							root := func(scope string) string {
								if scope == "project" {
									return project
								}
								return fixture.home
							}
							if out, err := executeCommand(t, NewRootCommand(opts), command(firstScope), pack.ID(), "--surface", first); err != nil {
								t.Fatalf("first install: %v\n%s", err, out)
							}
							firstTarget := issue851SkillPath(root(firstScope), firstScope, first, "short-names-guide")
							firstTree := snapshotTree(t, firstTarget)
							if body := readFileString(t, filepath.Join(firstTarget, "SKILL.md")); !strings.Contains(body, "name: short-names-guide") {
								t.Fatalf("default name requires no alias:\n%s", body)
							}
							out, err := executeCommand(t, NewRootCommand(opts), command(secondScope), pack.ID(), "--surface", second, "--json")
							if err != nil {
								t.Fatalf("compatible short names failed: %v\n%s", err, out)
							}
							secondTarget := issue851SkillPath(root(secondScope), secondScope, second, "short-names-guide")
							if body := readFileString(t, filepath.Join(secondTarget, "SKILL.md")); !strings.Contains(body, "name: short-names-guide") {
								t.Fatalf("second host has a different native name:\n%s", body)
							}
							remove := "deactivate"
							if secondScope == "project" {
								remove = "uninstall"
							}
							if out, err := executeCommand(t, NewRootCommand(opts), remove, pack.ID(), "--surface", second); err != nil {
								t.Fatalf("remove second host: %v\n%s", err, out)
							}
							if snapshotTree(t, firstTarget) != firstTree {
								t.Fatal("another host's installation or removal changed the original tree")
							}
						})
					}
				}
			}
		}
	}
}

func issue851EnablePackNaming(t *testing.T, catalogRoot, packID string) {
	t.Helper()
	path := filepath.Join(catalogRoot, "packs", packID, "pack.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, value := range manifest["resources"].([]any) {
		resource := value.(map[string]any)
		if resource["kind"] != "skill" {
			continue
		}
		for _, value := range resource["bindings"].([]any) {
			binding := value.(map[string]any)
			if binding["surface"] == "agents" {
				binding["invocation"] = binding["name"]
			}
			binding["capabilities"] = []any{map[string]any{
				"type": "skill-template", "skill_template": map[string]any{"naming": "pack"},
			}}
		}
	}
	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
