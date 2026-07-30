package issuedelivery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

const runSchema = "packy.issue-delivery-run/v2"

type runWire struct {
	Schema             string                               `json:"schema"`
	ID                 string                               `json:"id"`
	Repository         deliveryevidence.RepositoryIdentity  `json:"repository"`
	Issue              deliveryevidence.IssueIdentity       `json:"issue"`
	AuthoritySHA256    string                               `json:"authority_sha256"`
	State              State                                `json:"state"`
	Reason             string                               `json:"reason"`
	SupersedesRunID    string                               `json:"supersedes_run_id,omitempty"`
	Evidence           json.RawMessage                      `json:"evidence,omitempty"`
	PendingDecision    *DecisionRequest                     `json:"pending_decision,omitempty"`
	Decisions          []Decision                           `json:"decisions"`
	Observations       Observations                         `json:"observations"`
	Candidates         []Candidate                          `json:"candidates,omitempty"`
	PendingRepair      *RepairDecisionRequest               `json:"pending_repair,omitempty"`
	LocalReadiness     *LocalReadiness                      `json:"local_readiness,omitempty"`
	EffectiveProfile   deliveryevidence.DeliveryRiskProfile `json:"effective_profile,omitempty"`
	RequiredBoundaries []SensitiveBoundary                  `json:"required_boundaries,omitempty"`
	ProfileHistory     []ProfileTransition                  `json:"profile_history,omitempty"`
	NonLocal           *NonLocalDelivery                    `json:"non_local,omitempty"`
	Timing             []Timing                             `json:"timing"`
	CreatedAt          string                               `json:"created_at"`
	UpdatedAt          string                               `json:"updated_at"`
}

func encodeRun(record runRecord) ([]byte, error) {
	if record.Schema == legacyRunSchema {
		return encodeLegacyRun(record)
	}
	wire := runWire{
		Schema: record.Schema, ID: record.ID, Repository: record.Repository, Issue: record.Issue,
		AuthoritySHA256: record.AuthoritySHA256, State: record.State, Reason: record.Reason,
		SupersedesRunID: record.SupersedesRunID, PendingDecision: record.PendingDecision,
		Decisions: record.Decisions, Observations: record.Observations, Timing: record.Timing,
		Candidates: record.Candidates, PendingRepair: record.PendingRepair,
		LocalReadiness:   record.LocalReadiness,
		EffectiveProfile: record.EffectiveProfile, RequiredBoundaries: record.RequiredBoundaries,
		ProfileHistory: record.ProfileHistory,
		NonLocal:       record.NonLocal,
		CreatedAt:      record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
	if record.Evidence != nil {
		evidence, err := deliveryevidence.CanonicalJSON(*record.Evidence)
		if err != nil {
			return nil, err
		}
		wire.Evidence = bytes.TrimSuffix(evidence, []byte{'\n'})
	}
	return json.Marshal(wire)
}

func decodeRun(data []byte) (runRecord, error) {
	var envelope struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return runRecord{}, fmt.Errorf("decode issue delivery run: %w", err)
	}
	if envelope.Schema == legacyRunSchema {
		return decodeLegacyRun(data)
	}
	var wire runWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return runRecord{}, fmt.Errorf("decode issue delivery run: %w", err)
	}
	canonical, err := json.Marshal(wire)
	if err != nil {
		return runRecord{}, err
	}
	if !bytes.Equal(data, canonical) || wire.Schema != runSchema || !validRunID(wire.ID) {
		return runRecord{}, fmt.Errorf("issue delivery run is not canonical")
	}
	record := runRecord{
		Schema: wire.Schema, ID: wire.ID, Repository: wire.Repository, Issue: wire.Issue,
		AuthoritySHA256: wire.AuthoritySHA256, State: wire.State, Reason: wire.Reason,
		SupersedesRunID: wire.SupersedesRunID, PendingDecision: wire.PendingDecision,
		Decisions: wire.Decisions, Observations: wire.Observations, Timing: wire.Timing,
		Candidates: wire.Candidates, PendingRepair: wire.PendingRepair,
		LocalReadiness:   wire.LocalReadiness,
		EffectiveProfile: wire.EffectiveProfile, RequiredBoundaries: wire.RequiredBoundaries,
		ProfileHistory: wire.ProfileHistory,
		NonLocal:       wire.NonLocal,
		CreatedAt:      wire.CreatedAt, UpdatedAt: wire.UpdatedAt,
	}
	if len(wire.Evidence) > 0 {
		evidence, err := deliveryevidence.Decode(append(append([]byte(nil), wire.Evidence...), '\n'))
		if err != nil {
			return runRecord{}, err
		}
		record.Evidence = &evidence
	}
	if err := validateRun(record); err != nil {
		return runRecord{}, err
	}
	return record, nil
}

