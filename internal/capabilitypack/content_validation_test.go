package capabilitypack

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func currentFixtureResource(manifest map[string]any) map[string]any {
	return manifest["resources"].([]any)[0].(map[string]any)
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
  "external_requirements": ["example-tool"],
  "resources": [
    {
      "kind": "instruction",
      "id": "guide",
      "source": "packs/` + id + `/instructions/guide.md",
      "requires": [],
      "conflicts": [],
      "bindings": [
        {
          "surface": "codex",
          "projection": "instruction",
          "name": "guide",
          "invocation": "guide",
          "mode": "native",
          "sharing": "shared"
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
	manifest := `{"id":"` + id + `","version":"1.0.0","description":"Fixture","selectable":true,"surfaces":["codex"],"external_requirements":[],"resources":[{"kind":"instruction","id":"guidance","source":"` + source + `","requires":[],"conflicts":[],"bindings":[{"surface":"codex","projection":"instruction","name":"guidance","invocation":"guidance","mode":"native","sharing":"shared"}],"surface_exclusions":[]}],"exclusions":[]}`
	if err := os.WriteFile(filepath.Join(bundle, "packs", id, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}
