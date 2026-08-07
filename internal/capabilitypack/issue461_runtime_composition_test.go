package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type issue461ProjectAdapter struct{}

func (issue461ProjectAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	var resources []ResourceIdentity
	if transition.ProjectInstallation != nil {
		for _, projection := range transition.ProjectInstallation.Lock.Projections {
			resources = append(resources, projection.Resource)
		}
	} else {
		for _, resource := range transition.Desired.Resources {
			resources = append(resources, ResourceIdentity{Kind: resource.Kind, ID: resource.ID})
		}
	}
	inspection := SurfaceInspection{Revision: "issue461", Readiness: ReadinessObservation{AuthorizationObserved: true, Authorized: true, UsabilityObserved: true, Usable: true}}
	for _, resource := range resources {
		target := filepath.Join(transition.ProjectRoot, ".runtime", resource.Kind+"-"+resource.ID)
		desired := fingerprintProjectBytes([]byte(resource.String()))
		_, err := os.Stat(target)
		exists := err == nil
		observed := "missing"
		if exists {
			observed = desired
		}
		inspection.Projections = append(inspection.Projections, ObservedProjection{
			ID: resource.String(), Goal: ProjectionPresent, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired,
			Action: ProjectionAction{ID: resource.String(), Surface: SurfaceOpenCode, Kind: ActionOpenCodeAssetFile, Target: target, Content: resource.String(), FileMode: 0o644, PreviewOnly: true},
		})
	}
	return inspection, nil
}

func (issue461ProjectAdapter) ApplyProjections(_ context.Context, actions []ProjectionAction) *ProjectionActionError {
	for _, action := range actions {
		if err := os.MkdirAll(filepath.Dir(action.Target), 0o700); err != nil {
			return &ProjectionActionError{Err: err}
		}
		if err := os.WriteFile(action.Target, []byte(action.Content), os.FileMode(action.FileMode)); err != nil {
			return &ProjectionActionError{Err: err}
		}
	}
	return nil
}

