package packsyncworkflow

import (
	"context"
	"errors"

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
	RepositoryRoot string
	Apply          packsync.CompositeApplyRequest
}

// BundlePublisher applies the existing ADR 0009 publication policy to one
// Pack-scoped composite proposal. Proposal.SourceID is deliberately the PackID
// so existing ownership evaluation uses sync/<pack-id>.
type BundlePublisher struct {
	Applier    CompositeApplier
	Builder    ProposalBuilder
	Diff       DiffVerifier
	Provenance CompositeProvenanceVerifier
	GitHub     PublicationGateway
}

// Prepare proves the complete Pack-scoped proposal through stable read-only
// publication observation without publishing or finalizing it.
func (publisher BundlePublisher) Prepare(ctx context.Context, request BundlePublishRequest) (PublicationPreparation, error) {
	delegate, err := publisher.delegate(request)
	if err != nil {
		return PublicationPreparation{}, err
	}
	preparation, err := delegate.Prepare(ctx, bundlePublishRequest(request))
	if err != nil {
		return preparation, err
	}
	if preparation.Proposal.SourceID != request.Apply.Plan.PackID {
		return PublicationPreparation{}, errors.New("bundle publication proposal is not Pack-scoped")
	}
	return preparation, nil
}

func (publisher BundlePublisher) Run(ctx context.Context, request BundlePublishRequest) (PublishResult, error) {
	delegate, err := publisher.delegate(request)
	if err != nil {
		return PublishResult{}, err
	}
	result, err := delegate.Run(ctx, bundlePublishRequest(request))
	if err != nil {
		return result, err
	}
	if result.Proposal.SourceID != request.Apply.Plan.PackID {
		return PublishResult{}, errors.New("bundle publication proposal is not Pack-scoped")
	}
	return result, nil
}

func (publisher BundlePublisher) delegate(request BundlePublishRequest) (Publisher, error) {
	if publisher.Applier == nil || publisher.Provenance == nil {
		return Publisher{}, errors.New("bundle publish requires complete-set Apply and provenance revalidation")
	}
	plan := request.Apply.Plan
	if request.RepositoryRoot == "" || plan.PlanID == "" || !ValidSourceID(plan.PackID) {
		return Publisher{}, errors.New("bundle publish requires one sealed Pack plan and sandbox repository")
	}
	return Publisher{
		Applier:    boundCompositeApplier{delegate: publisher.Applier, request: request.Apply},
		Builder:    packScopedProposalBuilder{delegate: publisher.Builder, packID: plan.PackID},
		Diff:       publisher.Diff,
		Provenance: boundCompositeProvenance{delegate: publisher.Provenance, plan: plan},
		GitHub:     publisher.GitHub,
	}, nil
}

func bundlePublishRequest(request BundlePublishRequest) PublishRequest {
	return PublishRequest{
		RepositoryRoot: request.RepositoryRoot,
		Apply:          packsync.ApplyRequest{Plan: packsync.Plan{PlanID: request.Apply.Plan.PlanID}},
	}
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
