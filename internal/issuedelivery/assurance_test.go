package issuedelivery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

type fakeReviewExecutor struct {
	mu        sync.Mutex
	responses map[deliveryevidence.ReviewAxis][]CandidateReview
	calls     map[deliveryevidence.ReviewAxis]int
	hook      func(deliveryevidence.ReviewAxis)
}

type fakeCandidateRiskObserver struct {
	mu      sync.Mutex
	effects []EffectObservation
	calls   int
}

func (f *fakeCandidateRiskObserver) ObserveCandidateRisk(
	_ context.Context,
	request CandidateRiskRequest,
) (CandidateRiskObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	effects := append([]EffectObservation(nil), f.effects...)
	if len(effects) == 0 {
		effects = []EffectObservation{{
			Effect: EffectPassive, Evidence: "test-only passive candidate", Complete: true,
		}}
	}
	return CandidateRiskObservation{
		CandidateID: request.CandidateID, CommitSHA: request.CommitSHA, TreeSHA: request.TreeSHA,
		Effects: effects, Completed: true,
	}, nil
}

func (f *fakeReviewExecutor) Review(_ context.Context, request ReviewRequest) (CandidateReview, error) {
	if f.hook != nil {
		f.hook(request.Axis)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[request.Axis]++
	response := CandidateReview{Completed: true}
	queue := f.responses[request.Axis]
	if len(queue) > 0 {
		response = queue[0]
		f.responses[request.Axis] = queue[1:]
	}
	response.Axis = request.Axis
	response.CandidateID = request.CandidateID
	if response.Findings == nil {
		response.Findings = []deliveryevidence.ReviewFinding{}
	}
	return response, nil
}

type fakeValidationExecutor struct {
	mu                sync.Mutex
	focusedCalls      int
	exhaustiveCalls   int
	invalidSandbox    bool
	invalidCommand    bool
	missingAcceptance bool
	missingNegative   bool
	missingMigration  bool
	migrationNA       bool
	afterExhaustive   func()
}

func (f *fakeValidationExecutor) Focused(_ context.Context, request ValidationRequest) (ValidationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.focusedCalls++
	return f.result(request), nil
}

func (f *fakeValidationExecutor) Exhaustive(_ context.Context, request ValidationRequest) (ValidationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exhaustiveCalls++
	result := f.result(request)
	result.Command = "./scripts/validate-packy.sh"
	if f.invalidCommand {
		result.Command = "true"
	}
	result.CheckoutSHA256 = strings.Repeat("d", 64)
	result.ValidatorIdentity = "scripts/validate-packy.sh"
	result.ValidatorSHA256 = strings.Repeat("e", 64)
	result.ValidatorIdentityExpiresAt = "2030-01-01T00:00:00Z"
	result.WorkspaceClean = true
	for _, row := range request.AcceptanceRows {
		binding := "candidate " + request.CandidateID
		proof := AcceptanceProof{
			Identity: row.Identity, PositiveEvidence: binding + " positive",
			NegativeEvidence: binding + " negative", FailureEvidence: binding + " failure",
			MutationEvidence: binding + " mutation", CompatibilityEvidence: binding + " compatibility",
			PreservationEvidence: binding + " preservation", MigrationEvidence: binding + " migration",
		}
		if f.missingNegative {
			proof.NegativeEvidence = ""
		}
		if f.missingMigration {
			proof.MigrationEvidence = ""
		}
		if f.migrationNA {
			proof.MigrationEvidence = "not-applicable: candidate has no migration effect"
		}
		result.Acceptance = append(result.Acceptance, proof)
	}
	if f.missingAcceptance && len(result.Acceptance) > 0 {
		result.Acceptance = result.Acceptance[:len(result.Acceptance)-1]
	}
	if f.afterExhaustive != nil {
		f.afterExhaustive()
	}
	return result, nil
}

func (f *fakeValidationExecutor) result(request ValidationRequest) ValidationResult {
	home, config := request.HomeRoot, request.ConfigRoot
	if f.invalidSandbox {
		home = "relative/home"
	}
	return ValidationResult{
		CommitSHA: request.CommitSHA, TreeSHA: request.TreeSHA,
		Command: "test assurance", HomeRoot: home, ConfigRoot: config,
		Sandboxed: true, Succeeded: true, Completed: true,
	}
}

func assuranceFixture(t *testing.T) (*Module, *fakeGitObserver, *fakeGitHubObserver, *fakeReviewExecutor, *fakeValidationExecutor) {
	t.Helper()
	fixture := assuranceFixtureWithoutQualification(t)
	approveQualificationFixture(t, fixture.module, 357)
	return fixture.module, fixture.git, fixture.tracker, fixture.reviewer, fixture.validator
}

type assuranceTestFixture struct {
	module    *Module
	git       *fakeGitObserver
	tracker   *fakeGitHubObserver
	reviewer  *fakeReviewExecutor
	validator *fakeValidationExecutor
}

func assuranceFixtureWithoutQualification(t *testing.T) assuranceTestFixture {
	t.Helper()
	module, git, tracker := moduleFixture(t, 357)
	git.value.Branch = "chore/issue-357-prove-low-risk-candidate"
	reviewer := &fakeReviewExecutor{
		responses: map[deliveryevidence.ReviewAxis][]CandidateReview{},
		calls:     map[deliveryevidence.ReviewAxis]int{},
	}
	validator := &fakeValidationExecutor{}
	risk := &fakeCandidateRiskObserver{}
	sandbox, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"home", "config"} {
		if err := os.Mkdir(filepath.Join(sandbox, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	module.review, module.validation, module.risk, module.sandboxRoot = reviewer, validator, risk, sandbox
	return assuranceTestFixture{
		module: module, git: git, tracker: tracker, reviewer: reviewer, validator: validator,
	}
}

func approveQualificationFixture(t *testing.T, module *Module, issue int) {
	t.Helper()
	request := Request{RepositoryPath: "/repo", IssueNumber: issue}
	qualified := mustAdvance(t, module, request)
	matrixHash, err := acceptanceMatrixDigest(qualified.Evidence.AcceptanceMatrix)
	if err != nil {
		t.Fatal(err)
	}
	findings := make([]deliveryevidence.ReviewFinding, len(qualified.Evidence.AcceptanceMatrix))
	links := make(map[string]string, len(qualified.Evidence.Scope.OwnedNow))
	for _, entry := range qualified.Evidence.Scope.OwnedNow {
		links[entry.Identity] = entry.EvidenceLink
	}
	for index, row := range qualified.Evidence.AcceptanceMatrix {
		findings[index] = deliveryevidence.ReviewFinding{
			ID: "fixture-qualification-" + row.Identity, Axis: deliveryevidence.ReviewSpec,
			Severity: deliveryevidence.SeverityP1, Authority: deliveryevidence.AuthoritySpecRequirement,
			Citation: links[row.Identity], Location: row.Identity,
			Evidence: "the production-shaped assurance fixture requires an explicit evidence seam",
		}
	}
	rejected := mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: issue,
		QualificationReview: &QualificationReview{
			AuthoritySHA256:        qualified.Observations.AuthoritySHA256,
			AcceptanceMatrixSHA256: matrixHash,
			Findings:               findings, Completed: true,
		},
	})
	rows := append([]deliveryevidence.AcceptanceRow(nil), rejected.Evidence.AcceptanceMatrix...)
	findingIDs := make([]string, len(findings))
	for index := range rows {
		findingIDs[index] = findings[index].ID
		rows[index].OwningSeam = "issuedelivery assurance fixture seam"
		rows[index].PositiveEvidence = "planned: focused positive evidence"
		rows[index].NegativeEvidence = "planned: focused negative evidence"
		rows[index].FailureEvidence = "planned: focused failure evidence"
		rows[index].MutationEvidence = "planned: focused mutation evidence"
		rows[index].CompatibilityEvidence = "planned: focused compatibility evidence"
		rows[index].PreservationEvidence = "planned: focused preservation evidence"
	}
	corrected := mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: issue,
		QualificationCorrection: &QualificationCorrection{
			AuthoritySHA256:      rejected.Observations.AuthoritySHA256,
			ReviewedMatrixSHA256: matrixHash,
			FindingIDs:           findingIDs,
			AcceptanceMatrix:     rows,
			Evidence:             "mapped every assurance criterion through the typed correction envelope",
		},
	})
	correctedHash, err := acceptanceMatrixDigest(corrected.Evidence.AcceptanceMatrix)
	if err != nil {
		t.Fatal(err)
	}
	approved := mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: issue,
		QualificationReview: &QualificationReview{
			AuthoritySHA256:        corrected.Observations.AuthoritySHA256,
			AcceptanceMatrixSHA256: correctedHash,
			Findings:               []deliveryevidence.ReviewFinding{}, Completed: true,
		},
	})
	if !approved.QualificationApproved {
		t.Fatalf("fixture qualification was not approved: %#v", approved)
	}
}