func TestIssue461ExactGlobalRuntimeEffectsCoverProjectWithoutPersonalReceipts(t *testing.T) {
	ctx := context.Background()
	pack := issue461RuntimePack()
	bundleRoot := writeProjectBundleFixture(t)
	manifest := `{
  "schema_version": 4,
  "id": "runtime-pack",
  "version": "1.0.0",
  "surfaces": ["opencode"],
  "provides": [],
  "requires": {"capabilities": [], "tools": []},
  "conflicts": [],
  "root_migrations": [],
  "resources": [
    {"kind":"lifecycle","id":"memory-hook","requires":[],"conflicts":[],"provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[],"notices":[],"bindings":[{"surface":"opencode","projection":"lifecycle","name":"memory-hook","invocation":"memory-hook","mode":"native","sharing":"exclusive"}],"surface_exclusions":[]},
    {"kind":"mcp_server","id":"memory","command":"memory","args":[],"requires":[],"conflicts":[],"provides_capabilities":[],"requires_capabilities":[],"requires_tools":[],"capability_conflicts":[],"notices":[],"bindings":[{"surface":"opencode","projection":"mcp_server","name":"memory","invocation":"memory","mode":"native","sharing":"exclusive"}],"surface_exclusions":[]}
  ],
  "contract": {"exclusions": []}
}`
	if err := os.MkdirAll(filepath.Join(bundleRoot, "packs", pack.ID), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleRoot, "packs", pack.ID, "pack.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{packs: []Pack{pack}, entries: []catalogEntry{{ID: pack.ID, Surfaces: pack.Surfaces}}, bundleRoot: bundleRoot, allowSyntheticHistory: true}
	project := t.TempDir()
	packyHome := filepath.Join(t.TempDir(), ".packy")
	adapter := issue461ProjectAdapter{}
	installFacade := NewFacade(catalog)
	install, err := installFacade.PreviewProjectInstall(ctx, ProjectInstallRequest{PackID: pack.ID, Surface: SurfaceOpenCode, ProjectRoot: project}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if install.Disposition != ProjectInstallPreviewable {
		t.Fatalf("install blockers = %+v", install.Blockers)
	}
	if _, err := installFacade.ApplyProjectInstall(ctx, ProjectInstallApplyRequest{Preview: install, PackyHome: packyHome, Adapter: adapter}); err != nil {
		t.Fatal(err)
	}

	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceOpenCode, Version: pack.Version, Active: true, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}, ProviderChoices: []ProviderChoice{}}
	store := &fakeActivationStore{state: ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}}
	facade := NewFacade(catalog, WithActivation(store, nil))
	status, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, RequireUsable: true, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Packs) != 1 || status.Packs[0].Runtime != ProjectRuntimeInheritedGlobal || !status.Packs[0].RequirementSatisfied {
		t.Fatalf("status = %+v", status)
	}
	if len(status.Packs[0].RuntimeEffects) == 0 {
		t.Fatal("status omitted resource-level runtime coverage")
	}
	for _, effect := range status.Packs[0].RuntimeEffects {
		if effect.Coverage != ProjectRuntimeCoverageInheritedGlobal || effect.GlobalVersion != pack.Version {
			t.Fatalf("runtime effect = %+v", effect)
		}
	}
	preview, err := facade.PreviewProjectActivation(ctx, ProjectActivationRequest{PackID: pack.ID, Surface: SurfaceOpenCode, ProjectRoot: project, PackyHome: packyHome, Adapter: adapter})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Disposition != ProjectActivationInheritedGlobal || len(preview.Categories) != 0 {
		t.Fatalf("activation preview = %+v", preview)
	}
	if _, err := facade.ApplyProjectActivation(ctx, ProjectActivationApplyRequest{Preview: preview, Adapter: adapter, Interactive: true}); err == nil {
		t.Fatal("inherited global coverage created a redundant project activation")
	}
	if matches, err := filepath.Glob(filepath.Join(packyHome, "projects", "*", "state-*-opencode.json")); err != nil || len(matches) != 0 {
		t.Fatalf("inherited coverage created personal receipts: %v, %v", matches, err)
	}
}

