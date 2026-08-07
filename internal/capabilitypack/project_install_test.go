package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type changingProjectAdapter struct{ revision string }

type projectMCPAdapter struct{}

func (projectMCPAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	target := filepath.Join(transition.ProjectRoot, ".codex", "config.toml")
	return SurfaceInspection{Revision: "mcp-preview", Projections: []ObservedProjection{{
		ID: "mcp_server:memory", Goal: ProjectionPresent, ObservedFingerprint: "missing", DesiredFingerprint: "mcp-definition-v1",
		Action: ProjectionAction{ID: "mcp_server:memory", Surface: SurfaceCodex, Kind: ActionCodexMCPConfig, Target: target, Content: "[mcp_servers.memory]\ncommand = \"memory\"\n", FileMode: 0o644, Command: "memory", PreviewOnly: true},
	}}}, nil
}

func (projectMCPAdapter) ApplyProjections(context.Context, []ProjectionAction) *ProjectionActionError {
	return nil
}

func (a *changingProjectAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	return SurfaceInspection{Revision: a.revision, Projections: []ObservedProjection{{
		ID: "skill:ask-matt", Goal: ProjectionPresent, DesiredFingerprint: "safe-" + a.revision,
		Action: ProjectionAction{ID: "skill:ask-matt", Target: filepath.Join(transition.ProjectRoot, ".agents", "skills", "ask-matt"), PreviewOnly: true},
	}}}, nil
}

func (a *changingProjectAdapter) ApplyProjections(context.Context, []ProjectionAction) *ProjectionActionError {
	return nil
}

