package capabilitypack

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestAllSelectionDerivesOperationalClosureAndExcludesOrphans(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app",
		Resources: []Resource{
			{Kind: "asset", ID: "orphan", Notices: []string{}},
			{Kind: "asset", ID: "required", Notices: []string{"notice:license"}},
			{Kind: "notice", ID: "license"},
			{Kind: "notice", ID: "orphan"},
			{Kind: "skill", ID: "root", Requires: []string{"asset:required"}, Notices: []string{}},
		},
	}
	selected, err := selectPackResources(pack, ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}})
	if err != nil {
		t.Fatal(err)
	}
	var identities []string
	for _, resource := range selected.Resources {
		identities = append(identities, resource.Kind+":"+resource.ID)
	}
	want := []string{"asset:required", "skill:root", "notice:license"}
	if !reflect.DeepEqual(identities, want) {
		t.Fatalf("all closure = %v, want %v", identities, want)
	}
}

func TestPreviewPreservesDependencyOrderThroughComposition(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app",
		Version:         "1.0.0",
		Surfaces:        []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: "instruction", ID: "a", Requires: []string{"skill:z"}, Notices: []string{}},
			{Kind: "skill", ID: "z", Notices: []string{}},
		},
	}
	adapter := &fakeSurfaceAdapter{}
	facade := NewFacade(
		Catalog{packs: []Pack{pack}},
		WithActivation(&fakeActivationStore{}, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}),
	)
	plan, err := facade.Preview(context.Background(), ActivationRequest{
		PackID: "app", Surface: SurfaceCodex,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "instruction", ID: "a"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(adapter.calls) != 1 || len(adapter.calls[0].desired.Resources) != 2 {
		t.Fatalf("desired transition = %#v", adapter.calls)
	}
	got := []string{
		adapter.calls[0].desired.Resources[0].Kind + ":" + adapter.calls[0].desired.Resources[0].ID,
		adapter.calls[0].desired.Resources[1].Kind + ":" + adapter.calls[0].desired.Resources[1].ID,
	}
	if want := []string{"skill:z", "instruction:a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("desired resource order = %v, want %v", got, want)
	}
	report := plan.JSONReport(true)
	if !reflect.DeepEqual(report.ResourceGraph, report.Contract.ResourceGraph) {
		t.Fatalf("preview graphs disagree: top-level=%#v contract=%#v", report.ResourceGraph, report.Contract.ResourceGraph)
	}
	if report.ResourceGraph.Resources[0].Role != ResourceRoleDependency {
		t.Fatalf("dependency role = %q, want %q", report.ResourceGraph.Resources[0].Role, ResourceRoleDependency)
	}
}

func TestCompositionPreservesLegacyLexicalResourceOrder(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV1,
		ID:              "legacy",
		Resources: []Resource{
			{Kind: "skill", ID: "z"},
			{Kind: "instruction", ID: "a"},
		},
	}
	combined := (composition{requested: pack, packs: []Pack{pack}}).combinedPack()
	got := []string{
		combined.Resources[0].Kind + ":" + combined.Resources[0].ID,
		combined.Resources[1].Kind + ":" + combined.Resources[1].ID,
	}
	if want := []string{"instruction:a", "skill:z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy resource order = %v, want %v", got, want)
	}
}

func TestUnavailableMandatoryDependencyBlocksSelectedRootBeforeMutation(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app",
		Version:         "1.0.0",
		Surfaces:        []Surface{SurfaceCodex},
		Resources: []Resource{
			{
				Kind: "instruction", ID: "root", Requires: []string{"skill:required"}, Notices: []string{},
				Bindings: []Binding{{Surface: SurfaceCodex, Projection: "instruction", Name: "root", Mode: "native"}},
			},
			{
				Kind: "skill", ID: "required", Notices: []string{},
				SurfaceExclusions: []SurfaceExclusion{{
					Surface: SurfaceCodex,
					Mode:    "mandatory",
					Code:    "required-resource-unavailable",
					Reason:  "the dependency cannot be realized on codex",
				}},
			},
		},
	}
	adapter := &fakeSurfaceAdapter{}
	store := &fakeActivationStore{}
	facade := NewFacade(
		Catalog{packs: []Pack{pack}},
		WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}),
	)

	plan, err := facade.Preview(context.Background(), ActivationRequest{
		PackID:  "app",
		Surface: SurfaceCodex,
		Selection: ResourceSelection{
			Mode:  SelectionCustom,
			Roots: []ResourceIdentity{{Kind: "instruction", ID: "root"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Disposition() != PlanBlocked {
		t.Fatalf("disposition = %s, want %s", plan.Disposition(), PlanBlocked)
	}
	if len(plan.Blockers()) != 2 ||
		plan.Blockers()[0].Kind != BlockerCompatibility ||
		!strings.Contains(plan.Blockers()[0].Detail, "dependency closure") ||
		plan.Blockers()[1].Kind != BlockerSelectionUnavailable ||
		!strings.Contains(plan.Blockers()[1].Detail, "instruction:root") ||
		!strings.Contains(plan.Blockers()[1].Detail, "skill:required") {
		t.Fatalf("blockers = %+v", plan.Blockers())
	}

	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err == nil {
		t.Fatal("blocked dependency plan applied")
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("blocked dependency caused mutation: actions=%v saves=%v", adapter.actions, store.saves)
	}
}
