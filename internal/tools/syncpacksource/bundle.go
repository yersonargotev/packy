package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

var bundleClassificationAttempt = func(ctx context.Context, request packclassification.Request) (packsync.ClassificationEvidence, error) {
	model, err := newGitHubModel()
	if err != nil {
		return packsync.ClassificationEvidence{}, err
	}
	return model.Attempt(ctx, request)
}

func isBundleDispatch(option options) (bool, error) {
	if option.requestPath == "" {
		return os.Getenv("PACKY_OPERATION") == string(packsyncworkflow.OperationRegisterBundle), nil
	}
	data, err := os.ReadFile(option.requestPath)
	if err != nil {
		return false, err
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return false, fmt.Errorf("decode %s schema version: %w", option.requestPath, err)
	}
	switch header.SchemaVersion {
	case 1, 2:
		return false, nil
	case 3:
		return true, nil
	default:
		return false, fmt.Errorf("unsupported Pack Source schema version %d", header.SchemaVersion)
	}
}

func bundleDispatch(option options) (packsyncworkflow.BundleDispatchRequest, error) {
	var request packsyncworkflow.BundleDispatchRequest
	if option.requestPath != "" {
		if err := readJSON(option.requestPath, &request); err != nil {
			return request, err
		}
	} else {
		request = packsyncworkflow.BundleDispatchRequest{
			SchemaVersion: 3, Operation: packsyncworkflow.OperationRegisterBundle,
			PackID: os.Getenv("PACKY_PACK_ID"), RegistrationBundleSHA256: os.Getenv("PACKY_REGISTRATION_BUNDLE_SHA256"),
			ProposedVersion: os.Getenv("PACKY_PROPOSED_VERSION"), ProposedManifest: json.RawMessage(os.Getenv("PACKY_PROPOSED_MANIFEST_JSON")),
			ProposedManifestSHA256: os.Getenv("PACKY_PROPOSED_MANIFEST_SHA256"),
			ClassificationMode:     packsyncworkflow.ClassificationMode(os.Getenv("PACKY_CLASSIFICATION_MODE")),
			RequestReason:          os.Getenv("PACKY_REQUEST_REASON"),
			ExpectedPlanID:         os.Getenv("PACKY_EXPECTED_PLAN_ID"),
			ExpectedBaseSHA:        os.Getenv("PACKY_EXPECTED_BASE_SHA"),
		}
		if raw := os.Getenv("PACKY_HUMAN_EVIDENCE_JSON"); raw != "" {
			request.HumanEvidence = json.RawMessage(raw)
		}
		decoder := json.NewDecoder(strings.NewReader(os.Getenv("PACKY_REGISTRATIONS_JSON")))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request.Registrations); err != nil {
			return request, fmt.Errorf("decode PACKY_REGISTRATIONS_JSON: %w", err)
		}
	}
	if canonical, err := packsync.CanonicalCompositePackManifest(request.ProposedManifest); err == nil {
		request.ProposedManifest = canonical
	}
	if err := request.Validate(); err != nil {
		return request, err
	}
	return request, nil
}

func bundleGeneration(request packsyncworkflow.BundleDispatchRequest) (json.RawMessage, string) {
	return append(json.RawMessage(nil), request.ProposedManifest...), request.ProposedVersion
}

