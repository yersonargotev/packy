package capabilitypack

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEvaluateRuntimeModesTriStateStaleFallbackAndDeterminism(t *testing.T) {
	pack := runtimeTestPack()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	records := []RuntimeModeEvidence{
		runtimeTestEvidence("skill", "fallback", nil, nil),
		runtimeTestEvidence("skill", "primary",
			[]RuntimeRequirementObservation{runtimeRequirementObservation(ObservationAvailable, now)},
			[]RuntimeAuthorityObservation{runtimeAuthorityObservation(ObservationUnverified, ObservationReasonObserverError, now)}),
		runtimeTestEvidence("skill", "unavailable",
			[]RuntimeRequirementObservation{runtimeRequirementObservation(ObservationUnavailable, now)},
			[]RuntimeAuthorityObservation{runtimeAuthorityObservation(ObservationUnverified, ObservationReasonObserverError, now)}),
	}

	first, err := EvaluateRuntimeModes(pack, records, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EvaluateRuntimeModes(pack, records, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("evaluation changed across identical rerun")
	}
	if got := []RuntimeModeState{first[0].State, first[1].State, first[2].State}; !reflect.DeepEqual(got, []RuntimeModeState{RuntimeModeAvailable, RuntimeModeUnverified, RuntimeModeUnavailable}) {
		t.Fatalf("states = %v", got)
	}
	if first[1].FallbackState == nil || *first[1].FallbackState != RuntimeModeAvailable {
		t.Fatalf("fallback truth = %#v", first[1].FallbackState)
	}
	if _, err := PreflightRuntimeMode(pack, "skill", "primary", records, now, time.Hour); err == nil {
		t.Fatal("unverified primary must fail before effects rather than select fallback")
	} else {
		var preflight RuntimePreflightError
		if !errors.As(err, &preflight) || preflight.ModeID != "primary" {
			t.Fatalf("preflight error = %T %v", err, err)
		}
	}

	staleRecords := append([]RuntimeModeEvidence(nil), records...)
	staleRecords[1] = runtimeTestEvidence("skill", "primary",
		[]RuntimeRequirementObservation{runtimeRequirementObservation(ObservationAvailable, now.Add(-2*time.Hour))},
		[]RuntimeAuthorityObservation{runtimeAuthorityObservation(ObservationAvailable, ObservationReasonVerified, now)})
	stale, err := EvaluateRuntimeModes(pack, staleRecords, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if stale[1].State != RuntimeModeUnverified || stale[1].Evidence.Requirements[0].Reason != ObservationReasonStale {
		t.Fatalf("stale result = %#v", stale[1])
	}

	futureRecords := append([]RuntimeModeEvidence(nil), records...)
	futureRecords[1] = runtimeTestEvidence("skill", "primary",
		[]RuntimeRequirementObservation{runtimeRequirementObservation(ObservationAvailable, now.Add(time.Minute))},
		[]RuntimeAuthorityObservation{runtimeAuthorityObservation(ObservationAvailable, ObservationReasonVerified, now)})
	future, err := EvaluateRuntimeModes(pack, futureRecords, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if future[1].State != RuntimeModeUnverified || future[1].Evidence.Requirements[0].Reason != ObservationReasonStale {
		t.Fatalf("future-dated result = %#v", future[1])
	}
}

func TestEvaluateRuntimeModesRequiresExactCoverageAndSecretSafeDiagnostics(t *testing.T) {
	pack := runtimeTestPack()
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	valid := []RuntimeModeEvidence{
		runtimeTestEvidence("skill", "fallback", nil, nil),
		runtimeTestEvidence("skill", "primary",
			[]RuntimeRequirementObservation{runtimeRequirementObservation(ObservationAvailable, now)},
			[]RuntimeAuthorityObservation{runtimeAuthorityObservation(ObservationAvailable, ObservationReasonVerified, now)}),
		runtimeTestEvidence("skill", "unavailable",
			[]RuntimeRequirementObservation{runtimeRequirementObservation(ObservationAvailable, now)},
			[]RuntimeAuthorityObservation{runtimeAuthorityObservation(ObservationAvailable, ObservationReasonVerified, now)}),
	}
	cases := map[string][]RuntimeModeEvidence{
		"missing":   valid[:2],
		"duplicate": append(append([]RuntimeModeEvidence(nil), valid...), valid[0]),
		"extra":     append(append([]RuntimeModeEvidence(nil), valid...), runtimeTestEvidence("skill", "extra", nil, nil)),
	}
	mismatch := append([]RuntimeModeEvidence(nil), valid...)
	mismatch[1].Evidence.Requirements[0].ID = "other"
	cases["mismatch"] = mismatch
	for name, records := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := EvaluateRuntimeModes(pack, records, now, time.Hour)
			if err == nil {
				t.Fatal("expected rejection")
			}
			if strings.Contains(err.Error(), "top-secret") {
				t.Fatalf("diagnostic exposed secret: %v", err)
			}
		})
	}
}

