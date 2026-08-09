package capabilitypack

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

type readinessResolver struct{ available bool }

func (r readinessResolver) Resolve(_ context.Context, tool string) (ExecutableResolution, error) {
	return ExecutableResolution{Tool: tool, Available: r.available}, nil
}

func TestFacadeStatusPreservesConditionTruthAndAggregatesDimensions(t *testing.T) {
	pack := Pack{
		manifestVersion:      manifestSchemaV4,
		ID:                   "app",
		Version:              "1.0.0",
		Surfaces:             []Surface{SurfaceCodex},
		ReadinessObligations: []ReadinessObligation{ReadinessSurfaceAuthorization, ReadinessRuntimeUsability},
		Resources: []Resource{{
			Kind: "skill", ID: "guide", Source: "guide", Description: "Guide",
			Requires: []string{}, Conflicts: []string{}, Bindings: testCapabilityBindings("guide"), SurfaceExclusions: []SurfaceExclusion{},
		}},
		Contract: Contract{Exclusions: []Exclusion{}, OptionalModes: []OptionalMode{}},
	}
	state := ActivationState{
		Intent:    ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}},
		Ownership: []ProjectionOwnership{{ID: "skill:guide", ProjectionID: "skill:guide", PackID: pack.ID, Surface: SurfaceCodex, Fingerprint: "exact"}},
	}
	observation := SurfaceInspection{
		Revision: "codex-observation-v1",
		Projections: []ObservedProjection{{
			ID: "skill:guide", Exists: true, ObservedFingerprint: "exact", DesiredFingerprint: "exact",
			Action: ProjectionAction{ID: "skill:guide", Target: "/tmp/guide"},
		}},
		Readiness: ReadinessObservation{
			AuthorizationObserved: true, Authorized: false,
			UsabilityObserved: false,
			Evidence:          []string{"Codex trust was denied"},
		},
	}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(&fakeActivationStore{state: state}, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	report, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 1 {
		t.Fatalf("entries = %#v", report.Entries)
	}
	entry := report.Entries[0]
	if entry.Readiness.Configured != ReadinessTrue || entry.Readiness.Authorized != ReadinessFalse || entry.Readiness.Usable != ReadinessUnknown {
		t.Fatalf("readiness = %#v", entry.Readiness)
	}
	if len(entry.Conditions) != 3 {
		t.Fatalf("conditions = %#v", entry.Conditions)
	}
	want := []struct {
		typeName  ReadinessConditionType
		dimension ReadinessDimension
		value     ReadinessValue
		reason    ReadinessReason
	}{
		{ConditionProjectionIntegrity, ReadinessConfigured, ReadinessTrue, ReasonProjectionVerified},
		{ConditionSurfaceAuthorization, ReadinessAuthorized, ReadinessFalse, ReasonAuthorizationDenied},
		{ConditionRuntimeUsability, ReadinessUsable, ReadinessUnknown, ReasonRuntimeUnobservable},
	}
	for i, expected := range want {
		condition := entry.Conditions[i]
		if condition.Type != expected.typeName || condition.Dimension != expected.dimension || condition.Value != expected.value || condition.Reason != expected.reason {
			t.Fatalf("condition[%d] = %#v", i, condition)
		}
		if condition.Scope.Pack != pack.ID || condition.Scope.Surface != SurfaceCodex || condition.Scope.Kind != ReadinessScopeGlobal || condition.Message == "" || condition.Freshness.ObservedAt == "" || condition.Freshness.ValidityIdentity == "" {
			t.Fatalf("condition[%d] is incomplete: %#v", i, condition)
		}
	}
}

func TestReadinessAggregationFalseDominatesUnknown(t *testing.T) {
	conditions := []ReadinessCondition{
		{Dimension: ReadinessUsable, Value: ReadinessUnknown},
		{Dimension: ReadinessUsable, Value: ReadinessFalse},
		{Dimension: ReadinessUsable, Value: ReadinessTrue},
	}
	if got := aggregateReadinessDimension(conditions, ReadinessUsable); got != ReadinessFalse {
		t.Fatalf("aggregate = %q, want false", got)
	}
	conditions = conditions[:1]
	if got := aggregateReadinessDimension(conditions, ReadinessUsable); got != ReadinessUnknown {
		t.Fatalf("aggregate = %q, want unknown", got)
	}
	conditions = []ReadinessCondition{{Dimension: ReadinessUsable, Value: ReadinessTrue}}
	if got := aggregateReadinessDimension(conditions, ReadinessUsable); got != ReadinessTrue {
		t.Fatalf("aggregate = %q, want true", got)
	}
}

func TestInstalledReceiptPreservesReviewedReadinessObligations(t *testing.T) {
	obligations := []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization}
	intent := ActivationIntent{
		PackID: "app", Version: "1.0.0", Surface: SurfaceCodex, Active: true,
		ReadinessObligations: obligations,
		Selection:            ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}},
		Resources:            []ResourceIdentity{},
	}
	receipts := receiptDocumentFromActivation(activationDocument{Revision: 1, Activations: []ActivationState{{Intent: intent, Intents: []ActivationIntent{intent}}}})
	if len(receipts.Receipts) != 1 || len(receipts.Receipts[0].ReadinessObligations) != 2 || receipts.Receipts[0].ExternalRequirements == nil {
		t.Fatalf("installed receipt obligations = %#v", receipts.Receipts)
	}
	restored, err := activationDocumentFromReceipts(receipts)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Activations) != 1 || len(restored.Activations[0].Intent.ReadinessObligations) != 2 {
		t.Fatalf("restored intent obligations = %#v", restored.Activations)
	}
}

