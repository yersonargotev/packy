package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func testAcmeRef(commit string) CatalogRef {
	return CatalogRef{Repository: "acme/packs", Commit: strings.Repeat(commit, 40)}
}

func TestCatalogSpansEverySelectedSnapshot(t *testing.T) {
	official, acme, retained := testCatalogRef("a"), testAcmeRef("c"), testAcmeRef("b")
	officialRoot, acmeRoot, retainedRoot := filepath.Join(t.TempDir(), "bundle"), filepath.Join(t.TempDir(), "bundle"), filepath.Join(t.TempDir(), "bundle")
	writeCurrentPackFixture(t, officialRoot, "alpha")
	writeCurrentPackFixture(t, acmeRoot, "beta")
	writeCurrentPackFixture(t, retainedRoot, "beta")
	roots := map[CatalogRef]string{official: officialRoot, acme: acmeRoot, retained: retainedRoot}
	catalog, err := DiscoverRetainedForDurableIntents(context.Background(), []SelectedSnapshot{{Root: officialRoot, Ref: official}, {Root: acmeRoot, Ref: acme}}, func(_ context.Context, ref CatalogRef) (string, error) {
		if root, ok := roots[ref]; ok {
			return root, nil
		}
		return "", errors.New("unknown snapshot")
	})
	if err != nil {
		t.Fatal(err)
	}

	packs, err := catalog.ListCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 2 || packs[0].ID != "alpha" || packs[0].CatalogRef != official || packs[1].ID != "beta" || packs[1].CatalogRef != acme {
		t.Fatalf("ListCurrent() = %+v", packs)
	}
	beta, err := catalog.Show(context.Background(), "beta")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := beta.Resources[0].CatalogRootOr("fallback"), filepath.Join(acmeRoot, "packs", "beta"); got != want {
		t.Fatalf("beta resource root = %q, want %q", got, want)
	}
	resolve := catalog.IntentPackResolver()
	for id, ref := range map[string]CatalogRef{"alpha": official, "beta": acme} {
		if pack, err := resolve(context.Background(), id, "1.0.0", ref); err != nil || pack.CatalogRef != ref {
			t.Fatalf("resolver(%s, %s) = %+v, %v", id, ref.label(), pack.CatalogRef, err)
		}
	}
	if pack, err := catalog.ResolveIntentPackAt(context.Background(), "beta", "1.0.0", retained); err != nil || pack.CatalogRef != retained {
		t.Fatalf("ResolveIntentPackAt(retained acme snapshot) = %+v, %v", pack.CatalogRef, err)
	}
	if _, err := catalog.ResolveIntentPackAt(context.Background(), "alpha", "1.0.0", acme); err == nil || !strings.Contains(err.Error(), "is not in Catalog Snapshot acme/packs@") {
		t.Fatalf("ResolveIntentPackAt(alpha from acme) = %v", err)
	}
	if _, err := catalog.ResolveIntentPackAt(context.Background(), "gamma", "1.0.0", CatalogRef{Repository: "gone/packs", Commit: official.Commit}); err == nil || !strings.Contains(err.Error(), "catalog gone/packs, which is not registered") || !strings.Contains(err.Error(), "packy catalog add gone/packs") {
		t.Fatalf("ResolveIntentPackAt(unregistered catalog) = %v", err)
	}
}

func TestCatalogRejectsAPackIDInTwoSelectedSnapshots(t *testing.T) {
	officialRoot, acmeRoot := filepath.Join(t.TempDir(), "bundle"), filepath.Join(t.TempDir(), "bundle")
	writeCurrentPackFixture(t, officialRoot, "shared")
	writeCurrentPackFixture(t, acmeRoot, "shared")
	resolver := func(_ context.Context, ref CatalogRef) (string, error) {
		if ref.Repository == "acme/packs" {
			return acmeRoot, nil
		}
		return officialRoot, nil
	}
	_, err := DiscoverRetainedForDurableIntents(context.Background(), []SelectedSnapshot{{Root: officialRoot, Ref: testCatalogRef("a")}, {Root: acmeRoot, Ref: testAcmeRef("c")}}, resolver)
	if err == nil || !strings.Contains(err.Error(), `Pack "shared" is in both catalog yersonargotev/packy-catalog and catalog acme/packs`) {
		t.Fatalf("Discover over a Pack ID in two catalogs = %v", err)
	}
}