func inspectBundle(ctx context.Context, option options, output io.Writer) error {
	request, err := bundleDispatch(option)
	if err != nil {
		return err
	}
	manifest, version := bundleGeneration(request)
	acquisition, err := os.MkdirTemp("", "packy-bundle-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(acquisition)
	plan, err := (packsync.Engine{Source: workflowSourceFactory(), Validate: workflowValidatorFactory()}).CheckComposite(ctx, packsync.CompositeCheckRequest{
		RepositoryRoot: option.repositoryRoot, AcquisitionDir: acquisition, PackID: request.PackID,
		ProposedVersion: version, ProposedManifest: manifest, Members: request.Registrations,
	})
	if err != nil {
		return err
	}
	if plan.Status == "no-op" {
		return errors.New("register_bundle cannot produce a no-op plan")
	}
	if option.outputDir == "" {
		return errors.New("register_bundle Inspect requires an artifact output directory")
	}
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	if err := writeCanonical(filepath.Join(option.outputDir, "request.json"), request); err != nil {
		return err
	}
	if err := writeCanonical(filepath.Join(option.outputDir, "plan.json"), plan); err != nil {
		return err
	}
	artifact := packsyncworkflow.BundleInspectionArtifact{SchemaVersion: 3, BundleArtifactIdentity: bundleIdentity(plan)}
	if err := artifact.Validate(); err != nil {
		return err
	}
	if err := writeCanonical(filepath.Join(option.outputDir, "inspection.json"), artifact); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, filepath.Join(option.outputDir, "plan.json"))
	return err
}

func bundleIdentity(plan packsync.CompositePlan) packsyncworkflow.BundleArtifactIdentity {
	identity := packsyncworkflow.BundleArtifactIdentity{
		PackID: plan.PackID, SourceIDs: append([]string(nil), plan.SourceIDs...),
		RegistrationBundleSHA256: plan.RegistrationBundleSHA256, PlanID: plan.PlanID,
		ProposedVersion: plan.ProposedVersion, ProposedManifestSHA256: plan.ProposedManifestSHA256,
		BaseSHA: plan.Preconditions.BaseCommit, ConfigSHA256: plan.ResultingConfigSHA256,
		ManifestsSHA256: plan.Preconditions.ManifestsSHA256, LockSetSHA256: plan.LockSetSHA256,
		ResultBundleSHA256: plan.ResultBundleSHA256,
	}
	for _, member := range plan.Members {
		identity.Members = append(identity.Members, packsyncworkflow.BundleArtifactMember{
			SourceID: member.SourceID, CandidateSHA: member.Candidate.Commit, SourceLockSHA256: member.SourceLockSHA256,
			LegalEvidenceRef: member.LegalAdmission.EvidenceReference, LegalEvidenceSHA256: member.LegalAdmission.EvidenceSHA256,
		})
	}
	return identity
}

func readBundleInputs(option options) (packsyncworkflow.BundleDispatchRequest, packsync.CompositePlan, error) {
	request, err := bundleDispatch(option)
	if err != nil {
		return request, packsync.CompositePlan{}, err
	}
	var plan packsync.CompositePlan
	if err := readJSON(option.planPath, &plan); err != nil {
		return request, plan, err
	}
	if canonical, err := packsync.CanonicalCompositePackManifest(plan.ProposedManifest); err == nil {
		plan.ProposedManifest = canonical
	}
	if !plan.VerifySeal() || plan.PackID != request.PackID || plan.RegistrationBundleSHA256 != request.RegistrationBundleSHA256 ||
		plan.ProposedVersion != request.ProposedVersion || plan.ProposedManifestSHA256 != request.ProposedManifestSHA256 {
		return request, plan, errors.New("v3 dispatch and sealed composite plan identity contradict")
	}
	return request, plan, nil
}

