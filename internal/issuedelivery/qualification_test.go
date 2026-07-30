package issuedelivery

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

func TestAdvancePersistsRejectedQualificationCorrectionAndIndependentRereview(t *testing.T) {
	module, _, _ := moduleFixture(t, 370)
	request := Request{RepositoryPath: "/repo", IssueNumber: 370}

	qualified, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	originalPath := filepath.Join(module.storePathForTest(t, 370), "runs", qualified.RunID+".json")
	originalBytes, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	matrixHash, err := acceptanceMatrixDigest(qualified.Evidence.AcceptanceMatrix)
	if err != nil {
		t.Fatal(err)
	}
	finding := deliveryevidence.ReviewFinding{
		ID: "qualification-product-seam", Axis: deliveryevidence.ReviewSpec,
		Severity: deliveryevidence.SeverityP1, Authority: deliveryevidence.AuthoritySpecRequirement,
		Citation: qualified.Evidence.Scope.OwnedNow[0].EvidenceLink,
		Location: qualified.Evidence.AcceptanceMatrix[0].Identity,
		Evidence: "the row names issuedelivery.Advance instead of the observable product seam",
	}
	review := &QualificationReview{
		AuthoritySHA256:        qualified.Observations.AuthoritySHA256,
		AcceptanceMatrixSHA256: matrixHash,
		Findings:               []deliveryevidence.ReviewFinding{finding}, Completed: true,
	}

	rejected, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 370,
		QualificationReview: review,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.State != StateNeedsDecision || rejected.QualificationCorrection == nil ||
		len(rejected.QualificationCorrection.FindingIDs) != 1 {
		t.Fatalf("rejected qualification = %#v", rejected)
	}
	resumed, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.State != StateNeedsDecision || resumed.QualificationCorrection == nil ||
		resumed.QualificationCorrection.ReviewedMatrixSHA256 != matrixHash {
		t.Fatalf("resumed rejected qualification = %#v", resumed)
	}
	revisions, err := os.ReadDir(filepath.Join(
		module.storePathForTest(t, 370), "revisions", qualified.RunID,
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 {
		t.Fatalf("qualification rejection revisions = %d, want 1", len(revisions))
	}
	replayedRejection, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 370, QualificationReview: review,
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayedRejection.State != StateNeedsDecision {
		t.Fatalf("replayed qualification rejection = %#v", replayedRejection)
	}
	assertQualificationRevisionCount(t, module, qualified.RunID, 1)
	gotOriginal, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotOriginal, originalBytes) {
		t.Fatal("qualification rejection rewrote the original run bytes")
	}

	correctedRows := append([]deliveryevidence.AcceptanceRow(nil), rejected.Evidence.AcceptanceMatrix...)
	correctedRows[0].OwningSeam = "internal/cli pack-show renderer"
	correctedRows[0].PositiveEvidence = "planned: pack-show human renderer ordering test"
	correctedRows[0].NegativeEvidence = "planned: compact summary omission fails the renderer test"
	correctedRows[0].FailureEvidence = "planned: invalid pack inspection preserves actionable failure output"
	correctedRows[0].MutationEvidence = "not applicable: rendering does not mutate state"
	correctedRows[0].CompatibilityEvidence = "planned: versioned JSON output remains byte-compatible"
	correctedRows[0].PreservationEvidence = "planned: detailed resource and surface contracts remain present"
	_, err = module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 370,
		QualificationCorrection: &QualificationCorrection{
			AuthoritySHA256:      rejected.Observations.AuthoritySHA256,
			ReviewedMatrixSHA256: strings.Repeat("0", 64),
			FindingIDs:           []string{finding.ID},
			AcceptanceMatrix:     correctedRows,
			Evidence:             "mismatched correction must fail closed",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched qualification correction error = %v", err)
	}
	correction := &QualificationCorrection{
		AuthoritySHA256:      rejected.Observations.AuthoritySHA256,
		ReviewedMatrixSHA256: matrixHash,
		FindingIDs:           []string{finding.ID},
		AcceptanceMatrix:     correctedRows,
		Evidence:             "mapped the criterion to its observable renderer and compatibility tests",
	}
	corrected, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 370, QualificationCorrection: correction,
	})
	if err != nil {
		t.Fatal(err)
	}
	if corrected.State != StateNeedsReview || corrected.QualificationCorrection != nil ||
		corrected.Evidence.AcceptanceMatrix[0].OwningSeam != "internal/cli pack-show renderer" {
		t.Fatalf("corrected qualification = %#v", corrected)
	}
	replayedCorrection, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 370, QualificationCorrection: correction,
	})
	if err != nil {
		t.Fatal(err)
	}
	if replayedCorrection.State != StateNeedsReview {
		t.Fatalf("replayed qualification correction = %#v", replayedCorrection)
	}
	assertQualificationRevisionCount(t, module, qualified.RunID, 2)
	correctedHash, err := acceptanceMatrixDigest(corrected.Evidence.AcceptanceMatrix)
	if err != nil {
		t.Fatal(err)
	}
	approval := &QualificationReview{
		AuthoritySHA256:        corrected.Observations.AuthoritySHA256,
		AcceptanceMatrixSHA256: correctedHash,
		Findings:               []deliveryevidence.ReviewFinding{}, Completed: true,
	}
	approved, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 370, QualificationReview: approval,
	})
	if err != nil {
		t.Fatal(err)
	}
	if approved.State != StateNeedsReview || !approved.QualificationApproved ||
		len(approved.QualificationReviews) != 2 || len(approved.QualificationCorrections) != 1 {
		t.Fatalf("approved qualification = %#v", approved)
	}
	replayedApproval, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 370, QualificationReview: approval,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !replayedApproval.QualificationApproved {
		t.Fatalf("replayed qualification approval = %#v", replayedApproval)
	}
	assertQualificationRevisionCount(t, module, qualified.RunID, 3)
	reloaded, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.QualificationApproved || len(reloaded.QualificationReviews) != 2 {
		t.Fatalf("reloaded approved qualification = %#v", reloaded)
	}
}

