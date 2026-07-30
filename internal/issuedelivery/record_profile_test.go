package issuedelivery

import (
	"context"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

func TestValidateRunRejectsProfileDescentAcrossCandidates(t *testing.T) {
	record := candidateJournalFixture(t)
	first := record.Candidates[0]
	setJournalCandidateRisk(&first, EffectSecurity, deliveryevidence.RiskHigh, BoundarySecurity)
	second := nextJournalCandidate(record, first)
	setJournalCandidateRisk(&second, EffectPassive, deliveryevidence.RiskLow, "")
	record.Candidates = []Candidate{first, second}
	record.EffectiveProfile = deliveryevidence.RiskLow
	record.RequiredBoundaries = []SensitiveBoundary{}
	record.Evidence.RiskProfile = deliveryevidence.RiskLow
	record.ProfileHistory = []ProfileTransition{{
		Sequence: 1, CandidateID: second.ID, ObservedFloor: deliveryevidence.RiskLow,
		EffectiveProfile: deliveryevidence.RiskLow, Boundaries: []SensitiveBoundary{},
		ObservedAt: record.UpdatedAt,
	}}

	if err := validateRun(record); err == nil ||
		!strings.Contains(err.Error(), "candidate assurance is not monotonic") {
		t.Fatalf("descending candidate profile error=%v", err)
	}
}

func TestValidateRunRejectsSensitiveBoundaryRemovalAcrossCandidates(t *testing.T) {
	record := candidateJournalFixture(t)
	first := record.Candidates[0]
	setJournalCandidateRisk(&first, EffectSecurity, deliveryevidence.RiskHigh, BoundarySecurity)
	second := nextJournalCandidate(record, first)
	setJournalCandidateRisk(&second, EffectGovernance, deliveryevidence.RiskHigh, BoundaryGovernance)
	record.Candidates = []Candidate{first, second}
	record.EffectiveProfile = deliveryevidence.RiskHigh
	record.RequiredBoundaries = []SensitiveBoundary{BoundaryGovernance}
	record.Evidence.RiskProfile = deliveryevidence.RiskHigh
	record.ProfileHistory = []ProfileTransition{{
		Sequence: 1, CandidateID: second.ID, ObservedFloor: deliveryevidence.RiskHigh,
		EffectiveProfile: deliveryevidence.RiskHigh,
		Boundaries:       []SensitiveBoundary{BoundaryGovernance},
		ObservedAt:       record.UpdatedAt,
	}}

	if err := validateRun(record); err == nil ||
		!strings.Contains(err.Error(), "candidate assurance is not monotonic") {
		t.Fatalf("removed candidate boundary error=%v", err)
	}
}

func TestValidateRunRejectsProfileTransitionsOutsideCandidateOrder(t *testing.T) {
	record := candidateJournalFixture(t)
	first := record.Candidates[0]
	second := nextJournalCandidate(record, first)
	setJournalCandidateRisk(&second, EffectPassive, deliveryevidence.RiskLow, "")
	record.Candidates = []Candidate{first, second}
	record.ProfileHistory = []ProfileTransition{
		{
			Sequence: 1, CandidateID: second.ID, ObservedFloor: deliveryevidence.RiskLow,
			EffectiveProfile: deliveryevidence.RiskLow, Boundaries: []SensitiveBoundary{},
			ObservedAt: record.UpdatedAt,
		},
		{
			Sequence: 2, CandidateID: first.ID, ObservedFloor: deliveryevidence.RiskLow,
			EffectiveProfile: deliveryevidence.RiskLow, Boundaries: []SensitiveBoundary{},
			ObservedAt: record.UpdatedAt,
		},
	}

	if err := validateRun(record); err == nil ||
		!strings.Contains(err.Error(), "profile history is invalid") {
		t.Fatalf("permuted profile history error=%v", err)
	}
}

func candidateJournalFixture(t *testing.T) runRecord {
	t.Helper()
	module, git, _, _, _ := assuranceFixture(t)
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	var record runRecord
	err := module.store.withIssueLock(
		context.Background(), git.value.CommonDir, request.IssueNumber,
		func(store lockedIssueStore) error {
			_, data, _, loadErr := store.loadActive()
			if loadErr != nil {
				return loadErr
			}
			record, loadErr = decodeRun(data)
			return loadErr
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func nextJournalCandidate(record runRecord, previous Candidate) Candidate {
	commit, tree := strings.Repeat("d", 40), strings.Repeat("e", 40)
	return Candidate{
		ID:      candidateIdentity(record.ID, previous.BaseSHA, commit, tree),
		BaseSHA: previous.BaseSHA, CommitSHA: commit, TreeSHA: tree,
		RepairClass:     RepairCandidateChanging,
		RequiredReviews: bothReviewAxes(), Reviews: []CandidateReview{},
		Effects: []EffectObservation{}, Boundaries: []SensitiveBoundary{},
		RequiredSpecialists: []SensitiveBoundary{}, SpecialistReviews: []SpecialistReview{},
		BoundaryProofs: []BoundaryProof{},
	}
}

func setJournalCandidateRisk(
	candidate *Candidate,
	effect CandidateEffect,
	profile deliveryevidence.DeliveryRiskProfile,
	boundary SensitiveBoundary,
) {
	candidate.ObservedFloor = profile
	candidate.Profile = profile
	candidate.Effects = []EffectObservation{{
		Effect: effect, Evidence: "journal monotonicity test", Complete: true,
	}}
	candidate.Boundaries = []SensitiveBoundary{}
	candidate.RequiredSpecialists = []SensitiveBoundary{}
	if boundary != "" {
		candidate.Boundaries = append(candidate.Boundaries, boundary)
		candidate.RequiredSpecialists = append(candidate.RequiredSpecialists, boundary)
	}
}
