package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/packy/internal/packclassification"
	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

func readSingleSourceAdmissionInputs(option options) (packsyncworkflow.DispatchRequest, packsync.SingleSourceAdmissionPlan, error) {
	var request packsyncworkflow.DispatchRequest
	var plan packsync.SingleSourceAdmissionPlan
	if err := readJSON(option.requestPath, &request); err != nil {
		return request, plan, err
	}
	if err := request.Validate(); err != nil || !request.IsSingleSourceAdmission() {
		return request, plan, errors.New("single-source admission requires one complete valid v2.3 register dispatch")
	}
	if err := readJSON(option.planPath, &plan); err != nil {
		return request, plan, err
	}
	if !plan.VerifySeal() || request.Registration == nil || request.LegalAdmission == nil ||
		request.SourceID != plan.Registration.ID || request.RegistrationSHA256 != plan.RegistrationSHA256 ||
		request.ProposedVersion != plan.ProposedVersion || request.ProposedManifestSHA256 != plan.ProposedManifestSHA256 ||
		request.LegalAdmission.EvidenceReference != plan.LegalAdmission.EvidenceReference ||
		request.LegalAdmission.EvidenceSHA256 != plan.LegalAdmission.EvidenceSHA256 ||
		request.LegalAdmission.Disposition != plan.LegalAdmission.Disposition {
		return request, plan, errors.New("v2.3 dispatch and sealed single-source admission plan identity contradict")
	}
	return request, plan, nil
}

func classifySingleSourceAdmission(ctx context.Context, option options, output io.Writer) error {
	if option.requestPath == "" || option.planPath == "" || option.outputDir == "" {
		return errors.New("single-source admission Classify requires request, plan, and output paths")
	}
	request, plan, err := readSingleSourceAdmissionInputs(option)
	if err != nil {
		return err
	}
	var set packsync.CompositeClassificationEvidence
	switch request.ClassificationMode {
	case packsyncworkflow.ClassificationAI:
		classificationRequest := packclassification.Request{
			SchemaVersion: 1, RequestID: plan.PlanID + "/" + plan.PackID, Mode: packclassification.ModeAI,
			PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, PackID: plan.PackID,
			CurrentVersion: plan.Classification.CurrentVersion, MechanicalFloor: plan.Classification.MechanicalFloor,
			SemanticEvidenceRequired: plan.Classification.SemanticEvidenceRequired,
			MechanicalReasons:        []string{"initial complete single-source Pack generation"},
		}
		evidence, modelErr := bundleClassificationAttempt(ctx, classificationRequest)
		if modelErr != nil {
			return classificationFailure(modelErr)
		}
		set = packsync.CompositeClassificationEvidence{SchemaVersion: 1, PlanID: plan.PlanID, PackID: plan.PackID, Evidence: evidence}
	case packsyncworkflow.ClassificationHuman:
		if request.ExpectedPlanID != plan.PlanID || request.ExpectedBaseSHA != plan.Preconditions.BaseCommit {
			return classificationFailure(errors.New("human single-source admission evidence is stale against the sealed plan"))
		}
		if err := json.Unmarshal(request.HumanEvidence, &set); err != nil {
			return classificationFailure(fmt.Errorf("decode human single-source admission evidence: %w", err))
		}
	}
	if err := packsync.ValidateSingleSourceAdmissionClassificationEvidence(plan, set); err != nil {
		return classificationFailure(err)
	}
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	name := filepath.Join(option.outputDir, "classification.json")
	if err := writeCanonical(name, set); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, name)
	return err
}

func readSingleSourceAdmissionEvidence(path string, plan packsync.SingleSourceAdmissionPlan) (packsync.CompositeClassificationEvidence, error) {
	var evidence packsync.CompositeClassificationEvidence
	if err := readJSON(path, &evidence); err != nil {
		return evidence, err
	}
	if err := packsync.ValidateSingleSourceAdmissionClassificationEvidence(plan, evidence); err != nil {
		return evidence, err
	}
	return evidence, nil
}

