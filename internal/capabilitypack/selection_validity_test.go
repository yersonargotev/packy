package capabilitypack

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestFacadeShowExplainsPerSurfaceRootAndAllSelectability(t *testing.T) {
	pack := Pack{
		ID:       "app",
		Version:  "1.0.0",
		Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{
			{
				Kind: "command", ID: "consumer",
				Requires: []string{"skill:shared"}, Notices: []string{},
				Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "consumer", Mode: "native"}},
			},
			{
				Kind: "skill", ID: "optional", Notices: []string{},
				SurfaceExclusions: []SurfaceExclusion{{
					Surface: SurfaceCodex, Mode: "optional", Code: "unsupported", Reason: "not projected by this host",
				}},
			},
			{
				Kind: "skill", ID: "shared", Notices: []string{},
				SurfaceExclusions: []SurfaceExclusion{{
					Surface: SurfaceCodex, Mode: "mandatory", Code: "unsupported", Reason: "required host support is missing",
				}},
			},
		},
	}
	facade := NewFacade(
		Catalog{packs: []Pack{pack}, entries: []catalogEntry{{ID: "app"}}},
		WithActivation(&fakeActivationStore{}, nil),
	)

	report, err := facade.Show(context.Background(), "app")
	if err != nil {
		t.Fatal(err)
	}
	validity := report.Surfaces[0].Contract.SelectionValidity
	if validity.All.Available {
		t.Fatalf("all unexpectedly selectable: %#v", validity.All)
	}
	if len(validity.All.OptionalExclusions) != 1 ||
		validity.All.OptionalExclusions[0].Resource.String() != "skill:optional" {
		t.Fatalf("optional exclusions = %#v", validity.All.OptionalExclusions)
	}
	roots := map[string]ResourceSelectability{}
	for _, root := range validity.Roots {
		roots[root.Resource.String()] = root
	}
	if roots["skill:optional"].Available ||
		len(roots["skill:optional"].Reasons) != 1 ||
		roots["skill:optional"].Reasons[0].Code != SelectionReasonRootExcluded {
		t.Fatalf("optional custom root = %#v", roots["skill:optional"])
	}
	consumer := roots["command:consumer"]
	if consumer.Available || len(consumer.Reasons) != 1 ||
		consumer.Reasons[0].Code != SelectionReasonDependencyUnavailable ||
		!reflect.DeepEqual(consumer.Reasons[0].DependencyChain, []ResourceIdentity{
			{Kind: "command", ID: "consumer"}, {Kind: "skill", ID: "shared"},
		}) {
		t.Fatalf("consumer validity = %#v", consumer)
	}
}

func TestExternalExecutableAcquisitionIsSelectedOnlyWithItsDeclaringResourceClosure(t *testing.T) {
	pack := Pack{
		ID: "portable", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Tools: []string{"engram"}},
		Resources: []Resource{
			{Kind: "mcp_server", ID: "memory", Requires: []string{}, Bindings: []Binding{{Surface: SurfaceCodex, Capabilities: []SurfaceCapability{{Type: SurfaceCapabilityExternalExecutableAcquisition, ExternalExecutableAcquisition: &ExternalExecutableAcquisitionCapability{Tool: "engram"}}}}}, SurfaceExclusions: []SurfaceExclusion{}},
			{Kind: "skill", ID: "plain", Requires: []string{}, Bindings: []Binding{{Surface: SurfaceCodex}}, SurfaceExclusions: []SurfaceExclusion{}},
		},
	}
	resolver := &recordingReadinessResolver{paths: map[string]string{}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(&fakeActivationStore{}, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{}}), WithExternalEffects(resolver, map[SurfaceCapabilityType]ExecutableAcquirer{SurfaceCapabilityExternalExecutableAcquisition: &recordingAcquirer{}}, nil))

	plain, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex, Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "plain"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if planHasAction(plain, "external:engram:acquire") {
		t.Fatal("unselected acquisition entered the plan")
	}
	selected, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex, Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "mcp_server", ID: "memory"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !planHasAction(selected, "external:engram:acquire") {
		t.Fatal("selected capability omitted acquisition")
	}
	if planHasAction(selected, "external:engram:setup:codex") {
		t.Fatal("acquisition planned host setup")
	}
}

func planHasAction(plan ReconciliationPlan, id string) bool {
	for _, phase := range plan.Phases() {
		for _, action := range phase.Actions {
			if action.ID == id {
				return true
			}
		}
	}
	return false
}

