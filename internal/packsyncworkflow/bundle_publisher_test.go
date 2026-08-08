package packsyncworkflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/packsync"
)

func TestBundlePublisherUsesPackScopedExistingPublicationPolicy(t *testing.T) {
	proposal := bundleProposal()
	base := bundlePublicationState(proposal)
	tests := map[string]func(*PublicationState){
		"edited metadata": func(state *PublicationState) { state.PR.LastEditor = "maintainer" },
		"divergence":      func(state *PublicationState) { state.Branch.Diverged = true },
		"changed base":    func(state *PublicationState) { state.BaseSHA = baseB },
		"closed PR":       func(state *PublicationState) { state.PR.Open = false },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			state := base
			mutate(&state)
			decision, err := EvaluatePublication(proposal, state)
			if err == nil || decision.Action != PublicationBlock || decision.Branch != "sync/vercel" {
				t.Fatalf("decision = %#v, %v", decision, err)
			}
		})
	}
	decision, err := EvaluatePublication(proposal, base)
	if err != nil || decision.Action != PublicationNoop || decision.Branch != "sync/vercel" {
		t.Fatalf("exact noop = %#v, %v", decision, err)
	}
}

func TestBundlePublisherRevalidatesCompleteMembersTwiceBeforeExactNoop(t *testing.T) {
	events := []string{}
	proposal := bundleProposal()
	state := bundlePublicationState(proposal)
	gateway := &fakePublicationGateway{events: &events, states: []PublicationState{state, state}}
	provenance := &fakeCompositeProvenance{events: &events}
	publisher := BundlePublisher{
		Applier: fakeCompositeApplier{events: &events},
		Builder: bundleProposalBuilder{proposal: proposal}, Diff: fakeDiff{},
		Provenance: provenance, GitHub: gateway,
	}
	result, err := publisher.Run(context.Background(), BundlePublishRequest{
		RepositoryRoot: t.TempDir(),
		Apply:          packsync.CompositeApplyRequest{Plan: packsync.CompositePlan{PlanID: proposal.PlanID, PackID: "vercel"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Action != PublicationNoop || gateway.publishCalls != 0 || provenance.calls != 2 {
		t.Fatalf("result=%#v writes=%d provenance=%d", result.Decision, gateway.publishCalls, provenance.calls)
	}

	provenance.failAt = 2
	provenance.calls = 0
	gateway.states = []PublicationState{state}
	if _, err := publisher.Run(context.Background(), BundlePublishRequest{RepositoryRoot: t.TempDir(), Apply: packsync.CompositeApplyRequest{Plan: packsync.CompositePlan{PlanID: proposal.PlanID, PackID: "vercel"}}}); err == nil {
		t.Fatal("changed complete-member provenance reached publication")
	}
}

func TestBundlePublisherPrepareReturnsStableReadOnlyEvidenceWithoutPublishing(t *testing.T) {
	events := []string{}
	proposal := bundleProposal()
	state := bundlePublicationState(proposal)
	gateway := &fakePublicationGateway{events: &events, states: []PublicationState{state, state}}
	publisher := BundlePublisher{
		Applier: fakeCompositeApplier{events: &events},
		Builder: bundleProposalBuilder{proposal: proposal}, Diff: fakeDiff{},
		Provenance: &fakeCompositeProvenance{events: &events}, GitHub: gateway,
	}

	preparation, err := publisher.Prepare(context.Background(), BundlePublishRequest{
		RepositoryRoot: t.TempDir(),
		Apply:          packsync.CompositeApplyRequest{Plan: packsync.CompositePlan{PlanID: proposal.PlanID, PackID: "vercel"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if preparation.Proposal.SourceID != "vercel" || preparation.ObservedState.PR.Number != state.PR.Number {
		t.Fatalf("preparation = %#v", preparation)
	}
	if contains(events, "publish") || contains(events, "finalize") || gateway.publishCalls != 0 {
		t.Fatalf("read-only preparation crossed publication mutation boundary: %v", events)
	}
}

func TestBundlePublisherPrepareFailsClosedWithoutPublishing(t *testing.T) {
	events := []string{}
	proposal := bundleProposal()
	first := bundlePublicationState(proposal)
	second := first
	second.BaseSHA = baseB
	gateway := &fakePublicationGateway{events: &events, states: []PublicationState{first, second}}
	publisher := BundlePublisher{
		Applier: fakeCompositeApplier{events: &events},
		Builder: bundleProposalBuilder{proposal: proposal}, Diff: fakeDiff{},
		Provenance: &fakeCompositeProvenance{events: &events}, GitHub: gateway,
	}

	_, err := publisher.Prepare(context.Background(), BundlePublishRequest{
		RepositoryRoot: t.TempDir(),
		Apply:          packsync.CompositeApplyRequest{Plan: packsync.CompositePlan{PlanID: proposal.PlanID, PackID: "vercel"}},
	})
	if err == nil {
		t.Fatal("changed observation prepared publication")
	}
	if contains(events, "publish") || contains(events, "finalize") || gateway.publishCalls != 0 {
		t.Fatalf("failed preparation crossed publication mutation boundary: %v", events)
	}
}

type fakeCompositeApplier struct{ events *[]string }

func (fake fakeCompositeApplier) ApplyComposite(context.Context, packsync.CompositeApplyRequest) (packsync.ApplyResult, error) {
	*fake.events = append(*fake.events, "apply-composite")
	return packsync.ApplyResult{Status: "applied", Changed: true}, nil
}
func (fake fakeCompositeApplier) RecoverPending(context.Context, string) (packsync.ApplyResult, bool, error) {
	return packsync.ApplyResult{}, false, nil
}

type fakeCompositeProvenance struct {
	events *[]string
	calls  int
	failAt int
}

func (fake *fakeCompositeProvenance) RevalidateComposite(context.Context, packsync.CompositePlan) error {
	*fake.events = append(*fake.events, "provenance-composite")
	fake.calls++
	if fake.calls == fake.failAt {
		return errors.New("member moved")
	}
	return nil
}

type bundleProposalBuilder struct{ proposal Proposal }

func (builder bundleProposalBuilder) Build(context.Context, string, packsync.ApplyResult) (Proposal, error) {
	return builder.proposal, nil
}

func bundleProposal() Proposal {
	return Proposal{
		SourceID: "vercel", PlanID: "bundle-plan", BaseSHA: baseA, CandidateSHA: candidateA,
		ResultTreeSHA: headA, HeadSHA: headA, ProvenanceSHA256: strings.Repeat("5", 64),
		ManagedTitle: "register(vercel): composite", ManagedMetadataHash: strings.Repeat("6", 64),
		Validation: completeValidationGates(), InvalidationConditions: DecisionReadyInvalidationConditions(),
	}
}

func bundlePublicationState(proposal Proposal) PublicationState {
	return PublicationState{
		BaseSHA: proposal.BaseSHA, ProvenanceCurrent: true, CandidateRelation: CandidateSame,
		Branch: BranchState{Exists: true, Name: "sync/vercel", HeadSHA: proposal.HeadSHA, Owner: AutomationOwner, ManagedMetadataHash: proposal.ManagedMetadataHash},
		PR:     PRState{Exists: true, Number: 7, Open: true, BaseBranch: "main", HeadBranch: "sync/vercel", HeadSHA: proposal.HeadSHA, MetadataHash: proposal.ManagedMetadataHash, Owner: AutomationOwner},
		Record: PublicationRecord{PlanID: proposal.PlanID, BaseSHA: proposal.BaseSHA, CandidateSHA: proposal.CandidateSHA, HeadSHA: proposal.HeadSHA, ResultTreeSHA: proposal.ResultTreeSHA, ProvenanceSHA256: proposal.ProvenanceSHA256, MetadataHash: proposal.ManagedMetadataHash},
	}
}
