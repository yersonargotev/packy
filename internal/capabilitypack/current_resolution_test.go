package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/catalogtransaction"
)

func testCatalogRef(commit string) CatalogRef {
	return CatalogRef{Repository: "yersonargotev/packy-catalog", Commit: strings.Repeat(commit, 40)}
}

func TestResolveIntentPackAtUsesExactCurrentSnapshotContract(t *testing.T) {
	ref := testCatalogRef("a")
	catalog := Catalog{
		snapshots: []SelectedSnapshot{{Ref: ref}},
		packs:     []Pack{{ID: "current", Version: "2.0.0", CatalogRef: ref}},
	}

	pack, err := catalog.ResolveIntentPackAt(context.Background(), "current", "2.0.0", ref)
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "current" || pack.Version != "2.0.0" || pack.CatalogRef != ref {
		t.Fatalf("ResolveIntentPackAt() = %#v", pack)
	}

	_, err = catalog.ResolveIntentPackAt(context.Background(), "current", "1.0.0", ref)
	if err == nil || !strings.Contains(err.Error(), "only catalog-current version 2.0.0 is available") {
		t.Fatalf("version mismatch error = %v", err)
	}
}

func TestIntentPackResolverUsesLoadedCurrentContractWithoutCatalogObservation(t *testing.T) {
	ref := testCatalogRef("a")
	catalog := Catalog{
		snapshots: []SelectedSnapshot{{Ref: ref}},
		packs:     []Pack{{ID: "current", Version: "2.0.0", CatalogRef: ref}},
		validateSource: func(context.Context) error {
			t.Fatal("current ownership resolution re-observed the catalog")
			return nil
		},
	}
	resolver := catalog.IntentPackResolver()

	pack, err := resolver(context.Background(), "current", "2.0.0", ref)
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "current" || pack.Version != "2.0.0" || pack.CatalogRef != ref {
		t.Fatalf("resolver() = %#v", pack)
	}
}

func TestIntentPackResolverRequiresExactCurrentContract(t *testing.T) {
	ref := testCatalogRef("a")
	resolver := (Catalog{
		snapshots: []SelectedSnapshot{{Ref: ref}},
		packs:     []Pack{{ID: "current", Version: "2.0.0", CatalogRef: ref}},
	}).IntentPackResolver()

	type contract struct {
		version string
		ref     CatalogRef
	}
	for name, contract := range map[string]contract{
		"missing version":  {"", ref},
		"wrong version":    {"1.0.0", ref},
		"missing catalog":  {"2.0.0", CatalogRef{}},
		"other repository": {"2.0.0", CatalogRef{Repository: "someone/catalog", Commit: ref.Commit}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolver(context.Background(), "current", contract.version, contract.ref)
			if err == nil {
				t.Fatal("resolver accepted an inexact current contract")
			}
		})
	}
}

func TestIntentPackResolverAcceptsExactSyntheticContract(t *testing.T) {
	resolver := (Catalog{packs: []Pack{{ID: "synthetic", Version: "1.0.0"}}}).IntentPackResolver()
	pack, err := resolver(context.Background(), "synthetic", "1.0.0", CatalogRef{})
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != "synthetic" || pack.Version != "1.0.0" {
		t.Fatalf("resolver() = %#v", pack)
	}
}

