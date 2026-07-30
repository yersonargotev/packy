package issuedelivery

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeLocalCompletionGateway struct {
	observation      LocalCompletionObservation
	observeErr       error
	removeErr        error
	deleteErr        error
	fastForwardErr   error
	observeCalls     int
	removeCalls      int
	deleteCalls      int
	fastForwardCalls int
	order            []string
	lastRemove       RemoveManagedWorktreeRequest
	lastDelete       DeleteLocalIssueBranchRequest
	lastFastForward  FastForwardLocalMainRequest
}

func (f *fakeLocalCompletionGateway) ObserveLocalCompletion(
	_ context.Context,
	_ LocalCompletionObserveRequest,
) (LocalCompletionObservation, error) {
	f.observeCalls++
	if f.observeErr != nil {
		return LocalCompletionObservation{}, f.observeErr
	}
	return f.observation, nil
}

func (f *fakeLocalCompletionGateway) EnsureManagedWorktreeAbsent(
	_ context.Context,
	request RemoveManagedWorktreeRequest,
) error {
	f.removeCalls++
	f.order = append(f.order, "worktree")
	f.lastRemove = request
	if f.removeErr != nil {
		return f.removeErr
	}
	for index, worktree := range f.observation.Worktrees {
		if worktree.Path == request.Path {
			f.observation.Worktrees = append(
				f.observation.Worktrees[:index], f.observation.Worktrees[index+1:]...,
			)
			break
		}
	}
	return nil
}

func (f *fakeLocalCompletionGateway) EnsureLocalIssueBranchAbsent(
	_ context.Context,
	request DeleteLocalIssueBranchRequest,
) error {
	f.deleteCalls++
	f.order = append(f.order, "branch")
	f.lastDelete = request
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.observation.LocalBranch = nil
	return nil
}

func (f *fakeLocalCompletionGateway) EnsureLocalMainFastForward(
	_ context.Context,
	request FastForwardLocalMainRequest,
) error {
	f.fastForwardCalls++
	f.order = append(f.order, "main")
	f.lastFastForward = request
	if f.fastForwardErr != nil {
		return f.fastForwardErr
	}
	f.observation.LocalMain.HeadSHA = request.OriginMainSHA
	f.observation.LocalMain.OriginHeadSHA = request.OriginMainSHA
	f.observation.LocalMain.Relation = LocalMainSynced
	return nil
}

func completionFixture(
	t *testing.T,
) (*Module, *fakeGitObserver, *fakeGitHubObserver, *fakeNonLocalGateway, *fakeLocalCompletionGateway, Request, Outcome) {
	t.Helper()
	module, git, remote, request, ready := nonLocalFixture(t)
	tracker := module.github.(*fakeGitHubObserver)
	local := &fakeLocalCompletionGateway{observation: LocalCompletionObservation{
		OperatorStateSHA256: strings.Repeat("9", 64),
		Integration: IntegrationWorkspaceObservation{
			Path: "/repo", Branch: ready.LocalReadiness.Branch, Clean: true,
		},
		Worktrees: []ManagedWorktreeObservation{},
		LocalBranch: &LocalBranchObservation{
			Name: ready.LocalReadiness.Branch, HeadSHA: ready.Candidate.CommitSHA,
		},
		LocalMain: LocalMainObservation{
			Exists: true, HeadSHA: strings.Repeat("a", 40),
			OriginHeadSHA: strings.Repeat("a", 40), Relation: LocalMainSynced, Clean: true,
		},
	}}
	module.localCompletion = local
	return module, git, tracker, remote, local, request, ready
}

func prepareExactMerge(
	t *testing.T,
	module *Module,
	remote *fakeNonLocalGateway,
	local *fakeLocalCompletionGateway,
	request Request,
	ready Outcome,
) Outcome {
	t.Helper()
	advanceToExactPullRequest(t, module, remote, request, ready)
	remote.observation.Checks = exactSuccessfulChecks(ready.Candidate.CommitSHA)
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization
	intent := mustAdvance(t, module, request)
	if intent.NonLocal == nil || intent.NonLocal.MergeIntent == nil ||
		intent.NonLocal.MergeIntent.DispatchedAt != "" || local.observeCalls == 0 {
		t.Fatalf("merge intent outcome=%#v local=%#v", intent, local)
	}
	return intent
}

