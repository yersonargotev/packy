package capabilitypack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidatePortableContentValidatesEveryManifestAndReferencedResource(t *testing.T) {
	bundle := t.TempDir()
	writePortableFixture(t, bundle, "one", "instructions/one.md")
	writePortableFixture(t, bundle, "two", "instructions/two.md")

	if err := ValidatePortableContent(bundle); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(bundle, "instructions", "two.md")); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePortableContent(bundle); err == nil || !strings.Contains(err.Error(), "two.md") {
		t.Fatalf("missing referenced resource error = %v", err)
	}
}

func TestValidatePortableContentRejectsRetiredPackDirectories(t *testing.T) {
	bundle := t.TempDir()
	writePortableFixture(t, bundle, "current", "instructions/current.md")
	vercelDir := filepath.Join(bundle, "packs", "vercel")
	if err := os.MkdirAll(vercelDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vercelDir, "pack.json"), []byte(`{"schema_version":4,"id":"vercel"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	err := ValidatePortableContent(bundle)
	if err == nil || !strings.Contains(err.Error(), "vercel") {
		t.Fatalf("retired Pack error = %v", err)
	}
}

func TestValidatePackContentAcceptsCurrentManifest(t *testing.T) {
	bundle := t.TempDir()
	packDir := writeCurrentPackFixture(t, bundle, "example-pack")

	pack, err := ValidatePackContent(bundle, packDir)
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "example-pack" || pack.Version != "1.0.0" || pack.Description != "Example Pack" || !pack.Selectable {
		t.Fatalf("pack = %#v", pack)
	}
	if len(pack.Requires.Tools) != 1 || pack.Requires.Tools[0] != "example-tool" {
		t.Fatalf("external requirements = %#v", pack.Requires.Tools)
	}
	if got, want := pack.ReadinessObligations, []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization}; !reflect.DeepEqual(got, want) {
		t.Fatalf("readiness obligations = %#v, want %#v", got, want)
	}
	if got, want := pack.Resources[0].Bindings[0].Capabilities, []SurfaceCapability{{Type: SurfaceCapabilityProjectInstruction, ProjectInstruction: &ProjectInstructionCapability{ID: "guide", Source: "packs/example-pack/instructions/guide.md"}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("surface capabilities = %#v, want %#v", got, want)
	}
}

func TestLoadCurrentManifestRejectsInvalidSurfaceCapabilities(t *testing.T) {
	for _, test := range []struct {
		name, replace, want string
	}{
		{
			name:    "null capabilities",
			replace: `          "capabilities": null`,
			want:    "capabilities is a required non-null array",
		},
		{
			name:    "unknown capability",
			replace: `          "capabilities": [{"type": "custom-extension", "project_instruction": {"id": "guide", "source": "packs/example-pack/instructions/guide.md"}}]`,
			want:    `surface capability "custom-extension" is unsupported`,
		},
		{
			name:    "missing typed data",
			replace: `          "capabilities": [{"type": "project-instruction"}]`,
			want:    `surface capability "project-instruction" requires project_instruction data`,
		},
		{
			name:    "generic extension data",
			replace: `          "capabilities": [{"type": "project-instruction", "project_instruction": {"id": "guide", "source": "packs/example-pack/instructions/guide.md"}, "data": {"custom": true}}]`,
			want:    `unknown field "data"`,
		},
		{
			name:    "missing primary prompt data",
			replace: `          "capabilities": [{"type": "opencode-primary-prompt"}]`,
			want:    `surface capability "opencode-primary-prompt" requires primary_prompt data`,
		},
		{
			name:    "malformed primary prompt identity",
			replace: `          "capabilities": [{"type": "opencode-primary-prompt", "primary_prompt": {"id": "Primary Prompt", "source": "packs/example-pack/instructions/guide.md"}}]`,
			want:    `surface capability "opencode-primary-prompt" primary_prompt id must be lowercase kebab-case`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := t.TempDir()
			packDir := writeCurrentPackFixture(t, bundle, "example-pack")
			path := filepath.Join(packDir, "pack.json")
			data := string(mustReadFile(t, path))
			data = strings.Replace(data, `          "capabilities": [{"type": "project-instruction", "project_instruction": {"id": "guide", "source": "packs/example-pack/instructions/guide.md"}}]`, test.replace, 1)
			if strings.Contains(test.replace, `"type": "opencode-primary-prompt"`) {
				data = strings.Replace(data, `"surfaces": ["codex"]`, `"surfaces": ["opencode"]`, 1)
				data = strings.Replace(data, `"surface": "codex"`, `"surface": "opencode"`, 1)
			}
			if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadCurrentManifest(path, bundle, false)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("surface capability error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidatePackContentRequiresExplicitSelectability(t *testing.T) {
	bundle := t.TempDir()
	packDir := writeCurrentPackFixture(t, bundle, "example-pack")
	manifest := filepath.Join(packDir, "pack.json")
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "  \"selectable\": true,\n", "", 1))
	if err := os.WriteFile(manifest, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = ValidatePackContent(bundle, packDir)
	if err == nil || !strings.Contains(err.Error(), "field selectable is required") {
		t.Fatalf("selectability error = %v", err)
	}
}

func TestLoadCurrentManifestRequiresReadinessObligations(t *testing.T) {
	bundle := t.TempDir()
	packDir := writeCurrentPackFixture(t, bundle, "example-pack")
	path := filepath.Join(packDir, "pack.json")
	var manifest map[string]any
	if err := json.Unmarshal(mustReadFile(t, path), &manifest); err != nil {
		t.Fatal(err)
	}
	delete(manifest, "readiness_obligations")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = LoadCurrentManifest(path, bundle, false)
	if err == nil || !strings.Contains(err.Error(), "field readiness_obligations") {
		t.Fatalf("readiness obligations error = %v", err)
	}
}

func TestLoadCurrentManifestRequiresDescriptionForEveryResourceKind(t *testing.T) {
	bundle, err := filepath.Abs(filepath.Join("..", "..", "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, pack, kind, id string
		description          any
	}{
		{"agent", "addy", "agent", "code-reviewer", nil},
		{"asset", "addy", "asset", "accessibility-checklist", nil},
		{"command", "addy", "command", "build", nil},
		{"instruction", "argote", "instruction", "guidance", nil},
		{"lifecycle", "engram", "lifecycle", "engram-memory", nil},
		{"mcp server", "engram", "mcp_server", "engram", nil},
		{"notice", "addy", "notice", "mit", nil},
		{"skill", "addy", "skill", "api-and-interface-design", nil},
		{"whitespace-only", "argote", "instruction", "guidance", " \n\t "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var manifest map[string]any
			path := filepath.Join(bundle, "packs", test.pack, "pack.json")
			if err := json.Unmarshal(mustReadFile(t, path), &manifest); err != nil {
				t.Fatal(err)
			}
			for _, value := range manifest["resources"].([]any) {
				resource := value.(map[string]any)
				if resource["kind"] == test.kind && resource["id"] == test.id {
					if test.description == nil {
						delete(resource, "description")
					} else {
						resource["description"] = test.description
					}
				}
			}
			encoded, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			invalidPath := filepath.Join(t.TempDir(), "pack.json")
			if err := os.WriteFile(invalidPath, encoded, 0o600); err != nil {
				t.Fatal(err)
			}

			_, err = LoadCurrentManifest(invalidPath, bundle, false)
			want := `Pack "` + test.pack + `" resource "` + test.kind + `:` + test.id + `" field description is required`
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("resource description error = %v, want %q", err, want)
			}
		})
	}
}

func TestValidatePortableContentReportsMissingSelectability(t *testing.T) {
	bundle := t.TempDir()
	packDir := writeCurrentPackFixture(t, bundle, "example-pack")
	manifest := filepath.Join(packDir, "pack.json")
	data := strings.Replace(string(mustReadFile(t, manifest)), "  \"selectable\": true,\n", "", 1)
	if err := os.WriteFile(manifest, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	err := ValidatePortableContent(bundle)
	if err == nil || !strings.Contains(err.Error(), "field selectable is required") {
		t.Fatalf("whole-bundle selectability error = %v", err)
	}
}

func TestValidatePackContentReportsCurrentContractErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"legacy schema selector", func(m map[string]any) { m["schema_version"] = 4 }, "unknown field"},
		{"runtime modes", func(m map[string]any) { currentFixtureResource(m)["runtime_modes"] = []any{} }, "unknown field"},
		{"root migrations", func(m map[string]any) { m["root_migrations"] = []any{} }, "unknown field"},
		{"cross-Pack capabilities", func(m map[string]any) { m["provides"] = []any{"cap:example"} }, "unknown field"},
		{"missing resources array", func(m map[string]any) { m["resources"] = nil }, "field resources"},
		{"missing readiness obligations", func(m map[string]any) { delete(m, "readiness_obligations") }, "field readiness_obligations"},
		{"null readiness obligations", func(m map[string]any) { m["readiness_obligations"] = nil }, "field readiness_obligations"},
		{"unsorted readiness obligations", func(m map[string]any) {
			m["readiness_obligations"] = []any{"surface-authorization", "runtime-usability"}
		}, "readiness_obligations"},
		{"unknown readiness obligation", func(m map[string]any) { m["readiness_obligations"] = []any{"readiness-probe", "runtime-usability"} }, "readiness_obligations"},
		{"readiness probe", func(m map[string]any) { m["readiness_probe"] = map[string]any{} }, "unknown field"},
		{"readiness extensions", func(m map[string]any) { m["readiness_extensions"] = []any{} }, "unknown field"},
		{"unknown dependency", func(m map[string]any) { currentFixtureResource(m)["requires"] = []any{"skill:missing"} }, `dependency "skill:missing" does not exist`},
		{"unsupported surface", func(m map[string]any) { m["surfaces"] = []any{"mobile"} }, "unsupported CLI surface"},
		{"invalid external requirement", func(m map[string]any) { m["external_requirements"] = []any{"Example Tool"} }, "external_requirements"},
		{"unknown concrete conflict", func(m map[string]any) { currentFixtureResource(m)["conflicts"] = []any{"skill:missing"} }, `conflict "skill:missing" does not exist`},
		{"overlapping exclusion", func(m map[string]any) {
			m["exclusions"] = []any{map[string]any{"id": "selected", "source_paths": []any{"packs/example-pack/instructions/guide.md"}, "reason": "must remain selected"}}
		}, "overlaps selected resource"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := t.TempDir()
			packDir := writeCurrentPackFixture(t, bundle, "example-pack")
			path := filepath.Join(packDir, "pack.json")
			var manifest map[string]any
			if err := json.Unmarshal(mustReadFile(t, path), &manifest); err != nil {
				t.Fatal(err)
			}
			test.mutate(manifest)
			encoded, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = ValidatePackContent(bundle, packDir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidatePackContentAcceptsExplicitExternalHostSetupCapability(t *testing.T) {
	bundle, err := filepath.Abs(filepath.Join("..", "..", "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePackContent(bundle, filepath.Join(bundle, "packs", "engram")); err != nil {
		t.Fatalf("explicit external host setup capability: %v", err)
	}
}

func TestValidatePackContentRejectsInvalidExternalHostSetupDeclarations(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "missing managed resources",
			mutate: func(setup map[string]any) {
				delete(setup, "managed_resources")
			},
			want: "managed_resources must be a non-empty sorted set",
		},
		{
			name: "unsupported tool",
			mutate: func(setup map[string]any) {
				setup["tool"] = "unknown-tool"
			},
			want: `tool "unknown-tool" is unsupported`,
		},
		{
			name: "surface-mismatched setup arguments",
			mutate: func(setup map[string]any) {
				setup["setup_args"] = []any{"setup", "opencode"}
			},
			want: `setup_args must be ["setup", "codex"]`,
		},
		{
			name: "incomplete Codex contract",
			mutate: func(setup map[string]any) {
				delete(setup, "codex")
			},
			want: "on codex requires only codex data",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := copyCheckedInBundle(t)
			path := filepath.Join(bundle, "packs", "engram", "pack.json")
			manifest := decodeManifestMap(t, path)
			test.mutate(externalHostSetupMap(t, manifest, "mcp_server", "engram", "codex"))
			writeManifestMap(t, path, manifest)
			_, err := ValidatePackContent(bundle, filepath.Dir(path))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("external host setup error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidatePackContentRejectsConflictingExternalHostSetupDeclarations(t *testing.T) {
	bundle := copyCheckedInBundle(t)
	path := filepath.Join(bundle, "packs", "engram", "pack.json")
	manifest := decodeManifestMap(t, path)
	resources := manifest["resources"].([]any)
	original := resources[len(resources)-1].(map[string]any)
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var duplicate map[string]any
	if err := json.Unmarshal(encoded, &duplicate); err != nil {
		t.Fatal(err)
	}
	duplicate["id"] = "engram-copy"
	for _, value := range duplicate["bindings"].([]any) {
		binding := value.(map[string]any)
		binding["name"] = "engram-copy"
		binding["invocation"] = "engram-copy"
		capabilities := binding["capabilities"].([]any)
		if len(capabilities) == 0 {
			continue
		}
		managed := capabilities[0].(map[string]any)["external_host_setup"].(map[string]any)["managed_resources"].([]any)
		managed[1].(map[string]any)["id"] = "engram-copy"
	}
	manifest["resources"] = append(resources, duplicate)
	writeManifestMap(t, path, manifest)
	_, err = ValidatePackContent(bundle, filepath.Dir(path))
	if err == nil || !strings.Contains(err.Error(), "declare conflicting external host setup") {
		t.Fatalf("conflicting external host setup error = %v", err)
	}
}

func TestValidatePackContentRejectsExternalHostSetupWithoutRequirement(t *testing.T) {
	bundle := t.TempDir()
	packDir := writeCurrentPackFixture(t, bundle, "example-pack")
	path := filepath.Join(packDir, "pack.json")
	var manifest map[string]any
	if err := json.Unmarshal(mustReadFile(t, path), &manifest); err != nil {
		t.Fatal(err)
	}
	binding := currentFixtureResource(manifest)["bindings"].([]any)[0].(map[string]any)
	binding["capabilities"] = []any{map[string]any{
		"type": "external-host-setup",
		"external_host_setup": map[string]any{
			"tool":              "engram",
			"setup_args":        []any{"setup", "codex"},
			"managed_resources": []any{map[string]any{"kind": "skill", "id": "guide"}},
			"codex": map[string]any{
				"mcp_args":                   []any{"mcp", "--tools=agent"},
				"instructions_file":          "engram-instructions.md",
				"instructions_fingerprint":   "74176fb0847b06fb725ae8992c9a5fa12022ff347ca3ee2ef3e77c6d318d5fb3",
				"compact_prompt_file":        "engram-compact-prompt.md",
				"compact_prompt_fingerprint": "c779d9584c8ca16331ebb31a753f7fbb5bcb8193b229572a54da189ffaa97fd1",
				"marketplace_repository":     "https://github.com/Gentleman-Programming/engram.git",
				"marketplace_revision":       "main",
				"plugin":                     "engram@engram",
			},
		},
	}}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ValidatePackContent(bundle, packDir)
	if err == nil || !strings.Contains(err.Error(), "requires external requirement \"engram\"") {
		t.Fatalf("error = %v", err)
	}
}

func currentFixtureResource(manifest map[string]any) map[string]any {
	return manifest["resources"].([]any)[0].(map[string]any)
}

func copyCheckedInBundle(t *testing.T) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("..", "..", "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "bundle")
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	return destination
}

func decodeManifestMap(t *testing.T, path string) map[string]any {
	t.Helper()
	var manifest map[string]any
	if err := json.Unmarshal(mustReadFile(t, path), &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func writeManifestMap(t *testing.T, path string, manifest map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func externalHostSetupMap(t *testing.T, manifest map[string]any, kind, id, surface string) map[string]any {
	t.Helper()
	for _, value := range manifest["resources"].([]any) {
		resource := value.(map[string]any)
		if resource["kind"] != kind || resource["id"] != id {
			continue
		}
		for _, bindingValue := range resource["bindings"].([]any) {
			binding := bindingValue.(map[string]any)
			if binding["surface"] != surface {
				continue
			}
			return binding["capabilities"].([]any)[0].(map[string]any)["external_host_setup"].(map[string]any)
		}
	}
	t.Fatalf("missing external host setup for %s:%s on %s", kind, id, surface)
	return nil
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCheckedInPackTemplateUsesCurrentContract(t *testing.T) {
	bundle, err := filepath.Abs(filepath.Join("..", "..", "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	pack, err := ValidatePackContent(bundle, filepath.Join(bundle, "pack-template"))
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "example-pack" || pack.Version != "1.0.0" || pack.Selectable {
		t.Fatalf("template Pack = %#v", pack)
	}
	if len(pack.Resources) != 1 || strings.TrimSpace(pack.Resources[0].Description) == "" {
		t.Fatalf("template resources = %#v", pack.Resources)
	}
	if got, want := pack.ReadinessObligations, []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization}; !reflect.DeepEqual(got, want) {
		t.Fatalf("template readiness obligations = %#v, want %#v", got, want)
	}
}

func TestCheckedInCurrentManifestsOmitRetiredContractTerms(t *testing.T) {
	bundle, err := filepath.Abs(filepath.Join("..", "..", "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := Discover(bundle)
	if err != nil {
		t.Fatal(err)
	}
	packs, err := catalog.ListCurrent()
	if err != nil {
		t.Fatal(err)
	}
	for _, pack := range packs {
		if got, want := pack.ReadinessObligations, []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization}; !reflect.DeepEqual(got, want) {
			t.Fatalf("Pack %s readiness obligations = %#v, want %#v", pack.ID, got, want)
		}
		data := string(mustReadFile(t, filepath.Join(bundle, "packs", pack.ID, "pack.json")))
		for _, retired := range []string{"schema_version", "runtime_modes", "root_migrations", "optional-mode:", "provides_capabilities", "requires_capabilities", "capability_conflicts"} {
			if strings.Contains(data, retired) {
				t.Fatalf("Pack %s current manifest contains retired contract term %q", pack.ID, retired)
			}
		}
	}
}

func writeCurrentPackFixture(t *testing.T, bundle, id string) string {
	t.Helper()
	packDir := filepath.Join(bundle, "packs", id)
	if err := os.MkdirAll(packDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(packDir, "instructions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "instructions", "guide.md"), []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "id": "` + id + `",
  "version": "1.0.0",
  "description": "Example Pack",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": ["example-tool"],
  "resources": [
    {
      "kind": "instruction",
      "id": "guide",
      "source": "packs/` + id + `/instructions/guide.md",
	  "description": "Explains the reviewed guidance",
      "requires": [],
      "conflicts": [],
      "bindings": [
        {
          "surface": "codex",
          "projection": "instruction",
          "name": "guide",
          "invocation": "guide",
          "mode": "native",
          "sharing": "shared",
          "capabilities": [{"type": "project-instruction", "project_instruction": {"id": "guide", "source": "packs/` + id + `/instructions/guide.md"}}]
        }
      ],
      "surface_exclusions": []
    }
  ],
  "exclusions": [],
  "source_reference": {
    "repository": "https://example.com/example-pack.git",
    "revision": "v1.0.0"
  }
}`
	if err := os.WriteFile(filepath.Join(packDir, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return packDir
}

func TestValidatePortableContentRejectsDirectoryManifestIdentityMismatch(t *testing.T) {
	bundle := t.TempDir()
	writePortableFixture(t, bundle, "one", "instructions/one.md")
	manifest := filepath.Join(bundle, "packs", "one", "pack.json")
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(strings.Replace(string(data), `"id":"one"`, `"id":"other"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePortableContent(bundle); err == nil || !strings.Contains(err.Error(), "contains manifest id") {
		t.Fatalf("identity mismatch error = %v", err)
	}
}

func writePortableFixture(t *testing.T, bundle, id, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(bundle, "packs", id), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bundle, filepath.Dir(source)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, source), []byte("inert\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"` + id + `","version":"1.0.0","description":"Fixture","selectable":true,"surfaces":["codex"],"readiness_obligations":["runtime-usability","surface-authorization"],"external_requirements":[],"resources":[{"kind":"instruction","id":"guidance","source":"` + source + `","description":"Explains the reviewed guidance","requires":[],"conflicts":[],"bindings":[{"surface":"codex","projection":"instruction","name":"guidance","invocation":"guidance","mode":"native","sharing":"shared","capabilities":[{"type":"project-instruction","project_instruction":{"id":"guidance","source":"` + source + `"}}]}],"surface_exclusions":[]}],"exclusions":[]}`
	if err := os.WriteFile(filepath.Join(bundle, "packs", id, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}
