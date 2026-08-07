package packsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

const portableManifestV4Fixture = `{
  "schema_version": 4,
  "id": "example",
  "version": "1.0.0",
  "surfaces": ["codex"],
  "provides": [],
  "requires": {"capabilities": [], "tools": []},
  "conflicts": [],
  "root_migrations": [],
  "resources": [{
    "kind": "skill",
    "id": "example",
    "source": "skills/example.md",
    "requires": [],
    "conflicts": [],
    "notices": [],
    "provides_capabilities": [],
    "requires_capabilities": [],
    "requires_tools": [],
    "capability_conflicts": [],
    "bindings": [{
      "surface": "codex",
      "projection": "skill",
      "name": "example",
      "invocation": "$example",
      "mode": "native",
      "sharing": "exclusive"
    }],
    "surface_exclusions": [],
    "runtime_modes": [{
      "id": "local",
      "role": "primary",
      "requirements": [],
      "authorities": [],
      "effects": [],
      "fallback": {"kind": "none"},
      "on_unavailable": "fail_before_effects"
    }]
  }],
  "contract": {"exclusions": []}
}`

func TestLoadManifestsAcceptsCanonicalPortableManifestV4(t *testing.T) {
	root := t.TempDir()
	manifestPath := runtimeManifestPath(t, root)
	encoded := canonicalPortableManifestV4(t, root)
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	first, firstHash, err := loadManifests(root)
	if err != nil {
		t.Fatal(err)
	}
	_, secondHash, err := loadManifests(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest := first["example"]
	if manifest.SchemaVersion != 4 || manifest.ID != "example" || manifest.Version != "1.0.0" {
		t.Fatalf("manifest identity changed: %#v", manifest)
	}
	if len(manifest.Resources) != 1 || manifest.Resources[0] != (manifestResource{Kind: "skill", ID: "example", Source: "skills/example.md"}) {
		t.Fatalf("manifest resources changed: %#v", manifest.Resources)
	}
	if firstHash == "" || firstHash != secondHash {
		t.Fatalf("manifest-set hash is not non-empty and deterministic: first=%q second=%q", firstHash, secondHash)
	}
}

func TestLoadManifestsObservesCurrentCatalogIdentityWithoutLegacyValidation(t *testing.T) {
	root := t.TempDir()
	manifestPath := runtimeManifestPath(t, root)
	current := `{"id":"example","version":"1.0.0","description":"Example","selectable":true,"surfaces":["codex"],"external_requirements":[],"resources":[],"exclusions":[]}`
	if err := os.WriteFile(manifestPath, []byte(current), 0o600); err != nil {
		t.Fatal(err)
	}

	manifests, _, err := loadManifests(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest := manifests["example"]
	if manifest.ID != "example" || manifest.Version != "1.0.0" || manifest.SchemaVersion != 4 {
		t.Fatalf("legacy synchronization current identity = %#v", manifest)
	}
}

func TestLoadManifestsRejectsUnsupportedPortableManifestVersion(t *testing.T) {
	root := t.TempDir()
	encoded := canonicalPortableManifestV4(t, root)
	unsupported := strings.Replace(string(encoded), `"schema_version": 4`, `"schema_version": 5`, 1)
	if err := os.WriteFile(runtimeManifestPath(t, root), []byte(unsupported), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := loadManifests(root); err == nil || !strings.Contains(err.Error(), "invalid or duplicate runtime manifest") {
		t.Fatalf("error = %v, want unsupported runtime manifest rejection", err)
	}
}

func TestLoadManifestsPreservesPortableManifestV4SemanticValidation(t *testing.T) {
	root := t.TempDir()
	encoded := canonicalPortableManifestV4(t, root)
	invalid := strings.Replace(string(encoded), `"role": "primary"`, `"role": "secondary"`, 1)
	if err := os.WriteFile(runtimeManifestPath(t, root), []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := loadManifests(root); err == nil || !strings.Contains(err.Error(), "disagrees with capability-pack contract") {
		t.Fatalf("error = %v, want capability-pack semantic rejection", err)
	}
}

func canonicalPortableManifestV4(t *testing.T, root string) []byte {
	t.Helper()
	fixturePath := filepath.Join(t.TempDir(), "pack.json")
	if err := os.WriteFile(fixturePath, []byte(portableManifestV4Fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	pack, err := capabilitypack.LoadPortableManifest(fixturePath, filepath.Join(root, "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := capabilitypack.EncodePortableManifestV4(pack)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func runtimeManifestPath(t *testing.T, root string) string {
	t.Helper()
	manifestPath := filepath.Join(root, "bundle", "packs", "example", "pack.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	return manifestPath
}