func adoptExactMerge(
	t *testing.T,
	module *Module,
	remote *fakeNonLocalGateway,
	local *fakeLocalCompletionGateway,
	request Request,
	ready Outcome,
) Outcome {
	t.Helper()
	prepareExactMerge(t, module, remote, local, request, ready)
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization
	dispatched := mustAdvance(t, module, request)
	if dispatched.NonLocal.MergeIntent.DispatchedAt == "" || remote.mergeCalls != 1 {
		t.Fatalf("merge dispatch outcome=%#v remote=%#v", dispatched, remote)
	}
	adopted := mustAdvance(t, module, request)
	if adopted.NonLocal.Merge == nil || adopted.State != StateWaiting {
		t.Fatalf("merge adoption outcome=%#v", adopted)
	}
	return adopted
}

func TestAdvanceDoesNotMergeUntilEveryRequiredCheckIsExactAndGreen(t *testing.T) {
	module, _, _, remote, local, request, ready := completionFixture(t)
	advanceToExactPullRequest(t, module, remote, request, ready)
	checks := exactSuccessfulChecks(ready.Candidate.CommitSHA)
	checks[0].Conclusion = ""
	remote.observation.Checks = checks
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization

	waiting := mustAdvance(t, module, request)
	if waiting.State != StateWaiting || waiting.NonLocal.MergeIntent != nil ||
		remote.mergeCalls != 0 || local.observeCalls != 0 {
		t.Fatalf("pending checks outcome=%#v remote=%#v local=%#v", waiting, remote, local)
	}
}

func TestAdvanceInvalidatesPreparedMergeWhenExactCIStopsBeingGreen(t *testing.T) {
	module, _, _, remote, local, request, ready := completionFixture(t)
	prepared := prepareExactMerge(t, module, remote, local, request, ready)
	if prepared.NonLocal.MergeIntent == nil {
		t.Fatalf("prepared outcome=%#v", prepared)
	}
	checks := exactSuccessfulChecks(ready.Candidate.CommitSHA)
	checks[0].Conclusion = ""
	remote.observation.Checks = checks
	authorization := exactNonLocalAuthorization(ready)
	request.NonLocal = &authorization

	waiting := mustAdvance(t, module, request)
	if waiting.NonLocal.MergeIntent != nil || remote.mergeCalls != 0 {
		t.Fatalf("invalidated outcome=%#v remote=%#v", waiting, remote)
	}
}

