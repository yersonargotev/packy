package issuedelivery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

type QualificationReview struct {
	AuthoritySHA256        string                           `json:"authority_sha256"`
	AcceptanceMatrixSHA256 string                           `json:"acceptance_matrix_sha256"`
	Findings               []deliveryevidence.ReviewFinding `json:"findings"`
	Completed              bool                             `json:"completed"`
}

type QualificationCorrectionRequest struct {
	AuthoritySHA256      string   `json:"authority_sha256"`
	ReviewedMatrixSHA256 string   `json:"reviewed_matrix_sha256"`
	FindingIDs           []string `json:"finding_ids"`
}

type QualificationCorrection struct {
	AuthoritySHA256      string                           `json:"authority_sha256"`
	ReviewedMatrixSHA256 string                           `json:"reviewed_matrix_sha256"`
	FindingIDs           []string                         `json:"finding_ids"`
	AcceptanceMatrix     []deliveryevidence.AcceptanceRow `json:"acceptance_matrix"`
	Evidence             string                           `json:"evidence"`
}

func (m *Module) advanceQualification(
	store lockedIssueStore,
	record runRecord,
	request Request,
) (Outcome, bool, error) {
	review, correction := request.QualificationReview, request.QualificationCorrection
	if review != nil && correction != nil {
		return Outcome{}, true, errors.New("one Advance call cannot review and correct qualification")
	}
	if record.PendingQualificationCorrection != nil {
		if correction == nil {
			if review != nil {
				return Outcome{}, true, errors.New("qualification correction is required before independent rereview")
			}
			return outcomeFromRecord(record), true, nil
		}
		if err := validateQualificationCorrection(record, *correction); err != nil {
			return Outcome{}, true, err
		}
		nextEvidence := *record.Evidence
		nextEvidence.AcceptanceMatrix = append(
			[]deliveryevidence.AcceptanceRow(nil), correction.AcceptanceMatrix...,
		)
		if err := deliveryevidence.Validate(nextEvidence); err != nil {
			return Outcome{}, true, fmt.Errorf("corrected qualification evidence is invalid: %w", err)
		}
		stored := *correction
		stored.FindingIDs = append([]string(nil), correction.FindingIDs...)
		stored.AcceptanceMatrix = append(
			[]deliveryevidence.AcceptanceRow(nil), correction.AcceptanceMatrix...,
		)
		record.Evidence = &nextEvidence
		record.QualificationCorrections = append(record.QualificationCorrections, stored)
		record.PendingQualificationCorrection = nil
		record.QualificationApproved = false
		outcome, err := m.persistAssuranceTransition(
			store, record, StateNeedsReview,
			"corrected qualification evidence is ready for independent rereview",
			"qualification-correction",
		)
		return outcome, true, err
	}
	if correction != nil {
		return Outcome{}, true, errors.New("qualification correction was not requested")
	}
	if review == nil {
		return Outcome{}, false, nil
	}
	if record.QualificationApproved {
		return Outcome{}, true, errors.New("qualification is already independently approved")
	}
	if err := validateQualificationReview(record, *review); err != nil {
		return Outcome{}, true, err
	}
	stored := *review
	stored.Findings = append([]deliveryevidence.ReviewFinding{}, review.Findings...)
	record.QualificationReviews = append(record.QualificationReviews, stored)
	if len(stored.Findings) == 0 {
		record.QualificationApproved = true
		outcome, err := m.persistAssuranceTransition(
			store, record, StateNeedsReview,
			"qualification evidence passed independent review",
			"qualification-review",
		)
		return outcome, true, err
	}
	findingIDs := make([]string, 0, len(stored.Findings))
	for _, finding := range stored.Findings {
		findingIDs = append(findingIDs, finding.ID)
	}
	sort.Strings(findingIDs)
	record.PendingQualificationCorrection = &QualificationCorrectionRequest{
		AuthoritySHA256:      record.AuthoritySHA256,
		ReviewedMatrixSHA256: stored.AcceptanceMatrixSHA256,
		FindingIDs:           findingIDs,
	}
	record.QualificationApproved = false
	outcome, err := m.persistAssuranceTransition(
		store, record, StateNeedsDecision,
		"qualification review findings require one persisted correction",
		"qualification-review",
	)
	return outcome, true, err
}

