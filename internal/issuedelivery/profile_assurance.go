package issuedelivery

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

func (m *Module) observeCandidateRisk(
	ctx context.Context,
	record *runRecord,
	candidate *Candidate,
	previous *Candidate,
	repositoryPath string,
) (bool, error) {
	if m.risk == nil {
		return false, errors.New("candidate risk observer is required for assurance")
	}
	observation, err := m.risk.ObserveCandidateRisk(ctx, CandidateRiskRequest{
		RunID: record.ID, CandidateID: candidate.ID, RepositoryPath: repositoryPath,
		StartingBaseSHA: record.Evidence.StartingBaseSHA,
		CommitSHA:       candidate.CommitSHA, TreeSHA: candidate.TreeSHA,
	})
	if err != nil {
		return false, fmt.Errorf("observe candidate risk: %w", err)
	}
	if !observation.Completed || observation.CandidateID != candidate.ID ||
		observation.CommitSHA != candidate.CommitSHA || observation.TreeSHA != candidate.TreeSHA {
		return false, errors.New("candidate risk observation does not match the exact candidate")
	}
	assessment := mechanicalProfileFloor(observation.Effects)
	effective := maxRiskProfile(
		maxRiskProfile(record.EffectiveProfile, m.declaredProfile),
		assessment.Profile,
	)
	boundaries := unionBoundaries(record.RequiredBoundaries, assessment.Boundaries)
	if effective == deliveryevidence.RiskHigh && len(boundaries) == 0 {
		boundaries = []SensitiveBoundary{BoundaryGovernance}
	}
	changed := candidate.Profile == "" ||
		record.EffectiveProfile != effective ||
		!equalBoundaries(record.RequiredBoundaries, boundaries)

	candidate.ObservedFloor = assessment.Profile
	candidate.Profile = effective
	candidate.Effects = append([]EffectObservation{}, assessment.Effects...)
	candidate.Boundaries = append([]SensitiveBoundary{}, boundaries...)
	if candidate.RepairClass == RepairBounded && previous != nil &&
		!boundedAssuranceShape(assessment, effective, boundaries, *previous) {
		candidate.RepairClass = RepairCandidateChanging
		candidate.RequiredReviews = bothReviewAxes()
	}
	candidate.RequiredSpecialists = []SensitiveBoundary{}
	if effective == deliveryevidence.RiskHigh {
		candidate.RequiredSpecialists = append(candidate.RequiredSpecialists, boundaries...)
		if candidate.RepairClass == RepairBounded && previous != nil {
			candidate.RequiredSpecialists = acceptedSpecialistBoundaries(*previous)
		}
	}
	record.EffectiveProfile = effective
	record.RequiredBoundaries = append([]SensitiveBoundary{}, boundaries...)
	record.Evidence.RiskProfile = effective

	if changed {
		record.ProfileHistory = append(record.ProfileHistory, ProfileTransition{
			Sequence: len(record.ProfileHistory) + 1, CandidateID: candidate.ID,
			ObservedFloor: assessment.Profile, EffectiveProfile: effective,
			Boundaries: append([]SensitiveBoundary{}, boundaries...),
			ObservedAt: m.clock.Now().UTC().Format(timeFormat),
		})
	}
	return changed, nil
}

func boundedAssuranceShape(
	assessment RiskAssessment,
	effective deliveryevidence.DeliveryRiskProfile,
	boundaries []SensitiveBoundary,
	previous Candidate,
) bool {
	if effective != previous.Profile {
		return false
	}
	for _, boundary := range boundaries {
		if !containsBoundary(previous.Boundaries, boundary) {
			return false
		}
	}
	previousEffects := map[CandidateEffect]bool{}
	for _, observation := range previous.Effects {
		previousEffects[observation.Effect] = true
	}
	for _, observation := range assessment.Effects {
		if !previousEffects[observation.Effect] {
			return false
		}
	}
	return true
}

