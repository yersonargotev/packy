package capabilitypack

import (
	"context"
	"strings"
	"testing"
)

func receiptStatusFixture(t *testing.T, mutate func(*ActivationState), inspect func(SurfaceTransition) SurfaceInspection) (Facade, *fakeActivationStore, *fakeSurfaceAdapter) {
	t.Helper()
	identity := ResourceIdentity{Kind: "skill", ID: "retired-guide"}
	intent := ActivationIntent{PackID: "changing", Version: "1.0.0", Surface: SurfaceCodex, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{identity}}, Resources: []ResourceIdentity{identity}, ReadinessObligations: []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization}, ExternalRequirements: []string{}}
	owner := ProjectionOwnership{ID: "path:/fixture/retired-guide", ProjectionID: identity.String(), Target: "/fixture/retired-guide", PackID: intent.PackID, Surface: intent.Surface, Fingerprint: strings.Repeat("a", 64)}
	state := ActivationState{Intent: intent, Ownership: []ProjectionOwnership{owner}}
	if mutate != nil {
		mutate(&state)
	}
	store := &fakeActivationStore{state: state}
	adapter := &fakeSurfaceAdapter{inspect: inspect}
	if inspect == nil {
		adapter.inspect = func(transition SurfaceTransition) SurfaceInspection {
			if !transition.ObservationOnly || transition.Desired.ID != "" || transition.Prior.ID != "" || transition.ReceiptOwnership == nil {
				t.Fatal("historical status consulted manifest projections")
			}
			observed := SurfaceInspection{Revision: "receipt-v1", Readiness: ReadinessObservation{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true}}
			for _, owner := range transition.ReceiptOwnership {
				observed.Projections = append(observed.Projections, ObservedProjection{ID: owner.ID, Goal: ProjectionPresent, Exists: true, DesiredFingerprint: owner.Fingerprint, ObservedFingerprint: owner.Fingerprint, Action: ProjectionAction{ID: owner.ID, Target: owner.Target}})
			}
			return observed
		}
	}
	current := Pack{ID: "changing", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "skill", ID: "replacement"}}, ReadinessObligations: []ReadinessObligation{}}
	return NewFacade(Catalog{packs: []Pack{current}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter})), store, adapter
}

