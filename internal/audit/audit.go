// Package audit assembles a portable, machine-readable view of Packy's
// workstation, global, and current-project health.
package audit

import (
	"sort"

	"github.com/yersonargotev/packy/internal/setuphealth"
)

const (
	SchemaVersion = 1
	ReportName    = "packy-audit"
)

type Result string

const (
	Healthy  Result = "healthy"
	Warnings Result = "warnings"
	Failures Result = "failures"
)

type Scope string

const (
	Workstation Scope = "workstation"
	Global      Scope = "global"
	Project     Scope = "project"
)

type Severity string

const (
	Pass Severity = "PASS"
	Info Severity = "INFO"
	Warn Severity = "WARN"
	Fail Severity = "FAIL"
)

type Check struct {
	Scope       Scope    `json:"scope"`
	Severity    Severity `json:"severity"`
	Code        string   `json:"code"`
	Detail      string   `json:"detail"`
	Remediation string   `json:"remediation,omitempty"`
}

type Summary struct {
	Passes   int `json:"passes"`
	Infos    int `json:"infos"`
	Warnings int `json:"warnings"`
	Failures int `json:"failures"`
}

type ProjectState string

const (
	ProjectUnavailable ProjectState = "unavailable"
	ProjectAbsent      ProjectState = "absent"
	ProjectVerified    ProjectState = "verified"
	ProjectFailed      ProjectState = "failed"
)

// VerificationSummary intentionally mirrors the portable counts emitted by
// capabilitypack's project verifier without making audit depend on that
// package's report shape.
type VerificationSummary struct {
	Packs       int `json:"packs"`
	Surfaces    int `json:"surfaces"`
	Projections int `json:"projections"`
	Verified    int `json:"verified"`
	Findings    int `json:"findings"`
}

type ProjectFinding struct {
	Code        string `json:"code"`
	Detail      string `json:"detail"`
	Remediation string `json:"remediation,omitempty"`
}

// ProjectObservation is the project-owned input to Aggregate. State is
// explicit so an absent or unavailable project is never inferred from empty
// verification counts.
type ProjectObservation struct {
	State    ProjectState        `json:"state"`
	Summary  VerificationSummary `json:"summary"`
	Findings []ProjectFinding    `json:"findings,omitempty"`
	Detail   string              `json:"detail,omitempty"`
}

type ProjectReport struct {
	State       ProjectState `json:"state"`
	Packs       int          `json:"packs"`
	Surfaces    int          `json:"surfaces"`
	Projections int          `json:"projections"`
	Verified    int          `json:"verified"`
	Findings    int          `json:"findings"`
}

type Report struct {
	SchemaVersion int           `json:"schema_version"`
	Report        string        `json:"report"`
	Result        Result        `json:"result"`
	Checks        []Check       `json:"checks"`
	Project       ProjectReport `json:"project"`
	Summary       Summary       `json:"summary"`
}

// Aggregate converts setuphealth and a project observation into the audit
// contract. It does not reinterpret unknown readiness information: setuphealth
// INFO checks remain INFO checks.
func Aggregate(health setuphealth.Report, observation ProjectObservation) Report {
	checks := make([]Check, 0, len(health.Checks)+len(observation.Findings)+1)
	globalChecks := 0
	for _, check := range health.Checks {
		scope := Global
		if check.Scope == setuphealth.CheckScopeWorkstation {
			scope = Workstation
		} else {
			globalChecks++
		}
		checks = append(checks, Check{Scope: scope, Severity: Severity(check.Severity), Code: check.Name, Detail: check.Detail})
	}
	if globalChecks == 0 {
		checks = append(checks, Check{Scope: Global, Severity: Info, Code: "active-global-packs-absent", Detail: "no active global Packs were observed"})
	}
	projectDetail := observation.Detail
	if projectDetail == "" {
		projectDetail = "project state is " + string(observation.State)
	}
	switch observation.State {
	case ProjectUnavailable, ProjectAbsent:
		checks = append(checks, Check{Scope: Project, Severity: Info, Code: "project-" + string(observation.State), Detail: projectDetail})
	case ProjectVerified:
		checks = append(checks, Check{Scope: Project, Severity: Pass, Code: "project-verified", Detail: projectDetail})
	case ProjectFailed:
		for _, finding := range observation.Findings {
			checks = append(checks, Check{Scope: Project, Severity: Fail, Code: finding.Code, Detail: finding.Detail, Remediation: finding.Remediation})
		}
		if len(observation.Findings) == 0 {
			checks = append(checks, Check{Scope: Project, Severity: Fail, Code: "project-verification-failed", Detail: projectDetail})
		}
	default:
		checks = append(checks, Check{Scope: Project, Severity: Info, Code: "project-unavailable", Detail: "project state is unavailable"})
		observation.State = ProjectUnavailable
	}
	sort.SliceStable(checks, func(i, j int) bool {
		if checks[i].Scope != checks[j].Scope {
			return scopeRank(checks[i].Scope) < scopeRank(checks[j].Scope)
		}
		if checks[i].Code != checks[j].Code {
			return checks[i].Code < checks[j].Code
		}
		if checks[i].Detail != checks[j].Detail {
			return checks[i].Detail < checks[j].Detail
		}
		if checks[i].Severity != checks[j].Severity {
			return checks[i].Severity < checks[j].Severity
		}
		return checks[i].Remediation < checks[j].Remediation
	})
	summary := summarize(checks)
	result := Healthy
	if summary.Failures > 0 {
		result = Failures
	} else if summary.Warnings > 0 {
		result = Warnings
	}
	return Report{SchemaVersion: SchemaVersion, Report: ReportName, Result: result, Checks: checks,
		Project: ProjectReport{State: observation.State, Packs: observation.Summary.Packs, Surfaces: observation.Summary.Surfaces,
			Projections: observation.Summary.Projections, Verified: observation.Summary.Verified, Findings: observation.Summary.Findings}, Summary: summary}
}

func scopeRank(scope Scope) int {
	switch scope {
	case Workstation:
		return 0
	case Global:
		return 1
	case Project:
		return 2
	}
	return 3
}

func summarize(checks []Check) Summary {
	var result Summary
	for _, check := range checks {
		switch check.Severity {
		case Pass:
			result.Passes++
		case Info:
			result.Infos++
		case Warn:
			result.Warnings++
		case Fail:
			result.Failures++
		}
	}
	return result
}
