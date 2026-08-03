package capabilitypack

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type changingProjectAdapter struct{ revision string }

func (a *changingProjectAdapter) InspectSurface(_ context.Context, transition SurfaceTransition) (SurfaceInspection, error) {
	return SurfaceInspection{Revision: a.revision, Projections: []ObservedProjection{{
		ID: "skill:ask-matt", Goal: ProjectionPresent, DesiredFingerprint: "safe-" + a.revision,
		Action: ProjectionAction{ID: "skill:ask-matt", Target: filepath.Join(transition.ProjectRoot, ".agents", "skills", "ask-matt")},
	}}}, nil
}

func (a *changingProjectAdapter) ApplyProjections(context.Context, []ProjectionAction) *ProjectionActionError {
	return nil
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
