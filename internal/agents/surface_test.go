package agents

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/localprojection"
)

func TestBindAdapterProvenancePublishesCanonicalObservationWithoutChangingAction(t *testing.T) {
	inspection := capabilitypack.SurfaceInspection{Projections: []capabilitypack.ObservedProjection{{Goal: capabilitypack.ProjectionPresent, Action: capabilitypack.ProjectionAction{Kind: capabilitypack.ActionCodexAgentFile}}}}
	bindAdapterProvenance(&inspection)
	projection := inspection.Projections[0]
	if projection.AdapterProvenance != "agents-projection/v1/codex-agent-file" || projection.Action.AdapterProvenance != "" {
		t.Fatalf("provenance = observed:%q action:%q", projection.AdapterProvenance, projection.Action.AdapterProvenance)
	}
}

func TestUnobservablePackReadinessIsUnknownRatherThanDenied(t *testing.T) {
	adapter := &SurfaceAdapter{}
	observed, err := adapter.inspectReadiness(context.Background(), capabilitypack.Pack{ID: "argote"}, capabilitypack.SurfaceInspection{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if observed.AuthorizationObserved || observed.Authorized || observed.UsabilityObserved || observed.Usable {
		t.Fatalf("unobservable readiness = %#v", observed)
	}
	matty, err := adapter.inspectReadiness(context.Background(), capabilitypack.Pack{ID: "synthetic", Resources: []capabilitypack.Resource{{Kind: "skill", ID: "guide"}}}, capabilitypack.SurfaceInspection{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if matty.AuthorizationObserved || matty.Authorized || matty.UsabilityObserved || matty.Usable || len(matty.PendingHumanActions) != 0 {
		t.Fatalf("Matty runtime-unobservable readiness = %#v", matty)
	}
	withMetadata, err := adapter.inspectReadiness(context.Background(), capabilitypack.Pack{ID: "synthetic", Resources: []capabilitypack.Resource{{Kind: "skill", ID: "coordinate"}, {Kind: "lifecycle", ID: "session"}, {Kind: "notice", ID: "license"}}}, capabilitypack.SurfaceInspection{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if withMetadata.AuthorizationObserved || withMetadata.Authorized || withMetadata.UsabilityObserved {
		t.Fatalf("skill Pack metadata changed Agents discovery authorization: %#v", withMetadata)
	}
}

func TestBindAdapterProvenancePublishesCanonicalSkillTopology(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".agents", "skills", "ask-matt")
	inspection := capabilitypack.SurfaceInspection{Projections: []capabilitypack.ObservedProjection{{
		ID: "skill:ask-matt", Action: capabilitypack.ProjectionAction{Kind: capabilitypack.ActionSkillTree, Target: target},
	}}}
	bindAdapterProvenance(&inspection)
	projection := inspection.Projections[0]
	if projection.ProjectionKey != "path:"+filepath.Clean(target) || !projection.Shared {
		t.Fatalf("Agents shared topology = %+v", projection)
	}
	if projection.Action.ProjectionKey != projection.ProjectionKey || !projection.Action.Shared {
		t.Fatalf("Agents shared action topology = %+v", projection.Action)
	}
}

func TestCodexApplyRollsBackWhenAProjectionCannotBeStaged(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "skills"), filepath.Join(root, ".codex", "agents"))
	first := filepath.Join(root, ".codex", "agents", "first.toml")
	err := adapter.ApplyProjections(context.Background(), []capabilitypack.ProjectionAction{
		{ID: "agent:first", Kind: capabilitypack.ActionCodexAgentFile, Target: first, Content: "first"},
		{ID: "agent:blocked", Kind: capabilitypack.ActionCodexAgentFile, Target: filepath.Join(blocker, "blocked.toml"), Content: "blocked"},
	})
	if err == nil {
		t.Fatal("partial failure unexpectedly succeeded")
	}
	if _, statErr := os.Stat(first); !os.IsNotExist(statErr) {
		t.Fatalf("first projection leaked after failure: %v", statErr)
	}
	got, _ := os.ReadFile(blocker)
	if string(got) != "keep" {
		t.Fatalf("blocker changed: %q", got)
	}
}

func TestCodexResidualInspectionReportsDriftBeforeOwnedRemoval(t *testing.T) {
	root := t.TempDir()
	roles := filepath.Join(root, ".codex", "agents")
	target := filepath.Join(root, ".codex", "agents", "coach.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := NewSurfaceAdapter(root, filepath.Join(root, "skills"), roles)
	initial, exists, err := localprojection.FingerprintPath(target)
	if err != nil || !exists {
		t.Fatalf("fingerprint: %q %v %v", initial, exists, err)
	}
	if err := os.WriteFile(target, []byte("drifted"), 0o600); err != nil {
		t.Fatal(err)
	}
	inspection, err := adapter.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: capabilitypack.Pack{ID: "empty"}, ResidualOwnership: []capabilitypack.ProjectionOwnership{{ID: "agent:coach", Fingerprint: initial, PackID: "pack", Surface: capabilitypack.SurfaceAgents}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Projections) != 1 {
		t.Fatalf("projections = %+v", inspection.Projections)
	}
	projection := inspection.Projections[0]
	if projection.ObservedFingerprint == initial || projection.Action.Mode != capabilitypack.ProjectionDeleteTarget {
		t.Fatalf("drift was hidden: %+v", projection)
	}
}
