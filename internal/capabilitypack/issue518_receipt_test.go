package capabilitypack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIssue518FailedApplicationDoesNotReplaceInstalledReceipt(t *testing.T) {
	ctx := context.Background()
	target := filepath.Join(t.TempDir(), "guide.md")
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	explicit := true
	state := ActivationState{
		Intent: ActivationIntent{
			PackID: "app", Version: "1.0.0", Surface: SurfaceCodex, Active: true, Revision: 1,
			Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}},
			Resources: []ResourceIdentity{{Kind: "instruction", ID: "guide"}}, Explicit: &explicit,
		},
		Ownership: []ProjectionOwnership{{
			ID: "path:" + target, ProjectionID: "instruction:guide", Target: target, Fingerprint: "old",
			PackID: "app", Surface: SurfaceCodex,
		}},
	}
	state.Intents = []ActivationIntent{state.Intent}
	if _, err := store.SaveSnapshot(ctx, SurfaceCodex, 0, state); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}

	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{
		ID: "instruction:guide", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", AdapterProvenance: "test-adapter/v1",
		Action: ProjectionAction{ID: "instruction:guide", Target: target, Content: "new", Description: "write guide", AdapterProvenance: "test-adapter/v1"},
	}}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation, observation}, applyErr: errors.New("disk full")}
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide.md", Bindings: testCapabilityBindings("guide")}}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.PreviewUpdate(ctx, UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Apply(ctx, ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if err == nil {
		t.Fatal("failed application unexpectedly succeeded")
	}
	after, readErr := os.ReadFile(store.path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Fatalf("failed application replaced installed receipt\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestIssue518UpdateRetiresOnlyRequestedPackProjectionMissingFromCurrentPack(t *testing.T) {
	oldTarget := filepath.Join(t.TempDir(), "old-guide")
	newTarget := filepath.Join(t.TempDir(), "new-guide")
	otherTarget := filepath.Join(t.TempDir(), "other-guide")
	intent := ActivationIntent{
		PackID: "app", Version: "1.0.0", Surface: SurfaceCodex, Active: true, Revision: 1,
		Selection: ResourceSelection{Mode: SelectionAll},
	}
	state := ActivationState{
		Intent: intent, Intents: []ActivationIntent{intent}, snapshotManaged: true,
		Ownership: []ProjectionOwnership{
			{
				ID: "path:" + oldTarget, ProjectionID: "skill:old-guide", Target: oldTarget, Fingerprint: "old-digest",
				PackID: "app", Surface: SurfaceCodex,
			},
			{
				ID: "path:" + otherTarget, ProjectionID: "skill:other-guide", Target: otherTarget, Fingerprint: "other-digest",
				PackID: "other", Surface: SurfaceCodex,
			},
		},
	}
	pack := Pack{
		manifestVersion: manifestSchemaV4, ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{{Kind: "skill", ID: "new-guide", Source: "new-guide", Bindings: testCapabilityBindings("new-guide")}},
	}
	adapter := &fakeSurfaceAdapter{inspect: func(transition SurfaceTransition) SurfaceInspection {
		projections := []ObservedProjection{{
			ID: "skill:new-guide", ObservedFingerprint: "missing", DesiredFingerprint: "new-digest",
			Action: ProjectionAction{ID: "skill:new-guide", Target: newTarget, Description: "install new guide"},
		}}
		for _, owner := range transition.ResidualOwnership {
			if owner.ID != "skill:old-guide" {
				continue
			}
			projections = append(projections, RemovalCandidate(ObservedProjection{
				ID: owner.ID, Exists: true, ObservedFingerprint: owner.Fingerprint, ProjectionKey: owner.PhysicalID,
				Action: ProjectionAction{ID: owner.ID, Kind: ActionSkillLink, Target: owner.Target, ProjectionKey: owner.PhysicalID},
			}, ProjectionDeleteTarget, "", "remove old guide"))
		}
		return SurfaceInspection{Revision: "host", Projections: projections}
	}}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(adapter.calls) != 1 || len(adapter.calls[0].ownership) != 1 || adapter.calls[0].ownership[0].ID != "skill:old-guide" {
		t.Fatalf("update residual ownership = %#v", adapter.calls)
	}
	if !hasPhaseActionID(plan.Phases(), ConsentReversibleLocal, "skill:new-guide") || !hasPhaseActionID(plan.Phases(), ConsentDestructiveCleanup, "skill:old-guide") {
		t.Fatalf("update phases = %#v", plan.Phases())
	}
}

func TestIssue518UpdateRequiresForceToRetireDriftedResidualProjection(t *testing.T) {
	oldTarget := filepath.Join(t.TempDir(), "old-guide")
	newTarget := filepath.Join(t.TempDir(), "new-guide")
	intent := ActivationIntent{
		PackID: "app", Version: "1.0.0", Surface: SurfaceCodex, Active: true, Revision: 1,
		Selection: ResourceSelection{Mode: SelectionAll},
	}
	state := ActivationState{
		Intent: intent, Intents: []ActivationIntent{intent}, snapshotManaged: true,
		Ownership: []ProjectionOwnership{{
			ID: "path:" + oldTarget, ProjectionID: "skill:old-guide", Target: oldTarget, Fingerprint: "receipt-digest",
			PackID: "app", Surface: SurfaceCodex,
		}},
	}
	pack := Pack{
		manifestVersion: manifestSchemaV4, ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{{Kind: "skill", ID: "new-guide", Source: "new-guide", Bindings: testCapabilityBindings("new-guide")}},
	}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{
		{
			ID: "skill:new-guide", ObservedFingerprint: "missing", DesiredFingerprint: "new-digest",
			Action: ProjectionAction{ID: "skill:new-guide", Target: newTarget, Description: "install new guide"},
		},
		RemovalCandidate(ObservedProjection{
			ID: "skill:old-guide", Exists: true, ObservedFingerprint: "broken", ProjectionKey: "path:" + oldTarget,
			Action: ProjectionAction{ID: "skill:old-guide", Kind: ActionSkillLink, Target: oldTarget, ProjectionKey: "path:" + oldTarget},
		}, ProjectionDeleteTarget, "", "remove old guide"),
	}}

	ordinary, _, _ := updateFixture([]Pack{pack}, state, observation)
	ordinaryPlan, err := ordinary.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if ordinaryPlan.Applicable() || len(ordinaryPlan.Blockers()) != 1 || hasPhaseActionID(ordinaryPlan.Phases(), ConsentDestructiveCleanup, "skill:old-guide") {
		t.Fatalf("ordinary drift plan: disposition=%s blockers=%#v phases=%#v", ordinaryPlan.Disposition(), ordinaryPlan.Blockers(), ordinaryPlan.Phases())
	}

	forced, _, _ := updateFixture([]Pack{pack}, state, observation)
	forcedPlan, err := forced.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if !forcedPlan.Applicable() || len(forcedPlan.Blockers()) != 0 || !hasPhaseActionID(forcedPlan.Phases(), ConsentDestructiveCleanup, "skill:old-guide") {
		t.Fatalf("forced drift plan: disposition=%s blockers=%#v phases=%#v", forcedPlan.Disposition(), forcedPlan.Blockers(), forcedPlan.Phases())
	}
}

