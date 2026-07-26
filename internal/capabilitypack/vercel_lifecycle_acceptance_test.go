package capabilitypack

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVercelCollisionRequiresExplicitAliasBeforeMutation(t *testing.T) {
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude} {
		t.Run(string(surface), func(t *testing.T) {
			testVercelCollisionRequiresExplicitAliasBeforeMutation(t, surface)
		})
	}
}

func testVercelCollisionRequiresExplicitAliasBeforeMutation(t *testing.T, surface Surface) {
	catalog := completeVercelCatalog(t)
	const resourceID = "vercel-composition-patterns"
	adapter := &fakeSurfaceAdapter{inspect: func(transition SurfaceTransition) SurfaceInspection {
		inspection := completeVercelObservation(transition.Desired, "missing", surface)
		inspection.OccupiedNames = []OccupiedName{{
			Namespace: "skill", Name: resourceID, OwnerType: "unmanaged", Fingerprint: "operator",
		}}
		for i := range inspection.Projections {
			if inspection.Projections[i].ID == "skill:"+resourceID {
				inspection.Projections[i].Exists = true
				inspection.Projections[i].ObservedFingerprint = "operator"
			}
		}
		return inspection
	}}
	store := &fakeActivationStore{}
	facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{surface: adapter}))

	blocked, err := facade.Preview(context.Background(), ActivationRequest{PackID: "vercel", Surface: surface})
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Applicable() || len(blocked.Blockers()) != 1 {
		t.Fatalf("unmanaged Vercel collision was not a complete-surface blocker: %+v", blocked.JSONReport(true))
	}
	if _, err := facade.Apply(context.Background(), ApplyRequest{Plan: blocked, Interactive: true}); !errors.Is(err, ErrPlanNotActionable) {
		t.Fatalf("blocked Apply error = %v", err)
	}

	alias := SurfaceAlias{Kind: "skill", ID: resourceID, Name: "vercel-pack-" + resourceID}
	replanned, err := facade.Preview(context.Background(), ActivationRequest{
		PackID: "vercel", Surface: surface, Aliases: []SurfaceAlias{alias},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !replanned.Applicable() || len(replanned.Blockers()) != 0 ||
		!reflect.DeepEqual(replanned.Aliases(), []SurfaceAlias{alias}) ||
		replanned.ID() == blocked.ID() || replanned.Digest() == blocked.Digest() {
		t.Fatalf("explicit Vercel alias did not create a fresh applicable plan: %+v", replanned.JSONReport(true))
	}
	if len(adapter.actions) != 0 || len(store.saves) != 0 {
		t.Fatal("collision and alias previews crossed the mutation boundary")
	}

	prefix := "$"
	switch surface {
	case SurfaceClaude:
		prefix = "/"
	case SurfaceOpenCode:
		prefix = ""
	}
	requested := Pack{ID: "vercel", Version: "1.0.0", manifestVersion: manifestSchemaV4, Surfaces: []Surface{surface}, Resources: []Resource{{
		Kind: "skill", ID: resourceID, Bindings: []Binding{{
			Surface: surface, Projection: "skill", Name: resourceID, Invocation: prefix + resourceID, Mode: "native", Sharing: "exclusive",
		}},
	}}}
	other := Pack{ID: "other-pack", Version: "1.0.0", manifestVersion: manifestSchemaV4, Surfaces: []Surface{surface}, Resources: []Resource{{
		Kind: "skill", ID: "other-skill", Source: "skills/other", Bindings: []Binding{{
			Surface: surface, Projection: "skill", Name: resourceID, Invocation: prefix + resourceID, Mode: "native", Sharing: "exclusive",
		}},
	}}}
	otherIntent := ActivationIntent{PackID: other.ID, Version: other.Version, Surface: surface, Active: true}
	incompatible, err := NewFacade(Catalog{packs: []Pack{requested, other}}).compose(
		requested,
		ActivationState{Intent: otherIntent, Intents: []ActivationIntent{otherIntent}},
		surface,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(incompatible.blockers) != 1 || incompatible.blockers[0].Kind != BlockerAlias || incompatible.blockers[0].Subject != "personal-skill:"+resourceID {
		t.Fatalf("incompatible exclusive Vercel collision blockers = %+v", incompatible.blockers)
	}
}

func TestVercelLifecycleIsAtomicStaleSafeRecoverableAndOwnershipSafe(t *testing.T) {
	for _, surface := range []Surface{SurfaceCodex, SurfaceOpenCode, SurfaceClaude} {
		t.Run(string(surface), func(t *testing.T) {
			testVercelLifecycleIsAtomicStaleSafeRecoverableAndOwnershipSafe(t, surface)
		})
	}
}

func testVercelLifecycleIsAtomicStaleSafeRecoverableAndOwnershipSafe(t *testing.T, surface Surface) {
	catalog := completeVercelCatalog(t)
	pack, err := catalog.Show("vercel")
	if err != nil {
		t.Fatal(err)
	}
	pending := completeVercelObservation(pack, "missing", surface)
	changed := completeVercelObservation(pack, "operator-change", surface)

	t.Run("stale-preflight-zero-effects", func(t *testing.T) {
		adapter := &fakeSurfaceAdapter{observations: []SurfaceInspection{pending, changed}}
		store := &fakeActivationStore{}
		facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{surface: adapter}))
		plan, err := facade.Preview(context.Background(), ActivationRequest{PackID: "vercel", Surface: surface})
		if err != nil {
			t.Fatal(err)
		}
		_, err = facade.Apply(context.Background(), ApplyRequest{
			Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true,
		})
		if !errors.Is(err, ErrStalePlan) || len(adapter.actions) != 0 || len(store.saves) != 0 {
			t.Fatalf("stale Vercel plan crossed boundary: err=%v actions=%d saves=%d", err, len(adapter.actions), len(store.saves))
		}
	})

	t.Run("atomic-failure-fresh-recovery", func(t *testing.T) {
		adapter := &fakeSurfaceAdapter{
			observations: []SurfaceInspection{pending, pending, pending},
			applyErr:     errors.New("atomic adapter interruption"),
		}
		store := &fakeActivationStore{}
		facade := NewFacade(catalog, WithActivation(store, map[Surface]SurfaceAdapter{surface: adapter}))
		plan, _ := facade.Preview(context.Background(), ActivationRequest{PackID: "vercel", Surface: surface})
		if _, err := facade.Apply(context.Background(), ApplyRequest{
			Plan: plan, Approvals: []ApprovalReceipt{facade.Approve(plan, ConsentReversibleLocal)}, Interactive: true,
		}); err == nil {
			t.Fatal("atomic interruption unexpectedly succeeded")
		}
		if store.state.Journal == nil || !store.state.Intent.Active || len(store.state.Ownership) != 0 {
			t.Fatalf("failed Vercel attempt state = %+v", store.state)
		}
		firstAttempt := cloneJournal(*store.state.Journal)
		adapter.applyErr = nil
		adapter.inspectCalls = 0
		verified := completeVercelObservation(pack, "desired", surface)
		adapter.observations = []SurfaceInspection{pending, pending, verified}
		recovery, err := facade.Preview(context.Background(), ActivationRequest{PackID: "vercel", Surface: surface})
		if err != nil {
			t.Fatal(err)
		}
		if !recovery.Recovery() || recovery.ID() == plan.ID() || recovery.HistoricalAttempt() == nil {
			t.Fatalf("fresh recovery plan = %+v", recovery.JSONReport(true))
		}
		result, err := facade.Apply(context.Background(), ApplyRequest{
			Plan: recovery, Approvals: []ApprovalReceipt{facade.Approve(recovery, ConsentReversibleLocal)}, Interactive: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.Verified || len(store.state.Ownership) != 9 || store.state.Journal != nil ||
			len(store.state.History) != 1 || !reflect.DeepEqual(store.state.History[0], firstAttempt) {
			t.Fatalf("recovered Vercel state/result = %+v %+v", store.state, result)
		}

		before := cloneActivationState(store.state)
		adapter.inspectCalls = 0
		adapter.observations = []SurfaceInspection{verified}
		update, err := facade.PreviewUpdate(context.Background(), UpdateRequest{PackID: "vercel", Surface: surface})
		if err != nil {
			t.Fatal(err)
		}
		if !update.NoOp() || len(update.Phases()) != 0 || !reflect.DeepEqual(store.state, before) {
			t.Fatalf("exact Vercel update changed lifecycle state: %+v", update.JSONReport(true))
		}

		drifted := completeVercelObservation(pack, "desired", surface)
		drifted.Projections[0].ObservedFingerprint = "operator-drift"
		adapter.inspectCalls = 0
		adapter.actions = nil
		adapter.observations = []SurfaceInspection{drifted, drifted, verified}
		reconcile, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "vercel", Surface: surface})
		if err != nil {
			t.Fatal(err)
		}
		reconciled, err := facade.Apply(context.Background(), ApplyRequest{
			Plan: reconcile, Approvals: []ApprovalReceipt{facade.Approve(reconcile, ConsentReversibleLocal)}, Interactive: true,
		})
		if err != nil || !reconciled.Verified || len(adapter.actions) != 1 {
			t.Fatalf("Vercel reconcile result=%+v actions=%+v err=%v", reconciled, adapter.actions, err)
		}
		if !reflect.DeepEqual(store.state.Intent, before.Intent) {
			t.Fatal("Vercel reconcile changed durable intent")
		}

		driftedRemoval := completeVercelRemovalObservation(store.state.Ownership, "desired")
		driftedRemoval.Projections[0].ObservedFingerprint = "operator-drift"
		adapter.inspectCalls = 0
		adapter.actions = nil
		adapter.observations = []SurfaceInspection{driftedRemoval}
		deactivate, err := facade.PreviewDeactivate(context.Background(), DeactivationRequest{PackID: "vercel", Surface: surface})
		if err != nil {
			t.Fatal(err)
		}
		phases := deactivate.Phases()
		if !deactivate.Applicable() || len(deactivate.PendingHumanActions()) == 0 ||
			len(phases) != 1 || len(phases[0].Actions) != 8 || len(adapter.actions) != 0 {
			t.Fatalf("drifted Vercel deactivation was not preserved: %+v", deactivate.JSONReport(true))
		}
		for _, action := range phases[0].Actions {
			if action.ID == driftedRemoval.Projections[0].ID {
				t.Fatalf("drifted Vercel target %s was scheduled for deletion", action.ID)
			}
		}
	})
}

