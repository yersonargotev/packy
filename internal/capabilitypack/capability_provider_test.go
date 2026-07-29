package capabilitypack

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPreviewResolvesSelectedV4CapabilityProviderRootAndInternalClosure(t *testing.T) {
	empty := []string{}
	consumer := Pack{
		manifestVersion: manifestSchemaV4, ID: "consumer", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: "instruction", ID: "inactive", Notices: empty, ProvidesCapabilities: []string{"cap:inactive"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: []string{"cap:storage"}, Bindings: testCapabilityBindings("inactive")},
			{Kind: "instruction", ID: "root", Notices: empty, ProvidesCapabilities: empty, RequiresCapabilities: []string{"cap:storage"}, RequiresTools: []string{"consumer-tool"}, CapabilityConflicts: empty, Bindings: testCapabilityBindings("root"), RuntimeModes: []RuntimeMode{{ID: "consumer", Authorities: []RuntimeAuthority{{Kind: RuntimeAuthorityNetwork, Scope: RuntimeScopeRemoteGit}}}}},
		},
	}
	provider := Pack{
		manifestVersion: manifestSchemaV4, ID: "provider", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		Resources: []Resource{
			{Kind: "asset", ID: "data", Notices: empty, ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
			{Kind: "skill", ID: "helper", Requires: []string{"asset:data"}, Notices: empty, ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty, Bindings: testCapabilityBindings("helper"), RuntimeModes: []RuntimeMode{{ID: "dependency", Authorities: []RuntimeAuthority{{Kind: RuntimeAuthorityGitInspect, Scope: RuntimeScopeLocalGit}}}}},
			{Kind: "skill", ID: "storage", Requires: []string{"skill:helper"}, Notices: empty, ProvidesCapabilities: []string{"cap:storage"}, RequiresCapabilities: empty, RequiresTools: []string{"provider-tool"}, CapabilityConflicts: empty, Permissions: []string{"filesystem-read"}, Bindings: testCapabilityBindings("storage"), RuntimeModes: []RuntimeMode{{ID: "provider", Authorities: []RuntimeAuthority{{Kind: RuntimeAuthorityFilesystemRead, Scope: RuntimeScopeConsumerProject}}}}},
			{Kind: "skill", ID: "unrelated", Notices: empty, ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty, Bindings: testCapabilityBindings("unrelated")},
		},
	}
	observedAt := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{{Revision: "host", RuntimeModeEvidence: []RuntimeModeEvidence{
		{ResourceID: "root", ModeID: "consumer", Evidence: RuntimeEvidence{Requirements: []RuntimeRequirementObservation{}, Authorities: []RuntimeAuthorityObservation{{
			Kind: RuntimeAuthorityNetwork, Scope: RuntimeScopeRemoteGit, RuntimeObservation: RuntimeObservation{State: ObservationAvailable, Reason: ObservationReasonVerified, ObservedAt: observedAt, ObserverRevision: "host"},
		}}}},
		{ResourceID: "storage", ModeID: "provider", Evidence: RuntimeEvidence{Requirements: []RuntimeRequirementObservation{}, Authorities: []RuntimeAuthorityObservation{{
			Kind: RuntimeAuthorityFilesystemRead, Scope: RuntimeScopeConsumerProject, RuntimeObservation: RuntimeObservation{State: ObservationAvailable, Reason: ObservationReasonVerified, ObservedAt: observedAt, ObserverRevision: "host"},
		}}}},
		{ResourceID: "helper", ModeID: "dependency", Evidence: RuntimeEvidence{Requirements: []RuntimeRequirementObservation{}, Authorities: []RuntimeAuthorityObservation{{
			Kind: RuntimeAuthorityGitInspect, Scope: RuntimeScopeLocalGit, RuntimeObservation: RuntimeObservation{State: ObservationAvailable, Reason: ObservationReasonVerified, ObservedAt: observedAt, ObserverRevision: "host"},
		}}}},
	}}}}
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{
		{Tool: "consumer-tool", Available: true, Path: "/tools/consumer", ResolvedPath: "/tools/consumer", Origin: "path", Precondition: "available"},
		{Tool: "provider-tool", Available: true, Path: "/tools/provider", ResolvedPath: "/tools/provider", Origin: "path", Precondition: "available"},
	}}
	facade := NewFacade(
		Catalog{packs: []Pack{consumer, provider}},
		WithActivation(&fakeActivationStore{}, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}),
		WithExternalEffects(resolver, nil),
	)
	plan, err := facade.Preview(context.Background(), ActivationRequest{
		PackID: "consumer", Surface: SurfaceCodex,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "instruction", ID: "root"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if blockers := plan.Blockers(); len(blockers) != 0 {
		t.Fatalf("blockers = %#v", blockers)
	}
	activations := plan.Activations()
	if len(activations) != 2 || activations[1].Pack.ID != "provider" ||
		!reflect.DeepEqual(activations[1].Selection.Roots, []ResourceIdentity{{Kind: "skill", ID: "storage"}}) {
		t.Fatalf("activations = %#v", activations)
	}
	if got := resourceIDs(adapter.calls[0].desired.Resources); !reflect.DeepEqual(got, []string{"instruction:root", "asset:data", "skill:helper", "skill:storage"}) {
		t.Fatalf("desired resources = %v", got)
	}
	if got := adapter.calls[0].desired.Requires.Tools; !reflect.DeepEqual(got, []string{"consumer-tool", "provider-tool"}) {
		t.Fatalf("required tools = %v", got)
	}
	facts := plan.CapabilityRequirements()
	if len(facts) != 1 || facts[0].ConsumerPack != "consumer" || facts[0].ConsumerResource == nil || facts[0].ConsumerResource.String() != "instruction:root" ||
		facts[0].ProviderPack != "provider" || facts[0].ProviderResource == nil || facts[0].ProviderResource.String() != "skill:storage" ||
		!reflect.DeepEqual(facts[0].RequiredTools, []string{"consumer-tool"}) ||
		!reflect.DeepEqual(facts[0].RequiredAuthority, []string{"filesystem-read", "filesystem_read:consumer_project", "git_inspect:local_git", "network:remote_git"}) {
		t.Fatalf("capability facts = %#v", facts)
	}
	if report := plan.JSONReport(true); !reflect.DeepEqual(report.CapabilityRequirements, facts) {
		t.Fatalf("JSON capability facts = %#v, want %#v", report.CapabilityRequirements, facts)
	}
}

func TestActiveV4ProviderSelectionAddsRequiredProviderRoot(t *testing.T) {
	empty := []string{}
	consumer := Pack{manifestVersion: manifestSchemaV4, ID: "consumer", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "instruction", ID: "root", Notices: empty, Bindings: testCapabilityBindings("root"),
		ProvidesCapabilities: empty, RequiresCapabilities: []string{"cap:storage"}, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	provider := Pack{manifestVersion: manifestSchemaV4, ID: "provider", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "skill", ID: "storage", Notices: empty, Bindings: testCapabilityBindings("storage"), ProvidesCapabilities: []string{"cap:storage"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
		{Kind: "skill", ID: "unrelated", Notices: empty, Bindings: testCapabilityBindings("unrelated"), ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
	}}
	providerIntent := ActivationIntent{PackID: "provider", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "unrelated"}}}}
	store := &fakeActivationStore{state: ActivationState{Intent: providerIntent, Intents: []ActivationIntent{providerIntent}}}
	adapter := &fakeSurfaceAdapter{}
	facade := NewFacade(Catalog{packs: []Pack{consumer, provider}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "consumer", Surface: SurfaceCodex,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "instruction", ID: "root"}}}})
	if err != nil {
		t.Fatal(err)
	}
	activations := plan.Activations()
	if len(activations) != 2 || activations[1].Pack.ID != "provider" ||
		!reflect.DeepEqual(activations[1].Selection.Roots, []ResourceIdentity{{Kind: "skill", ID: "storage"}, {Kind: "skill", ID: "unrelated"}}) {
		t.Fatalf("merged provider activation = %#v", activations)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	intent, ok := intentForPack(store.state, "provider", SurfaceCodex)
	if !ok || !reflect.DeepEqual(intent.Selection, activations[1].Selection) {
		t.Fatalf("persisted provider selection = %#v, want %#v", intent.Selection, activations[1].Selection)
	}
}

