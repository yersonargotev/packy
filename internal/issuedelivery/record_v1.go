package issuedelivery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

const legacyRunSchema = "packy.issue-delivery-run/v1"

type legacyCandidate struct {
	ID              string                        `json:"id"`
	BaseSHA         string                        `json:"base_sha"`
	CommitSHA       string                        `json:"commit_sha"`
	TreeSHA         string                        `json:"tree_sha"`
	RepairClass     RepairClass                   `json:"repair_class,omitempty"`
	RequiredReviews []deliveryevidence.ReviewAxis `json:"required_reviews"`
	Reviews         []CandidateReview             `json:"reviews"`
	Acceptance      []AcceptanceProof             `json:"acceptance,omitempty"`
	Focused         *ValidationProof              `json:"focused,omitempty"`
	Exhaustive      *ValidationProof              `json:"exhaustive,omitempty"`
	RepairDecision  *RepairDecision               `json:"repair_decision,omitempty"`
}

type legacyRunWire struct {
	Schema          string                              `json:"schema"`
	ID              string                              `json:"id"`
	Repository      deliveryevidence.RepositoryIdentity `json:"repository"`
	Issue           deliveryevidence.IssueIdentity      `json:"issue"`
	AuthoritySHA256 string                              `json:"authority_sha256"`
	State           State                               `json:"state"`
	Reason          string                              `json:"reason"`
	SupersedesRunID string                              `json:"supersedes_run_id,omitempty"`
	Evidence        json.RawMessage                     `json:"evidence,omitempty"`
	PendingDecision *DecisionRequest                    `json:"pending_decision,omitempty"`
	Decisions       []Decision                          `json:"decisions"`
	Observations    Observations                        `json:"observations"`
	Candidates      []legacyCandidate                   `json:"candidates,omitempty"`
	PendingRepair   *RepairDecisionRequest              `json:"pending_repair,omitempty"`
	LocalReadiness  *LocalReadiness                     `json:"local_readiness,omitempty"`
	Timing          []Timing                            `json:"timing"`
	CreatedAt       string                              `json:"created_at"`
	UpdatedAt       string                              `json:"updated_at"`
}

func encodeLegacyRun(record runRecord) ([]byte, error) {
	candidates := make([]legacyCandidate, 0, len(record.Candidates))
	for _, candidate := range record.Candidates {
		candidates = append(candidates, legacyCandidate{
			ID: candidate.ID, BaseSHA: candidate.BaseSHA, CommitSHA: candidate.CommitSHA,
			TreeSHA: candidate.TreeSHA, RepairClass: candidate.RepairClass,
			RequiredReviews: candidate.RequiredReviews, Reviews: candidate.Reviews,
			Acceptance: candidate.Acceptance, Focused: candidate.Focused,
			Exhaustive: candidate.Exhaustive, RepairDecision: candidate.RepairDecision,
		})
	}
	wire := legacyRunWire{
		Schema: legacyRunSchema, ID: record.ID, Repository: record.Repository, Issue: record.Issue,
		AuthoritySHA256: record.AuthoritySHA256, State: record.State, Reason: record.Reason,
		SupersedesRunID: record.SupersedesRunID, PendingDecision: record.PendingDecision,
		Decisions: record.Decisions, Observations: record.Observations, Candidates: candidates,
		PendingRepair: record.PendingRepair, LocalReadiness: record.LocalReadiness,
		Timing: record.Timing, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
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

func decodeLegacyRun(data []byte) (runRecord, error) {
	var wire legacyRunWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return runRecord{}, fmt.Errorf("decode legacy issue delivery run: %w", err)
	}
	canonical, err := json.Marshal(wire)
	if err != nil {
		return runRecord{}, err
	}
	if !bytes.Equal(data, canonical) || wire.Schema != legacyRunSchema || !validRunID(wire.ID) {
		return runRecord{}, fmt.Errorf("legacy issue delivery run is not canonical")
	}
	record := runRecord{
		Schema: legacyRunSchema, ID: wire.ID, Repository: wire.Repository, Issue: wire.Issue,
		AuthoritySHA256: wire.AuthoritySHA256, State: wire.State, Reason: wire.Reason,
		SupersedesRunID: wire.SupersedesRunID, PendingDecision: wire.PendingDecision,
		Decisions: wire.Decisions, Observations: wire.Observations,
		PendingRepair: wire.PendingRepair, LocalReadiness: wire.LocalReadiness,
		Timing: wire.Timing, CreatedAt: wire.CreatedAt, UpdatedAt: wire.UpdatedAt,
	}
	if len(wire.Evidence) > 0 {
		evidence, err := deliveryevidence.Decode(append(append([]byte(nil), wire.Evidence...), '\n'))
		if err != nil {
			return runRecord{}, err
		}
		record.Evidence = &evidence
	}
	for _, legacy := range wire.Candidates {
		record.Candidates = append(record.Candidates, Candidate{
			ID: legacy.ID, BaseSHA: legacy.BaseSHA, CommitSHA: legacy.CommitSHA, TreeSHA: legacy.TreeSHA,
			RepairClass: legacy.RepairClass, RequiredReviews: legacy.RequiredReviews,
			Reviews: legacy.Reviews, Acceptance: legacy.Acceptance,
			Focused: legacy.Focused, Exhaustive: legacy.Exhaustive, RepairDecision: legacy.RepairDecision,
		})
	}
	if err := validateRun(record); err != nil {
		return runRecord{}, err
	}
	return record, nil
}

func validateLegacyCandidates(record runRecord) error {
	if len(record.Candidates) == 0 {
		if record.PendingRepair != nil || record.LocalReadiness != nil {
			return fmt.Errorf("issue delivery assurance state requires a candidate")
		}
		return nil
	}
	seen := make(map[string]bool, len(record.Candidates))
	for index, candidate := range record.Candidates {
		if candidate.ID != candidateIdentity(record.ID, candidate.BaseSHA, candidate.CommitSHA, candidate.TreeSHA) ||
			seen[candidate.ID] ||
			!fullGitSHAPattern.MatchString(candidate.BaseSHA) ||
			!fullGitSHAPattern.MatchString(candidate.CommitSHA) ||
			!fullGitSHAPattern.MatchString(candidate.TreeSHA) ||
			len(candidate.RequiredReviews) == 0 || candidate.Reviews == nil {
			return fmt.Errorf("issue delivery candidate %d is invalid", index+1)
		}
		seen[candidate.ID] = true
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
			len(record.Evidence.ValidationReceipts) == 0 {
			return fmt.Errorf("local readiness lacks exact candidate assurance")
		}
		for _, row := range record.Evidence.AcceptanceMatrix {
			if row.State != deliveryevidence.AcceptanceProved {
				return fmt.Errorf("local readiness contains unproved acceptance")
			}
		}
	}
	return nil
}
