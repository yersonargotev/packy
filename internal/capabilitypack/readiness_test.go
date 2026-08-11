package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type readinessResolver struct{ available bool }

func (r readinessResolver) Resolve(_ context.Context, tool string) (ExecutableResolution, error) {
	return ExecutableResolution{Tool: tool, Available: r.available}, nil
}

type recordingReadinessResolver struct {
	paths map[string]string
	calls []string
}

func (r *recordingReadinessResolver) Resolve(_ context.Context, tool string) (ExecutableResolution, error) {
	r.calls = append(r.calls, tool)
	path := r.paths[tool]
	return ExecutableResolution{Tool: tool, Available: path != "", Path: path, ResolvedPath: path, Origin: "path"}, nil
}

type recordingAcquirer struct{ calls int }

func (a *recordingAcquirer) ResolveAcquisition(context.Context) (ExecutableAcquisition, error) {
	a.calls++
	return ExecutableAcquisition{Path: "/opt/homebrew/bin/engram", Command: "brew", Args: []string{"install", "reviewed/engram"}, Source: "reviewed/engram", Version: "1.0.0"}, nil
}

type syntheticRequirementAdapter struct {
	applied      bool
	inspectCalls int
	applyCalls   int
	target       string
}

func (a *syntheticRequirementAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	a.inspectCalls++
	fingerprint := strings.Repeat("a", 64)
	observed := ""
	if a.applied {
		observed = fingerprint
	}
	target := a.target
	if target == "" {
		target = "/tmp/synthetic-guide"
	}
	return SurfaceInspection{
		Revision: "synthetic-adapter-v1",
		Projections: []ObservedProjection{{
			ID: "skill:guide", Goal: ProjectionPresent, Exists: a.applied, ObservedFingerprint: observed, DesiredFingerprint: fingerprint, AdapterProvenance: "synthetic-adapter/v1",
			Action: ProjectionAction{ID: "skill:guide", Kind: ActionSkillLink, Target: target, Description: "project synthetic guide", AdapterProvenance: "synthetic-adapter/v1", PreviewOnly: transition.ProjectRoot != ""},
		}},
	}, nil
}

func (a *syntheticRequirementAdapter) ApplyProjections(_ context.Context, _ []ProjectionAction) *ProjectionActionError {
	a.applyCalls++
	a.applied = true
	return nil
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
		WithExternalEffects(readinessResolver{}, nil, nil),
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

func TestFacadeStatusObservesDifferentlyNamedExternalRequirementsWithoutToolDispatch(t *testing.T) {
	for _, scenario := range []struct {
		tool       string
		path       string
		wantValue  ReadinessValue
		wantReason ReadinessReason
		wantUsable bool
	}{
		{tool: "synthetic-present", path: "/tmp/synthetic-present", wantValue: ReadinessTrue, wantReason: ReasonRequirementAvailable, wantUsable: true},
		{tool: "synthetic-missing", wantValue: ReadinessFalse, wantReason: ReasonRequirementMissing, wantUsable: false},
	} {
		t.Run(scenario.tool, func(t *testing.T) {
			pack := Pack{
				manifestVersion: manifestSchemaV4, ID: "synthetic-pack", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
				Requires:  Requirements{Tools: []string{scenario.tool}},
				Resources: []Resource{{Kind: "skill", ID: "guide", Description: "Guide", Requires: []string{}, Conflicts: []string{}, Bindings: testCapabilityBindings("guide"), SurfaceExclusions: []SurfaceExclusion{}}},
				Contract:  Contract{Exclusions: []Exclusion{}, OptionalModes: []OptionalMode{}},
			}
			state := ActivationState{
				Intent:    ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}},
				Ownership: []ProjectionOwnership{{ID: "skill:guide", ProjectionID: "skill:guide", PackID: pack.ID, Surface: SurfaceCodex, Fingerprint: "exact"}},
			}
			adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "codex-v1", Projections: []ObservedProjection{{ID: "skill:guide", Exists: true, ObservedFingerprint: "exact", DesiredFingerprint: "exact", Action: ProjectionAction{ID: "skill:guide", Target: "/tmp/guide"}}}}}}
			resolver := &recordingReadinessResolver{paths: map[string]string{scenario.tool: scenario.path}}
			store := &fakeActivationStore{state: state}
			facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}), WithExternalEffects(resolver, nil, nil))

			report, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex, RequireUsable: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(resolver.calls) != 1 || resolver.calls[0] != scenario.tool {
				t.Fatalf("resolver calls = %#v", resolver.calls)
			}
			var requirement ReadinessCondition
			for _, condition := range report.Entries[0].Conditions {
				if condition.Type == ConditionExternalRequirement {
					requirement = condition
				}
			}
			if requirement.Value != scenario.wantValue || requirement.Reason != scenario.wantReason || !strings.Contains(requirement.Message, scenario.tool) {
				t.Fatalf("requirement condition = %#v", requirement)
			}
			if report.Requirement == nil || report.Requirement.Satisfied != scenario.wantUsable {
				t.Fatalf("strict usable gate = %#v, want %t", report.Requirement, scenario.wantUsable)
			}
			if len(store.saves) != 0 || len(adapter.applied) != 0 {
				t.Fatalf("status observation caused side effects: saves=%d applied=%d", len(store.saves), len(adapter.applied))
			}
		})
	}
}

