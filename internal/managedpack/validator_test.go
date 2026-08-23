package managedpack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"golang.org/x/sys/unix"
)

type originResolver map[string]string

func (r originResolver) Resolve(_ context.Context, origin Origin) (string, error) {
	return r[origin.ID], nil
}

func TestValidateProjectAcceptsEverySupportedSurfaceAndBuildsDeterministicClosure(t *testing.T) {
	project, origin := writeValidProject(t)

	first, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err != nil {
		t.Fatal(err)
	}

	if first.Manifest.ID != "example" || first.Manifest.Version != "1.0.0" {
		t.Fatalf("manifest = %#v", first.Manifest)
	}
	if first.ManifestSHA256 == "" || first.ClosureSHA256 == "" {
		t.Fatalf("digests = manifest %q closure %q", first.ManifestSHA256, first.ClosureSHA256)
	}
	if !reflect.DeepEqual(first.Files, []FileRecord{
		{Path: "notices/mit", Mode: "100644", SHA256: "0188c8cdbb2342a2c57751a6fec4feb64612c8ec0407d0f21b2efc05bca45389"},
		{Path: "pack.json", Mode: "100644", SHA256: first.ManifestSHA256},
		{Path: "skills/guide/SKILL.md", Mode: "100644", SHA256: "3e99af1fa8e1986e6ccee509104f66ba8892131f99eaba6d9b83076fb88d3bb1"},
	}) {
		t.Fatalf("files = %#v", first.Files)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("validation is not deterministic:\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

func TestValidateProjectRejectsMalformedManifestAndOriginRelationships(t *testing.T) {
	tests := []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{"schema version", func(manifest map[string]any) { manifest["schema_version"] = 2 }, "schema_version must be 1"},
		{"null origins", func(manifest map[string]any) { manifest["origins"] = nil }, "origins is a required non-null array"},
		{"unknown origin", func(manifest map[string]any) { resource(manifest)["origin"].(map[string]any)["id"] = "missing" }, "unknown origin"},
		{"invalid relationship", func(manifest map[string]any) {
			resource(manifest)["origin"].(map[string]any)["relationship"] = "partial"
		}, "relationship must be exact-copy or adapted"},
		{"git metadata origin path", func(manifest map[string]any) {
			resource(manifest)["origin"].(map[string]any)["path"] = ".git/config"
		}, "must not select Git metadata"},
		{"case-folded git metadata origin path", func(manifest map[string]any) {
			resource(manifest)["origin"].(map[string]any)["path"] = ".GIT/config"
		}, "must not select Git metadata"},
		{"derived without notice", func(manifest map[string]any) { delete(resource(manifest), "notices") }, "must reference at least one notice"},
		{"retired exclusions", func(manifest map[string]any) { manifest["exclusions"] = []any{} }, `unknown field "exclusions"`},
		{"retired source reference", func(manifest map[string]any) {
			manifest["source_reference"] = map[string]any{"repository": "example/upstream", "revision": "v1"}
		}, `unknown field "source_reference"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project, origin := writeValidProject(t)
			mutateManifest(t, project, test.edit)
			_, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateProjectRejectsUnsafeClosureEntries(t *testing.T) {
	tests := []struct {
		name string
		edit func(*testing.T, string)
		want string
	}{
		{"absolute path", func(t *testing.T, project string) {
			mutateManifest(t, project, func(manifest map[string]any) { resource(manifest)["source"] = "/skills/guide" })
		}, "relative path"},
		{"traversal", func(t *testing.T, project string) {
			mutateManifest(t, project, func(manifest map[string]any) { resource(manifest)["source"] = "../guide" })
		}, "escapes the bundle root"},
		{"missing path", func(t *testing.T, project string) {
			if err := os.Remove(filepath.Join(project, "skills", "guide", "SKILL.md")); err != nil {
				t.Fatal(err)
			}
		}, "missing SKILL.md"},
		{"symlink", func(t *testing.T, project string) {
			if err := os.Symlink("SKILL.md", filepath.Join(project, "skills", "guide", "linked.md")); err != nil {
				t.Fatal(err)
			}
		}, "is a symlink"},
		{"submodule", func(t *testing.T, project string) {
			writeFile(t, filepath.Join(project, "skills", "guide", ".git"), "gitdir: elsewhere\n", 0o644)
		}, "contains submodule or Git metadata"},
		{"non-regular file", func(t *testing.T, project string) {
			if err := unix.Mkfifo(filepath.Join(project, "skills", "guide", "pipe"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "is not a regular file"},
		{"overlapping roots", func(t *testing.T, project string) {
			writeFile(t, filepath.Join(project, "skills", "guide", "nested", "SKILL.md"), "nested\n", 0o644)
			mutateManifest(t, project, func(manifest map[string]any) {
				resources := manifest["resources"].([]any)
				resources[1].(map[string]any)["origin"].(map[string]any)["relationship"] = "adapted"
				clone := deepCopyMap(t, resources[1].(map[string]any))
				clone["id"] = "nested"
				clone["source"] = "skills/guide/nested"
				delete(clone, "origin")
				delete(clone, "notices")
				for _, value := range clone["bindings"].([]any) {
					binding := value.(map[string]any)
					binding["name"] = "nested"
					binding["invocation"] = "nested"
				}
				manifest["resources"] = append(resources, clone)
			})
		}, "overlap"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project, origin := writeValidProject(t)
			test.edit(t, project)
			_, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateProjectEnforcesExactCopyAndAllowsNoticedAdaptation(t *testing.T) {
	project, origin := writeValidProject(t)
	writeFile(t, filepath.Join(project, "skills", "guide", "SKILL.md"), "adapted guidance\n", 0o644)

	_, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err == nil || !IsExactCopyMismatch(err) || !strings.Contains(err.Error(), "exact-copy mismatch") {
		t.Fatalf("exact-copy error = %v", err)
	}

	mutateManifest(t, project, func(manifest map[string]any) {
		resource(manifest)["origin"].(map[string]any)["relationship"] = "adapted"
	})
	if _, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin}); err != nil {
		t.Fatalf("noticed adaptation: %v", err)
	}
}

func TestValidateProjectReportsBoundedDeterministicExactCopyDifferences(t *testing.T) {
	project, origin := writeValidProject(t)
	projectRoot := filepath.Join(project, "skills", "guide")
	originRoot := filepath.Join(origin, "guide")
	writeFile(t, filepath.Join(projectRoot, "a-changed.txt"), "project secret\n", 0o644)
	writeFile(t, filepath.Join(originRoot, "a-changed.txt"), "origin secret\n", 0o644)
	writeFile(t, filepath.Join(originRoot, "b-missing.txt"), "missing secret\n", 0o644)
	writeFile(t, filepath.Join(projectRoot, "c-additional.txt"), "additional secret\n", 0o644)
	for i := 0; i < maxExactCopyMismatchDetails; i++ {
		writeFile(t, filepath.Join(projectRoot, fmt.Sprintf("extra-%02d.txt", i)), "bounded secret\n", 0o644)
	}

	_, first := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	_, second := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if first == nil || second == nil || first.Error() != second.Error() {
		t.Fatalf("exact-copy diagnostics are not deterministic:\nfirst:  %v\nsecond: %v", first, second)
	}
	if !IsExactCopyMismatch(first) {
		t.Fatalf("error type = %T, want exact-copy mismatch", first)
	}
	message := first.Error()
	for _, want := range []string{
		`resource "skill:guide"`, `origin "upstream" path "guide"`,
		`mismatch=changed path="a-changed.txt"`,
		`project_sha256="` + digestBytes([]byte("project secret\n")) + `"`,
		`origin_sha256="` + digestBytes([]byte("origin secret\n")) + `"`,
		`mismatch=missing path="b-missing.txt"`,
		`mismatch=additional path="c-additional.txt"`,
		`3 additional differences omitted`,
		`restore exact bytes from the declared origin or explicitly declare the whole resource "adapted" and review its notices`,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("exact-copy error = %q, want %q", message, want)
		}
	}
	if changed, missing, additional := strings.Index(message, `path="a-changed.txt"`), strings.Index(message, `path="b-missing.txt"`), strings.Index(message, `path="c-additional.txt"`); changed < 0 || changed >= missing || missing >= additional {
		t.Fatalf("mismatch ordering is not deterministic by relative path: %q", message)
	}
	if got := strings.Count(message, "mismatch="); got != maxExactCopyMismatchDetails {
		t.Fatalf("reported mismatch count = %d, want %d: %q", got, maxExactCopyMismatchDetails, message)
	}
	for _, secret := range []string{"project secret", "origin secret", "missing secret", "additional secret", "bounded secret"} {
		if strings.Contains(message, secret) {
			t.Fatalf("exact-copy diagnostic exposed file content %q: %q", secret, message)
		}
	}
}

func TestValidateProjectChecksCapabilityLayoutWhenResourceHasNoSource(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "prompts", "guide.md"), "guidance\n", 0o644)
	writeFile(t, filepath.Join(project, "pack.json"), lifecycleManifest, 0o644)

	_, err := ValidateProject(context.Background(), project, nil)
	if err == nil || !strings.Contains(err.Error(), "canonical instructions/ layout") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateProjectRejectsSourceOnDeclarationOnlyResource(t *testing.T) {
	project := t.TempDir()
	writeFile(t, filepath.Join(project, "instructions", "guide.md"), "guidance\n", 0o644)
	writeFile(t, filepath.Join(project, "pack.json"), lifecycleManifest, 0o644)
	mutateManifest(t, project, func(manifest map[string]any) {
		lifecycle := manifest["resources"].([]any)[0].(map[string]any)
		lifecycle["source"] = "instructions/guide.md"
		capability := lifecycle["bindings"].([]any)[0].(map[string]any)["capabilities"].([]any)[0].(map[string]any)
		capability["project_instruction"].(map[string]any)["source"] = "instructions/guide.md"
	})

	_, err := ValidateProject(context.Background(), project, nil)
	if err == nil || !strings.Contains(err.Error(), "does not own a source root") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateProjectRequiresNoticeCoverageForDerivedNotice(t *testing.T) {
	project, origin := writeValidProject(t)
	writeFile(t, filepath.Join(origin, "LICENSE"), "MIT notice\n", 0o644)
	mutateManifest(t, project, func(manifest map[string]any) {
		notice := manifest["resources"].([]any)[0].(map[string]any)
		notice["origin"] = map[string]any{
			"id":           "upstream",
			"path":         "LICENSE",
			"relationship": "exact-copy",
		}
	})

	_, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err == nil || !strings.Contains(err.Error(), "must reference at least one notice") {
		t.Fatalf("error = %v", err)
	}

	mutateManifest(t, project, func(manifest map[string]any) {
		notice := manifest["resources"].([]any)[0].(map[string]any)
		notice["notices"] = []any{"notice:mit"}
	})
	if _, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin}); err != nil {
		t.Fatalf("self-covered derived notice: %v", err)
	}
}

func TestValidateProjectAcceptsExactCopyFromOriginRoot(t *testing.T) {
	project, origin := writeValidProject(t)
	if err := os.RemoveAll(filepath.Join(origin, "guide")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(origin, "SKILL.md"), "managed guidance\n", 0o644)
	writeFile(t, filepath.Join(origin, ".gitmodules"), "tracked content\n", 0o644)
	writeFile(t, filepath.Join(project, "skills", "guide", ".gitmodules"), "tracked content\n", 0o644)
	mutateManifest(t, project, func(manifest map[string]any) {
		resource(manifest)["origin"].(map[string]any)["path"] = "."
	})

	if _, err := ValidateProject(context.Background(), project, originResolver{"upstream": origin}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProjectRejectsSubmoduleInExactCopyOrigin(t *testing.T) {
	project, origin := writeValidProject(t)
	repository, err := git.PlainInit(origin, false)
	if err != nil {
		t.Fatal(err)
	}
	index, err := repository.Storer.Index()
	if err != nil {
		t.Fatal(err)
	}
	entry := index.Add("guide/vendor")
	entry.Mode = filemode.Submodule
	entry.Hash = plumbing.NewHash("0123456789abcdef0123456789abcdef01234567")
	if err := repository.Storer.SetIndex(index); err != nil {
		t.Fatal(err)
	}

	_, err = ValidateProject(context.Background(), project, originResolver{"upstream": origin})
	if err == nil || !strings.Contains(err.Error(), "origin \"upstream\"") || !strings.Contains(err.Error(), "submodule") {
		t.Fatalf("error = %v", err)
	}
}

func writeFile(t *testing.T, name, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func writeValidProject(t *testing.T) (string, string) {
	t.Helper()
	project := t.TempDir()
	origin := t.TempDir()
	writeFile(t, filepath.Join(project, "skills", "guide", "SKILL.md"), "managed guidance\n", 0o644)
	writeFile(t, filepath.Join(project, "notices", "mit"), "MIT notice\n", 0o644)
	writeFile(t, filepath.Join(origin, "guide", "SKILL.md"), "managed guidance\n", 0o644)
	writeFile(t, filepath.Join(project, "pack.json"), validManifest, 0o644)
	return project, origin
}

func mutateManifest(t *testing.T, project string, edit func(map[string]any)) {
	t.Helper()
	path := filepath.Join(project, "pack.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	edit(manifest)
	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data)+"\n", 0o644)
}

func resource(manifest map[string]any) map[string]any {
	return manifest["resources"].([]any)[1].(map[string]any)
}

func deepCopyMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

const validManifest = `{
  "schema_version": 1,
  "id": "example",
  "version": "1.0.0",
  "description": "Example Managed Pack",
  "selectable": true,
  "surfaces": ["claude", "codex", "opencode"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [
    {
      "id": "upstream",
      "repository": "example/upstream",
      "commit": "0123456789abcdef0123456789abcdef01234567",
      "revision": "v1.0.0"
    }
  ],
  "resources": [
    {
      "kind": "notice",
      "id": "mit",
      "source": "notices/mit",
      "description": "Preserves the MIT notice",
      "license": "MIT",
      "attribution": "Copyright Example",
      "requires": [],
      "conflicts": [],
      "bindings": [],
      "surface_exclusions": []
    },
    {
      "kind": "skill",
      "id": "guide",
      "source": "skills/guide",
      "description": "Provides managed guidance",
      "requires": [],
      "conflicts": [],
      "notices": ["notice:mit"],
      "origin": {
        "id": "upstream",
        "path": "guide",
        "relationship": "exact-copy"
      },
      "bindings": [
        {
          "surface": "claude",
          "projection": "skill",
          "name": "guide",
          "invocation": "guide",
          "mode": "native",
          "sharing": "shared",
          "capabilities": []
        },
        {
          "surface": "codex",
          "projection": "skill",
          "name": "guide",
          "invocation": "guide",
          "mode": "native",
          "sharing": "shared",
          "capabilities": []
        },
        {
          "surface": "opencode",
          "projection": "skill",
          "name": "guide",
          "invocation": "guide",
          "mode": "native",
          "sharing": "shared",
          "capabilities": []
        }
      ],
      "surface_exclusions": []
    }
  ]
}
`

const lifecycleManifest = `{
  "schema_version": 1,
  "id": "example",
  "version": "1.0.0",
  "description": "Example Managed Pack",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [],
  "resources": [
    {
      "kind": "lifecycle",
      "id": "guide",
      "description": "Contributes project guidance",
      "requires": [],
      "conflicts": [],
      "bindings": [
        {
          "surface": "codex",
          "projection": "lifecycle",
          "name": "guide",
          "invocation": "guide",
          "mode": "native",
          "sharing": "exclusive",
          "capabilities": [
            {
              "type": "project-instruction",
              "project_instruction": {
                "id": "guide",
                "source": "prompts/guide.md"
              }
            }
          ]
        }
      ],
      "surface_exclusions": []
    }
  ]
}
`