func TestSelectedResourcesConflictWithinOneV4Pack(t *testing.T) {
	empty := []string{}
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "skill", ID: "a", Notices: empty, Bindings: testCapabilityBindings("a"), ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: []string{"cap:b"}},
		{Kind: "skill", ID: "b", Notices: empty, Bindings: testCapabilityBindings("b"), ProvidesCapabilities: []string{"cap:b"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
	}}
	composed, err := NewFacade(Catalog{packs: []Pack{pack}}).compose(pack, ActivationState{}, SurfaceCodex, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(composed.blockers) != 1 || composed.blockers[0].Kind != BlockerCapabilityConflict {
		t.Fatalf("blockers = %#v", composed.blockers)
	}
}

func TestManifestV4RequiresResourceCapabilityArraysAndRestrictsMetadataProviders(t *testing.T) {
	missing := strings.Replace(validManifestV4, `    "requires_tools": [],
`, "", 1)
	if _, err := LoadPortableManifest(writeManifestV4(t, missing), t.TempDir()); err == nil || !strings.Contains(err.Error(), "required non-null arrays") {
		t.Fatalf("missing v4 resource capability arrays error = %v", err)
	}
	base := Pack{manifestVersion: manifestSchemaV4, ID: "example", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex},
		Provides: []string{}, Requires: Requirements{Capabilities: []string{}, Tools: []string{}}, Conflicts: []string{}, Contract: Contract{Exclusions: []Exclusion{}},
	}
	empty := []string{}
	base.Resources = []Resource{{Kind: "notice", ID: "terms", Source: "NOTICE", License: "MIT", Attribution: "Example", Requires: empty,
		Bindings: []Binding{}, SurfaceExclusions: []SurfaceExclusion{}, ProvidesCapabilities: []string{"cap:bad"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty}}
	if err := validatePackMetadataWithContract(base, manifestSchemaV4, true); err == nil || !strings.Contains(err.Error(), "notice capability and tool arrays must be empty") {
		t.Fatalf("notice capability error = %v", err)
	}
	base.Resources = []Resource{{Kind: "asset", ID: "data", Source: "data.txt", Requires: empty, Conflicts: empty, Notices: empty,
		Bindings: []Binding{}, SurfaceExclusions: []SurfaceExclusion{}, ProvidesCapabilities: []string{"cap:bad"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty}}
	if err := validatePackMetadataWithContract(base, manifestSchemaV4, true); err == nil || !strings.Contains(err.Error(), "non-rootable asset cannot provide capabilities") {
		t.Fatalf("asset provider error = %v", err)
	}
}

func TestCompositionNeverUsesNoticeCapabilityOrToolFacts(t *testing.T) {
	empty := []string{}
	consumer := Pack{manifestVersion: manifestSchemaV4, ID: "consumer", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "skill", ID: "root", Bindings: testCapabilityBindings("root"), ProvidesCapabilities: empty, RequiresCapabilities: []string{"cap:notice"}, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	invalidNoticeProvider := Pack{manifestVersion: manifestSchemaV4, ID: "metadata", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "notice", ID: "terms", ProvidesCapabilities: []string{"cap:notice"}, RequiresCapabilities: empty, RequiresTools: []string{"notice-tool"}, CapabilityConflicts: empty,
	}}}
	composed, err := NewFacade(Catalog{packs: []Pack{consumer, invalidNoticeProvider}}).compose(consumer, ActivationState{}, SurfaceCodex, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(composed.blockers) != 1 || composed.blockers[0].Kind != BlockerDependency {
		t.Fatalf("notice capability affected composition: %#v", composed.blockers)
	}
	if tools := composed.combinedPack().Requires.Tools; len(tools) != 0 {
		t.Fatalf("notice tools affected composition: %v", tools)
	}
}

func TestPortableManifestV4CapabilityContractIsCanonicalAndResourceLocal(t *testing.T) {
	manifest := strings.Replace(validManifestV4, `"provides_capabilities": []`, `"provides_capabilities": ["cap:storage"]`, 1)
	manifest = strings.Replace(manifest, `"requires_tools": []`, `"requires_tools": ["storage-cli"]`, 1)
	pack, err := LoadPortableManifest(writeManifestV4(t, manifest), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	resource := pack.Resources[0]
	if !reflect.DeepEqual(resource.ProvidesCapabilities, []string{"cap:storage"}) || !reflect.DeepEqual(resource.RequiresTools, []string{"storage-cli"}) {
		t.Fatalf("resource capability contract = %#v", resource)
	}

	noncanonical := strings.Replace(manifest, `"requires_tools": ["storage-cli"]`, `"requires_tools": ["z-tool", "a-tool"]`, 1)
	if _, err := LoadPortableManifest(writeManifestV4(t, noncanonical), t.TempDir()); err == nil || !strings.Contains(err.Error(), "requires_tools must be a sorted set") {
		t.Fatalf("noncanonical resource tools error = %v", err)
	}
	crossPackDependency := strings.Replace(manifest, `"requires": []`, `"requires": ["other/skill:data"]`, 1)
	if _, err := LoadPortableManifest(writeManifestV4(t, crossPackDependency), t.TempDir()); err == nil || !strings.Contains(err.Error(), "must be <kind>:<id>") {
		t.Fatalf("cross-Pack resource dependency error = %v", err)
	}
}

func testCapabilityBindings(name string) []Binding {
	return []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: name, Mode: "native", Sharing: "exclusive"}}
}