func TestAdvanceMergesOnceAndCompletesCleanupInSafeOrder(t *testing.T) {
	module, git, tracker, remote, local, request, ready := completionFixture(t)
	adoptExactMerge(t, module, remote, local, request, ready)
	mergeSHA := remote.observation.Merge.MergeCommitSHA
	remote.observation.OriginMain = &OriginMainObservation{
		HeadSHA: mergeSHA, MergeCommitSHA: mergeSHA,
		CandidateHeadSHA:    ready.Candidate.CommitSHA,
		ContainsMergeCommit: true, ContainsCandidateHead: true,
	}
	tracker.value.State = "CLOSED"
	git.value.Branch, git.value.HeadSHA = "main", mergeSHA
	local.observation.Integration.Branch = "main"
	local.observation.Worktrees = []ManagedWorktreeObservation{{
		Path: "/tmp/packy-delivery-360", Branch: ready.LocalReadiness.Branch,
		HeadSHA: ready.Candidate.CommitSHA, RunID: ready.RunID,
		CandidateID: ready.Candidate.ID, Clean: true,
	}}
	local.observation.LocalMain = LocalMainObservation{
		Exists: true, HeadSHA: strings.Repeat("a", 40), OriginHeadSHA: mergeSHA,
		Relation: LocalMainBehind, Clean: true,
	}
	request.NonLocal = nil

	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	mustAdvance(t, module, request)
	completed := mustAdvance(t, module, request)
	if completed.State != StateCompleted || completed.NonLocal.Completion == nil ||
		remote.mergeCalls != 1 || remote.remoteDeleteCalls != 1 ||
		local.removeCalls != 1 || local.deleteCalls != 1 || local.fastForwardCalls != 1 {
		t.Fatalf("completed=%#v remote=%#v local=%#v", completed, remote, local)
	}
	if got, want := strings.Join(local.order, ","), "worktree,branch,main"; got != want {
		t.Fatalf("cleanup order=%q want=%q", got, want)
	}
	for name, identity := range map[string][2]string{
		"worktree": {local.lastRemove.CommonDir, local.lastRemove.RepositoryPath},
		"branch":   {local.lastDelete.CommonDir, local.lastDelete.RepositoryPath},
		"main":     {local.lastFastForward.CommonDir, local.lastFastForward.RepositoryPath},
	} {
		if identity != [2]string{git.value.CommonDir, "/repo"} {
			t.Fatalf("%s mutation repository identity=%v", name, identity)
		}
	}
	resumed := mustAdvance(t, module, request)
	if resumed.State != StateCompleted || remote.mergeCalls != 1 ||
		remote.remoteDeleteCalls != 1 || local.removeCalls != 1 ||
		local.deleteCalls != 1 || local.fastForwardCalls != 1 {
		t.Fatalf("completed resume=%#v remote=%#v local=%#v", resumed, remote, local)
	}
}

func TestAdvanceAdoptsMatchingCompletedMergeWithoutRepeatingIt(t *testing.T) {
	module, _, _, remote, local, request, ready := completionFixture(t)
	prepareExactMerge(t, module, remote, local, request, ready)
	remote.observation.Merge = &MergeObservation{
		PullRequest: 1, URL: "https://github.com/yersonargotev/packy/pull/1",
		BaseRef: "main", HeadSHA: ready.Candidate.CommitSHA,
		MergeCommitSHA: strings.Repeat("f", 40),
		MergedAt:       "2026-07-30T01:00:00.000000000Z",
	}
	request.NonLocal = nil

	adopted := mustAdvance(t, module, request)
	if adopted.NonLocal.Merge == nil || remote.mergeCalls != 0 {
		t.Fatalf("adopted=%#v remote=%#v", adopted, remote)
	}
}

func TestAdvanceBlocksIncompatibleMergeAndNeverRollsBack(t *testing.T) {
	module, _, tracker, remote, local, request, ready := completionFixture(t)
	prepareExactMerge(t, module, remote, local, request, ready)
	remote.observation.Merge = &MergeObservation{
		PullRequest: 1, URL: "https://github.com/yersonargotev/packy/pull/1",
		BaseRef: "main", HeadSHA: strings.Repeat("d", 40),
		MergeCommitSHA: strings.Repeat("f", 40),
		MergedAt:       "2026-07-30T01:00:00.000000000Z",
	}
	tracker.value.State = "CLOSED"
	request.NonLocal = nil

	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || remote.mergeCalls != 0 ||
		remote.remoteDeleteCalls != 0 || local.removeCalls != 0 {
		t.Fatalf("blocked=%#v remote=%#v local=%#v", blocked, remote, local)
	}
}

