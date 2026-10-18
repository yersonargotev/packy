package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

func TestIssue855SkillInspectionRequiresImmutableSourceIdentity(t *testing.T) {
	home := t.TempDir()
	catalog := filepath.Join(home, "catalog")
	sourceA := filepath.Join(catalog, "snapshot-a", "guide")
	sourceB := filepath.Join(catalog, "snapshot-b", "guide")
	for _, source := range []string{sourceA, sourceB} {
		if err := os.MkdirAll(source, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("identical reviewed skill\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	layout := NewCanonicalLayout(home)
	if err := os.MkdirAll(layout.SkillsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(layout.SkillsDir, "guide")
	if err := os.Symlink(sourceA, target); err != nil {
		t.Fatal(err)
	}
	treeFingerprint, err := localprojection.FingerprintTree(sourceA)
	if err != nil {
		t.Fatal(err)
	}
	resolvedA, err := filepath.EvalSymlinks(sourceA)
	if err != nil {
		t.Fatal(err)
	}
	record := OwnershipRecord{
		StateOwner: "capabilitypack", ContributorID: "snapshot-pack", ID: "skill:guide",
		Kind: string(ActionSkillLink), Target: target, Fingerprint: treeFingerprint,
		Skill: SkillIdentity{
			Surface: "claude", ProjectionID: "skill:guide", Path: target, SymlinkType: "directory",
			ResolvedTarget: resolvedA, ExpectedSource: resolvedA, SourceTreeFingerprint: treeFingerprint,
		},
		DeletionAuthorized: true, Contributors: []string{"snapshot-pack"},
	}
	adapter := NewSurfaceAdapter(catalog, layout, filepath.Join(home, "state"), "claude", &recordingRunner{}, StaticOwnershipSnapshot(NewOwnershipSnapshot(record)))
	pack := capabilitypack.Pack{ID: "snapshot-pack", Version: "2.0.0", Resources: []capabilitypack.Resource{{
		Kind: "skill", ID: "guide", Source: "snapshot-b/guide",
		Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "skill", Name: "guide"}},
	}}}

	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	projection := inspection.Projections[0]
	if projection.ObservedFingerprint != projection.DesiredFingerprint || !projection.NeedsReconciliation {
		t.Fatalf("byte-identical skill linked to the previous immutable source was reported converged: %+v", projection)
	}
	if !strings.Contains(strings.Join(inspection.Readiness.PendingHumanActions, "\n"), "converge every exact desired Claude Code projection") {
		t.Fatalf("pending source identity was reported configured: %+v", inspection.Readiness)
	}
	if err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{projection.Action}); err != nil {
		t.Fatal(err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	resolvedB, err := filepath.EvalSymlinks(sourceB)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedTarget != resolvedB {
		t.Fatalf("updated skill target = %s, want %s", resolvedTarget, resolvedB)
	}

	converged, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if converged.Projections[0].NeedsReconciliation || converged.Projections[0].ObservedFingerprint != converged.Projections[0].DesiredFingerprint {
		t.Fatalf("same-snapshot skill did not converge: %+v", converged.Projections[0])
	}
	if converged.Revision == inspection.Revision {
		t.Fatal("source identity handoff did not change the sealed surface revision")
	}
}
