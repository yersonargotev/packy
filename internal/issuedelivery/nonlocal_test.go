package issuedelivery

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeNonLocalGateway struct {
	observation       NonLocalObservation
	observeErr        error
	pushErr           error
	createErr         error
	retryErr          error
	observeCalls      int
	pushCalls         int
	createCalls       int
	retryCalls        int
	hideEnsuredPR     bool
	mergeCalls        int
	remoteDeleteCalls int
	mergeErr          error
	hideEnsuredMerge  bool
	remoteDeleteErr   error
}

func (f *fakeNonLocalGateway) ObserveNonLocal(
	_ context.Context,
	_ NonLocalObserveRequest,
) (NonLocalObservation, error) {
	f.observeCalls++
	if f.observeErr != nil {
		return NonLocalObservation{}, f.observeErr
	}
	return f.observation, nil
}

func (f *fakeNonLocalGateway) PushIssueBranch(
	_ context.Context,
	request PushIssueBranchRequest,
) error {
	f.pushCalls++
	if f.pushErr != nil {
		return f.pushErr
	}
	f.observation.Branch = &RemoteBranchObservation{Name: request.Branch, HeadSHA: request.HeadSHA}
	return nil
}

func (f *fakeNonLocalGateway) EnsurePullRequest(
	_ context.Context,
	request EnsurePullRequestRequest,
) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	pullRequest := RemotePullRequestObservation{
		Number: 1, URL: "https://github.com/yersonargotev/packy/pull/1",
		State: "OPEN", BaseRef: request.BaseRef, BaseSHA: strings.Repeat("a", 40),
		HeadBranch: request.HeadBranch,
		HeadSHA:    request.HeadSHA, HeadRepositoryNodeID: request.Repository.NodeID,
		ClosingIssue: request.Issue.Number,
	}
	if !f.hideEnsuredPR {
		f.observation.PullRequests = []RemotePullRequestObservation{pullRequest}
	}
	return nil
}

func (f *fakeNonLocalGateway) RetryInfrastructureCheck(
	_ context.Context,
	_ RetryInfrastructureCheckRequest,
) error {
	f.retryCalls++
	return f.retryErr
}

func (f *fakeNonLocalGateway) EnsureMerge(
	_ context.Context,
	request EnsureMergeRequest,
) error {
	f.mergeCalls++
	if f.mergeErr != nil {
		return f.mergeErr
	}
	if !f.hideEnsuredMerge {
		url := ""
		if len(f.observation.PullRequests) == 1 {
			url = f.observation.PullRequests[0].URL
		}
		f.observation.Merge = &MergeObservation{
			PullRequest: request.PullRequest, URL: url, BaseRef: "main",
			HeadSHA: request.HeadSHA, MergeCommitSHA: strings.Repeat("f", 40),
			MergedAt: "2026-07-30T01:00:00.000000000Z",
		}
	}
	return nil
}

func (f *fakeNonLocalGateway) EnsureRemoteIssueBranchAbsent(
	_ context.Context,
	_ DeleteRemoteIssueBranchRequest,
) error {
	f.remoteDeleteCalls++
	f.observation.Branch = nil
	return f.remoteDeleteErr
}

func nonLocalFixture(t *testing.T) (*Module, *fakeGitObserver, *fakeNonLocalGateway, Request, Outcome) {
	t.Helper()
	module, git, _, _, _ := assuranceFixture(t)
	gateway := &fakeNonLocalGateway{
		observation: NonLocalObservation{
			PullRequests: []RemotePullRequestObservation{},
			Checks:       []CICheckObservation{},
		},
	}
	module.nonlocal = gateway
	request := Request{RepositoryPath: "/repo", IssueNumber: 357}
	var ready Outcome
	for range 8 {
		ready = mustAdvance(t, module, request)
		if ready.LocalReadiness != nil {
			break
		}
	}
	if ready.LocalReadiness == nil || ready.Candidate == nil {
		t.Fatalf("fixture did not reach local readiness: %#v", ready)
	}
	return module, git, gateway, request, ready
}