func invalidateForProfileEscalation(record *runRecord, candidate *Candidate) {
	record.LocalReadiness = nil
	candidate.Acceptance = nil
	candidate.Exhaustive = nil
	record.Evidence.ValidationReceipts = []deliveryevidence.ValidationReceipt{}
	invalidateAcceptance(record.Evidence)
}

func (m *Module) executeSpecialistReviews(
	ctx context.Context,
	record runRecord,
	candidate Candidate,
	boundaries []SensitiveBoundary,
) ([]SpecialistReview, error) {
	if m.specialist == nil {
		return nil, errors.New("high-risk assurance requires a specialist review executor")
	}
	reviews := make([]SpecialistReview, 0, len(boundaries))
	for _, boundary := range boundaries {
		review, err := m.specialist.ReviewSpecialist(ctx, SpecialistReviewRequest{
			RunID: record.ID, CandidateID: candidate.ID, Repository: record.Repository, Issue: record.Issue,
			Boundary: boundary, Specialist: specialistForBoundary(boundary),
			BaseSHA: candidate.BaseSHA, CommitSHA: candidate.CommitSHA, TreeSHA: candidate.TreeSHA,
		})
		if err != nil {
			return nil, fmt.Errorf("execute %s specialist review: %w", boundary, err)
		}
		if err := validateSpecialistReview(review, candidate, boundary); err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	seen := map[string]bool{}
	for _, review := range candidate.Reviews {
		for _, finding := range review.Findings {
			seen[finding.ID] = true
		}
	}
	for _, review := range candidate.SpecialistReviews {
		for _, finding := range review.Findings {
			seen[finding.ID] = true
		}
	}
	for _, review := range reviews {
		for _, finding := range review.Findings {
			if seen[finding.ID] {
				return nil, fmt.Errorf("duplicate candidate finding ID %q", finding.ID)
			}
			seen[finding.ID] = true
		}
	}
	return reviews, nil
}

func validateSpecialistReview(review SpecialistReview, candidate Candidate, boundary SensitiveBoundary) error {
	if review.CandidateID != candidate.ID || review.Boundary != boundary ||
		review.Specialist != specialistForBoundary(boundary) || review.Findings == nil {
		return errors.New("specialist review does not match its exact boundary request")
	}
	if !review.Completed && len(review.Findings) != 0 {
		return errors.New("incomplete specialist review cannot contain findings")
	}
	seen := map[string]bool{}
	for _, finding := range review.Findings {
		if seen[finding.ID] || strings.TrimSpace(finding.ID) == "" ||
			strings.TrimSpace(finding.Citation) == "" ||
			strings.TrimSpace(finding.Location) == "" ||
			strings.TrimSpace(finding.Evidence) == "" {
			return errors.New("specialist review contains an invalid finding")
		}
		seen[finding.ID] = true
	}
	return nil
}

func missingSpecialistBoundaries(candidate *Candidate) []SensitiveBoundary {
	have := map[SensitiveBoundary]bool{}
	for _, review := range candidate.SpecialistReviews {
		if review.Completed {
			have[review.Boundary] = true
		}
	}
	var missing []SensitiveBoundary
	for _, boundary := range candidate.RequiredSpecialists {
		if !have[boundary] {
			missing = append(missing, boundary)
		}
	}
	return missing
}

func (m *Module) executeBoundaryProofs(
	ctx context.Context,
	record runRecord,
	candidate Candidate,
	boundaries []SensitiveBoundary,
) ([]BoundaryProof, error) {
	if m.boundary == nil {
		return nil, errors.New("high-risk assurance requires a boundary validation executor")
	}
	if err := validateSandboxRoot(m.sandboxRoot); err != nil {
		return nil, err
	}
	proofs := make([]BoundaryProof, 0, len(boundaries))
	for _, boundary := range boundaries {
		result, err := m.boundary.ValidateBoundary(ctx, BoundaryValidationRequest{
			RunID: record.ID, CandidateID: candidate.ID, Repository: record.Repository, Issue: record.Issue,
			Boundary: boundary, CommitSHA: candidate.CommitSHA, TreeSHA: candidate.TreeSHA,
			HomeRoot: m.sandboxRoot + "/home", ConfigRoot: m.sandboxRoot + "/config",
		})
		if err != nil {
			return nil, fmt.Errorf("validate %s boundary: %w", boundary, err)
		}
		if err := m.validateBoundaryResult(result, candidate, boundary); err != nil {
			return nil, err
		}
		proofs = append(proofs, BoundaryProof{
			Result: result, CompletedAt: m.clock.Now().UTC().Format(timeFormat),
		})
	}
	return proofs, nil
}

func (m *Module) validateBoundaryResult(
	result BoundaryValidationResult,
	candidate Candidate,
	boundary SensitiveBoundary,
) error {
	if err := validateSandboxRoot(m.sandboxRoot); err != nil {
		return err
	}
	if result.CandidateID != candidate.ID || result.Boundary != boundary ||
		result.CommitSHA != candidate.CommitSHA || result.TreeSHA != candidate.TreeSHA ||
		result.HomeRoot != m.sandboxRoot+"/home" || result.ConfigRoot != m.sandboxRoot+"/config" ||
		!result.Sandboxed || !result.Succeeded || !result.Completed ||
		!runIDPattern.MatchString(result.ToolSHA256) ||
		!runIDPattern.MatchString(result.OperatorStateBeforeSHA256) ||
		result.OperatorStateBeforeSHA256 != result.OperatorStateAfterSHA256 ||
		!runIDPattern.MatchString(result.WriteManifestSHA256) ||
		strings.TrimSpace(result.Command) == "" ||
		strings.TrimSpace(result.ToolIdentity) == "" ||
		strings.TrimSpace(result.Evidence) == "" {
		return errors.New("boundary proof does not prove the exact sandboxed candidate without operator mutation")
	}
	return nil
}

func missingBoundaryProofs(candidate *Candidate) []SensitiveBoundary {
	have := map[SensitiveBoundary]bool{}
	for _, proof := range candidate.BoundaryProofs {
		have[proof.Result.Boundary] = true
	}
	var missing []SensitiveBoundary
	for _, boundary := range candidate.Boundaries {
		if !have[boundary] {
			missing = append(missing, boundary)
		}
	}
	return missing
}

func acceptedSpecialistBoundaries(candidate Candidate) []SensitiveBoundary {
	accepted := map[string]bool{}
	if candidate.RepairDecision != nil {
		for _, decision := range candidate.RepairDecision.Findings {
			if decision.Disposition == FindingAccepted {
				accepted[decision.FindingID] = true
			}
		}
	}
	var boundaries []SensitiveBoundary
	for _, review := range candidate.SpecialistReviews {
		for _, finding := range review.Findings {
			if accepted[finding.ID] {
				boundaries = append(boundaries, review.Boundary)
				break
			}
		}
	}
	return unionBoundaries(nil, boundaries)
}

func unionBoundaries(left, right []SensitiveBoundary) []SensitiveBoundary {
	set := map[SensitiveBoundary]bool{}
	for _, boundary := range append(append([]SensitiveBoundary(nil), left...), right...) {
		if specialistForBoundary(boundary) != "governance-specialist" || boundary == BoundaryGovernance {
			set[boundary] = true
		}
	}
	out := []SensitiveBoundary{}
	for boundary := range set {
		out = append(out, boundary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func equalBoundaries(left, right []SensitiveBoundary) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalEffectObservations(left, right []EffectObservation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func containsBoundary(boundaries []SensitiveBoundary, wanted SensitiveBoundary) bool {
	for _, boundary := range boundaries {
		if boundary == wanted {
			return true
		}
	}
	return false
}
