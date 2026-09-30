package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIssue817ApprovedActivationPreservesUnaliasedContract(t *testing.T) {
	for _, mode := range []SelectionMode{SelectionAll, SelectionCustom} {
		for _, aliased := range []bool{false, true} {
			name := string(mode)
			if aliased {
				name += "/alias"
			} else {
				name += "/original"
			}
			t.Run(name, func(t *testing.T) {
				facade, adapter, store, pack := issue817ActivationFixture(t)
				selection := ResourceSelection{Mode: mode}
				if mode == SelectionCustom {
					selection.Roots = []ResourceIdentity{{Kind: "skill", ID: "guide"}}
				}
				var aliases []SurfaceAlias
				expected := "guide"
				if aliased {
					aliases = []SurfaceAlias{{Kind: "skill", ID: "guide", Name: "renamed"}}
					expected = "renamed"
				}
				plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex, Selection: selection, Aliases: aliases})
				if err != nil {
					t.Fatal(err)
				}
				result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}})
				if err != nil || !result.Verified {
					t.Fatalf("approved activation: verified=%v err=%v", result.Verified, err)
				}
				if !reflect.DeepEqual(plan.Pack(), pack) {
					t.Fatalf("sealed contract changed: %#v", plan.Pack())
				}
				current, err := facade.catalog.Show(context.Background(), pack.ID)
				if err != nil || !reflect.DeepEqual(current, pack) {
					t.Fatalf("catalog contract changed: %#v err=%v", current, err)
				}
				if len(adapter.actions) != 1 || filepath.Base(adapter.actions[0].Target) != expected {
					t.Fatalf("projection actions = %#v", adapter.actions)
				}
				if len(store.saves) != 1 {
					t.Fatalf("activation saves = %d", len(store.saves))
				}
			})
		}
	}
}

func TestIssue817AliasedActivationRejectsRealFreshnessChanges(t *testing.T) {
	for _, change := range []string{"catalog", "state"} {
		t.Run(change, func(t *testing.T) {
			facade, adapter, store, pack := issue817ActivationFixture(t)
			plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex, Aliases: []SurfaceAlias{{Kind: "skill", ID: "guide", Name: "renamed"}}})
			if err != nil {
				t.Fatal(err)
			}
			approval := facade.Approve(plan, ConsentReversibleLocal)
			if change == "catalog" {
				facade.catalog.packs[0].Resources[0].Bindings[0].Name = "changed"
			} else {
				store.state.documentRevision++
			}
			_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true, Approvals: []ApprovalReceipt{approval}})
			if !errors.Is(err, ErrStalePlan) || !strings.Contains(err.Error(), "changed") {
				t.Fatalf("freshness error = %v", err)
			}
			if len(adapter.actions) != 0 || len(store.saves) != 0 {
				t.Fatalf("stale plan mutated projections or state: actions=%v saves=%v", adapter.actions, store.saves)
			}
		})
	}
}

func TestIssue817LifecycleContractAliasesDoNotMutatePack(t *testing.T) {
	_, _, _, pack := issue817ActivationFixture(t)
	original := clonePack(pack)
	contract := LifecycleContractFor(pack, SurfaceCodex, []SurfaceAlias{{Kind: "skill", ID: "guide", Name: "renamed"}})
	if len(contract.Bindings) != 1 || contract.Bindings[0].Name != "renamed" {
		t.Fatalf("aliased contract = %#v", contract.Bindings)
	}
	if !reflect.DeepEqual(pack, original) {
		t.Fatalf("lifecycle presentation mutated catalog contract: %#v", pack)
	}
}

func issue817ActivationFixture(t *testing.T) (Facade, *fakeSurfaceAdapter, *fakeActivationStore, Pack) {
	t.Helper()
	root := t.TempDir()
	pack := Pack{ID: "contract-fixture", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide", Bindings: testCapabilityBindings("guide")}}}
	facade, adapter, store := updateFixture([]Pack{clonePack(pack)}, ActivationState{})
	adapter.inspect = func(transition SurfaceTransition) SurfaceInspection {
		target := filepath.Join(root, transition.Desired.Resources[0].Bindings[0].Name)
		observed := "missing"
		if len(adapter.actions) != 0 {
			observed = "exact"
		}
		return SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "skill:guide", ProjectionKey: "path:" + target, Exists: observed == "exact", ObservedFingerprint: observed, DesiredFingerprint: "exact", Action: ProjectionAction{ID: "skill:guide", Target: target, ProjectionKey: "path:" + target}}}}
	}
	return facade, adapter, store, pack
}