func runtimeTestPack() Pack {
	requirement := RuntimeRequirement{Kind: RuntimeRequirementAuthentication, ID: "service"}
	authority := RuntimeAuthority{Kind: RuntimeAuthoritySecretUse, Scope: RuntimeScopeVercelAccount}
	return Pack{Resources: []Resource{{ID: "skill", RuntimeModes: []RuntimeMode{
		{ID: "fallback", Role: RuntimeModeFallbackOnly, Requirements: []RuntimeRequirement{}, Authorities: []RuntimeAuthority{}, Effects: []RuntimeEffect{}, Fallback: RuntimeFallback{Kind: RuntimeFallbackNone}, OnUnavailable: RuntimeFailBeforeEffects},
		{ID: "primary", Role: RuntimeModePrimary, Requirements: []RuntimeRequirement{requirement}, Authorities: []RuntimeAuthority{authority}, Effects: []RuntimeEffect{{Kind: RuntimeEffectUpload, Scope: RuntimeScopeDeploymentPayload}}, Fallback: RuntimeFallback{Kind: RuntimeFallbackMode, Mode: "fallback"}, OnUnavailable: RuntimeFailBeforeEffects},
		{ID: "unavailable", Role: RuntimeModePrimary, Requirements: []RuntimeRequirement{requirement}, Authorities: []RuntimeAuthority{authority}, Effects: []RuntimeEffect{}, Fallback: RuntimeFallback{Kind: RuntimeFallbackNone}, OnUnavailable: RuntimeFailBeforeEffects},
	}}}}
}

func runtimeTestEvidence(resourceID, modeID string, requirements []RuntimeRequirementObservation, authorities []RuntimeAuthorityObservation) RuntimeModeEvidence {
	if requirements == nil {
		requirements = []RuntimeRequirementObservation{}
	}
	if authorities == nil {
		authorities = []RuntimeAuthorityObservation{}
	}
	return RuntimeModeEvidence{ResourceID: resourceID, ModeID: modeID, Evidence: RuntimeEvidence{Requirements: requirements, Authorities: authorities}}
}

func runtimeRequirementObservation(state ObservationState, observedAt time.Time) RuntimeRequirementObservation {
	reason := ObservationReasonVerified
	if state == ObservationUnavailable {
		reason = ObservationReasonNotFound
	}
	return RuntimeRequirementObservation{
		Kind: RuntimeRequirementAuthentication, ID: "service",
		RuntimeObservation: RuntimeObservation{State: state, Reason: reason, ObservedAt: observedAt.Format(time.RFC3339), ObserverRevision: "observer-v1", RedactedIdentity: "top-secret"},
	}
}

func runtimeAuthorityObservation(state ObservationState, reason ObservationReason, observedAt time.Time) RuntimeAuthorityObservation {
	return RuntimeAuthorityObservation{
		Kind: RuntimeAuthoritySecretUse, Scope: RuntimeScopeVercelAccount,
		RuntimeObservation: RuntimeObservation{State: state, Reason: reason, ObservedAt: observedAt.Format(time.RFC3339), ObserverRevision: "observer-v1", RedactedIdentity: "top-secret"},
	}
}
