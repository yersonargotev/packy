package capabilitypack

import (
	"context"
	"testing"
)

func TestIssue761NoOpApplyReportsSelectedPackProjectionCount(t *testing.T) {
	pack := Pack{
		ID: "issue761-no-op", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{{
			Kind: "skill", ID: "guide", Source: "guide", Description: "Synthetic guide",
			Requires: []string{}, Conflicts: []string{}, Bindings: testCapabilityBindings("guide"), SurfaceExclusions: []SurfaceExclusion{},
		}},
		Contract: Contract{OptionalModes: []OptionalMode{}},
	}
	explicit := true
	intent := ActivationIntent{
		PackID: pack.ID, Version: pack.Version, Surface: SurfaceCodex, Active: true, Revision: 1,
		Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}},
		Resources: []ResourceIdentity{{Kind: "skill", ID: "guide"}}, Explicit: &explicit,
	}
	state := ActivationState{
		SchemaVersion: 3, Intent: intent, Intents: []ActivationIntent{intent}, snapshotManaged: true,
		Ownership: []ProjectionOwnership{{
			ID: "path:/tmp/issue761-guide", ProjectionID: "skill:guide", Target: "/tmp/issue761-guide", Fingerprint: "exact",
			PackID: pack.ID, Surface: SurfaceCodex,
		}},
	}
	observation := SurfaceInspection{Revision: "codex-v1", Projections: []ObservedProjection{{
		ID: "skill:guide", ProjectionKey: "path:/tmp/issue761-guide", Exists: true, ObservedFingerprint: "exact", DesiredFingerprint: "exact",
		Action: ProjectionAction{ID: "skill:guide", Target: "/tmp/issue761-guide", ProjectionKey: "path:/tmp/issue761-guide"},
	}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation}}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex, Selection: ResourceSelection{Mode: SelectionAll}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NoOp() {
		t.Fatalf("converged activation plan = %s, want no-op", plan.Disposition())
	}
	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.Projections != 1 {
		t.Fatalf("no-op Apply result = %#v, want one selected-Pack projection", result)
	}
	if len(store.saves) != 0 || len(adapter.applied) != 0 {
		t.Fatalf("no-op Apply mutated state: saves=%d applications=%d", len(store.saves), len(adapter.applied))
	}
}
