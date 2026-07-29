package capabilitypack

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIssue291CustomClosureAliasesAndAuthorityDisclosure(t *testing.T) {
	pack := Pack{
		manifestVersion: manifestSchemaV4,
		ID:              "typed-closure", Version: "1", Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: "instruction", ID: "root", Requires: []string{"skill:shared"}, Permissions: []string{"filesystem-read"}, Bindings: issue291Bindings("root")},
			{Kind: "skill", ID: "other", Permissions: []string{"network"}, Bindings: issue291Bindings("other")},
			{
				Kind: "skill", ID: "shared", Permissions: []string{"secret-use"}, Bindings: issue291Bindings("shared"),
				RuntimeModes: []RuntimeMode{{
					ID: "publish", Role: RuntimeModePrimary,
					Authorities: []RuntimeAuthority{{Kind: RuntimeAuthorityGitPush, Scope: RuntimeScopeRemoteGit}},
					Effects:     []RuntimeEffect{{Kind: RuntimeEffectRemoteGitChange, Scope: RuntimeScopeRemoteGit}},
				}},
			},
		},
	}
	runtimeEvidence, err := UnverifiedRuntimeModeEvidence(pack, time.Now().UTC(), "host")
	if err != nil {
		t.Fatal(err)
	}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "host", RuntimeModeEvidence: runtimeEvidence}}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	selection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "instruction", ID: "root"}}}

	alias := SurfaceAlias{Kind: "skill", ID: "shared", Name: "shared-for-root"}
	plan, err := facade.Preview(context.Background(), ActivationRequest{
		PackID: "typed-closure", Surface: SurfaceCodex, Selection: selection, Aliases: []SurfaceAlias{alias},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Aliases(), []SurfaceAlias{alias}) {
		t.Fatalf("closure alias = %+v", plan.Aliases())
	}
	report := plan.JSONReport(true)
	facts := report.ResourceGraph.Resources
	if len(facts) != 2 {
		t.Fatalf("selected resource graph = %+v", facts)
	}
	byID := map[string]ResourceClosureFact{}
	for _, fact := range facts {
		byID[fact.Resource.String()] = fact
	}
	if _, ok := byID["skill:other"]; ok {
		t.Fatalf("unselected resource authority was disclosed: %+v", facts)
	}
	if len(report.SensitiveEffects) != 2 {
		t.Fatalf("sensitive effect origins = %+v", report.SensitiveEffects)
	}
	origins := map[string]SensitiveEffectOrigin{}
	for _, origin := range report.SensitiveEffects {
		if origin.Pack != pack.ID {
			t.Fatalf("sensitive effect pack = %q", origin.Pack)
		}
		origins[origin.Resource.String()] = origin
	}
	if got := origins["instruction:root"]; !reflect.DeepEqual(got.PromptAuthorities, []string{"filesystem-read"}) ||
		got.Root.String() != "instruction:root" ||
		!reflect.DeepEqual(got.DependencyChain, []ResourceIdentity{{Kind: "instruction", ID: "root"}}) {
		t.Fatalf("root disclosure = %+v", got)
	}
	if got := origins["skill:shared"]; !reflect.DeepEqual(got.PromptAuthorities, []string{"secret-use"}) ||
		!reflect.DeepEqual(got.RuntimeAuthorities, []RuntimeAuthorityOrigin{{ModeID: "publish", Kind: RuntimeAuthorityGitPush, Scope: RuntimeScopeRemoteGit}}) ||
		!reflect.DeepEqual(got.RuntimeEffects, []RuntimeEffectOrigin{{ModeID: "publish", Kind: RuntimeEffectRemoteGitChange, Scope: RuntimeScopeRemoteGit}}) ||
		got.Root.String() != "instruction:root" ||
		!reflect.DeepEqual(got.DependencyChain, []ResourceIdentity{{Kind: "instruction", ID: "root"}, {Kind: "skill", ID: "shared"}}) {
		t.Fatalf("dependency disclosure = %+v", got)
	}
	if _, ok := origins["skill:other"]; ok {
		t.Fatalf("unselected sensitive effect origin was disclosed: %+v", report.SensitiveEffects)
	}

	_, err = facade.Preview(context.Background(), ActivationRequest{
		PackID: "typed-closure", Surface: SurfaceCodex, Selection: selection,
		Aliases: []SurfaceAlias{{Kind: "skill", ID: "other", Name: "not-selected"}},
	})
	if err == nil || !strings.Contains(err.Error(), "selected") {
		t.Fatalf("unselected alias error = %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("rejected alias crossed mutation boundary: actions=%d saves=%d", len(adapter.actions), len(store.saves))
	}
}

func TestIssue291SensitiveEffectsCoverEveryRetainingRootAndProviderPack(t *testing.T) {
	rootA := ResourceIdentity{Kind: "instruction", ID: "a"}
	rootB := ResourceIdentity{Kind: "instruction", ID: "b"}
	shared := ResourceIdentity{Kind: "skill", ID: "shared"}
	providerRoot := ResourceIdentity{Kind: "skill", ID: "storage"}
	providerDependency := ResourceIdentity{Kind: "skill", ID: "helper"}
	left := ResourceIdentity{Kind: "skill", ID: "left"}
	right := ResourceIdentity{Kind: "skill", ID: "right"}
	consumer := Pack{manifestVersion: manifestSchemaV4, ID: "consumer", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: rootA.Kind, ID: rootA.ID, Requires: []string{left.String(), right.String()}, Bindings: issue291Bindings(rootA.ID)},
		{Kind: rootB.Kind, ID: rootB.ID, Requires: []string{shared.String()}, Bindings: issue291Bindings(rootB.ID)},
		{Kind: left.Kind, ID: left.ID, Requires: []string{shared.String()}, Bindings: issue291Bindings(left.ID)},
		{Kind: right.Kind, ID: right.ID, Requires: []string{shared.String()}, Bindings: issue291Bindings(right.ID)},
		{Kind: shared.Kind, ID: shared.ID, Permissions: []string{"network"}, RequiresCapabilities: []string{"cap:storage"}, Bindings: issue291Bindings(shared.ID)},
	}}
	provider := Pack{manifestVersion: manifestSchemaV4, ID: "provider", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: providerRoot.Kind, ID: providerRoot.ID, Permissions: []string{"filesystem-write"}, Requires: []string{providerDependency.String()}, ProvidesCapabilities: []string{"cap:storage"}, Bindings: issue291Bindings(providerRoot.ID)},
		{Kind: providerDependency.Kind, ID: providerDependency.ID, Permissions: []string{"filesystem-read"}, Bindings: issue291Bindings(providerDependency.ID)},
	}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "host"}}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{consumer, provider}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{
		PackID: consumer.ID, Surface: SurfaceCodex,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{rootA, rootB}},
	})
	if err != nil {
		t.Fatal(err)
	}
	origins := plan.SensitiveEffects()
	want := map[string][]ResourceIdentity{
		"consumer|instruction:a|skill:left|skill:shared":  {rootA, left, shared},
		"consumer|instruction:a|skill:right|skill:shared": {rootA, right, shared},
		"consumer|instruction:b|skill:shared":             {rootB, shared},
		"provider|skill:storage":                          {providerRoot},
		"provider|skill:storage|skill:helper":             {providerRoot, providerDependency},
	}
	if len(origins) != len(want) {
		t.Fatalf("sensitive effects = %+v", origins)
	}
	for _, origin := range origins {
		key := origin.Pack + "|" + strings.ReplaceAll(identityChainKey(origin.DependencyChain), "\x00", "|")
		if !reflect.DeepEqual(origin.DependencyChain, want[key]) {
			t.Fatalf("origin %s = %+v", key, origin)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing sensitive effects = %+v", want)
	}

	tampered := plan
	tampered.sensitiveEffects = cloneSensitiveEffectOrigins(plan.sensitiveEffects)
	tampered.sensitiveEffects[0].Pack = "tampered"
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: tampered, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("tampered sensitive effects error = %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("tampered provenance crossed mutation boundary: actions=%d saves=%d", len(adapter.actions), len(store.saves))
	}
}

