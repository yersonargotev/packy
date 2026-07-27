package capabilitypack

import (
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
	manifest := `{"schema_version":1,"id":"` + id + `","version":"1.0.0","provides":[],"requires":{"capabilities":[],"tools":[]},"conflicts":[],"resources":[{"kind":"instruction","id":"guidance","source":"` + source + `"}]}`
	if err := os.WriteFile(filepath.Join(bundle, "packs", id, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}
