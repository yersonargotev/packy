package vercelacceptance_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/vercelacceptance"
)

func TestCanonicalRuntimeContractHasFreshExactCodexPreflight(t *testing.T) {
	fixture := vercelacceptance.Canonical()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	records := availableRuntimeEvidence(fixture.Pack, now)

	first, err := capabilitypack.EvaluateRuntimeModes(fixture.Pack, records, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := capabilitypack.EvaluateRuntimeModes(fixture.Pack, records, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 28 {
		t.Fatalf("evaluated modes = %d, want 28", len(first))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical fresh evidence produced different runtime results")
	}
	for _, result := range first {
		if result.State != capabilitypack.RuntimeModeAvailable {
			t.Fatalf("%s:%s state = %s", result.ResourceID, result.ModeID, result.State)
		}
		got, err := capabilitypack.PreflightRuntimeMode(
			fixture.Pack, result.ResourceID, result.ModeID, records, now, time.Hour,
		)
		if err != nil {
			t.Fatalf("%s:%s preflight: %v", result.ResourceID, result.ModeID, err)
		}
		if !reflect.DeepEqual(got.Requirements, result.Requirements) ||
			!reflect.DeepEqual(got.Authorities, result.Authorities) ||
			!reflect.DeepEqual(got.Effects, result.Effects) ||
			!reflect.DeepEqual(got.Fallback, result.Fallback) {
			t.Fatalf("%s:%s declaration changed during preflight", result.ResourceID, result.ModeID)
		}
	}

	stale := append([]capabilitypack.RuntimeModeEvidence(nil), records...)
	staleIndex := -1
	for i := range stale {
		if len(stale[i].Evidence.Requirements) > 0 {
			staleIndex = i
			break
		}
	}
	if staleIndex == -1 {
		t.Fatal("fixture must carry a runtime requirement for stale-evidence acceptance")
	}
	stale[staleIndex].Evidence.Requirements = append(
		[]capabilitypack.RuntimeRequirementObservation(nil),
		stale[staleIndex].Evidence.Requirements...,
	)
	stale[staleIndex].Evidence.Requirements[0].ObservedAt = now.Add(-2 * time.Hour).Format(time.RFC3339)
	if _, err := capabilitypack.PreflightRuntimeMode(
		fixture.Pack, stale[staleIndex].ResourceID, stale[staleIndex].ModeID, stale, now, time.Hour,
	); err == nil {
		t.Fatal("stale indispensable fact must fail before effects")
	}
}

func availableRuntimeEvidence(pack capabilitypack.Pack, observedAt time.Time) []capabilitypack.RuntimeModeEvidence {
	records := make([]capabilitypack.RuntimeModeEvidence, 0, 28)
	observation := capabilitypack.RuntimeObservation{
		State:            capabilitypack.ObservationAvailable,
		Reason:           capabilitypack.ObservationReasonVerified,
		ObservedAt:       observedAt.Format(time.RFC3339),
		ObserverRevision: "vercel-codex-acceptance-v1",
	}
	for _, resource := range pack.Resources {
		for _, mode := range resource.RuntimeModes {
			evidence := capabilitypack.RuntimeEvidence{
				Requirements: make([]capabilitypack.RuntimeRequirementObservation, 0, len(mode.Requirements)),
				Authorities:  make([]capabilitypack.RuntimeAuthorityObservation, 0, len(mode.Authorities)),
			}
			for _, requirement := range mode.Requirements {
				evidence.Requirements = append(evidence.Requirements, capabilitypack.RuntimeRequirementObservation{
					Kind: requirement.Kind, ID: requirement.ID, RuntimeObservation: observation,
				})
			}
			for _, authority := range mode.Authorities {
				evidence.Authorities = append(evidence.Authorities, capabilitypack.RuntimeAuthorityObservation{
					Kind: authority.Kind, Scope: authority.Scope, RuntimeObservation: observation,
				})
			}
			records = append(records, capabilitypack.RuntimeModeEvidence{
				ResourceID: resource.ID, ModeID: mode.ID, Evidence: evidence,
			})
		}
	}
	return records
}
