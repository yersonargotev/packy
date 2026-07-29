package capabilitypack

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// AC-01..AC-03: reconcile is a repair operation over approved intent, not a
// second selection surface.
func TestIssue294ReconcileDerivesApprovedClosureAndRejectsSelectionOrAuthorityChanges(t *testing.T) {
	root := ResourceIdentity{Kind: "instruction", ID: "root"}
	dependency := ResourceIdentity{Kind: "skill", ID: "dependency"}
	unselected := ResourceIdentity{Kind: "skill", ID: "unselected"}
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: root.Kind, ID: root.ID, Requires: []string{dependency.String()}, Bindings: issue291Bindings(root.ID)},
			{Kind: dependency.Kind, ID: dependency.ID, Permissions: []string{"filesystem-read"}, Bindings: issue291Bindings(dependency.ID)},
			{Kind: unselected.Kind, ID: unselected.ID, Permissions: []string{"network"}, Bindings: issue291Bindings(unselected.ID)},
		},
	}
	selection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{root}}
	intent := ActivationIntent{
		PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 7,
		Selection: selection,
	}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}
	inspection := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{
		{ID: root.String(), ObservedFingerprint: "old-root", DesiredFingerprint: "new-root", Action: ProjectionAction{ID: root.String()}},
		{ID: dependency.String(), ObservedFingerprint: "old-dependency", DesiredFingerprint: "new-dependency", Action: ProjectionAction{ID: dependency.String()}},
		{ID: unselected.String(), ObservedFingerprint: "old-unselected", DesiredFingerprint: "new-unselected", Action: ProjectionAction{ID: unselected.String()}},
	}}
	facade, adapter, store := reconcileFixture([]Pack{pack}, state, inspection)

	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Selection(), selection) {
		t.Fatalf("reconcile selection = %+v, want persisted %+v", plan.Selection(), selection)
	}
	graph := plan.JSONReport(true).ResourceGraph.Resources
	gotResources := make([]string, 0, len(graph))
	for _, fact := range graph {
		gotResources = append(gotResources, fact.Resource.String())
	}
	if !reflect.DeepEqual(gotResources, []string{dependency.String(), root.String()}) {
		t.Fatalf("reconcile closure = %v", gotResources)
	}
	for _, phase := range plan.Phases() {
		for _, action := range phase.Actions {
			if action.ID == unselected.String() {
				t.Fatalf("reconcile adopted unselected resource: %+v", action)
			}
		}
	}

	_, err = facade.PreviewReconcile(context.Background(), ReconcileRequest{
		PackID: pack.ID, Surface: SurfaceCodex,
		Aliases: []SurfaceAlias{{Kind: dependency.Kind, ID: dependency.ID, Name: "new-authority"}},
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "preserv") {
		t.Fatalf("reconcile alias/authority change error = %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("rejected reconcile crossed mutation boundary: actions=%d saves=%d", len(adapter.actions), len(store.saves))
	}
}

// AC-04: a projection-free promotion is still intent and must be sealed
// against the exact persisted revision.
func TestIssue294IntentOnlyPromotionRejectsStaleRevisionWithoutEffects(t *testing.T) {
	root := ResourceIdentity{Kind: "instruction", ID: "root"}
	shared := ResourceIdentity{Kind: "skill", ID: "shared"}
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: root.Kind, ID: root.ID, Requires: []string{shared.String()}, Bindings: issue291Bindings(root.ID)},
			{Kind: shared.Kind, ID: shared.ID, Bindings: issue291Bindings(shared.ID)},
		},
	}
	beforeSelection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{root}}
	afterSelection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{root, shared}}
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 4, Selection: beforeSelection}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}
	converged := SurfaceInspection{Revision: "host"}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{converged, converged}}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex, Selection: afterSelection})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Selection(), afterSelection) || plan.NoOp() {
		t.Fatalf("intent-only promotion was not sealed as a state-changing plan: selection=%+v noop=%t", plan.Selection(), plan.NoOp())
	}
	store.state.Intent.Revision++
	store.state.Intents[0].Revision++
	_, err = facade.Apply(context.Background(), ApplyRequest{
		Plan: plan, Approvals: issue294Approvals(facade, plan), Interactive: true,
	})
	if !errors.Is(err, ErrStalePlan) || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("stale intent-only apply: err=%v actions=%d saves=%d", err, len(adapter.actions), len(store.saves))
	}
}

// AC-04: projection-changing plans seal the observed host precondition too.
func TestIssue294ProjectionChangingPlanRejectsStaleObservationWithoutEffects(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 2}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}
	preview := SurfaceInspection{Revision: "host-1", Projections: []ObservedProjection{{
		ID: "instruction:guide", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide"},
	}}}
	changed := preview
	changed.Revision = "host-2"
	facade, adapter, store := reconcileFixture([]Pack{pack}, state, preview, changed)

	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: issue294Approvals(facade, plan), Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("stale projection apply: err=%v actions=%d saves=%d", err, len(adapter.actions), len(store.saves))
	}
}

