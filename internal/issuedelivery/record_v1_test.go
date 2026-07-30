package issuedelivery

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

func TestDecodeLegacyRunPreservesCanonicalV1SemanticsAndEncoding(t *testing.T) {
	module, git, _, _, _ := assuranceFixture(t)
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)

	var current runRecord
	err := module.store.withIssueLock(
		context.Background(), git.value.CommonDir, request.IssueNumber,
		func(store lockedIssueStore) error {
			_, data, found, loadErr := store.loadActive()
			if loadErr != nil {
				return loadErr
			}
			if !found {
				t.Fatal("active run not found")
			}
			current, loadErr = decodeRun(data)
			return loadErr
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := deliveryevidence.CanonicalJSON(*current.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	legacyCandidates := make([]legacyCandidate, 0, len(current.Candidates))
	for _, candidate := range current.Candidates {
		legacyCandidates = append(legacyCandidates, legacyCandidate{
			ID: candidate.ID, BaseSHA: candidate.BaseSHA, CommitSHA: candidate.CommitSHA,
			TreeSHA: candidate.TreeSHA, RepairClass: candidate.RepairClass,
			RequiredReviews: candidate.RequiredReviews, Reviews: candidate.Reviews,
			Acceptance: candidate.Acceptance, Focused: candidate.Focused,
			Exhaustive: candidate.Exhaustive, RepairDecision: candidate.RepairDecision,
		})
	}
	legacyBytes, err := json.Marshal(legacyRunWire{
		Schema: legacyRunSchema, ID: current.ID,
		Repository: current.Repository, Issue: current.Issue,
		AuthoritySHA256: current.AuthoritySHA256, State: current.State, Reason: current.Reason,
		SupersedesRunID: current.SupersedesRunID,
		Evidence:        bytes.TrimSuffix(evidence, []byte{'\n'}),
		PendingDecision: current.PendingDecision, Decisions: current.Decisions,
		Observations: current.Observations, Candidates: legacyCandidates,
		PendingRepair: current.PendingRepair, LocalReadiness: current.LocalReadiness,
		Timing: current.Timing, CreatedAt: current.CreatedAt, UpdatedAt: current.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := decodeRun(legacyBytes)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != legacyRunSchema ||
		decoded.EffectiveProfile != "" ||
		len(decoded.Candidates) != 1 ||
		decoded.Candidates[0].Profile != "" ||
		decoded.Candidates[0].Effects != nil ||
		decoded.ProfileHistory != nil {
		t.Fatalf("decoded legacy run=%#v", decoded)
	}
	encoded, err := encodeRun(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, legacyBytes) {
		t.Fatalf("v1 encoding changed:\n got %s\nwant %s", encoded, legacyBytes)
	}
}

func TestLegacyRunContinuesUnderV1AssuranceWithoutRiskReclassification(t *testing.T) {
	module, git, tracker, _, validator := assuranceFixture(t)
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	risk := module.risk.(*fakeCandidateRiskObserver)
	initialRiskCalls := risk.calls

	var outcome Outcome
	err := module.store.withIssueLock(
		context.Background(), git.value.CommonDir, request.IssueNumber,
		func(store lockedIssueStore) error {
			_, data, _, loadErr := store.loadActive()
			if loadErr != nil {
				return loadErr
			}
			record, loadErr := decodeRun(data)
			if loadErr != nil {
				return loadErr
			}
			record.Schema = legacyRunSchema
			record.EffectiveProfile = ""
			record.RequiredBoundaries = nil
			record.ProfileHistory = nil
			for index := range record.Candidates {
				record.Candidates[index].ObservedFloor = ""
				record.Candidates[index].Profile = ""
				record.Candidates[index].Effects = nil
				record.Candidates[index].Boundaries = nil
				record.Candidates[index].RequiredSpecialists = nil
				record.Candidates[index].SpecialistReviews = nil
				record.Candidates[index].BoundaryProofs = nil
			}
			compiled, compileErr := compileAuthority(
				git.value, tracker.value, record.Decisions, nil, deliveryevidence.RiskStandard,
			)
			if compileErr != nil {
				return compileErr
			}
			outcome, loadErr = module.advanceAssurance(
				context.Background(), store, record, git.value, tracker.value, compiled, request,
			)
			return loadErr
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != StateNeedsReview || outcome.Candidate == nil ||
		len(outcome.Candidate.Reviews) != 2 || risk.calls != initialRiskCalls {
		t.Fatalf("legacy continuation outcome=%#v risk calls=%d", outcome, risk.calls)
	}
	err = module.store.withIssueLock(
		context.Background(), git.value.CommonDir, request.IssueNumber,
		func(store lockedIssueStore) error {
			_, data, _, loadErr := store.loadActive()
			if loadErr != nil {
				return loadErr
			}
			var envelope struct {
				Schema string `json:"schema"`
			}
			if loadErr = json.Unmarshal(data, &envelope); loadErr != nil {
				return loadErr
			}
			if envelope.Schema != legacyRunSchema {
				t.Fatalf("continued legacy schema=%q", envelope.Schema)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	validator.missingNegative = true
	if _, err = module.Advance(context.Background(), request); err == nil ||
		!strings.Contains(err.Error(), "explicit legacy-v1") {
		t.Fatalf("normal Advance admitted legacy v1 run: %v", err)
	}
	module.allowLegacyV1 = true
	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked ||
		!strings.Contains(blocked.Reason, "acceptance traceability") ||
		risk.calls != initialRiskCalls {
		t.Fatalf("weakened legacy acceptance outcome=%#v risk calls=%d", blocked, risk.calls)
	}
}