func classifyBundle(ctx context.Context, option options, output io.Writer) error {
	if option.requestPath == "" || option.planPath == "" || option.outputDir == "" {
		return errors.New("register_bundle Classify requires request, plan, and output paths")
	}
	request, plan, err := readBundleInputs(option)
	if err != nil {
		return err
	}
	var set packsync.CompositeClassificationEvidence
	switch request.ClassificationMode {
	case packsyncworkflow.ClassificationAI:
		classificationRequest := packclassification.Request{
			SchemaVersion: 1, RequestID: plan.PlanID + "/" + plan.PackID, Mode: packclassification.ModeAI,
			PlanID: plan.PlanID, BaseSHA: plan.Preconditions.BaseCommit, PackID: plan.PackID,
			CurrentVersion: "0.0.0", MechanicalFloor: packsync.LevelMajor, SemanticEvidenceRequired: true,
			MechanicalReasons: []string{"initial complete composite Pack generation"},
		}
		evidence, modelErr := bundleClassificationAttempt(ctx, classificationRequest)
		if modelErr != nil {
			return classificationFailure(modelErr)
		}
		set = packsync.CompositeClassificationEvidence{SchemaVersion: 1, PlanID: plan.PlanID, PackID: plan.PackID, Evidence: evidence}
	case packsyncworkflow.ClassificationHuman:
		if request.ExpectedPlanID != plan.PlanID || request.ExpectedBaseSHA != plan.Preconditions.BaseCommit {
			return classificationFailure(errors.New("human composite evidence request is stale against the sealed plan"))
		}
		if err := json.Unmarshal(request.HumanEvidence, &set); err != nil {
			return classificationFailure(fmt.Errorf("decode human composite evidence: %w", err))
		}
	}
	if err := packsync.ValidateCompositeClassificationEvidence(plan, set); err != nil {
		return classificationFailure(err)
	}
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	evidencePath := filepath.Join(option.outputDir, "classification-evidence.json")
	if err := writeCanonical(evidencePath, set); err != nil {
		return err
	}
	evidenceBytes, err := os.ReadFile(evidencePath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(evidenceBytes)
	artifact := packsyncworkflow.BundleClassificationArtifact{SchemaVersion: 3, BundleArtifactIdentity: bundleIdentity(plan), ClassificationSHA256: hex.EncodeToString(digest[:])}
	if err := artifact.Validate(); err != nil {
		return err
	}
	name := filepath.Join(option.outputDir, "classification.json")
	if err := writeCanonical(name, artifact); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, name)
	return err
}

func readBundleEvidence(path string) (packsync.CompositeClassificationEvidence, error) {
	var evidence packsync.CompositeClassificationEvidence
	if err := readJSON(path, &evidence); err != nil {
		return evidence, err
	}
	var artifact packsyncworkflow.BundleClassificationArtifact
	artifactPath := filepath.Join(filepath.Dir(path), "classification.json")
	if err := readJSON(artifactPath, &artifact); err != nil {
		return evidence, errors.New("v3 classification artifact is missing")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return evidence, err
	}
	sum := sha256.Sum256(data)
	if err := artifact.Validate(); err != nil || artifact.ClassificationSHA256 != hex.EncodeToString(sum[:]) {
		return evidence, errors.New("v3 classification evidence digest is stale or tampered")
	}
	return evidence, nil
}

func matchBundleClassification(path string, identity packsyncworkflow.BundleArtifactIdentity) error {
	var artifact packsyncworkflow.BundleClassificationArtifact
	if err := readJSON(filepath.Join(filepath.Dir(path), "classification.json"), &artifact); err != nil {
		return err
	}
	return artifact.BundleArtifactIdentity.Matches(identity)
}

func applyBundle(ctx context.Context, option options) (packsync.CompositePlan, packsyncworkflow.BundleArtifactIdentity, error) {
	request, plan, err := readBundleInputs(option)
	if err != nil {
		return plan, packsyncworkflow.BundleArtifactIdentity{}, err
	}
	evidence, err := readBundleEvidence(option.evidencePath)
	if err != nil {
		return plan, packsyncworkflow.BundleArtifactIdentity{}, err
	}
	if err := matchBundleClassification(option.evidencePath, bundleIdentity(plan)); err != nil {
		return plan, packsyncworkflow.BundleArtifactIdentity{}, errors.New("v3 classification artifact is stale or mixed")
	}
	manifest, version := bundleGeneration(request)
	acquisition, err := os.MkdirTemp("", "packy-bundle-apply-")
	if err != nil {
		return plan, packsyncworkflow.BundleArtifactIdentity{}, err
	}
	defer os.RemoveAll(acquisition)
	engine := packsync.Engine{Source: workflowSourceFactory(), Validate: workflowValidatorFactory()}
	_, err = engine.ApplyComposite(ctx, packsync.CompositeApplyRequest{
		CompositeCheckRequest: packsync.CompositeCheckRequest{
			RepositoryRoot: option.repositoryRoot, AcquisitionDir: acquisition, PackID: request.PackID,
			ProposedVersion: version, ProposedManifest: manifest, Members: request.Registrations,
		},
		Plan: plan, ClassificationEvidence: evidence,
	})
	return plan, bundleIdentity(plan), err
}

