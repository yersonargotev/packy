package capabilitypack

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func deactivationFixture(packs []Pack, state ActivationState, observations ...SurfaceInspection) (Facade, *fakeSurfaceAdapter, *fakeActivationStore) {
	adapter := &fakeSurfaceAdapter{observations: observations}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: packs}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	return facade, adapter, store
}

func deletionObservation(revision, observed string, exists bool) SurfaceInspection {
	return SurfaceInspection{Revision: revision, Projections: []ObservedProjection{{
		ID:                  "instruction:guide",
		Exists:              exists,
		ObservedFingerprint: observed,
		Action:              ProjectionAction{ID: "instruction:guide", Description: "delete guide"},
	}}}
}

func incrementalSelectionPack() Pack {
	return Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app",
		Version:         "1.0.0",
		Surfaces:        []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: "skill", ID: "one", Requires: []string{"skill:shared"}, Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "one", Mode: "native", Sharing: "shared"}}},
			{Kind: "skill", ID: "two", Requires: []string{"skill:shared"}, Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "two", Mode: "native", Sharing: "shared"}}},
			{Kind: "skill", ID: "shared", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "shared", Mode: "native", Sharing: "shared"}}},
		},
	}
}

func incrementalSelectionState(roots ...string) ActivationState {
	identities := make([]ResourceIdentity, 0, len(roots))
	for _, root := range roots {
		identity, err := ParseResourceIdentity(root)
		if err != nil {
			panic(err)
		}
		identities = append(identities, identity)
	}
	intent := ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4, Selection: ResourceSelection{Mode: SelectionCustom, Roots: identities}}
	return ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}
}