func exactNonLocalAuthorization(ready Outcome) NonLocalAuthorization {
	return NonLocalAuthorization{
		RunID: ready.RunID, CandidateID: ready.Candidate.ID,
		CommitSHA: ready.Candidate.CommitSHA, TreeSHA: ready.Candidate.TreeSHA,
		Branch: ready.LocalReadiness.Branch, LocalReadyAt: ready.LocalReadiness.ReadyAt,
	}
}

func exactSuccessfulChecks(head string) []CICheckObservation {
	checks := successfulCIChecks()
	for index := range checks {
		checks[index].HeadSHA = head
		if checks[index].Identity != requiredCIGovernance {
			checks[index].Workflow.DefinitionRef = head
		}
	}
	return checks
}

func advanceToExactPullRequest(
	t *testing.T,
	module *Module,
	gateway *fakeNonLocalGateway,
	request Request,
	ready Outcome,
) Outcome {
	t.Helper()
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization
	mustAdvance(t, module, request)
	branch := mustAdvance(t, module, request)
	if branch.NonLocal == nil || branch.NonLocal.Branch == nil {
		t.Fatalf("branch outcome=%#v", branch)
	}
	var pullRequest Outcome
	for range 4 {
		pullRequest = mustAdvance(t, module, request)
		if pullRequest.NonLocal != nil && pullRequest.NonLocal.PullRequest != nil {
			break
		}
	}
	if pullRequest.NonLocal == nil || pullRequest.NonLocal.PullRequest == nil {
		t.Fatalf("pull request outcome=%#v", pullRequest)
	}
	if gateway.pushCalls != 1 || gateway.createCalls != 1 {
		t.Fatalf("remote mutations push=%d create=%d", gateway.pushCalls, gateway.createCalls)
	}
	return pullRequest
}

func TestAdvanceRequiresFreshExplicitAuthorizationBeforeNonLocalMutation(t *testing.T) {
	module, git, gateway, request, ready := nonLocalFixture(t)

	unauthorized := mustAdvance(t, module, request)
	if unauthorized.State != StateWaiting || !strings.Contains(unauthorized.Reason, "authorization") ||
		gateway.observeCalls != 0 || gateway.pushCalls != 0 || gateway.createCalls != 0 {
		t.Fatalf("unauthorized outcome=%#v gateway=%#v", unauthorized, gateway)
	}

	authorization := exactNonLocalAuthorization(ready)
	authorization.CommitSHA = strings.Repeat("f", 40)
	request.NonLocal = &authorization
	if _, err := module.Advance(context.Background(), request); err == nil ||
		!strings.Contains(err.Error(), "exact local readiness") {
		t.Fatalf("mismatched authorization err=%v", err)
	}
	if gateway.observeCalls != 0 || gateway.pushCalls != 0 {
		t.Fatalf("mismatched authorization mutated gateway=%#v", gateway)
	}

	authorization = exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization
	recorded := mustAdvance(t, module, request)
	if recorded.NonLocal == nil || recorded.NonLocal.Authorization != authorization ||
		gateway.observeCalls != 0 || gateway.pushCalls != 0 {
		t.Fatalf("recorded authority outcome=%#v gateway=%#v", recorded, gateway)
	}

	git.value.WorkspaceClean = false
	if _, err := module.Advance(context.Background(), request); err == nil ||
		!strings.Contains(err.Error(), "clean workspace") ||
		gateway.observeCalls != 0 || gateway.pushCalls != 0 {
		t.Fatalf("stale err=%v gateway=%#v", err, gateway)
	}
}