func TestIssue518DistinctResourcesTargetingOnePathBlockBeforeMutation(t *testing.T) {
	target := filepath.Join(t.TempDir(), "shared.md")
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "instruction", ID: "one", Source: "one.md"},
		{Kind: "instruction", ID: "two", Source: "two.md"},
	}}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{
		{ID: "instruction:one", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:one", Target: target, Content: "same"}},
		{ID: "instruction:two", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:two", Target: target, Content: "same"}},
	}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "app", Surface: SurfaceCodex, Selection: ResourceSelection{Mode: SelectionAll}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.Blockers()) == 0 || len(plan.Phases()) != 0 {
		t.Fatalf("target collision was not blocked: disposition=%s blockers=%+v phases=%+v", plan.Disposition(), plan.Blockers(), plan.Phases())
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("target collision mutated state: actions=%+v saves=%d", adapter.actions, len(store.saves))
	}

	intent := ActivationIntent{PackID: pack.ID, Version: pack.Version, Surface: SurfaceCodex, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll}}
	deactivationStore := &fakeActivationStore{state: ActivationState{Intent: intent, Intents: []ActivationIntent{intent}, snapshotManaged: true}}
	deactivationAdapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation}}
	deactivationFacade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(deactivationStore, map[Surface]SurfaceAdapter{SurfaceCodex: deactivationAdapter}))
	deactivationPlan, err := deactivationFacade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if deactivationPlan.Applicable() || len(deactivationPlan.Blockers()) == 0 || len(deactivationPlan.Phases()) != 0 || len(deactivationAdapter.actions) != 0 || len(deactivationStore.saves) != 0 {
		t.Fatalf("deactivation target collision was not blocked before mutation: disposition=%s blockers=%+v phases=%+v", deactivationPlan.Disposition(), deactivationPlan.Blockers(), deactivationPlan.Phases())
	}
}

