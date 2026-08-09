// Package setuphealth owns read-only diagnosis of Packy core.
package setuphealth

import (
	"fmt"
	"strings"
)

type Severity string

const (
	Pass Severity = "PASS"
	Info Severity = "INFO"
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
	Infos    int
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
	ID                      string
	Surface                 string
	InspectionFailed        bool
	UpdateAvailable         bool
	ProjectionProblems      int
	MissingRequirements     int
	PendingHumanActions     int
	Conditions              []ReadinessCondition
	ControlledCheckState    string
	ControlledCheckResult   string
	ControlledCheckObserved string
	ControlledCheckIdentity string
}

// ReadinessCondition is the detached condition fact Doctor needs from a Pack
// status. Capability-pack remains the owner of its complete domain condition.
type ReadinessCondition struct {
	Type      string
	Dimension string
	Value     string
	Reason    string
	Message   string
}

type Observation struct {
	ActivePacks         []ActivePack
	FailedStateSurfaces []string
}

// Diagnose reports Packy core availability plus compact active-pack health.
// Inactive pack projections are intentionally absent.
func Diagnose(homeDir, configHome string, observation Observation) Report {
	report := Report{
		SchemaVersion: 3,
		Kind:          "doctor",
		Context:       Context{HomeDir: homeDir, ConfigHome: configHome},
		Checks: []Check{{
			Name:     "packy-core",
			Severity: Pass,
			Detail:   "Packy core is available; no capability-pack activation is implied",
		}},
	}
	for _, surface := range observation.FailedStateSurfaces {
		report.Checks = append(report.Checks, Check{
			Name:     fmt.Sprintf("pack-state-%s", surface),
			Severity: Fail,
			Detail:   fmt.Sprintf("capability-pack activation state for %s could not be observed; run packy status", surface),
		})
	}
	for _, pack := range observation.ActivePacks {
		report.Checks = append(report.Checks, diagnoseActivePack(pack)...)
	}
	report.Summary = summarize(report.Checks)
	return report
}

func diagnoseActivePack(pack ActivePack) []Check {
	name := fmt.Sprintf("pack-%s-%s", pack.ID, pack.Surface)
	statusCommand := fmt.Sprintf("packy status %s --surface %s", pack.ID, pack.Surface)
	var findings []string
	severity := Pass
	if pack.InspectionFailed {
		severity = Fail
		findings = append(findings, "inspection failed")
	}
	if pack.ProjectionProblems > 0 {
		if severity == Pass {
			severity = Warn
		}
		findings = append(findings, fmt.Sprintf("%d projection findings", pack.ProjectionProblems))
	}
	if pack.MissingRequirements > 0 {
		if severity == Pass {
			severity = Warn
		}
		findings = append(findings, fmt.Sprintf("%d missing requirements", pack.MissingRequirements))
	}
	if pack.UpdateAvailable {
		if severity == Pass {
			severity = Warn
		}
		findings = append(findings, "an update is available")
	}
	for _, condition := range pack.Conditions {
		if condition.Value != "false" {
			continue
		}
		if severity == Pass {
			severity = Warn
		}
		findings = append(findings, condition.Message)
	}
	if pack.PendingHumanActions > 0 {
		if severity == Pass {
			severity = Warn
		}
		findings = append(findings, fmt.Sprintf("%d pending human actions", pack.PendingHumanActions))
	}
	if len(findings) == 0 {
		checks := append(informationalConditions(name, pack.Conditions), controlledCheckInformation(name, pack)...)
		return append([]Check{{Name: name, Severity: Pass, Detail: fmt.Sprintf("active pack %s on %s has no confirmed health problems", pack.ID, pack.Surface)}}, checks...)
	}

	remediation := []string{statusCommand}
	if pack.ProjectionProblems > 0 {
		remediation = append([]string{fmt.Sprintf("packy activate %s --surface %s", pack.ID, pack.Surface)}, remediation...)
	}
	if pack.UpdateAvailable {
		remediation = append([]string{fmt.Sprintf("packy update %s --surface %s", pack.ID, pack.Surface)}, remediation...)
	}
	checks := append(informationalConditions(name, pack.Conditions), controlledCheckInformation(name, pack)...)
	return append([]Check{{
		Name:     name,
		Severity: severity,
		Detail:   fmt.Sprintf("active pack %s on %s has %s; run %s", pack.ID, pack.Surface, strings.Join(findings, ", "), strings.Join(remediation, "; then run ")),
	}}, checks...)
}

func controlledCheckInformation(packName string, pack ActivePack) []Check {
	if pack.ControlledCheckState != "current" && pack.ControlledCheckState != "stale" {
		return nil
	}
	return []Check{{Name: packName + "-controlled-runtime-check", Severity: Info, Detail: fmt.Sprintf("controlled runtime check state=%s result=%s observed_at=%s identity=%s", pack.ControlledCheckState, pack.ControlledCheckResult, pack.ControlledCheckObserved, pack.ControlledCheckIdentity)}}
}

func informationalConditions(packName string, conditions []ReadinessCondition) []Check {
	checks := make([]Check, 0, len(conditions))
	for _, condition := range conditions {
		if condition.Value != "unknown" {
			continue
		}
		checks = append(checks, Check{
			Name:     fmt.Sprintf("%s-%s-%s-%s", packName, condition.Dimension, condition.Type, condition.Reason),
			Severity: Info,
			Detail:   condition.Message,
		})
	}
	return checks
}

func summarize(checks []Check) Summary {
	summary := Summary{Status: "healthy"}
	for _, check := range checks {
		switch check.Severity {
		case Pass:
			summary.Passes++
		case Info:
			summary.Infos++
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
