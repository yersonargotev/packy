package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func updateFixture(packs []Pack, state ActivationState, observations ...SurfaceInspection) (Facade, *fakeSurfaceAdapter, *fakeActivationStore) {
	adapter := &fakeSurfaceAdapter{observations: observations}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: packs}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	return facade, adapter, store
}

func TestProductionCatalogRejectsUnsupportedVersionGapBeforePlanning(t *testing.T) {
	workflowPackID := "mat" + "ty"
	pack := Pack{manifestVersion: manifestSchemaV3, ID: workflowPackID, Version: "3.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v3"}}}
	state := ActivationState{Intent: ActivationIntent{PackID: workflowPackID, Surface: SurfaceCodex, Version: "1.0.0", Active: true}}
	store := &fakeActivationStore{state: state}
	adapter := &fakeSurfaceAdapter{}
	facade := NewFacade(Catalog{packs: []Pack{pack}, enforceUpdateRoutes: true}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	_, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: workflowPackID, Surface: SurfaceCodex})
	if err == nil || !strings.Contains(err.Error(), "no supported update route from 1.0.0 to 3.0.0") {
		t.Fatalf("error = %v", err)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("unsupported gap reached host adapter: %#v", adapter.calls)
	}
}

func TestMattyThreeToCurrentUpdateRetiresOnlyExactOwnedInstructionsOnEverySurface(t *testing.T) {
	bundleRoot := filepath.Join("..", "..", "bundle")
	catalog, err := DiscoverForDurableIntents(bundleRoot)
	if err != nil {
		t.Fatal(err)
	}
	current, err := catalog.Show("matty")
	if err != nil {
		t.Fatal(err)
	}
	for _, surface := range []Surface{SurfaceClaude, SurfaceCodex, SurfaceOpenCode} {
		t.Run(string(surface), func(t *testing.T) {
			intent := ActivationIntent{PackID: "matty", Surface: surface, Version: "3.0.0", Active: true, Revision: 4}
			state := ActivationState{
				Intent:  intent,
				Intents: []ActivationIntent{intent},
				Ownership: []ProjectionOwnership{
					{ID: "instruction:matty-guidance", Contributors: []string{"matty"}, Fingerprint: "guidance-exact"},
					{ID: "instruction:matty-workflow-conventions", Contributors: []string{"matty"}, Fingerprint: "conventions-exact"},
				},
			}
			inspection := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{
				{ID: "instruction:matty-guidance", Exists: true, ObservedFingerprint: "guidance-exact", Action: ProjectionAction{ID: "instruction:matty-guidance", Mode: ProjectionRemoveContent}},
				{ID: "instruction:matty-workflow-conventions", Exists: true, ObservedFingerprint: "conventions-exact", Action: ProjectionAction{ID: "instruction:matty-workflow-conventions", Mode: ProjectionRemoveContent}},
			}}
			adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{inspection}}
			store := &fakeActivationStore{state: state}
			facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{surface: adapter}))

			plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "matty", Surface: surface})
			if err != nil {
				t.Fatal(err)
			}
			cleanup := phaseActions(plan.phases, ConsentDestructiveCleanup)
			if plan.OldVersion() != "3.0.0" || plan.Pack().Version != current.Version || !plan.Applicable() || len(cleanup) != 2 || len(plan.PendingHumanActions()) != 0 {
				t.Fatalf("exact retirement plan = %#v", plan.JSONReport(true))
			}
			if len(adapter.calls) != 1 || len(adapter.calls[0].prior.Resources) != 25 || adapter.calls[0].prior.Resources[0].ID != "matty-guidance" || adapter.calls[0].desired.Version != current.Version {
				t.Fatalf("adapter update transition = %#v", adapter.calls)
			}

			inspection.Projections[0].ObservedFingerprint = "operator-drift"
			adapter.observations = []SurfaceInspection{inspection}
			drifted, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "matty", Surface: surface})
			if err != nil {
				t.Fatal(err)
			}
			if got := phaseActions(drifted.phases, ConsentDestructiveCleanup); len(got) != 1 || got[0].ID != "instruction:matty-workflow-conventions" {
				t.Fatalf("drift cleanup actions = %#v", got)
			}
			if drifted.Applicable() || len(drifted.blockers) != 1 || drifted.blockers[0].Kind != "ownership" || drifted.blockers[0].Subject != "instruction:matty-guidance" || !strings.Contains(drifted.blockers[0].Detail, "preserving existing") {
				t.Fatalf("drift preservation blockers = %#v", drifted.blockers)
			}

			inspection.Projections[0].ObservedFingerprint = "guidance-exact"
			for _, tc := range []struct {
				name  string
				owner *ProjectionOwnership
			}{
				{name: "unowned"},
				{name: "foreign", owner: &ProjectionOwnership{ID: "instruction:matty-guidance", Contributors: []string{"foreign-pack"}, Fingerprint: "guidance-exact"}},
				{name: "shared", owner: &ProjectionOwnership{ID: "instruction:matty-guidance", Contributors: []string{"matty", "foreign-pack"}, Fingerprint: "guidance-exact"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					guardedState := state
					guardedState.Ownership = guardedState.Ownership[1:]
					if tc.owner != nil {
						guardedState.Ownership = append(guardedState.Ownership, *tc.owner)
					}
					guardedAdapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{inspection}}
					guardedFacade := NewFacade(catalog, WithActivation(&fakeActivationStore{state: guardedState}, map[Surface]SurfaceAdapter{surface: guardedAdapter}))
					guarded, err := guardedFacade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "matty", Surface: surface})
					if err != nil {
						t.Fatal(err)
					}
					if got := phaseActions(guarded.phases, ConsentDestructiveCleanup); len(got) != 1 || got[0].ID != "instruction:matty-workflow-conventions" {
						t.Fatalf("guarded cleanup actions = %#v", got)
					}
					if guarded.Applicable() || len(guarded.blockers) != 1 || guarded.blockers[0].Subject != "instruction:matty-guidance" || !strings.Contains(guarded.blockers[0].Detail, "preserving existing") {
						t.Fatalf("guarded blockers = %#v", guarded.blockers)
					}
				})
			}
		})
	}
}