func validateQualificationReview(record runRecord, review QualificationReview) error {
	if record.Evidence == nil || !review.Completed || review.Findings == nil ||
		review.AuthoritySHA256 != record.AuthoritySHA256 {
		return errors.New("qualification review does not match the completed authority")
	}
	digest, err := acceptanceMatrixDigest(record.Evidence.AcceptanceMatrix)
	if err != nil {
		return err
	}
	if review.AcceptanceMatrixSHA256 != digest {
		return errors.New("qualification review does not match the current acceptance matrix")
	}
	links := make(map[string]string, len(record.Evidence.Scope.OwnedNow))
	for _, entry := range record.Evidence.Scope.OwnedNow {
		links[entry.Identity] = entry.EvidenceLink
	}
	seen := make(map[string]bool, len(review.Findings))
	for _, finding := range review.Findings {
		if !validQualificationFinding(finding) || seen[finding.ID] ||
			links[finding.Location] == "" || finding.Citation != links[finding.Location] ||
			strings.TrimSpace(finding.Evidence) == "" {
			return errors.New("qualification review contains an invalid or untraceable finding")
		}
		seen[finding.ID] = true
	}
	return nil
}

func validateQualificationHistory(record runRecord) error {
	if len(record.QualificationReviews) == 0 {
		if record.QualificationApproved || len(record.QualificationCorrections) > 0 ||
			record.PendingQualificationCorrection != nil {
			return errors.New("qualification state requires a persisted independent review")
		}
		return nil
	}
	if record.Evidence == nil || len(record.QualificationCorrections) > len(record.QualificationReviews) {
		return errors.New("qualification history is incomplete")
	}
	links := make(map[string]string, len(record.Evidence.Scope.OwnedNow))
	for _, entry := range record.Evidence.Scope.OwnedNow {
		links[entry.Identity] = entry.EvidenceLink
	}
	for _, review := range record.QualificationReviews {
		if review.AuthoritySHA256 != record.AuthoritySHA256 ||
			!runIDPattern.MatchString(review.AcceptanceMatrixSHA256) ||
			!review.Completed {
			return errors.New("qualification history contains an invalid review")
		}
		seen := make(map[string]bool, len(review.Findings))
		for _, finding := range review.Findings {
			if !validQualificationFinding(finding) || seen[finding.ID] ||
				links[finding.Location] == "" || finding.Citation != links[finding.Location] {
				return errors.New("qualification history contains an invalid finding")
			}
			seen[finding.ID] = true
		}
	}
	for index, correction := range record.QualificationCorrections {
		review := record.QualificationReviews[index]
		if correction.AuthoritySHA256 != record.AuthoritySHA256 ||
			correction.ReviewedMatrixSHA256 != review.AcceptanceMatrixSHA256 ||
			len(review.Findings) == 0 || strings.TrimSpace(correction.Evidence) == "" ||
			len(correction.FindingIDs) != len(review.Findings) {
			return errors.New("qualification history contains an invalid correction")
		}
		want := make([]string, 0, len(review.Findings))
		for _, finding := range review.Findings {
			want = append(want, finding.ID)
		}
		got := append([]string(nil), correction.FindingIDs...)
		sort.Strings(want)
		sort.Strings(got)
		for item := range want {
			if got[item] != want[item] {
				return errors.New("qualification correction does not cover its review findings")
			}
		}
	}
	currentDigest, err := acceptanceMatrixDigest(record.Evidence.AcceptanceMatrix)
	if err != nil {
		return err
	}
	lastReview := record.QualificationReviews[len(record.QualificationReviews)-1]
	if record.PendingQualificationCorrection != nil {
		pending := record.PendingQualificationCorrection
		if len(lastReview.Findings) == 0 ||
			len(record.QualificationReviews) != len(record.QualificationCorrections)+1 ||
			pending.AuthoritySHA256 != record.AuthoritySHA256 ||
			pending.ReviewedMatrixSHA256 != currentDigest ||
			pending.ReviewedMatrixSHA256 != lastReview.AcceptanceMatrixSHA256 {
			return errors.New("pending qualification correction does not match its rejected review")
		}
	}
	if record.QualificationApproved {
		if len(lastReview.Findings) != 0 ||
			!qualificationApprovalMatchesCurrentPlan(record, currentDigest, lastReview) ||
			len(record.QualificationReviews) != len(record.QualificationCorrections)+1 {
			return errors.New("qualification approval does not match the current matrix")
		}
	} else if record.PendingQualificationCorrection == nil &&
		len(record.QualificationReviews) != len(record.QualificationCorrections) {
		return errors.New("qualification history is not ready for independent rereview")
	}
	return nil
}