func singleSourceAdmissionApplyRequest(repositoryRoot, acquisition string, request packsyncworkflow.DispatchRequest, plan packsync.SingleSourceAdmissionPlan, evidence packsync.CompositeClassificationEvidence) packsync.SingleSourceAdmissionApplyRequest {
	return packsync.SingleSourceAdmissionApplyRequest{
		SingleSourceAdmissionCheckRequest: packsync.SingleSourceAdmissionCheckRequest{
			RepositoryRoot: repositoryRoot, AcquisitionDir: acquisition,
			Registration: *request.Registration, RegistrationSHA256: request.RegistrationSHA256,
			ProposedVersion: request.ProposedVersion, ProposedManifest: request.ProposedManifest,
			ProposedManifestSHA256: request.ProposedManifestSHA256, LegalAdmission: *request.LegalAdmission,
		},
		Plan: plan, ClassificationEvidence: evidence,
	}
}

func applySingleSourceAdmission(ctx context.Context, option options) (packsyncworkflow.DispatchRequest, packsync.SingleSourceAdmissionPlan, error) {
	request, plan, err := readSingleSourceAdmissionInputs(option)
	if err != nil {
		return request, plan, err
	}
	evidence, err := readSingleSourceAdmissionEvidence(option.evidencePath, plan)
	if err != nil {
		return request, plan, err
	}
	acquisition, err := os.MkdirTemp("", "packy-single-source-admission-apply-")
	if err != nil {
		return request, plan, err
	}
	defer os.RemoveAll(acquisition)
	engine := packsync.Engine{Source: workflowSourceFactory(), Validate: workflowValidatorFactory()}
	_, err = engine.ApplySingleSourceAdmission(ctx, singleSourceAdmissionApplyRequest(option.repositoryRoot, acquisition, request, plan, evidence))
	return request, plan, err
}

func validateSingleSourceAdmissionSandbox(ctx context.Context, option options, output io.Writer) error {
	if option.requestPath == "" || option.planPath == "" || option.evidencePath == "" || option.outputDir == "" {
		return errors.New("single-source admission Validate requires request, plan, evidence, and output paths")
	}
	request, plan, err := applySingleSourceAdmission(ctx, option)
	if err != nil {
		return err
	}
	validator := workflowValidatorFactory()
	if applied, ok := validator.(packsyncworkflow.AppliedValidator); ok {
		err = applied.ValidateApplied(ctx, option.repositoryRoot)
	} else {
		err = validator.Validate(ctx, option.repositoryRoot)
	}
	if err != nil {
		return packsyncworkflow.Failure{Kind: packsyncworkflow.FailureValidation, Err: err}
	}
	if err := stageAll(ctx, option.repositoryRoot); err != nil {
		return err
	}
	resultTree, err := command(ctx, option.repositoryRoot, "git", "write-tree")
	if err != nil {
		return err
	}
	artifact := packsyncworkflow.ValidationArtifact{
		SchemaVersion: 2, SourceID: request.SourceID, PlanID: plan.PlanID,
		BaseSHA: plan.Preconditions.BaseCommit, CandidateSHA: plan.Candidate.Commit,
		ArtifactProvenance:               singleSourceAdmissionProvenance(plan),
		InitialAdmissionArtifactIdentity: singleSourceAdmissionIdentity(plan),
		ResultTreeSHA:                    strings.TrimSpace(resultTree), PackySuite: true, Apply: true,
		ClosingIssue: request.ClosingIssue,
	}
	if err := artifact.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	name := filepath.Join(option.outputDir, "validation.json")
	if err := writeCanonical(name, artifact); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, name)
	return err
}

func singleSourceAdmissionProvenance(plan packsync.SingleSourceAdmissionPlan) packsyncworkflow.ArtifactProvenance {
	return packsyncworkflow.ArtifactProvenance{
		SourceLockSHA256: plan.SourceLockSHA256, LockSetSHA256: plan.LockSetSHA256,
		ConfigSHA256: plan.Preconditions.ConfigSHA256, ManifestsSHA256: plan.Preconditions.ManifestsSHA256,
	}
}

func singleSourceAdmissionIdentity(plan packsync.SingleSourceAdmissionPlan) packsyncworkflow.InitialAdmissionArtifactIdentity {
	return packsyncworkflow.InitialAdmissionArtifactIdentity{
		PackID: plan.PackID, ProposedVersion: plan.ProposedVersion, ProposedManifestSHA256: plan.ProposedManifestSHA256,
		LegalEvidenceReference: plan.LegalAdmission.EvidenceReference, LegalEvidenceSHA256: plan.LegalAdmission.EvidenceSHA256,
		ResultBundleSHA256: plan.ResultBundleSHA256,
	}
}

