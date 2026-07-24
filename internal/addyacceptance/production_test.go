package addyacceptance

import (
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
		Acceptance:       NewAcceptanceRunReport(ctx.Repository, ctx.EvaluatedMergeSHA, ctx.WorkflowDigest, ctx.RunID, rows),
		AcceptanceSHA256: strings.Repeat("1", 64), QualificationSHA256: strings.Repeat("2", 64),
		GovernanceSHA256: strings.Repeat("3", 64), WorkflowBlobSHA: strings.Repeat("4", 40),
		DisposableHarnessRoot: t.TempDir(),
		Qualification: ProductionQualification{
			Repository: ctx.Repository, Workflow: ctx.Workflow, WorkflowDigest: ctx.WorkflowDigest,
			RunID: ctx.RunID, Commit: ctx.EvaluatedMergeSHA, CollectedAt: ctx.Now.Add(-time.Minute),
			PackySHA: ctx.EvaluatedMergeSHA, PackyExecutableDigest: strings.Repeat("5", 64),
			RequestedClaudeVersion: "2.1.203", ResolvedClaudeVersion: "2.1.203",
			ClaudeIntegrity: "sha512-integrity", ClaudeDigest: strings.Repeat("6", 64), AtomicitySHA256: strings.Repeat("7", 64),
		},
		GovernanceEvaluation: governancedrift.Evaluation{
			Identity: governancedrift.EvidenceIdentity{Repository: ctx.Repository, Ref: "refs/heads/main", CommitSHA: ctx.EvaluatedMergeSHA, WorkflowSHA: strings.Repeat("4", 40), CollectedAt: ctx.Now.Add(-time.Minute)},
			State:    governancedrift.StateClean, Findings: []governancedrift.Finding{},
		},
		GovernanceDecision: governancedrift.GateDecision{Allowed: true, Reasons: []string{}},
	}
	if _, err := BuildProductionPromotionEvidence(ctx, input); err != nil {
		t.Fatal(err)
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