func TestUpdatePlansCatalogCurrentAndPersistsTargetBeforeEffects(t *testing.T) {
	pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
	pending := SurfaceInspection{Revision: "host-1", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide", Description: "write v2"}}}}
	verified := pending
	verified.Revision = "host-2"
	verified.Projections = append([]ObservedProjection(nil), pending.Projections...)
	verified.Projections[0].ObservedFingerprint = "new"
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "old"}}}
	facade, adapter, store := updateFixture([]Pack{pack}, state, pending, pending, verified)
	events := []string{}
	adapter.events, store.events = &events, &events

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Operation() != OperationUpdate || plan.OldVersion() != "1.0.0" || plan.Pack().Version != "2.0.0" || plan.IntentRevision() != 4 {
		t.Fatalf("update facts = operation %s, %s -> %s, revision %d", plan.Operation(), plan.OldVersion(), plan.Pack().Version, plan.IntentRevision())
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events[:2], []string{"persist", "effects"}) || store.saves[0].Intent.Version != "2.0.0" || store.saves[0].Journal == nil {
		t.Fatalf("ordering/state = %v %+v", events, store.saves[0])
	}
	if store.state.Journal != nil || store.state.Ownership[0].Fingerprint != "new" {
		t.Fatalf("final state = %+v", store.state)
	}
}

func TestUpdateIncludesNewDependencyAndRetainsUnchangedSharedProjection(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"dep"}}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
		{ID: "dep", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"dep"}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
	}
	obs := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:shared", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:shared"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}, Ownership: []ProjectionOwnership{{ID: "instruction:shared", Contributors: []string{"app"}, Fingerprint: "same"}}}
	facade, adapter, _ := updateFixture(packs, state, obs)

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	activations := plan.Activations()
	if len(activations) != 2 || activations[1].Pack.ID != "dep" || activations[1].Role != ActivationRequired {
		t.Fatalf("activations = %+v", activations)
	}
	retained := plan.RetainedProjections()
	if len(retained) != 1 || !reflect.DeepEqual(retained[0].Contributors, []string{"pack:app:instruction:shared", "pack:dep:instruction:shared"}) || len(plan.Phases()) != 0 || plan.NoOp() {
		t.Fatalf("retained/plan = %+v phases=%+v noop=%v", retained, plan.Phases(), plan.NoOp())
	}
	if len(adapter.actions) != 0 {
		t.Fatal("unchanged shared projection was rewritten")
	}
}

func TestCatalogCurrentUpdateIsNoOpOnlyWhenConverged(t *testing.T) {
	pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
	converged := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:guide"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "2.0.0", Active: true, Revision: 7}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "same"}}}
	facade, _, store := updateFixture([]Pack{pack}, state, converged)
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || !plan.NoOp() {
		t.Fatalf("plan noop=%v err=%v", plan.NoOp(), err)
	}
	if len(store.saves) != 0 {
		t.Fatal("no-op persisted state")
	}
}

func TestUpdateRejectsStaleCatalogAndExactPlanApproval(t *testing.T) {
	pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
	obs := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 1}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "old"}}}
	facade, adapter, store := updateFixture([]Pack{pack}, state, obs)
	plan, _ := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	facade.catalog.packs[0].Version = "3.0.0"

	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("stale update err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
	}
}

func TestUpdateBlocksIncompatibleNewContribution(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "new"}}}, {ID: "other", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "other"}}}}
	obs := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:shared", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:shared"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3}, Intents: []ActivationIntent{{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true}, {PackID: "other", Surface: SurfaceCodex, Version: "1.0.0", Active: true}}}
	facade, adapter, store := updateFixture(packs, state, obs)
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.Blockers()) == 0 || len(plan.Phases()) != 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("blocked plan = applicable %v blockers %+v", plan.Applicable(), plan.Blockers())
	}
}

