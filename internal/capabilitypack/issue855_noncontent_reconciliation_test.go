package capabilitypack

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIssue855NonContentReconciliationSchedulesAndVerifiesAdapterAction(t *testing.T) {
	target := "/tmp/issue855-guide"
	pack := Pack{ID: "issue855", Version: "2.0.0", Surfaces: []Surface{SurfaceClaude}, Resources: []Resource{{
		Kind: "skill", ID: "guide", Source: "guide", Bindings: []Binding{{Surface: SurfaceClaude, Projection: "skill", Name: "guide"}},
	}}}
	intent := ActivationIntent{PackID: pack.ID, Version: "1.0.0", Surface: SurfaceClaude, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll}}
	state := ActivationState{
		Intent: intent, Intents: []ActivationIntent{intent}, snapshotManaged: true,
		Ownership: []ProjectionOwnership{{
			ID: "path:" + target, ProjectionID: "skill:guide", Target: target, Fingerprint: "same-tree",
			PackID: pack.ID, Surface: SurfaceClaude,
		}},
	}
	projection := ObservedProjection{
		ID: "skill:guide", ProjectionKey: "path:" + target, Exists: true, ObservedFingerprint: "same-tree", DesiredFingerprint: "same-tree", NeedsReconciliation: true,
		Action: ProjectionAction{ID: "skill:guide", Kind: "test-link", Target: target, ProjectionKey: "path:" + target, Source: "/snapshot-b/guide", Description: "retarget guide"},
	}
	converged := projection
	converged.NeedsReconciliation = false
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{
		{Revision: "source-a", Projections: []ObservedProjection{projection}},
		{Revision: "source-a", Projections: []ObservedProjection{projection}},
		{Revision: "source-b", Projections: []ObservedProjection{converged}},
	}}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceClaude: adapter}))

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: pack.ID, Surface: SurfaceClaude})
	if err != nil {
		t.Fatal(err)
	}
	if plan.NoOp() || len(plan.Phases()) != 1 || len(plan.Phases()[0].Actions) != 1 {
		t.Fatalf("non-content reconciliation was not actionable: disposition=%s phases=%+v", plan.Disposition(), plan.Phases())
	}
	result, err := facade.Apply(context.Background(), ApplyRequest{
		Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true,
	})
	if err != nil || !result.Verified {
		t.Fatalf("Apply = %#v, %v", result, err)
	}
	if len(adapter.applied) != 1 || len(adapter.applied[0]) != 1 {
		t.Fatalf("adapter actions = %+v", adapter.applied)
	}
	if len(store.saves) != 1 || len(store.saves[0].Ownership) != 1 || store.saves[0].Ownership[0].Fingerprint != "same-tree" {
		t.Fatalf("content receipt changed during identity handoff: %+v", store.saves)
	}
}

func TestIssue855NonContentReconciliationMustClearDuringVerification(t *testing.T) {
	target := "/tmp/issue855-stale-guide"
	pack := Pack{ID: "issue855-stale", Version: "2.0.0", Surfaces: []Surface{SurfaceClaude}, Resources: []Resource{{
		Kind: "skill", ID: "guide", Source: "guide", Bindings: []Binding{{Surface: SurfaceClaude, Projection: "skill", Name: "guide"}},
	}}}
	intent := ActivationIntent{PackID: pack.ID, Version: "1.0.0", Surface: SurfaceClaude, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll}}
	state := ActivationState{
		Intent: intent, Intents: []ActivationIntent{intent}, snapshotManaged: true,
		Ownership: []ProjectionOwnership{{ID: "path:" + target, ProjectionID: "skill:guide", Target: target, Fingerprint: "same-tree", PackID: pack.ID, Surface: SurfaceClaude}},
	}
	projection := ObservedProjection{
		ID: "skill:guide", ProjectionKey: "path:" + target, Exists: true, ObservedFingerprint: "same-tree", DesiredFingerprint: "same-tree", NeedsReconciliation: true,
		Action: ProjectionAction{ID: "skill:guide", Kind: "test-link", Target: target, ProjectionKey: "path:" + target, Source: "/snapshot-b/guide", Description: "retarget guide"},
	}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "source-a", Projections: []ObservedProjection{projection}}}}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceClaude: adapter}))
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: pack.ID, Surface: SurfaceClaude})
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{
		Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true,
	})
	if !errors.Is(err, ErrVerificationFailed) || !strings.Contains(err.Error(), "adapter reconciliation still pending") {
		t.Fatalf("uncleared reconciliation fact passed verification: %v", err)
	}
	if len(store.saves) != 0 {
		t.Fatal("failed reconciliation advanced the receipt")
	}
}

func TestIssue855NonContentReconciliationIsReportedAsOwnedDrift(t *testing.T) {
	target := "/tmp/issue855-status-guide"
	pack := Pack{ID: "issue855-status", Version: "2.0.0"}
	projection := ObservedProjection{
		ID: "skill:guide", ProjectionKey: "path:" + target, Exists: true,
		ObservedFingerprint: "same-tree", DesiredFingerprint: "same-tree", NeedsReconciliation: true,
		Action: ProjectionAction{ID: "skill:guide", Target: target, ProjectionKey: "path:" + target},
	}
	owners := []ProjectionOwnership{{
		ID: "path:" + target, ProjectionID: projection.ID, Target: target, Fingerprint: "same-tree",
		PackID: pack.ID, Surface: SurfaceClaude,
	}}
	projections, summary := deriveProjectionStatus(pack.ID, []ObservedProjection{projection}, owners, composition{requested: pack, surface: SurfaceClaude})
	if len(projections) != 1 || projections[0].Health != ProjectionDrifted || projections[0].Owner != "packy" || summary.Drifted != 1 || summary.Verified != 0 {
		t.Fatalf("status treated pending identity reconciliation as converged: projections=%+v summary=%+v", projections, summary)
	}
}
