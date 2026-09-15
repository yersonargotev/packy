package capabilitypack

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveIntentPackAtUsesExactCurrentSnapshotContract(t *testing.T) {
	snapshotID := strings.Repeat("a", 40)
	catalog := Catalog{
		snapshotID: snapshotID,
		packs:      []Pack{{ID: "current", Version: "2.0.0", CatalogSnapshot: snapshotID}},
	}

	pack, err := catalog.ResolveIntentPackAt(context.Background(), "current", "2.0.0", snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "current" || pack.Version != "2.0.0" || pack.CatalogSnapshot != snapshotID {
		t.Fatalf("ResolveIntentPackAt() = %#v", pack)
	}

	_, err = catalog.ResolveIntentPackAt(context.Background(), "current", "1.0.0", snapshotID)
	if err == nil || !strings.Contains(err.Error(), "only catalog-current version 2.0.0 is available") {
		t.Fatalf("version mismatch error = %v", err)
	}
}

func TestIntentPackResolverUsesLoadedCurrentContractWithoutCatalogObservation(t *testing.T) {
	snapshotID := strings.Repeat("a", 40)
	catalog := Catalog{
		snapshotID: snapshotID,
		packs:      []Pack{{ID: "current", Version: "2.0.0", CatalogSnapshot: snapshotID}},
		validateSource: func(context.Context) error {
			t.Fatal("current ownership resolution re-observed the catalog")
			return nil
		},
	}
	resolver := catalog.IntentPackResolver()

	pack, err := resolver(context.Background(), "current", "2.0.0", snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "current" || pack.Version != "2.0.0" || pack.CatalogSnapshot != snapshotID {
		t.Fatalf("resolver() = %#v", pack)
	}
}

func TestIntentPackResolverRequiresExactCurrentContract(t *testing.T) {
	snapshotID := strings.Repeat("a", 40)
	resolver := (Catalog{
		snapshotID: snapshotID,
		packs:      []Pack{{ID: "current", Version: "2.0.0", CatalogSnapshot: snapshotID}},
	}).IntentPackResolver()

	for name, contract := range map[string][2]string{
		"missing version":  {"", snapshotID},
		"wrong version":    {"1.0.0", snapshotID},
		"missing snapshot": {"2.0.0", ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolver(context.Background(), "current", contract[0], contract[1])
			if err == nil {
				t.Fatal("resolver accepted an inexact current contract")
			}
		})
	}
}

func TestIntentPackResolverAcceptsExactSyntheticContract(t *testing.T) {
	resolver := (Catalog{packs: []Pack{{ID: "synthetic", Version: "1.0.0"}}}).IntentPackResolver()
	pack, err := resolver(context.Background(), "synthetic", "1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "synthetic" || pack.Version != "1.0.0" {
		t.Fatalf("resolver() = %#v", pack)
	}
}

func TestResolveIntentPackAtLoadsExactRetainedSnapshot(t *testing.T) {
	currentSnapshot := strings.Repeat("a", 40)
	retainedSnapshot := strings.Repeat("b", 40)
	retainedBundle := filepath.Join(t.TempDir(), "bundle")
	writeCurrentPackFixture(t, retainedBundle, "retained")
	catalog := Catalog{
		snapshotID: currentSnapshot,
		packs:      []Pack{{ID: "retained", Version: "2.0.0", CatalogSnapshot: currentSnapshot}},
		resolveSnapshot: func(_ context.Context, snapshotID string) (string, error) {
			if snapshotID != retainedSnapshot {
				t.Fatalf("snapshotID = %q; want %q", snapshotID, retainedSnapshot)
			}
			return retainedBundle, nil
		},
	}

	pack, err := catalog.IntentPackResolver()(context.Background(), "retained", "1.0.0", retainedSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != "1.0.0" || pack.CatalogSnapshot != retainedSnapshot {
		t.Fatalf("ResolveIntentPackAt() = %#v", pack)
	}
	if got := pack.Resources[0].CatalogRootOr("fallback"); got != retainedBundle {
		t.Fatalf("CatalogRootOr() = %q; want retained bundle %q", got, retainedBundle)
	}
}

func TestResourceCatalogRootOrFallsBackForSyntheticResource(t *testing.T) {
	if got := (Resource{}).CatalogRootOr("synthetic-root"); got != "synthetic-root" {
		t.Fatalf("CatalogRootOr() = %q", got)
	}
}

func TestRetainedCatalogRejectsReceiptWithoutSnapshotIdentity(t *testing.T) {
	catalog := Catalog{snapshotID: strings.Repeat("a", 40)}
	_, err := catalog.resolveIntentPackAt(context.Background(), "legacy", "1.0.0", "")
	if err == nil || !strings.Contains(err.Error(), "predates Catalog Snapshots") || !strings.Contains(err.Error(), "docs/catalog-adoption.md") || strings.Contains(err.Error(), "reset") {
		t.Fatalf("resolveIntentPackAt() error = %v", err)
	}
}
