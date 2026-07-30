package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
	"github.com/yersonargotev/packy/internal/issuedelivery"
)

type fakeIssueDeliveryAdvancer struct {
	outcomes []issuedelivery.Outcome
	requests []issuedelivery.Request
}

type commandGitObserver struct {
	observation issuedelivery.GitObservation
}

func (o commandGitObserver) ObserveGit(context.Context, string) (issuedelivery.GitObservation, error) {
	return o.observation, nil
}

type commandTrackerObserver struct {
	observation issuedelivery.TrackerObservation
}

func (o commandTrackerObserver) ObserveIssue(
	_ context.Context,
	_ issuedelivery.GitObservation,
	_ int,
) (issuedelivery.TrackerObservation, error) {
	return o.observation, nil
}

type commandClock struct {
	now time.Time
}

func (c *commandClock) Now() time.Time {
	c.now = c.now.Add(time.Second)
	return c.now
}

func (f *fakeIssueDeliveryAdvancer) Advance(_ context.Context, request issuedelivery.Request) (issuedelivery.Outcome, error) {
	f.requests = append(f.requests, request)
	outcome := f.outcomes[0]
	f.outcomes = f.outcomes[1:]
	return outcome, nil
}

func TestAdvanceCommandCallsDeepModuleAndSynthesizesOnlyExactRemoteAuthorization(t *testing.T) {
	repository := t.TempDir()
	resolvedRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	candidate := &issuedelivery.Candidate{
		ID: "candidate-1", CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40),
	}
	readiness := &issuedelivery.LocalReadiness{
		CandidateID: candidate.ID, CommitSHA: candidate.CommitSHA, TreeSHA: candidate.TreeSHA,
		AuthorityHash: strings.Repeat("c", 64), Branch: "chore/issue-361-cutover",
		ReadyAt: "2026-07-30T06:00:00.000000000Z",
	}
	fake := &fakeIssueDeliveryAdvancer{outcomes: []issuedelivery.Outcome{
		{
			RunID: "run-1", State: issuedelivery.StateWaiting, Reason: "local ready",
			Candidate: candidate, LocalReadiness: readiness,
		},
		{
			RunID: "run-1", State: issuedelivery.StateWaiting,
			Reason: "exact non-local delivery authority is recorded",
		},
	}}
	var configured advanceOptions
	cmd := command{AdvanceFactory: func(options advanceOptions) (issueDeliveryAdvancer, error) {
		configured = options
		return fake, nil
	}}
	var stdout bytes.Buffer
	err = cmd.run(context.Background(), []string{
		"advance", "--repository", repository, "--issue", "361",
		"--spec", "354",
		"--risk-profile", "low-risk", "--authorize-non-local",
	}, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if configured.RepositoryPath != resolvedRepository || configured.IssueNumber != 361 ||
		configured.SpecificationNumber != 354 ||
		configured.DeclaredProfile != deliveryevidence.RiskLow || !configured.AuthorizeRemote {
		t.Fatalf("configured options = %#v", configured)
	}
	if len(fake.requests) != 2 || fake.requests[0].NonLocal != nil {
		t.Fatalf("requests = %#v", fake.requests)
	}
	wantAuthorization := &issuedelivery.NonLocalAuthorization{
		RunID: "run-1", CandidateID: candidate.ID, CommitSHA: candidate.CommitSHA,
		TreeSHA: candidate.TreeSHA, Branch: readiness.Branch, LocalReadyAt: readiness.ReadyAt,
	}
	if got := fake.requests[1].NonLocal; got == nil || *got != *wantAuthorization {
		t.Fatalf("non-local authorization = %#v, want %#v", got, wantAuthorization)
	}
	var report advanceReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.RunID != "run-1" || report.State != issuedelivery.StateWaiting ||
		report.Reason != "exact non-local delivery authority is recorded" {
		t.Fatalf("report = %#v", report)
	}
}

