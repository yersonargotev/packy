package audit

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/yersonargotev/packy/internal/setuphealth"
)

func TestAggregateScopesAndPreservesHealthSemantics(t *testing.T) {
	report := Aggregate(setuphealth.Report{Checks: []setuphealth.Check{
		{Name: "z-global", Scope: setuphealth.CheckScopeGlobal, Severity: setuphealth.Info, Detail: "unknown runtime"},
		{Name: "packy-core", Scope: setuphealth.CheckScopeWorkstation, Severity: setuphealth.Pass, Detail: "core available"},
		{Name: "a-global", Scope: setuphealth.CheckScopeGlobal, Severity: setuphealth.Warn, Detail: "drift"},
	}}, ProjectObservation{State: ProjectAbsent, Detail: "no project contract"})
	want := []Check{
		{Scope: Workstation, Severity: Pass, Code: "packy-core", Detail: "core available"},
		{Scope: Global, Severity: Warn, Code: "a-global", Detail: "drift"},
		{Scope: Global, Severity: Info, Code: "z-global", Detail: "unknown runtime"},
		{Scope: Project, Severity: Info, Code: "project-absent", Detail: "no project contract"},
	}
	if !reflect.DeepEqual(report.Checks, want) {
		t.Fatalf("checks = %#v, want %#v", report.Checks, want)
	}
	if report.Result != Warnings || report.Summary != (Summary{Passes: 1, Infos: 2, Warnings: 1}) {
		t.Fatalf("report = %#v", report)
	}
}

func TestAggregateFailedProjectFindingsAreFailures(t *testing.T) {
	report := Aggregate(setuphealth.Report{}, ProjectObservation{State: ProjectFailed, Summary: VerificationSummary{Packs: 1, Projections: 2, Verified: 1, Findings: 1}, Findings: []ProjectFinding{{Code: "projection_drift", Detail: "projection differs", Remediation: "run packy install"}}})
	if report.Result != Failures || len(report.Checks) != 2 {
		t.Fatalf("report = %#v", report)
	}
	if got := report.Checks[1]; got.Scope != Project || got.Severity != Fail || got.Code != "projection_drift" || got.Remediation != "run packy install" {
		t.Fatalf("check = %#v", got)
	}
	if report.Project.Verified != 1 {
		t.Fatalf("project report = %#v", report.Project)
	}
}

func TestAggregateIsJSONReadyAndDefaultsUnknownProjectToUnavailable(t *testing.T) {
	report := Aggregate(setuphealth.Report{}, ProjectObservation{})
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) == 0 || report.Project.State != ProjectUnavailable || report.Summary.Infos != 2 {
		t.Fatalf("report = %#v, json = %s", report, encoded)
	}
}
