package capabilitypack

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
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

func TestIssue420ContributorsAcrossCodexOpenCodeAndClaude(t *testing.T) {
	// This synthetic topology exercises the surface-neutral ownership policy.
	// Real host adapters remain authoritative for which physical targets they share.
	pack := Pack{manifestVersion: manifestSchemaV4, ID: "shared", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude}, Resources: []Resource{{
		Kind: "skill", ID: "shared", Source: "/skills/shared", Bindings: []Binding{
			{Surface: SurfaceCodex, Projection: "skill", Name: "shared", Mode: "native", Sharing: "shared"},
			{Surface: SurfaceOpenCode, Projection: "skill", Name: "shared", Mode: "native", Sharing: "shared"},
			{Surface: SurfaceClaude, Projection: "skill", Name: "shared", Mode: "native", Sharing: "shared"},
		},
	}}}
	pendingCodex := sharedSkillObservation(SurfaceCodex, false, "", "same", ProjectionPresent)
	verified := map[Surface]SurfaceInspection{
		SurfaceCodex:    sharedSkillObservation(SurfaceCodex, true, "same", "same", ProjectionPresent),
		SurfaceOpenCode: sharedSkillObservation(SurfaceOpenCode, true, "same", "same", ProjectionPresent),
		SurfaceClaude:   sharedSkillObservation(SurfaceClaude, true, "same", "same", ProjectionPresent),
	}
	adapters := map[Surface]SurfaceAdapter{
		SurfaceCodex:    &fakeSurfaceAdapter{observations: []SurfaceInspection{pendingCodex, pendingCodex, verified[SurfaceCodex], verified[SurfaceCodex]}},
		SurfaceOpenCode: &fakeSurfaceAdapter{observations: []SurfaceInspection{verified[SurfaceOpenCode], verified[SurfaceOpenCode], verified[SurfaceOpenCode]}},
		SurfaceClaude:   &fakeSurfaceAdapter{observations: []SurfaceInspection{verified[SurfaceClaude], verified[SurfaceClaude], verified[SurfaceClaude]}},
	}
	store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, adapters))

	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude} {
		plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: surface})
		if err != nil {
			t.Fatal(err)
		}
		applyApprovedPlan(t, facade, plan)
	}

	state, err := store.LoadSnapshot(context.Background(), SurfaceClaude)
	if err != nil {
		t.Fatal(err)
	}
	owner, ok := ownershipByID(state.Ownership, "global-skill:shared")
	if !ok || len(owner.Contributors) != 3 || len(owner.Authorities) != 3 {
		t.Fatalf("three-surface owner = %+v", owner)
	}
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude} {
		want := qualifyContributor(surface, "pack:shared:skill:shared")
		if !slices.Contains(owner.Contributors, want) {
			t.Fatalf("shared owner missing %q: %+v", want, owner)
		}
	}

	report, err := facade.Status(context.Background(), StatusRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 1 || len(report.Entries[0].ProjectionDetails) != 1 || !report.Entries[0].ProjectionDetails[0].Shared ||
		!strings.Contains(report.Entries[0].ProjectionDetails[0].DiscoveryNotice, "does not create activation intent") {
		t.Fatalf("facade shared status = %+v", report.Entries)
	}
}

func TestIssue420SharedProjectionRequiresApprovedActivationPlan(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selection ResourceSelection
	}{
		{name: "full-pack", selection: ResourceSelection{Mode: SelectionAll}},
		{name: "resource-scoped", selection: ResourceSelection{Mode: SelectionCustom, Roots: []ResourceIdentity{{Kind: "skill", ID: "shared"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pack := Pack{manifestVersion: manifestSchemaV4, ID: "shared", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{
				{Kind: "skill", ID: "shared", Source: "/skills/shared", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "skill", Name: "shared", Mode: "native", Sharing: "shared"}}},
				{Kind: "instruction", ID: "unselected", Source: "/instructions/unselected", Bindings: []Binding{{Surface: SurfaceCodex, Projection: "instruction", Name: "unselected", Mode: "native"}}},
			}}
			pending := sharedSkillObservation(SurfaceCodex, false, "", "same", ProjectionPresent)
			verified := sharedSkillObservation(SurfaceCodex, true, "same", "same", ProjectionPresent)
			adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, pending, verified}}
			store := NewFileActivationStore(filepath.Join(t.TempDir(), "packs.json"))
			facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

			plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: pack.ID, Surface: SurfaceCodex, Selection: tc.selection})
			if err != nil {
				t.Fatal(err)
			}
			if shared := plan.JSONReport(false).SharedProjections; len(shared) != 1 || shared[0].ProjectionKey != "global-skill:shared" {
				t.Fatalf("shared activation preview = %+v", shared)
			}
			if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
				t.Fatalf("unapproved shared activation = %v", err)
			}
			before, err := store.LoadSnapshot(context.Background(), SurfaceCodex)
			if err != nil {
				t.Fatal(err)
			}
			if len(before.Ownership) != 0 || len(adapter.actions) != 0 {
				t.Fatalf("preview or rejected apply mutated state: ownership=%+v actions=%+v", before.Ownership, adapter.actions)
			}
			applyApprovedPlan(t, facade, plan)
			after, err := store.LoadSnapshot(context.Background(), SurfaceCodex)
			if err != nil {
				t.Fatal(err)
			}
			owner, ok := ownershipByID(after.Ownership, "global-skill:shared")
			if !ok || len(owner.Contributors) != 1 || after.Intent.Selection.Mode != tc.selection.Mode {
				t.Fatalf("approved shared activation state = %+v", after)
			}
		})
	}
}