func TestDeactivateResourceRemovesOneCustomRootAndPersistsSelection(t *testing.T) {
	pack := incrementalSelectionPack()
	facade, adapter, store := deactivationFixture([]Pack{pack}, incrementalSelectionState("skill:one", "skill:two"), SurfaceInspection{Revision: "host"})

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex, Resources: []ResourceIdentity{{Kind: "skill", ID: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.partialSelection || plan.NoOp() || len(plan.Blockers()) != 0 || len(plan.Phases()) != 0 {
		t.Fatalf("partial plan = partial %v noop %v blockers %+v phases %+v", plan.partialSelection, plan.NoOp(), plan.Blockers(), plan.Phases())
	}
	if got := plan.Selection().Roots; len(got) != 1 || got[0].String() != "skill:two" {
		t.Fatalf("remaining selection = %+v", got)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if !store.state.Intent.Active || len(store.state.Intent.Selection.Roots) != 1 || store.state.Intent.Selection.Roots[0].String() != "skill:two" {
		t.Fatalf("persisted partial deactivation = %+v", store.state.Intent)
	}
	if len(adapter.actions) != 0 {
		t.Fatalf("intent-only selection change applied projection actions: %+v", adapter.actions)
	}
}

func TestDeactivateResourceRetainsSharedDependencyAndReportsRemainingRoot(t *testing.T) {
	pack := incrementalSelectionPack()
	facade, _, _ := deactivationFixture([]Pack{pack}, incrementalSelectionState("skill:one", "skill:two"), SurfaceInspection{Revision: "host"})

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex, Resources: []ResourceIdentity{{Kind: "skill", ID: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	graph := plan.JSONReport(false).ResourceGraph.Resources
	var shared, remaining bool
	for _, resource := range graph {
		if resource.Resource.String() == "skill:shared" {
			shared = resource.Role == ResourceRoleDependency && len(resource.DependencyChain) > 0 && resource.DependencyChain[0].String() == "skill:two"
		}
		if resource.Resource.String() == "skill:two" {
			remaining = resource.Role == ResourceRoleRoot
		}
	}
	if !shared || !remaining {
		t.Fatalf("remaining resource graph = %+v", graph)
	}
}

func TestDeactivateResourceRejectsDerivedDependencyWithConsumerGuidance(t *testing.T) {
	pack := incrementalSelectionPack()
	facade, adapter, store := deactivationFixture([]Pack{pack}, incrementalSelectionState("skill:one", "skill:two"), SurfaceInspection{Revision: "host"})

	_, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex, Resources: []ResourceIdentity{{Kind: "skill", ID: "shared"}}})
	if err == nil || !strings.Contains(err.Error(), "dependency-only") || !strings.Contains(err.Error(), "skill:one") || !strings.Contains(err.Error(), "skill:two") {
		t.Fatalf("derived dependency error = %v", err)
	}
	if adapter.inspectCalls != 0 || len(store.saves) != 0 {
		t.Fatalf("invalid resource removal caused effects: inspect=%d saves=%d", adapter.inspectCalls, len(store.saves))
	}
}

func TestDeactivateFinalCustomRootDeactivatesPack(t *testing.T) {
	pack := incrementalSelectionPack()
	facade, _, store := deactivationFixture([]Pack{pack}, incrementalSelectionState("skill:one"), SurfaceInspection{Revision: "host"})

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex, Resources: []ResourceIdentity{{Kind: "skill", ID: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.partialSelection {
		t.Fatal("final root was treated as partial deactivation")
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if store.state.Intent.Active {
		t.Fatalf("final-root deactivation left Pack active: %+v", store.state.Intent)
	}
}

func TestDeactivateResourceFromAllDisclosesCustomSelection(t *testing.T) {
	pack := incrementalSelectionPack()
	state := incrementalSelectionState("skill:one")
	state.Intent.Selection = ResourceSelection{Mode: SelectionAll}
	state.Intents[0].Selection = ResourceSelection{Mode: SelectionAll}
	facade, _, store := deactivationFixture([]Pack{pack}, state, SurfaceInspection{Revision: "host"})

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex, Resources: []ResourceIdentity{{Kind: "skill", ID: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Selection().Roots; len(got) != 2 || got[0].String() != "skill:shared" || got[1].String() != "skill:two" {
		t.Fatalf("all-to-custom selection = %+v", got)
	}
	if migrations := plan.JSONReport(false).Migrations; len(migrations) != 1 || !strings.Contains(migrations[0], "all to custom") {
		t.Fatalf("all-to-custom migrations = %+v", migrations)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	if !store.state.Intent.Active || store.state.Intent.Selection.Mode != SelectionCustom || len(store.state.Intent.Selection.Roots) != 2 || store.state.Intent.Selection.Roots[0].String() != "skill:shared" || store.state.Intent.Selection.Roots[1].String() != "skill:two" {
		t.Fatalf("persisted all-to-custom state = %+v", store.state.Intent)
	}
}

func TestDeactivatePartialSelectionPreservesAliasesAndHistoricalVersion(t *testing.T) {
	pack := incrementalSelectionPack()
	packID := "mat" + "ty"
	pack.ID, pack.Version = packID, "2.0.0"
	intent := ActivationIntent{PackID: packID, Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 7, Aliases: []SurfaceAlias{{Kind: "skill", ID: "two", Name: "aliased-two"}}, Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "one"}, {Kind: "skill", ID: "two"}}}}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}
	store := &fakeActivationStore{state: state}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "host"}}}
	facade := NewFacade(Catalog{packs: []Pack{pack}, allowSyntheticHistory: true}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: packID, Surface: SurfaceCodex, Resources: []ResourceIdentity{{Kind: "skill", ID: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.OldVersion() != "1.0.0" || len(plan.aliases) != 1 || plan.aliases[0].Name != "aliased-two" {
		t.Fatalf("historical aliased partial plan = version %q aliases %+v", plan.OldVersion(), plan.aliases)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
		t.Fatalf("historical aliased partial apply: %v blockers=%+v phases=%+v", err, plan.Blockers(), plan.Phases())
	}
	if store.state.Intent.Version != "1.0.0" || len(store.state.Intent.Aliases) != 1 || store.state.Intent.Aliases[0].Name != "aliased-two" {
		t.Fatalf("historical aliased partial state = %+v", store.state.Intent)
	}
}

func TestDeactivatePersistsInactiveIntentBeforeVerifiedLastContributorDeletion(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"pack:app:instruction:guide"}, Fingerprint: "verified"}}}
	deleted := deletionObservation("host-2", "", false)
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, deletionObservation("host-1", "verified", true), deletionObservation("host-1", "verified", true), deleted)
	events := []string{}
	adapter.events, store.events = &events, &events

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Operation() != OperationDeactivate || plan.OldVersion() != "1.0.0" || plan.IntentRevision() != 4 {
		t.Fatalf("deactivation facts = operation %s version %q revision %d", plan.Operation(), plan.OldVersion(), plan.IntentRevision())
	}
	if phases := plan.Phases(); len(phases) != 1 || phases[0].Kind != ConsentDestructiveCleanup || len(phases[0].Actions) != 1 {
		t.Fatalf("phases = %+v", phases)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("local approval authorized deletion: %v", err)
	}
	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentDestructiveCleanup)}, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || !reflect.DeepEqual(events[:2], []string{"persist", "effects"}) {
		t.Fatalf("result/events = %+v %v", result, events)
	}
	if store.saves[0].Intent.Active || store.saves[0].Journal == nil || store.state.Journal != nil || len(store.state.Ownership) != 0 {
		t.Fatalf("deactivation state = first %+v final %+v", store.saves[0], store.state)
	}
}