type singleSourceAdmissionProposalBuilder struct {
	request      packsyncworkflow.DispatchRequest
	plan         packsync.SingleSourceAdmissionPlan
	evidence     packsync.CompositeClassificationEvidence
	evidencePath string
	gateway      singleSourceAdmissionGateway
}

type singleSourceAdmissionGateway interface {
	packsyncworkflow.PublicationGateway
	configureSingleSourceAdmission(string, *packsyncworkflow.ReviewBrief)
	singleSourceAdmissionReviewBrief() *packsyncworkflow.ReviewBrief
}

var singleSourceAdmissionGatewayFactory = func(repositoryRoot string, plan packsync.Plan) singleSourceAdmissionGateway {
	return workflowGatewayFactory(repositoryRoot, plan)
}

func (builder *singleSourceAdmissionProposalBuilder) Build(ctx context.Context, root string, result packsync.ApplyResult) (packsyncworkflow.Proposal, error) {
	if err := stageAll(ctx, root); err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	provenance, err := fileHash(filepath.Join(root, "bundle", "sources", builder.plan.Registration.ID+".lock.json"))
	if err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	head, err := prepareCommit(ctx, root, builder.plan.Registration.ID, packsync.Plan{PlanID: builder.plan.PlanID, Candidate: builder.plan.Candidate})
	if err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	traces := []packsyncworkflow.ClassifierTrace{}
	_ = readJSON(filepath.Join(filepath.Dir(builder.evidencePath), "classifier-trace.json"), &traces)
	brief := &packsyncworkflow.ReviewBrief{
		SchemaVersion: 1, Actor: os.Getenv("GITHUB_ACTOR"), RunID: os.Getenv("GITHUB_RUN_ID"),
		RunAttempt: os.Getenv("GITHUB_RUN_ATTEMPT"), RunURL: actionsRunURL(), Repository: os.Getenv("GITHUB_REPOSITORY"),
		Request: builder.request, Candidate: builder.plan.Candidate, PlanID: builder.plan.PlanID,
		BaseSHA: builder.plan.Preconditions.BaseCommit, HeadSHA: head, Branch: "sync/" + builder.plan.Registration.ID,
		SelectedResources: builder.plan.ProposedLock.Resources, ProposedSnapshotSHA256: builder.plan.ResultBundleSHA256,
		Classification: []packsync.ClassificationEvidence{builder.evidence.Evidence}, ClassifierTrace: traces,
		ApplyStatus: result.Status, UpstreamContentExecuted: false,
		Blockers:  []string{"Publication remains blocked until the exact post-write pull request identity is reobserved."},
		AutoMerge: false, ManualMergeRequired: true,
		Recovery: []string{"Repeat Inspect when any sealed initial-generation fact changes."},
	}
	title := fmt.Sprintf("sync(%s): %s", builder.plan.Registration.ID, builder.plan.Candidate.Commit[:12])
	proposal := packsyncworkflow.Proposal{
		SourceID: builder.plan.Registration.ID, PlanID: builder.plan.PlanID,
		BaseSHA: builder.plan.Preconditions.BaseCommit, CandidateSHA: builder.plan.Candidate.Commit,
		HeadSHA: head, ProvenanceSHA256: provenance, ManagedTitle: title, ClosingIssue: builder.request.ClosingIssue,
	}
	builder.gateway.configureSingleSourceAdmission(title, brief)
	return proposal, nil
}

type singleSourceAdmissionPublicationRuntime struct {
	request   packsyncworkflow.DispatchRequest
	plan      packsync.SingleSourceAdmissionPlan
	publisher packsyncworkflow.SingleSourceAdmissionPublisher
	gateway   singleSourceAdmissionGateway
	publish   packsyncworkflow.SingleSourceAdmissionPublishRequest
	cleanup   func()
}