func TestIssue518ForceUpdateIsLimitedToReceiptOwnedTargets(t *testing.T) {
	target := filepath.Join(t.TempDir(), "guide.md")
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide.md", Bindings: testCapabilityBindings("guide")}}}
	state := ActivationState{
		Intent: ActivationIntent{PackID: "app", Version: "1.0.0", Surface: SurfaceCodex, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll}},
		Ownership: []ProjectionOwnership{{
			ID: "path:" + target, ProjectionID: "skill:guide", Target: target, Fingerprint: "receipt-digest",
			PackID: "app", Surface: SurfaceCodex,
		}},
		snapshotManaged: true,
	}
	state.Intents = []ActivationIntent{state.Intent}
	drifted := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{
		ID: "skill:guide", Exists: true, ObservedFingerprint: "user-edit", DesiredFingerprint: "catalog-current", AdapterProvenance: "test-adapter/v1",
		Action: ProjectionAction{ID: "skill:guide", Target: target, Content: "catalog-current", Description: "write guide", AdapterProvenance: "test-adapter/v1"},
	}}}

	ordinary, _, _ := updateFixture([]Pack{pack}, state, drifted)
	ordinaryPlan, err := ordinary.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if ordinaryPlan.Applicable() || len(ordinaryPlan.Phases()) != 0 {
		t.Fatalf("ordinary drift was writable: disposition=%s phases=%+v", ordinaryPlan.Disposition(), ordinaryPlan.Phases())
	}

	forced, _, _ := updateFixture([]Pack{pack}, state, drifted)
	forcedPlan, err := forced.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if !forcedPlan.Applicable() || len(forcedPlan.Phases()) != 1 || len(forcedPlan.Phases()[0].Actions) != 1 {
		t.Fatalf("receipt-owned force was not actionable: disposition=%s blockers=%+v phases=%+v", forcedPlan.Disposition(), forcedPlan.Blockers(), forcedPlan.Phases())
	}

	foreignObservation := drifted
	foreignObservation.Projections = append([]ObservedProjection(nil), drifted.Projections...)
	foreignObservation.Projections[0].Action.Target = filepath.Join(t.TempDir(), "foreign.md")
	foreign, _, _ := updateFixture([]Pack{pack}, state, foreignObservation)
	foreignPlan, err := foreign.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if foreignPlan.Applicable() || len(foreignPlan.Phases()) != 0 {
		t.Fatalf("force escaped receipt ownership: disposition=%s phases=%+v", foreignPlan.Disposition(), foreignPlan.Phases())
	}
}

func TestIssue518ForceDeactivationIsLimitedToReceiptOwnedTargets(t *testing.T) {
	target := filepath.Join(t.TempDir(), "guide")
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide", Bindings: testCapabilityBindings("guide")}}}
	state := ActivationState{
		Intent: ActivationIntent{PackID: "app", Version: "1.0.0", Surface: SurfaceCodex, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll}},
		Ownership: []ProjectionOwnership{{
			ID: "path:" + target, ProjectionID: "skill:guide", Target: target, Fingerprint: "receipt-digest",
			PackID: "app", Surface: SurfaceCodex,
		}},
		snapshotManaged: true,
	}
	state.Intents = []ActivationIntent{state.Intent}
	drifted := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{
		ID: "skill:guide", Exists: true, ObservedFingerprint: "user-edit", AdapterProvenance: "test-adapter/v1",
		Action: ProjectionAction{ID: "skill:guide", Target: target, Description: "remove guide", AdapterProvenance: "test-adapter/v1"},
	}}}

	ordinary, _, _ := deactivationFixture([]Pack{pack}, state, drifted)
	ordinaryPlan, err := ordinary.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if ordinaryPlan.Applicable() || len(ordinaryPlan.Phases()) != 0 {
		t.Fatalf("ordinary drift was removable: disposition=%s phases=%+v", ordinaryPlan.Disposition(), ordinaryPlan.Phases())
	}

	forced, _, _ := deactivationFixture([]Pack{pack}, state, drifted)
	forcedPlan, err := forced.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if !forcedPlan.Applicable() || len(forcedPlan.Phases()) != 1 || forcedPlan.Phases()[0].Kind != ConsentDestructiveCleanup {
		t.Fatalf("receipt-owned force deactivation was not actionable: disposition=%s blockers=%+v phases=%+v", forcedPlan.Disposition(), forcedPlan.Blockers(), forcedPlan.Phases())
	}

	foreignObservation := drifted
	foreignObservation.Projections = append([]ObservedProjection(nil), drifted.Projections...)
	foreignObservation.Projections[0].Action.Target = filepath.Join(t.TempDir(), "foreign")
	foreign, _, _ := deactivationFixture([]Pack{pack}, state, foreignObservation)
	foreignPlan, err := foreign.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if foreignPlan.Applicable() || len(foreignPlan.Phases()) != 0 {
		t.Fatalf("force deactivation escaped receipt ownership: disposition=%s phases=%+v", foreignPlan.Disposition(), foreignPlan.Phases())
	}
}
