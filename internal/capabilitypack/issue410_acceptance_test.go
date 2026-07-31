package capabilitypack

import (
	"context"
	"reflect"
	"testing"
)

func TestIssue410FullDeactivatePersistsInactiveIntentAndDriftedOwnership(t *testing.T) {
	pack := issue410Pack("guide")
	selection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "instruction", ID: "guide"}}}
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 7, Selection: selection}
	owner := ProjectionOwnership{
		ID: "instruction:guide", Contributors: []string{"pack:app:instruction:guide"},
		Fingerprint: "packy-exact", AdapterProvenance: "codex-instructions-v2",
	}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}, Ownership: []ProjectionOwnership{owner}}
	drifted := issue410RemovalObservation("host-drifted", map[string]string{"instruction:guide": "operator-edit"})
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, drifted, drifted, drifted)

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable() || len(plan.Phases()) != 0 || len(plan.PendingHumanActions()) == 0 {
		t.Fatalf("drift-preserving deactivation = applicable:%v phases:%+v blockers:%+v pending:%v", plan.Applicable(), plan.Phases(), plan.Blockers(), plan.PendingHumanActions())
	}
	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || store.state.Intent.Active || store.state.Intent.Version != pack.Version || !reflect.DeepEqual(store.state.Intent.Selection, selection) {
		t.Fatalf("inactive intent did not retain its deletion context: result=%+v intent=%+v", result, store.state.Intent)
	}
	if !reflect.DeepEqual(store.state.Ownership, []ProjectionOwnership{owner}) {
		t.Fatalf("drifted residual lost ownership authority: got=%+v want=%+v", store.state.Ownership, owner)
	}
	if len(adapter.actions) != 0 {
		t.Fatalf("drifted projection was mutated: %+v", adapter.actions)
	}
}

func TestIssue410PhysicallyPartialDeactivateRetiresExactAndRetainsDriftedOwnership(t *testing.T) {
	pack := issue410Pack("drifted", "exact")
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 3, Selection: ResourceSelection{Mode: SelectionAll}}
	driftedOwner := ProjectionOwnership{ID: "instruction:drifted", Contributors: []string{"pack:app:instruction:drifted"}, Fingerprint: "drifted-exact", AdapterProvenance: "codex-instructions-v2"}
	exactOwner := ProjectionOwnership{ID: "instruction:exact", Contributors: []string{"pack:app:instruction:exact"}, Fingerprint: "exact", AdapterProvenance: "codex-instructions-v2"}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}, Ownership: []ProjectionOwnership{driftedOwner, exactOwner}}
	before := issue410RemovalObservation("host-before", map[string]string{"instruction:drifted": "operator-edit", "instruction:exact": "exact"})
	after := issue410RemovalObservation("host-after", map[string]string{"instruction:drifted": "operator-edit", "instruction:exact": ""})
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, before, before, after)

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable() || len(plan.Phases()) != 1 || len(plan.Phases()[0].Actions) != 1 || plan.Phases()[0].Actions[0].ID != "instruction:exact" {
		t.Fatalf("partial physical cleanup plan = applicable:%v phases:%+v blockers:%+v", plan.Applicable(), plan.Phases(), plan.Blockers())
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentDestructiveCleanup)}, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if store.state.Intent.Active || !reflect.DeepEqual(store.state.Ownership, []ProjectionOwnership{driftedOwner}) {
		t.Fatalf("partial physical cleanup state = intent:%+v ownership:%+v", store.state.Intent, store.state.Ownership)
	}
	if len(adapter.actions) != 1 || adapter.actions[0].ID != "instruction:exact" {
		t.Fatalf("cleanup actions = %+v", adapter.actions)
	}
}

func TestIssue410RepeatedInactiveDeactivateDeletesNowExactResidualWithoutReactivation(t *testing.T) {
	pack := issue410Pack("guide")
	selection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "instruction", ID: "guide"}}}
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: false, Revision: 9, Selection: selection}
	owner := ProjectionOwnership{ID: "instruction:guide", Contributors: []string{"pack:app:instruction:guide"}, Fingerprint: "packy-exact", AdapterProvenance: "codex-instructions-v2"}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}, Ownership: []ProjectionOwnership{owner}}
	present := issue410RemovalObservation("host-present", map[string]string{"instruction:guide": "packy-exact"})
	absent := issue410RemovalObservation("host-absent", map[string]string{"instruction:guide": ""})
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, present, present, absent)

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable() || len(plan.Phases()) != 1 || plan.Phases()[0].Actions[0].ID != owner.ID {
		t.Fatalf("inactive residual cleanup plan = applicable:%v phases:%+v blockers:%+v", plan.Applicable(), plan.Phases(), plan.Blockers())
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentDestructiveCleanup)}, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if store.state.Intent.Active || store.state.Intent.Version != pack.Version || !reflect.DeepEqual(store.state.Intent.Selection, selection) {
		t.Fatalf("repeated deactivation changed inactive intent: %+v", store.state.Intent)
	}
	if len(store.state.Ownership) != 0 || len(adapter.actions) != 1 || adapter.actions[0].ID != owner.ID {
		t.Fatalf("residual was not deleted and retired: ownership=%+v actions=%+v", store.state.Ownership, adapter.actions)
	}
}

