package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/packy/internal/audit"
	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/setuphealth"
	"github.com/yersonargotev/packy/internal/workstation"
)

var ErrAuditFailures = errors.New("Packy audit found confirmed failures")

func newAuditCommand(opts Options, resolver *workstation.Resolver) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Create a shareable trust report for Packy and the current project",
		Long: "Audit Packy's workstation health, active global Packs, and the current project's portable Pack contract without changing state. " +
			"Warnings and unknown runtime observations remain reportable without failing automation; confirmed failures return a non-zero exit status.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var (
				health setuphealth.Report
				err    error
			)
			if opts.SetupHealthDiagnose != nil {
				health, err = opts.SetupHealthDiagnose()
			} else {
				health, err = diagnoseSetupHealth(cmd.Context(), opts, resolver)
			}
			if err != nil {
				health = setuphealth.Report{Checks: []setuphealth.Check{{
					Name: "packy-core-inspection", Scope: setuphealth.CheckScopeWorkstation, Severity: setuphealth.Fail,
					Detail: "Packy's workstation context could not be inspected; ensure HOME, XDG_CONFIG_HOME, and the current directory are readable, then rerun packy audit",
				}}}
			}
			project := observeAuditProject(cmd.Context(), resolver)
			report := audit.Aggregate(health, project)
			if jsonOutput {
				err = json.NewEncoder(cmd.OutOrStdout()).Encode(report)
			} else {
				err = renderAudit(cmd.OutOrStdout(), report)
			}
			if err != nil {
				return err
			}
			if report.Result == audit.Failures {
				return ErrAuditFailures
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable versioned JSON")
	return cmd
}

func observeAuditProject(ctx context.Context, resolver *workstation.Resolver) audit.ProjectObservation {
	snapshot, err := resolver.Resolve(workstation.Options{})
	if err != nil {
		return audit.ProjectObservation{State: audit.ProjectUnavailable, Detail: "the current project context is unavailable"}
	}
	currentDirectory, err := snapshot.CurrentDirectory()
	if err != nil {
		return failedAuditProject("project-context-inspection-failed", "the current directory could not be inspected", "restore current-directory access, then rerun packy audit")
	}
	projectRoot, err := capabilitypack.DiscoverProjectRoot(currentDirectory)
	if err != nil {
		var absent capabilitypack.ProjectNotFoundError
		if errors.As(err, &absent) {
			return audit.ProjectObservation{State: audit.ProjectUnavailable, Detail: "the current directory is outside a Git worktree"}
		}
		return failedAuditProject("project-discovery-failed", "the current Git worktree could not be inspected", "repair the Git worktree, then rerun packy audit")
	}
	present, err := capabilitypack.HasProjectContract(projectRoot)
	if err != nil {
		return failedAuditProject("project-contract-inspection-failed", "the current project's Pack contract could not be inspected", "restore access to the project root, then rerun packy audit")
	}
	if !present {
		return audit.ProjectObservation{State: audit.ProjectAbsent, Detail: "the current Git project has no Pack contract"}
	}
	verification := capabilitypack.VerifyProject(ctx, projectRoot, map[capabilitypack.Surface]capabilitypack.SurfaceAdapter{
		capabilitypack.SurfaceClaude:   projectOfflineAdapter(capabilitypack.SurfaceClaude),
		capabilitypack.SurfaceCodex:    projectOfflineAdapter(capabilitypack.SurfaceCodex),
		capabilitypack.SurfaceOpenCode: projectOfflineAdapter(capabilitypack.SurfaceOpenCode),
	})
	observation := audit.ProjectObservation{Summary: audit.VerificationSummary{
		Packs: verification.Summary.Packs, Surfaces: verification.Summary.Surfaces,
		Projections: verification.Summary.Projections, Verified: verification.Summary.Verified, Findings: verification.Summary.Findings,
	}}
	if verification.Result == capabilitypack.ProjectVerificationPassed {
		observation.State = audit.ProjectVerified
		observation.Detail = fmt.Sprintf("the current project Pack contract is verified (%d/%d projections)", verification.Summary.Verified, verification.Summary.Projections)
		return observation
	}
	observation.State = audit.ProjectFailed
	for _, finding := range verification.Findings {
		observation.Findings = append(observation.Findings, auditProjectFinding(finding))
	}
	for _, entry := range verification.Entries {
		for _, finding := range entry.Findings {
			converted := auditProjectFinding(finding)
			converted.Detail = fmt.Sprintf("%s %s on %s: %s", entry.Pack.ID, entry.Pack.Version, entry.Surface, converted.Detail)
			observation.Findings = append(observation.Findings, converted)
		}
	}
	observation.Detail = "the current project Pack contract failed portable verification"
	return observation
}

func failedAuditProject(code, detail, remediation string) audit.ProjectObservation {
	return audit.ProjectObservation{State: audit.ProjectFailed, Detail: detail, Summary: audit.VerificationSummary{Findings: 1}, Findings: []audit.ProjectFinding{{Code: code, Detail: detail, Remediation: remediation}}}
}

func auditProjectFinding(finding capabilitypack.ProjectVerificationFinding) audit.ProjectFinding {
	context := make([]string, 0, 2)
	if finding.Resource != nil {
		context = append(context, finding.Resource.String())
	}
	if finding.Target != "" {
		context = append(context, finding.Target)
	}
	detail := finding.Detail
	if len(context) > 0 {
		detail = strings.Join(context, " · ") + ": " + detail
	}
	return audit.ProjectFinding{Code: finding.Code, Detail: detail, Remediation: finding.Remediation}
}

func renderAudit(output io.Writer, report audit.Report) error {
	if _, err := fmt.Fprintf(output, "Packy audit: %s\n", report.Result); err != nil {
		return err
	}
	currentScope := audit.Scope("")
	for _, check := range report.Checks {
		if check.Scope != currentScope {
			currentScope = check.Scope
			if _, err := fmt.Fprintf(output, "\n%s\n", auditScopeTitle(currentScope)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(output, "[%s] %s: %s\n", check.Severity, check.Code, check.Detail); err != nil {
			return err
		}
		if check.Remediation != "" {
			if _, err := fmt.Fprintf(output, "  Remediation: %s\n", check.Remediation); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(output, "\nSummary: passes=%d infos=%d warnings=%d failures=%d\nProject: %s; packs=%d surfaces=%d projections=%d/%d verified findings=%d\n",
		report.Summary.Passes, report.Summary.Infos, report.Summary.Warnings, report.Summary.Failures,
		report.Project.State, report.Project.Packs, report.Project.Surfaces, report.Project.Verified, report.Project.Projections, report.Project.Findings)
	return err
}

func auditScopeTitle(scope audit.Scope) string {
	switch scope {
	case audit.Workstation:
		return "Workstation"
	case audit.Global:
		return "Active global Packs"
	default:
		return "Current project"
	}
}
