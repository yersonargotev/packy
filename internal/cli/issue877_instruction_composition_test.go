package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestIssue877ProjectInstructionsComposeAcrossPacksAndRemoveExactly(t *testing.T) {
	for _, sameMarker := range []bool{false, true} {
		name := "distinct contributions"
		if sameMarker {
			name = "same marker collision"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, testsupport.PortableAllSurfaces("alpha"), testsupport.PortableAllSurfaces("beta"))
			for _, id := range []string{"alpha", "beta"} {
				manifest := fixture.pack(t, id).Manifest()
				for i := range manifest.Resources {
					resource := &manifest.Resources[i]
					if resource.Kind != "instruction" {
						continue
					}
					resource.ID = id
					for j := range resource.Bindings {
						binding := &resource.Bindings[j]
						binding.Name, binding.Invocation = id, id
						for k := range binding.Capabilities {
							if contribution := binding.Capabilities[k].ProjectInstruction; contribution != nil {
								contribution.ID = id
								if sameMarker {
									contribution.ID = "shared-marker"
								}
							}
						}
					}
				}
				data, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(fixture.catalogRoot, "packs", id, "pack.json"), data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			project, err := fixture.options.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(project, "AGENTS.md")
			const prelude = "User guidance.\n"
			const alpha = "<!-- packy:project:instruction:alpha:start -->\n# Alpha guidance\n\nSynthetic guidance for every supported surface.\n<!-- packy:project:instruction:alpha:end -->\n"
			const beta = "<!-- packy:project:instruction:beta:start -->\n# Beta guidance\n\nSynthetic guidance for every supported surface.\n<!-- packy:project:instruction:beta:end -->\n"
			if err := os.WriteFile(path, []byte(prelude), 0644); err != nil {
				t.Fatal(err)
			}
			run := func(verb, id string) {
				t.Helper()
				out, err := executeCommand(t, NewRootCommand(fixture.options), verb, id, "--surface", "agents")
				if err != nil {
					t.Fatalf("%s %s: %v\n%s", verb, id, err, out)
				}
			}
			assertDocument := func(want string) {
				t.Helper()
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("complete AGENTS.md differs: error=%v\ngot: %q\nwant: %q", err, got, want)
				}
			}
			assertRemainingContribution := func(want string) {
				t.Helper()
				got, err := os.ReadFile(path)
				// Marked contributions own their content, not the document's separator whitespace.
				// Compare the complete remainder so no retired text can hide between retained parts.
				if err != nil || !strings.HasPrefix(string(got), prelude) || strings.TrimSpace(strings.TrimPrefix(string(got), prelude)) != strings.TrimSpace(want) {
					t.Fatalf("complete retained document differs: error=%v\ngot: %q\nwant prelude plus: %q", err, got, want)
				}
			}
			run("install", "alpha")
			if sameMarker {
				beforeProject, beforeHome := snapshotTree(t, project), snapshotTree(t, fixture.home)
				out, err := executeCommand(t, NewRootCommand(fixture.options), "install", "beta", "--surface", "agents")
				if err == nil || !strings.Contains(strings.ToLower(out+err.Error()), "collision") {
					t.Fatalf("same marker was not rejected: %v\n%s", err, out)
				}
				if snapshotTree(t, project) != beforeProject || snapshotTree(t, fixture.home) != beforeHome {
					t.Fatal("colliding contribution changed project or personal state")
				}
				return
			}
			assertDocument(prelude + "\n" + alpha)
			run("install", "beta")
			assertDocument(prelude + "\n" + alpha + "\n" + beta)
			run("uninstall", "alpha")
			assertRemainingContribution(beta)
			run("uninstall", "beta")
			assertRemainingContribution("")
		})
	}
}