func TestIssue291SharedDependencyAliasTracksTheResultingClosure(t *testing.T) {
	rootA := ResourceIdentity{Kind: "instruction", ID: "a"}
	rootB := ResourceIdentity{Kind: "instruction", ID: "b"}
	shared := ResourceIdentity{Kind: "skill", ID: "shared"}
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "closure-lifecycle", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: rootA.Kind, ID: rootA.ID, Requires: []string{shared.String()}, Bindings: issue291Bindings(rootA.ID)},
		{Kind: rootB.Kind, ID: rootB.ID, Requires: []string{shared.String()}, Bindings: issue291Bindings(rootB.ID)},
		{Kind: shared.Kind, ID: shared.ID, Bindings: issue291Bindings(shared.ID)},
	}}
	alias := SurfaceAlias{Kind: shared.Kind, ID: shared.ID, Name: "retained-shared"}
	selection := ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{rootA, rootB}}
	state := ActivationState{Intent: ActivationIntent{
		PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 2,
		Selection: selection, Aliases: []SurfaceAlias{alias},
	}}
	inspection := SurfaceInspection{Revision: "host"}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{inspection, inspection, inspection, inspection}}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	removeA, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{
		PackID: pack.ID, Surface: SurfaceCodex, Resources: []ResourceIdentity{rootA},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(removeA.Aliases(), []SurfaceAlias{alias}) {
		t.Fatalf("shared dependency alias did not survive retained root: %+v", removeA.Aliases())
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: removeA, Interactive: true}); err != nil {
		t.Fatal(err)
	}

	removeFinal, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{
		PackID: pack.ID, Surface: SurfaceCodex, Resources: []ResourceIdentity{rootB},
	})
	if err != nil {
		t.Fatal(err)
	}
	if aliases := removeFinal.Aliases(); len(aliases) != 0 {
		t.Fatalf("alias outside resulting closure survived final root removal: %+v", aliases)
	}
}