func TestAdvanceCompilesIssue347ProductSpecificQualificationEvidence(t *testing.T) {
	module, _, tracker := moduleFixture(t, 347)
	tracker.value.Title = "Make pack inspection and dry-run output easier to scan"
	tracker.value.Criteria = []AuthorityItem{
		{Text: "`packy pack show` presents a compact decision-oriented summary before detailed resource and surface contracts.", EvidenceLink: "issue#347:acceptance-1"},
		{Text: "Classic `--dry-run` output groups or summarizes repetitive actions before the complete action detail.", EvidenceLink: "issue#347:acceptance-2"},
		{Text: "Users can still obtain every planned action needed for safety and auditability.", EvidenceLink: "issue#347:acceptance-3"},
		{Text: "Existing versioned JSON schemas and redaction guarantees remain compatible.", EvidenceLink: "issue#347:acceptance-4"},
		{Text: "Human-output tests cover the new ordering and guidance.", EvidenceLink: "issue#347:acceptance-5"},
		{Text: "`./scripts/validate-packy.sh` passes.", EvidenceLink: "issue#347:acceptance-6"},
	}

	outcome, err := module.Advance(
		context.Background(), Request{RepositoryPath: "/repo", IssueNumber: 347},
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Candidate != nil || outcome.QualificationApproved {
		t.Fatalf("unqualified product criteria advanced: %#v", outcome)
	}
	matrixHash, err := acceptanceMatrixDigest(outcome.Evidence.AcceptanceMatrix)
	if err != nil {
		t.Fatal(err)
	}
	links := make(map[string]string, len(outcome.Evidence.Scope.OwnedNow))
	for _, entry := range outcome.Evidence.Scope.OwnedNow {
		links[entry.Identity] = entry.EvidenceLink
	}
	findings := make([]deliveryevidence.ReviewFinding, len(outcome.Evidence.AcceptanceMatrix))
	for index, row := range outcome.Evidence.AcceptanceMatrix {
		if row.OwningSeam != qualificationPlanRequired {
			t.Fatalf("unqualified criterion inferred a product seam: %#v", row)
		}
		findings[index] = deliveryevidence.ReviewFinding{
			ID:   "qualification-product-seam-" + row.Identity,
			Axis: deliveryevidence.ReviewSpec, Severity: deliveryevidence.SeverityP1,
			Authority: deliveryevidence.AuthoritySpecRequirement,
			Citation:  links[row.Identity], Location: row.Identity,
			Evidence: "the criterion requires an explicit observable product evidence plan",
		}
	}
	rejected := mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: 347,
		QualificationReview: &QualificationReview{
			AuthoritySHA256:        outcome.Observations.AuthoritySHA256,
			AcceptanceMatrixSHA256: matrixHash,
			Findings:               findings, Completed: true,
		},
	})
	correctedRows := append(
		[]deliveryevidence.AcceptanceRow(nil), rejected.Evidence.AcceptanceMatrix...,
	)
	plans := []*QualificationPlan{
		{
			OwningSeam:            "internal/cli pack-show human renderer",
			PositiveEvidence:      "planned: pack show summary ordering test",
			NegativeEvidence:      "planned: ordering regression test",
			FailureEvidence:       "planned: pack show actionable failure output",
			MutationEvidence:      "not applicable: pack show is read-only",
			CompatibilityEvidence: "planned: structured pack-show JSON compatibility",
			PreservationEvidence:  "planned: detailed resource contracts remain available",
		},
		{
			OwningSeam:            "internal/cli classic dry-run human renderer",
			PositiveEvidence:      "planned: dry-run groups actions before complete action detail",
			NegativeEvidence:      "planned: repetitive detail ordering regression test",
			FailureEvidence:       "planned: blocked-action guidance",
			MutationEvidence:      "planned: dry-run proves no lifecycle mutation",
			CompatibilityEvidence: "planned: structured dry-run JSON compatibility",
			PreservationEvidence:  "planned: complete action detail remains available",
		},
		{
			OwningSeam:            "internal/cli classic dry-run audit renderer",
			PositiveEvidence:      "planned: every action remains available for audit",
			NegativeEvidence:      "planned: omitted action fails preservation test",
			FailureEvidence:       "planned: incomplete audit detail blocks acceptance",
			MutationEvidence:      "planned: audit rendering is non-mutating",
			CompatibilityEvidence: "planned: action identity compatibility",
			PreservationEvidence:  "planned: complete action set is preserved",
		},
		{
			OwningSeam:            "versioned JSON and redaction contracts",
			PositiveEvidence:      "planned: JSON schema and redaction tests",
			NegativeEvidence:      "planned: sensitive value exposure fails tests",
			FailureEvidence:       "planned: incompatible JSON blocks acceptance",
			MutationEvidence:      "not applicable: serialization tests are observational",
			CompatibilityEvidence: "planned: versioned JSON remains compatible",
			PreservationEvidence:  "planned: redaction guarantees remain intact",
		},
		{
			OwningSeam:            "internal/cli human-output contract tests",
			PositiveEvidence:      "planned: human ordering and guidance tests",
			NegativeEvidence:      "planned: missing guidance fails tests",
			FailureEvidence:       "planned: blocked output remains actionable",
			MutationEvidence:      "not applicable: renderer tests do not mutate state",
			CompatibilityEvidence: "planned: JSON remains independent",
			PreservationEvidence:  "planned: detailed audit output remains available",
		},
		{
			OwningSeam:            "scripts/validate-packy.sh",
			PositiveEvidence:      "planned: exact validate-packy.sh success",
			NegativeEvidence:      "planned: candidate mismatch is rejected",
			FailureEvidence:       "planned: validator failure blocks readiness",
			MutationEvidence:      "not applicable: validation is read-only",
			CompatibilityEvidence: "planned: validator authority remains canonical",
			PreservationEvidence:  "planned: sandbox preserves operator configuration",
		},
	}
	plansByCriterion := make(map[string]*QualificationPlan, len(plans))
	for index, plan := range plans {
		plansByCriterion[tracker.value.Criteria[index].Text] = plan
	}
	for index := range correctedRows {
		plan := plansByCriterion[correctedRows[index].Criterion]
		correctedRows[index].OwningSeam = plan.OwningSeam
		correctedRows[index].PositiveEvidence = plan.PositiveEvidence
		correctedRows[index].NegativeEvidence = plan.NegativeEvidence
		correctedRows[index].FailureEvidence = plan.FailureEvidence
		correctedRows[index].MutationEvidence = plan.MutationEvidence
		correctedRows[index].CompatibilityEvidence = plan.CompatibilityEvidence
		correctedRows[index].PreservationEvidence = plan.PreservationEvidence
	}
	corrected := mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: 347,
		QualificationCorrection: &QualificationCorrection{
			AuthoritySHA256:      rejected.Observations.AuthoritySHA256,
			ReviewedMatrixSHA256: matrixHash,
			FindingIDs: []string{
				findings[0].ID, findings[1].ID, findings[2].ID,
				findings[3].ID, findings[4].ID, findings[5].ID,
			},
			AcceptanceMatrix: correctedRows,
			Evidence:         "mapped every criterion to its explicit product evidence seam",
		},
	})
	rows := make(map[string]deliveryevidence.AcceptanceRow)
	for _, row := range corrected.Evidence.AcceptanceMatrix {
		rows[row.Criterion] = row
	}
	assertQualificationRowContains(t, rows[tracker.value.Criteria[0].Text], "pack show", "ordering")
	assertQualificationRowContains(t, rows[tracker.value.Criteria[1].Text], "dry-run", "complete action")
	assertQualificationRowContains(t, rows[tracker.value.Criteria[2].Text], "dry-run", "audit")
	assertQualificationRowContains(t, rows[tracker.value.Criteria[3].Text], "JSON", "redaction")
	assertQualificationRowContains(t, rows[tracker.value.Criteria[4].Text], "human", "guidance")
	assertQualificationRowContains(t, rows[tracker.value.Criteria[5].Text], "validate-packy.sh", "exact")
}