func TestAdvanceCommandHighestSeamCreatesV2RunThroughRealModule(t *testing.T) {
	repository := t.TempDir()
	resolvedRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(repository, "common")
	if err = os.Mkdir(common, 0o700); err != nil {
		t.Fatal(err)
	}
	module, err := issuedelivery.New(issuedelivery.Config{
		Git: commandGitObserver{observation: issuedelivery.GitObservation{
			CommonDir: common, Worktree: resolvedRepository,
			OriginURL: "git@github.com:yersonargotev/packy.git",
			Owner:     "yersonargotev", Name: "packy",
			StartingBaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40),
			TreeSHA: strings.Repeat("c", 40), WorkspaceClean: true,
			Branch: "chore/issue-361-advance-cutover",
		}},
		GitHub: commandTrackerObserver{observation: issuedelivery.TrackerObservation{
			Repository: deliveryevidence.RepositoryIdentity{
				Owner: "yersonargotev", Name: "packy", NodeID: "R1",
			},
			Issue: deliveryevidence.IssueIdentity{Number: 361, NodeID: "I361"},
			Title: "Cut over issue delivery to Advance", Body: "self-contained authority",
			State: "OPEN", Labels: []string{"status:approved", "type:chore"},
			Criteria: []issuedelivery.AuthorityItem{{
				Text: "The private CLI invokes Advance.", EvidenceLink: "issue#361:criterion-1",
			}},
			Exclusions: []issuedelivery.AuthorityItem{{
				Text: "No public Packy command.", EvidenceLink: "issue#361:out-of-scope",
			}},
			Dependencies: []issuedelivery.DependencyObservation{},
			References:   []issuedelivery.ReferenceObservation{},
			Ambiguities:  []issuedelivery.AuthorityItem{},
		}},
		Clock:           &commandClock{now: time.Date(2026, 7, 30, 6, 0, 0, 0, time.UTC)},
		DeclaredProfile: deliveryevidence.RiskLow,
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := command{
		Now: func() time.Time { return time.Date(2026, 7, 30, 6, 1, 0, 0, time.UTC) },
		AdvanceFactory: func(options advanceOptions) (issueDeliveryAdvancer, error) {
			return module, nil
		},
	}
	var stdout bytes.Buffer
	if err = cmd.run(context.Background(), []string{
		"advance", "--repository", resolvedRepository, "--issue", "361", "--risk-profile", "low-risk",
	}, &stdout); err != nil {
		t.Fatal(err)
	}
	var report advanceReport
	if err = json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.State != issuedelivery.StateNeedsReview || report.RunID == "" ||
		report.Evidence == nil || report.Evidence.Schema != deliveryevidence.SchemaV2 ||
		report.Evidence.RiskProfile != deliveryevidence.RiskLow ||
		report.TimingReport.LowRisk.PRReadinessObjectiveNanoseconds != int64(25*time.Minute) {
		t.Fatalf("highest-seam Advance report = %#v", report)
	}
}

func TestAdvanceCommandAdmitsOnlyTypedSemanticContent(t *testing.T) {
	root := t.TempDir()
	repository := t.TempDir()
	write := func(name string, value any) string {
		t.Helper()
		path := filepath.Join(root, name+".json")
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	decision := issuedelivery.Decision{
		RequestID: "decision-1", Disposition: issuedelivery.DecisionOwnedNow,
		Requirement: "clarified behavior", EvidenceLink: "issue#361",
	}
	reviews := advanceReviewContent{Reviews: []issuedelivery.CandidateReview{{
		CandidateID: "candidate-1", Axis: deliveryevidence.ReviewStandards, Completed: true,
	}}, Acceptance: []issuedelivery.AcceptanceProof{{
		Identity: "criterion-1", PositiveEvidence: "candidate-1: reviewed positive proof",
	}}, QualificationReview: &issuedelivery.QualificationReview{
		AuthoritySHA256:        strings.Repeat("b", 64),
		AcceptanceMatrixSHA256: strings.Repeat("c", 64),
		Findings:               []deliveryevidence.ReviewFinding{}, Completed: true,
	}, QualificationCorrection: &issuedelivery.QualificationCorrection{
		AuthoritySHA256:      strings.Repeat("b", 64),
		ReviewedMatrixSHA256: strings.Repeat("c", 64),
		FindingIDs:           []string{"qualification-1"},
		AcceptanceMatrix: []deliveryevidence.AcceptanceRow{{
			Identity: "criterion-1", Criterion: "observable behavior",
			OwningSeam: "product seam", PositiveEvidence: "positive",
			NegativeEvidence: "negative", FailureEvidence: "failure",
			MutationEvidence: "mutation", CompatibilityEvidence: "compatibility",
			PreservationEvidence: "preservation", MigrationEvidence: "migration",
			State: deliveryevidence.AcceptancePlanned,
		}},
		Evidence: "corrected exact owning seam and planned evidence",
	}}
	ciAttributions := []advanceCIFailureAttribution{{
		CheckIdentity: "Validate Packy-owned code", RunID: 42,
		HeadSHA:     strings.Repeat("a", 40),
		DetailsURL:  "https://github.com/yersonargotev/packy/actions/runs/42",
		Attribution: issuedelivery.FailureInfrastructure,
	}}
	fake := &fakeIssueDeliveryAdvancer{outcomes: []issuedelivery.Outcome{{
		RunID: "run-1", State: issuedelivery.StateNeedsReview, Reason: "candidate review required",
	}}}
	var configured advanceOptions
	cmd := command{AdvanceFactory: func(options advanceOptions) (issueDeliveryAdvancer, error) {
		configured = options
		return fake, nil
	}}
	err := cmd.run(context.Background(), []string{
		"advance", "--repository", repository, "--issue", "361",
		"--decision", write("decision", decision),
		"--review-content", write("reviews", reviews),
		"--ci-attribution", write("ci-attribution", ciAttributions),
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if configured.Decision == nil || *configured.Decision != decision ||
		len(configured.Reviews) != 1 || configured.Reviews[0].Axis != deliveryevidence.ReviewStandards ||
		len(configured.Acceptance) != 1 || configured.Acceptance[0].Identity != "criterion-1" ||
		configured.QualificationReview == nil || !configured.QualificationReview.Completed ||
		configured.QualificationCorrection == nil ||
		configured.QualificationCorrection.FindingIDs[0] != "qualification-1" ||
		len(configured.CIFailureAttributions) != 1 ||
		configured.CIFailureAttributions[0].Attribution != issuedelivery.FailureInfrastructure {
		t.Fatalf("configured semantic content = %#v", configured)
	}
	if len(fake.requests) != 1 || fake.requests[0].Decision == nil ||
		*fake.requests[0].Decision != decision ||
		fake.requests[0].QualificationReview == nil ||
		fake.requests[0].QualificationCorrection == nil {
		t.Fatalf("Advance requests = %#v", fake.requests)
	}

	unknown := filepath.Join(root, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"request_id":"x","unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err = cmd.run(context.Background(), []string{
		"advance", "--repository", repository, "--issue", "361", "--decision", unknown,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown semantic field accepted: %v", err)
	}
}

func TestAdvanceCommandRejectsCallerPhaseSequencingInputs(t *testing.T) {
	repository := t.TempDir()
	cmd := command{AdvanceFactory: func(options advanceOptions) (issueDeliveryAdvancer, error) {
		return &fakeIssueDeliveryAdvancer{}, nil
	}}
	for _, args := range [][]string{
		{"advance", "--repository", repository, "--issue", "361", "qualification"},
		{"advance", "--repository", repository, "--issue", "361", "--risk-profile", "routine"},
		{"advance", "--repository", repository, "--issue", "361", "--spec", "361"},
		{"advance", "--repository", repository, "--issue", "361", "--sandbox-root", t.TempDir()},
		{"advance", "--repository", repository},
	} {
		if err := cmd.run(context.Background(), args, &bytes.Buffer{}); err == nil {
			t.Fatalf("invalid caller sequencing accepted: %v", args)
		}
	}
}

func TestProductionCommandExposesHistoricalSequencingOnlyBehindLegacyV1(t *testing.T) {
	cmd := command{LegacyPrefixRequired: true}
	err := cmd.run(context.Background(), []string{"initialize"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "only through legacy-v1") {
		t.Fatalf("direct legacy command accepted: %v", err)
	}
	err = cmd.run(context.Background(), []string{"legacy-v1", "initialize"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "qualified-bundle") {
		t.Fatalf("explicit legacy-v1 path did not dispatch historical command: %v", err)
	}
	qualificationPath := filepath.Join(t.TempDir(), "v2.json")
	if err = os.WriteFile(
		qualificationPath,
		[]byte(`{"schema":"packy.delivery-evidence/v2","issue_number":361}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	err = cmd.run(context.Background(), []string{
		"legacy-v1", "initialize", "--qualified-bundle", qualificationPath,
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "only schema v1") {
		t.Fatalf("legacy-v1 accepted caller-assembled v2 evidence: %v", err)
	}
}