func TestAdvanceAfterMergeWaitsForClosureAndBlocksPartialIntegration(t *testing.T) {
	module, _, tracker, remote, local, request, ready := completionFixture(t)
	adoptExactMerge(t, module, remote, local, request, ready)
	request.NonLocal = nil
	waiting := mustAdvance(t, module, request)
	if waiting.State != StateWaiting || remote.remoteDeleteCalls != 0 {
		t.Fatalf("open issue outcome=%#v remote=%#v", waiting, remote)
	}

	tracker.value.State = "CLOSED"
	remote.observation.OriginMain = &OriginMainObservation{
		HeadSHA: strings.Repeat("f", 40), MergeCommitSHA: strings.Repeat("f", 40),
		CandidateHeadSHA: ready.Candidate.CommitSHA, ContainsMergeCommit: false,
		ContainsCandidateHead: true,
	}
	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || remote.remoteDeleteCalls != 0 ||
		local.removeCalls != 0 {
		t.Fatalf("partial integration outcome=%#v remote=%#v local=%#v", blocked, remote, local)
	}
}

func TestAdvancePreservesUnsafeOperatorStateAfterMerge(t *testing.T) {
	module, git, tracker, remote, local, request, ready := completionFixture(t)
	adoptExactMerge(t, module, remote, local, request, ready)
	mergeSHA := remote.observation.Merge.MergeCommitSHA
	remote.observation.OriginMain = &OriginMainObservation{
		HeadSHA: mergeSHA, MergeCommitSHA: mergeSHA,
		CandidateHeadSHA:    ready.Candidate.CommitSHA,
		ContainsMergeCommit: true, ContainsCandidateHead: true,
	}
	remote.observation.Branch = nil
	tracker.value.State = "CLOSED"
	git.value.Branch, git.value.HeadSHA = "main", mergeSHA
	local.observation.Integration.Branch = "main"
	local.observation.LocalBranch = nil
	local.observation.LocalMain = LocalMainObservation{
		Exists: true, HeadSHA: strings.Repeat("d", 40), OriginHeadSHA: mergeSHA,
		Relation: LocalMainDiverged, Clean: true,
	}
	request.NonLocal = nil

	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || local.fastForwardCalls != 0 ||
		local.deleteCalls != 0 || local.removeCalls != 0 {
		t.Fatalf("unsafe operator outcome=%#v local=%#v", blocked, local)
	}
}

func TestAdvanceBlocksUnsafeOrUnownedWorktreeWithoutCleanup(t *testing.T) {
	module, git, tracker, remote, local, request, ready := completionFixture(t)
	adoptExactMerge(t, module, remote, local, request, ready)
	mergeSHA := remote.observation.Merge.MergeCommitSHA
	remote.observation.OriginMain = &OriginMainObservation{
		HeadSHA: mergeSHA, MergeCommitSHA: mergeSHA,
		CandidateHeadSHA:    ready.Candidate.CommitSHA,
		ContainsMergeCommit: true, ContainsCandidateHead: true,
	}
	remote.observation.Branch = nil
	tracker.value.State = "CLOSED"
	git.value.Branch, git.value.HeadSHA = "main", mergeSHA
	local.observation.Integration.Branch = "main"
	local.observation.Worktrees = []ManagedWorktreeObservation{{
		Path: "/tmp/operator-worktree", Branch: ready.LocalReadiness.Branch,
		HeadSHA: ready.Candidate.CommitSHA, RunID: "other-run",
		CandidateID: ready.Candidate.ID, Clean: true,
	}}
	request.NonLocal = nil

	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || local.removeCalls != 0 ||
		local.deleteCalls != 0 {
		t.Fatalf("unsafe worktree outcome=%#v local=%#v", blocked, local)
	}
}

func TestAdvanceAfterMergeRejectsRepairAndCannotReturnToCandidateFlow(t *testing.T) {
	module, _, _, remote, local, request, ready := completionFixture(t)
	adopted := adoptExactMerge(t, module, remote, local, request, ready)
	request.NonLocal = nil
	request.Repair = &RepairDecision{
		CandidateID: adopted.Candidate.ID, Class: RepairCandidateChanging,
	}
	if _, err := module.Advance(context.Background(), request); err == nil ||
		!strings.Contains(err.Error(), "post-merge") {
		t.Fatalf("post-merge repair err=%v", err)
	}
}