func qualificationApprovalMatchesCurrentPlan(
	record runRecord,
	currentDigest string,
	lastReview QualificationReview,
) bool {
	if lastReview.AcceptanceMatrixSHA256 == currentDigest {
		return true
	}
	if len(record.Candidates) == 0 || len(record.QualificationCorrections) == 0 ||
		!hasLegacyGenericInvalidation(record.Evidence.AcceptanceMatrix) {
		return false
	}
	corrected := record.QualificationCorrections[len(record.QualificationCorrections)-1].AcceptanceMatrix
	correctedDigest, err := acceptanceMatrixDigest(corrected)
	if err != nil || correctedDigest != lastReview.AcceptanceMatrixSHA256 ||
		len(corrected) != len(record.Evidence.AcceptanceMatrix) {
		return false
	}
	for index := range corrected {
		if corrected[index].Identity != record.Evidence.AcceptanceMatrix[index].Identity ||
			corrected[index].Criterion != record.Evidence.AcceptanceMatrix[index].Criterion {
			return false
		}
	}
	return true
}

func hasLegacyGenericInvalidation(rows []deliveryevidence.AcceptanceRow) bool {
	for _, row := range rows {
		if row.PositiveEvidence != "planned: focused positive behavior through Advance" ||
			row.NegativeEvidence != "planned: focused negative behavior through Advance" ||
			row.FailureEvidence != "planned: failure behavior through Advance" ||
			row.MutationEvidence != "planned: persisted run mutation inspection" ||
			row.CompatibilityEvidence != "planned: compatibility validation" ||
			row.PreservationEvidence != "planned: prior run byte preservation" {
			return false
		}
	}
	return len(rows) > 0
}

func validQualificationFinding(finding deliveryevidence.ReviewFinding) bool {
	return strings.TrimSpace(finding.ID) != "" &&
		finding.Axis == deliveryevidence.ReviewSpec &&
		finding.Authority == deliveryevidence.AuthoritySpecRequirement &&
		(finding.Severity == deliveryevidence.SeverityP0 ||
			finding.Severity == deliveryevidence.SeverityP1 ||
			finding.Severity == deliveryevidence.SeverityP2 ||
			finding.Severity == deliveryevidence.SeverityP3) &&
		strings.TrimSpace(finding.Evidence) != ""
}

func validateQualificationCorrection(record runRecord, correction QualificationCorrection) error {
	pending := record.PendingQualificationCorrection
	if pending == nil || correction.AuthoritySHA256 != pending.AuthoritySHA256 ||
		correction.ReviewedMatrixSHA256 != pending.ReviewedMatrixSHA256 ||
		strings.TrimSpace(correction.Evidence) == "" ||
		len(correction.FindingIDs) != len(pending.FindingIDs) {
		return errors.New("qualification correction does not match the pending review findings")
	}
	gotIDs := append([]string(nil), correction.FindingIDs...)
	sort.Strings(gotIDs)
	for index := range gotIDs {
		if gotIDs[index] != pending.FindingIDs[index] {
			return errors.New("qualification correction must address every finding as one batch")
		}
	}
	current := record.Evidence.AcceptanceMatrix
	if len(correction.AcceptanceMatrix) != len(current) {
		return errors.New("qualification correction must preserve every acceptance criterion")
	}
	for index, row := range correction.AcceptanceMatrix {
		if row.Identity != current[index].Identity || row.Criterion != current[index].Criterion ||
			row.State != deliveryevidence.AcceptancePlanned ||
			strings.TrimSpace(row.OwningSeam) == "" ||
			strings.TrimSpace(row.PositiveEvidence) == "" ||
			strings.TrimSpace(row.NegativeEvidence) == "" ||
			strings.TrimSpace(row.FailureEvidence) == "" ||
			strings.TrimSpace(row.MutationEvidence) == "" ||
			strings.TrimSpace(row.CompatibilityEvidence) == "" ||
			strings.TrimSpace(row.PreservationEvidence) == "" ||
			strings.TrimSpace(row.MigrationEvidence) == "" {
			return errors.New("qualification correction must preserve traceability and complete every evidence row")
		}
	}
	return nil
}

func acceptanceMatrixDigest(rows []deliveryevidence.AcceptanceRow) (string, error) {
	raw, err := json.Marshal(rows)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