func newSingleSourceAdmissionPublicationRuntime(option options) (singleSourceAdmissionPublicationRuntime, error) {
	if option.requestPath == "" || option.planPath == "" || option.evidencePath == "" || option.outputDir == "" {
		return singleSourceAdmissionPublicationRuntime{}, errors.New("single-source admission publication requires request, plan, evidence, and output paths")
	}
	request, plan, err := readSingleSourceAdmissionInputs(option)
	if err != nil {
		return singleSourceAdmissionPublicationRuntime{}, err
	}
	evidence, err := readSingleSourceAdmissionEvidence(option.evidencePath, plan)
	if err != nil {
		return singleSourceAdmissionPublicationRuntime{}, err
	}
	acquisition, err := os.MkdirTemp("", "packy-single-source-admission-publish-")
	if err != nil {
		return singleSourceAdmissionPublicationRuntime{}, err
	}
	validator := workflowValidatorFactory()
	engine := packsync.Engine{Source: workflowSourceFactory(), Validate: validator}
	apply := singleSourceAdmissionApplyRequest(option.repositoryRoot, acquisition, request, plan, evidence)
	gateway := singleSourceAdmissionGatewayFactory(option.repositoryRoot, packsync.Plan{SourceID: plan.Registration.ID, Candidate: plan.Candidate, Preconditions: plan.Preconditions})
	builder := &singleSourceAdmissionProposalBuilder{request: request, plan: plan, evidence: evidence, evidencePath: option.evidencePath, gateway: gateway}
	publisher := packsyncworkflow.SingleSourceAdmissionPublisher{Applier: engine, Builder: builder, Diff: gitDiffVerifier{}, Provenance: engine, GitHub: gateway}
	return singleSourceAdmissionPublicationRuntime{
		request: request, plan: plan, publisher: publisher, gateway: gateway,
		publish: packsyncworkflow.SingleSourceAdmissionPublishRequest{RepositoryRoot: option.repositoryRoot, Apply: apply},
		cleanup: func() { _ = os.RemoveAll(acquisition) },
	}, nil
}

func publishSingleSourceAdmission(ctx context.Context, option options, output io.Writer) error {
	runtime, err := newSingleSourceAdmissionPublicationRuntime(option)
	if err != nil {
		return err
	}
	defer runtime.cleanup()
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	if err := stageAll(ctx, option.repositoryRoot); err != nil {
		return err
	}
	baseStatus, err := command(ctx, option.repositoryRoot, "git", "status", "--porcelain")
	if err != nil || strings.TrimSpace(baseStatus) != "" {
		return errors.New("publish sandbox must begin from the exact clean base")
	}
	result, err := runtime.publisher.Run(ctx, runtime.publish)
	if err != nil {
		return err
	}
	brief := runtime.gateway.singleSourceAdmissionReviewBrief()
	if brief == nil {
		return errors.New("single-source admission publication did not produce canonical review evidence")
	}
	brief.PullRequest = result.PullRequest.Number
	brief.HeadSHA = result.PullRequest.HeadSHA
	brief.ResultTreeSHA = result.Proposal.ResultTreeSHA
	brief.Validation = result.Readiness.Gates
	brief.DecisionReady = result.Readiness.DecisionReady
	brief.Blockers = nil
	brief.InvalidationConditions = result.Proposal.InvalidationConditions
	if err := writeCanonical(filepath.Join(option.outputDir, "proposal-brief.json"), brief); err != nil {
		return err
	}
	markdown, err := brief.Markdown()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(option.outputDir, "proposal-brief.md"), []byte(markdown), 0o600); err != nil {
		return err
	}
	artifact := packsyncworkflow.PublicationArtifact{
		SchemaVersion: 2, SourceID: runtime.request.SourceID, PlanID: runtime.plan.PlanID,
		BaseSHA: runtime.plan.Preconditions.BaseCommit, CandidateSHA: runtime.plan.Candidate.Commit,
		ArtifactProvenance:               singleSourceAdmissionProvenance(runtime.plan),
		InitialAdmissionArtifactIdentity: singleSourceAdmissionIdentity(runtime.plan),
		ResultTreeSHA:                    result.Proposal.ResultTreeSHA, HeadSHA: result.Proposal.HeadSHA,
		ProvenanceSHA256: result.Proposal.ProvenanceSHA256, BranchName: result.Decision.Branch,
		PRNumber: result.PullRequest.Number, PRStateSHA256: result.PullRequest.MetadataHash,
		ManagedTitle: result.Proposal.ManagedTitle, ManagedMetadataHash: result.PullRequest.MetadataHash,
		Validation: result.Readiness.Gates, DecisionReady: result.Readiness.DecisionReady,
		AutoMerge: false, ManualMergeRequired: true, UpstreamContentExecuted: false,
		InvalidationConditions: result.Proposal.InvalidationConditions, ClosingIssue: runtime.request.ClosingIssue,
	}
	if err := artifact.Validate(); err != nil {
		return err
	}
	name := filepath.Join(option.outputDir, "publication.json")
	if err := writeCanonical(name, artifact); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, name)
	return err
}