func completeVercelCatalog(t *testing.T) Catalog {
	t.Helper()
	bundle := filepath.Join(t.TempDir(), "bundle")
	materializeVercelAcceptanceArchive(t, bundle)
	manifest, err := os.ReadFile(filepath.Join("..", "vercelacceptance", "testdata", "vercel-pack-v4.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bundle, "packs", "vercel", "pack.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := discoverCatalogWithSourceValidation(bundle, []catalogEntry{{
		ID: "vercel", Description: "detached Vercel acceptance cohort",
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func materializeVercelAcceptanceArchive(t *testing.T, bundle string) {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "vercelacceptance", "testdata", "vercel-1.0.0.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(header.Name))
		if clean == "." || filepath.IsAbs(clean) || clean != filepath.FromSlash(header.Name) ||
			strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			t.Fatalf("unsafe Vercel acceptance archive path %q", header.Name)
		}
		target := filepath.Join(bundle, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, content, os.FileMode(header.Mode)&0o777); err != nil {
			t.Fatal(err)
		}
	}
}

func completeVercelObservation(pack Pack, observed string, surface Surface) SurfaceInspection {
	inspection := SurfaceInspection{Revision: "vercel-" + string(surface) + "-host"}
	actionKind := ActionSkillLink
	if surface == SurfaceOpenCode {
		actionKind = ActionOpenCodeSkillLink
	}
	for _, resource := range pack.Resources {
		for _, binding := range resource.Bindings {
			if binding.Surface != surface {
				continue
			}
			inspection.Projections = append(inspection.Projections, ObservedProjection{
				ID: "skill:" + binding.Name, Goal: ProjectionPresent,
				Exists: observed == "desired", ObservedFingerprint: observed, DesiredFingerprint: "desired",
				Action: ProjectionAction{ID: "skill:" + binding.Name, Kind: actionKind, Description: "project " + resource.ID},
			})
		}
	}
	return inspection
}

func completeVercelRemovalObservation(ownership []ProjectionOwnership, observed string) SurfaceInspection {
	inspection := SurfaceInspection{Revision: "vercel-codex-removal"}
	for _, owner := range ownership {
		inspection.Projections = append(inspection.Projections, ObservedProjection{
			ID: owner.ID, Goal: ProjectionAbsent, Exists: true,
			ObservedFingerprint: observed,
			Action:              ProjectionAction{ID: owner.ID, Kind: ActionSkillLink, Mode: ProjectionDeleteTarget, Description: "remove " + owner.ID},
		})
	}
	return inspection
}
