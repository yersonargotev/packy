package setuphealth

import (
	"strings"
	"testing"
)

func TestHistoricalEvidenceAbsencePreservesActionableUpdate(t *testing.T) {
	report := Diagnose("/sandbox/home", "/sandbox/config", Observation{ActivePacks: []ActivePack{{ID: "older", Surface: "codex", UpdateAvailable: true, HistoricalEvidenceMessage: "Historical resource and contract evidence is unavailable"}}})
	if report.Summary.Failures != 0 || report.Summary.Warnings != 1 || report.Summary.Infos != 1 {
		t.Fatalf("unexpected health: %#v", report)
	}
	if !strings.Contains(report.Checks[1].Detail, "packy update older --surface codex") || !strings.Contains(report.Checks[2].Detail, "unavailable") {
		t.Fatalf("missing update/evidence: %#v", report.Checks)
	}
}

func TestRemovedSurfaceUpdateDoesNotRecommendImpossibleMutation(t *testing.T) {
	report := Diagnose("/sandbox/home", "/sandbox/config", Observation{ActivePacks: []ActivePack{{ID: "older", Surface: "claude", IntentVersion: "1.0.0", CatalogVersion: "1.0.1", UpdateAvailable: true, UpdateActionUnavailable: true, HistoricalEvidenceMessage: "Historical manifest is unavailable"}}})
	detail := report.Checks[1].Detail
	if report.Summary.Failures != 0 || report.Summary.Warnings != 1 || strings.Contains(detail, "packy update") || !strings.Contains(detail, "1.0.0 -> 1.0.1") || !strings.Contains(detail, "cannot be applied on this surface") || !strings.Contains(detail, "packy status older --surface claude") {
		t.Fatalf("misleading removed-surface diagnosis: %#v", report)
	}
}
