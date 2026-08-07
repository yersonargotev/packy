package setuphealth

import (
	"reflect"
	"strings"
	"testing"
)

func TestDiagnoseWithNoActivePacksIsHealthy(t *testing.T) {
	want := Report{
		SchemaVersion: 2,
		Kind:          "doctor",
		Context:       Context{HomeDir: "/sandbox/home", ConfigHome: "/sandbox/xdg"},
		Checks: []Check{{
			Name:     "packy-core",
			Severity: Pass,
			Detail:   "Packy core is available; no capability-pack activation is implied",
		}},
		Summary: Summary{Status: "healthy", Passes: 1},
	}
	if got := Diagnose("/sandbox/home", "/sandbox/xdg", Observation{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Diagnose() = %#v, want %#v", got, want)
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
			want:     []string{"inspection failed", "packy pack status missing --surface codex"},
		},
		{
			name:     "converged",
			pack:     ActivePack{ID: "ma" + "tty", Surface: "codex"},
			severity: Pass,
			status:   "healthy",
			want:     []string{"converged and ready"},
		},
		{
			name:     "drifted",
			pack:     ActivePack{ID: "ma" + "tty", Surface: "codex", ProjectionProblems: 2, ReadinessPending: true},
			severity: Warn,
			status:   "warnings",
			want:     []string{"2 projection findings", "packy pack activate ma" + "tty --surface codex", "packy pack status ma" + "tty --surface codex"},
		},
		{
			name:     "missing requirement",
			pack:     ActivePack{ID: "engram", Surface: "opencode", MissingRequirements: 1, ReadinessPending: true},
			severity: Warn,
			status:   "warnings",
			want:     []string{"1 missing requirements", "packy pack status engram --surface opencode"},
		},
		{
			name:     "pending human action",
			pack:     ActivePack{ID: "ma" + "tty", Surface: "claude", ReadinessPending: true, PendingHumanActions: 1},
			severity: Warn,
			status:   "warnings",
			want:     []string{"readiness is pending", "1 pending human actions", "packy pack status ma" + "tty --surface claude"},
		},
		{
			name:     "update available",
			pack:     ActivePack{ID: "ma" + "tty", Surface: "codex", UpdateAvailable: true},
			severity: Warn,
			status:   "warnings",
			want:     []string{"an update is available", "packy pack update ma" + "tty --surface codex"},
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
