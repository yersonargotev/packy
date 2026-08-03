package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type changingProjectAdapter struct{ revision string }

func (a *changingProjectAdapter) InspectProject(_ context.Context, _ Pack, _ string) (ProjectSurfaceObservation, error) {
	return ProjectSurfaceObservation{Revision: a.revision, Projections: []ProjectProjectionObservation{{Resource: ResourceIdentity{Kind: "skill", ID: "one"}, Target: ".agents/skills/one", Mode: "copy_tree", DesiredFingerprint: "safe", Representable: true}}}, nil
}

func TestDiscoverProjectRootResolvesNestedAndLinkedWorktrees(t *testing.T) {
	root := filepath.Join(t.TempDir(), "linked")
	gitDirectory := filepath.Join(t.TempDir(), "git-admin")
	nested := filepath.Join(root, "one", "two")
	for _, directory := range []string{root, gitDirectory, nested} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
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
	if err := os.WriteFile(filepath.Join(invalid, ".git"), []byte("not a worktree marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverProjectRoot(invalid); err == nil {
		t.Fatal("invalid .git file was accepted as a worktree")
	}
}

func TestProjectInstallStaleObservationIsNonActionable(t *testing.T) {
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "app", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "skill", ID: "one"}}}
	facade := NewFacade(Catalog{packs: []Pack{pack}, allowSyntheticHistory: true})
	adapter := &changingProjectAdapter{revision: "before"}
	preview, err := facade.PreviewProjectInstall(context.Background(), ProjectInstallRequest{PackID: "app", Surface: SurfaceCodex, ProjectRoot: "/project"}, adapter)
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
