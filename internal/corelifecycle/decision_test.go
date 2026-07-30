package corelifecycle

import (
	"reflect"
	"testing"
)

func TestPlanDecisionGuidanceOwnsRiskRecoveryAndRetryPolicy(t *testing.T) {
	plan := Plan{
		operation: Install,
		pending:   []string{"approve Claude MCP registration"},
		blockers:  []string{"remove foreign Claude skill path"},
		warnings:  []string{"operator content is preserved"},
		recovery:  []string{"restore the owned prompt snapshot"},
	}

	got := plan.DecisionGuidance()
	if !reflect.DeepEqual(got.Risks, []string{
		"prerequisite: approve Claude MCP registration",
		"blocker: remove foreign Claude skill path",
		"warning: operator content is preserved",
	}) {
		t.Fatalf("risks = %#v", got.Risks)
	}
	if !reflect.DeepEqual(got.Recovery, []string{"restore the owned prompt snapshot"}) {
		t.Fatalf("recovery = %#v", got.Recovery)
	}
	if got.NextCommand != "resolve blockers above, then run packy install" {
		t.Fatalf("next command = %q", got.NextCommand)
	}

	got.Risks[0] = "mutated"
	got.Recovery[0] = "mutated"
	if plan.pending[0] != "approve Claude MCP registration" ||
		plan.recovery[0] != "restore the owned prompt snapshot" {
		t.Fatalf("caller mutated opaque plan: %#v", plan)
	}
}

func TestPlanDecisionGuidanceUsesRecoveryBeforeRetry(t *testing.T) {
	plan := Plan{operation: Update, recovery: []string{"verify the recovery state"}}
	if got := plan.DecisionGuidance(); got.NextCommand != "follow recovery guidance above, then run packy update" {
		t.Fatalf("next command = %q", got.NextCommand)
	}
	if got := (Plan{operation: Uninstall}).DecisionGuidance(); got.NextCommand != "packy uninstall" {
		t.Fatalf("plain next command = %q", got.NextCommand)
	}
}
