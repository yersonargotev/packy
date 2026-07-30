package issuedelivery

import (
	"bytes"
	"encoding/json"
	"fmt"

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
	return record, nil
}