func TestConflictingSelectionBlocksWithRolesChainsAndNoMutation(t *testing.T) {
	pack := Pack{
		ID:       "app",
		Version:  "1.0.0",
		Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{
			{
				Kind: "command", ID: "consumer",
				Requires: []string{"skill:derived"}, Notices: []string{},
				Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "consumer", Mode: "native"}},
			},
			{
				Kind: "skill", ID: "derived", Conflicts: []string{"skill:other"}, Notices: []string{},
				Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "derived", Mode: "native"}},
			},
			{
				Kind: "skill", ID: "other", Conflicts: []string{"skill:derived"}, Notices: []string{},
				Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "other", Mode: "native"}},
			},
		},
	}
	store := &fakeActivationStore{}
	adapter := &fakeSurfaceAdapter{}
	facade := NewFacade(
		Catalog{packs: []Pack{pack}},
		WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}),
	)
	plan, err := facade.Preview(context.Background(), ActivationRequest{
		PackID: "app", Surface: SurfaceCodex,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{
			{Kind: "command", ID: "consumer"}, {Kind: "skill", ID: "other"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Disposition() != PlanBlocked {
		t.Fatalf("disposition = %s, blockers=%#v", plan.Disposition(), plan.Blockers())
	}
	var conflict PlanBlocker
	for _, blocker := range plan.Blockers() {
		if blocker.Kind == BlockerResourceConflict {
			conflict = blocker
			break
		}
	}
	for _, want := range []string{
		"skill:derived", "dependency", "command:consumer -> skill:derived",
		"skill:other", "root", "remove one explicit root",
	} {
		if !strings.Contains(conflict.Detail, want) {
			t.Fatalf("conflict blocker omitted %q: %#v", want, conflict)
		}
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err == nil {
		t.Fatal("blocked conflict plan applied")
	}
	if len(adapter.applied) != 0 || len(store.saves) != 0 {
		t.Fatalf("blocked conflict mutated state: applied=%#v saves=%d", adapter.applied, len(store.saves))
	}
}

func TestAddingConflictingRootPreservesActiveCustomIntent(t *testing.T) {
	current := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "left"}}}
	pack := Pack{
		ID:       "app",
		Version:  "1.0.0",
		Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{
			{
				Kind: "skill", ID: "left", Conflicts: []string{"skill:right"}, Notices: []string{},
				Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "left", Mode: "native"}},
			},
			{
				Kind: "skill", ID: "right", Conflicts: []string{"skill:left"}, Notices: []string{},
				Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "right", Mode: "native"}},
			},
		},
	}
	before := ActivationState{
		SchemaVersion: 1,
		Intent: ActivationIntent{
			PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 7, Selection: current,
		},
	}
	store := &fakeActivationStore{state: before}
	facade := NewFacade(
		Catalog{packs: []Pack{pack}},
		WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{}}),
	)

	plan, err := facade.Preview(context.Background(), ActivationRequest{
		PackID: "app", Surface: SurfaceCodex,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "right"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Disposition() != PlanBlocked {
		t.Fatalf("disposition = %s", plan.Disposition())
	}
	if !reflect.DeepEqual(store.state, before) || len(store.saves) != 0 {
		t.Fatalf("preview changed prior intent: before=%#v after=%#v saves=%d", before, store.state, len(store.saves))
	}
}

func TestAllOmitsOptionalSurfaceExclusionsWhileCustomRootBlocks(t *testing.T) {
	pack := Pack{
		ID:       "app",
		Version:  "1.0.0",
		Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{
			{
				Kind: "skill", ID: "included", Notices: []string{},
				Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "included", Mode: "native"}},
			},
			{
				Kind: "skill", ID: "optional", Notices: []string{},
				SurfaceExclusions: []SurfaceExclusion{{
					Surface: SurfaceCodex, Mode: "optional", Code: "unsupported", Reason: "not projected by this host",
				}},
			},
		},
	}
	allAdapter := &fakeSurfaceAdapter{}
	allFacade := NewFacade(
		Catalog{packs: []Pack{pack}},
		WithActivation(&fakeActivationStore{}, map[Surface]SurfaceAdapter{SurfaceCodex: allAdapter}),
	)
	allPlan, err := allFacade.Preview(context.Background(), ActivationRequest{
		PackID: "app", Surface: SurfaceCodex,
		Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if allPlan.Disposition() == PlanBlocked || len(allAdapter.calls) != 1 ||
		len(allAdapter.calls[0].desired.Resources) != 1 ||
		allAdapter.calls[0].desired.Resources[0].ID != "included" {
		t.Fatalf("all selection desired facts = plan:%#v calls:%#v", allPlan.Blockers(), allAdapter.calls)
	}
	if exclusions := allPlan.LifecycleContract().SelectionValidity.All.OptionalExclusions; len(exclusions) != 1 ||
		exclusions[0].Resource.String() != "skill:optional" {
		t.Fatalf("all optional exclusions = %#v", exclusions)
	}
	if _, err := allFacade.preflightPlan(context.Background(), allPlan); err != nil {
		t.Fatalf("all selection with optional exclusions became stale before Apply: %v", err)
	}

	customStore := &fakeActivationStore{}
	customFacade := NewFacade(
		Catalog{packs: []Pack{pack}},
		WithActivation(customStore, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{}}),
	)
	customPlan, err := customFacade.Preview(context.Background(), ActivationRequest{
		PackID: "app", Surface: SurfaceCodex,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "optional"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if customPlan.Disposition() != PlanBlocked {
		t.Fatalf("excluded custom root disposition = %s blockers=%#v", customPlan.Disposition(), customPlan.Blockers())
	}
	found := false
	for _, blocker := range customPlan.Blockers() {
		if blocker.Kind == BlockerSelectionUnavailable && strings.Contains(blocker.Detail, "requested root skill:optional") {
			found = true
		}
	}
	if !found || len(customStore.saves) != 0 {
		t.Fatalf("excluded custom root blockers=%#v saves=%d", customPlan.Blockers(), len(customStore.saves))
	}
}
