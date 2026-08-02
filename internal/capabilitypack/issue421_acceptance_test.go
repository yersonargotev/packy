package capabilitypack

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIssue421ReconcileCannotPlanOrExecuteAcquisition(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{missingEngramResolution()}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"))
	store.state = ActivationState{Intent: activeIntent("engram", "1.0.0", 6)}

	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers()) != 1 || !strings.Contains(plan.Blockers()[0].Detail, "activation or update") {
		t.Fatalf("reconcile acquisition blockers = %+v", plan.Blockers())
	}
	if actions := externalEffectActions(plan.Phases()); len(actions) != 0 {
		t.Fatalf("reconcile planned external effects = %+v", actions)
	}

	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: requiredApprovals(facade, plan), Interactive: true})
	if !errors.Is(err, ErrPlanNotActionable) {
		t.Fatalf("reconcile apply error = %v", err)
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 || len(executor.actions) != 0 {
		t.Fatalf("blocked reconcile caused effects: saves=%d host=%d external=%d", len(store.saves), len(adapter.actions), len(executor.actions))
	}
}

func TestIssue421ReconcileCannotPlanOrExecuteToolHostSetup(t *testing.T) {
	resolver := &fakeExecutableResolver{resolutions: []ExecutableResolution{availableEngramResolution("/opt/homebrew/bin/engram")}}
	executor := &fakeExternalExecutor{}
	facade, adapter, store := engramFacadeForTest(resolver, executor, engramObservation("missing"))
	store.state = ActivationState{Intent: activeIntent("engram", "1.0.0", 6)}

	plan, err := facade.PreviewReconcile(context.Background(), ReconcileRequest{PackID: "engram", Surface: SurfaceCodex})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers()) != 1 || !strings.Contains(plan.Blockers()[0].Detail, "tool-owned host setup") ||
		!strings.Contains(plan.Blockers()[0].Detail, "activation or update") {
		t.Fatalf("reconcile setup blockers = %+v", plan.Blockers())
	}
	if actions := externalEffectActions(plan.Phases()); len(actions) != 0 {
		t.Fatalf("reconcile planned external effects = %+v", actions)
	}

	_, err = facade.Apply(context.Background(), ApplyRequest{Plan: plan, Approvals: requiredApprovals(facade, plan), Interactive: true})
	if !errors.Is(err, ErrPlanNotActionable) {
		t.Fatalf("reconcile apply error = %v", err)
	}
	if len(store.saves) != 0 || len(adapter.actions) != 0 || len(executor.actions) != 0 {
		t.Fatalf("blocked reconcile caused effects: saves=%d host=%d external=%d", len(store.saves), len(adapter.actions), len(executor.actions))
	}
}