func validateRun(record runRecord) error {
	if (record.Schema != runSchema && record.Schema != legacyRunSchema) ||
		record.ID != runIdentity(record.Repository, record.Issue, record.AuthoritySHA256, record.SupersedesRunID) {
		return fmt.Errorf("issue delivery run identity is invalid")
	}
	if record.Repository.Owner == "" || record.Repository.Name == "" || record.Repository.NodeID == "" ||
		record.Issue.Number <= 0 || record.Issue.NodeID == "" ||
		len(record.AuthoritySHA256) != 64 || !runIDPattern.MatchString(record.AuthoritySHA256) {
		return fmt.Errorf("issue delivery run authority identity is incomplete")
	}
	if strings.TrimSpace(record.Reason) == "" || record.Decisions == nil || record.Timing == nil ||
		strings.TrimSpace(record.CreatedAt) == "" || strings.TrimSpace(record.UpdatedAt) == "" {
		return fmt.Errorf("issue delivery run state is incomplete")
	}
	switch record.State {
	case StateNeedsDecision:
		authorityDecision := record.PendingDecision != nil && record.PendingRepair == nil && record.Evidence == nil
		repairDecision := record.PendingDecision == nil && record.PendingRepair != nil && record.Evidence != nil
		if !authorityDecision && !repairDecision {
			return fmt.Errorf("needs-decision run requires exactly one authority or repair decision")
		}
	case StateNeedsReview, StateWaiting, StateBlocked, StateCompleted:
		if record.PendingDecision != nil || record.PendingRepair != nil || record.Evidence == nil {
			return fmt.Errorf("%s run requires admitted evidence and no pending decision", record.State)
		}
	default:
		return fmt.Errorf("persisted issue delivery run has invalid state %q", record.State)
	}
	if record.Evidence != nil {
		if record.Evidence.Repository != record.Repository || record.Evidence.Issue != record.Issue ||
			record.Evidence.Authority.IssueSHA256 != record.AuthoritySHA256 {
			return fmt.Errorf("issue delivery evidence does not match its run")
		}
	}
	if record.Observations.Repository != record.Repository || record.Observations.Issue != record.Issue ||
		record.Observations.AuthoritySHA256 != record.AuthoritySHA256 ||
		!fullGitSHAPattern.MatchString(record.Observations.CommitSHA) ||
		!fullGitSHAPattern.MatchString(record.Observations.TreeSHA) {
		return fmt.Errorf("issue delivery run observations do not match its authority")
	}
	for i, timing := range record.Timing {
		started, startErr := time.Parse(timeFormat, timing.StartedAt)
		completed, completeErr := time.Parse(timeFormat, timing.CompletedAt)
		if timing.Sequence != i+1 || strings.TrimSpace(timing.Phase) == "" ||
			startErr != nil || completeErr != nil || completed.Before(started) {
			return fmt.Errorf("issue delivery run timing is invalid")
		}
	}
	if len(record.Timing) == 0 || record.Timing[len(record.Timing)-1].To != record.State {
		return fmt.Errorf("issue delivery run timing does not reach current state")
	}
	if _, err := time.Parse(timeFormat, record.CreatedAt); err != nil {
		return fmt.Errorf("issue delivery run creation time is invalid")
	}
	if _, err := time.Parse(timeFormat, record.UpdatedAt); err != nil {
		return fmt.Errorf("issue delivery run update time is invalid")
	}
	for _, decision := range record.Decisions {
		if strings.TrimSpace(decision.RequestID) == "" {
			return fmt.Errorf("issue delivery run contains an invalid decision")
		}
	}
	if err := validateCandidates(record); err != nil {
		return err
	}
	return nil
}

