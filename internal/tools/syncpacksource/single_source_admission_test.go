package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/packclassification"
	"github.com/yersonargotev/packy/internal/packsync"
	"github.com/yersonargotev/packy/internal/packsyncworkflow"
)

func TestInspectRoutesV23RegisterToSingleSourceAdmission(t *testing.T) {
	repository := t.TempDir()
	existingBinding := packsync.Binding{PackID: "existing", Kind: "skill", ResourceID: "existing", UpstreamPath: "existing", VendoredPath: "bundle/skills/existing"}
	writeToolFile(t, filepath.Join(repository, "bundle", "sources.json"), `{"schema_version":1,"sources":[{"id":"existing-source","provider":"github","repository":"example/existing","selector":{"mode":"stable-release"},"resources":[{"pack_id":"existing","kind":"skill","resource_id":"existing","upstream_path":"existing"}]}]}`)
	writeToolFile(t, filepath.Join(repository, "bundle", "packs", "existing", "pack.json"), `{"schema_version":1,"id":"existing","version":"1.0.0","resources":[{"kind":"skill","id":"existing","source":"skills/existing"}]}`)
	writeToolFile(t, filepath.Join(repository, "bundle", "skills", "existing", "SKILL.md"), "existing\n")
	existingLock := packsync.Lock{SchemaVersion: 1, SourceID: "existing-source", Resources: []packsync.ResourceEvidence{{Binding: existingBinding, Files: []packsync.FileEvidence{}}}}
	lockBytes, _, err := packsync.CanonicalSourceLock(existingLock)
	if err != nil {
		t.Fatal(err)
	}
	writeToolFile(t, filepath.Join(repository, "bundle", "sources", "existing-source.lock.json"), string(lockBytes))

	snapshot := t.TempDir()
	writeToolFile(t, filepath.Join(snapshot, "skill", "SKILL.md"), "# Coordinate\n")
	writeToolFile(t, filepath.Join(snapshot, "LICENSE"), "MIT\n")
	candidate := toolCandidate()
	evidenceReference := "docs/evidence/admission.json"
	evidence := toolLegalEvidence(t, evidenceReference, candidate)
	writeToolFile(t, filepath.Join(repository, filepath.FromSlash(evidenceReference)), string(evidence))
	registration := packsync.SourceConfig{
		ID: "new-source", Provider: "github", Repository: candidate.Repository,
		Selector: packsync.Selector{Mode: packsync.SelectorStableRelease},
		Resources: []packsync.Binding{
			{PackID: "new-pack", Kind: "notice", ResourceID: "mit", UpstreamPath: "LICENSE"},
			{PackID: "new-pack", Kind: "skill", ResourceID: "coordinate", UpstreamPath: "skill"},
		},
	}
	registrationDigest, err := packsyncworkflow.CanonicalRegistrationSHA256(registration)
	if err != nil {
		t.Fatal(err)
	}
	manifest, manifestDigest, err := packsyncworkflow.CanonicalProposedManifest(json.RawMessage(`{"id":"new-pack","version":"1.0.0","description":"Coordinate work","selectable":true,"surfaces":["codex"],"external_requirements":[],"resources":[{"kind":"notice","id":"mit","source":"notices/mit","license":"MIT","attribution":"Example","requires":[],"conflicts":[],"bindings":[],"surface_exclusions":[]},{"kind":"skill","id":"coordinate","source":"skills/coordinate","requires":[],"conflicts":[],"notices":["notice:mit"],"bindings":[{"surface":"codex","projection":"skill","name":"coordinate","invocation":"$coordinate","mode":"native","sharing":"exclusive"}],"surface_exclusions":[]}],"exclusions":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	request := packsyncworkflow.DispatchRequest{
		SchemaVersion: 2, Operation: packsyncworkflow.OperationRegister, SourceID: registration.ID,
		Selector: packsyncworkflow.SelectorLatestStable, ClassificationMode: packsyncworkflow.ClassificationAI, RequestReason: "admit exact source",
		Registration: &registration, RegistrationSHA256: registrationDigest,
		ProposedVersion: "1.0.0", ProposedManifest: manifest, ProposedManifestSHA256: manifestDigest,
		LegalAdmission: &packsync.CompositeLegalAdmission{EvidenceReference: evidenceReference, EvidenceSHA256: fmt.Sprintf("%x", sha256.Sum256(evidence)), Disposition: packsync.RedistributableDisposition},
		ClosingIssue:   "https://github.com/example/packy/issues/544",
	}
	initializeToolRepository(t, repository)
	requestPath := filepath.Join(t.TempDir(), "request.json")
	requestBytes, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeToolFile(t, requestPath, string(requestBytes)+"\n")
	outputDir := t.TempDir()

	previousSource, previousValidator := workflowSourceFactory, workflowValidatorFactory
	workflowSourceFactory = func() packsync.Source { return &toolSource{root: snapshot, candidate: candidate} }
	workflowValidatorFactory = func() phaseValidator { return toolValidator{} }
	t.Cleanup(func() {
		workflowSourceFactory, workflowValidatorFactory = previousSource, previousValidator
	})
	missingIssueRequest := request
	missingIssueRequest.ClosingIssue = ""
	missingIssuePath := filepath.Join(t.TempDir(), "missing-issue-request.json")
	missingIssueBytes, err := json.MarshalIndent(missingIssueRequest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeToolFile(t, missingIssuePath, string(missingIssueBytes)+"\n")
	missingIssueOutput := t.TempDir()
	if err := run(context.Background(), []string{"--phase", "inspect", "--repository-root", repository, "--request", missingIssuePath, "--output", missingIssueOutput}, io.Discard); err == nil {
		t.Fatal("single-source admission without a closing issue reached inspection")
	}
	if _, err := os.Stat(filepath.Join(missingIssueOutput, "plan.json")); !os.IsNotExist(err) {
		t.Fatalf("issue-less admission wrote a plan: %v", err)
	}
	if err := run(context.Background(), []string{"--phase", "inspect", "--repository-root", repository, "--request", requestPath, "--output", outputDir}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var plan packsync.SingleSourceAdmissionPlan
	if err := readJSON(filepath.Join(outputDir, "plan.json"), &plan); err != nil || !plan.VerifySeal() || plan.PackID != "new-pack" {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	if _, err := os.Stat(filepath.Join(repository, "bundle", "packs", "new-pack")); !os.IsNotExist(err) {
		t.Fatalf("inspect wrote proposed Pack: %v", err)
	}
	changedIssueRequest := request
	changedIssueRequest.ClosingIssue = "https://github.com/example/packy/issues/545"
	changedIssuePath := filepath.Join(t.TempDir(), "changed-issue-request.json")
	changedIssueBytes, err := json.MarshalIndent(changedIssueRequest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeToolFile(t, changedIssuePath, string(changedIssueBytes)+"\n")
	if _, _, err := readSingleSourceAdmissionInputs(options{requestPath: changedIssuePath, planPath: filepath.Join(outputDir, "plan.json")}); err == nil {
		t.Fatal("later phase replaced the closing issue preserved by Inspect")
	}
	writeToolFile(t, filepath.Join(outputDir, "request.json"), string(changedIssueBytes)+"\n")
	if _, _, err := readSingleSourceAdmissionInputs(options{requestPath: changedIssuePath, planPath: filepath.Join(outputDir, "plan.json")}); err == nil {
		t.Fatal("later phase replaced both the supplied and preserved closing issue")
	}
	writeToolFile(t, filepath.Join(outputDir, "request.json"), string(requestBytes)+"\n")
	previousClassificationAttempt := bundleClassificationAttempt
	bundleClassificationAttempt = func(_ context.Context, classificationRequest packclassification.Request) (packsync.ClassificationEvidence, error) {
		return packsync.ClassificationEvidence{
			PackID: classificationRequest.PackID, Classifier: packsync.ClassifierIdentity{Type: packsync.ClassifierAI, ID: "fixture"}, Rationale: "initial single-source Pack admission",
			CurrentVersion: classificationRequest.CurrentVersion, ProposedVersion: plan.ProposedVersion, ChangedAspects: []string{"initial complete Pack generation"},
			MechanicalFloor: classificationRequest.MechanicalFloor, FinalLevel: packsync.LevelMajor, Migration: "initial generation has no predecessor",
			RequiredActions: []string{"review initial complete Pack contract"},
		}, nil
	}
	t.Cleanup(func() { bundleClassificationAttempt = previousClassificationAttempt })
	classificationDir := t.TempDir()
	if err := run(context.Background(), []string{"--phase", "classify", "--repository-root", repository, "--request", requestPath, "--plan", filepath.Join(outputDir, "plan.json"), "--output", classificationDir}, io.Discard); err != nil {
		t.Fatal(err)
	}
	classificationPath := filepath.Join(classificationDir, "classification.json")
	validationRepository := t.TempDir()
	if err := copyToolTree(repository, validationRepository); err != nil {
		t.Fatal(err)
	}
	validationDir := t.TempDir()
	if err := run(context.Background(), []string{"--phase", "validate", "--repository-root", validationRepository, "--request", requestPath, "--plan", filepath.Join(outputDir, "plan.json"), "--evidence", classificationPath, "--output", validationDir}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var validation packsyncworkflow.ValidationArtifact
	if err := readJSON(filepath.Join(validationDir, "validation.json"), &validation); err != nil || validation.Validate() != nil {
		t.Fatalf("validation=%#v err=%v", validation, err)
	}
	if validation.PackID != plan.PackID || validation.ResultBundleSHA256 != plan.ResultBundleSHA256 || validation.ResultTreeSHA == "" {
		t.Fatalf("validation lost complete admission identity: %#v", validation)
	}
	if validation.ClosingIssue != request.ClosingIssue {
		t.Fatalf("validation closing issue = %q, want %q", validation.ClosingIssue, request.ClosingIssue)
	}
	for _, path := range []string{
		filepath.Join(validationRepository, "bundle", "sources", "new-source.lock.json"),
		filepath.Join(validationRepository, "bundle", "skills", "coordinate", "SKILL.md"),
		filepath.Join(validationRepository, "bundle", "notices", "mit"),
		filepath.Join(validationRepository, "bundle", "packs", "new-pack", "pack.json"),
		filepath.Join(validationRepository, "bundle", "history", "new-pack", "1.0.0", "artifact.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("validated complete generation path %s: %v", path, err)
		}
	}
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_REPOSITORY", "example/packy")
	t.Setenv("GITHUB_RUN_ID", "543")
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	t.Setenv("GITHUB_ACTOR", "github-actions[bot]")
	publicationBase := t.TempDir()
	if err := copyToolTree(repository, publicationBase); err != nil {
		t.Fatal(err)
	}
	previousAdmissionGateway := singleSourceAdmissionGatewayFactory
	fakeGateway := &fakeSingleSourceAdmissionGateway{}
	singleSourceAdmissionGatewayFactory = func(string, packsync.Plan) singleSourceAdmissionGateway { return fakeGateway }
	t.Cleanup(func() { singleSourceAdmissionGatewayFactory = previousAdmissionGateway })
	publicationRepository := t.TempDir()
	if err := copyToolTree(publicationBase, publicationRepository); err != nil {
		t.Fatal(err)
	}
	publicationDir := t.TempDir()
	if err := run(context.Background(), []string{"--phase", "publish", "--repository-root", publicationRepository, "--request", requestPath, "--plan", filepath.Join(outputDir, "plan.json"), "--evidence", classificationPath, "--output", publicationDir}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var publication packsyncworkflow.PublicationArtifact
	if err := readJSON(filepath.Join(publicationDir, "publication.json"), &publication); err != nil || publication.Validate() != nil {
		t.Fatalf("publication=%#v err=%v", publication, err)
	}
	if !publication.DecisionReady || publication.PackID != plan.PackID || publication.ResultBundleSHA256 != plan.ResultBundleSHA256 ||
		publication.ClosingIssue != request.ClosingIssue || fakeGateway.publishCalls != 1 || fakeGateway.finalizeCalls != 1 {
		t.Fatalf("publication lost complete decision-ready admission identity: %#v publish=%d finalize=%d", publication, fakeGateway.publishCalls, fakeGateway.finalizeCalls)
	}
	var brief packsyncworkflow.ReviewBrief
	if err := readJSON(filepath.Join(publicationDir, "proposal-brief.json"), &brief); err != nil {
		t.Fatal(err)
	}
	managed, err := brief.ManagedMarkdown()
	if err != nil || brief.Request.ClosingIssue != request.ClosingIssue || !strings.Contains(managed, "Closes "+request.ClosingIssue) {
		t.Fatalf("managed proposal lost closing issue: request=%q err=%v\n%s", brief.Request.ClosingIssue, err, managed)
	}
	for _, mode := range []string{"unapproved-issue", "stale-base", "foreign-owner"} {
		t.Run(mode, func(t *testing.T) {
			rejectedRepository := t.TempDir()
			if err := copyToolTree(publicationBase, rejectedRepository); err != nil {
				t.Fatal(err)
			}
			rejectedGateway := &fakeSingleSourceAdmissionGateway{rejection: mode}
			singleSourceAdmissionGatewayFactory = func(string, packsync.Plan) singleSourceAdmissionGateway { return rejectedGateway }
			rejectedOutput := t.TempDir()
			err := run(context.Background(), []string{"--phase", "publish", "--repository-root", rejectedRepository, "--request", requestPath, "--plan", filepath.Join(outputDir, "plan.json"), "--evidence", classificationPath, "--output", rejectedOutput}, io.Discard)
			if err == nil || rejectedGateway.publishCalls != 0 || rejectedGateway.finalizeCalls != 0 {
				t.Fatalf("%s publication was not rejected before GitHub writes: publish=%d finalize=%d err=%v", mode, rejectedGateway.publishCalls, rejectedGateway.finalizeCalls, err)
			}
			var failure packsyncworkflow.FailureArtifact
			if err := readJSON(filepath.Join(rejectedOutput, "operational-artifact.json"), &failure); err != nil {
				t.Fatal(err)
			}
			if failure.ClosingIssue != request.ClosingIssue || failure.ContainsSecrets || failure.ContainsUpstreamBytes || len(failure.Blockers) == 0 || len(failure.Recovery) == 0 {
				t.Fatalf("%s failure artifact is not safe and retryable: %#v", mode, failure)
			}
		})
	}
	singleSourceAdmissionGatewayFactory = func(string, packsync.Plan) singleSourceAdmissionGateway { return fakeGateway }
	if err := writeFailureArtifact(options{repositoryRoot: repository, requestPath: requestPath, planPath: filepath.Join(outputDir, "plan.json"), outputDir: outputDir}, fmt.Errorf("later phase blocked")); err != nil {
		t.Fatal(err)
	}
	var artifact packsyncworkflow.FailureArtifact
	if err := readJSON(filepath.Join(outputDir, "operational-artifact.json"), &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.PlanID != plan.PlanID || artifact.PackID != plan.PackID || artifact.ProposedVersion != plan.ProposedVersion ||
		artifact.ProposedManifestSHA256 != plan.ProposedManifestSHA256 || artifact.LegalEvidenceReference != plan.LegalAdmission.EvidenceReference ||
		artifact.LegalEvidenceSHA256 != plan.LegalAdmission.EvidenceSHA256 || artifact.ResultBundleSHA256 != plan.ResultBundleSHA256 ||
		artifact.ClosingIssue != request.ClosingIssue || artifact.ContainsSecrets || artifact.ContainsUpstreamBytes || len(artifact.Recovery) == 0 {
		t.Fatalf("failure artifact lost initial-admission identity: %#v", artifact)
	}
}

type fakeSingleSourceAdmissionGateway struct {
	proposal      packsyncworkflow.Proposal
	brief         *packsyncworkflow.ReviewBrief
	publishCalls  int
	finalizeCalls int
	published     bool
	finalized     bool
	rejection     string
}

func (fake *fakeSingleSourceAdmissionGateway) configureSingleSourceAdmission(_ string, brief *packsyncworkflow.ReviewBrief) {
	fake.brief = brief
}

func (fake *fakeSingleSourceAdmissionGateway) singleSourceAdmissionReviewBrief() *packsyncworkflow.ReviewBrief {
	return fake.brief
}

func (fake *fakeSingleSourceAdmissionGateway) Prepare(proposal packsyncworkflow.Proposal) (packsyncworkflow.Proposal, error) {
	proposal.ManagedMetadataHash = strings.Repeat("6", 64)
	fake.proposal = proposal
	return proposal, nil
}

func (fake *fakeSingleSourceAdmissionGateway) Observe(context.Context, string) (packsyncworkflow.PublicationState, error) {
	proposal := fake.proposal
	state := packsyncworkflow.PublicationState{
		BaseSHA: proposal.BaseSHA, ProvenanceCurrent: true, CandidateRelation: packsyncworkflow.CandidateSame,
		ClosingIssue: proposal.ClosingIssue, IssueApproved: proposal.ClosingIssue != "",
	}
	if !fake.published {
		switch fake.rejection {
		case "unapproved-issue":
			state.IssueApproved = false
		case "stale-base":
			state.BaseSHA = strings.Repeat("8", 40)
		case "foreign-owner":
			state.Branch = packsyncworkflow.BranchState{Exists: true, Name: "sync/" + proposal.SourceID, HeadSHA: proposal.HeadSHA, Owner: "reviewer"}
			state.PR = packsyncworkflow.PRState{Exists: true, Number: 7, Open: true, BaseBranch: "main", HeadBranch: "sync/" + proposal.SourceID, HeadSHA: proposal.HeadSHA, Owner: "reviewer"}
		}
		return state, nil
	}
	metadataHash := strings.Repeat("6", 64)
	draft := true
	if fake.finalized {
		metadataHash = strings.Repeat("7", 64)
		draft = false
	}
	state.Branch = packsyncworkflow.BranchState{Exists: true, Name: "sync/" + proposal.SourceID, HeadSHA: proposal.HeadSHA, Owner: packsyncworkflow.AutomationOwner, ManagedMetadataHash: metadataHash}
	state.PR = packsyncworkflow.PRState{Exists: true, Number: 7, Open: true, BaseBranch: "main", HeadBranch: "sync/" + proposal.SourceID, HeadSHA: proposal.HeadSHA, MetadataHash: metadataHash, Owner: packsyncworkflow.AutomationOwner, Draft: draft}
	state.Record = packsyncworkflow.PublicationRecord{PlanID: proposal.PlanID, BaseSHA: proposal.BaseSHA, CandidateSHA: proposal.CandidateSHA, HeadSHA: proposal.HeadSHA, ResultTreeSHA: proposal.ResultTreeSHA, ProvenanceSHA256: proposal.ProvenanceSHA256, MetadataHash: metadataHash, ClosingIssue: proposal.ClosingIssue}
	return state, nil
}

func (fake *fakeSingleSourceAdmissionGateway) Publish(context.Context, packsyncworkflow.Proposal, packsyncworkflow.PublicationDecision) (packsyncworkflow.PRState, error) {
	fake.publishCalls++
	fake.published = true
	return packsyncworkflow.PRState{Number: 7}, nil
}

func (fake *fakeSingleSourceAdmissionGateway) Finalize(context.Context, packsyncworkflow.Proposal, packsyncworkflow.PublicationDecision, packsyncworkflow.PRState) (string, error) {
	fake.finalizeCalls++
	fake.finalized = true
	return strings.Repeat("7", 64), nil
}

func initializeToolRepository(t *testing.T, repository string) {
	t.Helper()
	commands := [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Fixture"},
		{"config", "user.email", "fixture@example.com"},
		{"add", "."},
		{"commit", "-m", "fixture"},
	}
	for _, arguments := range commands {
		command := exec.Command("git", arguments...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", arguments, err, output)
		}
	}
}

type toolValidator struct{}

func (toolValidator) ValidateBundle(context.Context, string, string) error { return nil }
func (toolValidator) Validate(context.Context, string) error               { return nil }
func (toolValidator) ValidateApplied(context.Context, string) error        { return nil }

type toolSource struct {
	root      string
	candidate packsync.Candidate
}

func (source *toolSource) Releases(context.Context, packsync.SourceConfig) ([]packsync.Release, error) {
	return []packsync.Release{*source.candidate.Release}, nil
}
func (source *toolSource) ResolveRelease(_ context.Context, _ packsync.SourceConfig, release packsync.Release) (packsync.Candidate, error) {
	candidate := source.candidate
	candidate.Release = &release
	return candidate, nil
}
func (source *toolSource) ResolveCommit(context.Context, packsync.SourceConfig, string) (packsync.Candidate, error) {
	return packsync.Candidate{}, fmt.Errorf("unexpected commit resolution")
}
func (source *toolSource) WithSnapshot(_ context.Context, _ packsync.Candidate, root string, visit func(string) error) error {
	target := filepath.Join(root, "snapshot")
	if err := copyToolTree(source.root, target); err != nil {
		return err
	}
	err := visit(target)
	cleanupErr := os.RemoveAll(target)
	if err != nil {
		return err
	}
	return cleanupErr
}

func toolCandidate() packsync.Candidate {
	when := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	signature, payload := strings.Repeat("a", 64), strings.Repeat("b", 64)
	commit, tag, tree := strings.Repeat("c", 40), strings.Repeat("d", 40), strings.Repeat("e", 40)
	return packsync.Candidate{
		Repository: "example/new-source", RepositoryID: 1, RepositoryNodeID: "repo", RepositoryHTML: "https://github.com/example/new-source", RepositoryClone: "https://github.com/example/new-source.git", RepositoryAPI: "https://api.github.com/repos/example/new-source", Visibility: "public",
		Owner: "example", OwnerID: 2, OwnerNodeID: "owner", Public: true,
		Release:    &packsync.Release{ID: 3, NodeID: "release", Tag: "v1.0.0", Name: "v1.0.0", Target: "main", CreatedAt: when, PublishedAt: when.Add(time.Minute), Author: packsync.Actor{Login: "maintainer", ID: 4, NodeID: "actor"}},
		TagRefName: "refs/tags/v1.0.0", TagRefType: "tag", TagRefSHA: tag,
		TagObjects: []packsync.TagObject{{SHA: tag, Name: "v1.0.0", TargetSHA: commit, TargetType: "commit", Verification: packsync.Verification{Reason: "unsigned"}}},
		Commit:     commit, CommitNodeID: "commit", Tree: tree, Parents: []string{strings.Repeat("f", 40)},
		CommitVerify: packsync.Verification{Verified: true, Reason: "valid", VerifiedAt: &when, SignatureSHA256: &signature, PayloadSHA256: &payload},
	}
}

type toolLegalEvidenceDocument struct {
	SchemaVersion    int                              `json:"schema_version"`
	EvidenceID       string                           `json:"evidence_id"`
	DurableReference string                           `json:"durable_reference"`
	Issuer           string                           `json:"issuer"`
	EvidenceOrigin   string                           `json:"evidence_origin"`
	Decision         string                           `json:"decision"`
	Candidate        packsync.LegalAdmissionCandidate `json:"candidate"`
	Disposition      string                           `json:"disposition"`
	Rights           []string                         `json:"rights"`
	Obligations      []string                         `json:"obligations"`
	Disclosures      []string                         `json:"disclosures"`
	Scope            packsync.LegalAdmissionScope     `json:"scope"`
	Validity         string                           `json:"validity"`
	Invalidation     string                           `json:"invalidation"`
}

func toolLegalEvidence(t *testing.T, reference string, candidate packsync.Candidate) []byte {
	t.Helper()
	document := toolLegalEvidenceDocument{SchemaVersion: 1, EvidenceID: "fixture", DurableReference: reference, Issuer: "fixture", EvidenceOrigin: "LICENSE", Decision: "admit", Candidate: packsync.LegalAdmissionCandidate{Repository: candidate.Repository, Commit: candidate.Commit, READMEBlob: strings.Repeat("a", 40), READMELength: 1, READMESHA256: strings.Repeat("b", 64)}, Disposition: packsync.RedistributableDisposition, Rights: []string{"copy"}, Obligations: []string{"notice"}, Disclosures: []string{"fixture"}, Scope: packsync.LegalAdmissionScope{SelectedRoots: []string{"LICENSE", "skill"}, Exclusions: []string{}}, Validity: "exact", Invalidation: "change"}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func writeToolFile(t *testing.T, name, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyToolTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
