package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func sharedSkillObservation(surface Surface, exists bool, observed, desired string, goal ProjectionGoal) SurfaceInspection {
	provenance := "test-adapter/v1/" + string(surface)
	mode := ProjectionActionMode("")
	if goal == ProjectionAbsent {
		mode = ProjectionDeleteTarget
	}
	return SurfaceInspection{Revision: "host", Projections: []ObservedProjection{{
		ID: "skill:shared", ProjectionKey: "global-skill:shared", Shared: true,
		DiscoverableBy: []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude}, AdapterProvenance: provenance,
		Goal: goal, Exists: exists, ObservedFingerprint: observed, DesiredFingerprint: desired,
		Action: ProjectionAction{ID: "skill:shared", ProjectionKey: "global-skill:shared", Shared: true,
			DiscoverableBy: []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude}, AdapterProvenance: provenance,
			Mode: mode, Description: "project shared skill"},
	}}}
}

func applyApprovedPlan(t *testing.T, facade Facade, plan ReconciliationPlan) {
	t.Helper()
	var approvals []ApprovalReceipt
	for _, phase := range plan.Phases() {
		if phase.ApprovalRequired {
			approvals = append(approvals, facade.Approve(plan, phase.Kind))
		}
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: approvals, Interactive: true}); err != nil {
		t.Fatal(err)
	}
}

func TestIssue420SharedProjectionCrossSurfaceRetentionCleanupAndStaleCAS(t *testing.T) {
	pack := Pack{ID: "shared", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode}, Resources: []Resource{{
		Kind: "skill", ID: "shared", Source: "/skills/shared", Bindings: []Binding{
			{Surface: SurfaceCodex, Projection: "skill", Name: "shared", Mode: "native", Sharing: "shared"},
			{Surface: SurfaceOpenCode, Projection: "skill", Name: "shared", Mode: "native", Sharing: "shared"},
		},
	}}}
	pendingCodex := sharedSkillObservation(SurfaceCodex, false, "", "same", ProjectionPresent)
	verifiedCodex := sharedSkillObservation(SurfaceCodex, true, "same", "same", ProjectionPresent)
	verifiedOpen := sharedSkillObservation(SurfaceOpenCode, true, "same", "same", ProjectionPresent)
	retainedCodex := sharedSkillObservation(SurfaceCodex, true, "same", "same", ProjectionPresent)
	removeOpen := sharedSkillObservation(SurfaceOpenCode, true, "same", "", ProjectionAbsent)
	removedOpen := sharedSkillObservation(SurfaceOpenCode, false, "", "", ProjectionAbsent)
	codexAdapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pendingCodex, pendingCodex, verifiedCodex, retainedCodex, retainedCodex, retainedCodex, retainedCodex}}
	openAdapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{verifiedOpen, verifiedOpen, verifiedOpen, removeOpen, removeOpen, removedOpen}}
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: codexAdapter, SurfaceOpenCode: openAdapter}))

	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode} {
		plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "shared", Surface: surface})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.JSONReport(false).SharedProjections) != 1 || !strings.Contains(plan.JSONReport(false).SharedProjections[0].DiscoveryNotice, "does not create activation intent") {
			t.Fatalf("shared preview disclosure = %+v", plan.JSONReport(false).SharedProjections)
		}
		applyApprovedPlan(t, facade, plan)
		if surface == SurfaceCodex {
			discovered, err := store.LoadSnapshot(context.Background(), SurfaceOpenCode)
			if err != nil {
				t.Fatal(err)
			}
			if discovered.Intent.Active || len(discovered.Intents) != 0 || len(discovered.Ownership) != 1 {
				t.Fatalf("incidental discovery created OpenCode intent: %+v", discovered)
			}
		}
	}

	state, err := store.LoadSnapshot(context.Background(), SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := ownershipByID(state.Ownership, "global-skill:shared")
	if !ok || len(owner.Contributors) != 2 || len(owner.Authorities) != 2 {
		t.Fatalf("cross-surface owner = %+v", owner)
	}

	stale, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "shared", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	other := state
	if _, err := store.SaveSnapshot(context.Background(), SurfaceCodex, other.documentRevision, other); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: stale, Interactive: true}); !errors.Is(err, ErrStalePlan) {
		t.Fatalf("cross-surface snapshot race = %v", err)
	}

	retention, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "shared", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range retention.Phases() {
		if phase.Kind == ConsentDestructiveCleanup {
			t.Fatalf("one-contributor removal became destructive: %+v", phase)
		}
	}
	if retained := retention.RetainedProjections(); len(retained) != 1 || len(retained[0].Contributors) != 1 || !strings.Contains(retained[0].Contributors[0], "surface:opencode:") {
		t.Fatalf("retention preview contributors = %+v", retained)
	}
	applyApprovedPlan(t, facade, retention)
	state, err = store.LoadSnapshot(context.Background(), SurfaceOpenCode)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok = ownershipByID(state.Ownership, "global-skill:shared")
	if !ok || len(owner.Contributors) != 1 || !strings.Contains(owner.Contributors[0], "surface:opencode:") {
		t.Fatalf("retained shared owner = %+v", owner)
	}

	cleanup, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "shared", Surface: SurfaceOpenCode})
	if err != nil {
		t.Fatal(err)
	}
	if phases := cleanup.Phases(); len(phases) != 1 || phases[0].Kind != ConsentDestructiveCleanup {
		t.Fatalf("last-contributor cleanup phases = %+v pending=%+v blockers=%+v owner=%+v", phases, cleanup.PendingHumanActions(), cleanup.Blockers(), owner)
	}
	applyApprovedPlan(t, facade, cleanup)
	state, err = store.LoadSnapshot(context.Background(), SurfaceOpenCode)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ownershipByID(state.Ownership, "global-skill:shared"); ok {
		t.Fatalf("last-contributor ownership retained: %+v", state.Ownership)
	}
}

func TestIssue420SharedProjectionStatusDisclosesDiscoveryWithoutIntent(t *testing.T) {
	projection := sharedSkillObservation(SurfaceCodex, true, "same", "same", ProjectionPresent).Projections[0]
	owner := ProjectionOwnership{ID: "global-skill:shared", ProjectionID: projection.ID, Fingerprint: "same", Contributors: []string{qualifyContributor(SurfaceCodex, "pack:shared:skill:shared")}}
	composition := composition{surface: SurfaceCodex, contributors: map[string][]string{projection.ID: {"pack:shared:skill:shared"}}}
	details, _ := deriveProjectionStatus("shared", []ObservedProjection{projection}, []ProjectionOwnership{owner}, composition)
	jsonDetails := jsonProjectionDetails(details)
	if len(jsonDetails) != 1 || !jsonDetails[0].Shared || len(jsonDetails[0].DiscoverableBy) != 3 || !strings.Contains(jsonDetails[0].DiscoveryNotice, "does not create activation intent") {
		t.Fatalf("shared status disclosure = %+v", jsonDetails)
	}
}
