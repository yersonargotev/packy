package capabilitypack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectActivationsFromCatalogFindsActiveActivationsOfTheCatalog(t *testing.T) {
	packyHome := t.TempDir()
	for name, state := range map[string]string{
		"one/state-acme-agents.json":    `{"schema_version":2,"state":{"pack_id":"acme","version":"1.0.0","catalog_repository":"acme/packs","catalog_snapshot":"cccccccccccccccccccccccccccccccccccccccc","surface":"agents","active":true}}`,
		"two/state-acme-claude.json":    `{"schema_version":2,"state":{"pack_id":"acme","version":"1.0.0","catalog_repository":"acme/packs","catalog_snapshot":"cccccccccccccccccccccccccccccccccccccccc","surface":"claude","active":false}}`,
		"two/state-fixture-agents.json": `{"schema_version":2,"state":{"pack_id":"fixture","version":"1.0.0","catalog_repository":"yersonargotev/packy-catalog","catalog_snapshot":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","surface":"agents","active":true}}`,
	} {
		path := filepath.Join(packyHome, "projects", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(state), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	activations, err := ProjectActivationsFromCatalog(packyHome, "acme/packs")
	if err != nil || len(activations) != 1 || activations[0].PackID != "acme" || activations[0].Surface != SurfaceAgents {
		t.Fatalf("ProjectActivationsFromCatalog() = %+v, %v", activations, err)
	}
	if activations, err := ProjectActivationsFromCatalog(t.TempDir(), "acme/packs"); err != nil || len(activations) != 0 {
		t.Fatalf("ProjectActivationsFromCatalog() without projects = %+v, %v", activations, err)
	}
}