func TestExecutableAcquisitionRequiresExplicitReviewedCapability(t *testing.T) {
	resolver := &recordingReadinessResolver{paths: map[string]string{}}
	acquirer := &recordingAcquirer{}
	pack := Pack{
		manifestVersion: manifestSchemaV4, ID: "acquisition", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		Requires: Requirements{Tools: []string{"engram"}}, Contract: Contract{Exclusions: []Exclusion{}, OptionalModes: []OptionalMode{}},
		Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide", Description: "Guide", Requires: []string{}, Conflicts: []string{}, Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "guide", Invocation: "guide", Mode: "native", Sharing: "exclusive", Capabilities: []SurfaceCapability{{Type: SurfaceCapabilityExternalExecutableAcquisition, ExternalExecutableAcquisition: &ExternalExecutableAcquisitionCapability{Tool: "engram"}}}}}, SurfaceExclusions: []SurfaceExclusion{}}},
	}
	adapter := &fakeSurfaceAdapter{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(&fakeActivationStore{}, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}), WithExternalEffects(resolver, map[SurfaceCapabilityType]ExecutableAcquirer{SurfaceCapabilityExternalExecutableAcquisition: acquirer}, nil))

	missing, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex, Selection: ResourceSelection{Mode: SelectionAll}})
	if err != nil {
		t.Fatal(err)
	}
	if acquirer.calls != 1 || !planHasAction(missing, "external:engram:acquire") {
		t.Fatalf("missing executable preview = calls=%d phases=%#v", acquirer.calls, missing.Phases())
	}

	resolver.paths["engram"] = "/opt/reviewed/engram"
	available, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex, Selection: ResourceSelection{Mode: SelectionAll}})
	if err != nil {
		t.Fatal(err)
	}
	if acquirer.calls != 1 {
		t.Fatalf("available executable invoked acquisition: calls=%d", acquirer.calls)
	}
	if planHasAction(available, "external:engram:acquire") {
		t.Fatalf("available executable preview retained acquisition: phases=%#v", available.Phases())
	}
}

func TestExternalPlanDoesNotInventSetupForOrdinaryToolRequirement(t *testing.T) {
	resolution := ExecutableResolution{Tool: "synthetic-helper", Available: true, Path: "/tmp/synthetic-helper", Origin: "path"}
	actions, blockers := (Facade{}).externalPlan(OperationActivate, Pack{}, SurfaceCodex, ActivationState{}, []ExecutableResolution{resolution})
	if len(actions) != 0 || len(blockers) != 0 {
		t.Fatalf("ordinary PATH requirement produced setup policy: actions=%#v blockers=%#v", actions, blockers)
	}

	resolution.Available = false
	resolution.Path = ""
	actions, blockers = (Facade{}).externalPlan(OperationActivate, Pack{}, SurfaceCodex, ActivationState{}, []ExecutableResolution{resolution})
	if len(actions) != 0 || len(blockers) != 1 || !strings.Contains(blockers[0].Detail, "missing from PATH") || !strings.Contains(blockers[0].Detail, "install it and retry") {
		t.Fatalf("missing ordinary requirement = actions=%#v blockers=%#v", actions, blockers)
	}
}