func TestAdvanceReviewsAccumulatedCandidateInParallelAndReachesExactLocalReadiness(t *testing.T) {
	module, _, _, reviewer, validator := assuranceFixture(t)
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	focused, err := module.Advance(context.Background(), request)
	if err != nil || focused.State != StateNeedsReview || focused.Candidate == nil ||
		focused.Candidate.Focused == nil {
		t.Fatalf("focused outcome=%#v err=%v", focused, err)
	}

	entered := make(chan deliveryevidence.ReviewAxis, 2)
	release := make(chan struct{})
	reviewer.hook = func(axis deliveryevidence.ReviewAxis) {
		entered <- axis
		<-release
	}
	reviewed := make(chan Outcome, 1)
	reviewErr := make(chan error, 1)
	go func() {
		outcome, advanceErr := module.Advance(context.Background(), request)
		reviewErr <- advanceErr
		reviewed <- outcome
	}()
	seen := map[deliveryevidence.ReviewAxis]bool{}
	for len(seen) < 2 {
		select {
		case axis := <-entered:
			seen[axis] = true
		case err := <-reviewErr:
			t.Fatalf("advance returned before parallel reviews: %v", err)
		case outcome := <-reviewed:
			t.Fatalf("advance returned before parallel reviews: %#v", outcome)
		}
	}
	if !seen[deliveryevidence.ReviewStandards] || !seen[deliveryevidence.ReviewSpec] {
		t.Fatalf("reviews did not start independently: %v", seen)
	}
	close(release)
	if err := <-reviewErr; err != nil {
		t.Fatal(err)
	}
	reviewOutcome := <-reviewed
	if reviewOutcome.State != StateNeedsReview || len(reviewOutcome.Candidate.Reviews) != 2 {
		t.Fatalf("review outcome=%#v", reviewOutcome)
	}
	reviewer.hook = nil

	ready, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if ready.State != StateWaiting || ready.LocalReadiness == nil ||
		ready.LocalReadiness.CommitSHA != strings.Repeat("b", 40) ||
		ready.LocalReadiness.TreeSHA != strings.Repeat("c", 40) {
		t.Fatalf("local readiness=%#v", ready)
	}
	for _, row := range ready.Evidence.AcceptanceMatrix {
		if row.State != deliveryevidence.AcceptanceProved {
			t.Fatalf("acceptance row %q state=%q", row.Identity, row.State)
		}
	}
	if validator.focusedCalls != 1 || validator.exhaustiveCalls != 1 {
		t.Fatalf("validation calls focused=%d exhaustive=%d", validator.focusedCalls, validator.exhaustiveCalls)
	}
}

