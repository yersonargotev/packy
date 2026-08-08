package packsyncworkflow

import (
	"context"
	"errors"
	"testing"

	"github.com/yersonargotev/packy/internal/packsync"
)

func TestSingleSourceAdmissionPublisherUsesCompleteApplyAndProvenanceForDecisionReadiness(t *testing.T) {
	events := []string{}
	proposal := validProposal()
	state := pristineState()
	gateway := &fakePublicationGateway{events: &events, states: []PublicationState{state, state}}
	provenance := &fakeSingleSourceAdmissionProvenance{events: &events}
	publisher := SingleSourceAdmissionPublisher{
		Applier: fakeSingleSourceAdmissionApplier{events: &events}, Builder: fakeProposalBuilder{events: &events},
		Diff: fakeDiff{}, Provenance: provenance, GitHub: gateway,
	}
	result, err := publisher.Run(context.Background(), SingleSourceAdmissionPublishRequest{
		RepositoryRoot: t.TempDir(),
		Apply: packsync.SingleSourceAdmissionApplyRequest{Plan: packsync.SingleSourceAdmissionPlan{
			PlanID: proposal.PlanID, Registration: packsync.SourceConfig{ID: proposal.SourceID},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Action != PublicationNoop || !result.Readiness.DecisionReady || gateway.publishCalls != 0 || provenance.calls != 2 {
		t.Fatalf("result=%#v writes=%d provenance=%d", result, gateway.publishCalls, provenance.calls)
	}
	if !contains(events, "apply-single-source-admission") || contains(events, "apply") {
		t.Fatalf("publisher used an individual-component Apply seam: %v", events)
	}

	provenance.calls = 0
	provenance.failAt = 2
	gateway.states = []PublicationState{state}
	if _, err := publisher.Run(context.Background(), SingleSourceAdmissionPublishRequest{
		RepositoryRoot: t.TempDir(),
		Apply: packsync.SingleSourceAdmissionApplyRequest{Plan: packsync.SingleSourceAdmissionPlan{
			PlanID: proposal.PlanID, Registration: packsync.SourceConfig{ID: proposal.SourceID},
		}},
	}); err == nil {
		t.Fatal("changed single-source admission provenance reached decision readiness")
	}
}

type fakeSingleSourceAdmissionApplier struct{ events *[]string }

func (fake fakeSingleSourceAdmissionApplier) ApplySingleSourceAdmission(context.Context, packsync.SingleSourceAdmissionApplyRequest) (packsync.ApplyResult, error) {
	*fake.events = append(*fake.events, "apply-single-source-admission")
	return packsync.ApplyResult{Status: "applied", Changed: true}, nil
}

func (fake fakeSingleSourceAdmissionApplier) RecoverPending(context.Context, string) (packsync.ApplyResult, bool, error) {
	return packsync.ApplyResult{}, false, nil
}

type fakeSingleSourceAdmissionProvenance struct {
	events *[]string
	calls  int
	failAt int
}

func (fake *fakeSingleSourceAdmissionProvenance) RevalidateSingleSourceAdmissionCandidate(context.Context, packsync.SingleSourceAdmissionPlan) error {
	*fake.events = append(*fake.events, "provenance-single-source-admission")
	fake.calls++
	if fake.calls == fake.failAt {
		return errors.New("candidate moved")
	}
	return nil
}