func TestDeactivatePreservesProjectionWithoutExactResourceContributor(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
	for _, contributor := range []string{"pack:app:skill:other", "pack:app:not-a-resource"} {
		t.Run(contributor, func(t *testing.T) {
			state := ActivationState{
				Intent:    ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4},
				Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{contributor}, Fingerprint: "verified"}},
			}
			facade, adapter, store := deactivationFixture([]Pack{pack}, state, deletionObservation("host-1", "verified", true))

			plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Phases()) != 0 || len(plan.PendingHumanActions()) == 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
				t.Fatalf("wrong resource owner authorized cleanup: phases=%+v pending=%v actions=%v saves=%d", plan.Phases(), plan.PendingHumanActions(), adapter.actions, len(store.saves))
			}
		})
	}
}

func TestPartialDeactivatePreservesProjectionWithoutExactRemovedResourceContributor(t *testing.T) {
	pack := incrementalSelectionPack()
	state := incrementalSelectionState("skill:one", "skill:two")
	state.Ownership = []ProjectionOwnership{{ID: "skill:one", Contributors: []string{"pack:app:skill:two"}, Fingerprint: "verified"}}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{
		ID:                  "skill:one",
		Exists:              true,
		ObservedFingerprint: "verified",
		Action:              ProjectionAction{ID: "skill:one", Description: "delete skill one"},
	}}}
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, observation)

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex, Resources: []ResourceIdentity{{Kind: "skill", ID: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Phases()) != 0 || len(plan.PendingHumanActions()) == 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("wrong partial resource owner authorized cleanup: phases=%+v pending=%v actions=%v saves=%d", plan.Phases(), plan.PendingHumanActions(), adapter.actions, len(store.saves))
	}
}

func TestDeactivateRejectsActiveDependentWithoutCascade(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:app"}},
		{ID: "dependent", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"cap:app"}}},
	}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}, Intents: []ActivationIntent{
		{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2},
		{PackID: "dependent", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3},
	}}
	facade, adapter, store := deactivationFixture(packs, state, SurfaceInspection{Revision: "host"})

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.Blockers()) != 1 || len(plan.Phases()) != 0 {
		t.Fatalf("blocked plan = applicable %v blockers %+v phases %+v", plan.Applicable(), plan.Blockers(), plan.Phases())
	}
	detail := strings.ToLower(plan.Blockers()[0].Detail)
	for _, want := range []string{"app", "dependent", "cap:app", "cascade"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("blocker detail %q does not mention %q", detail, want)
		}
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("blocked deactivation caused effects")
	}
}

