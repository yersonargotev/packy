package addyacceptance

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/governancedrift"
)

func TestAcceptanceRunReportRequiresExactRegisteredProductionRows(t *testing.T) {
	ctx := harnessContext()
	ctx.PromotionChange = true
	ctx.PullRequest = 7
	ctx.EvaluatedMergeSHA = strings.Repeat("d", 40)
	rows := make([]AcceptanceRunRow, len(PromotionRows()))
	for i, row := range PromotionRows() {
		rows[i] = AcceptanceRunRow{
			ID: row.ID, Package: "./internal/addyacceptance", OwningTest: row.OwningTest,
			Result: PromotionPassed, EvidenceSHA256: strings.Repeat("a", 64),
		}
	}
	valid := NewAcceptanceRunReport(ctx.Repository, ctx.EvaluatedMergeSHA, ctx.WorkflowDigest, ctx.RunID, rows)
	if err := valid.Validate(ctx); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*AcceptanceRunReport)
	}{
		{"synthetic", func(r *AcceptanceRunReport) { r.Synthetic = true }},
		{"missing", func(r *AcceptanceRunReport) { r.Rows = r.Rows[:13] }},
		{"duplicate", func(r *AcceptanceRunReport) { r.Rows[1] = r.Rows[0] }},
		{"unknown", func(r *AcceptanceRunReport) { r.Rows[0].ID = "unknown" }},
		{"bad-owner", func(r *AcceptanceRunReport) { r.Rows[0].OwningTest = "TestOther" }},
		{"bad-package", func(r *AcceptanceRunReport) { r.Rows[0].Package = "./internal/ci" }},
		{"bad-result", func(r *AcceptanceRunReport) { r.Rows[0].Result = "failed" }},
		{"bad-digest", func(r *AcceptanceRunReport) { r.Rows[0].EvidenceSHA256 = "bad" }},
		{"cross-repository", func(r *AcceptanceRunReport) { r.Repository = "other/repo" }},
		{"cross-run", func(r *AcceptanceRunReport) { r.RunID = "other" }},
		{"cross-commit", func(r *AcceptanceRunReport) { r.CommitSHA = strings.Repeat("e", 40) }},
		{"cross-workflow", func(r *AcceptanceRunReport) { r.WorkflowDigest = strings.Repeat("f", 64) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := valid
			got.Rows = append([]AcceptanceRunRow(nil), valid.Rows...)
			test.mutate(&got)
			if err := got.Validate(ctx); err == nil {
				t.Fatal("invalid acceptance report admitted")
			}
		})
	}
}