func TestDiscoverProjectRootResolvesNestedAndLinkedWorktrees(t *testing.T) {
	root := filepath.Join(t.TempDir(), "linked")
	gitDirectory := filepath.Join(t.TempDir(), "git-admin")
	commonDirectory := filepath.Join(t.TempDir(), "git-common")
	nested := filepath.Join(root, "one", "two")
	for _, directory := range []string{root, gitDirectory, nested, filepath.Join(commonDirectory, "objects"), filepath.Join(commonDirectory, "refs")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(gitDirectory, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDirectory, "commondir"), []byte(commonDirectory+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: "+gitDirectory+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DiscoverProjectRoot(nested); err != nil || got != canonical {
		t.Fatalf("project root = %q, %v; want %q", got, err, canonical)
	}
	invalid := t.TempDir()
	if err := os.Mkdir(filepath.Join(invalid, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverProjectRoot(invalid); err == nil {
		t.Fatal("invalid .git file was accepted as a worktree")
	}
}

func TestProjectInstallStaleObservationIsNonActionable(t *testing.T) {
	bundleRoot, err := filepath.Abs(filepath.Join("..", "..", "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := DiscoverForDurableIntents(bundleRoot)
	if err != nil {
		t.Fatal(err)
	}
	facade := NewFacade(catalog)
	adapter := &changingProjectAdapter{revision: "before"}
	preview, err := facade.PreviewProjectInstall(context.Background(), ProjectInstallRequest{PackID: "matty", Surface: SurfaceCodex, ProjectRoot: t.TempDir()}, adapter)
	if err != nil {
		t.Fatal(err)
	}
	adapter.revision = "after"
	freshness, err := facade.CheckProjectInstallFreshness(context.Background(), preview, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if freshness.Disposition != ProjectInstallBlocked || len(freshness.Blockers) != 1 || freshness.Blockers[0].Code != "stale_observation" {
		t.Fatalf("freshness = %#v", freshness)
	}
}

func TestIssue455ProjectProvidersRequireAndPersistAnExplicitChoice(t *testing.T) {
	consumer, first, second := providerStatusFixture()
	consumer.Resources[0].Requires = []string{"asset:data"}
	consumer.Resources[0].Notices = []string{"notice:license"}
	consumer.Resources[0].Bindings[0].Mode = "degraded"
	consumer.Resources[0].Bindings[0].Degradation = "project-test-degradation"
	empty := []string{}
	consumer.Resources = append(consumer.Resources,
		Resource{Kind: "asset", ID: "data", Requires: empty, Conflicts: empty, Notices: empty, Bindings: []Binding{}, SurfaceExclusions: []SurfaceExclusion{}, ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
		Resource{Kind: "notice", ID: "license", Requires: empty, Bindings: []Binding{}, SurfaceExclusions: []SurfaceExclusion{}, ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
	)
	second.Resources[0].Bindings[0].Name = "root"
	bundleRoot := writeProjectBundleFixture(t)
	adapter := &fakeSurfaceAdapter{}
	entries := []catalogEntry{{ID: consumer.ID, Surfaces: consumer.Surfaces}, {ID: first.ID, Surfaces: first.Surfaces}, {ID: second.ID, Surfaces: second.Surfaces}}
	globalProvider := ActivationIntent{PackID: second.ID, Surface: SurfaceCodex, Version: second.Version, Active: true, Selection: ResourceSelection{Mode: SelectionAll}}
	facade := NewFacade(Catalog{packs: []Pack{consumer, first, second}, entries: entries, bundleRoot: bundleRoot}, WithActivation(&fakeActivationStore{state: ActivationState{Intent: globalProvider, Intents: []ActivationIntent{globalProvider}}}, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))
	request := ProjectInstallRequest{PackID: consumer.ID, Surface: SurfaceCodex, ProjectRoot: t.TempDir(), Selection: ResourceSelection{Mode: SelectionAll}}
	unambiguousFacade := NewFacade(Catalog{packs: []Pack{consumer, first}, entries: entries[:2], bundleRoot: bundleRoot})
	unambiguous, err := unambiguousFacade.previewProjectInstall(context.Background(), request, adapter)
	if err != nil || unambiguous.Disposition != ProjectInstallPreviewable || len(unambiguous.Pack.ProviderChoices) != 1 || unambiguous.Pack.ProviderChoices[0].ProviderPack != first.ID {
		t.Fatalf("unambiguous provider intent = err:%v pack:%#v blockers:%#v", err, unambiguous.Pack, unambiguous.Blockers)
	}

	ambiguous, err := facade.previewProjectInstall(context.Background(), request, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.Disposition != ProjectInstallBlocked || len(ambiguous.Blockers) != 1 || ambiguous.Blockers[0].Code != "ambiguous_provider" {
		t.Fatalf("ambiguous provider preview = %#v", ambiguous.Blockers)
	}

	providerResource := ResourceIdentity{Kind: "skill", ID: "storage"}
	request.ProviderChoices = []ProviderChoice{{Capability: "cap:storage", ProviderPack: second.ID, ProviderResource: &providerResource}}
	collision, err := facade.previewProjectInstall(context.Background(), request, adapter)
	if err != nil || collision.Disposition != ProjectInstallBlocked || len(collision.Blockers) != 1 || collision.Blockers[0].Code != "native_name_collision" {
		t.Fatalf("provider collision = err:%v blockers:%#v", err, collision.Blockers)
	}
	request.Aliases = []SurfaceAlias{{Kind: "skill", ID: "storage", Name: "provider-storage"}}
	chosen, err := facade.previewProjectInstall(context.Background(), request, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if chosen.Disposition != ProjectInstallPreviewable || len(chosen.Pack.ProviderChoices) != 1 || chosen.Pack.ProviderChoices[0].ProviderPack != second.ID {
		t.Fatalf("chosen provider intent = %#v blockers=%#v", chosen.Pack.ProviderChoices, chosen.Blockers)
	}
	if len(chosen.Lock.Packs) != 2 || chosen.Lock.Packs[0].ID != consumer.ID || chosen.Lock.Packs[0].Role != ActivationRequested || chosen.Lock.Packs[1].ID != second.ID || chosen.Lock.Packs[1].Role != ActivationRequired {
		t.Fatalf("resolved pack roles = %#v", chosen.Lock.Packs)
	}
	if len(chosen.Lock.Sources) != 2 || chosen.Lock.Sources[0].PackID != consumer.ID || chosen.Lock.Sources[1].PackID != second.ID {
		t.Fatalf("resolved provider sources = %#v", chosen.Lock.Sources)
	}
	foundProviderBinding := false
	for _, binding := range chosen.Lock.Bindings {
		if binding.Kind == "skill" && binding.ID == "storage" && binding.Name == "provider-storage" {
			foundProviderBinding = true
		}
	}
	if !foundProviderBinding {
		t.Fatalf("provider binding missing from lock: %#v", chosen.Lock.Bindings)
	}
	foundDegradation := false
	for _, binding := range chosen.Lock.Bindings {
		if binding.Kind == "skill" && binding.ID == "root" && binding.Mode == "degraded" && binding.Degradation == "project-test-degradation" {
			foundDegradation = true
		}
	}
	if !foundDegradation {
		t.Fatalf("declared degradation missing from lock: %#v", chosen.Lock.Bindings)
	}
	rootGraph := chosen.Lock.Packs[0].ResourceGraph.Resources
	roles := map[ResourceIdentity]ResourceRole{}
	chains := map[ResourceIdentity][]ResourceIdentity{}
	for _, fact := range rootGraph {
		roles[fact.Resource], chains[fact.Resource] = fact.Role, fact.DependencyChain
	}
	if roles[ResourceIdentity{Kind: "skill", ID: "root"}] != ResourceRoleRoot || roles[ResourceIdentity{Kind: "asset", ID: "data"}] != ResourceRoleAsset || roles[ResourceIdentity{Kind: "notice", ID: "license"}] != ResourceRoleNotice || len(chains[ResourceIdentity{Kind: "asset", ID: "data"}]) != 2 {
		t.Fatalf("resolved roles and chains = %#v %#v", roles, chains)
	}
	chosen.Lock.Projections = make([]ProjectProjectionPlan, 0, len(chosen.Lock.Bindings))
	for _, binding := range chosen.Lock.Bindings {
		chosen.Lock.Projections = append(chosen.Lock.Projections, ProjectProjectionPlan{
			Resource: ResourceIdentity{Kind: binding.Kind, ID: binding.ID}, Target: ".agents/skills/" + binding.Name,
			Mode: "copy_tree", FileMode: 0o700, DesiredFingerprint: strings.Repeat("c", 64), ObservedState: "installed", Contributor: "surface:codex:pack:consumer",
		})
	}
	if err := validateProjectInstallation(chosen.Manifest, chosen.Lock); err != nil {
		t.Fatalf("generated provider contract is invalid: %v", err)
	}
	missingNativeProjection := chosen.Lock
	missingNativeProjection.Projections = append([]ProjectProjectionPlan(nil), chosen.Lock.Projections[1:]...)
	if err := validateProjectInstallation(chosen.Manifest, missingNativeProjection); err == nil || !strings.Contains(err.Error(), "has no projection plan") {
		t.Fatalf("missing native projection error = %v", err)
	}
	missingProviderResource := chosen.Lock
	missingProviderResource.Packs = append([]ProjectResolvedPack(nil), chosen.Lock.Packs...)
	missingProviderResource.Packs[1].ResourceGraph.Resources = []ResourceClosureFact{}
	if err := validateProjectInstallation(chosen.Manifest, missingProviderResource); err == nil || !strings.Contains(err.Error(), "provider resource") {
		t.Fatalf("missing exact provider resource error = %v", err)
	}
	missingOperationalBinding := chosen.Lock
	missingOperationalBinding.Bindings = append([]LifecycleBinding(nil), chosen.Lock.Bindings...)
	for i, binding := range missingOperationalBinding.Bindings {
		if binding.Kind == "skill" && binding.ID == "root" {
			missingOperationalBinding.Bindings = append(missingOperationalBinding.Bindings[:i], missingOperationalBinding.Bindings[i+1:]...)
			break
		}
	}
	if err := validateProjectInstallation(chosen.Manifest, missingOperationalBinding); err == nil || !strings.Contains(err.Error(), "binding or declared degradation") {
		t.Fatalf("missing operational binding error = %v", err)
	}
	substituted := chosen.Manifest
	substituted.Packs = append([]ProjectManifestPack(nil), chosen.Manifest.Packs...)
	substituted.Packs[0].ProviderChoices = cloneProviderChoices(chosen.Manifest.Packs[0].ProviderChoices)
	substituted.Packs[0].ProviderChoices[0].ProviderPack = first.ID
	if err := validateProjectInstallation(substituted, chosen.Lock); err == nil || !strings.Contains(err.Error(), "requested resolution") {
		t.Fatalf("silent provider substitution error = %v", err)
	}
}

func TestIssue455ProjectNativeNameCollisionRequiresExplicitAlias(t *testing.T) {
	empty := []string{}
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "collision", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
		{Kind: "skill", ID: "first", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "shared", Mode: "native", Sharing: "exclusive"}}, ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
		{Kind: "skill", ID: "second", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "shared", Mode: "native", Sharing: "exclusive"}}, ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty},
	}}
	bundleRoot := writeProjectBundleFixture(t)
	facade := NewFacade(Catalog{packs: []Pack{pack}, entries: []catalogEntry{{ID: pack.ID, Surfaces: pack.Surfaces}}, bundleRoot: bundleRoot})
	request := ProjectInstallRequest{PackID: pack.ID, Surface: SurfaceCodex, ProjectRoot: t.TempDir(), Selection: ResourceSelection{Mode: SelectionAll}}
	blocked, err := facade.previewProjectInstall(context.Background(), request, &fakeSurfaceAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Disposition != ProjectInstallBlocked || len(blocked.Blockers) != 1 || blocked.Blockers[0].Code != "native_name_collision" {
		t.Fatalf("collision blockers = %#v", blocked.Blockers)
	}
	request.Aliases = []SurfaceAlias{{Kind: "skill", ID: "second", Name: "second-project"}}
	resolved, err := facade.previewProjectInstall(context.Background(), request, &fakeSurfaceAdapter{})
	if err != nil || resolved.Disposition != ProjectInstallPreviewable {
		t.Fatalf("aliased collision = err:%v blockers:%#v", err, resolved.Blockers)
	}
	if len(resolved.Pack.Aliases) != 1 || resolved.Pack.Aliases[0].Name != "second-project" {
		t.Fatalf("persisted aliases = %#v", resolved.Pack.Aliases)
	}
}

func TestProjectInstallLocksCodexMCPAsSensitiveDeclarativeDefinition(t *testing.T) {
	empty := []string{}
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "project-memory", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Requires: Requirements{Capabilities: empty, Tools: []string{"memory"}}, Resources: []Resource{{
		Kind: "mcp_server", ID: "memory", Command: "memory", Args: []string{"serve"},
		Bindings:             []Binding{{Surface: SurfaceCodex, Projection: "mcp_server", Name: "memory", Mode: "native", Sharing: "exclusive"}},
		ProvidesCapabilities: empty, RequiresCapabilities: empty, RequiresTools: empty, CapabilityConflicts: empty,
	}}}
	bundleRoot := writeProjectBundleFixture(t)
	facade := NewFacade(Catalog{packs: []Pack{pack}, entries: []catalogEntry{{ID: pack.ID, Surfaces: pack.Surfaces}}, bundleRoot: bundleRoot})
	preview, err := facade.previewProjectInstall(context.Background(), ProjectInstallRequest{PackID: pack.ID, Surface: SurfaceCodex, ProjectRoot: t.TempDir(), Selection: ResourceSelection{Mode: SelectionAll}}, projectMCPAdapter{})
	if err != nil || preview.Disposition != ProjectInstallPreviewable || len(preview.Projections) != 1 {
		t.Fatalf("Codex MCP project preview = %+v, %v", preview, err)
	}
	if projection := preview.Projections[0]; projection.Mode != "merge_marked_file" || projection.Target != ".codex/config.toml" || projection.Command != "memory" {
		t.Fatalf("locked Codex MCP projection = %+v", projection)
	}
	categories := projectActivationCategories(preview.Lock, SurfaceCodex)
	if len(categories) != 3 || categories[0].Kind != ProjectActivationMCP || categories[1].Kind != ProjectActivationExternalRequirements || categories[2].Kind != ProjectActivationTrust {
		t.Fatalf("Codex MCP activation categories = %+v", categories)
	}
}

func writeProjectBundleFixture(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}
