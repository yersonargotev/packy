package packsyncworkflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/yersonargotev/packy/internal/packsync"
)

// CompositeApplier is the complete-set transaction seam for register_bundle.
// ApplyComposite must never converge or publish individual members.
type CompositeApplier interface {
	ApplyComposite(context.Context, packsync.CompositeApplyRequest) (packsync.ApplyResult, error)
	RecoverPending(context.Context, string) (packsync.ApplyResult, bool, error)
}

// CompositeProvenanceVerifier reacquires and revalidates every exact member of
// one sealed composite plan. A source-level success has no authority.
type CompositeProvenanceVerifier interface {
	RevalidateComposite(context.Context, packsync.CompositePlan) error
}

type BundlePublishRequest struct {
	RepositoryRoot        string
	Apply                 packsync.CompositeApplyRequest
	ExpectedResultTreeSHA string
}

// BundlePublisher applies the existing ADR 0009 publication policy to one
// Pack-scoped composite proposal. Proposal.SourceID is deliberately the PackID
// so existing ownership evaluation uses sync/<pack-id>.
type BundlePublisher struct {
	Applier    CompositeApplier
	Validator  Validator
	Builder    ProposalBuilder
	Diff       DiffVerifier
	Provenance CompositeProvenanceVerifier
	GitHub     PublicationGateway
}

func (publisher BundlePublisher) Run(ctx context.Context, request BundlePublishRequest) (PublishResult, error) {
	if publisher.Applier == nil || publisher.Provenance == nil {
		return PublishResult{}, errors.New("bundle publish requires complete-set Apply and provenance revalidation")
	}
	plan := request.Apply.Plan
	if request.RepositoryRoot == "" || plan.PlanID == "" || !ValidSourceID(plan.PackID) || requireFullSHA("expected result tree", request.ExpectedResultTreeSHA) != nil {
		return PublishResult{}, errors.New("bundle publish requires one sealed Pack plan, expected result tree, and sandbox repository")
	}
	result, err := (Publisher{
		Applier:    boundCompositeApplier{delegate: publisher.Applier, request: request.Apply},
		Validator:  publisher.Validator,
		Builder:    packScopedProposalBuilder{delegate: publisher.Builder, packID: plan.PackID},
		Diff:       expectedBundleDiff{delegate: publisher.Diff, expectedResultTreeSHA: request.ExpectedResultTreeSHA},
		Provenance: boundCompositeProvenance{delegate: publisher.Provenance, plan: plan},
		GitHub:     publisher.GitHub,
	}).Run(ctx, PublishRequest{
		RepositoryRoot: request.RepositoryRoot,
		Apply:          packsync.ApplyRequest{Plan: packsync.Plan{PlanID: plan.PlanID}},
	})
	if err != nil {
		return result, err
	}
	if result.Proposal.SourceID != plan.PackID {
		return PublishResult{}, errors.New("bundle publication proposal is not Pack-scoped")
	}
	return result, nil
}

// expectedBundleDiff binds the Publish sandbox to the exact tree already
// reproduced by Validate. Seal runs before proposal construction or any
// publication-state observation, so a changed complete result cannot write.
type expectedBundleDiff struct {
	delegate              DiffVerifier
	expectedResultTreeSHA string
}

func (diff expectedBundleDiff) Seal(ctx context.Context, root string) (string, error) {
	sealed, err := diff.delegate.Seal(ctx, root)
	if err != nil {
		return "", err
	}
	if sealed != diff.expectedResultTreeSHA {
		return "", fmt.Errorf("Publish result tree %s does not reproduce sealed Validation result %s", sealed, diff.expectedResultTreeSHA)
	}
	return sealed, nil
}

func (diff expectedBundleDiff) VerifyWorkspace(ctx context.Context, root, seal string) error {
	return diff.delegate.VerifyWorkspace(ctx, root, seal)
}

func (diff expectedBundleDiff) VerifyCommit(ctx context.Context, root, seal, head string) error {
	return diff.delegate.VerifyCommit(ctx, root, seal, head)
}

type boundCompositeApplier struct {
	delegate CompositeApplier
	request  packsync.CompositeApplyRequest
}

func (bound boundCompositeApplier) Apply(ctx context.Context, _ packsync.ApplyRequest) (packsync.ApplyResult, error) {
	return bound.delegate.ApplyComposite(ctx, bound.request)
}

func (bound boundCompositeApplier) RecoverPending(ctx context.Context, root string) (packsync.ApplyResult, bool, error) {
	return bound.delegate.RecoverPending(ctx, root)
}

type boundCompositeProvenance struct {
	delegate CompositeProvenanceVerifier
	plan     packsync.CompositePlan
}

func (bound boundCompositeProvenance) RevalidateCandidate(ctx context.Context, _ packsync.Plan) error {
	return bound.delegate.RevalidateComposite(ctx, bound.plan)
}

type packScopedProposalBuilder struct {
	delegate ProposalBuilder
	packID   string
}

func (builder packScopedProposalBuilder) Build(ctx context.Context, root string, result packsync.ApplyResult) (Proposal, error) {
	if builder.delegate == nil {
		return Proposal{}, errors.New("bundle publish requires proposal construction")
	}
	proposal, err := builder.delegate.Build(ctx, root, result)
	if err != nil {
		return Proposal{}, err
	}
	if proposal.SourceID != builder.packID {
		return Proposal{}, errors.New("bundle publication proposal must use Pack identity")
	}
	return proposal, nil
}
