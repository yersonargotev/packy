package capabilitypack

import (
	"context"
	"path/filepath"
	"strings"
)

const ProjectVerificationSchemaVersion = 1

type ProjectVerificationResult string

const (
	ProjectVerificationPassed ProjectVerificationResult = "passed"
	ProjectVerificationFailed ProjectVerificationResult = "failed"
)

type ProjectVerificationEntry struct {
	Pack         ProjectVerificationPack      `json:"pack"`
	Surface      Surface                      `json:"surface"`
	Installation ProjectInstallationState     `json:"installation"`
	Projections  int                          `json:"projections"`
	Verified     int                          `json:"verified"`
	Findings     []ProjectVerificationFinding `json:"findings"`
}

type ProjectVerificationPack struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type ProjectVerificationFinding struct {
	Code        string            `json:"code"`
	Resource    *ResourceIdentity `json:"resource,omitempty"`
	Target      string            `json:"target,omitempty"`
	Detail      string            `json:"detail"`
	Remediation string            `json:"remediation"`
}

type ProjectVerificationSummary struct {
	Packs       int `json:"packs"`
	Surfaces    int `json:"surfaces"`
	Projections int `json:"projections"`
	Verified    int `json:"verified"`
	Findings    int `json:"findings"`
}

type ProjectVerificationReport struct {
	SchemaVersion int                          `json:"schema_version"`
	Report        string                       `json:"report"`
	ProjectRoot   string                       `json:"project_root"`
	Result        ProjectVerificationResult    `json:"result"`
	Summary       ProjectVerificationSummary   `json:"summary"`
	Entries       []ProjectVerificationEntry   `json:"entries"`
	Findings      []ProjectVerificationFinding `json:"findings"`
}

// VerifyProject checks only the committed project contract and its projections.
// It deliberately excludes personal activation, executable discovery, runtime
// readiness, and controlled-check evidence so the result is portable in CI.
func VerifyProject(ctx context.Context, projectRoot string, adapters map[Surface]SurfaceAdapter) ProjectVerificationReport {
	report := ProjectVerificationReport{
		SchemaVersion: ProjectVerificationSchemaVersion,
		Report:        "project-verification",
		ProjectRoot:   "<project-root>",
		Result:        ProjectVerificationPassed,
		Entries:       []ProjectVerificationEntry{},
		Findings:      []ProjectVerificationFinding{},
	}
	status, err := InspectProjectStatus(ctx, ProjectStatusRequest{ProjectRoot: projectRoot, Adapters: adapters, InspectionScope: ProjectInspectionScopeContract})
	if err != nil {
		report.Result = ProjectVerificationFailed
		report.Findings = append(report.Findings, ProjectVerificationFinding{
			Code: "project_contract_invalid", Detail: portableProjectError(err, projectRoot),
			Remediation: "repair or reinstall the project Pack contract, then rerun packy verify",
		})
		report.Summary.Findings = 1
		return report
	}
	if len(status.Packs) == 0 {
		report.Result = ProjectVerificationFailed
		report.Findings = append(report.Findings, ProjectVerificationFinding{
			Code: "project_contract_absent", Detail: "the project has no installed Pack contract",
			Remediation: "run packy install for a Pack and surface, then commit packy.json, packy.lock.json, notices, and projections",
		})
		report.Summary.Findings = 1
		return report
	}

	packIDs := map[string]bool{}
	for _, statusEntry := range status.Packs {
		entry := ProjectVerificationEntry{
			Pack:    ProjectVerificationPack{ID: statusEntry.Pack.ID, Version: statusEntry.Pack.Version},
			Surface: statusEntry.Surface, Installation: statusEntry.Installation,
			Projections: len(statusEntry.Projections), Findings: verificationFindings(statusEntry.Blockers),
		}
		for _, projection := range statusEntry.Projections {
			if projection.Health == "verified" {
				entry.Verified++
			}
		}
		if statusEntry.Installation != ProjectInstallationInstalled && len(entry.Findings) == 0 {
			entry.Findings = append(entry.Findings, ProjectVerificationFinding{
				Code:        "project_installation_" + string(statusEntry.Installation),
				Detail:      "the installed Pack contract is " + string(statusEntry.Installation),
				Remediation: "run packy install " + statusEntry.Pack.ID + " --surface " + string(statusEntry.Surface) + " to restore the exact contract",
			})
		}
		if statusEntry.Installation != ProjectInstallationInstalled || len(entry.Findings) > 0 || entry.Verified != entry.Projections {
			report.Result = ProjectVerificationFailed
		}
		packIDs[statusEntry.Pack.ID] = true
		report.Summary.Surfaces++
		report.Summary.Projections += entry.Projections
		report.Summary.Verified += entry.Verified
		report.Summary.Findings += len(entry.Findings)
		report.Entries = append(report.Entries, entry)
	}
	report.Summary.Packs = len(packIDs)
	return report
}

func portableProjectError(err error, projectRoot string) string {
	detail := err.Error()
	for _, root := range []string{projectRoot, filepath.Clean(projectRoot)} {
		if root != "" && root != "." {
			detail = strings.ReplaceAll(detail, root, "<project-root>")
		}
	}
	return detail
}

func verificationFindings(blockers []ProjectInstallBlocker) []ProjectVerificationFinding {
	findings := make([]ProjectVerificationFinding, 0, len(blockers))
	for _, blocker := range blockers {
		finding := ProjectVerificationFinding{Code: blocker.Code, Target: blocker.Target, Detail: blocker.Detail, Remediation: blocker.Remediation}
		if blocker.Resource.Kind != "" || blocker.Resource.ID != "" {
			resource := blocker.Resource
			finding.Resource = &resource
		}
		findings = append(findings, finding)
	}
	return findings
}
