package capabilitypack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProjectLockWithCatalog(t *testing.T, ref CatalogRef) string {
	t.Helper()
	data, err := json.Marshal(ProjectLockProposal{
		SchemaVersion: projectLockSchemaVersion,
		Receipts: []installedPackReceipt{{
			Pack: installedPackIdentity{ID: "app", Version: "1.0.0", CatalogRef: ref}, Surface: SurfaceAgents,
			ReadinessObligations: []ReadinessObligation{}, ExternalRequirements: []string{},
			Selection:   ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}},
			Resources:   []ResourceIdentity{{Kind: "skill", ID: "guide"}},
			Projections: []installedProjection{{ID: "skill:guide", Target: ".agents/skills/guide", Digest: strings.Repeat("a", 64), FileMode: 0o755}},
			Sensitive:   []ProjectSensitiveDisclosure{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "packy.lock.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestReadExistingProjectLockCanonicalizesTheCatalogRepository(t *testing.T) {
	commit := strings.Repeat("a", 40)
	lock, exists, err := readExistingProjectLock(writeProjectLockWithCatalog(t, CatalogRef{Repository: "YersonArgotev/Packy-Catalog", Commit: commit}))
	if err != nil || !exists {
		t.Fatalf("readExistingProjectLock() = %v, %v", exists, err)
	}
	if want := (CatalogRef{Repository: "yersonargotev/packy-catalog", Commit: commit}); lock.Receipts[0].Pack.CatalogRef != want {
		t.Fatalf("lock catalog = %+v; want %+v", lock.Receipts[0].Pack.CatalogRef, want)
	}
	if _, _, err := readExistingProjectLock(writeProjectLockWithCatalog(t, CatalogRef{Repository: "not a repository", Commit: commit})); err == nil || !strings.Contains(err.Error(), "is not a GitHub owner/repo") {
		t.Fatalf("lock with an invalid catalog repository = %v", err)
	}
}

func TestReadExistingProjectLockRejectsAnUnpairedCatalogReference(t *testing.T) {
	for name, ref := range map[string]CatalogRef{
		"repository without snapshot": {Repository: "yersonargotev/packy-catalog"},
		"snapshot without repository": {Commit: strings.Repeat("a", 40)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := readExistingProjectLock(writeProjectLockWithCatalog(t, ref)); err == nil || !strings.Contains(err.Error(), "incomplete catalog reference") {
				t.Fatalf("lock with %s = %v", name, err)
			}
		})
	}
}