func TestAdvancePushesAndCreatesOnceThenAdoptsExactRemoteState(t *testing.T) {
	module, _, gateway, request, ready := nonLocalFixture(t)
	pullRequest := advanceToExactPullRequest(t, module, gateway, request, ready)
	if pullRequest.NonLocal.PullRequest.HeadSHA != ready.Candidate.CommitSHA {
		t.Fatalf("pull request=%#v", pullRequest.NonLocal.PullRequest)
	}

	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization
	resumed := mustAdvance(t, module, request)
	if resumed.State != StateWaiting || gateway.pushCalls != 1 || gateway.createCalls != 1 {
		t.Fatalf("resume outcome=%#v gateway=%#v", resumed, gateway)
	}
}

func TestAdvanceAdoptsMatchingBranchAndPullRequestWithoutMutation(t *testing.T) {
	module, _, gateway, request, ready := nonLocalFixture(t)
	gateway.observation.Branch = &RemoteBranchObservation{
		Name: ready.LocalReadiness.Branch, HeadSHA: ready.Candidate.CommitSHA,
	}
	gateway.observation.PullRequests = []RemotePullRequestObservation{{
		Number: 9, URL: "https://github.com/yersonargotev/packy/pull/9",
		State: "OPEN", BaseRef: "main", BaseSHA: strings.Repeat("a", 40),
		HeadBranch: ready.LocalReadiness.Branch,
		HeadSHA:    ready.Candidate.CommitSHA, HeadRepositoryNodeID: "R1", ClosingIssue: 357,
	}}
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	adopted := mustAdvance(t, module, request)
	if adopted.NonLocal == nil || adopted.NonLocal.PullRequest == nil ||
		gateway.pushCalls != 0 || gateway.createCalls != 0 {
		t.Fatalf("adopted outcome=%#v gateway=%#v", adopted, gateway)
	}
}

func TestAdvanceBlocksIncompatibleRemoteIdentityWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeNonLocalGateway, Outcome)
	}{
		{
			name: "unrelated branch head",
			mutate: func(gateway *fakeNonLocalGateway, ready Outcome) {
				gateway.observation.Branch = &RemoteBranchObservation{
					Name: ready.LocalReadiness.Branch, HeadSHA: strings.Repeat("f", 40),
				}
			},
		},
		{
			name: "wrong pull request base",
			mutate: func(gateway *fakeNonLocalGateway, ready Outcome) {
				gateway.observation.Branch = &RemoteBranchObservation{
					Name: ready.LocalReadiness.Branch, HeadSHA: ready.Candidate.CommitSHA,
				}
				gateway.observation.PullRequests = []RemotePullRequestObservation{{
					Number: 2, URL: "https://github.com/yersonargotev/packy/pull/2",
					State: "OPEN", BaseRef: "release", BaseSHA: strings.Repeat("a", 40),
					HeadBranch: ready.LocalReadiness.Branch,
					HeadSHA:    ready.Candidate.CommitSHA, HeadRepositoryNodeID: "R1", ClosingIssue: 357,
				}}
			},
		},
		{
			name: "untrusted pull request URL",
			mutate: func(gateway *fakeNonLocalGateway, ready Outcome) {
				gateway.observation.Branch = &RemoteBranchObservation{
					Name: ready.LocalReadiness.Branch, HeadSHA: ready.Candidate.CommitSHA,
				}
				gateway.observation.PullRequests = []RemotePullRequestObservation{{
					Number: 2, URL: "https://example.com/yersonargotev/packy/pull/2",
					State: "OPEN", BaseRef: "main", BaseSHA: strings.Repeat("a", 40),
					HeadBranch: ready.LocalReadiness.Branch,
					HeadSHA:    ready.Candidate.CommitSHA, HeadRepositoryNodeID: "R1", ClosingIssue: 357,
				}}
			},
		},
		{
			name: "other GitHub repository URL",
			mutate: func(gateway *fakeNonLocalGateway, ready Outcome) {
				gateway.observation.Branch = &RemoteBranchObservation{
					Name: ready.LocalReadiness.Branch, HeadSHA: ready.Candidate.CommitSHA,
				}
				gateway.observation.PullRequests = []RemotePullRequestObservation{{
					Number: 2, URL: "https://github.com/other/fork/pull/2",
					State: "OPEN", BaseRef: "main", BaseSHA: strings.Repeat("a", 40),
					HeadBranch: ready.LocalReadiness.Branch,
					HeadSHA:    ready.Candidate.CommitSHA, HeadRepositoryNodeID: "R1", ClosingIssue: 357,
				}}
			},
		},
		{
			name: "duplicate pull requests",
			mutate: func(gateway *fakeNonLocalGateway, ready Outcome) {
				gateway.observation.Branch = &RemoteBranchObservation{
					Name: ready.LocalReadiness.Branch, HeadSHA: ready.Candidate.CommitSHA,
				}
				pr := RemotePullRequestObservation{
					Number: 2, URL: "https://github.com/yersonargotev/packy/pull/2",
					State: "OPEN", BaseRef: "main", BaseSHA: strings.Repeat("a", 40),
					HeadBranch: ready.LocalReadiness.Branch,
					HeadSHA:    ready.Candidate.CommitSHA, HeadRepositoryNodeID: "R1", ClosingIssue: 357,
				}
				gateway.observation.PullRequests = []RemotePullRequestObservation{pr, pr}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, _, gateway, request, ready := nonLocalFixture(t)
			tt.mutate(gateway, ready)
			authorization := exactNonLocalAuthorization(ready)
			request.NonLocal = &authorization
			mustAdvance(t, module, request)
			var blocked Outcome
			for range 2 {
				blocked = mustAdvance(t, module, request)
				if blocked.State == StateBlocked {
					break
				}
			}
			if blocked.State != StateBlocked || gateway.pushCalls != 0 || gateway.createCalls != 0 {
				t.Fatalf("outcome=%#v gateway=%#v", blocked, gateway)
			}
		})
	}
}