func TestQualificationPlanDoesNotChangeApprovedAuthorityDigest(t *testing.T) {
	_, git, tracker := moduleFixture(t, 370)
	withoutPlan := tracker.value
	withoutPlan.Criteria = append([]AuthorityItem(nil), tracker.value.Criteria...)
	for index := range withoutPlan.Criteria {
		withoutPlan.Criteria[index].Plan = nil
	}
	plain, err := compileAuthority(
		git.value, withoutPlan, nil, nil, deliveryevidence.RiskStandard,
	)
	if err != nil {
		t.Fatal(err)
	}
	planned, err := compileAuthority(
		git.value, tracker.value, nil, nil, deliveryevidence.RiskStandard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if planned.hash != plain.hash {
		t.Fatalf("qualification plan changed authority digest: %s != %s", planned.hash, plain.hash)
	}
}

func TestCandidateInvalidationPreservesCorrectedQualificationEvidencePlan(t *testing.T) {
	module, _, _, _, _ := assuranceFixture(t)
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	qualified := mustAdvance(t, module, request)
	matrixHash, err := acceptanceMatrixDigest(qualified.Evidence.AcceptanceMatrix)
	if err != nil {
		t.Fatal(err)
	}
	finding := deliveryevidence.ReviewFinding{
		ID: "qualification-specific-plan", Axis: deliveryevidence.ReviewSpec,
		Severity: deliveryevidence.SeverityP1, Authority: deliveryevidence.AuthoritySpecRequirement,
		Citation: qualified.Evidence.Scope.OwnedNow[0].EvidenceLink,
		Location: qualified.Evidence.AcceptanceMatrix[0].Identity,
		Evidence: "the row requires a product-specific evidence plan",
	}
	rejected := mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: 357,
		QualificationReview: &QualificationReview{
			AuthoritySHA256:        qualified.Observations.AuthoritySHA256,
			AcceptanceMatrixSHA256: matrixHash,
			Findings:               []deliveryevidence.ReviewFinding{finding}, Completed: true,
		},
	})
	rows := append([]deliveryevidence.AcceptanceRow(nil), rejected.Evidence.AcceptanceMatrix...)
	rows[0].OwningSeam = "specific product seam"
	rows[0].PositiveEvidence = "planned: specific positive evidence"
	rows[0].NegativeEvidence = "planned: specific negative evidence"
	rows[0].FailureEvidence = "planned: specific failure evidence"
	rows[0].MutationEvidence = "planned: specific mutation evidence"
	rows[0].CompatibilityEvidence = "planned: specific compatibility evidence"
	rows[0].PreservationEvidence = "planned: specific preservation evidence"
	corrected := mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: 357,
		QualificationCorrection: &QualificationCorrection{
			AuthoritySHA256:      rejected.Observations.AuthoritySHA256,
			ReviewedMatrixSHA256: matrixHash,
			FindingIDs:           []string{finding.ID}, AcceptanceMatrix: rows,
			Evidence: "mapped the criterion to its specific product evidence",
		},
	})
	correctedHash, err := acceptanceMatrixDigest(corrected.Evidence.AcceptanceMatrix)
	if err != nil {
		t.Fatal(err)
	}
	mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: 357,
		QualificationReview: &QualificationReview{
			AuthoritySHA256:        corrected.Observations.AuthoritySHA256,
			AcceptanceMatrixSHA256: correctedHash,
			Findings:               []deliveryevidence.ReviewFinding{}, Completed: true,
		},
	})
	candidate := mustAdvance(t, module, request)
	if candidate.Candidate == nil ||
		candidate.Evidence.AcceptanceMatrix[0].PositiveEvidence !=
			"planned: specific positive evidence" {
		t.Fatalf("candidate invalidated corrected qualification plan: %#v", candidate)
	}
}

func assertQualificationRevisionCount(t *testing.T, module *Module, runID string, want int) {
	t.Helper()
	revisions, err := os.ReadDir(filepath.Join(
		module.storePathForTest(t, 370), "revisions", runID,
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != want {
		t.Fatalf("qualification revisions = %d, want %d", len(revisions), want)
	}
}

func assertQualificationRowContains(
	t *testing.T,
	row deliveryevidence.AcceptanceRow,
	values ...string,
) {
	t.Helper()
	compiled := strings.ToLower(strings.Join([]string{
		row.OwningSeam, row.PositiveEvidence, row.NegativeEvidence, row.FailureEvidence,
		row.MutationEvidence, row.CompatibilityEvidence, row.PreservationEvidence,
	}, " "))
	for _, value := range values {
		if !strings.Contains(compiled, strings.ToLower(value)) {
			t.Fatalf("qualification row %q does not contain %q: %#v", row.Criterion, value, row)
		}
	}
}