func TestDeactivateV4ProviderRejectsSelectedResourceDependentWithoutReintroduction(t *testing.T) {
	empty := []string{}
	provider := Pack{manifestVersion: manifestSchemaV4, ID: "provider", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "skill", ID: "storage", Bindings: testCapabilityBindings("storage"), Notices: empty, ProvidesCapabilities: []string{"cap:storage"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
		{Kind: "skill", ID: "unselected", Bindings: testCapabilityBindings("unselected"), Notices: empty, ProvidesCapabilities: []string{"cap:other"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
	}}
	consumer := Pack{manifestVersion: manifestSchemaV4, ID: "consumer", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "skill", ID: "root", Bindings: testCapabilityBindings("root"), Notices: empty, ProvidesCapabilities: empty, RequiresCapabilities: []string{"cap:storage"}, RequiresTools: empty, CapabilityConflicts: empty},
		{Kind: "skill", ID: "inactive", Bindings: testCapabilityBindings("inactive"), Notices: empty, ProvidesCapabilities: empty, RequiresCapabilities: []string{"cap:other"}, RequiresTools: empty, CapabilityConflicts: empty},
	}}
	providerSelection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "storage"}}}
	consumerSelection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "root"}}}
	intents := []ActivationIntent{
		{PackID: "provider", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3, Selection: providerSelection},
		{PackID: "consumer", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4, Selection: consumerSelection},
	}
	state := ActivationState{Intent: intents[0], Intents: intents}
	facade, adapter, store := deactivationFixture([]Pack{consumer, provider}, state, SurfaceInspection{Revision: "host"})
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "provider", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.Blockers()) != 1 || plan.Blockers()[0].Kind != BlockerActiveDependent ||
		plan.Blockers()[0].Subject != "provider" || !strings.Contains(plan.Blockers()[0].Detail, "cap:storage") {
		t.Fatalf("v4 dependent blocker = applicable:%v blockers:%#v", plan.Applicable(), plan.Blockers())
	}
	for _, pack := range plan.compositionFacts {
		if pack.ID == "provider" {
			t.Fatalf("removed provider was rediscovered in target composition: %#v", plan.compositionFacts)
		}
	}
	if len(plan.Phases()) != 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("blocked v4 deactivation mutated: phases=%#v actions=%d saves=%d", plan.Phases(), len(adapter.actions), len(store.saves))
	}
}

func TestDeactivateRetainsSharedProjectionAndResultingContributorsWithoutRewrite(t *testing.T) {
	packs := []Pack{
		{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
		{ID: "other", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}},
	}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 5}, Intents: []ActivationIntent{
		{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 5},
		{PackID: "other", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 1},
	}, Ownership: []ProjectionOwnership{{ID: "instruction:shared", Contributors: []string{"pack:app:instruction:shared", "pack:other:instruction:shared"}, Fingerprint: "same"}}}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:shared", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:shared"}}}}
	facade, adapter, _ := deactivationFixture(packs, state, observation)

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if retained := plan.RetainedProjections(); len(retained) != 1 || retained[0].ID != "instruction:shared" || !reflect.DeepEqual(retained[0].Contributors, []string{"pack:other:instruction:shared"}) {
		t.Fatalf("retained = %+v", retained)
	}
	if got := plan.Contributors()["instruction:shared"]; !reflect.DeepEqual(got, []string{"pack:other:instruction:shared"}) {
		t.Fatalf("result contributors = %v", got)
	}
	if len(plan.Phases()) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("shared projection was rewritten: phases=%+v actions=%+v", plan.Phases(), adapter.actions)
	}
}

func TestDeactivatePreservesAndBlocksDriftedSharedProjection(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}}, {ID: "other", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "shared", Source: "same"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1", Active: true, Revision: 3}, Intents: []ActivationIntent{{PackID: "app", Surface: SurfaceCodex, Version: "1", Active: true}, {PackID: "other", Surface: SurfaceCodex, Version: "1", Active: true}}, Ownership: []ProjectionOwnership{{ID: "instruction:shared", Contributors: []string{"app", "other"}, Fingerprint: "verified"}}}
	obs := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:shared", Exists: true, ObservedFingerprint: "user-drift", DesiredFingerprint: "verified", Action: ProjectionAction{ID: "instruction:shared"}}}}
	facade, adapter, store := deactivationFixture(packs, state, obs)
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.PendingHumanActions()) == 0 || len(plan.Phases()) != 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("plan=%+v pending=%v", plan.Blockers(), plan.PendingHumanActions())
	}
}

func TestDeactivatePreservesDriftedAndUnmanagedLastContributorTargets(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
	for _, tc := range []struct {
		name      string
		ownership []ProjectionOwnership
	}{
		{name: "drifted", ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"pack:app:instruction:guide"}, Fingerprint: "verified"}}},
		{name: "unmanaged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 1}, Ownership: tc.ownership}
			facade, adapter, _ := deactivationFixture([]Pack{pack}, state, deletionObservation("host", "user-content", true))
			plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Phases()) != 0 || len(plan.PendingHumanActions()) == 0 || len(adapter.actions) != 0 {
				t.Fatalf("unsafe target was not preserved: phases=%+v pending=%v actions=%v", plan.Phases(), plan.PendingHumanActions(), adapter.actions)
			}
		})
	}
}

