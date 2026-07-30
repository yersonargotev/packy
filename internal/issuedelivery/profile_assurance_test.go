package issuedelivery

import (
	"context"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

type fakeSpecialistReviewExecutor struct {
	calls     []SensitiveBoundary
	responses map[SensitiveBoundary][][]SpecialistFinding
}

func (f *fakeSpecialistReviewExecutor) ReviewSpecialist(
	_ context.Context,
	request SpecialistReviewRequest,
) (SpecialistReview, error) {
	f.calls = append(f.calls, request.Boundary)
	findings := []SpecialistFinding{}
	if queue := f.responses[request.Boundary]; len(queue) > 0 {
		findings = queue[0]
		f.responses[request.Boundary] = queue[1:]
	}
	return SpecialistReview{
		CandidateID: request.CandidateID,
		Boundary:    request.Boundary,
		Specialist:  request.Specialist,
		Findings:    findings,
		Completed:   true,
	}, nil
}

type fakeBoundaryValidationExecutor struct {
	calls          []SensitiveBoundary
	mutateOperator bool
}

func (f *fakeBoundaryValidationExecutor) ValidateBoundary(
	_ context.Context,
	request BoundaryValidationRequest,
) (BoundaryValidationResult, error) {
	f.calls = append(f.calls, request.Boundary)
	before := strings.Repeat("1", 64)
	after := before
	if f.mutateOperator {
		after = strings.Repeat("2", 64)
	}
	return BoundaryValidationResult{
		CandidateID: request.CandidateID, Boundary: request.Boundary,
		CommitSHA: request.CommitSHA, TreeSHA: request.TreeSHA,
		Command: "prove boundary", ToolIdentity: "test-boundary-validator",
		ToolSHA256: strings.Repeat("3", 64),
		HomeRoot:   request.HomeRoot, ConfigRoot: request.ConfigRoot,
		OperatorStateBeforeSHA256: before, OperatorStateAfterSHA256: after,
		WriteManifestSHA256: strings.Repeat("4", 64),
		Evidence:            "writes remained inside the isolated roots",
		Sandboxed:           true, Succeeded: true, Completed: true,
	}, nil
}

func TestStandardCandidateRequiresBehavioralNegativePreservationAndCompatibilityEvidence(t *testing.T) {
	module, _, _, _, validator := assuranceFixture(t)
	module.risk.(*fakeCandidateRiskObserver).effects = []EffectObservation{{
		Effect: EffectOrdinaryBehavior, Evidence: "changes command behavior", Complete: true,
	}}
	validator.missingNegative = true
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}

	for step := 0; step < 3; step++ {
		if _, err := module.Advance(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	outcome, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != StateBlocked ||
		outcome.Candidate == nil ||
		outcome.Candidate.Profile != deliveryevidence.RiskStandard ||
		!strings.Contains(outcome.Reason, "acceptance traceability") {
		t.Fatalf("standard assurance outcome=%#v", outcome)
	}
}

func TestHighRiskCandidateRequiresSpecialistAndSandboxedBoundaryProof(t *testing.T) {
	module, _, _, _, validator := assuranceFixture(t)
	module.risk.(*fakeCandidateRiskObserver).effects = []EffectObservation{{
		Effect: EffectSecurity, Evidence: "changes credential handling", Complete: true,
	}}
	specialist := &fakeSpecialistReviewExecutor{}
	boundary := &fakeBoundaryValidationExecutor{}
	module.specialist, module.boundary = specialist, boundary
	validator.migrationNA = true
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}

	var outcome Outcome
	var err error
	for step := 0; step < 6; step++ {
		outcome, err = module.Advance(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
	}
	if outcome.State != StateWaiting || outcome.LocalReadiness == nil ||
		outcome.Candidate == nil || outcome.Candidate.Profile != deliveryevidence.RiskHigh {
		t.Fatalf("high-risk state=%q readiness=%#v candidate=%#v", outcome.State, outcome.LocalReadiness, outcome.Candidate)
	}
	if len(specialist.calls) != 1 || specialist.calls[0] != BoundarySecurity ||
		len(boundary.calls) != 1 || boundary.calls[0] != BoundarySecurity {
		t.Fatalf("specialist calls=%v boundary calls=%v", specialist.calls, boundary.calls)
	}
}

func TestHighRiskBoundaryProofRejectsOperatorConfigurationMutation(t *testing.T) {
	module, _, _, _, _ := assuranceFixture(t)
	module.risk.(*fakeCandidateRiskObserver).effects = []EffectObservation{{
		Effect: EffectRealConfiguration, Evidence: "writes configuration", Complete: true,
	}}
	module.specialist = &fakeSpecialistReviewExecutor{}
	module.boundary = &fakeBoundaryValidationExecutor{mutateOperator: true}
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}

	var outcome Outcome
	var err error
	for step := 0; step < 5; step++ {
		outcome, err = module.Advance(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
	}
	if outcome.State != StateBlocked || !strings.Contains(outcome.Reason, "boundary proof") {
		t.Fatalf("mutating boundary outcome=%#v", outcome)
	}
}

func TestMigrationEffectRequiresMigrationEvidence(t *testing.T) {
	module, _, _, _, validator := assuranceFixture(t)
	module.risk.(*fakeCandidateRiskObserver).effects = []EffectObservation{{
		Effect: EffectMigration, Evidence: "migrates persisted state", Complete: true,
	}}
	module.specialist = &fakeSpecialistReviewExecutor{}
	module.boundary = &fakeBoundaryValidationExecutor{}
	validator.missingMigration = true
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}

	var outcome Outcome
	var err error
	for step := 0; step < 6; step++ {
		outcome, err = module.Advance(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
	}
	if outcome.State != StateBlocked || !strings.Contains(outcome.Reason, "acceptance traceability") {
		t.Fatalf("migration assurance outcome=%#v", outcome)
	}
}

func TestCandidateRiskProfileAndBoundariesAreMonotonic(t *testing.T) {
	module, _, _, _, _ := assuranceFixture(t)
	risk := module.risk.(*fakeCandidateRiskObserver)
	risk.effects = []EffectObservation{{
		Effect: EffectPublication, Evidence: "publishes an artifact", Complete: true,
	}}
	module.specialist = &fakeSpecialistReviewExecutor{}
	module.boundary = &fakeBoundaryValidationExecutor{}
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}

	if _, err := module.Advance(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	first, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	risk.effects = []EffectObservation{{
		Effect: EffectPassive, Evidence: "later observation appears passive", Complete: true,
	}}
	next, err := module.Advance(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Candidate.Profile != deliveryevidence.RiskHigh ||
		next.Candidate.Profile != deliveryevidence.RiskHigh ||
		len(next.Candidate.Boundaries) != 1 || next.Candidate.Boundaries[0] != BoundaryPublication ||
		next.Reason != "required candidate reviews completed without findings" {
		t.Fatalf("first profile=%q next profile=%q boundaries=%v reason=%q",
			first.Candidate.Profile, next.Candidate.Profile, next.Candidate.Boundaries, next.Reason)
	}
}

func TestBoundedRepairThatIntroducesSensitiveEffectBecomesCandidateChanging(t *testing.T) {
	module, git, _, reviewer, _ := assuranceFixture(t)
	risk := module.risk.(*fakeCandidateRiskObserver)
	finding := deliveryevidence.ReviewFinding{
		ID: "S358-risk-1", Axis: deliveryevidence.ReviewStandards,
		Severity:  deliveryevidence.SeverityP2,
		Authority: deliveryevidence.AuthorityDocumentedStandard,
		Citation:  "AGENTS.md", Location: "internal/issuedelivery",
		Evidence: "ordinary bounded repair requested",
	}
	reviewer.responses[deliveryevidence.ReviewStandards] = []CandidateReview{{
		Completed: true, Findings: []deliveryevidence.ReviewFinding{finding},
	}}
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	found := mustAdvance(t, module, request)
	mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: 357,
		Repair: &RepairDecision{
			CandidateID: found.Repair.CandidateID, Class: RepairBounded,
			Findings: []FindingDecision{{
				FindingID: finding.ID, Disposition: FindingAccepted, Evidence: "repair in one batch",
			}},
		},
	})

	risk.effects = []EffectObservation{{
		Effect: EffectSecurity, Evidence: "repair now changes a security boundary", Complete: true,
	}}
	module.specialist = &fakeSpecialistReviewExecutor{}
	module.boundary = &fakeBoundaryValidationExecutor{}
	git.value.HeadSHA, git.value.TreeSHA = strings.Repeat("d", 40), strings.Repeat("e", 40)
	changed := mustAdvance(t, module, request)
	if changed.Candidate == nil ||
		changed.Candidate.RepairClass != RepairCandidateChanging ||
		len(changed.Candidate.RequiredReviews) != 2 ||
		len(changed.Candidate.RequiredSpecialists) != 1 ||
		changed.Candidate.RequiredSpecialists[0] != BoundarySecurity {
		t.Fatalf("sensitive bounded repair=%#v", changed.Candidate)
	}
}

func TestBoundedRepairThatRaisesLowToStandardBecomesCandidateChanging(t *testing.T) {
	module, git, _, reviewer, _ := assuranceFixture(t)
	risk := module.risk.(*fakeCandidateRiskObserver)
	finding := deliveryevidence.ReviewFinding{
		ID: "S358-standard-1", Axis: deliveryevidence.ReviewStandards,
		Severity:  deliveryevidence.SeverityP2,
		Authority: deliveryevidence.AuthorityDocumentedStandard,
		Citation:  "AGENTS.md", Location: "internal/issuedelivery",
		Evidence: "ordinary bounded repair requested",
	}
	reviewer.responses[deliveryevidence.ReviewStandards] = []CandidateReview{{
		Completed: true, Findings: []deliveryevidence.ReviewFinding{finding},
	}}
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	found := mustAdvance(t, module, request)
	mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: 357,
		Repair: &RepairDecision{
			CandidateID: found.Repair.CandidateID, Class: RepairBounded,
			Findings: []FindingDecision{{
				FindingID: finding.ID, Disposition: FindingAccepted, Evidence: "repair in one batch",
			}},
		},
	})

	risk.effects = []EffectObservation{{
		Effect: EffectOrdinaryBehavior, Evidence: "repair changes ordinary behavior", Complete: true,
	}}
	git.value.HeadSHA, git.value.TreeSHA = strings.Repeat("d", 40), strings.Repeat("e", 40)
	changed := mustAdvance(t, module, request)
	if changed.Candidate == nil ||
		changed.Candidate.Profile != deliveryevidence.RiskStandard ||
		changed.Candidate.RepairClass != RepairCandidateChanging ||
		len(changed.Candidate.RequiredReviews) != 2 {
		t.Fatalf("standard bounded repair=%#v", changed.Candidate)
	}
}