func TestUpdateBlocksCapabilityConflictIntroducedByCatalogCurrent(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Conflicts: []string{"cap:other"}},
		{ID: "other", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:other"}},
	}
	obs := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "combined", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "combined"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3}, Intents: []ActivationIntent{{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true}, {PackID: "other", Surface: SurfaceCodex, Version: "1.0.0", Active: true}}}
	facade, _, _ := updateFixture(packs, state, obs)
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.Blockers()) != 1 || plan.Blockers()[0].Kind != BlockerCapabilityConflict {
		t.Fatalf("blockers = %+v", plan.Blockers())
	}
}

func TestCatalogCurrentDriftPlansSafeRepair(t *testing.T) {
	pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
	drift := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "drift", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:guide"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "2.0.0", Active: true, Revision: 7}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "drift"}}}
	facade, _, _ := updateFixture([]Pack{pack}, state, drift)
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || plan.NoOp() || !plan.Applicable() || len(phaseActions(plan.phases, ConsentReversibleLocal)) != 1 {
		t.Fatalf("drift plan noop=%v applicable=%v phases=%+v err=%v", plan.NoOp(), plan.Applicable(), plan.Phases(), err)
	}
}

func TestUpdateRejectsStaleIntentOwnershipAndHostFactsWithZeroEffects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*fakeActivationStore)
		obs    []SurfaceInspection
	}{
		{name: "intent", mutate: func(s *fakeActivationStore) { s.state.Intent.Revision++ }},
		{name: "ownership", mutate: func(s *fakeActivationStore) { s.state.Ownership[0].Contributors = []string{"changed"} }},
		{name: "host", obs: []SurfaceInspection{
			{Revision: "changed", Projections: []ObservedProjection{
				{ID: "instruction:guide", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide"}},
			}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pack := Pack{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "v2"}}}
			preview := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:guide", Exists: true, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "instruction:guide"}}}}
			observations := append([]SurfaceInspection{preview}, tc.obs...)
			state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"app"}, Fingerprint: "old"}}}
			facade, adapter, store := updateFixture([]Pack{pack}, state, observations...)
			plan, _ := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
			if tc.mutate != nil {
				tc.mutate(store)
			}
			_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
			if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
				t.Fatalf("err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
			}
		})
	}
}

func TestUpdateRejectsChangedSurfaceAliasWithZeroEffects(t *testing.T) {
	pack := Pack{ID: "addy", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "command", ID: "review", Source: "review.md", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "review", Invocation: "$review", Mode: "degraded", Degradation: "codex-command-as-workflow-skill", Sharing: "exclusive"}}}}}
	preview := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "workflow:review", Goal: ProjectionPresent, ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "workflow:review"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "addy", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4, Aliases: []SurfaceAlias{}}}
	facade, adapter, store := updateFixture([]Pack{pack}, state, preview)
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "addy", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	store.state.Intent.Aliases = []SurfaceAlias{{Kind: "command", ID: "review", Name: "addy-review"}}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("alias change effects: err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
	}
}

func TestUpdateExternalPhasesUseTypedApprovalsAndStopAtBarrier(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{missingEngramResolution()}}
	executor := &fakeExternalExecutor{failID: "external:engram:setup:codex", failErr: errors.New("setup failed")}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"), engramObservation("missing"), engramObservation("ready"))
	facade.catalog.packs[0].Version = "2.0.0"
	store.state = ActivationState{Intent: ActivationIntent{PackID: "engram", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}}
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	phases := plan.Phases()
	if len(phases) != 4 || phases[0].Kind != ConsentReversibleLocal || phases[1].Kind != ConsentExecutableExternal || phases[2].Kind != ConsentToolHostSetup || phases[3].Kind != ConsentHostFollowUp {
		t.Fatalf("phases = %+v", phases)
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrApprovalMismatch) || len(store.saves) != 0 {
		t.Fatalf("local-only approval err=%v saves=%d", err, len(store.saves))
	}
	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: requiredApprovals(facade, plan), Interactive: true})
	if err == nil || len(executor.actions) != 2 || len(adapter.actions) == 0 || store.state.Journal == nil || store.state.Journal.FailedAction != "external:engram:setup:codex" {
		t.Fatalf("barrier err=%v external=%+v state=%+v", err, executor.actions, store.state)
	}
}

func TestUpdateRejectsChangedDependencyClosureWithZeroEffects(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"dep"}}}, {ID: "dep", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"dep"}}}
	obs := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "combined", ObservedFingerprint: "old", DesiredFingerprint: "new", Action: ProjectionAction{ID: "combined"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}}
	facade, adapter, store := updateFixture(packs, state, obs)
	plan, _ := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "app", Surface: SurfaceCodex})
	facade.catalog.packs[0].Requires.Capabilities = nil
	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("dependency stale err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
	}
}

func TestUpdateRejectsChangedExecutableResolutionWithZeroEffects(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/v1/engram"), availableEngramResolution("/v2/engram")}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"))
	facade.catalog.packs[0].Version = "2.0.0"
	store.state = ActivationState{Intent: ActivationIntent{PackID: "engram", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}}
	plan, _ := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "engram", Surface: SurfaceCodex})
	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: requiredApprovals(facade, plan), Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 || len(executor.actions) != 0 {
		t.Fatalf("executable stale err=%v saves=%d local=%d external=%d", err, len(store.saves), len(adapter.actions), len(executor.actions))
	}
}
