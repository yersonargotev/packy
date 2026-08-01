// Package setuphealth owns read-only diagnosis of Packy core.
package setuphealth

import (
	"fmt"
	"strings"
)

type Severity string

const (
	Pass Severity = "PASS"
	Warn Severity = "WARN"
	Fail Severity = "FAIL"
)

type Check struct {
	Name     string
	Severity Severity
	Detail   string
}

type Summary struct {
	Status   string
	Passes   int
	Warnings int
	Failures int
}

type Context struct {
	HomeDir    string
	ConfigHome string
}

type Report struct {
	SchemaVersion int
	Kind          string
	Context       Context
	Checks        []Check
	Summary       Summary
}

// ActivePack is the compact, detached status input Doctor needs. Detailed
// readiness evidence remains owned by capability-pack status.
type ActivePack struct {
	ID                  string
	Surface             string
	InspectionFailed    bool
	RecoveryRequired    bool
	UpdateAvailable     bool
	ProjectionProblems  int
	MissingRequirements int
	ReadinessPending    bool
	PendingHumanActions int
}

// Diagnose reports Packy core availability plus compact active-pack health.
// Removed classic state and inactive pack projections are intentionally absent.
func Diagnose(homeDir, configHome string, activePacks ...ActivePack) Report {
	report := Report{
		SchemaVersion: 2,
		Kind:          "doctor",
		Context:       Context{HomeDir: homeDir, ConfigHome: configHome},
		Checks: []Check{{
			Name:     "packy-core",
			Severity: Pass,
			Detail:   "Packy core is available; no capability-pack activation is implied",
		}},
	}
	for _, pack := range activePacks {
		report.Checks = append(report.Checks, diagnoseActivePack(pack))
	}
	report.Summary = summarize(report.Checks)
	return report
}

func diagnoseActivePack(pack ActivePack) Check {
	name := fmt.Sprintf("pack-%s-%s", pack.ID, pack.Surface)
	statusCommand := fmt.Sprintf("packy pack status %s --surface %s", pack.ID, pack.Surface)
	var findings []string
	severity := Pass
	if pack.InspectionFailed {
		severity = Fail
		findings = append(findings, "inspection failed")
	}
	if pack.RecoveryRequired {
		severity = Fail
		findings = append(findings, "recovery is required")
	}
	if pack.ProjectionProblems > 0 {
		severity = Fail
		findings = append(findings, fmt.Sprintf("%d projection findings", pack.ProjectionProblems))
	}
	if pack.MissingRequirements > 0 {
		severity = Fail
		findings = append(findings, fmt.Sprintf("%d missing requirements", pack.MissingRequirements))
	}
	if pack.UpdateAvailable {
		if severity == Pass {
			severity = Warn
		}
		findings = append(findings, "an update is available")
	}
	if pack.ReadinessPending {
		if severity == Pass {
			severity = Warn
		}
		findings = append(findings, "readiness is pending")
	}
	if pack.PendingHumanActions > 0 {
		if severity == Pass {
			severity = Warn
		}
		findings = append(findings, fmt.Sprintf("%d pending human actions", pack.PendingHumanActions))
	}
	if len(findings) == 0 {
		return Check{Name: name, Severity: Pass, Detail: fmt.Sprintf("active pack %s on %s is converged and ready", pack.ID, pack.Surface)}
	}

	remediation := []string{statusCommand}
	if pack.ProjectionProblems > 0 {
		remediation = append([]string{fmt.Sprintf("packy pack reconcile %s --surface %s", pack.ID, pack.Surface)}, remediation...)
	}
	if pack.UpdateAvailable {
		remediation = append([]string{fmt.Sprintf("packy pack update %s --surface %s", pack.ID, pack.Surface)}, remediation...)
	}
	return Check{
		Name:     name,
		Severity: severity,
		Detail:   fmt.Sprintf("active pack %s on %s has %s; run %s", pack.ID, pack.Surface, strings.Join(findings, ", "), strings.Join(remediation, "; then run ")),
	}
}

func summarize(checks []Check) Summary {
	summary := Summary{Status: "healthy"}
	for _, check := range checks {
		switch check.Severity {
		case Pass:
			summary.Passes++
		case Warn:
			summary.Warnings++
		case Fail:
			summary.Failures++
		}
	}
	if summary.Failures > 0 {
		summary.Status = "failures"
	} else if summary.Warnings > 0 {
		summary.Status = "warnings"
	}
	return summary
}