func TestIssue461PartialGlobalCoverageLeavesOnlyUncoveredEffectsPending(t *testing.T) {
	ctx, catalog, adapter, project, packyHome, pack := issue461InstalledRuntimeProject(t)
	intent := ActivationIntent{
		PackID: pack.ID, Surface: SurfaceOpenCode, Version: pack.Version, Active: true,
		Selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "lifecycle", ID: "memory-hook"}}},
		Aliases:   []SurfaceAlias{}, ProviderChoices: []ProviderChoice{},
	}
	store := &fakeActivationStore{state: ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}}
	facade := NewFacade(catalog, WithActivation(store, nil))
	status, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	if status.Packs[0].Runtime != ProjectRuntimePending {
		t.Fatalf("runtime = %s", status.Packs[0].Runtime)
	}
	coverage := map[ResourceIdentity]ProjectRuntimeCoverage{}
	for _, effect := range status.Packs[0].RuntimeEffects {
		coverage[effect.Resource] = effect.Coverage
	}
	if coverage[ResourceIdentity{Kind: "lifecycle", ID: "memory-hook"}] != ProjectRuntimeCoverageInheritedGlobal || coverage[ResourceIdentity{Kind: "mcp_server", ID: "memory"}] != ProjectRuntimeCoveragePending {
		t.Fatalf("coverage = %+v", coverage)
	}
	preview, err := facade.PreviewProjectActivation(ctx, ProjectActivationRequest{PackID: pack.ID, Surface: SurfaceOpenCode, ProjectRoot: project, PackyHome: packyHome, Adapter: adapter})
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range preview.Categories {
		for _, detail := range category.Details {
			if detail.Resource == (ResourceIdentity{Kind: "lifecycle", ID: "memory-hook"}) {
				t.Fatalf("preview requested redundant consent for inherited effect: %+v", preview.Categories)
			}
		}
	}
	approvals := make([]ProjectActivationApproval, 0, len(preview.Categories))
	for _, category := range preview.Categories {
		approvals = append(approvals, facade.ApproveProjectActivation(preview, category.Kind))
	}
	if _, err := facade.ApplyProjectActivation(ctx, ProjectActivationApplyRequest{Preview: preview, Approvals: approvals, Adapter: adapter, Interactive: true}); err != nil {
		t.Fatal(err)
	}
	active, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
	if err != nil || active.Packs[0].Runtime != ProjectRuntimeActive {
		t.Fatalf("partial project activation status = %+v, %v", active, err)
	}
	store.state.Intent.Active = false
	store.state.Intents[0].Active = false
	withoutGlobal, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
	if err != nil || withoutGlobal.Packs[0].Runtime != ProjectRuntimePending {
		t.Fatalf("status after removing partial global coverage = %+v, %v", withoutGlobal, err)
	}
	coverage = map[ResourceIdentity]ProjectRuntimeCoverage{}
	for _, effect := range withoutGlobal.Packs[0].RuntimeEffects {
		coverage[effect.Resource] = effect.Coverage
	}
	if coverage[ResourceIdentity{Kind: "lifecycle", ID: "memory-hook"}] != ProjectRuntimeCoveragePending || coverage[ResourceIdentity{Kind: "mcp_server", ID: "memory"}] != ProjectRuntimeCoverageProject {
		t.Fatalf("coverage after removing partial global activation = %+v", coverage)
	}
}

func TestIssue461AddingAndRemovingGlobalCoveragePreservesProjectActivation(t *testing.T) {
	ctx, catalog, adapter, project, packyHome, pack := issue461InstalledRuntimeProject(t)
	local := NewFacade(catalog)
	preview, err := local.PreviewProjectActivation(ctx, ProjectActivationRequest{PackID: pack.ID, Surface: SurfaceOpenCode, ProjectRoot: project, PackyHome: packyHome, Adapter: adapter})
	if err != nil {
		t.Fatal(err)
	}
	approvals := make([]ProjectActivationApproval, 0, len(preview.Categories))
	for _, category := range preview.Categories {
		approvals = append(approvals, local.ApproveProjectActivation(preview, category.Kind))
	}
	if _, err := local.ApplyProjectActivation(ctx, ProjectActivationApplyRequest{Preview: preview, Approvals: approvals, Adapter: adapter, Interactive: true}); err != nil {
		t.Fatal(err)
	}

	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceOpenCode, Version: pack.Version, Active: true, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}, ProviderChoices: []ProviderChoice{}}
	store := &fakeActivationStore{state: ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}}
	facade := NewFacade(catalog, WithActivation(store, nil))
	combined, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
	if err != nil || combined.Packs[0].Runtime != ProjectRuntimeActive {
		t.Fatalf("combined status = %+v, %v", combined, err)
	}
	for _, effect := range combined.Packs[0].RuntimeEffects {
		if effect.Coverage != ProjectRuntimeCoverageGlobalAndProject {
			t.Fatalf("combined effect = %+v", effect)
		}
	}
	converged, err := facade.PreviewProjectActivation(ctx, ProjectActivationRequest{PackID: pack.ID, Surface: SurfaceOpenCode, ProjectRoot: project, PackyHome: packyHome, Adapter: adapter})
	if err != nil || converged.Disposition != ProjectActivationConverged || len(converged.Categories) != 0 {
		t.Fatalf("combined activation preview = %+v, %v", converged, err)
	}

	store.state.Intent.Active = false
	store.state.Intents[0].Active = false
	personal, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
	if err != nil || personal.Packs[0].Runtime != ProjectRuntimeActive {
		t.Fatalf("personal status after removing global coverage = %+v, %v", personal, err)
	}
	for _, effect := range personal.Packs[0].RuntimeEffects {
		if effect.Coverage != ProjectRuntimeCoverageProject {
			t.Fatalf("personal effect = %+v", effect)
		}
	}
}