func TestAdvancePersistsMergeLatchWhenLocalCompletionAdapterIsUnavailable(t *testing.T) {
	module, _, tracker, remote, local, request, ready := completionFixture(t)
	prepareExactMerge(t, module, remote, local, request, ready)
	remote.observation.Merge = &MergeObservation{
		PullRequest: 1, URL: "https://github.com/yersonargotev/packy/pull/1",
		BaseRef: "main", HeadSHA: ready.Candidate.CommitSHA,
		MergeCommitSHA: strings.Repeat("f", 40),
		MergedAt:       "2026-07-30T01:00:00.000000000Z",
	}
	module.localCompletion = nil
	tracker.value.State = "CLOSED"
	request.NonLocal = nil

	adopted := mustAdvance(t, module, request)
	if adopted.NonLocal.Merge == nil {
		t.Fatalf("merge latch was not persisted: %#v", adopted)
	}
	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || blocked.NonLocal.Merge == nil ||
		!strings.Contains(blocked.Reason, "candidate flow remains disabled") {
		t.Fatalf("missing local adapter outcome=%#v", blocked)
	}
}

func TestAdvanceBlocksExistingPullRequestWithoutRemoteObserverBeforeAuthorityRecompile(t *testing.T) {
	module, _, tracker, remote, _, request, ready := completionFixture(t)
	advanceToExactPullRequest(t, module, remote, request, ready)
	tracker.value.State = "CLOSED"
	module.nonlocal = nil
	request.NonLocal = nil

	blocked := mustAdvance(t, module, request)
	if blocked.State != StateBlocked || blocked.RunID != ready.RunID ||
		blocked.NonLocal == nil || blocked.NonLocal.PullRequest == nil ||
		strings.Contains(blocked.Reason, "supersed") ||
		!strings.Contains(blocked.Reason, "non-local observer") {
		t.Fatalf("missing remote observer outcome=%#v", blocked)
	}
}

func TestAdvanceAdoptsCleanupThatCompletedBeforePersisting(t *testing.T) {
	module, git, tracker, remote, local, request, ready := completionFixture(t)
	adoptExactMerge(t, module, remote, local, request, ready)
	mergeSHA := remote.observation.Merge.MergeCommitSHA
	remote.observation.OriginMain = &OriginMainObservation{
		HeadSHA: mergeSHA, MergeCommitSHA: mergeSHA,
		CandidateHeadSHA:    ready.Candidate.CommitSHA,
		ContainsMergeCommit: true, ContainsCandidateHead: true,
	}
	tracker.value.State = "CLOSED"
	git.value.Branch, git.value.HeadSHA = "main", mergeSHA
	local.observation.Integration.Branch = "main"
	local.observation.Worktrees = []ManagedWorktreeObservation{}
	local.observation.LocalBranch = nil
	local.observation.LocalMain = LocalMainObservation{
		Exists: true, HeadSHA: mergeSHA, OriginHeadSHA: mergeSHA,
		Relation: LocalMainSynced, Clean: true,
	}
	remote.remoteDeleteErr = errors.New("connection lost after delete")
	request.NonLocal = nil

	waiting := mustAdvance(t, module, request)
	if waiting.State != StateWaiting || remote.observation.Branch != nil {
		t.Fatalf("ambiguous delete outcome=%#v remote=%#v", waiting, remote)
	}
	completed := mustAdvance(t, module, request)
	if completed.State != StateCompleted || remote.remoteDeleteCalls != 1 {
		t.Fatalf("cleanup adoption outcome=%#v remote=%#v", completed, remote)
	}
}

func TestValidateRunRejectsCompletedStateWithoutFinalCompletionProof(t *testing.T) {
	module, git, _, remote, local, request, ready := completionFixture(t)
	adoptExactMerge(t, module, remote, local, request, ready)
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
	record.State = StateCompleted
	record.Timing[len(record.Timing)-1].To = StateCompleted
	if err := validateRun(record); err == nil ||
		!strings.Contains(err.Error(), "completion report") {
		t.Fatalf("validateRun err=%v", err)
	}
}
