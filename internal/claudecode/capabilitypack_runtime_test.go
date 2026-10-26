package claudecode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestBindAdapterProvenancePreservesDetailedIdentityAndKeepsDeletePayloadClean(t *testing.T) {
	inspection := capabilitypack.SurfaceInspection{Projections: []capabilitypack.ObservedProjection{{Goal: capabilitypack.ProjectionAbsent, AdapterProvenance: "sealed-composite-identity", Action: capabilitypack.ProjectionAction{Kind: ActionSkillTree, Mode: capabilitypack.ProjectionDeleteTarget}}}}
	bindAdapterProvenance(&inspection)
	projection := inspection.Projections[0]
	if projection.AdapterProvenance != "sealed-composite-identity" || projection.Action.AdapterProvenance != "" {
		t.Fatalf("provenance = observed:%q action:%q", projection.AdapterProvenance, projection.Action.AdapterProvenance)
	}
}

func TestClaudePersonalSkillTopologyDoesNotClaimSharedAgentsTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".claude", "skills", "ask-matt")
	inspection := capabilitypack.SurfaceInspection{Projections: []capabilitypack.ObservedProjection{{
		ID: "skill:ask-matt", Action: capabilitypack.ProjectionAction{Kind: ActionSkillLink, Target: target},
	}}}
	bindAdapterProvenance(&inspection)
	projection := inspection.Projections[0]
	if projection.Shared || projection.Action.Shared {
		t.Fatalf("Claude personal skill was misclassified as shared: %+v", projection)
	}
}

type ownershipStore struct {
	state capabilitypack.ActivationState
}

func ownershipPackResolver(packs map[string]capabilitypack.Pack) func(context.Context, string, string, string) (capabilitypack.Pack, error) {
	return func(_ context.Context, id, version, _ string) (capabilitypack.Pack, error) {
		pack, ok := packs[id+"@"+version]
		if !ok {
			pack, ok = packs[id]
		}
		if !ok || pack.Version != version {
			return capabilitypack.Pack{}, fmt.Errorf("Pack %s@%s is unavailable", id, version)
		}
		return pack, nil
	}
}

func (s ownershipStore) LoadSnapshot(context.Context, capabilitypack.Surface) (capabilitypack.ActivationState, error) {
	return s.state, nil
}
func (s ownershipStore) SaveSnapshot(context.Context, capabilitypack.Surface, int, capabilitypack.ActivationState) (int, error) {
	return 1, nil
}

type staticRuntimeEvidence []RuntimeEvidence

func (s staticRuntimeEvidence) ObserveRuntimeEvidence(context.Context) []RuntimeEvidence {
	return append([]RuntimeEvidence(nil), s...)
}

func TestRuntimeEvidenceIsBoundToEveryCurrentProjectionFact(t *testing.T) {
	pack := capabilitypack.Pack{ID: "pack", Version: "3.0.0"}
	auth := AuthorizationObservation{PolicyObserved: true, ToolPermissionObserved: true}
	projections := []capabilitypack.ObservedProjection{
		{ID: "skill:one", Goal: capabilitypack.ProjectionPresent, ObservedFingerprint: "one", DesiredFingerprint: "one", Action: capabilitypack.ProjectionAction{Kind: ActionSkillLink}},
		{ID: "agent:reviewer", Goal: capabilitypack.ProjectionPresent, ObservedFingerprint: "agent", DesiredFingerprint: "agent", Action: capabilitypack.ProjectionAction{Kind: ActionAgentFile}},
	}
	evidence := []RuntimeEvidence{NewRuntimeEvidence(pack, projections[0], "2.1.203", auth, "loading"), NewRuntimeEvidence(pack, projections[1], "2.1.203", auth, "loading")}
	a := (&SurfaceAdapter{}).WithRuntimeEvidence(staticRuntimeEvidence(evidence))
	usable, observed, facts := a.runtimeReadiness(context.Background(), pack, projections, VersionObservation{Version: "2.1.203"}, auth)
	if !usable || !observed || len(facts) != 2 {
		t.Fatalf("usable=%v observed=%v facts=%v", usable, observed, facts)
	}
	projections[0].DesiredFingerprint = "changed-definition"
	if usable, observed, _ := a.runtimeReadiness(context.Background(), pack, projections, VersionObservation{Version: "2.1.203"}, auth); usable || observed {
		t.Fatal("definition change retained stale evidence")
	}
	projections[0].DesiredFingerprint = "one"
	pack.Version = "3.0.1"
	if usable, observed, _ := a.runtimeReadiness(context.Background(), pack, projections, VersionObservation{Version: "2.1.203"}, auth); usable || observed {
		t.Fatal("pack version change retained stale evidence")
	}
}