func TestIssue461IncompatibleGlobalAliasBlocksRuntimeButNotProjectInstallation(t *testing.T) {
	ctx, catalog, adapter, project, packyHome, pack := issue461InstalledRuntimeProject(t)
	intent := ActivationIntent{
		PackID: pack.ID, Surface: SurfaceOpenCode, Version: pack.Version, Active: true,
		Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}},
		Aliases:   []SurfaceAlias{{Kind: "mcp_server", ID: "memory", Name: "global-memory"}}, ProviderChoices: []ProviderChoice{},
	}
	facade := NewFacade(catalog, WithActivation(&fakeActivationStore{state: ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}}, nil))
	status, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, RequireInstalled: true, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Packs[0].RequirementSatisfied || status.Packs[0].Installation != ProjectInstallationInstalled || status.Packs[0].Runtime != ProjectRuntimeBlocked {
		t.Fatalf("conflict changed shared installation state: %+v", status.Packs[0])
	}
	if len(status.Packs[0].Blockers) != 1 || status.Packs[0].Blockers[0].Code != "activation_scope_conflict" {
		t.Fatalf("scope conflict blockers = %+v", status.Packs[0].Blockers)
	}
	preview, err := facade.PreviewProjectActivation(ctx, ProjectActivationRequest{PackID: pack.ID, Surface: SurfaceOpenCode, ProjectRoot: project, PackyHome: packyHome, Adapter: adapter})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Disposition != ProjectActivationBlocked {
		t.Fatalf("conflicting activation preview = %+v", preview)
	}
}

func TestIssue461IncompatibleGlobalContractAndSensitiveDefinitionBlockRuntime(t *testing.T) {
	t.Run("contract", func(t *testing.T) {
		ctx, catalog, adapter, project, packyHome, pack := issue461InstalledRuntimeProject(t)
		intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceOpenCode, Version: "0.9.0", Active: true, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}, ProviderChoices: []ProviderChoice{}}
		facade := NewFacade(catalog, WithActivation(&fakeActivationStore{state: ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}}, nil))
		status, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
		if err != nil || status.Packs[0].Runtime != ProjectRuntimeBlocked {
			t.Fatalf("contract conflict status = %+v, %v", status, err)
		}
	})

	t.Run("sensitive definition", func(t *testing.T) {
		ctx, catalog, adapter, project, packyHome, pack := issue461InstalledRuntimeProject(t)
		installation, err := LoadProjectInstallation(project)
		if err != nil {
			t.Fatal(err)
		}
		installation.Lock.Receipts[0].Sensitive = deduplicateProjectSensitiveDisclosures(append(installation.Lock.Receipts[0].Sensitive, ProjectSensitiveDisclosure{Category: ProjectActivationTrust, Surface: SurfaceOpenCode, Resource: ResourceIdentity{Kind: "lifecycle", ID: "memory-hook"}, Detail: "changed-sensitive-definition"}))
		lock, err := marshalProjectLock(installation.Lock)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, "packy.lock.json"), lock, 0o644); err != nil {
			t.Fatal(err)
		}
		intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceOpenCode, Version: pack.Version, Active: true, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}, ProviderChoices: []ProviderChoice{}}
		facade := NewFacade(catalog, WithActivation(&fakeActivationStore{state: ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}}, nil))
		status, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
		if err != nil || status.Packs[0].Runtime != ProjectRuntimeBlocked {
			t.Fatalf("sensitive definition conflict status = %+v, %v", status, err)
		}
	})
}