func TestAdvanceBoundedRepairUsesFocusedOriginatingAxisConfirmation(t *testing.T) {
	module, git, _, reviewer, validator := assuranceFixture(t)
	finding := deliveryevidence.ReviewFinding{
		ID: "S357-1", Axis: deliveryevidence.ReviewStandards, Severity: deliveryevidence.SeverityP2,
		Authority: deliveryevidence.AuthorityDocumentedStandard, Citation: "AGENTS.md",
		Location: "internal/issuedelivery/assurance.go", Evidence: "bounded repair required",
	}
	reviewer.responses[deliveryevidence.ReviewStandards] = []CandidateReview{
		{Completed: true, Findings: []deliveryevidence.ReviewFinding{finding}},
		{Completed: true, Findings: []deliveryevidence.ReviewFinding{}},
	}
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	found := mustAdvance(t, module, request)
	if found.State != StateNeedsDecision || found.Repair == nil {
		t.Fatalf("finding outcome=%#v", found)
	}
	planned := mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: 357,
		Repair: &RepairDecision{
			CandidateID: found.Repair.CandidateID, Class: RepairBounded,
			Findings: []FindingDecision{{
				FindingID: "S357-1", Disposition: FindingAccepted, Evidence: "repair as one bounded batch",
			}},
		},
	})
	if planned.State != StateNeedsReview {
		t.Fatalf("repair plan outcome=%#v", planned)
	}
	git.value.HeadSHA, git.value.TreeSHA = strings.Repeat("d", 40), strings.Repeat("e", 40)
	repaired := mustAdvance(t, module, request)
	if repaired.Candidate == nil || repaired.Candidate.RepairClass != RepairBounded ||
		repaired.Candidate.Focused == nil ||
		repaired.Candidate.BaseSHA != strings.Repeat("a", 40) {
		t.Fatalf("bounded candidate=%#v", repaired)
	}
	confirmed := mustAdvance(t, module, request)
	if confirmed.State != StateNeedsReview || len(confirmed.Candidate.Reviews) != 1 ||
		confirmed.Candidate.Reviews[0].Axis != deliveryevidence.ReviewStandards {
		t.Fatalf("bounded confirmation=%#v", confirmed)
	}
	ready := mustAdvance(t, module, request)
	if ready.State != StateWaiting || ready.LocalReadiness == nil {
		t.Fatalf("bounded readiness=%#v", ready)
	}
	if reviewer.calls[deliveryevidence.ReviewStandards] != 2 ||
		reviewer.calls[deliveryevidence.ReviewSpec] != 1 ||
		validator.focusedCalls != 2 || validator.exhaustiveCalls != 1 {
		t.Fatalf("calls reviews=%v focused=%d exhaustive=%d", reviewer.calls, validator.focusedCalls, validator.exhaustiveCalls)
	}
}