func TestIssue420DeactivationRetainsProjectionWhileContributorRemains(t *testing.T) {
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

func TestIssue420LastContributorRequiresOwnershipAndDestructiveConsent(t *testing.T) {
	pack := Pack{ID: "shared", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "skill", ID: "shared", Source: "/skills/shared"}}}
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll}}
	state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}, Ownership: []ProjectionOwnership{{
		ID: "global-skill:shared", ProjectionID: "skill:shared", Contributors: []string{qualifyContributor(SurfaceCodex, "pack:shared:skill:shared")},
		Fingerprint: "same", Authorities: []ProjectionAuthority{{Surface: SurfaceCodex, AdapterProvenance: "test-adapter/v1/codex"}},
	}}, snapshotManaged: true}
	removing := sharedSkillObservation(SurfaceCodex, true, "same", "", ProjectionAbsent)
	removed := sharedSkillObservation(SurfaceCodex, false, "", "", ProjectionAbsent)
	adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{removing, removing, removed}}
	store := &fakeActivationStore{state: state}
	facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

	plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if phases := plan.Phases(); len(phases) != 1 || phases[0].Kind != ConsentDestructiveCleanup {
		t.Fatalf("last-contributor phases = %+v", phases)
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: plan, Interactive: true}); !errors.Is(err, ErrApprovalMismatch) {
		t.Fatalf("unapproved last-contributor cleanup = %v", err)
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 {
		t.Fatalf("unapproved cleanup mutated state: saves=%d actions=%+v", len(store.saves), adapter.actions)
	}
	applyApprovedPlan(t, facade, plan)
	if len(adapter.actions) != 1 {
		t.Fatalf("approved cleanup actions = %+v", adapter.actions)
	}
}

func TestIssue420SharedProjectionCollisionsArePreservedAndReported(t *testing.T) {
	pack := Pack{ID: "shared", Version: "1.0.0", Surfaces: []Surface{SurfaceCodex}, Resources: []Resource{{Kind: "skill", ID: "shared", Source: "/skills/shared"}}}
	intent := ActivationIntent{PackID: pack.ID, Surface: SurfaceCodex, Version: pack.Version, Active: true, Revision: 1, Selection: ResourceSelection{Mode: SelectionAll}}
	exact := ProjectionOwnership{ID: "global-skill:shared", ProjectionID: "skill:shared", Contributors: []string{qualifyContributor(SurfaceCodex, "pack:shared:skill:shared")}, Fingerprint: "same", Authorities: []ProjectionAuthority{{Surface: SurfaceCodex, AdapterProvenance: "test-adapter/v1/codex"}}}
	ambiguous := exact
	ambiguous.Contributors = append(append([]string(nil), exact.Contributors...), qualifyContributor(SurfaceOpenCode, "pack:foreign:skill:shared"))
	for _, tc := range []struct {
		name      string
		observed  string
		ownership []ProjectionOwnership
	}{
		{name: "modified", observed: "operator-content", ownership: []ProjectionOwnership{exact}},
		{name: "ambiguous", observed: "same", ownership: []ProjectionOwnership{ambiguous}},
		{name: "unowned", observed: "same"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := ActivationState{Intent: intent, Intents: []ActivationIntent{intent}, Ownership: tc.ownership, snapshotManaged: true}
			observation := sharedSkillObservation(SurfaceCodex, true, tc.observed, "", ProjectionAbsent)
			adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{observation}}
			store := &fakeActivationStore{state: state}
			facade := NewFacade(Catalog{packs: []Pack{pack}}, WithActivation(store, map[Surface]SurfaceAdapter{SurfaceCodex: adapter}))

			plan, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: pack.ID, Surface: SurfaceCodex})
			if err != nil {
				t.Fatal(err)
			}
			if !plan.Applicable() || len(plan.PendingHumanActions()) == 0 || len(plan.Phases()) != 0 || len(adapter.actions) != 0 || len(store.saves) != 0 {
				t.Fatalf("collision was not preserved: applicable=%v blockers=%+v pending=%+v actions=%+v saves=%d", plan.Applicable(), plan.Blockers(), plan.PendingHumanActions(), adapter.actions, len(store.saves))
			}
		})
	}
}

func TestIssue420PreviewAndStatusDiscloseSharedDiscoveryWithoutIntent(t *testing.T) {
	projection := sharedSkillObservation(SurfaceCodex, true, "same", "same", ProjectionPresent).Projections[0]
	owner := ProjectionOwnership{ID: "global-skill:shared", ProjectionID: projection.ID, Fingerprint: "same", Contributors: []string{qualifyContributor(SurfaceCodex, "pack:shared:skill:shared")}}
	composition := composition{surface: SurfaceCodex, contributors: map[string][]string{projection.ID: {"pack:shared:skill:shared"}}}
	details, _ := deriveProjectionStatus("shared", []ObservedProjection{projection}, []ProjectionOwnership{owner}, composition)
	jsonDetails := jsonProjectionDetails(details)
	if len(jsonDetails) != 1 || !jsonDetails[0].Shared || len(jsonDetails[0].DiscoverableBy) != 3 || !strings.Contains(jsonDetails[0].DiscoveryNotice, "does not create activation intent") {
		t.Fatalf("shared status disclosure = %+v", jsonDetails)
	}
}
