package setuphealth

import (
	"reflect"
	"strings"
	"testing"
)

func TestDiagnoseWithNoActivePacksIsHealthy(t *testing.T) {
	want := Report{
		SchemaVersion: 3,
		Kind:          "doctor",
		Context:       Context{HomeDir: "/sandbox/home", ConfigHome: "/sandbox/xdg"},
		Checks: []Check{{
			Name:     "packy-core",
			Scope:    CheckScopeWorkstation,
			Severity: Pass,
			Detail:   "Packy core is available; no capability-pack activation is implied",
		}},
		Summary: Summary{Status: "healthy", Passes: 1},
	}
	if got := Diagnose("/sandbox/home", "/sandbox/xdg", Observation{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Diagnose() = %#v, want %#v", got, want)
	}
}

func TestDiagnoseReportsUnknownConditionsAsInformational(t *testing.T) {
	report := Diagnose("/sandbox/home", "/sandbox/xdg", Observation{ActivePacks: []ActivePack{{
		ID: "matty", Surface: "codex",
		Conditions: []ReadinessCondition{{
			Type: "runtime-usability", Dimension: "usable", Value: "unknown", Reason: "runtime-unobservable",
			Message: "runtime usability cannot be observed",
		}},
	}}})
	if report.Summary != (Summary{Status: "healthy", Passes: 2, Infos: 1}) {
		t.Fatalf("summary = %+v", report.Summary)
	}
	if got := report.Checks[2]; got.Severity != Info || !strings.Contains(got.Name, "runtime-unobservable") || got.Detail != "runtime usability cannot be observed" {
		t.Fatalf("informational condition = %+v", got)
	}
}

func TestDiagnoseReportsFalseConditionsAsWarnings(t *testing.T) {
	report := Diagnose("/sandbox/home", "/sandbox/xdg", Observation{ActivePacks: []ActivePack{{
		ID: "matty", Surface: "codex",
		Conditions: []ReadinessCondition{{
			Type: "surface-authorization", Dimension: "authorized", Value: "false", Reason: "authorization-denied",
			Message: "surface authorization was denied",
		}},
	}}})
	if report.Summary.Status != "warnings" || report.Summary.Warnings != 1 || report.Checks[1].Severity != Warn || !strings.Contains(report.Checks[1].Detail, "surface authorization was denied") {
		t.Fatalf("report = %+v", report)
	}
}

func TestDiagnoseExposesControlledRuntimeCheckEvidence(t *testing.T) {
	report := Diagnose("/sandbox/home", "/sandbox/xdg", Observation{ActivePacks: []ActivePack{{
		ID: "orchestrate", Surface: "codex", ControlledCheckState: "stale", ControlledCheckResult: "true",
		ControlledCheckObserved: "2026-08-09T00:00:00Z", ControlledCheckIdentity: "check-identity",
	}}})
	if report.Summary.Infos != 1 || len(report.Checks) != 3 {
		t.Fatalf("report = %+v", report)
	}
	if detail := report.Checks[2].Detail; !strings.Contains(detail, "state=stale") || !strings.Contains(detail, "result=true") || !strings.Contains(detail, "identity=check-identity") {
		t.Fatalf("controlled check detail = %q", detail)
	}
}

func TestDiagnoseSummarizesActivePackHealth(t *testing.T) {
	tests := []struct {
		name     string
		pack     ActivePack
		severity Severity
		status   string
		want     []string
	}{
		{
			name:     "inspection failed",
			pack:     ActivePack{ID: "missing", Surface: "codex", InspectionFailed: true},
			severity: Fail,
			status:   "failures",
			want:     []string{"inspection failed", "packy status missing --surface codex"},
		},
		{
			name:     "converged",
			pack:     ActivePack{ID: "ma" + "tty", Surface: "codex"},
			severity: Pass,
			status:   "healthy",
			want:     []string{"no confirmed health problems"},
		},
		{
			name:     "drifted",
			pack:     ActivePack{ID: "ma" + "tty", Surface: "codex", ProjectionProblems: 2},
			severity: Warn,
			status:   "warnings",
			want:     []string{"2 projection findings", "packy activate ma" + "tty --surface codex", "packy status ma" + "tty --surface codex"},
		},
		{
			name:     "missing requirement",
			pack:     ActivePack{ID: "engram", Surface: "opencode", MissingRequirements: 1},
			severity: Warn,
			status:   "warnings",
			want:     []string{"1 missing requirements", "packy status engram --surface opencode"},
		},
		{
			name:     "pending human action",
			pack:     ActivePack{ID: "ma" + "tty", Surface: "claude", PendingHumanActions: 1},
			severity: Warn,
			status:   "warnings",
			want:     []string{"1 pending human actions", "packy status ma" + "tty --surface claude"},
		},
		{
			name:     "update available",
			pack:     ActivePack{ID: "ma" + "tty", Surface: "codex", UpdateAvailable: true},
			severity: Warn,
			status:   "warnings",
			want:     []string{"an update is available", "packy update ma" + "tty --surface codex"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := Diagnose("/sandbox/home", "/sandbox/xdg", Observation{ActivePacks: []ActivePack{tc.pack}})
			if len(report.Checks) != 2 || report.Checks[1].Severity != tc.severity || report.Summary.Status != tc.status {
				t.Fatalf("report = %+v", report)
			}
			for _, want := range tc.want {
				if !strings.Contains(report.Checks[1].Detail, want) {
					t.Fatalf("detail %q missing %q", report.Checks[1].Detail, want)
				}
			}
		})
	}
}

func TestDiagnoseWithNoActivePacksReportsOnlyCoreHealth(t *testing.T) {
	report := Diagnose("/sandbox/home", "/sandbox/xdg", Observation{})
	if len(report.Checks) != 1 || report.Checks[0].Name != "packy-core" || report.Summary != (Summary{Status: "healthy", Passes: 1}) {
		t.Fatalf("report = %+v", report)
	}
}

func TestDiagnoseIncludesStateObservationFailuresAndContinues(t *testing.T) {
	report := Diagnose("/sandbox/home", "/sandbox/xdg", Observation{
		FailedStateSurfaces: []string{"codex"},
		ActivePacks:         []ActivePack{{ID: "engram", Surface: "opencode"}},
	})
	if len(report.Checks) != 3 || report.Checks[0].Name != "packy-core" || report.Checks[1].Name != "pack-state-codex" || report.Checks[1].Severity != Fail || report.Checks[2].Severity != Pass {
		t.Fatalf("report = %+v", report)
	}
	if report.Summary != (Summary{Status: "failures", Passes: 2, Failures: 1}) {
		t.Fatalf("summary = %+v", report.Summary)
	}
}