func TestAdvanceBlocksWrongBranchNameBeforeUpdatingPriorCandidateHead(t *testing.T) {
	module, git, gateway, request, firstReady := nonLocalFixture(t)
	git.value.HeadSHA = strings.Repeat("d", 40)
	git.value.TreeSHA = strings.Repeat("e", 40)
	var secondReady Outcome
	for range 8 {
		secondReady = mustAdvance(t, module, request)
		if secondReady.LocalReadiness != nil &&
			secondReady.LocalReadiness.CommitSHA == git.value.HeadSHA {
			break
		}
	}
	if secondReady.LocalReadiness == nil {
		t.Fatalf("second candidate did not reach readiness: %#v", secondReady)
	}
	gateway.observation.Branch = &RemoteBranchObservation{
		Name: "chore/issue-999-incompatible", HeadSHA: firstReady.Candidate.CommitSHA,
	}
	authorization := exactNonLocalAuthorization(secondReady)
	request.NonLocal = &authorization
	mustAdvance(t, module, request)
	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || gateway.pushCalls != 0 {
		t.Fatalf("wrong-name outcome=%#v gateway=%#v", blocked, gateway)
	}
}

func TestAdvanceMonitorsRequiredCIForExactHead(t *testing.T) {
	module, _, gateway, request, ready := nonLocalFixture(t)
	advanceToExactPullRequest(t, module, gateway, request, ready)
	gateway.observation.Checks = exactSuccessfulChecks(ready.Candidate.CommitSHA)
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization

	green := mustAdvance(t, module, request)
	if green.State != StateWaiting || green.NonLocal.CIStatus != string(CISuccess) ||
		!strings.Contains(green.Reason, "awaiting merge") {
		t.Fatalf("green outcome=%#v", green)
	}
	for _, check := range green.NonLocal.Checks {
		if check.HeadSHA != ready.Candidate.CommitSHA {
			t.Fatalf("stale check persisted: %#v", check)
		}
	}
}