func TestIssue294TechnicalFailureRecordsRecoverySubjectsAndNextCommand(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 3}
	state := ActivationState{
		Intent: intent, Intents: []ActivationIntent{intent},
		Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"pack:app:instruction:guide"}, Fingerprint: "desired"}},
	}
	drifted := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{
		ID: "instruction:guide", Exists: true, ObservedFingerprint: "drift", DesiredFingerprint: "desired",
		Action: ProjectionAction{ID: "instruction:guide", Description: "repair guide"},
	}}}
	facade, adapter, store := reconcileFixture([]Pack{pack}, state, drifted, drifted)
	adapter.applyErr = &ProjectionActionError{ID: "instruction:guide", Err: errors.New("write failed")}

	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: issue294Approvals(facade, plan), Interactive: true})
	if err == nil || store.state.Journal == nil {
		t.Fatalf("technical failure journal: err=%v state=%+v", err, store.state)
	}
	journal := store.state.Journal
	wantResources := []RecoveryAffectedResource{{Pack: "app", Resource: ResourceIdentity{Kind: "instruction", ID: "guide"}}}
	if journal.Outcome != AttemptRecoveryRequired ||
		!reflect.DeepEqual(journal.AffectedResources, wantResources) ||
		journal.ReconcileScope != ReconcileTargeted ||
		journal.FailedAction != "instruction:guide" ||
		len(journal.Completed) != 0 ||
		len(journal.NotStarted()) != 0 {
		t.Fatalf("truthful recovery context = %+v", journal)
	}
}

// AC-05..AC-06: recovery is a new preview with new consent, while the old
// attempt remains the truthful source for completed/failed/not-started facts
// and for operator/automation guidance.
func TestIssue294RecoveryPreservesTruthfulAttemptFactsAndRequiresFreshApproval(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "instruction", ID: "one"}, {Kind: "instruction", ID: "two"}, {Kind: "instruction", ID: "three"},
	}}
	history := ApplyingJournal{
		PlanID: "old-plan", PlanDigest: "old-digest", Operation: OperationReconcile, Surface: SurfaceCodex, PackID: pack.ID,
		Outcome: AttemptRecoveryRequired, Actions: []string{"instruction:one", "instruction:two", "instruction:three"},
		Completed: []string{"instruction:one"}, FailedAction: "instruction:two", FailureDetail: "disk full",
		AffectedResources: []RecoveryAffectedResource{{Pack: pack.ID, Resource: ResourceIdentity{Kind: "instruction", ID: "two"}}},
		Consumers:         []RecoveryConsumer{{Pack: "consumer", Resource: &ResourceIdentity{Kind: "instruction", ID: "root"}, Capability: "cap:app"}},
		ReconcileScope:    ReconcileTargeted,
	}
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 5}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}, Journal: &history}
	pending := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{
		{ID: "instruction:one", ObservedFingerprint: "new", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:one"}},
		{ID: "instruction:two", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:two"}},
		{ID: "instruction:three", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:three"}},
	}}
	facade, adapter, store := reconcileFixture([]Pack{pack}, state, pending, pending)

	recovery, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	attempt := recovery.HistoricalAttempt()
	if !recovery.Recovery() || recovery.ID() == history.PlanID || attempt == nil ||
		!reflect.DeepEqual(attempt.Completed, []string{"instruction:one"}) ||
		attempt.FailedAction != "instruction:two" ||
		!reflect.DeepEqual(attempt.NotStarted(), []string{"instruction:three"}) {
		t.Fatalf("fresh recovery/attempt facts = recovery:%t id:%q attempt:%+v", recovery.Recovery(), recovery.ID(), attempt)
	}
	report := recovery.JSONReport(true)
	if !report.Recovery || report.Operation != OperationReconcile || report.Pack != pack.ID || report.RecoveryGuidance == nil {
		t.Fatalf("JSON recovery origin = %+v", report)
	}
	guidance := report.RecoveryGuidance
	if guidance.OriginatingOperation != OperationReconcile ||
		!reflect.DeepEqual(guidance.AffectedResources, history.AffectedResources) ||
		!reflect.DeepEqual(guidance.Consumers, history.Consumers) ||
		guidance.NextCommand != "packy pack reconcile app --surface codex" ||
		!reflect.DeepEqual(guidance.Completed, history.Completed) ||
		guidance.FailedAction != history.FailedAction ||
		!reflect.DeepEqual(guidance.NotStarted, history.NotStarted()) {
		t.Fatalf("JSON recovery guidance = %+v", guidance)
	}
	legacyHistory := history
	legacyHistory.ReconcileScope = ""
	legacyState := state
	legacyState.Journal = &legacyHistory
	legacyFacade, _, _ := reconcileFixture([]Pack{pack}, legacyState, pending, pending)
	legacyRecovery, err := legacyFacade.PreviewReconcile(context.Background(), ReconcileRequest{Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if got := legacyRecovery.RecoveryGuidance().NextCommand; got != "packy pack reconcile --surface codex" {
		t.Fatalf("legacy surface-wide next command = %q", got)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: recovery, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("recovery without fresh approvals error = %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 || !reflect.DeepEqual(store.state.Journal, &history) {
		t.Fatalf("unapproved recovery mutated state: actions=%d saves=%d state=%+v", len(adapter.actions), len(store.saves), store.state)
	}
}

func issue294Approvals(facade Facade, plan ReconciliationPlan) []ApprovalReceipt {
	approvals := make([]ApprovalReceipt, 0, len(plan.Phases()))
	for _, phase := range plan.Phases() {
		if phase.ApprovalRequired {
			approvals = append(approvals, facade.Approve(plan, phase.Kind))
		}
	}
	return approvals
}
