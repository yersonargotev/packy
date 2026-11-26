package cli

import (
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/catalogstore"
)

func TestSnapshotCatalogRefUsesTheCanonicalRepository(t *testing.T) {
	commit := strings.Repeat("a", 40)
	ref, err := snapshotCatalogRef(catalogstore.Snapshot{Commit: commit, Index: cataloglayout.CatalogSnapshotIndex{Source: cataloglayout.CatalogSnapshotSource{Repository: "YersonArgotev/Packy-Catalog", Commit: commit}}})
	if err != nil || ref.Repository != "yersonargotev/packy-catalog" || ref.Commit != commit {
		t.Fatalf("snapshotCatalogRef() = %+v, %v; want yersonargotev/packy-catalog@%s", ref, err, commit)
	}
}