func resourceIDs(resources []Resource) []string {
	result := make([]string, len(resources))
	for i, resource := range resources {
		result[i] = resource.Kind + ":" + resource.ID
	}
	return result
}

func TestLegacyCapabilityProviderStillContributesWholePack(t *testing.T) {
	consumer := Pack{manifestVersion: manifestSchemaV3, ID: "consumer", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: []string{"cap:legacy"}}, Resources: []Resource{{Kind: "instruction", ID: "root"}}}
	provider := Pack{manifestVersion: manifestSchemaV3, ID: "provider", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Provides: []string{"cap:legacy"}, Resources: []Resource{{Kind: "asset", ID: "one"}, {Kind: "skill", ID: "two"}}}
	composed, err := NewFacade(Catalog{packs: []Pack{consumer, provider}}).compose(consumer, ActivationState{}, SurfaceCodex, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := resourceIDs(composed.combinedPack().Resources); !reflect.DeepEqual(got, []string{"asset:one", "instruction:root", "skill:two"}) {
		t.Fatalf("legacy composition = %v", got)
	}
}

func TestExplicitProviderChoiceSelectsOneEligibleProviderAndPersistsWithConsumer(t *testing.T) {
	empty := []string{}
	consumer := Pack{manifestVersion: manifestSchemaV4, ID: "consumer", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "skill", ID: "root", Bindings: testCapabilityBindings("root"), ProvidesCapabilities: empty, RequiresCapabilities: []string{"cap:storage"}, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	provider := func(id string) Pack {
		return Pack{manifestVersion: manifestSchemaV4, ID: id, Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
			Kind: "skill", ID: "storage", Bindings: testCapabilityBindings(id), ProvidesCapabilities: []string{"cap:storage"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty,
		}}}
	}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{consumer, provider("a"), provider("b")}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{}}))
	resource := ResourceIdentity{Kind: "skill", ID: "storage"}
	request := ActivationRequest{PackID: "consumer", Surface: SurfaceCodex,
		Selection:       ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "root"}}},
		ProviderChoices: []ProviderChoice{{Capability: "cap:storage", ProviderPack: "b", ProviderResource: &resource}},
	}
	plan, err := facade.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Activations()
	if len(got) != 2 || got[0].Pack.ID != "b" || got[0].Role != ActivationRequired {
		t.Fatalf("activations = %#v", got)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	intent, ok := intentForPack(store.state, "consumer", SurfaceCodex)
	if !ok || !reflect.DeepEqual(intent.ProviderChoices, request.ProviderChoices) {
		t.Fatalf("consumer provider choices = %#v", intent.ProviderChoices)
	}
	providerIntent, ok := intentForPack(store.state, "b", SurfaceCodex)
	if !ok || providerIntent.Explicit == nil || *providerIntent.Explicit {
		t.Fatalf("provider intent role = %#v", providerIntent)
	}
	deactivation, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "consumer", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: deactivation, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	providerIntent, ok = intentForPack(store.state, "b", SurfaceCodex)
	if !ok || providerIntent.Active {
		t.Fatalf("required-only provider survived final consumer removal: %#v", providerIntent)
	}
}

