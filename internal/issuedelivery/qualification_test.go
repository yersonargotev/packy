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

	rejected, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 370,
		QualificationReview: &QualificationReview{
			AuthoritySHA256:        qualified.Observations.AuthoritySHA256,
			AcceptanceMatrixSHA256: matrixHash,
			Findings:               []deliveryevidence.ReviewFinding{finding}, Completed: true,
		},
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
	corrected, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 370,
		QualificationCorrection: &QualificationCorrection{
			AuthoritySHA256:      rejected.Observations.AuthoritySHA256,
			ReviewedMatrixSHA256: matrixHash,
			FindingIDs:           []string{finding.ID},
			AcceptanceMatrix:     correctedRows,
			Evidence:             "mapped the criterion to its observable renderer and compatibility tests",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if corrected.State != StateNeedsReview || corrected.QualificationCorrection != nil ||
		corrected.Evidence.AcceptanceMatrix[0].OwningSeam != "internal/cli pack-show renderer" {
		t.Fatalf("corrected qualification = %#v", corrected)
	}
	correctedHash, err := acceptanceMatrixDigest(corrected.Evidence.AcceptanceMatrix)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 370,
		QualificationReview: &QualificationReview{
			AuthoritySHA256:        corrected.Observations.AuthoritySHA256,
			AcceptanceMatrixSHA256: correctedHash,
			Findings:               []deliveryevidence.ReviewFinding{}, Completed: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if approved.State != StateNeedsReview || !approved.QualificationApproved ||
		len(approved.QualificationReviews) != 2 || len(approved.QualificationCorrections) != 1 {
		t.Fatalf("approved qualification = %#v", approved)
	}
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
	rows := make(map[string]deliveryevidence.AcceptanceRow)
	for _, row := range outcome.Evidence.AcceptanceMatrix {
		rows[row.Criterion] = row
		if row.OwningSeam == "issuedelivery.Advance" ||
			strings.Contains(row.PositiveEvidence, "behavior through Advance") {
			t.Fatalf("product criterion compiled to generic delivery evidence: %#v", row)
		}
	}
	assertQualificationRowContains(t, rows[tracker.value.Criteria[0].Text], "pack show", "ordering")
	assertQualificationRowContains(t, rows[tracker.value.Criteria[1].Text], "dry-run", "complete action")
	assertQualificationRowContains(t, rows[tracker.value.Criteria[2].Text], "dry-run", "audit")
	assertQualificationRowContains(t, rows[tracker.value.Criteria[3].Text], "JSON", "redaction")
	assertQualificationRowContains(t, rows[tracker.value.Criteria[4].Text], "human", "guidance")
	assertQualificationRowContains(t, rows[tracker.value.Criteria[5].Text], "validate-packy.sh", "exact")
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

func TestQualificationHistoryAdoptsOnlyExactLegacyGenericInvalidation(t *testing.T) {
	corrected := []deliveryevidence.AcceptanceRow{{
		Identity: "criterion-1", Criterion: "Product behavior.",
		OwningSeam: "product seam", PositiveEvidence: "specific positive",
		NegativeEvidence: "specific negative", FailureEvidence: "specific failure",
		MutationEvidence: "specific mutation", CompatibilityEvidence: "specific compatibility",
		PreservationEvidence: "specific preservation", MigrationEvidence: "not applicable",
		State: deliveryevidence.AcceptancePlanned,
	}}
	digest, err := acceptanceMatrixDigest(corrected)
	if err != nil {
		t.Fatal(err)
	}
	legacy := append([]deliveryevidence.AcceptanceRow(nil), corrected...)
	legacy[0].PositiveEvidence = "planned: focused positive behavior through Advance"
	legacy[0].NegativeEvidence = "planned: focused negative behavior through Advance"
	legacy[0].FailureEvidence = "planned: failure behavior through Advance"
	legacy[0].MutationEvidence = "planned: persisted run mutation inspection"
	legacy[0].CompatibilityEvidence = "planned: compatibility validation"
	legacy[0].PreservationEvidence = "planned: prior run byte preservation"
	record := runRecord{
		Evidence:                 &deliveryevidence.Bundle{AcceptanceMatrix: legacy},
		Candidates:               []Candidate{{ID: "candidate"}},
		QualificationCorrections: []QualificationCorrection{{AcceptanceMatrix: corrected}},
	}
	review := QualificationReview{AcceptanceMatrixSHA256: digest}
	if !qualificationApprovalMatchesCurrentPlan(record, "different", review) {
		t.Fatal("exact legacy generic invalidation was not adopted")
	}
	record.Evidence.AcceptanceMatrix[0].PositiveEvidence = "other generic evidence"
	if qualificationApprovalMatchesCurrentPlan(record, "different", review) {
		t.Fatal("non-canonical invalidation was adopted")
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