func TestCapabilityPackOwnershipProviderUsesAllActiveIntentsAndPersistedAliases(t *testing.T) {
	home := t.TempDir()
	layout := NewCanonicalLayout(home)
	agent := func(id, name string) capabilitypack.Resource {
		return capabilitypack.Resource{Kind: "agent", ID: id, Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "agent", Name: name}}}
	}
	packs := map[string]capabilitypack.Pack{
		"one": {ID: "one", Version: "1", Resources: []capabilitypack.Resource{agent("run", "run")}},
		"two": {ID: "two", Version: "2", Resources: []capabilitypack.Resource{agent("check", "check")}},
	}
	for _, name := range []string{"one-run", "two-check"} {
		target := filepath.Join(layout.AgentsDir, name)
		if err := os.MkdirAll(layout.AgentsDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target+".md", []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	state := capabilitypack.ActivationState{
		Intent: capabilitypack.ActivationIntent{PackID: "retired", Version: "9", Surface: capabilitypack.SurfaceClaude, Active: false},
		Intents: []capabilitypack.ActivationIntent{
			{PackID: "retired", Version: "9", Surface: capabilitypack.SurfaceClaude, Active: false},
			{PackID: "one", Version: "1", Surface: capabilitypack.SurfaceClaude, Active: true, Aliases: []capabilitypack.SurfaceAlias{{Kind: "agent", ID: "run", Name: "one-run"}}},
			{PackID: "two", Version: "2", Surface: capabilitypack.SurfaceClaude, Active: true, Aliases: []capabilitypack.SurfaceAlias{{Kind: "agent", ID: "check", Name: "two-check"}}},
		},
		Ownership: []capabilitypack.ProjectionOwnership{{ID: "agent:one-run", PackID: "one", Surface: capabilitypack.SurfaceClaude}, {ID: "agent:two-check", PackID: "two", Surface: capabilitypack.SurfaceClaude}},
	}
	snapshot, err := NewCapabilityPackOwnershipProvider(ownershipStore{state}, ownershipPackResolver(packs), layout, t.TempDir()).ObserveOwnership(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Records) != 2 || snapshot.Records[0].ID != "agent:one-run" || snapshot.Records[1].ID != "agent:two-check" {
		t.Fatalf("records=%+v", snapshot.Records)
	}
}

func TestCompleteRuntimeEvidenceDoesNotRequestMoreRuntimeEvidence(t *testing.T) {
	home := t.TempDir()
	layout := NewCanonicalLayout(home)
	pack := capabilitypack.Pack{ID: "p", Version: "1", Resources: []capabilitypack.Resource{{Kind: "agent", ID: "reviewer", Source: "reviewer.md", Bindings: []capabilitypack.Binding{{Surface: capabilitypack.SurfaceClaude, Projection: "agent", Name: "reviewer"}}}}}
	if err := os.MkdirAll(layout.AgentsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(home, "reviewer.md"), filepath.Join(layout.AgentsDir, "reviewer.md")} {
		if err := os.WriteFile(path, []byte("Review the changes."), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	auth := AuthorizationObservation{PolicyObserved: true, ToolPermissionObserved: true}
	base := NewSurfaceAdapterWithAuthorization(home, layout, filepath.Join(home, "state"), "claude", &recordingRunner{result: Result{Stdout: "2.1.203"}}, StaticOwnershipSnapshot(OwnershipSnapshot{}), AuthorizationObserverFunc(func(context.Context) AuthorizationObservation { return auth }))
	first, err := base.InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	evidence := []RuntimeEvidence{NewRuntimeEvidence(pack, first.Projections[0], "2.1.203", auth, "loading")}
	complete, err := base.WithRuntimeEvidence(staticRuntimeEvidence(evidence)).InspectSurface(context.Background(), capabilitypack.SurfaceTransition{Desired: pack})
	if err != nil {
		t.Fatal(err)
	}
	if !complete.Readiness.UsabilityObserved || !complete.Readiness.Usable {
		t.Fatalf("readiness=%+v", complete.Readiness)
	}
	for _, action := range complete.Readiness.PendingHumanActions {
		if strings.Contains(action, "runtime evidence") {
			t.Fatalf("unexpected runtime request: %v", complete.Readiness.PendingHumanActions)
		}
	}
}
