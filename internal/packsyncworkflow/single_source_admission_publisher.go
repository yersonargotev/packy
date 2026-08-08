package packsyncworkflow

import (
	"context"
	"errors"

	"github.com/yersonargotev/packy/internal/packsync"
)

// SingleSourceAdmissionApplier owns the complete initial-generation
// transaction. It must never publish or recover an individual component.
type SingleSourceAdmissionApplier interface {
	ApplySingleSourceAdmission(context.Context, packsync.SingleSourceAdmissionApplyRequest) (packsync.ApplyResult, error)
	RecoverPending(context.Context, string) (packsync.ApplyResult, bool, error)
}

type SingleSourceAdmissionProvenanceVerifier interface {
	RevalidateSingleSourceAdmissionCandidate(context.Context, packsync.SingleSourceAdmissionPlan) error
}

type SingleSourceAdmissionPublishRequest struct {
	RepositoryRoot string
	Apply          packsync.SingleSourceAdmissionApplyRequest
}

// SingleSourceAdmissionPublisher binds the generic protected publication
// workflow to one complete single-source Pack generation.
type SingleSourceAdmissionPublisher struct {
	Applier    SingleSourceAdmissionApplier
	Builder    ProposalBuilder
	Diff       DiffVerifier
	Provenance SingleSourceAdmissionProvenanceVerifier
	GitHub     PublicationGateway
}

func (publisher SingleSourceAdmissionPublisher) Run(ctx context.Context, request SingleSourceAdmissionPublishRequest) (PublishResult, error) {
	if publisher.Applier == nil || publisher.Provenance == nil || request.RepositoryRoot == "" ||
		request.Apply.Plan.PlanID == "" || !ValidSourceID(request.Apply.Plan.Registration.ID) {
		return PublishResult{}, errors.New("single-source admission publish requires complete Apply, provenance, and one sealed source plan")
	}
	delegate := Publisher{
		Applier:    boundSingleSourceAdmissionApplier{delegate: publisher.Applier, request: request.Apply},
		Builder:    publisher.Builder,
		Diff:       publisher.Diff,
		Provenance: boundSingleSourceAdmissionProvenance{delegate: publisher.Provenance, plan: request.Apply.Plan},
		GitHub:     publisher.GitHub,
	}
	return delegate.Run(ctx, PublishRequest{
		RepositoryRoot: request.RepositoryRoot,
		Apply:          packsync.ApplyRequest{Plan: packsync.Plan{PlanID: request.Apply.Plan.PlanID}},
	})
}

type boundSingleSourceAdmissionApplier struct {
	delegate SingleSourceAdmissionApplier
	request  packsync.SingleSourceAdmissionApplyRequest
}

func (bound boundSingleSourceAdmissionApplier) Apply(ctx context.Context, _ packsync.ApplyRequest) (packsync.ApplyResult, error) {
	return bound.delegate.ApplySingleSourceAdmission(ctx, bound.request)
}

func (bound boundSingleSourceAdmissionApplier) RecoverPending(ctx context.Context, root string) (packsync.ApplyResult, bool, error) {
	return bound.delegate.RecoverPending(ctx, root)
}

type boundSingleSourceAdmissionProvenance struct {
	delegate SingleSourceAdmissionProvenanceVerifier
	plan     packsync.SingleSourceAdmissionPlan
}

func (bound boundSingleSourceAdmissionProvenance) RevalidateCandidate(ctx context.Context, _ packsync.Plan) error {
	return bound.delegate.RevalidateSingleSourceAdmissionCandidate(ctx, bound.plan)
}