func validateBundle(ctx context.Context, option options, output io.Writer) error {
	if option.requestPath == "" || option.planPath == "" || option.evidencePath == "" || option.outputDir == "" {
		return errors.New("register_bundle Validate requires request, plan, evidence, and output paths")
	}
	_, identity, err := applyBundle(ctx, option)
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
	tree, err := gitResultTree(ctx, option.repositoryRoot)
	if err != nil {
		return err
	}
	gates := completeBundleGates()
	artifact := packsyncworkflow.BundleValidationArtifact{SchemaVersion: 3, BundleArtifactIdentity: identity, ResultTreeSHA: tree, Validation: gates}
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

func gitResultTree(ctx context.Context, repository string) (string, error) {
	if err := stageAll(ctx, repository); err != nil {
		return "", err
	}
	tree, err := command(ctx, repository, "git", "write-tree")
	return strings.TrimSpace(tree), err
}

func completeBundleGates() packsyncworkflow.ValidationGates {
	return packsyncworkflow.ValidationGates{Provenance: true, Classification: true, Reacquisition: true, Apply: true, Diff: true, Ownership: true, PackySuite: true}
}

func writeBundleFailureArtifact(option options, failure error) error {
	request, requestErr := bundleDispatch(option)
	if requestErr != nil {
		return requestErr
	}
	identity := packsyncworkflow.BundleFailureIdentity{
		PackID: request.PackID, RegistrationBundleSHA256: request.RegistrationBundleSHA256,
		ProposedVersion: request.ProposedVersion, ProposedManifestSHA256: request.ProposedManifestSHA256,
	}
	for _, member := range request.Registrations {
		identity.SourceIDs = append(identity.SourceIDs, member.Registration.ID)
		identity.Members = append(identity.Members, packsyncworkflow.BundleFailureMember{
			SourceID: member.Registration.ID, CandidateSHA: member.Registration.Selector.Ref,
			LegalEvidenceRef: member.LegalAdmission.EvidenceReference, LegalEvidenceSHA256: member.LegalAdmission.EvidenceSHA256,
		})
	}
	planPath := option.planPath
	if planPath == "" {
		planPath = filepath.Join(option.outputDir, "plan.json")
	}
	var plan packsync.CompositePlan
	if readJSON(planPath, &plan) == nil && plan.VerifySeal() {
		planned := bundleIdentity(plan)
		identity.Plan = &packsyncworkflow.BundleFailurePlanIdentity{
			PlanID: planned.PlanID, BaseSHA: planned.BaseSHA, ConfigSHA256: planned.ConfigSHA256,
			ManifestsSHA256: planned.ManifestsSHA256, LockSetSHA256: planned.LockSetSHA256,
			ResultBundleSHA256: planned.ResultBundleSHA256, Members: planned.Members,
		}
	}
	artifact := packsyncworkflow.BundleFailureArtifact{
		SchemaVersion: 3, State: "blocked", BundleFailureIdentity: identity,
		Blockers: []string{failure.Error()}, Recovery: []string{"Correct the complete bundle and repeat Inspect."},
	}
	if err := artifact.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	return writeCanonical(filepath.Join(option.outputDir, "operational-artifact.json"), artifact)
}

type compositeProvenance struct{ engine packsync.Engine }

func (p compositeProvenance) RevalidateComposite(ctx context.Context, plan packsync.CompositePlan) error {
	return p.engine.RevalidateCompositeCandidates(ctx, plan)
}

type bundleProposalBuilder struct {
	plan                 packsync.CompositePlan
	request              packsyncworkflow.BundleDispatchRequest
	classificationSHA256 string
	gateway              *githubGateway
}

type bundleReviewBrief struct {
	SchemaVersion           int                                     `json:"schema_version"`
	Actor                   string                                  `json:"actor"`
	RunID                   string                                  `json:"run_id"`
	RunAttempt              string                                  `json:"run_attempt"`
	RunURL                  string                                  `json:"run_url"`
	Repository              string                                  `json:"repository"`
	Request                 packsyncworkflow.BundleDispatchRequest  `json:"request"`
	Identity                packsyncworkflow.BundleArtifactIdentity `json:"identity"`
	ClassificationSHA256    string                                  `json:"classification_sha256"`
	HeadSHA                 string                                  `json:"head_sha"`
	ResultTreeSHA           string                                  `json:"result_tree_sha"`
	Branch                  string                                  `json:"branch"`
	PullRequest             int                                     `json:"pull_request,omitempty"`
	SelectedResources       []packsync.ResourceEvidence             `json:"selected_resources"`
	PreviousSnapshotSHA256  string                                  `json:"previous_snapshot_sha256"`
	ProposedSnapshotSHA256  string                                  `json:"proposed_snapshot_sha256"`
	ApplyStatus             string                                  `json:"apply_status"`
	Validation              packsyncworkflow.ValidationGates        `json:"validation"`
	UpstreamContentExecuted bool                                    `json:"upstream_content_executed"`
	Blockers                []string                                `json:"blockers"`
	DecisionReady           bool                                    `json:"decision_ready"`
	AutoMerge               bool                                    `json:"auto_merge"`
	ManualMergeRequired     bool                                    `json:"manual_merge_required"`
	InvalidationConditions  []string                                `json:"invalidation_conditions"`
	Recovery                []string                                `json:"recovery"`
}

func (brief *bundleReviewBrief) PreparePublication(proposal packsyncworkflow.Proposal) {
	brief.ResultTreeSHA = proposal.ResultTreeSHA
	brief.Validation = proposal.Validation
	brief.DecisionReady = false
	brief.Blockers = []string{"Publication remains blocked until the exact post-write pull request identity is reobserved."}
	brief.InvalidationConditions = proposal.InvalidationConditions
}

func (brief *bundleReviewBrief) FinalizePublication(proposal packsyncworkflow.Proposal, observed packsyncworkflow.PRState) {
	brief.PullRequest = observed.Number
	brief.HeadSHA = observed.HeadSHA
	brief.Validation = proposal.Validation
	brief.Blockers = nil
	brief.DecisionReady = true
	brief.InvalidationConditions = proposal.InvalidationConditions
}

func (brief *bundleReviewBrief) Markdown() (string, error) {
	if brief.SchemaVersion != 3 || brief.Request.Validate() != nil || brief.Identity.Validate() != nil ||
		brief.Request.PackID != brief.Identity.PackID ||
		brief.Request.RegistrationBundleSHA256 != brief.Identity.RegistrationBundleSHA256 ||
		brief.Request.ProposedVersion != brief.Identity.ProposedVersion ||
		brief.Request.ProposedManifestSHA256 != brief.Identity.ProposedManifestSHA256 ||
		!validLowerHex(brief.ClassificationSHA256, 64) ||
		!validLowerHex(brief.HeadSHA, 40) || !validLowerHex(brief.ResultTreeSHA, 40) ||
		brief.Branch != "sync/"+brief.Identity.PackID || len(brief.SelectedResources) == 0 ||
		!validLowerHex(brief.PreviousSnapshotSHA256, 64) || !validLowerHex(brief.ProposedSnapshotSHA256, 64) ||
		!brief.Validation.Complete() || brief.UpstreamContentExecuted || brief.AutoMerge || !brief.ManualMergeRequired ||
		brief.RunURL == "" || brief.Repository == "" || brief.RunID == "" {
		return "", errors.New("v3 bundle review brief is incomplete or contradictory")
	}
	canonical, err := json.MarshalIndent(brief, "", "  ")
	if err != nil {
		return "", err
	}
	canonical = append(canonical, '\n')
	status := "blocked"
	if brief.DecisionReady {
		status = "decision-ready"
	}
	return fmt.Sprintf("## Packy composite Pack registration\n\n- Pack: `%s`\n- Members: `%s`\n- Plan: `%s`\n- Base/head/tree: `%s` / `%s` / `%s`\n- State: **%s**\n- Auto-merge: disabled; manual merge required.\n\nAuthorization-Exception: automation\nAuthorization-Record: %s\n\n<details><summary>Canonical v3 composite admission evidence</summary>\n\n```json\n%s```\n</details>\n", brief.Identity.PackID, strings.Join(brief.Identity.SourceIDs, ", "), brief.Identity.PlanID, brief.Identity.BaseSHA, brief.HeadSHA, brief.ResultTreeSHA, status, brief.RunURL, string(canonical)), nil
}

func validLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func compositeCandidateSHA(plan packsync.CompositePlan) string {
	var sealed strings.Builder
	sealed.WriteString(plan.RegistrationBundleSHA256)
	for _, member := range plan.Members {
		sealed.WriteByte(0)
		sealed.WriteString(member.SourceID)
		sealed.WriteByte(0)
		sealed.WriteString(member.Candidate.Commit)
	}
	sum := sha256.Sum256([]byte(sealed.String()))
	return hex.EncodeToString(sum[:])[:40]
}

func (b *bundleProposalBuilder) Build(ctx context.Context, root string, result packsync.ApplyResult) (packsyncworkflow.Proposal, error) {
	if err := stageAll(ctx, root); err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	if _, err := command(ctx, root, "git", "config", "user.name", packsyncworkflow.AutomationOwner); err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	if _, err := command(ctx, root, "git", "config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com"); err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	completeCandidate := compositeCandidateSHA(b.plan)
	if _, err := command(ctx, root, "git", "commit", "-m", fmt.Sprintf("sync(%s): %s [%s]", b.plan.PackID, completeCandidate[:12], b.plan.PlanID)); err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	head, err := command(ctx, root, "git", "rev-parse", "HEAD")
	if err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	selected := []packsync.ResourceEvidence{}
	for _, member := range b.plan.Members {
		selected = append(selected, member.ProposedLock.Resources...)
	}
	tree, err := command(ctx, root, "git", "write-tree")
	if err != nil {
		return packsyncworkflow.Proposal{}, err
	}
	b.gateway.brief = &bundleReviewBrief{
		SchemaVersion: 3, Actor: os.Getenv("GITHUB_ACTOR"), RunID: os.Getenv("GITHUB_RUN_ID"),
		RunAttempt: os.Getenv("GITHUB_RUN_ATTEMPT"), RunURL: actionsRunURL(), Repository: os.Getenv("GITHUB_REPOSITORY"),
		Request: b.request, Identity: bundleIdentity(b.plan), ClassificationSHA256: b.classificationSHA256,
		HeadSHA: strings.TrimSpace(head), ResultTreeSHA: strings.TrimSpace(tree), Branch: "sync/" + b.plan.PackID,
		SelectedResources: selected, PreviousSnapshotSHA256: b.plan.Preconditions.BundleSHA256,
		ProposedSnapshotSHA256: b.plan.ResultBundleSHA256, ApplyStatus: result.Status, Validation: completeBundleGates(),
		UpstreamContentExecuted: false, Blockers: []string{"Publication remains blocked until exact reobservation."},
		AutoMerge: false, ManualMergeRequired: true, Recovery: []string{"Repeat Inspect when any sealed bundle fact changes."},
	}
	title := fmt.Sprintf("sync(%s): composite registration", b.plan.PackID)
	proposal := packsyncworkflow.Proposal{SourceID: b.plan.PackID, PlanID: b.plan.PlanID, BaseSHA: b.plan.Preconditions.BaseCommit, CandidateSHA: completeCandidate, ResultTreeSHA: strings.TrimSpace(tree), HeadSHA: strings.TrimSpace(head), ProvenanceSHA256: b.plan.RegistrationBundleSHA256, ManagedTitle: title}
	b.gateway.title = title
	return proposal, nil
}

func publishBundle(ctx context.Context, option options, output io.Writer) error {
	if option.requestPath == "" || option.planPath == "" || option.evidencePath == "" || option.validationPath == "" || option.outputDir == "" {
		return errors.New("register_bundle Publish requires request, plan, evidence, validation proof, and output paths")
	}
	request, plan, err := readBundleInputs(option)
	if err != nil {
		return err
	}
	var proof packsyncworkflow.BundleValidationArtifact
	if err := readJSON(option.validationPath, &proof); err != nil {
		return err
	}
	if err := proof.Validate(); err != nil {
		return err
	}
	if err := proof.BundleArtifactIdentity.Matches(bundleIdentity(plan)); err != nil {
		return errors.New("v3 validation proof is stale or mixed")
	}
	evidence, err := readBundleEvidence(option.evidencePath)
	if err != nil {
		return err
	}
	if err := matchBundleClassification(option.evidencePath, bundleIdentity(plan)); err != nil {
		return errors.New("v3 classification artifact is stale or mixed")
	}
	var classification packsyncworkflow.BundleClassificationArtifact
	if err := readJSON(filepath.Join(filepath.Dir(option.evidencePath), "classification.json"), &classification); err != nil {
		return err
	}
	manifest, version := bundleGeneration(request)
	acquisition, err := os.MkdirTemp("", "packy-bundle-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(acquisition)
	validator := workflowValidatorFactory()
	engine := packsync.Engine{Source: workflowSourceFactory(), Validate: validator}
	apply := packsync.CompositeApplyRequest{CompositeCheckRequest: packsync.CompositeCheckRequest{RepositoryRoot: option.repositoryRoot, AcquisitionDir: acquisition, PackID: request.PackID, ProposedVersion: version, ProposedManifest: manifest, Members: request.Registrations}, Plan: plan, ClassificationEvidence: evidence}
	completeCandidate := compositeCandidateSHA(plan)
	gateway := workflowGatewayFactory(option.repositoryRoot, packsync.Plan{SourceID: plan.PackID, Candidate: packsync.Candidate{Commit: completeCandidate}, Preconditions: plan.Preconditions})
	gateway.candidateRelation = func(record string) packsyncworkflow.CandidateRelation {
		if record == "" || record == completeCandidate {
			return packsyncworkflow.CandidateSame
		}
		return packsyncworkflow.CandidateRegressive
	}
	builder := &bundleProposalBuilder{plan: plan, request: request, classificationSHA256: classification.ClassificationSHA256, gateway: gateway}
	publisher := packsyncworkflow.BundlePublisher{Applier: engine, Validator: validator, Builder: builder, Diff: gitDiffVerifier{}, Provenance: compositeProvenance{engine: engine}, GitHub: gateway}
	result, err := publisher.Run(ctx, packsyncworkflow.BundlePublishRequest{
		RepositoryRoot:        option.repositoryRoot,
		Apply:                 apply,
		ExpectedResultTreeSHA: proof.ResultTreeSHA,
	})
	if err != nil {
		return err
	}
	artifact := packsyncworkflow.BundlePublicationArtifact{
		SchemaVersion: 3, BundleArtifactIdentity: bundleIdentity(plan),
		HeadSHA: result.Proposal.HeadSHA, ResultTreeSHA: result.Proposal.ResultTreeSHA,
		BranchName: result.Decision.Branch, PRNumber: result.PullRequest.Number, PRStateSHA256: result.PullRequest.MetadataHash,
		ProvenanceSHA256: result.Proposal.ProvenanceSHA256, ManagedTitle: result.Proposal.ManagedTitle,
		ManagedMetadataHash: result.PullRequest.MetadataHash, Validation: result.Readiness.Gates,
		DecisionReady: result.Readiness.DecisionReady, AutoMerge: false, ManualMergeRequired: true,
		UpstreamContentExecuted: false, InvalidationConditions: result.Proposal.InvalidationConditions,
	}
	if err := artifact.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(option.outputDir, 0o755); err != nil {
		return err
	}
	name := filepath.Join(option.outputDir, "publication.json")
	if err := writeCanonical(name, artifact); err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, name)
	return err
}