func TestIntentPackResolversDoNotNestLocksAcrossRetainedSnapshots(t *testing.T) {
	snapshotA := testCatalogRef("a")
	snapshotB := testCatalogRef("b")
	bundleA := filepath.Join(t.TempDir(), "bundle")
	bundleB := filepath.Join(t.TempDir(), "bundle")
	writeCurrentPackFixture(t, bundleA, "retained")
	writeCurrentPackFixture(t, bundleB, "retained")

	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	resolver := func(expectedID CatalogRef, targetBundle string) SnapshotResolver {
		return func(ctx context.Context, ref CatalogRef) (string, error) {
			if ref == snapshotA && expectedID == snapshotB {
				return bundleA, nil
			}
			if ref == snapshotB && expectedID == snapshotA {
				return bundleB, nil
			}
			if ref != expectedID {
				return "", errors.New("unexpected snapshot identity")
			}
			ready <- struct{}{}
			select {
			case <-release:
				return targetBundle, nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	catalogA, err := DiscoverRetainedForDurableIntents(context.Background(), []SelectedSnapshot{{Root: bundleA, Ref: snapshotA}}, resolver(snapshotB, bundleB))
	if err != nil {
		t.Fatal(err)
	}
	catalogB, err := DiscoverRetainedForDurableIntents(context.Background(), []SelectedSnapshot{{Root: bundleB, Ref: snapshotB}}, resolver(snapshotA, bundleA))
	if err != nil {
		t.Fatal(err)
	}

	guardA, err := catalogtransaction.Acquire(context.Background(), filepath.Dir(bundleA))
	if err != nil {
		t.Fatal(err)
	}
	defer guardA.Release()
	guardB, err := catalogtransaction.Acquire(context.Background(), filepath.Dir(bundleB))
	if err != nil {
		t.Fatal(err)
	}
	defer guardB.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errorsBySnapshot := make(chan error, 2)
	var started sync.WaitGroup
	started.Add(2)
	for _, request := range []struct {
		catalog Catalog
		ref     CatalogRef
	}{{catalogA, snapshotB}, {catalogB, snapshotA}} {
		go func() {
			started.Done()
			started.Wait()
			pack, resolveErr := request.catalog.IntentPackResolver()(ctx, "retained", "1.0.0", request.ref)
			if resolveErr == nil && pack.CatalogRef != request.ref {
				resolveErr = errors.New("resolved the wrong retained snapshot")
			}
			errorsBySnapshot <- resolveErr
		}()
	}
	<-ready
	<-ready
	close(release)
	for range 2 {
		if err := <-errorsBySnapshot; err != nil {
			t.Fatalf("cross-snapshot resolution failed: %v", err)
		}
	}
}

func TestResolveIntentPackAtLoadsExactRetainedSnapshot(t *testing.T) {
	currentSnapshot := testCatalogRef("a")
	retainedSnapshot := testCatalogRef("b")
	retainedBundle := filepath.Join(t.TempDir(), "bundle")
	writeCurrentPackFixture(t, retainedBundle, "retained")
	catalog := Catalog{
		snapshots: []SelectedSnapshot{{Ref: currentSnapshot}},
		packs:     []Pack{{ID: "retained", Version: "2.0.0", CatalogRef: currentSnapshot}},
		resolveSnapshot: func(_ context.Context, ref CatalogRef) (string, error) {
			if ref != retainedSnapshot {
				t.Fatalf("ref = %+v; want %+v", ref, retainedSnapshot)
			}
			return retainedBundle, nil
		},
	}

	pack, err := catalog.IntentPackResolver()(context.Background(), "retained", "1.0.0", retainedSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != "1.0.0" || pack.CatalogRef != retainedSnapshot {
		t.Fatalf("ResolveIntentPackAt() = %#v", pack)
	}
	wantPackRoot := filepath.Join(retainedBundle, "packs", "retained")
	if got := pack.Resources[0].CatalogRootOr("fallback"); got != wantPackRoot {
		t.Fatalf("CatalogRootOr() = %q; want retained Pack root %q", got, wantPackRoot)
	}
	_, err = catalog.IntentPackResolver()(context.Background(), "retained", "", retainedSnapshot)
	if err == nil || !strings.Contains(err.Error(), "no exact retained ownership contract") {
		t.Fatalf("snapshot resolver accepted missing receipt version: %v", err)
	}
}

func TestResourceCatalogRootOrFallsBackForSyntheticResource(t *testing.T) {
	if got := (Resource{}).CatalogRootOr("synthetic-root"); got != "synthetic-root" {
		t.Fatalf("CatalogRootOr() = %q", got)
	}
}

func TestResolveIntentPackAtKeysRetainedSnapshotsByRepository(t *testing.T) {
	current := testCatalogRef("a")
	sameCommitElsewhere := CatalogRef{Repository: "someone/catalog", Commit: current.Commit}
	bundle := filepath.Join(t.TempDir(), "bundle")
	writeCurrentPackFixture(t, bundle, "retained")
	var requested []CatalogRef
	catalog := Catalog{
		snapshots: []SelectedSnapshot{{Ref: current}, {Ref: CatalogRef{Repository: sameCommitElsewhere.Repository, Commit: strings.Repeat("b", 40)}}},
		packs:     []Pack{{ID: "retained", Version: "2.0.0", CatalogRef: current}},
		resolveSnapshot: func(_ context.Context, ref CatalogRef) (string, error) {
			requested = append(requested, ref)
			return bundle, nil
		},
	}

	pack, err := catalog.ResolveIntentPackAt(context.Background(), "retained", "1.0.0", sameCommitElsewhere)
	if err != nil {
		t.Fatal(err)
	}
	if pack.CatalogRef != sameCommitElsewhere || len(requested) != 1 || requested[0] != sameCommitElsewhere {
		t.Fatalf("pack = %+v, resolved %+v; want the retained snapshot of %s", pack.CatalogRef, requested, sameCommitElsewhere.Repository)
	}
}

func TestRetainedCatalogRejectsReceiptWithoutCatalogRef(t *testing.T) {
	catalog := Catalog{snapshots: []SelectedSnapshot{{Ref: testCatalogRef("a")}}}
	_, err := catalog.resolveIntentPackAt(context.Background(), "legacy", "1.0.0", CatalogRef{})
	if err == nil || !strings.Contains(err.Error(), "names no catalog") || !strings.Contains(err.Error(), "docs/catalog-adoption.md") || strings.Contains(err.Error(), "reset") {
		t.Fatalf("resolveIntentPackAt() error = %v", err)
	}
}

func TestActivationRefusesAReceiptFromAnotherCatalog(t *testing.T) {
	ctx := context.Background()
	official := testCatalogRef("a")
	elsewhere := CatalogRef{Repository: "someone/catalog", Commit: official.Commit}
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	explicit := true
	intent := ActivationIntent{PackID: "app", Version: "1.0.0", CatalogRef: elsewhere, Surface: SurfaceAgents, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Explicit: &explicit}
	if _, err := store.SaveSnapshot(ctx, SurfaceAgents, 0, ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}); err != nil {
		t.Fatal(err)
	}
	pack := Pack{ID: "app", Version: "1.0.0", CatalogRef: official, Surfaces: []Surface{SurfaceAgents}, Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide.md", Bindings: testCapabilityBindings("guide")}}}
	facade := NewFacade(Catalog{snapshots: []SelectedSnapshot{{Ref: official}}, packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceAgents: &fakeSurfaceAdapter{}}))

	if _, err := facade.Preview(ctx, ActivationRequest{PackID: "app", Surface: SurfaceAgents}); err == nil || !strings.Contains(err.Error(), "installed from catalog someone/catalog") {
		t.Fatalf("activate over a receipt from another catalog = %v", err)
	}
	if _, err := facade.PreviewUpdate(ctx, UpdateRequest{PackID: "app", Surface: SurfaceAgents}); err == nil || !strings.Contains(err.Error(), "installed from catalog someone/catalog") {
		t.Fatalf("update over a receipt from another catalog = %v", err)
	}
}