func TestProviderChoiceFailuresAreDeterministicAndMutationFree(t *testing.T) {
	empty := []string{}
	consumer := Pack{manifestVersion: manifestSchemaV4, ID: "consumer", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "skill", ID: "root", Bindings: testCapabilityBindings("root"), ProvidesCapabilities: empty, RequiresCapabilities: []string{"cap:storage"}, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	provider := Pack{manifestVersion: manifestSchemaV4, ID: "provider", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "skill", ID: "storage", Bindings: testCapabilityBindings("storage"), ProvidesCapabilities: []string{"cap:storage"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{consumer, provider}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{}}))
	choices := []ProviderChoice{{Capability: "cap:storage", ProviderPack: "missing"}, {Capability: "cap:storage", ProviderPack: "provider"}}
	_, err := facade.Preview(context.Background(), ActivationRequest{PackID: "consumer", Surface: SurfaceCodex, ProviderChoices: choices})
	if err == nil || !strings.Contains(err.Error(), "duplicate provider choice") {
		t.Fatalf("duplicate choice error = %v", err)
	}
	if len(store.saves) != 0 {
		t.Fatalf("preview mutated state: saves = %d", len(store.saves))
	}
}

func TestProviderStatusNamesEveryConsumerAndRequiredOnlyCleanupWaitsForLast(t *testing.T) {
	empty := []string{}
	consumer := func(id string) Pack {
		return Pack{manifestVersion: manifestSchemaV4, ID: id, Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
			Kind: "skill", ID: id, Bindings: testCapabilityBindings(id), ProvidesCapabilities: empty, RequiresCapabilities: []string{"cap:storage"}, RequiresTools: empty, CapabilityConflicts: empty,
		}}}
	}
	provider := Pack{manifestVersion: manifestSchemaV4, ID: "provider", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "skill", ID: "storage", Bindings: testCapabilityBindings("storage"), ProvidesCapabilities: []string{"cap:storage"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{consumer("consumer-a"), consumer("consumer-b"), provider}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{}}))
	applyActivation := func(packID string) {
		t.Helper()
		plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: packID, Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
			t.Fatal(err)
		}
	}
	applyDeactivation := func(packID string) {
		t.Helper()
		plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: packID, Surface: SurfaceCodex})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
			t.Fatal(err)
		}
	}
	applyActivation("consumer-a")
	applyActivation("consumer-b")

	report, err := facade.Status(context.Background(), StatusRequest{PackID: "provider", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	entry := report.Entries[0]
	if entry.ActivationRole != ActivationRequired || len(entry.Consumers) != 2 ||
		entry.Consumers[0].ConsumerPack != "consumer-a" || entry.Consumers[1].ConsumerPack != "consumer-b" {
		t.Fatalf("provider status = role %s consumers %#v", entry.ActivationRole, entry.Consumers)
	}
	providerRemoval, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "provider", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if providerRemoval.Applicable() || len(providerRemoval.Blockers()) == 0 {
		t.Fatalf("provider deactivation ignored persisted consumers: %#v", providerRemoval.Blockers())
	}

	applyDeactivation("consumer-a")
	if intent, ok := intentForPack(store.state, "provider", SurfaceCodex); !ok || !intent.Active {
		t.Fatalf("provider removed while one consumer remained: %#v", intent)
	}
	applyDeactivation("consumer-b")
	if intent, ok := intentForPack(store.state, "provider", SurfaceCodex); !ok || intent.Active {
		t.Fatalf("required-only provider survived final consumer: %#v", intent)
	}
}

func TestExplicitProviderSurvivesFinalConsumerRemoval(t *testing.T) {
	empty := []string{}
	consumer := Pack{manifestVersion: manifestSchemaV4, ID: "consumer", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "skill", ID: "consumer", Bindings: testCapabilityBindings("consumer"), ProvidesCapabilities: empty, RequiresCapabilities: []string{"cap:storage"}, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	provider := Pack{manifestVersion: manifestSchemaV4, ID: "provider", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "skill", ID: "storage", Bindings: testCapabilityBindings("storage"), ProvidesCapabilities: []string{"cap:storage"}, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	store := &fakeActivationStore{}
	facade := NewFacade(Catalog{packs: []Pack{consumer, provider}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{}}))
	for _, request := range []ActivationRequest{
		{PackID: "consumer", Surface: SurfaceCodex},
		{PackID: "provider", Surface: SurfaceCodex, Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "storage"}}}},
	} {
		plan, err := facade.Preview(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "consumer", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	intent, ok := intentForPack(store.state, "provider", SurfaceCodex)
	if !ok || !intent.Active || !intentIsExplicit(intent) {
		t.Fatalf("explicit provider was not retained: %#v", intent)
	}
}

func TestUpdateBlocksInvalidPersistedProviderChoiceWithoutMutation(t *testing.T) {
	empty := []string{}
	consumer := Pack{manifestVersion: manifestSchemaV4, ID: "consumer", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "skill", ID: "consumer", Bindings: testCapabilityBindings("consumer"), ProvidesCapabilities: empty, RequiresCapabilities: []string{"cap:storage"}, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	provider := Pack{manifestVersion: manifestSchemaV4, ID: "provider", Version: "2.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{
		Kind: "skill", ID: "storage", Bindings: testCapabilityBindings("storage"), ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	resource := ResourceIdentity{Kind: "skill", ID: "storage"}
	explicit, required := true, false
	consumerIntent := ActivationIntent{PackID: "consumer", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4, Selection: ResourceSelection{Mode: SelectionAll},
		ProviderChoices: []ProviderChoice{{Capability: "cap:storage", ProviderPack: "provider", ProviderResource: &resource}}, Explicit: &explicit}
	providerIntent := ActivationIntent{PackID: "provider", Surface: SurfaceCodex, Version: "1.0.0", Active: true, Revision: 4,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{resource}}, Explicit: &required}
	store := &fakeActivationStore{state: ActivationState{Intent: consumerIntent, Intents: []ActivationIntent{consumerIntent, providerIntent}}}
	facade := NewFacade(Catalog{packs: []Pack{consumer, provider}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: &fakeSurfaceAdapter{}}))

	plan, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "consumer", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Applicable() || len(plan.Blockers()) == 0 || !strings.Contains(plan.Blockers()[0].Detail, "not eligible") {
		t.Fatalf("invalid provider update plan = %#v", plan.Blockers())
	}
	if len(store.saves) != 0 {
		t.Fatalf("blocked update mutated state: %d saves", len(store.saves))
	}
}