func TestReceiptStatusRetainsHistoricalFactsWithoutCurrentResourceGraph(t *testing.T) {
	facade, store, adapter := receiptStatusFixture(t, nil, nil)
	report, err := facade.Status(context.Background(), StatusRequest{PackID: "changing", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	entry := report.Entries[0]
	if entry.Pack.Version != "2.0.0" || entry.Intent.Version != "1.0.0" || !entry.UpdateAvailable || !entry.UpdateActionAvailable || entry.HistoricalEvidence.Available || entry.HistoricalEvidence.Message == "" {
		t.Fatalf("identities and evidence: %+v", entry)
	}
	if entry.Projections.Verified != 1 || entry.ProjectionDetails[0].DesiredFingerprint != strings.Repeat("a", 64) || len(entry.Resources) != 0 || len(entry.ResourceSelections) != 0 || len(entry.Contract.Bindings) != 0 {
		t.Fatalf("invented historical semantics: %+v", entry)
	}
	if entry.Readiness.Configured != ReadinessTrue || entry.Readiness.Authorized != ReadinessUnknown || entry.Readiness.Usable != ReadinessUnknown || entry.ControlledCheckActionAvailable {
		t.Fatalf("historical readiness inferred from host observation: %+v", entry)
	}
	if len(store.saves) != 0 || len(adapter.applied) != 0 {
		t.Fatal("status mutated installed state")
	}
	active, err := facade.ActiveStatus(context.Background())
	if err != nil || len(active.Entries) != 1 || active.Entries[0].InspectionFailed {
		t.Fatalf("active status = %+v, %v", active, err)
	}
}

func TestReceiptStatusRetainsMissingAndDriftedProjectionEvidence(t *testing.T) {
	for _, health := range []ProjectionHealth{ProjectionMissing, ProjectionDrifted} {
		t.Run(string(health), func(t *testing.T) {
			facade, _, _ := receiptStatusFixture(t, nil, func(transition SurfaceTransition) SurfaceInspection {
				owner := transition.ReceiptOwnership[0]
				return SurfaceInspection{Revision: "changed", Projections: []ObservedProjection{{ID: owner.ID, Exists: health != ProjectionMissing, DesiredFingerprint: owner.Fingerprint, ObservedFingerprint: "changed", Action: ProjectionAction{ID: owner.ID, Target: owner.Target}}}}
			})
			report, err := facade.Status(context.Background(), StatusRequest{PackID: "changing", Surface: SurfaceCodex})
			if err != nil {
				t.Fatal(err)
			}
			entry := report.Entries[0]
			if entry.ProjectionDetails[0].Health != health || entry.Readiness.Configured != ReadinessFalse || !entry.UpdateAvailable {
				t.Fatalf("projection evidence = %+v", entry)
			}
		})
	}
}

func TestReceiptStatusRejectsCorruptHistoricalReceipt(t *testing.T) {
	cases := map[string]func(*ActivationState){
		"version": func(s *ActivationState) { s.Intent.Version = "old" },
		"invalid selection": func(s *ActivationState) {
			s.Intent.Selection.Roots = []ResourceIdentity{{Kind: "skill", ID: "unrecorded"}}
		},
		"duplicate resources":  func(s *ActivationState) { s.Intent.Resources = append(s.Intent.Resources, s.Intent.Resources[0]) },
		"invalid resource":     func(s *ActivationState) { s.Intent.Resources[0].ID = "../escape" },
		"readiness":            func(s *ActivationState) { s.Intent.ReadinessObligations = []ReadinessObligation{"invented"} },
		"digest":               func(s *ActivationState) { s.Ownership[0].Fingerprint = "not-a-digest" },
		"physical ownership":   func(s *ActivationState) { s.Ownership[0].ID = "path:/different" },
		"duplicate projection": func(s *ActivationState) { s.Ownership = append(s.Ownership, s.Ownership[0]) },
		"unrelated owner": func(s *ActivationState) {
			other := s.Ownership[0]
			other.PackID = "other"
			s.Ownership = append(s.Ownership, other)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			facade, _, adapter := receiptStatusFixture(t, mutate, nil)
			if _, err := facade.Status(context.Background(), StatusRequest{PackID: "changing", Surface: SurfaceCodex}); err == nil {
				t.Fatal("accepted corrupt receipt")
			}
			if adapter.inspectCalls != 0 {
				t.Fatal("inspected unsafe receipt before validation")
			}
		})
	}
}

func TestReceiptStatusRejectsAdapterThatOmitsOrReinterpretsReceipt(t *testing.T) {
	for _, kind := range []string{"omitted", "target", "digest"} {
		t.Run(kind, func(t *testing.T) {
			facade, _, _ := receiptStatusFixture(t, nil, func(transition SurfaceTransition) SurfaceInspection {
				if kind == "omitted" {
					return SurfaceInspection{}
				}
				owner := transition.ReceiptOwnership[0]
				projection := ObservedProjection{ID: owner.ID, DesiredFingerprint: owner.Fingerprint, ObservedFingerprint: owner.Fingerprint, Exists: true, Action: ProjectionAction{ID: owner.ID, Target: owner.Target}}
				if kind == "target" {
					projection.Action.Target = "/different"
				} else {
					projection.DesiredFingerprint = strings.Repeat("b", 64)
				}
				return SurfaceInspection{Projections: []ObservedProjection{projection}}
			})
			if _, err := facade.Status(context.Background(), StatusRequest{PackID: "changing", Surface: SurfaceCodex}); err == nil {
				t.Fatal("accepted reinterpreted receipt")
			}
		})
	}
}

func TestReceiptStatusExplainsUnavailableFocusedResource(t *testing.T) {
	facade, _, _ := receiptStatusFixture(t, nil, nil)
	_, err := facade.Status(context.Background(), StatusRequest{PackID: "changing", Surface: SurfaceCodex, Resource: "skill:retired-guide"})
	if err == nil || !strings.Contains(err.Error(), "selected in the installed receipt") || !strings.Contains(err.Error(), "historical resource semantics are unavailable") {
		t.Fatalf("focused resource error = %v", err)
	}
}

func TestReceiptStatusRetainsSurfaceRemovedFromCurrentCatalog(t *testing.T) {
	for _, request := range []StatusRequest{{PackID: "changing", Surface: SurfaceCodex}, {}} {
		name := "overview"
		if request.PackID != "" {
			name = "targeted"
		}
		t.Run(name, func(t *testing.T) {
			facade, store, _ := receiptStatusFixture(t, nil, nil)
			facade.catalog.packs[0].Surfaces = []Surface{SurfaceOpenCode}
			facade.catalog.packs[0].Resources = nil
			facade.activation.adapters[SurfaceOpenCode] = &fakeSurfaceAdapter{}
			report, err := facade.Status(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, entry := range report.Entries {
				if entry.Surface != SurfaceCodex {
					continue
				}
				found = true
				if entry.Pack.Version != "2.0.0" || entry.Intent.Version != "1.0.0" || !entry.UpdateAvailable || entry.UpdateActionAvailable || entry.HistoricalEvidence.Available || entry.Projections.Verified != 1 || len(entry.Resources) != 0 || len(entry.ResourceSelections) != 0 || len(entry.Contract.Bindings) != 0 {
					t.Fatalf("removed surface lost receipt evidence: %+v", entry)
				}
			}
			if !found {
				t.Fatalf("active receipted surface omitted: %+v", report)
			}
			if len(store.saves) != 0 {
				t.Fatal("inspection mutated state")
			}
		})
	}
}

func TestReceiptStatusRejectsUnsupportedSurfaceWithoutActiveReceipt(t *testing.T) {
	for _, surface := range []Surface{SurfaceCodex, Surface("unrecognized")} {
		t.Run(string(surface), func(t *testing.T) {
			facade, store, adapter := receiptStatusFixture(t, nil, nil)
			facade.catalog.packs[0].Surfaces = []Surface{SurfaceOpenCode}
			store.state.Intent.Active = false
			_, err := facade.Status(context.Background(), StatusRequest{PackID: "changing", Surface: surface})
			if err == nil || !strings.Contains(err.Error(), "does not support CLI surface") {
				t.Fatalf("unsupported surface error = %v", err)
			}
			if adapter.inspectCalls != 0 {
				t.Fatal("inspected unsupported surface without active receipt")
			}
		})
	}
}

func TestReceiptStatusRejectsSurfaceInconsistentWithMatchingManifest(t *testing.T) {
	facade, _, adapter := receiptStatusFixture(t, nil, nil)
	facade.catalog.packs[0].Surfaces = []Surface{SurfaceOpenCode}
	facade.catalog.packs[0].Version = "1.0.0"
	_, err := facade.Status(context.Background(), StatusRequest{PackID: "changing", Surface: SurfaceCodex})
	if err == nil || !strings.Contains(err.Error(), "absent from matching catalog Pack") {
		t.Fatalf("inconsistent surface error = %v", err)
	}
	if adapter.inspectCalls != 0 {
		t.Fatal("inspected manifest-inconsistent receipt")
	}
}
