package issuedelivery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

const runSchema = "packy.issue-delivery-run/v1"

type runWire struct {
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
	Timing          []Timing                            `json:"timing"`
	CreatedAt       string                              `json:"created_at"`
	UpdatedAt       string                              `json:"updated_at"`
}

func encodeRun(record runRecord) ([]byte, error) {
	wire := runWire{
		Schema: record.Schema, ID: record.ID, Repository: record.Repository, Issue: record.Issue,
		AuthoritySHA256: record.AuthoritySHA256, State: record.State, Reason: record.Reason,
		SupersedesRunID: record.SupersedesRunID, PendingDecision: record.PendingDecision,
		Decisions: record.Decisions, Observations: record.Observations, Timing: record.Timing,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
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
		CreatedAt: wire.CreatedAt, UpdatedAt: wire.UpdatedAt,
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
	if record.Schema != runSchema ||
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
		if record.PendingDecision == nil || record.Evidence != nil {
			return fmt.Errorf("needs-decision run requires one pending decision and no admitted evidence")
		}
	case StateNeedsReview, StateBlocked, StateCompleted:
		if record.PendingDecision != nil || record.Evidence == nil {
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
	return nil
}
