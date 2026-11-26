package capabilitypack_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

const catalogBoundReceipt = `{"schema_version":2,"revision":1,"receipts":[{"pack":{"id":"shared","version":"1.0.0","catalog_repository":"yersonargotev/packy-catalog","catalog_snapshot":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"surface":"agents","readiness_obligations":[],"external_requirements":[],"selection":{"mode":"all","roots":[]},"resources":[{"kind":"skill","id":"guide"}],"projections":[]}]}`

func TestFileActivationStoreBindsReceiptsToTheirCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	if err := os.WriteFile(path, []byte(catalogBoundReceipt), 0o600); err != nil {
		t.Fatal(err)
	}
	store := capabilitypack.NewFileActivationStore(path)
	state, err := store.LoadSnapshot(context.Background(), capabilitypack.SurfaceAgents)
	if err != nil {
		t.Fatal(err)
	}
	want := capabilitypack.CatalogRef{Repository: "yersonargotev/packy-catalog", Commit: strings.Repeat("a", 40)}
	if state.Intent.CatalogRef != want {
		t.Fatalf("loaded intent catalog = %+v; want %+v", state.Intent.CatalogRef, want)
	}
	if _, err := store.SaveSnapshot(context.Background(), capabilitypack.SurfaceAgents, 1, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var written struct {
		SchemaVersion int `json:"schema_version"`
		Receipts      []struct {
			Pack map[string]string `json:"pack"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}
	if written.SchemaVersion != 2 || len(written.Receipts) != 1 || written.Receipts[0].Pack["catalog_repository"] != want.Repository || written.Receipts[0].Pack["catalog_snapshot"] != want.Commit {
		t.Fatalf("saved receipts = %s", data)
	}
}

func TestFileActivationStoreParsesTheCatalogRepository(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	if err := os.WriteFile(path, []byte(strings.Replace(catalogBoundReceipt, "yersonargotev/packy-catalog", "YersonArgotev/Packy-Catalog", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := capabilitypack.NewFileActivationStore(path).LoadSnapshot(context.Background(), capabilitypack.SurfaceAgents)
	if err != nil || state.Intent.CatalogRef.Repository != "yersonargotev/packy-catalog" {
		t.Fatalf("loaded mixed-case catalog = %+v, %v", state.Intent.CatalogRef, err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(catalogBoundReceipt, "yersonargotev/packy-catalog", "yersonargotev/../catalog", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := capabilitypack.NewFileActivationStore(path).LoadSnapshot(context.Background(), capabilitypack.SurfaceAgents); err == nil || !strings.Contains(err.Error(), "is not a GitHub owner/repo") {
		t.Fatalf("store accepted an invalid catalog repository: %v", err)
	}
}

func TestFileActivationStoreRejectsEarlierReceiptsWithAdoptionGuidance(t *testing.T) {
	for name, test := range map[string]struct {
		document     string
		wantGuidance bool
	}{
		"schema version 1":   {`{"schema_version":1,"revision":1,"receipts":[{"pack":{"id":"shared","version":"1.0.0","catalog_snapshot":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"surface":"agents","readiness_obligations":[],"external_requirements":[],"selection":{"mode":"all","roots":[]},"resources":[],"projections":[]}]}`, true},
		"incomplete catalog": {strings.Replace(catalogBoundReceipt, `"catalog_repository":"yersonargotev/packy-catalog",`, "", 1), false},
	} {
		document := test.document
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "packs.json")
			if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := capabilitypack.NewFileActivationStore(path).LoadSnapshot(context.Background(), capabilitypack.SurfaceAgents)
			if err == nil {
				t.Fatal("store accepted a receipt that does not name its catalog")
			}
			if test.wantGuidance && !strings.Contains(err.Error(), "docs/catalog-adoption.md") {
				t.Fatalf("earlier receipt error names no adoption guidance: %v", err)
			}
			if data, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(data, []byte(document)) {
				t.Fatalf("rejected receipts changed: %q %v", data, readErr)
			}
		})
	}
}

func TestFileActivationStoreAcceptsAnEmptyEarlierReceiptDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"revision":4,"receipts":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := capabilitypack.NewFileActivationStore(path).LoadSnapshot(context.Background(), capabilitypack.SurfaceAgents)
	if err != nil || state.Intent.PackID != "" {
		t.Fatalf("empty earlier receipts = %+v, %v; want no installed Pack", state.Intent, err)
	}
}