func TestProjectReceiptRejectsInvalidExternalRequirementIdentity(t *testing.T) {
	err := validateProjectReceipts([]installedPackReceipt{
		{
			Pack:                 installedPackIdentity{ID: "app", Version: "1.0.0"},
			Surface:              SurfaceCodex,
			ReadinessObligations: []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization},
			ExternalRequirements: []string{"Example Tool"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid external requirements") {
		t.Fatalf("receipt validation error = %v", err)
	}
}

func TestReadinessEvaluationDoesNotDependOnPackOrResourceIdentity(t *testing.T) {
	for _, scope := range []ReadinessScopeKind{ReadinessScopeGlobal, ReadinessScopeProject} {
		var baseline ReadinessStatus
		for index, identity := range []struct{ pack, resource string }{{"alpha", "guide"}, {"beta", "coordinate"}} {
			resource := ResourceIdentity{Kind: "skill", ID: identity.resource}
			status, conditions := evaluateReadiness(readinessEvaluation{
				Pack:    Pack{ID: identity.pack, Version: "1.0.0", ReadinessObligations: []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization}},
				Surface: SurfaceCodex, Scope: scope, Resource: &resource, Revision: "synthetic-v1",
				Projections: []ProjectionStatus{{ID: resource.String(), Health: ProjectionVerified}},
			})
			if index == 0 {
				baseline = status
			} else if status != baseline {
				t.Fatalf("scope %s changed readiness by identity: first=%#v second=%#v", scope, baseline, status)
			}
			if len(conditions) != 3 || conditions[0].Scope.Pack != identity.pack || conditions[0].Scope.Resource == nil || *conditions[0].Scope.Resource != resource {
				t.Fatalf("scope %s conditions = %#v", scope, conditions)
			}
		}
	}
}

func TestFacadeProjectLifecycleUsesIdentityAgnosticReadiness(t *testing.T) {
	var baseline ReadinessStatus
	for index, identity := range []struct{ pack, resource string }{{"alpha", "guide"}, {"beta", "coordinate"}} {
		root := t.TempDir()
		pack := Pack{
			manifestVersion: manifestSchemaV4, ID: identity.pack, Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
			ReadinessObligations: []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization},
			Requires:             Requirements{Tools: []string{}},
			Resources: []Resource{{Kind: "skill", ID: identity.resource, Source: identity.resource, Description: "Synthetic skill",
				Requires: []string{}, Conflicts: []string{}, Bindings: testCapabilityBindings(identity.resource), SurfaceExclusions: []SurfaceExclusion{}}},
			Contract: Contract{Exclusions: []Exclusion{}, OptionalModes: []OptionalMode{}},
		}
		projectionID := "skill:" + identity.resource
		adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{
			Revision: "synthetic-v1", Projections: []ObservedProjection{{ID: projectionID, Goal: ProjectionPresent, DesiredFingerprint: "exact", Action: ProjectionAction{ID: projectionID, Target: filepath.Join(root, ".agents", "skills", identity.resource), PreviewOnly: true}}},
		}}}
		preview, err := NewFacade(Catalog{packs: []Pack{pack}}).PreviewProjectInstall(context.Background(), ProjectInstallRequest{PackID: pack.ID, Surface: SurfaceCodex, ProjectRoot: root, Selection: ResourceSelection{Mode: SelectionAll}}, adapter)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			baseline = preview.ExpectedReadiness
		} else if preview.ExpectedReadiness != baseline {
			t.Fatalf("project lifecycle readiness changed by identity: first=%#v second=%#v", baseline, preview.ExpectedReadiness)
		}
		if preview.ExpectedReadiness.Configured != ReadinessTrue || len(preview.Conditions) != 3 {
			t.Fatalf("project lifecycle readiness = %#v conditions=%#v", preview.ExpectedReadiness, preview.Conditions)
		}
	}
}

func TestFacadeStatusDerivesExternalRequirementConditionFromExistingRequirement(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4, ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		ReadinessObligations: []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization},
		Requires:             Requirements{Tools: []string{"helper"}},
		Resources:            []Resource{{Kind: "skill", ID: "guide", Description: "Guide", Requires: []string{}, Conflicts: []string{}, Bindings: testCapabilityBindings("guide"), SurfaceExclusions: []SurfaceExclusion{}}},
		Contract:             Contract{Exclusions: []Exclusion{}, OptionalModes: []OptionalMode{}},
	}
	state := ActivationState{
		Intent:    ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}},
		Ownership: []ProjectionOwnership{{ID: "skill:guide", ProjectionID: "skill:guide", PackID: pack.ID, Surface: SurfaceCodex, Fingerprint: "exact"}},
	}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{
		Revision: "codex-v1", Projections: []ObservedProjection{{ID: "skill:guide", Exists: true, ObservedFingerprint: "exact", DesiredFingerprint: "exact", Action: ProjectionAction{ID: "skill:guide", Target: "/tmp/guide"}}},
	}}}
	facade := NewFacade(Catalog{packs: []Pack{pack}},
		WithActivation(&fakeActivationStore{state: state}, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}),
		WithExternalEffects(readinessResolver{}, nil),
	)

	report, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	entry := report.Entries[0]
	if entry.Readiness.Usable != ReadinessFalse {
		t.Fatalf("usable = %q, conditions = %#v", entry.Readiness.Usable, entry.Conditions)
	}
	count := 0
	for _, condition := range entry.Conditions {
		if condition.Type == ConditionExternalRequirement {
			count++
			if condition.Value != ReadinessFalse || condition.Reason != ReasonRequirementMissing || condition.Evidence[0] != "executable:helper" {
				t.Fatalf("external requirement condition = %#v", condition)
			}
		}
	}
	if count != 1 {
		t.Fatalf("external requirement conditions = %d, want one", count)
	}
}
