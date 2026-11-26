package catalogstore_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/catalogstore"
)

func TestProvidingFindsThePackInEveryRegisteredCatalog(t *testing.T) {
	store := catalogstore.New(t.TempDir(), catalogSource{
		official.String(): validRelease(t, strings.Repeat("a", 40), validManifest("example", "1.2.3")),
		"acme/packs":      catalogRelease(t, "acme/packs", "acme/packs", strings.Repeat("b", 40), validManifest("tools", "1.0.0")),
	})
	if _, err := store.AcquireLatest(context.Background(), official); err != nil {
		t.Fatal(err)
	}
	acme := mustRepository(t, "acme/packs")
	if _, err := store.Register(context.Background(), catalogstore.Registration{Repository: acme, Trust: catalogstore.Attested{Workflow: catalogstore.DefaultWorkflow}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ pack, snapshotID, repository, commit string }{
		{"tools", "", "acme/packs", strings.Repeat("b", 40)},
		{"example", "", "yersonargotev/packy-catalog", strings.Repeat("a", 40)},
		{"tools", strings.Repeat("b", 40), "acme/packs", strings.Repeat("b", 40)},
		{"missing", "", "yersonargotev/packy-catalog", strings.Repeat("a", 40)},
	} {
		snapshot, err := store.Providing(want.pack, want.snapshotID)
		if err != nil || snapshot.Index.Source.Repository != want.repository || snapshot.ID != want.commit {
			t.Fatalf("Providing(%q, %q) = %s@%s, %v; want %s@%s", want.pack, want.snapshotID, snapshot.Index.Source.Repository, snapshot.ID, err, want.repository, want.commit)
		}
	}
	if _, err := store.Providing("tools", strings.Repeat("c", 40)); err == nil {
		t.Fatal("Providing() accepted a snapshot that no catalog retains")
	}
}

func TestProvidingRefusesASnapshotThatSeveralCatalogsRetain(t *testing.T) {
	shared := strings.Repeat("b", 40)
	source := catalogSource{
		official.String(): validRelease(t, strings.Repeat("a", 40), validManifest("example", "1.2.3")),
		"acme/packs":      catalogRelease(t, "acme/packs", "acme/packs", shared, validManifest("tools", "1.0.0")),
	}
	store := catalogstore.New(t.TempDir(), source)
	attested := catalogstore.Attested{Workflow: catalogstore.DefaultWorkflow}
	acme, beta := mustRepository(t, "acme/packs"), mustRepository(t, "beta/packs")
	if _, err := store.Register(context.Background(), catalogstore.Registration{Repository: acme, Trust: attested}); err != nil {
		t.Fatal(err)
	}
	source["acme/packs"] = catalogRelease(t, "acme/packs", "acme/packs", strings.Repeat("c", 40), validManifest("other", "1.0.0"))
	if _, err := store.AcquireLatest(context.Background(), acme); err != nil {
		t.Fatal(err)
	}
	source["beta/packs"] = catalogRelease(t, "beta/packs", "beta/packs", shared, validManifest("tools", "2.0.0"))
	if _, err := store.Register(context.Background(), catalogstore.Registration{Repository: beta, Trust: attested}); err != nil {
		t.Fatal(err)
	}
	if snapshot, err := store.Providing("tools", ""); err != nil || snapshot.Index.Source.Repository != "beta/packs" {
		t.Fatalf("Providing(tools, selected) = %s, %v", snapshot.Index.Source.Repository, err)
	}
	_, err := store.Providing("tools", shared)
	if !errors.Is(err, catalogstore.ErrAmbiguousPack) || !strings.Contains(err.Error(), "acme/packs@"+shared+", beta/packs@"+shared) {
		t.Fatalf("Providing(tools, %s) over a commit that two catalogs retain = %v", shared, err)
	}
}