func TestAdvanceRetriesInfrastructureFailureOnlyOnceWithoutInvalidatingCandidate(t *testing.T) {
	module, _, gateway, request, ready := nonLocalFixture(t)
	advanceToExactPullRequest(t, module, gateway, request, ready)
	checks := exactSuccessfulChecks(ready.Candidate.CommitSHA)
	checks[0].Conclusion = "failure"
	checks[0].FailureAttribution = FailureInfrastructure
	gateway.observation.Checks = checks
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization

	first := mustAdvance(t, module, request)
	second := mustAdvance(t, module, request)
	if gateway.retryCalls != 1 || len(second.NonLocal.Retries) != 1 ||
		first.LocalReadiness == nil || second.LocalReadiness == nil ||
		second.Candidate.Exhaustive == nil || len(second.Candidate.Acceptance) == 0 {
		t.Fatalf("first=%#v second=%#v gateway=%#v", first, second, gateway)
	}
}

func TestFailedCheckSelectionCannotRetrySuccessfulAttributedRun(t *testing.T) {
	checks := exactSuccessfulChecks(strings.Repeat("b", 40))
	checks[0].FailureAttribution = FailureInfrastructure
	checks[1].Conclusion = "failure"
	checks[1].FailureAttribution = FailureInfrastructure
	failure := firstFailedAttributedCheck(checks, strings.Repeat("b", 40), FailureInfrastructure)
	if failure == nil || failure.Identity != checks[1].Identity || failure.RunID != checks[1].RunID {
		t.Fatalf("selected failure=%#v checks=%#v", failure, checks)
	}
}

func TestAdvanceCandidateFailureInvalidatesReadinessUntilHeadChanges(t *testing.T) {
	module, git, gateway, request, ready := nonLocalFixture(t)
	advanceToExactPullRequest(t, module, gateway, request, ready)
	checks := exactSuccessfulChecks(ready.Candidate.CommitSHA)
	checks[0].Conclusion = "failure"
	checks[0].FailureAttribution = FailureCandidate
	gateway.observation.Checks = checks
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization

	failed := mustAdvance(t, module, request)
	if failed.State != StateNeedsReview || failed.LocalReadiness != nil ||
		failed.Candidate.Exhaustive != nil || len(failed.Candidate.Acceptance) != 0 ||
		failed.NonLocal.CandidateFailure == nil {
		t.Fatalf("candidate failure outcome=%#v", failed)
	}
	repairRequest := request
	repairRequest.NonLocal = nil
	repairRequest.Repair = &RepairDecision{
		CandidateID: failed.Candidate.ID, Class: RepairCandidateChanging,
	}
	if _, err := module.Advance(context.Background(), repairRequest); err == nil ||
		strings.Contains(err.Error(), "post-merge") ||
		!strings.Contains(err.Error(), "pending candidate") {
		t.Fatalf("pre-merge repair routing err=%v", err)
	}
	request.NonLocal = nil
	unchanged := mustAdvance(t, module, request)
	if unchanged.State != StateNeedsReview || !strings.Contains(unchanged.Reason, "candidate-changing") {
		t.Fatalf("unchanged candidate outcome=%#v", unchanged)
	}

	git.value.HeadSHA = strings.Repeat("d", 40)
	git.value.TreeSHA = strings.Repeat("e", 40)
	changed := mustAdvance(t, module, request)
	if changed.Candidate == nil || changed.Candidate.ID == ready.Candidate.ID ||
		changed.Candidate.CommitSHA != git.value.HeadSHA || changed.Candidate.TreeSHA != git.value.TreeSHA ||
		changed.NonLocal != nil {
		t.Fatalf("changed candidate outcome=%#v", changed)
	}
}