func TestAdvanceCandidateChangingRepairRepeatsBothReviewAxes(t *testing.T) {
	module, git, _, reviewer, _ := assuranceFixture(t)
	finding := deliveryevidence.ReviewFinding{
		ID: "S357-2", Axis: deliveryevidence.ReviewSpec, Severity: deliveryevidence.SeverityP2,
		Authority: deliveryevidence.AuthoritySpecRequirement, Citation: "issue#357",
		Location: "internal/issuedelivery/assurance.go", Evidence: "candidate behavior must change",
	}
	reviewer.responses[deliveryevidence.ReviewSpec] = []CandidateReview{
		{Completed: true, Findings: []deliveryevidence.ReviewFinding{finding}},
		{Completed: true, Findings: []deliveryevidence.ReviewFinding{}},
	}
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	found := mustAdvance(t, module, request)
	planned := mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: 357,
		Repair: &RepairDecision{
			CandidateID: found.Repair.CandidateID, Class: RepairCandidateChanging,
			Findings: []FindingDecision{{
				FindingID: "S357-2", Disposition: FindingAccepted, Evidence: "repair as one candidate-changing batch",
			}},
		},
	})
	oldCandidateID := planned.Candidate.ID
	git.value.HeadSHA, git.value.TreeSHA = strings.Repeat("d", 40), strings.Repeat("e", 40)
	changed := mustAdvance(t, module, request)
	if changed.Candidate == nil || changed.Candidate.ID == oldCandidateID ||
		changed.Candidate.RepairClass != RepairCandidateChanging {
		t.Fatalf("candidate-changing repair=%#v", changed)
	}
	reviewed := mustAdvance(t, module, request)
	if len(reviewed.Candidate.Reviews) != 2 ||
		reviewer.calls[deliveryevidence.ReviewStandards] != 2 ||
		reviewer.calls[deliveryevidence.ReviewSpec] != 2 {
		t.Fatalf("candidate-changing reviews=%#v calls=%v", reviewed.Candidate.Reviews, reviewer.calls)
	}
}

