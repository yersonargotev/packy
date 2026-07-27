package capabilitypack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseResourceIdentityRequiresCanonicalKindID(t *testing.T) {
	got, err := ParseResourceIdentity("skill:review")
	if err != nil || got != (ResourceIdentity{Kind: "skill", ID: "review"}) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	for _, value := range []string{"skill", "skill:review:extra", "unknown:review", "skill:Review"} {
		if _, err := ParseResourceIdentity(value); err == nil {
			t.Fatalf("ParseResourceIdentity(%q) succeeded", value)
		}
	}
}

func TestCustomSelectionProjectsExactlyOneIndependentV4Root(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app",
		Resources: []Resource{
			{Kind: "skill", ID: "one"},
			{Kind: "instruction", ID: "two"},
		},
	}
	selected, err := selectPackResources(pack, ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "one"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Resources) != 1 || selected.Resources[0].ID != "one" {
		t.Fatalf("selected resources = %+v", selected.Resources)
	}
	if len(pack.Resources) != 2 {
		t.Fatal("selection mutated catalog pack")
	}
}

func TestCustomSelectionRejectsUnsupportedGraphs(t *testing.T) {
	for name, pack := range map[string]Pack{
		"legacy":         {manifestVersion: manifestSchemaV3, ID: "app", Resources: []Resource{{Kind: "skill", ID: "one"}}},
		"dependency":     {manifestVersion: manifestSchemaV4, ID: "app", Resources: []Resource{{Kind: "skill", ID: "one", Requires: []string{"asset:data"}}, {Kind: "asset", ID: "data"}}},
		"nonoperational": {manifestVersion: manifestSchemaV4, ID: "app", Resources: []Resource{{Kind: "asset", ID: "one"}}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := selectPackResources(pack, ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: pack.Resources[0].Kind, ID: "one"}}})
			if err == nil {
				t.Fatal("custom selection succeeded")
			}
		})
	}
	if _, err := canonicalSelection(ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "one"}, {Kind: "skill", ID: "two"}}}); err == nil {
		t.Fatal("multiple roots succeeded")
	}
	selection, err := canonicalSelection(ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "one"}, {Kind: "skill", ID: "one"}}})
	if err != nil || len(selection.Roots) != 1 || selection.Roots[0].String() != "skill:one" {
		t.Fatalf("duplicate canonical root = %+v err=%v", selection, err)
	}
}

func TestSelectionIsImmutableAndSealedBeforeEffects(t *testing.T) {
	adapter := &fakeSurfaceAdapter{}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan := ReconciliationPlan{
		pack:      Pack{ID: "app", Version: "1.0.0"},
		operation: OperationActivate,
		surface:   SurfaceCodex,
		selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "one"}}},
		phases: []PlanPhase{{
			Kind: ConsentReversibleLocal, ApprovalRequired: true,
			Actions: []ProjectionAction{{ID: "skill:one", Description: "link one"}},
		}},
	}
	plan.seal()
	exposed := plan.Selection()
	exposed.Roots[0].ID = "two"
	if plan.Selection().Roots[0].ID != "one" {
		t.Fatal("plan exposed mutable selection storage")
	}
	receipt := facade.Approve(plan, ConsentReversibleLocal)
	plan.selection.Roots[0].ID = "two"
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: []ApprovalReceipt{receipt}, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("tampered selection error = %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("tampered selection caused effects")
	}
}

func TestFileActivationStoreRejectsInvalidV4SelectionWithoutRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "packs.json")
	data := `{"schema_version":4,"activations":[{"schema_version":3,"intent":{"pack_id":"app","surface":"codex","active":true,"revision":1,"aliases":[],"selection":{"mode":"future","roots":[]}}}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewFileActivationStore(path)
	if _, err := store.Load(context.Background(), SurfaceCodex); err == nil || !strings.Contains(err.Error(), "invalid resource selection") {
		t.Fatalf("Load error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != data {
		t.Fatal("invalid state was rewritten")
	}
}
