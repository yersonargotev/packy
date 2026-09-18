package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackListJSONRepresentsAnEmptyCatalogWithAnEmptyArray(t *testing.T) {
	catalogRoot := filepath.Join(t.TempDir(), "bundle")
	if err := os.MkdirAll(filepath.Join(catalogRoot, "packs"), 0o700); err != nil {
		t.Fatal(err)
	}
	createSkillSourceAt(t, catalogRoot)
	home := t.TempDir()
	opts := Options{Env: MapEnv{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, "xdg"), "PATH": "",
	}, catalogRootOverride: catalogRoot}

	output, err := executeCommand(t, NewRootCommand(opts), "list", "--json")
	if err != nil {
		t.Fatalf("pack list --json: %v\n%s", err, output)
	}
	if output != "{\"schema_version\":1,\"report\":\"pack-list\",\"packs\":[]}\n" {
		t.Fatalf("empty report = %q", output)
	}
}