func TestAdvanceRetriesTransientRemoteFailuresWithoutDuplicatingEffects(t *testing.T) {
	module, _, gateway, request, ready := nonLocalFixture(t)
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization
	mustAdvance(t, module, request)

	gateway.pushErr = errors.New("network")
	waiting := mustAdvance(t, module, request)
	if waiting.State != StateWaiting || gateway.pushCalls != 1 {
		t.Fatalf("push failure outcome=%#v gateway=%#v", waiting, gateway)
	}
	gateway.pushErr = nil
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	if gateway.pushCalls != 2 || gateway.createCalls != 1 {
		t.Fatalf("recovered gateway=%#v", gateway)
	}
}

func TestAdvanceDoesNotDuplicatePullRequestDuringEventualVisibility(t *testing.T) {
	module, _, gateway, request, ready := nonLocalFixture(t)
	gateway.hideEnsuredPR = true
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	dispatched := mustAdvance(t, module, request)
	if gateway.createCalls != 1 || dispatched.NonLocal.PullRequestIntent == nil ||
		dispatched.NonLocal.PullRequestIntent.DispatchedAt == "" {
		t.Fatalf("dispatch outcome=%#v gateway=%#v", dispatched, gateway)
	}
	for range 3 {
		waiting := mustAdvance(t, module, request)
		if waiting.State != StateWaiting || waiting.NonLocal.PullRequest != nil {
			t.Fatalf("eventual visibility outcome=%#v", waiting)
		}
	}
	if gateway.createCalls != 1 {
		t.Fatalf("pull request creation duplicated: gateway=%#v", gateway)
	}
}

func TestRiskEscalationDiscardsNonLocalAuthorizationAndRequiresFreshReadiness(t *testing.T) {
	module, _, gateway, request, ready := nonLocalFixture(t)
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization
	recorded := mustAdvance(t, module, request)
	if recorded.NonLocal == nil {
		t.Fatalf("authorization was not recorded: %#v", recorded)
	}

	risk := module.risk.(*fakeCandidateRiskObserver)
	risk.effects = []EffectObservation{{
		Effect: EffectOrdinaryBehavior, Evidence: "standard behavior", Complete: true,
	}}
	escalated := mustAdvance(t, module, request)
	if escalated.NonLocal != nil || escalated.LocalReadiness != nil ||
		escalated.State != StateNeedsReview {
		t.Fatalf("escalated outcome=%#v", escalated)
	}
	if gateway.observeCalls != 0 || gateway.pushCalls != 0 {
		t.Fatalf("risk escalation reached gateway=%#v", gateway)
	}

	request.NonLocal = nil
	var refreshed Outcome
	for range 8 {
		refreshed = mustAdvance(t, module, request)
		if refreshed.LocalReadiness != nil {
			break
		}
	}
	if refreshed.LocalReadiness == nil ||
		refreshed.LocalReadiness.ReadyAt == authorization.LocalReadyAt {
		t.Fatalf("fresh readiness=%#v", refreshed)
	}
	freshAuthorization := exactNonLocalAuthorization(refreshed)
	request.NonLocal = &freshAuthorization
	fresh := mustAdvance(t, module, request)
	if fresh.NonLocal == nil || fresh.NonLocal.Authorization != freshAuthorization {
		t.Fatalf("fresh authorization outcome=%#v", fresh)
	}
}

func TestValidateRunRejectsNonLocalAuthorityWithoutReadinessOrCandidateFailure(t *testing.T) {
	module, git, _, request, ready := nonLocalFixture(t)
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization
	mustAdvance(t, module, request)

	var record runRecord
	err := module.store.withIssueLock(context.Background(), git.value.CommonDir, 357, func(store lockedIssueStore) error {
		_, data, found, loadErr := store.loadActive()
		if loadErr != nil {
			return loadErr
		}
		if !found {
			return errors.New("active run not found")
		}
		decoded, decodeErr := decodeRun(data)
		record = decoded
		return decodeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	record.LocalReadiness = nil
	if err := validateRun(record); err == nil ||
		!strings.Contains(err.Error(), "lacks exact local readiness") {
		t.Fatalf("validateRun err=%v", err)
	}
}