func TestAdvanceRejectsBoundedClassificationWithoutAcceptedFinding(t *testing.T) {
	module, _, _, reviewer, _ := assuranceFixture(t)
	finding := deliveryevidence.ReviewFinding{
		ID: "S357-3", Axis: deliveryevidence.ReviewStandards, Severity: deliveryevidence.SeverityP2,
		Authority: deliveryevidence.AuthorityDocumentedStandard, Citation: "AGENTS.md",
		Location: "internal/issuedelivery/assurance.go", Evidence: "finding may be rejected",
	}
	reviewer.responses[deliveryevidence.ReviewStandards] = []CandidateReview{{
		Completed: true, Findings: []deliveryevidence.ReviewFinding{finding},
	}}
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	found := mustAdvance(t, module, request)
	_, err := module.Advance(context.Background(), Request{
		RepositoryPath: "/repo", IssueNumber: 357,
		Repair: &RepairDecision{
			CandidateID: found.Repair.CandidateID, Class: RepairBounded,
			Findings: []FindingDecision{{
				FindingID: "S357-3", Disposition: FindingRejected, Evidence: "review evidence disproves finding",
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "bounded repair requires") {
		t.Fatalf("bounded rejected-only adjudication error=%v", err)
	}
}

func TestAdvanceRejectsDuplicateFindingIDsAcrossReviewAxes(t *testing.T) {
	module, _, _, reviewer, _ := assuranceFixture(t)
	reviewer.responses[deliveryevidence.ReviewStandards] = []CandidateReview{{
		Completed: true, Findings: []deliveryevidence.ReviewFinding{{
			ID: "duplicate", Axis: deliveryevidence.ReviewStandards, Severity: deliveryevidence.SeverityP2,
			Authority: deliveryevidence.AuthorityDocumentedStandard, Citation: "AGENTS.md",
			Location: "internal/issuedelivery/assurance.go", Evidence: "standards evidence",
		}},
	}}
	reviewer.responses[deliveryevidence.ReviewSpec] = []CandidateReview{{
		Completed: true, Findings: []deliveryevidence.ReviewFinding{{
			ID: "duplicate", Axis: deliveryevidence.ReviewSpec, Severity: deliveryevidence.SeverityP2,
			Authority: deliveryevidence.AuthoritySpecRequirement, Citation: "issue#357",
			Location: "internal/issuedelivery/assurance.go", Evidence: "spec evidence",
		}},
	}}
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	if _, err := module.Advance(context.Background(), request); err == nil ||
		!strings.Contains(err.Error(), "duplicate candidate review finding") {
		t.Fatalf("duplicate review finding error=%v", err)
	}
}

func TestAdvanceReusesExactReceiptAndInvalidatesChangedCandidate(t *testing.T) {
	module, git, _, _, validator := assuranceFixture(t)
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	for range 4 {
		mustAdvance(t, module, request)
	}
	reused := mustAdvance(t, module, request)
	if reused.State != StateWaiting || reused.LocalReadiness == nil || validator.exhaustiveCalls != 1 {
		t.Fatalf("reused readiness=%#v exhaustive=%d", reused, validator.exhaustiveCalls)
	}
	git.value.Branch = "main"
	wrongBranch := mustAdvance(t, module, request)
	if wrongBranch.State != StateBlocked {
		t.Fatalf("wrong branch reused readiness=%#v", wrongBranch)
	}
	git.value.Branch = "chore/issue-357-prove-low-risk-candidate"
	restored := mustAdvance(t, module, request)
	if restored.State != StateWaiting || validator.exhaustiveCalls != 1 {
		t.Fatalf("restored readiness=%#v exhaustive=%d", restored, validator.exhaustiveCalls)
	}
	git.value.HeadSHA, git.value.TreeSHA = strings.Repeat("d", 40), strings.Repeat("e", 40)
	invalidated := mustAdvance(t, module, request)
	if invalidated.State != StateNeedsReview || invalidated.LocalReadiness != nil ||
		invalidated.Candidate == nil || invalidated.Candidate.Focused == nil {
		t.Fatalf("invalidated candidate=%#v", invalidated)
	}
	for _, row := range invalidated.Evidence.AcceptanceMatrix {
		if row.State != deliveryevidence.AcceptancePlanned {
			t.Fatalf("changed candidate retained acceptance row %q state=%q", row.Identity, row.State)
		}
	}
}

func TestAdvanceBlocksUnsafeValidationSandbox(t *testing.T) {
	module, _, _, _, validator := assuranceFixture(t)
	validator.invalidSandbox = true
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || !strings.Contains(blocked.Reason, "sandbox") {
		t.Fatalf("unsafe sandbox outcome=%#v", blocked)
	}
}

func TestAdvanceBlocksSymlinkedConfiguredSandbox(t *testing.T) {
	module, _, _, _, _ := assuranceFixture(t)
	home := filepath.Join(module.sandboxRoot, "home")
	if err := os.Remove(home); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), home); err != nil {
		t.Fatal(err)
	}
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || !strings.Contains(blocked.Reason, "physically isolated") {
		t.Fatalf("symlinked sandbox outcome=%#v", blocked)
	}
}

func TestAdvanceRetriesFreshGateWithoutRerunningExactExhaustiveReceipt(t *testing.T) {
	module, git, _, _, validator := assuranceFixture(t)
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	for range 2 {
		mustAdvance(t, module, request)
	}
	validator.afterExhaustive = func() {
		git.value.Branch = "main"
	}
	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || validator.exhaustiveCalls != 1 {
		t.Fatalf("transient gate outcome=%#v exhaustive=%d", blocked, validator.exhaustiveCalls)
	}
	validator.afterExhaustive = nil
	git.value.Branch = "chore/issue-357-prove-low-risk-candidate"
	ready := mustAdvance(t, module, request)
	if ready.State != StateWaiting || ready.LocalReadiness == nil || validator.exhaustiveCalls != 1 {
		t.Fatalf("retried gate outcome=%#v exhaustive=%d", ready, validator.exhaustiveCalls)
	}
}

func TestAdvanceBlocksNonCanonicalExhaustiveAuthority(t *testing.T) {
	module, _, _, _, validator := assuranceFixture(t)
	validator.invalidCommand = true
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	for range 3 {
		mustAdvance(t, module, request)
	}
	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || !strings.Contains(blocked.Reason, "exhaustive") {
		t.Fatalf("non-canonical exhaustive outcome=%#v", blocked)
	}
}

func TestAdvanceBlocksIncompleteAcceptanceTraceability(t *testing.T) {
	module, _, _, _, validator := assuranceFixture(t)
	validator.missingAcceptance = true
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	for range 3 {
		mustAdvance(t, module, request)
	}
	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || !strings.Contains(blocked.Reason, "traceability") {
		t.Fatalf("incomplete acceptance outcome=%#v", blocked)
	}
}

func mustAdvance(t *testing.T, module *Module, request Request) Outcome {
	t.Helper()
	outcome, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}