func TestSyntheticExternalRequirementsDrivePreviewApplyStatusAndMissingGate(t *testing.T) {
	packFor := func(id, tool string) Pack {
		return Pack{
			manifestVersion: manifestSchemaV4, ID: id, Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
			Requires:  Requirements{Tools: []string{tool}},
			Resources: []Resource{{Kind: "skill", ID: "guide", Source: "guide", Description: "Synthetic guide", Requires: []string{}, Conflicts: []string{}, Bindings: testCapabilityBindings("guide"), SurfaceExclusions: []SurfaceExclusion{}}},
			Contract:  Contract{Exclusions: []Exclusion{}, OptionalModes: []OptionalMode{}},
		}
	}

	present := packFor("synthetic-present", "present-helper")
	presentAdapter := &syntheticRequirementAdapter{}
	presentStore := &fakeActivationStore{}
	presentResolver := &recordingReadinessResolver{paths: map[string]string{"present-helper": "/tmp/present-helper"}}
	presentFacade := NewFacade(Catalog{packs: []Pack{present}}, WithActivation(presentStore, map[Surface]SurfaceAdapter{SurfaceCodex: presentAdapter}), WithExternalEffects(presentResolver, nil, nil))
	preview, err := presentFacade.Preview(context.Background(), ActivationRequest{PackID: present.ID, Surface: SurfaceCodex, Selection: ResourceSelection{Mode: SelectionAll}})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Disposition() != PlanApplicable || preview.Readiness().Usable != ReadinessTrue || presentAdapter.applyCalls != 0 || len(presentStore.saves) != 0 {
		t.Fatalf("present preview = disposition=%s readiness=%#v apply_calls=%d saves=%d", preview.Disposition(), preview.Readiness(), presentAdapter.applyCalls, len(presentStore.saves))
	}
	approvals := make([]ApprovalReceipt, 0, len(preview.Phases()))
	for _, phase := range preview.Phases() {
		if phase.ApprovalRequired {
			approvals = append(approvals, presentFacade.Approve(preview, phase.Kind))
		}
	}
	result, err := presentFacade.Apply(context.Background(), ApplyRequest{Plan: preview, Approvals: approvals, Interactive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.Readiness.Usable != ReadinessTrue || presentAdapter.applyCalls != 1 || len(presentStore.saves) == 0 {
		t.Fatalf("present apply = result=%#v apply_calls=%d saves=%d", result, presentAdapter.applyCalls, len(presentStore.saves))
	}
	status, err := presentFacade.Status(context.Background(), StatusRequest{PackID: present.ID, Surface: SurfaceCodex, RequireUsable: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.Requirement == nil || !status.Requirement.Satisfied || status.Entries[0].Readiness.Usable != ReadinessTrue {
		t.Fatalf("present strict status = %#v", status)
	}

	missing := packFor("synthetic-missing", "missing-helper")
	missingAdapter := &syntheticRequirementAdapter{}
	missingStore := &fakeActivationStore{}
	missingFacade := NewFacade(Catalog{packs: []Pack{missing}}, WithActivation(missingStore, map[Surface]SurfaceAdapter{SurfaceCodex: missingAdapter}), WithExternalEffects(&recordingReadinessResolver{paths: map[string]string{}}, nil, nil))
	blocked, err := missingFacade.Preview(context.Background(), ActivationRequest{PackID: missing.ID, Surface: SurfaceCodex, Selection: ResourceSelection{Mode: SelectionAll}})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Disposition() != PlanMixed || blocked.Readiness().Usable != ReadinessFalse || missingAdapter.applyCalls != 0 || len(missingStore.saves) != 0 {
		t.Fatalf("missing preview = disposition=%s readiness=%#v apply_calls=%d saves=%d", blocked.Disposition(), blocked.Readiness(), missingAdapter.applyCalls, len(missingStore.saves))
	}
}

func TestProjectActivationPreviewUsesGenericRequirementResolver(t *testing.T) {
	project, packyHome := t.TempDir(), filepath.Join(t.TempDir(), ".packy")
	pack := Pack{
		manifestVersion: manifestSchemaV4, ID: "synthetic-project", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		ReadinessObligations: []ReadinessObligation{ReadinessRuntimeUsability, ReadinessSurfaceAuthorization},
		Requires:             Requirements{Tools: []string{"project-helper"}},
		Resources:            []Resource{{Kind: "skill", ID: "guide", Source: "guide", Description: "Synthetic guide", Requires: []string{}, Conflicts: []string{}, Bindings: testCapabilityBindings("guide"), SurfaceExclusions: []SurfaceExclusion{}}},
		Contract:             Contract{Exclusions: []Exclusion{}, OptionalModes: []OptionalMode{}},
	}
	adapter := &syntheticRequirementAdapter{target: filepath.Join(project, ".agents", "skills", "guide")}
	resolver := &recordingReadinessResolver{paths: map[string]string{"project-helper": "/tmp/project-helper"}}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithExternalEffects(resolver, nil, nil))
	install, err := facade.PreviewProjectInstall(context.Background(), ProjectInstallRequest{PackID: pack.ID, Surface: SurfaceCodex, ProjectRoot: project, Selection: ResourceSelection{Mode: SelectionAll}}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if install.Disposition != ProjectInstallPreviewable {
		t.Fatalf("install disposition = %s blockers=%#v", install.Disposition, install.Blockers)
	}
	manifest, err := marshalProjectManifest(install.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := marshalProjectLock(install.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), lock, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "PACKY-NOTICES.md"), []byte(install.noticeContent), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter.applied = true
	preview, err := facade.PreviewProjectActivation(context.Background(), ProjectActivationRequest{PackID: pack.ID, Surface: SurfaceCodex, ProjectRoot: project, PackyHome: packyHome, Adapter: adapter})
	if err != nil {
		t.Fatal(err)
	}
	var requirement ReadinessCondition
	for _, condition := range preview.Conditions {
		if condition.Type == ConditionExternalRequirement {
			requirement = condition
		}
	}
	if requirement.Value != ReadinessTrue || requirement.Reason != ReasonRequirementAvailable {
		t.Fatalf("project activation requirement = %#v", requirement)
	}
}
