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
