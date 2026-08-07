package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
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
