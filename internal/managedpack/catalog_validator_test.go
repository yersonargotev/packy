package managedpack

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateCatalogProjectAcceptsAddedAndUpdatedPacksWithIndependentVersions(t *testing.T) {
	baseline := t.TempDir()
	writeCatalogPack(t, baseline, "alpha", "1.0.0", "old alpha\n", "skills/alpha")
	writeCatalogPack(t, baseline, "stable", "2.0.0", "stable\n", "skills/stable")

	candidate := t.TempDir()
	writeCatalogPack(t, candidate, "alpha", "1.1.0", "new alpha\n", "skills/alpha")
	writeCatalogPack(t, candidate, "beta", "1.0.0", "new beta\n", "skills/beta")
	writeCatalogPack(t, candidate, "stable", "2.0.0", "stable\n", "skills/stable")

	validation, err := ValidateCatalogProject(context.Background(), candidate, baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(validation.Packs))
	for _, pack := range validation.Packs {
		got = append(got, pack.Manifest.ID+"@"+pack.Manifest.Version)
	}
	want := []string{"alpha@1.1.0", "beta@1.0.0", "stable@2.0.0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("validated Packs = %v, want %v", got, want)
	}
}

func TestValidateCatalogProjectEnforcesIndependentVersionBumps(t *testing.T) {
	baseline := t.TempDir()
	writeCatalogPack(t, baseline, "alpha", "1.0.0", "original\n", "skills/alpha")

	tests := []struct {
		name    string
		version string
		content string
		want    string
	}{
		{name: "changed content without bump", version: "1.0.0", content: "changed\n", want: "changed content requires a version greater than 1.0.0"},
		{name: "changed content with lower version", version: "0.9.0", content: "changed\n", want: "changed content requires a version greater than 1.0.0"},
		{name: "unchanged content with version bump", version: "1.1.0", content: "original\n", want: "unchanged content must retain version 1.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := t.TempDir()
			writeCatalogPack(t, candidate, "alpha", test.version, test.content, "skills/alpha")
			_, err := ValidateCatalogProject(context.Background(), candidate, baseline, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateCatalogProjectRejectsSharedDeclaredClosurePaths(t *testing.T) {
	project := t.TempDir()
	writeCatalogPack(t, project, "alpha", "1.0.0", "shared\n", "skills/shared")
	writeCatalogPack(t, project, "beta", "1.0.0", "shared\n", "skills/shared")

	_, err := ValidateCatalogProject(context.Background(), project, "", nil)
	if err == nil || !strings.Contains(err.Error(), `declared closure path "skills/shared/SKILL.md" is owned by Packs "alpha" and "beta"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateCatalogProjectReadsButNeverExecutesCatalogContent(t *testing.T) {
	project := t.TempDir()
	marker := filepath.Join(project, "executed")
	content := "#!/bin/sh\ntouch " + marker + "\n"
	writeCatalogPack(t, project, "alpha", "1.0.0", content, "skills/alpha")

	if _, err := ValidateCatalogProject(context.Background(), project, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("catalog content executed: marker error = %v", err)
	}
}

func writeCatalogPack(t *testing.T, root, id, version, content, source string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "bundle", source, "SKILL.md"), content, 0o644)
	manifest := fmt.Sprintf(`{
  "schema_version": 1,
  "id": %q,
  "version": %q,
  "description": "Fixture Pack",
  "selectable": true,
  "surfaces": ["codex"],
  "readiness_obligations": ["runtime-usability", "surface-authorization"],
  "external_requirements": [],
  "origins": [],
  "resources": [{
    "kind": "skill",
    "id": %q,
    "source": %q,
    "description": "Fixture skill",
    "requires": [],
    "conflicts": [],
    "bindings": [{
      "surface": "codex",
      "projection": "skill",
      "name": %q,
      "invocation": %q,
      "mode": "native",
      "sharing": "exclusive",
      "capabilities": []
    }],
    "surface_exclusions": []
  }]
}
`, id, version, id, source, id, "$"+id)
	writeFile(t, filepath.Join(root, "bundle", "packs", id, "pack.json"), manifest, 0o644)
}