func TestIssue461RemovingGlobalCoverageReturnsEffectsToPendingWithoutLocalActivation(t *testing.T) {
	ctx, catalog, adapter, project, packyHome, pack := issue461InstalledRuntimeProject(t)
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceOpenCode, Version: pack.Version, Active: true, Selection: ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, Aliases: []SurfaceAlias{}, ProviderChoices: []ProviderChoice{}}
	store := &fakeActivationStore{state: ActivationState{Intent: intent, Intents: []ActivationIntent{intent}}}
	facade := NewFacade(catalog, WithActivation(store, nil))
	covered, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
	if err != nil || covered.Packs[0].Runtime != ProjectRuntimeInheritedGlobal {
		t.Fatalf("covered status = %+v, %v", covered, err)
	}
	store.state.Intent.Active = false
	store.state.Intents[0].Active = false
	pending, err := facade.InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: project, PackID: pack.ID, Surface: SurfaceOpenCode, PackyHome: packyHome, Adapters: map[Surface]SurfaceAdapter{SurfaceOpenCode: adapter}})
	if err != nil || pending.Packs[0].Runtime != ProjectRuntimePending {
		t.Fatalf("pending status = %+v, %v", pending, err)
	}
	for _, effect := range pending.Packs[0].RuntimeEffects {
		if effect.Coverage != ProjectRuntimeCoveragePending {
			t.Fatalf("deactivation retained coverage: %+v", pending.Packs[0].RuntimeEffects)
		}
	}
	if matches, err := filepath.Glob(filepath.Join(packyHome, "projects", "*", "state-*-opencode.json")); err != nil || len(matches) != 0 {
		t.Fatalf("global deactivation fabricated local activation: %v, %v", matches, err)
	}
}

func issue461InstalledRuntimeProject(t *testing.T) (context.Context, Catalog, SurfaceAdapter, string, string, Pack) {
	t.Helper()
	ctx := context.Background()
	pack := issue461RuntimePack()
	bundleRoot := writeProjectBundleFixture(t)
	catalog := Catalog{packs: []Pack{pack}, entries: []catalogEntry{{ID: pack.ID, Surfaces: pack.Surfaces}}, bundleRoot: bundleRoot, allowSyntheticHistory: true}
	project := t.TempDir()
	packyHome := filepath.Join(t.TempDir(), ".packy")
	adapter := issue461ProjectAdapter{}
	installFacade := NewFacade(catalog)
	install, err := installFacade.previewProjectInstall(ctx, ProjectInstallRequest{PackID: pack.ID, Surface: SurfaceOpenCode, ProjectRoot: project}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := marshalProjectManifest(install.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := marshalProjectLock(install.Lock)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range install.actions {
		if applyErr := adapter.ApplyProjections(ctx, []ProjectionAction{action}); applyErr != nil {
			t.Fatal(applyErr)
		}
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
	return ctx, catalog, adapter, project, packyHome, pack
}

func issue461RuntimePack() Pack {
	empty := []string{}
	return Pack{manifestVersion: manifestSchemaV4, ID: "runtime-pack", Version: "1.0.0", Surfaces: []Surface{SurfaceOpenCode}, Resources: []Resource{
		{Kind: "mcp_server", ID: "memory", Command: "memory", Args: []string{}, Requires: empty, Conflicts: empty, Notices: empty, Bindings: []Binding{{Surface: SurfaceOpenCode, Projection: "mcp_server", Name: "memory", Invocation: "memory", Mode: "native", Sharing: "exclusive"}}, SurfaceExclusions: []SurfaceExclusion{}, ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
		{Kind: "lifecycle", ID: "memory-hook", Requires: empty, Conflicts: empty, Notices: empty, Bindings: []Binding{{Surface: SurfaceOpenCode, Projection: "lifecycle", Name: "memory-hook", Invocation: "memory-hook", Mode: "native", Sharing: "exclusive"}}, SurfaceExclusions: []SurfaceExclusion{}, ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
	}}
}
