package skillaccess

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/cataloglayout"
)

func TestDeclaredFilesMatchesTheCanonicalRepositoryOfTheIndex(t *testing.T) {
	commit := strings.Repeat("a", 40)
	root := t.TempDir()
	index := cataloglayout.CatalogSnapshotIndex{SchemaVersion: 3, Source: cataloglayout.CatalogSnapshotSource{Repository: "Acme/Packs", Commit: commit}, Packs: []cataloglayout.CatalogSnapshotPack{{ID: "fixture", Files: []cataloglayout.FileRecord{{Path: "pack.json"}}}}}
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "catalog-index.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := declaredFiles(root, capabilitypack.CatalogRef{Repository: "acme/packs", Commit: commit}, "fixture", nil)
	if err != nil || len(files) != 1 {
		t.Fatalf("declaredFiles() = %v, %v; want the pack.json record of acme/packs", files, err)
	}
	if _, err := declaredFiles(root, capabilitypack.CatalogRef{Repository: "someone/packs", Commit: commit}, "fixture", nil); err == nil {
		t.Fatal("declaredFiles accepted an index of another catalog")
	}
}
