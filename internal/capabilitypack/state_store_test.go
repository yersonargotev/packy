package capabilitypack

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFileActivationStoreMigratesLegacyDocumentsToCanonicalAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	legacy := `{"schema_version":2,"activations":[{"schema_version":1,"intent":{"pack_id":"matty","surface":"codex","version":"1.0.0","active":true,"revision":1}}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewFileActivationStore(path)
	state, err := store.Load(context.Background(), SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 3 || state.Intent.Aliases == nil || len(state.Intent.Aliases) != 0 || state.Intent.Selection.Mode != SelectionAll || state.Intent.Selection.Roots == nil {
		t.Fatalf("migrated state = %+v", state)
	}
	if err := store.Save(context.Background(), SurfaceCodex, 1, state); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document["schema_version"] != float64(5) || document["revision"] != float64(1) || !strings.Contains(string(data), `"aliases": []`) || !strings.Contains(string(data), `"mode": "all"`) {
		t.Fatalf("document = %s", data)
	}
}

func TestFileActivationStorePersistsAliasesInCanonicalOrder(t *testing.T) {
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	state := ActivationState{Intent: ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Active: true, Revision: 1, Aliases: []SurfaceAlias{{Kind: "command", ID: "review", Name: "addy-review"}, {Kind: "agent", ID: "reviewer", Name: "addy-reviewer"}}}}
	if err := store.Save(context.Background(), SurfaceCodex, 0, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	want := []SurfaceAlias{{Kind: "agent", ID: "reviewer", Name: "addy-reviewer"}, {Kind: "command", ID: "review", Name: "addy-review"}}
	if !reflect.DeepEqual(loaded.Intent.Aliases, want) {
		t.Fatalf("aliases = %+v, want %+v", loaded.Intent.Aliases, want)
	}
}

func TestFileActivationStoreRoundTripsCanonicalProviderChoicesAndRole(t *testing.T) {
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	requiredOnly := false
	resource := ResourceIdentity{Kind: "skill", ID: "storage"}
	state := ActivationState{Intent: ActivationIntent{
		PackID: "consumer", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 1, Explicit: &requiredOnly,
		ProviderChoices: []ProviderChoice{
			{Capability: "cap:z", ProviderPack: "legacy"},
			{Capability: "cap:a", ProviderPack: "provider", ProviderResource: &resource},
		},
	}}
	if err := store.Save(context.Background(), SurfaceCodex, 0, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Intent.Explicit == nil || *loaded.Intent.Explicit || len(loaded.Intent.ProviderChoices) != 2 ||
		loaded.Intent.ProviderChoices[0].Capability != "cap:a" || loaded.Intent.ProviderChoices[0].ProviderResource == nil ||
		*loaded.Intent.ProviderChoices[0].ProviderResource != resource {
		t.Fatalf("provider intent round-trip = %#v", loaded.Intent)
	}
}

func TestFileActivationStorePreservesLegacyFingerprintWithoutInventingReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	legacy := `{"schema_version":5,"revision":1,"activations":[{"schema_version":3,"intent":{"pack_id":"engram","surface":"codex","version":"1","active":true,"revision":1,"aliases":[],"selection":{"mode":"all","roots":[]}},"external_effects":[{"id":"external:engram:setup:codex","fingerprint":"legacy-only"}]}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := NewFileActivationStore(path).Load(context.Background(), SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.External) != 1 || state.External[0].Receipt != nil {
		t.Fatalf("legacy fingerprint acquired reversal authority: %#v", state.External)
	}
}

func TestFileActivationStoreRoundTripsCanonicalExternalReceipt(t *testing.T) {
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	state := ActivationState{Intent: ActivationIntent{PackID: "engram", Surface: SurfaceCodex, Version: "1", Active: true, Revision: 1}, External: []ExternalEffect{{
		ID: "external:engram:setup:codex", Fingerprint: "sealed", Receipt: &ExternalEffectReceipt{
			SchemaVersion: 1, EffectID: "external:engram:setup:codex", EffectFingerprint: "sealed", Surface: SurfaceCodex,
			Contributors:  []string{"surface:codex:pack:engram:external:engram"},
			Contributions: []ExternalContribution{{ID: "external_setup:engram:codex:mcp", ObservedFingerprint: "exact", AdapterProvenance: "codex-engram-setup/v1/mcp"}},
			Reversal:      ExternalReversalContract{SchemaVersion: 1, Consent: ConsentDestructiveCleanup, AuthorityLimits: []string{"configuration only"}},
		},
	}}}
	if err := store.Save(context.Background(), SurfaceCodex, 0, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.External) != 1 || loaded.External[0].Receipt == nil || loaded.External[0].Receipt.Reversal.Consent != ConsentDestructiveCleanup {
		t.Fatalf("receipt round trip = %#v", loaded.External)
	}
	loaded.External[0].Receipt.SchemaVersion = 99
	if err := store.Save(context.Background(), SurfaceCodex, loaded.Intent.Revision, loaded); err == nil || !strings.Contains(err.Error(), "unsupported receipt schema") {
		t.Fatalf("future receipt schema error = %v", err)
	}
}

func TestFileActivationStoreRejectsInvalidAliases(t *testing.T) {
	for _, tc := range []struct {
		name    string
		aliases []SurfaceAlias
	}{
		{"duplicate identity", []SurfaceAlias{{Kind: "command", ID: "review", Name: "one"}, {Kind: "command", ID: "review", Name: "two"}}},
		{"unsupported kind", []SurfaceAlias{{Kind: "asset", ID: "reference", Name: "ref"}}},
		{"invalid identity", []SurfaceAlias{{Kind: "skill", ID: "Bad ID", Name: "good"}}},
		{"empty name", []SurfaceAlias{{Kind: "agent", ID: "reviewer", Name: ""}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
			state := ActivationState{Intent: ActivationIntent{PackID: "addy", Revision: 1, Aliases: tc.aliases}}
			if err := store.Save(context.Background(), SurfaceCodex, 0, state); err == nil || !strings.Contains(err.Error(), "alias") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestFileActivationStoreRejectsFutureContainedStateVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	data := `{"schema_version":3,"activations":[{"schema_version":999,"intent":{"surface":"codex","aliases":[]}}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewFileActivationStore(path).Load(context.Background(), SurfaceCodex)
	if err == nil || !strings.Contains(err.Error(), "unsupported activation schema_version 999") {
		t.Fatalf("error = %v", err)
	}
}

func TestFileActivationStoreExplainsCompareAndSwapStaleRevision(t *testing.T) {
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	state := ActivationState{SchemaVersion: 1, Intent: ActivationIntent{Revision: 1}}
	if err := store.Save(context.Background(), SurfaceCodex, 0, state); err != nil {
		t.Fatal(err)
	}

	err := store.Save(context.Background(), SurfaceCodex, 0, ActivationState{SchemaVersion: 1, Intent: ActivationIntent{Revision: 2}})
	if !errors.Is(err, ErrStalePlan) || !strings.Contains(err.Error(), "changed from 0 to 1 before persistence") || !strings.Contains(err.Error(), "rerun activation") {
		t.Fatalf("error = %v", err)
	}
}

func TestFileActivationStorePreservesIndependentSurfaceState(t *testing.T) {
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode} {
		state, err := store.LoadSnapshot(context.Background(), surface)
		if err != nil {
			t.Fatal(err)
		}
		state.Intent = ActivationIntent{PackID: "matty", Surface: surface, Active: true, Revision: 1}
		state.Ownership = append(state.Ownership, ProjectionOwnership{ID: "surface:" + string(surface) + ":instruction:matty-guidance", ProjectionID: "instruction:matty-guidance", Contributors: []string{qualifyContributor(surface, "pack:matty:instruction:matty-guidance")}, Fingerprint: string(surface)})
		if _, err := store.SaveSnapshot(context.Background(), surface, state.documentRevision, state); err != nil {
			t.Fatal(err)
		}
	}
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode} {
		state, err := store.Load(context.Background(), surface)
		if err != nil {
			t.Fatal(err)
		}
		if state.Intent.Surface != surface || len(state.Ownership) != 2 {
			t.Fatalf("%s state = %+v", surface, state)
		}
	}
}

func TestFileActivationStoreSnapshotCASDetectsOtherSurfaceRace(t *testing.T) {
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	codex, err := store.LoadSnapshot(context.Background(), SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	opencode, err := store.LoadSnapshot(context.Background(), SurfaceOpenCode)
	if err != nil {
		t.Fatal(err)
	}
	codex.Intent = ActivationIntent{Surface: SurfaceCodex, Revision: 1}
	if _, err := store.SaveSnapshot(context.Background(), SurfaceCodex, codex.documentRevision, codex); err != nil {
		t.Fatal(err)
	}
	opencode.Intent = ActivationIntent{Surface: SurfaceOpenCode, Revision: 1}
	if _, err := store.SaveSnapshot(context.Background(), SurfaceOpenCode, opencode.documentRevision, opencode); !errors.Is(err, ErrStalePlan) || !strings.Contains(err.Error(), "state revision changed") {
		t.Fatalf("cross-surface stale save error = %v", err)
	}
}

func TestFileActivationStoreMigratesV4OwnershipWithoutMergingPrivateSurfaceIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	v4 := `{"schema_version":4,"activations":[` +
		`{"schema_version":3,"intent":{"pack_id":"app","surface":"codex","active":true,"revision":1,"aliases":[],"selection":{"mode":"all","roots":[]}},"ownership":[{"id":"skill:shared-name","contributors":["pack:app:skill:one"],"fingerprint":"codex"}]},` +
		`{"schema_version":3,"intent":{"pack_id":"app","surface":"opencode","active":true,"revision":1,"aliases":[],"selection":{"mode":"all","roots":[]}},"ownership":[{"id":"skill:shared-name","contributors":["pack:app:skill:one"],"fingerprint":"opencode"}]}` + `]}`
	if err := os.WriteFile(path, []byte(v4), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := NewFileActivationStore(path).LoadSnapshot(context.Background(), SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Ownership) != 2 || state.Ownership[0].ID != "surface:codex:skill:shared-name" || state.Ownership[1].ID != "surface:opencode:skill:shared-name" {
		t.Fatalf("v4 private ownership migration = %+v", state.Ownership)
	}
	if state.Ownership[0].Contributors[0] != "surface:codex:pack:app:skill:one" {
		t.Fatalf("v4 contributor was not surface-qualified: %+v", state.Ownership[0])
	}
}