func TestDeactivateRejectsStaleHostFactWithZeroEffects(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide", Source: "guide"}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 1}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"pack:app:instruction:guide"}, Fingerprint: "verified"}}}
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, deletionObservation("host-1", "verified", true), deletionObservation("host-2", "verified", true))
	plan, _ := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})

	_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentDestructiveCleanup)}, Interactive: true})
	if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("stale deactivation err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
	}
}

func TestDeactivateRejectsChangedIntentOwnershipCatalogAndDependentsWithZeroEffects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Facade, *fakeActivationStore)
	}{
		{"intent", func(_ *Facade, s *fakeActivationStore) { s.state.Intent.Revision++ }},
		{"ownership", func(_ *Facade, s *fakeActivationStore) { s.state.Ownership[0].Fingerprint = "changed" }},
		{"catalog", func(f *Facade, _ *fakeActivationStore) { f.catalog.packs[0].Version = "2" }},
		{"active-dependents", func(f *Facade, s *fakeActivationStore) {
			f.catalog.packs = append(f.catalog.packs, Pack{ID: "dependent", Version: "1", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"cap:app"}}})
			s.state.Intents = append(s.state.Intents, ActivationIntent{PackID: "dependent", Surface: SurfaceCodex, Version: "1", Active: true})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:app"}, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}
			state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1", Active: true, Revision: 2}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"pack:app:instruction:guide"}, Fingerprint: "verified"}}}
			obs := deletionObservation("host", "verified", true)
			facade, adapter, store := deactivationFixture([]Pack{pack}, state, obs, obs)
			plan, _ := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
			tc.mutate(&facade, store)
			_, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentDestructiveCleanup)}, Interactive: true})
			if !errors.Is(err, ErrStalePlan) || len(store.saves) != 0 || len(adapter.actions) != 0 {
				t.Fatalf("err=%v saves=%d actions=%d", err, len(store.saves), len(adapter.actions))
			}
		})
	}
}

func TestDeactivateAlreadyInactiveConvergedIsNoOp(t *testing.T) {
	pack := Pack{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: false, Revision: 8}}
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, SurfaceInspection{Revision: "host"})

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || !plan.NoOp() || len(plan.Phases()) != 0 {
		t.Fatalf("inactive plan noop=%v phases=%+v err=%v", plan.NoOp(), plan.Phases(), err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("inactive no-op caused effects")
	}
}

func TestDeactivateInactiveConvergedPackIsNoOpWithUnrelatedSurfaceOwnership(t *testing.T) {
	packs := []Pack{{ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}}, {ID: "other", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "other"}}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: false, Revision: 8}, Intents: []ActivationIntent{{PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: false, Revision: 8}, {PackID: "other", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 2}}, Ownership: []ProjectionOwnership{{ID: "instruction:other", Contributors: []string{"other"}, Fingerprint: "same"}}}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "instruction:other", Exists: true, ObservedFingerprint: "same", DesiredFingerprint: "same", Action: ProjectionAction{ID: "instruction:other"}}}}
	facade, _, store := deactivationFixture(packs, state, observation)
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil || !plan.NoOp() || len(store.saves) != 0 {
		t.Fatalf("plan noop=%v saves=%d err=%v", plan.NoOp(), len(store.saves), err)
	}
}

func TestDeactivateInactivePartialStateIsReportOnlyWithoutApplyOrEffects(t *testing.T) {
	pack := Pack{ID: "app", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "instruction", ID: "guide"}}}
	state := ActivationState{Intent: ActivationIntent{PackID: "app", Surface: SurfaceCodex, Version: "1", Active: false, Revision: 4}, Ownership: []ProjectionOwnership{{ID: "instruction:guide", Contributors: []string{"pack:app:instruction:guide"}, Fingerprint: "verified"}}}
	facade, adapter, store := deactivationFixture([]Pack{pack}, state, deletionObservation("host", "user-drift", true))
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "app", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || plan.NoOp() || len(plan.PendingHumanActions()) == 0 || len(plan.Phases()) != 0 {
		t.Fatalf("applicable=%v noop=%v blockers=%v pending=%v", plan.Applicable(), plan.NoOp(), plan.Blockers(), plan.PendingHumanActions())
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err == nil {
		t.Fatal("blocked inactive partial state applied")
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatal("inactive partial preview/apply caused effects")
	}
}