func TestUpdateLooksOnlyInTheReceiptCatalog(t *testing.T) {
	ctx := context.Background()
	official, acme, acmeCurrent := testCatalogRef("a"), testAcmeRef("c"), testAcmeRef("d")
	retainedRoot := filepath.Join(t.TempDir(), "bundle")
	writeCurrentPackFixture(t, retainedRoot, "app")
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	explicit := true
	intent := ActivationIntent{PackID: "app", Version: "1.0.0", CatalogRef: acme, Surface: SurfaceAgents, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Explicit: &explicit}
	if _, err := store.SaveSnapshot(ctx, SurfaceAgents, 0, ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}); err != nil {
		t.Fatal(err)
	}
	pack := Pack{ID: "app", Version: "2.0.0", CatalogRef: official, Surfaces: []Surface{SurfaceAgents}, Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide.md", Bindings: testCapabilityBindings("guide")}}}
	resolveSnapshot := func(_ context.Context, ref CatalogRef) (string, error) {
		if ref != acme {
			return "", errors.New("unknown snapshot")
		}
		return retainedRoot, nil
	}
	facade := NewFacade(Catalog{snapshots: []SelectedSnapshot{{Ref: official}, {Ref: acmeCurrent}}, packs: []Pack{pack}, resolveSnapshot: resolveSnapshot}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceAgents: &fakeSurfaceAdapter{}}))

	_, err := facade.PreviewUpdate(ctx, UpdateRequest{PackID: "app", Surface: SurfaceAgents})
	if err == nil || !strings.Contains(err.Error(), "installed from catalog acme/packs, and updates look only in that catalog") || !strings.Contains(err.Error(), "packy deactivate app --surface agents") {
		t.Fatalf("update over a Pack ID that moved to another catalog = %v", err)
	}
	if _, err := facade.Preview(ctx, ActivationRequest{PackID: "app", Surface: SurfaceAgents}); err == nil || !strings.Contains(err.Error(), "to install it from yersonargotev/packy-catalog, run `packy deactivate app --surface agents`") {
		t.Fatalf("activate over a Pack ID that moved to another catalog = %v", err)
	}
	status, err := facade.ActiveStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Entries) != 1 || status.Entries[0].InspectionFailed || status.Entries[0].CatalogCurrent || status.Entries[0].UpdateAvailable || status.Entries[0].Pack.CatalogRef != acme {
		t.Fatalf("status follows the Pack ID into another catalog: %+v", status.Entries)
	}
}

func TestActivationRefusesAReceiptWithoutACatalog(t *testing.T) {
	ctx := context.Background()
	official := testCatalogRef("a")
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	explicit := true
	intent := ActivationIntent{PackID: "app", Version: "1.0.0", Surface: SurfaceAgents, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Explicit: &explicit}
	if _, err := store.SaveSnapshot(ctx, SurfaceAgents, 0, ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}); err != nil {
		t.Fatal(err)
	}
	pack := Pack{ID: "app", Version: "1.0.0", CatalogRef: official, Surfaces: []Surface{SurfaceAgents}, Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide.md", Bindings: testCapabilityBindings("guide")}}}
	facade := NewFacade(Catalog{snapshots: []SelectedSnapshot{{Ref: official}}, packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceAgents: &fakeSurfaceAdapter{}}))
	if _, err := facade.Preview(ctx, ActivationRequest{PackID: "app", Surface: SurfaceAgents}); err == nil || !strings.Contains(err.Error(), "names no catalog") {
		t.Fatalf("activate over a receipt without a catalog = %v", err)
	}
	if _, err := facade.PreviewUpdate(ctx, UpdateRequest{PackID: "app", Surface: SurfaceAgents}); err == nil || !strings.Contains(err.Error(), "names no catalog") {
		t.Fatalf("update over a receipt without a catalog = %v", err)
	}
}
