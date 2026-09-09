package capabilitypack

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestIssue762ConvergedUpdateRetainsCurrentContract(t *testing.T) {
	facade := issue762UpdateFixture("2.0.0")
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "contract-fixture", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	report := plan.JSONReport(true)
	if report.Disposition != PlanConverged {
		t.Fatalf("disposition = %s", report.Disposition)
	}
	diff := report.ContractDiff
	result, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan})
	if err != nil || !result.Verified {
		t.Fatalf("Apply converged update = %#v, err=%v", result, err)
	}
	if !diff.BaselineAvailable || diff.UnavailableReason != "" || len(diff.Added) != 0 || len(diff.Changed) != 0 || len(diff.Removed) != 0 || !reflect.DeepEqual(diff.Retained, []string{"skill:guide"}) {
		t.Fatalf("converged contract diff = %#v, want only retained skill:guide", diff)
	}
}

func TestIssue762VersionChangingUpdateReportsUnavailableHistoricalContract(t *testing.T) {
	facade := issue762UpdateFixture("1.0.0")
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "contract-fixture", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	diff := plan.JSONReport(true).ContractDiff
	if len(diff.Added) != 0 || len(diff.Changed) != 0 || len(diff.Removed) != 0 || len(diff.Retained) != 0 {
		t.Fatalf("unavailable historical contract fabricated classifications: %#v", diff)
	}
	encoded, err := json.Marshal(diff)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["baseline_available"] != false || wire["unavailable_reason"] != "historical_contract_unavailable" {
		t.Fatalf("historical comparison availability = %s", encoded)
	}
	for _, category := range []string{"added", "changed", "removed", "retained"} {
		if _, ok := wire[category].([]any); !ok {
			t.Fatalf("%s is not an array: %s", category, encoded)
		}
	}
}

func TestIssue762SameVersionMissingReceiptResourceHasUnavailableBaseline(t *testing.T) {
	facade, store := issue762UpdateFixtureWithStore("2.0.0")
	store.state.Intent.Resources = append(store.state.Intent.Resources, ResourceIdentity{Kind: "skill", ID: "missing"})
	store.state.Intents = []ActivationIntent{store.state.Intent}
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "contract-fixture", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	diff := plan.JSONReport(true).ContractDiff
	if diff.BaselineAvailable || len(diff.Retained) != 0 || diff.UnavailableReason != "historical_contract_unavailable" {
		t.Fatalf("missing receipt resource baseline = %#v", diff)
	}
}

func TestIssue762UpdateAliasChangeHasSupportedContractDiff(t *testing.T) {
	facade := issue762UpdateFixture("2.0.0")
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "contract-fixture", Surface: SurfaceCodex, Aliases: []SurfaceAlias{{Kind: "skill", ID: "guide", Name: "renamed"}}})
	if err != nil {
		t.Fatal(err)
	}
	diff := plan.JSONReport(true).ContractDiff
	if !diff.BaselineAvailable || !reflect.DeepEqual(diff.Changed, []string{"skill:guide"}) || len(diff.Added) != 0 || len(diff.Removed) != 0 || len(diff.Retained) != 0 {
		t.Fatalf("alias contract diff = %#v", diff)
	}
}

func TestIssue762ActivationAndDeactivationKeepCompleteContractDiffs(t *testing.T) {
	for _, operation := range []Operation{OperationActivate, OperationDeactivate} {
		t.Run(string(operation), func(t *testing.T) {
			facade, store := issue762UpdateFixtureWithStore("2.0.0")
			var plan ReconciliationPlan
			var err error
			if operation == OperationActivate {
				store.state = ActivationState{}
				plan, err = facade.Preview(context.Background(), ActivationRequest{PackID: "contract-fixture", Surface: SurfaceCodex, Selection: ResourceSelection{Mode: SelectionAll}})
			} else {
				plan, err = facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "contract-fixture", Surface: SurfaceCodex})
			}
			if err != nil {
				t.Fatal(err)
			}
			diff := plan.JSONReport(true).ContractDiff
			expected := JSONContractDiff{BaselineAvailable: true, Added: []string{}, Changed: []string{}, Removed: []string{}, Retained: []string{}}
			if operation == OperationActivate {
				expected.Added = []string{"skill:guide"}
			} else {
				expected.Removed = []string{"skill:guide"}
			}
			if !reflect.DeepEqual(diff, expected) {
				t.Fatalf("%s contract diff = %#v, want %#v", operation, diff, expected)
			}
		})
	}
}

func TestIssue762CustomSelectionDoesNotIncludeUnselectedOrUnrelatedResources(t *testing.T) {
	pack := Pack{ID: "contract-fixture", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "skill", ID: "guide", Source: "guide", Bindings: testCapabilityBindings("guide")},
		{Kind: "skill", ID: "unused", Source: "unused", Bindings: testCapabilityBindings("unused")},
	}}
	selection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "guide"}}}
	intent := ActivationIntent{PackID: pack.ID, Version: pack.Version, Surface: SurfaceCodex, Active: true, Selection: selection, Resources: []ResourceIdentity{{Kind: "skill", ID: "guide"}}}
	unrelated := ActivationIntent{PackID: "unrelated", Version: "1.0.0", Surface: SurfaceCodex, Active: true, Selection: ResourceSelection{Mode: SelectionAll}, Resources: []ResourceIdentity{{Kind: "skill", ID: "other"}}}
	facade, _, _ := updateFixture([]Pack{pack}, ActivationState{Intent: intent, Intents: []ActivationIntent{intent, unrelated}})
	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	diff := plan.JSONReport(true).ContractDiff
	expected := JSONContractDiff{BaselineAvailable: true, Added: []string{}, Changed: []string{}, Removed: []string{}, Retained: []string{"skill:guide"}}
	if !reflect.DeepEqual(diff, expected) {
		t.Fatalf("selected contract diff = %#v, want %#v", diff, expected)
	}
}

func issue762UpdateFixture(installedVersion string) Facade {
	facade, _ := issue762UpdateFixtureWithStore(installedVersion)
	return facade
}

func issue762UpdateFixtureWithStore(installedVersion string) (Facade, *fakeActivationStore) {
	pack := Pack{ID: "contract-fixture", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide", Bindings: testCapabilityBindings("guide")}}}
	explicit := true
	intent := ActivationIntent{PackID: pack.ID, Version: installedVersion, Surface: SurfaceCodex, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Resources: []ResourceIdentity{{Kind: "skill", ID: "guide"}}, Explicit: &explicit}
	state := ActivationState{SchemaVersion: 3, Intent: intent, Intents: []ActivationIntent{intent}, snapshotManaged: true, Ownership: []ProjectionOwnership{{ID: "path:/tmp/contract-guide", ProjectionID: "skill:guide", Target: "/tmp/contract-guide", Fingerprint: "exact", PackID: pack.ID, Surface: SurfaceCodex}}}
	observation := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{ID: "skill:guide", ProjectionKey: "path:/tmp/contract-guide", Exists: true, ObservedFingerprint: "exact", DesiredFingerprint: "exact", Action: ProjectionAction{ID: "skill:guide", Target: "/tmp/contract-guide", ProjectionKey: "path:/tmp/contract-guide"}}}}
	facade, _, store := updateFixture([]Pack{pack}, state, observation)
	return facade, store
}