func TestBuildProductionPromotionEvidenceRejectsQualificationAndGovernanceTwins(t *testing.T) {
	ctx := harnessContext()
	ctx.PromotionChange = true
	ctx.PullRequest = 7
	ctx.EvaluatedMergeSHA = strings.Repeat("d", 40)
	rows := make([]AcceptanceRunRow, len(PromotionRows()))
	for i, row := range PromotionRows() {
		rows[i] = AcceptanceRunRow{ID: row.ID, Package: "./internal/addyacceptance", OwningTest: row.OwningTest, Result: PromotionPassed, EvidenceSHA256: strings.Repeat("a", 64)}
	}
	input := ProductionPromotionInputs{
		Acceptance:            NewAcceptanceRunReport(ctx.Repository, ctx.EvaluatedMergeSHA, ctx.WorkflowDigest, ctx.RunID, rows),
		WorkflowBlobSHA:       strings.Repeat("4", 40),
		DisposableHarnessRoot: t.TempDir(),
		Qualification: ProductionQualification{
			Repository: ctx.Repository, Workflow: ctx.Workflow, WorkflowDigest: ctx.WorkflowDigest,
			RunID: ctx.RunID, Commit: ctx.EvaluatedMergeSHA, CollectedAt: ctx.Now.Add(-time.Minute),
			PackySHA: ctx.EvaluatedMergeSHA, PackyExecutableDigest: strings.Repeat("5", 64),
			RequestedClaudeVersion: "2.1.203", ResolvedClaudeVersion: "2.1.203",
			ClaudeIntegrity: "sha512-integrity", ClaudeDigest: strings.Repeat("6", 64),
			Sandbox: "/sandbox",
		},
		GovernanceEvaluation: governancedrift.Evaluation{
			Identity: governancedrift.EvidenceIdentity{Repository: ctx.Repository, Ref: "refs/heads/main", CommitSHA: ctx.EvaluatedMergeSHA, WorkflowSHA: strings.Repeat("4", 40), CollectedAt: ctx.Now.Add(-time.Minute)},
			State:    governancedrift.StateClean, Findings: []governancedrift.Finding{},
		},
		GovernanceDecision: governancedrift.GateDecision{Allowed: true, Reasons: []string{}},
	}
	input.Qualification.Atomicity = validProductionAtomicity(ctx.EvaluatedMergeSHA, input.Qualification.CollectedAt)
	baseline, err := BuildProductionPromotionEvidence(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	changedAcceptance := input
	changedAcceptance.DisposableHarnessRoot = t.TempDir()
	changedAcceptance.Acceptance.Rows = append([]AcceptanceRunRow(nil), input.Acceptance.Rows...)
	changedAcceptance.Acceptance.Rows[0].EvidenceSHA256 = strings.Repeat("8", 64)
	acceptanceEvidence, err := BuildProductionPromotionEvidence(ctx, changedAcceptance)
	if err != nil || acceptanceEvidence.Rows[0].EvidenceSHA256 == baseline.Rows[0].EvidenceSHA256 {
		t.Fatal("changing admitted acceptance evidence did not change its derived row authority")
	}
	changedQualification := input
	changedQualification.DisposableHarnessRoot = t.TempDir()
	changedQualification.Qualification.ClaudeIntegrity = "sha512-other"
	qualificationEvidence, err := BuildProductionPromotionEvidence(ctx, changedQualification)
	if err != nil || qualificationEvidence.Rows[10].EvidenceSHA256 == baseline.Rows[10].EvidenceSHA256 {
		t.Fatal("changing admitted qualification did not change its derived authority")
	}
	changedGovernance := input
	changedGovernance.DisposableHarnessRoot = t.TempDir()
	changedGovernance.GovernanceEvaluation.Identity.CollectedAt = ctx.Now.Add(-2 * time.Minute)
	governanceEvidence, err := BuildProductionPromotionEvidence(ctx, changedGovernance)
	if err != nil || governanceEvidence.Rows[12].EvidenceSHA256 == baseline.Rows[12].EvidenceSHA256 {
		t.Fatal("changing admitted governance evidence did not change its derived authority")
	}
	tests := []struct {
		name   string
		mutate func(*ProductionPromotionInputs)
	}{
		{"synthetic", func(in *ProductionPromotionInputs) { in.Qualification.Synthetic = true }},
		{"stale-qualification", func(in *ProductionPromotionInputs) { in.Qualification.CollectedAt = ctx.Now.Add(-25 * time.Hour) }},
		{"cross-run", func(in *ProductionPromotionInputs) { in.Qualification.RunID = "other" }},
		{"cross-commit", func(in *ProductionPromotionInputs) { in.Qualification.Commit = strings.Repeat("e", 40) }},
		{"wrong-floor", func(in *ProductionPromotionInputs) { in.Qualification.ResolvedClaudeVersion = "2.2.0" }},
		{"missing-commands", func(in *ProductionPromotionInputs) { in.Qualification.Atomicity.Commands = nil }},
		{"missing-manifest", func(in *ProductionPromotionInputs) { in.Qualification.Atomicity.Before = nil }},
		{"directory-with-digest", func(in *ProductionPromotionInputs) {
			in.Qualification.Atomicity.Before = append([]ProductionFile(nil), in.Qualification.Atomicity.Before...)
			in.Qualification.Atomicity.Before[1].SHA256 = strings.Repeat("a", 64)
		}},
		{"regular-without-digest", func(in *ProductionPromotionInputs) {
			in.Qualification.Atomicity.Before = append([]ProductionFile(nil), in.Qualification.Atomicity.Before...)
			in.Qualification.Atomicity.Before[0].SHA256 = ""
		}},
		{"unsupported-file-type", func(in *ProductionPromotionInputs) {
			in.Qualification.Atomicity.Before = append([]ProductionFile(nil), in.Qualification.Atomicity.Before...)
			in.Qualification.Atomicity.Before[1].Mode = uint32(os.ModeDevice | 0o600)
		}},
		{"false-runner-safety", func(in *ProductionPromotionInputs) { in.Qualification.Atomicity.Safety.DisposableSandbox = false }},
		{"false-observed-safety", func(in *ProductionPromotionInputs) { in.Qualification.Atomicity.Observation.Safety.NoGoRun = false }},
		{"malformed-process-digest", func(in *ProductionPromotionInputs) {
			in.Qualification.Atomicity.Observation.ProcessLogDigest = strings.Repeat("0", 64)
		}},
		{"stale-governance", func(in *ProductionPromotionInputs) {
			in.GovernanceEvaluation.Identity.CollectedAt = ctx.Now.Add(-2 * time.Hour)
		}},
		{"future-governance", func(in *ProductionPromotionInputs) {
			in.GovernanceEvaluation.Identity.CollectedAt = ctx.Now.Add(time.Minute)
		}},
		{"dirty-governance", func(in *ProductionPromotionInputs) {
			in.GovernanceEvaluation.State = governancedrift.StateConfirmedDrift
		}},
		{"disallowed-governance", func(in *ProductionPromotionInputs) {
			in.GovernanceDecision = governancedrift.GateDecision{Allowed: false, Reasons: []string{"blocked"}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := input
			got.DisposableHarnessRoot = t.TempDir()
			test.mutate(&got)
			if _, err := BuildProductionPromotionEvidence(ctx, got); err == nil {
				t.Fatal("invalid production input admitted")
			}
		})
	}
}

func validProductionAtomicity(commit string, collectedAt time.Time) ProductionAtomicity {
	operations := []struct {
		name string
		args []string
	}{
		{"claude", []string{"--version"}}, {"packy", []string{"version"}},
		{"packy", []string{"init", "--home", "/sandbox/home", "--source-root", "/sandbox/installed-source", "--repository-url", "/sandbox/source-repository", "--repository-ref", "packy-smoke-proved-source"}},
		{"packy", []string{"install", "--dry-run"}}, {"packy", []string{"install"}}, {"packy", []string{"doctor"}},
		{"packy", []string{"update", "--dry-run"}}, {"packy", []string{"update"}},
		{"packy", []string{"uninstall", "--dry-run"}}, {"packy", []string{"uninstall"}}, {"packy", []string{"doctor"}},
		{"claude", []string{"version"}}, {"claude", []string{"version"}}, {"claude", []string{"version"}},
		{"claude", []string{"mcp-add"}}, {"claude", []string{"version"}}, {"claude", []string{"version"}},
		{"claude", []string{"version"}}, {"claude", []string{"version"}}, {"claude", []string{"version"}},
		{"claude", []string{"mcp-remove"}}, {"claude", []string{"version"}},
	}
	commands := make([]ProductionCommand, len(operations))
	for i, operation := range operations {
		commands[i] = ProductionCommand{Name: operation.name, Args: operation.args}
	}
	process, _ := json.Marshal(commands)
	allRunnerSafety := ProductionRunnerSafety{true, true, true, true, true, true, true, true, true}
	allAssertions := ProductionAssertions{true, true, true, true, true, true, true, true, true, true, true, true, true}
	allObservedSafety := ProductionObservedSafety{true, true, true, true, true, true, true, true, true, true, true}
	return ProductionAtomicity{
		Commands: commands,
		Before: []ProductionFile{
			{Path: "before", SHA256: strings.Repeat("a", 64), Mode: 0o600, Size: 1},
			{Path: "directory", Mode: uint32(os.ModeDir | 0o700), Size: 0},
			{Path: "symlink", Mode: uint32(os.ModeSymlink | 0o777), Size: 6},
		},
		After: []ProductionFile{
			{Path: "after", SHA256: strings.Repeat("b", 64), Mode: 0o600, Size: 1},
			{Path: "directory", Mode: uint32(os.ModeDir | 0o700), Size: 0},
			{Path: "symlink", Mode: uint32(os.ModeSymlink | 0o777), Size: 6},
		},
		Safety: allRunnerSafety, Assertions: allAssertions,
		Observation: ProductionObservation{
			InstalledSource: "/sandbox/source", InstalledSourceCommit: commit, InstalledSourceClean: true,
			WritableRoots:    ProductionWritableRoots{"/sandbox/home", "/sandbox/config", "/sandbox/claude", "/sandbox/state", "/sandbox/package", "/sandbox/repository", "/sandbox/acquisition"},
			ProcessLogDigest: digestBytes(process), CollectedAt: collectedAt, Safety: allObservedSafety,
		},
	}
}
