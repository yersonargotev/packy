package capabilitypack

import (
	"context"
	"strings"
	"testing"
)

func TestUnavailableMandatoryDependencyBlocksSelectedRootBeforeMutation(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app",
		Version:         "1.0.0",
		Surfaces:        []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: "instruction", ID: "root", Requires: []string{"skill:required"}, Notices: []string{}},
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
	if len(plan.Blockers()) != 1 ||
		plan.Blockers()[0].Kind != BlockerCompatibility ||
		!strings.Contains(plan.Blockers()[0].Detail, "dependency closure") {
		t.Fatalf("blockers = %+v", plan.Blockers())
	}

	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err == nil {
		t.Fatal("blocked dependency plan applied")
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("blocked dependency caused mutation: actions=%v saves=%v", adapter.actions, store.saves)
	}
}
