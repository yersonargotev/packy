package capabilitypack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestCustomSelectionRejectsLegacyAndNonOperationalRoots(t *testing.T) {
	for name, pack := range map[string]Pack{
		"legacy":         {manifestVersion: manifestSchemaV3, ID: "app", Resources: []Resource{{Kind: "skill", ID: "one"}}},
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

func TestCustomSelectionResolvesTransitiveDependenciesAndAssociatedNotices(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app",
		Resources: []Resource{
			{Kind: "asset", ID: "data", Notices: []string{"notice:mit"}},
			{Kind: "instruction", ID: "guide", Requires: []string{"skill:shared"}, Notices: []string{"notice:guide"}},
			{Kind: "notice", ID: "guide"},
			{Kind: "notice", ID: "mit"},
			{Kind: "skill", ID: "shared", Requires: []string{"asset:data"}},
			{Kind: "skill", ID: "unused", Notices: []string{"notice:unused"}},
			{Kind: "notice", ID: "unused"},
		},
	}
	selected, err := selectPackResources(pack, ResourceSelection{
		Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "instruction", ID: "guide"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var identities []string
	for _, resource := range selected.Resources {
		identities = append(identities, (ResourceIdentity{Kind: resource.Kind, ID: resource.ID}).String())
	}
	want := []string{"asset:data", "skill:shared", "instruction:guide", "notice:guide", "notice:mit"}
	if !reflect.DeepEqual(identities, want) {
		t.Fatalf("closure = %v want %v", identities, want)
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

func TestActivateRejectsChangingAnActiveSelectionBeforeInspectionOrEffects(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app",
		Version:         "1.0.0",
		Surfaces:        []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: "skill", ID: "one"},
			{Kind: "instruction", ID: "two"},
		},
	}
	original := ActivationState{Intent: ActivationIntent{
		PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 3,
		Aliases: []SurfaceAlias{}, Selection: ResourceSelection{
			Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "one"}},
		},
	}}
	adapter := &fakeSurfaceAdapter{}
	store := &fakeActivationStore{state: original}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	_, err := facade.Preview(context.Background(), ActivationRequest{
		PackID: "app", Surface: SurfaceCodex,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "instruction", ID: "two"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "different resource selection") {
		t.Fatalf("selection change error = %v", err)
	}
	if adapter.inspectCalls != 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("rejected selection change caused effects: inspect=%d actions=%v saves=%v", adapter.inspectCalls, adapter.actions, store.saves)
	}
	if !reflect.DeepEqual(store.state, original) {
		t.Fatalf("rejected selection change mutated state: got=%+v want=%+v", store.state, original)
	}
}

func TestActivateAllowsNewSelectionAfterPriorIntentIsInactive(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "app",
		Version:         "1.0.0",
		Surfaces:        []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: "skill", ID: "one"},
			{Kind: "instruction", ID: "two"},
		},
	}
	original := ActivationState{Intent: ActivationIntent{
		PackID: "app", Surface: SurfaceCodex, Version: "1.0.0", Active: false, Revision: 4,
		Aliases: []SurfaceAlias{}, Selection: ResourceSelection{
			Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "one"}},
		},
	}}
	original.Intents = []ActivationIntent{original.Intent}
	adapter := &fakeSurfaceAdapter{}
	store := &fakeActivationStore{state: original}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	requested := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "instruction", ID: "two"}}}

	plan, err := facade.Preview(context.Background(), ActivationRequest{
		PackID: "app", Surface: SurfaceCodex, Selection: requested,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Selection(), requested) {
		t.Fatalf("reactivation selection = %+v want %+v", plan.Selection(), requested)
	}
	if adapter.inspectCalls != 1 || len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("reactivation preview effects: inspect=%d actions=%v saves=%v", adapter.inspectCalls, adapter.actions, store.saves)
	}
	if !reflect.DeepEqual(store.state, original) {
		t.Fatalf("reactivation preview mutated state: got=%+v want=%+v", store.state, original)
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