func TestIssue291ConsentKindsStaySeparateAndMissingReceiptIsZeroMutation(t *testing.T) {
	pack := Pack{ID: "typed-consent", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "instruction", ID: "root", Bindings: issue291Bindings("root")},
	}}
	inspection := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{
		{ID: "local", DesiredFingerprint: "local", Action: ProjectionAction{ID: "local"}},
		{ID: "external", DesiredFingerprint: "external", Action: ProjectionAction{ID: "external", Consent: ConsentExecutableExternal}},
	}, PendingHumanActions: []string{"reload host"}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{inspection, inspection}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	phases := plan.Phases()
	kinds := []ConsentKind{ConsentReversibleLocal, ConsentExecutableExternal, ConsentHostFollowUp}
	if len(phases) != len(kinds) {
		t.Fatalf("consent phases = %+v", phases)
	}
	for i, kind := range kinds {
		if phases[i].Kind != kind || len(phases[i].Actions) != 1 || phases[i].Digest == "" {
			t.Fatalf("phase %d = %+v", i, phases[i])
		}
	}
	approvals := []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: approvals, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("missing typed receipt error = %v", err)
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatalf("missing receipt crossed mutation boundary: actions=%d saves=%d", len(adapter.actions), len(store.saves))
	}

	owned := ActivationState{
		Intent:    ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true},
		Ownership: []ProjectionOwnership{{ID: "instruction:root", Contributors: []string{"pack:typed-consent:instruction:root"}, Fingerprint: "owned"}},
	}
	remove := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{
		ID: "instruction:root", Exists: true, ObservedFingerprint: "owned",
		Action: ProjectionAction{ID: "instruction:root"},
	}}}
	removeAdapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{remove, remove}}
	removeStore := &fakeActivationStore{state: owned}
	removeFacade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(removeStore, map[Surface]SurfaceAdapter{SurfaceCodex: removeAdapter}))
	removePlan, err := removeFacade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	removePhases := removePlan.Phases()
	if len(removePhases) != 1 || removePhases[0].Kind != ConsentDestructiveCleanup || !removePhases[0].ApprovalRequired || removePhases[0].Digest == "" {
		t.Fatalf("destructive consent phase = %+v", removePhases)
	}
	if _, err := removeFacade.Apply(context.Background(), ApplyRequest{Plan: removePlan, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("missing destructive receipt error = %v", err)
	}
	if len(removeAdapter.actions) != 0 || len(removeStore.saves) != 0 {
		t.Fatalf("missing destructive receipt crossed mutation boundary: actions=%d saves=%d", len(removeAdapter.actions), len(removeStore.saves))
	}
}

func TestIssue291SecretLikeActionDataIsNotReportedOrPersistedAfterFailure(t *testing.T) {
	const secret = "issue291-super-secret"
	pack := Pack{ID: "redacted", Version: "1", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "instruction", ID: "root", Bindings: issue291Bindings("root")},
	}}
	inspection := SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{
		ID: "instruction:root", DesiredFingerprint: "desired",
		Action: ProjectionAction{
			ID: "instruction:root", Consent: ConsentExecutableExternal, Content: secret,
			Args: []string{"tool", "--env", "TOKEN=" + secret}, Description: "configure secret safely",
		},
	}}, Readiness: ReadinessObservation{Evidence: []string{"observed TOKEN=" + secret}},
		PendingHumanActions: []string{"copy " + secret + " into the host"}}
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{inspection, inspection}, applyErr: &ProjectionActionError{ID: "instruction:root", Err: errors.New("adapter failed: " + secret)}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	report, err := json.Marshal(plan.JSONReport(true))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(report), secret) {
		t.Fatalf("JSON preview leaked action/evidence secret: %s", report)
	}
	_, applyErr := facade.Apply(context.Background(), ApplyRequest{
		Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentExecutableExternal)}, Interactive: true,
	})
	if applyErr == nil {
		t.Fatal("adapter failure unexpectedly succeeded")
	}
	state, err := json.Marshal(store.state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(applyErr.Error(), secret) || strings.Contains(string(state), secret) {
		t.Fatalf("secret leaked through safe error or durable state: err=%q state=%s", applyErr, state)
	}
}

func issue291Bindings(name string) []Binding {
	return []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: name, Invocation: "$" + name, Mode: "native", Sharing: "exclusive"}}
}