func TestIssue410UnmanagedLookalikeNeverGainsOwnershipDuringDeactivate(t *testing.T) {
	pack := issue410Pack("guide")
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 2, Selection: ResourceSelection{Mode: SelectionAll}}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}
	lookalike := issue410RemovalObservation("host-lookalike", map[string]string{"instruction:guide": "pack-catalog-fingerprint"})
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, lookalike, lookalike, lookalike)

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable() || len(plan.Phases()) != 0 {
		t.Fatalf("unmanaged lookalike plan = applicable:%v phases:%+v blockers:%+v", plan.Applicable(), plan.Phases(), plan.Blockers())
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if store.state.Intent.Active || len(store.state.Ownership) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("unmanaged lookalike gained lifecycle authority: intent=%+v ownership=%+v actions=%+v", store.state.Intent, store.state.Ownership, adapter.actions)
	}
}

func TestIssue410StatusDistinguishesInactiveCleanResidualAndRecoveryRequired(t *testing.T) {
	pack := issue410Pack("guide")
	inactive := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: false, Revision: 5, Selection: ResourceSelection{Mode: SelectionAll}}
	owner := ProjectionOwnership{ID: "instruction:guide", Contributors: []string{"pack:app:instruction:guide"}, Fingerprint: "packy-exact", AdapterProvenance: "codex-instructions-v2"}
	drifted := issue410RemovalObservation("host-drifted", map[string]string{"instruction:guide": "operator-edit"})

	tests := []struct {
		name        string
		state       ActivationState
		observation SurfaceInspection
		want        PackLifecycleState
	}{
		{name: "inactive-clean", state: ActivationState{Intent: inactive, Intents: []ActivationIntent{inactive}}, observation: SurfaceInspection{Revision: "clean"}, want: PackLifecycleInactiveClean},
		{name: "inactive-with-residuals", state: ActivationState{Intent: inactive, Intents: []ActivationIntent{inactive}, Ownership: []ProjectionOwnership{owner}}, observation: drifted, want: PackLifecycleInactiveWithResiduals},
		{name: "recovery-required", state: ActivationState{Intent: inactive, Intents: []ActivationIntent{inactive}, Ownership: []ProjectionOwnership{owner}, Journal: &ApplyingJournal{PlanID: "failed-deactivation", PackID: pack.ID, Surface: SurfaceCodex, Operation: OperationDeactivate, Outcome: AttemptRecoveryRequired}}, observation: drifted, want: PackLifecycleRecoveryRequired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			facade, _, store := deactivationFixture([]Pack{pack}, tc.state, tc.observation)
			report, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex})
			if err != nil {
				t.Fatal(err)
			}
			entry := report.Entries[0]
			if entry.LifecycleState != tc.want || report.JSONReport(true).Entries[0].LifecycleState != tc.want {
				t.Fatalf("lifecycle state = entry:%q json:%q want:%q", entry.LifecycleState, report.JSONReport(true).Entries[0].LifecycleState, tc.want)
			}
			if tc.want == PackLifecycleInactiveWithResiduals && (entry.Intent.Active || entry.Projections.Drifted != 1 || len(entry.ProjectionDetails) != 1 || entry.ProjectionDetails[0].Owner != "packy") {
				t.Fatalf("inactive residual status lost evidence: %+v", entry)
			}
			if tc.want == PackLifecycleRecoveryRequired && (entry.LatestAttempt == nil || entry.LatestAttempt.Outcome != string(AttemptRecoveryRequired)) {
				t.Fatalf("recovery status lost attempt evidence: %+v", entry)
			}
			if len(store.saves) != 0 {
				t.Fatalf("status mutated lifecycle state: saves=%d", len(store.saves))
			}
		})
	}
}

func issue410Pack(resources ...string) Pack {
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "app", Version: "1.2.3", Surfaces: []Surface{SurfaceCodex}}
	for _, id := range resources {
		pack.Resources = append(pack.Resources, Resource{Kind: "instruction", ID: id, Source: id, Bindings: []Binding{{Surface: SurfaceCodex, Projection: "instruction", Name: id, Mode: "native", Sharing: "exclusive"}}})
	}
	return pack
}

func issue410RemovalObservation(revision string, present map[string]string) SurfaceInspection {
	inspection := SurfaceInspection{Revision: revision}
	for _, id := range []string{"instruction:drifted", "instruction:exact", "instruction:guide"} {
		observed, included := present[id]
		if !included {
			continue
		}
		inspection.Projections = append(inspection.Projections, ObservedProjection{
			ID: id, Goal: ProjectionAbsent, Exists: observed != "", ObservedFingerprint: observed,
			Action: ProjectionAction{ID: id, Mode: ProjectionDeleteTarget, Description: "remove " + id, AdapterProvenance: "codex-instructions-v2"},
		})
	}
	return inspection
}