func validateCandidates(record runRecord) error {
	if record.Schema == legacyRunSchema {
		return validateLegacyCandidates(record)
	}
	if len(record.Candidates) == 0 {
		if record.PendingRepair != nil || record.LocalReadiness != nil {
			return fmt.Errorf("issue delivery assurance state requires a candidate")
		}
		if record.EffectiveProfile != "" && record.Evidence != nil &&
			record.EffectiveProfile != record.Evidence.RiskProfile {
			return fmt.Errorf("issue delivery run profile does not match its evidence")
		}
		return nil
	}
	seen := make(map[string]bool, len(record.Candidates))
	candidateOrder := make(map[string]int, len(record.Candidates))
	for index, candidate := range record.Candidates {
		if candidate.ID != candidateIdentity(record.ID, candidate.BaseSHA, candidate.CommitSHA, candidate.TreeSHA) ||
			seen[candidate.ID] ||
			!fullGitSHAPattern.MatchString(candidate.BaseSHA) ||
			!fullGitSHAPattern.MatchString(candidate.CommitSHA) ||
			!fullGitSHAPattern.MatchString(candidate.TreeSHA) ||
			(len(candidate.RequiredReviews) == 0 &&
				(candidate.RepairClass != RepairBounded || len(candidate.RequiredSpecialists) == 0)) ||
			candidate.Reviews == nil ||
			candidate.Effects == nil || candidate.Boundaries == nil ||
			candidate.RequiredSpecialists == nil || candidate.SpecialistReviews == nil ||
			candidate.BoundaryProofs == nil ||
			(candidate.ObservedFloor != deliveryevidence.RiskLow &&
				candidate.ObservedFloor != deliveryevidence.RiskStandard &&
				candidate.ObservedFloor != deliveryevidence.RiskHigh) ||
			(candidate.Profile != deliveryevidence.RiskLow &&
				candidate.Profile != deliveryevidence.RiskStandard &&
				candidate.Profile != deliveryevidence.RiskHigh) {
			return fmt.Errorf("issue delivery candidate %d is invalid", index+1)
		}
		seen[candidate.ID] = true
		candidateOrder[candidate.ID] = index
		assessment := mechanicalProfileFloor(candidate.Effects)
		if !assessment.Complete ||
			assessment.Profile != candidate.ObservedFloor ||
			!equalEffectObservations(assessment.Effects, candidate.Effects) ||
			maxRiskProfile(candidate.ObservedFloor, candidate.Profile) != candidate.Profile ||
			!equalBoundaries(candidate.Boundaries, unionBoundaries(nil, candidate.Boundaries)) {
			return fmt.Errorf("issue delivery candidate risk assessment is invalid")
		}
		for _, boundary := range assessment.Boundaries {
			if !containsBoundary(candidate.Boundaries, boundary) {
				return fmt.Errorf("issue delivery candidate omits an observed sensitive boundary")
			}
		}
		if candidate.Profile == deliveryevidence.RiskHigh && len(candidate.Boundaries) == 0 {
			return fmt.Errorf("high-risk issue delivery candidate requires a sensitive boundary")
		}
		if candidate.Profile != deliveryevidence.RiskHigh && len(candidate.RequiredSpecialists) != 0 {
			return fmt.Errorf("non-high-risk issue delivery candidate requires no specialist")
		}
		if !equalBoundaries(candidate.RequiredSpecialists, unionBoundaries(nil, candidate.RequiredSpecialists)) {
			return fmt.Errorf("issue delivery candidate specialist boundaries are invalid")
		}
		for _, boundary := range candidate.RequiredSpecialists {
			if !containsBoundary(candidate.Boundaries, boundary) {
				return fmt.Errorf("issue delivery candidate specialist is outside its sensitive boundaries")
			}
		}
		if candidate.Profile == deliveryevidence.RiskHigh && candidate.RepairClass != RepairBounded &&
			!equalBoundaries(candidate.RequiredSpecialists, candidate.Boundaries) {
			return fmt.Errorf("high-risk issue delivery candidate omits a required specialist")
		}
		if index > 0 {
			previous := record.Candidates[index-1]
			if maxRiskProfile(previous.Profile, candidate.Profile) != candidate.Profile ||
				!equalBoundaries(candidate.Boundaries, unionBoundaries(previous.Boundaries, candidate.Boundaries)) {
				return fmt.Errorf("issue delivery candidate assurance is not monotonic")
			}
		}
		required := make(map[deliveryevidence.ReviewAxis]bool, len(candidate.RequiredReviews))
		for _, axis := range candidate.RequiredReviews {
			if (axis != deliveryevidence.ReviewStandards && axis != deliveryevidence.ReviewSpec) || required[axis] {
				return fmt.Errorf("issue delivery candidate has invalid required reviews")
			}
			required[axis] = true
		}
		findingIDs := make(map[string]bool)
		for _, review := range candidate.Reviews {
			if review.CandidateID != candidate.ID || !required[review.Axis] || review.Findings == nil {
				return fmt.Errorf("issue delivery candidate contains an invalid review")
			}
			if !review.Completed && len(review.Findings) != 0 {
				return fmt.Errorf("incomplete issue delivery candidate review contains findings")
			}
			for _, finding := range review.Findings {
				if findingIDs[finding.ID] || strings.TrimSpace(finding.ID) == "" || finding.Axis != review.Axis {
					return fmt.Errorf("issue delivery candidate contains an invalid finding")
				}
				findingIDs[finding.ID] = true
			}
		}
		for _, proof := range []*ValidationProof{candidate.Focused, candidate.Exhaustive} {
			if proof == nil {
				continue
			}
			if proof.Result.CommitSHA != candidate.CommitSHA || proof.Result.TreeSHA != candidate.TreeSHA ||
				strings.TrimSpace(proof.CompletedAt) == "" || !proof.Result.Sandboxed ||
				!proof.Result.Succeeded || !proof.Result.Completed ||
				!filepath.IsAbs(proof.Result.HomeRoot) || filepath.Clean(proof.Result.HomeRoot) != proof.Result.HomeRoot ||
				!filepath.IsAbs(proof.Result.ConfigRoot) || filepath.Clean(proof.Result.ConfigRoot) != proof.Result.ConfigRoot ||
				proof.Result.HomeRoot == proof.Result.ConfigRoot {
				return fmt.Errorf("issue delivery candidate contains an invalid validation proof")
			}
		}
		if candidate.Focused != nil && candidate.Focused.Kind != "focused" {
			return fmt.Errorf("issue delivery candidate contains an invalid focused proof")
		}
		if candidate.Exhaustive != nil &&
			(candidate.Exhaustive.Kind != "exhaustive" ||
				candidate.Exhaustive.Result.Command != "./scripts/validate-packy.sh" ||
				candidate.Exhaustive.Result.ValidatorIdentity != "scripts/validate-packy.sh" ||
				!runIDPattern.MatchString(candidate.Exhaustive.Result.CheckoutSHA256) ||
				!runIDPattern.MatchString(candidate.Exhaustive.Result.ValidatorSHA256) ||
				!candidate.Exhaustive.Result.WorkspaceClean) {
			return fmt.Errorf("issue delivery candidate contains an invalid exhaustive proof")
		}
		specialists := map[SensitiveBoundary]bool{}
		for _, review := range candidate.SpecialistReviews {
			if specialists[review.Boundary] || review.CandidateID != candidate.ID ||
				!containsBoundary(candidate.RequiredSpecialists, review.Boundary) ||
				review.Specialist != specialistForBoundary(review.Boundary) || review.Findings == nil {
				return fmt.Errorf("issue delivery candidate contains an invalid specialist review")
			}
			specialists[review.Boundary] = true
		}
		proofs := map[SensitiveBoundary]bool{}
		for _, proof := range candidate.BoundaryProofs {
			result := proof.Result
			if proofs[result.Boundary] || result.CandidateID != candidate.ID ||
				!containsBoundary(candidate.Boundaries, result.Boundary) ||
				result.CommitSHA != candidate.CommitSHA || result.TreeSHA != candidate.TreeSHA ||
				!result.Sandboxed || !result.Succeeded || !result.Completed ||
				result.OperatorStateBeforeSHA256 != result.OperatorStateAfterSHA256 ||
				!runIDPattern.MatchString(result.OperatorStateBeforeSHA256) ||
				!runIDPattern.MatchString(result.ToolSHA256) ||
				!runIDPattern.MatchString(result.WriteManifestSHA256) {
				return fmt.Errorf("issue delivery candidate contains an invalid boundary proof")
			}
			proofs[result.Boundary] = true
		}
	}
	current := record.Candidates[len(record.Candidates)-1]
	if record.PendingRepair != nil &&
		(record.PendingRepair.CandidateID != current.ID || strings.TrimSpace(record.PendingRepair.ID) == "" ||
			len(record.PendingRepair.FindingIDs) == 0) {
		return fmt.Errorf("pending repair does not match the current candidate")
	}
	if record.LocalReadiness != nil &&
		(record.LocalReadiness.CandidateID != current.ID ||
			record.LocalReadiness.CommitSHA != current.CommitSHA ||
			record.LocalReadiness.TreeSHA != current.TreeSHA ||
			record.LocalReadiness.AuthorityHash != record.AuthoritySHA256 ||
			strings.TrimSpace(record.LocalReadiness.Branch) == "" ||
			strings.TrimSpace(record.LocalReadiness.ReadyAt) == "") {
		return fmt.Errorf("local readiness does not match the current candidate")
	}
	if record.LocalReadiness != nil {
		if current.Exhaustive == nil || len(current.Acceptance) != len(record.Evidence.AcceptanceMatrix) ||
			len(record.Evidence.ValidationReceipts) == 0 ||
			len(missingSpecialistBoundaries(&current)) != 0 ||
			len(missingBoundaryProofs(&current)) != 0 {
			return fmt.Errorf("local readiness lacks exact candidate assurance")
		}
		for _, row := range record.Evidence.AcceptanceMatrix {
			if row.State != deliveryevidence.AcceptanceProved {
				return fmt.Errorf("local readiness contains unproved acceptance")
			}
		}
	}
	if record.EffectiveProfile != current.Profile || record.Evidence.RiskProfile != record.EffectiveProfile ||
		!equalBoundaries(record.RequiredBoundaries, current.Boundaries) {
		return fmt.Errorf("issue delivery run profile does not match its current candidate")
	}
	previousProfile := deliveryevidence.RiskLow
	previousBoundaries := []SensitiveBoundary{}
	previousCandidateIndex := -1
	for index, transition := range record.ProfileHistory {
		candidateIndex, candidateFound := candidateOrder[transition.CandidateID]
		if transition.Sequence != index+1 || !candidateFound ||
			(candidateIndex != previousCandidateIndex && candidateIndex != previousCandidateIndex+1) ||
			(transition.ObservedFloor != deliveryevidence.RiskLow &&
				transition.ObservedFloor != deliveryevidence.RiskStandard &&
				transition.ObservedFloor != deliveryevidence.RiskHigh) ||
			maxRiskProfile(transition.ObservedFloor, transition.EffectiveProfile) != transition.EffectiveProfile ||
			maxRiskProfile(previousProfile, transition.EffectiveProfile) != transition.EffectiveProfile ||
			!equalBoundaries(transition.Boundaries, unionBoundaries(nil, transition.Boundaries)) ||
			!equalBoundaries(transition.Boundaries, unionBoundaries(previousBoundaries, transition.Boundaries)) ||
			strings.TrimSpace(transition.ObservedAt) == "" {
			return fmt.Errorf("issue delivery profile history is invalid")
		}
		candidate := record.Candidates[candidateIndex]
		if maxRiskProfile(transition.EffectiveProfile, candidate.Profile) != candidate.Profile ||
			!equalBoundaries(candidate.Boundaries, unionBoundaries(transition.Boundaries, candidate.Boundaries)) {
			return fmt.Errorf("issue delivery profile transition does not match its candidate")
		}
		if candidateIndex != previousCandidateIndex && previousCandidateIndex >= 0 {
			previousCandidate := record.Candidates[previousCandidateIndex]
			if previousProfile != previousCandidate.Profile ||
				!equalBoundaries(previousBoundaries, previousCandidate.Boundaries) {
				return fmt.Errorf("issue delivery profile history does not close its candidate")
			}
		}
		previousProfile = transition.EffectiveProfile
		previousBoundaries = transition.Boundaries
		previousCandidateIndex = candidateIndex
	}
	if len(record.ProfileHistory) == 0 ||
		previousCandidateIndex != len(record.Candidates)-1 ||
		record.ProfileHistory[len(record.ProfileHistory)-1].EffectiveProfile != record.EffectiveProfile ||
		!equalBoundaries(record.ProfileHistory[len(record.ProfileHistory)-1].Boundaries, record.RequiredBoundaries) {
		return fmt.Errorf("issue delivery profile history does not reach current profile")
	}
	if err := validateNonLocalRecord(record, current); err != nil {
		return err
	}
	return nil
}