func TestSpecialistOnlyFindingCanCompleteBoundedRepair(t *testing.T) {
	module, git, _, reviewer, _ := assuranceFixture(t)
	module.risk.(*fakeCandidateRiskObserver).effects = []EffectObservation{{
		Effect: EffectSecurity, Evidence: "changes credential handling", Complete: true,
	}}
	finding := SpecialistFinding{
		ID: "S358-security-1", Severity: deliveryevidence.SeverityP2,
		Citation: "security policy", Location: "internal/issuedelivery",
		Evidence: "bounded security correction required",
	}
	specialist := &fakeSpecialistReviewExecutor{
		responses: map[SensitiveBoundary][][]SpecialistFinding{
			BoundarySecurity: {{finding}, {}},
		},
	}
	module.specialist = specialist
	module.boundary = &fakeBoundaryValidationExecutor{}
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	found := mustAdvance(t, module, request)
	if found.State != StateNeedsDecision || found.Repair == nil {
		t.Fatalf("specialist finding outcome=%#v", found)
	}
	mustAdvance(t, module, Request{
		RepositoryPath: "/repo", IssueNumber: 357,
		Repair: &RepairDecision{
			CandidateID: found.Repair.CandidateID, Class: RepairBounded,
			Findings: []FindingDecision{{
				FindingID: finding.ID, Disposition: FindingAccepted, Evidence: "bounded specialist repair",
			}},
		},
	})

	git.value.HeadSHA, git.value.TreeSHA = strings.Repeat("d", 40), strings.Repeat("e", 40)
	repaired := mustAdvance(t, module, request)
	if repaired.Candidate.RepairClass != RepairBounded ||
		len(repaired.Candidate.RequiredReviews) != 0 ||
		len(repaired.Candidate.RequiredSpecialists) != 1 {
		t.Fatalf("specialist bounded candidate=%#v", repaired.Candidate)
	}
	confirmed := mustAdvance(t, module, request)
	if len(confirmed.Candidate.SpecialistReviews) != 1 ||
		len(confirmed.Candidate.Reviews) != 0 {
		t.Fatalf("specialist confirmation=%#v", confirmed.Candidate)
	}
	mustAdvance(t, module, request)
	ready := mustAdvance(t, module, request)
	if ready.State != StateWaiting || ready.LocalReadiness == nil ||
		len(specialist.calls) != 2 ||
		reviewer.calls[deliveryevidence.ReviewStandards] != 1 ||
		reviewer.calls[deliveryevidence.ReviewSpec] != 1 {
		t.Fatalf("specialist bounded readiness=%#v specialist=%v reviews=%v",
			ready, specialist.calls, reviewer.calls)
	}
}
