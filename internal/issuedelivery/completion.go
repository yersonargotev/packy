package issuedelivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const mergeMethod = "merge"

func (m *Module) resumeMergedBeforeAuthority(
	ctx context.Context,
	store lockedIssueStore,
	record runRecord,
	git GitObservation,
	tracker TrackerObservation,
	request Request,
) (Outcome, bool, error) {
	if record.NonLocal == nil || record.NonLocal.PullRequest == nil {
		return Outcome{}, false, nil
	}
	candidate := latestCandidate(&record)
	if candidate == nil {
		return Outcome{}, true, errors.New("post-merge delivery lacks its exact candidate")
	}
	if record.NonLocal.Merge != nil && invalidPostMergeRequest(record, request) {
		return Outcome{}, true, errors.New("post-merge delivery accepts no decision, repair, or changed authorization")
	}
	observeRequest := nonLocalObserveRequest(record, *candidate)
	observation, err := m.nonlocal.ObserveNonLocal(ctx, observeRequest)
	if err != nil {
		if record.NonLocal.Merge != nil || !strings.EqualFold(tracker.State, "OPEN") {
			outcome, persistErr := m.persistAssuranceTransition(
				store, record, StateWaiting,
				"post-merge observation failed; retry verification without rollback",
				"post-merge-observation",
			)
			return outcome, true, persistErr
		}
		return Outcome{}, false, nil
	}
	if observation.Merge == nil {
		if record.NonLocal.Merge != nil {
			outcome, persistErr := m.persistAssuranceTransition(
				store, record, StateBlocked,
				"confirmed merge is absent from current observation; preserve external state for inspection",
				"post-merge-observation",
			)
			return outcome, true, persistErr
		}
		if !strings.EqualFold(tracker.State, "OPEN") {
			outcome, persistErr := m.persistAssuranceTransition(
				store, record, StateBlocked,
				"issue closed without an exact matching merge; preserve external state for inspection",
				"post-merge-observation",
			)
			return outcome, true, persistErr
		}
		return Outcome{}, false, nil
	}
	if err := validateMergeObservation(record, *candidate, *observation.Merge); err != nil {
		outcome, persistErr := m.persistAssuranceTransition(
			store, record, StateBlocked, err.Error(), "merge-adoption",
		)
		return outcome, true, persistErr
	}
	if record.NonLocal.Merge == nil {
		if err := validateStoredMergeGate(record, *candidate); err != nil {
			outcome, persistErr := m.persistAssuranceTransition(
				store, record, StateBlocked, err.Error(), "merge-adoption",
			)
			return outcome, true, persistErr
		}
		record.NonLocal.Merge = mergeProofFromObservation(*observation.Merge, m.clock.Now())
		outcome, persistErr := m.persistAssuranceTransition(
			store, record, StateWaiting,
			"exact completed merge is adopted; only verification and cleanup may continue",
			"merge-adoption",
		)
		return outcome, true, persistErr
	}
	if !sameMergeProof(*record.NonLocal.Merge, *observation.Merge) {
		outcome, persistErr := m.persistAssuranceTransition(
			store, record, StateBlocked,
			"observed merge is incompatible with the irreversible merge proof; no rollback attempted",
			"post-merge-observation",
		)
		return outcome, true, persistErr
	}
	if record.NonLocal.MergeIntent == nil {
		if m.localCompletion == nil {
			outcome, persistErr := m.persistAssuranceTransition(
				store, record, StateBlocked,
				"confirmed merge requires a local verification and cleanup adapter; candidate flow remains disabled",
				"post-merge-observation",
			)
			return outcome, true, persistErr
		}
		local, localErr := m.localCompletion.ObserveLocalCompletion(ctx, localCompletionRequest(
			record, *candidate, git.CommonDir, request.RepositoryPath,
			observation.Merge.MergeCommitSHA,
		))
		if localErr != nil {
			outcome, persistErr := m.persistAssuranceTransition(
				store, record, StateWaiting,
				"matching merge is proved; retry operator-state capture before cleanup",
				"merge-adoption",
			)
			return outcome, true, persistErr
		}
		if err := validatePreMergeLocalObservation(local, git.CommonDir, request.RepositoryPath); err != nil {
			outcome, persistErr := m.persistAssuranceTransition(
				store, record, StateBlocked, err.Error(), "merge-adoption",
			)
			return outcome, true, persistErr
		}
		record.NonLocal.MergeIntent = newMergeIntent(
			record, *candidate, local.OperatorStateSHA256, m.clock.Now(),
		)
		outcome, persistErr := m.persistAssuranceTransition(
			store, record, StateWaiting,
			"post-merge operator-state baseline is recorded before cleanup",
			"merge-adoption",
		)
		return outcome, true, persistErr
	}
	if m.localCompletion == nil {
		outcome, persistErr := m.persistAssuranceTransition(
			store, record, StateBlocked,
			"confirmed merge requires a local verification and cleanup adapter; candidate flow remains disabled",
			"post-merge-observation",
		)
		return outcome, true, persistErr
	}
	outcome, err := m.advancePostMerge(ctx, store, record, git, tracker, request, observation, *candidate)
	return outcome, true, err
}

func invalidPostMergeRequest(record runRecord, request Request) bool {
	return request.Decision != nil || request.Repair != nil ||
		request.NonLocal != nil && *request.NonLocal != record.NonLocal.Authorization
}

func (m *Module) advanceMerge(
	ctx context.Context,
	store lockedIssueStore,
	record runRecord,
	candidate Candidate,
	commonDir string,
	repositoryPath string,
) (Outcome, error) {
	if m.localCompletion == nil {
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"required CI is green for the exact candidate HEAD; awaiting merge authority",
			"ci-success",
		)
	}
	local, err := m.localCompletion.ObserveLocalCompletion(ctx, localCompletionRequest(
		record, candidate, commonDir, repositoryPath, "",
	))
	if err != nil {
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"pre-merge operator state observation failed; retry without mutation",
			"merge-readiness",
		)
	}
	if err := validatePreMergeLocalObservation(local, commonDir, repositoryPath); err != nil {
		return m.persistAssuranceTransition(store, record, StateBlocked, err.Error(), "merge-readiness")
	}
	if record.NonLocal.MergeIntent == nil {
		record.NonLocal.MergeIntent = newMergeIntent(
			record, candidate, local.OperatorStateSHA256, m.clock.Now(),
		)
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"exact merge intent is recorded before mutation",
			"merge",
		)
	}
	intent := record.NonLocal.MergeIntent
	if local.OperatorStateSHA256 != intent.OperatorStateSHA256 {
		return m.persistAssuranceTransition(
			store, record, StateBlocked,
			"operator state changed after merge intent; preserve it and requalify",
			"merge-readiness",
		)
	}
	if intent.DispatchedAt != "" {
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"exact merge was dispatched; wait to adopt the completed merge",
			"merge",
		)
	}
	if err := m.nonlocal.EnsureMerge(ctx, EnsureMergeRequest{
		RunID: record.ID, Repository: record.Repository, Issue: record.Issue,
		CandidateID: candidate.ID, IdempotencyKey: intent.IdempotencyKey,
		PullRequest: intent.PullRequest, HeadSHA: intent.HeadSHA, BaseSHA: intent.BaseSHA,
		Method: intent.Method,
	}); err != nil {
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"exact merge dispatch failed; retry the same idempotent request",
			"merge",
		)
	}
	intent.DispatchedAt = m.clock.Now().UTC().Format(timeFormat)
	return m.persistAssuranceTransition(
		store, record, StateWaiting,
		"exact merge was dispatched; wait to adopt the completed merge",
		"merge",
	)
}

func (m *Module) advancePostMerge(
	ctx context.Context,
	store lockedIssueStore,
	record runRecord,
	git GitObservation,
	tracker TrackerObservation,
	request Request,
	remote NonLocalObservation,
	candidate Candidate,
) (Outcome, error) {
	merge := record.NonLocal.Merge
	if merge == nil {
		return Outcome{}, errors.New("post-merge delivery requires an irreversible merge proof")
	}
	if !strings.EqualFold(tracker.State, "CLOSED") {
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"exact merge is proved; waiting for issue closure observation",
			"integration-verification",
		)
	}
	if remote.OriginMain == nil {
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"origin/main integration observation is not yet available",
			"integration-verification",
		)
	}
	if err := validateOriginMain(*remote.OriginMain, *merge, candidate.CommitSHA); err != nil {
		return m.persistAssuranceTransition(
			store, record, StateBlocked, err.Error(), "integration-verification",
		)
	}
	if remote.Branch != nil {
		if err := validateRemoteBranch(*remote.Branch, record.NonLocal.Authorization.Branch, candidate.CommitSHA); err != nil {
			return m.persistAssuranceTransition(store, record, StateBlocked, err.Error(), "remote-cleanup")
		}
		if err := m.nonlocal.EnsureRemoteIssueBranchAbsent(ctx, DeleteRemoteIssueBranchRequest{
			RunID: record.ID, Repository: record.Repository, CandidateID: candidate.ID,
			Branch: remote.Branch.Name, HeadSHA: remote.Branch.HeadSHA,
		}); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"remote issue branch cleanup failed; retry exact absence without rollback",
				"remote-cleanup",
			)
		}
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"remote issue branch absence was requested by exact identity",
			"remote-cleanup",
		)
	}

	local, err := m.localCompletion.ObserveLocalCompletion(ctx, localCompletionRequest(
		record, candidate, git.CommonDir, request.RepositoryPath, merge.MergeCommitSHA,
	))
	if err != nil {
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"local completion observation failed; retry without mutation",
			"local-cleanup",
		)
	}
	if err := validatePostMergeLocalObservation(record, candidate, request.RepositoryPath, local); err != nil {
		return m.persistAssuranceTransition(store, record, StateBlocked, err.Error(), "local-cleanup")
	}
	if len(local.Worktrees) > 0 {
		worktrees := append([]ManagedWorktreeObservation{}, local.Worktrees...)
		sort.Slice(worktrees, func(i, j int) bool { return worktrees[i].Path < worktrees[j].Path })
		target := worktrees[0]
		if err := m.localCompletion.EnsureManagedWorktreeAbsent(ctx, RemoveManagedWorktreeRequest{
			RunID: record.ID, CandidateID: candidate.ID,
			CommonDir: git.CommonDir, RepositoryPath: request.RepositoryPath,
			Path: target.Path, Branch: target.Branch, HeadSHA: target.HeadSHA,
		}); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"managed worktree cleanup failed; retry exact absence",
				"worktree-cleanup",
			)
		}
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"one exact workflow-owned worktree absence was requested",
			"worktree-cleanup",
		)
	}
	if local.LocalBranch != nil {
		if local.LocalBranch.Name != record.NonLocal.Authorization.Branch ||
			local.LocalBranch.HeadSHA != candidate.CommitSHA {
			return m.persistAssuranceTransition(
				store, record, StateBlocked,
				"local issue branch identity is incompatible; preserve it for operator inspection",
				"local-branch-cleanup",
			)
		}
		if err := m.localCompletion.EnsureLocalIssueBranchAbsent(ctx, DeleteLocalIssueBranchRequest{
			RunID: record.ID, CandidateID: candidate.ID,
			CommonDir: git.CommonDir, RepositoryPath: request.RepositoryPath,
			Branch: local.LocalBranch.Name, HeadSHA: local.LocalBranch.HeadSHA,
		}); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"local issue branch cleanup failed; retry exact absence",
				"local-branch-cleanup",
			)
		}
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"local issue branch absence was requested after worktree cleanup",
			"local-branch-cleanup",
		)
	}
	main := local.LocalMain
	if main.OriginHeadSHA != remote.OriginMain.HeadSHA {
		return m.persistAssuranceTransition(
			store, record, StateBlocked,
			"local main observation is not bound to the verified origin/main head",
			"main-synchronization",
		)
	}
	switch main.Relation {
	case LocalMainBehind:
		if err := m.localCompletion.EnsureLocalMainFastForward(ctx, FastForwardLocalMainRequest{
			RunID: record.ID, ExpectedOldSHA: main.HeadSHA,
			CommonDir: git.CommonDir, RepositoryPath: request.RepositoryPath,
			OriginMainSHA: remote.OriginMain.HeadSHA, MergeCommitSHA: merge.MergeCommitSHA,
		}); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"local main fast-forward failed; operator state was preserved",
				"main-synchronization",
			)
		}
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"local main fast-forward was requested by compare-and-swap",
			"main-synchronization",
		)
	case LocalMainSynced:
		if main.HeadSHA != remote.OriginMain.HeadSHA {
			return m.persistAssuranceTransition(
				store, record, StateBlocked,
				"local main reports synchronized with an incompatible origin/main identity",
				"main-synchronization",
			)
		}
	case LocalMainAhead, LocalMainDiverged:
		return m.persistAssuranceTransition(
			store, record, StateBlocked,
			"local main has operator-owned commits or divergence; preserve it without forced mutation",
			"main-synchronization",
		)
	default:
		return m.persistAssuranceTransition(
			store, record, StateBlocked,
			"local main synchronization relation is incomplete",
			"main-synchronization",
		)
	}
	record.NonLocal.Completion = &CompletionReport{
		IssueClosed: true, OriginMain: *remote.OriginMain,
		RemoteBranchAbsent: true, WorktreesAbsent: true, LocalBranchAbsent: true,
		LocalMain: main, Integration: local.Integration,
		OperatorStateSHA256: local.OperatorStateSHA256,
		CompletedAt:         m.clock.Now().UTC().Format(timeFormat),
	}
	return m.persistAssuranceTransition(
		store, record, StateCompleted,
		"merge, integration, issue closure, branch cleanup, worktree cleanup, and local main state are verified",
		"completion",
	)
}

func nonLocalObserveRequest(record runRecord, candidate Candidate) NonLocalObserveRequest {
	return NonLocalObserveRequest{
		RunID: record.ID, Repository: record.Repository, Issue: record.Issue,
		CandidateID: candidate.ID, Branch: record.NonLocal.Authorization.Branch,
		BaseRef: nonLocalBaseRef, HeadSHA: candidate.CommitSHA,
	}
}

func localCompletionRequest(
	record runRecord,
	candidate Candidate,
	commonDir, repositoryPath, mergeCommit string,
) LocalCompletionObserveRequest {
	return LocalCompletionObserveRequest{
		RunID: record.ID, CandidateID: candidate.ID,
		CommonDir: commonDir, RepositoryPath: repositoryPath,
		IssueBranch:   record.NonLocal.Authorization.Branch,
		CandidateHead: candidate.CommitSHA, MergeCommitSHA: mergeCommit,
	}
}

func mergeIdempotencyKey(record runRecord, candidate Candidate) string {
	pr := record.NonLocal.PullRequest
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%s\x00%d\x00%s\x00%s\x00%s",
		record.ID, candidate.ID, pr.Number, candidate.CommitSHA, pr.BaseSHA, mergeMethod,
	)))
	return hex.EncodeToString(sum[:])
}

func newMergeIntent(
	record runRecord,
	candidate Candidate,
	operatorState string,
	preparedAt time.Time,
) *MergeIntent {
	return &MergeIntent{
		IdempotencyKey:      mergeIdempotencyKey(record, candidate),
		PullRequest:         record.NonLocal.PullRequest.Number,
		HeadSHA:             candidate.CommitSHA,
		BaseSHA:             record.NonLocal.PullRequest.BaseSHA,
		Method:              mergeMethod,
		OperatorStateSHA256: operatorState,
		PreparedAt:          preparedAt.UTC().Format(timeFormat),
	}
}

func validatePreMergeLocalObservation(
	observation LocalCompletionObservation,
	commonDir string,
	repositoryPath string,
) error {
	if !runIDPattern.MatchString(observation.OperatorStateSHA256) ||
		observation.Worktrees == nil ||
		!safeCompletionPath(commonDir) ||
		!safeCompletionPath(repositoryPath) ||
		observation.Integration.Path != repositoryPath ||
		!observation.Integration.Clean {
		return errors.New("pre-merge local/operator observation is incomplete or incompatible")
	}
	return nil
}

func validatePostMergeLocalObservation(
	record runRecord,
	candidate Candidate,
	repositoryPath string,
	observation LocalCompletionObservation,
) error {
	intent := record.NonLocal.MergeIntent
	if intent == nil || observation.OperatorStateSHA256 != intent.OperatorStateSHA256 {
		return errors.New("operator state changed after merge; preserve it without cleanup mutation")
	}
	if observation.Worktrees == nil || observation.Integration.Path != repositoryPath ||
		!observation.Integration.Clean {
		return errors.New("integration workspace is incomplete or dirty")
	}
	if observation.Integration.Branch == record.NonLocal.Authorization.Branch &&
		(observation.LocalBranch == nil ||
			observation.LocalBranch.Name != record.NonLocal.Authorization.Branch ||
			observation.LocalBranch.HeadSHA != candidate.CommitSHA) {
		return errors.New("integration workspace issue branch is not the exact cleanup candidate")
	}
	if !observation.LocalMain.Exists || !observation.LocalMain.Clean {
		return errors.New("local main is missing or dirty; preserve operator state without forced mutation")
	}
	seenPaths := map[string]bool{}
	for _, worktree := range observation.Worktrees {
		if !safeCompletionPath(worktree.Path) || seenPaths[worktree.Path] ||
			worktree.Path == observation.Integration.Path ||
			worktree.RunID != record.ID || worktree.CandidateID != candidate.ID ||
			worktree.Branch != record.NonLocal.Authorization.Branch ||
			worktree.HeadSHA != candidate.CommitSHA || !worktree.Clean {
			return errors.New("temporary worktree is not an exact clean workflow-owned delivery worktree")
		}
		seenPaths[worktree.Path] = true
	}
	return nil
}

func safeCompletionPath(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value &&
		value != string(filepath.Separator)
}

func validateOriginMain(
	observation OriginMainObservation,
	merge MergeProof,
	candidateHead string,
) error {
	if !fullGitSHAPattern.MatchString(observation.HeadSHA) ||
		observation.MergeCommitSHA != merge.MergeCommitSHA ||
		observation.CandidateHeadSHA != candidateHead ||
		!observation.ContainsMergeCommit || !observation.ContainsCandidateHead {
		return errors.New("origin/main does not contain the exact merge and candidate commits")
	}
	return nil
}

func validateMergeObservation(record runRecord, candidate Candidate, merge MergeObservation) error {
	pr := record.NonLocal.PullRequest
	if pr == nil || merge.PullRequest != pr.Number || merge.URL != pr.URL ||
		merge.BaseRef != nonLocalBaseRef || merge.HeadSHA != candidate.CommitSHA ||
		!fullGitSHAPattern.MatchString(merge.MergeCommitSHA) {
		return errors.New("completed merge identity is incompatible; preserve it and attempt no rollback")
	}
	if _, err := time.Parse(timeFormat, merge.MergedAt); err != nil {
		return errors.New("completed merge time is invalid")
	}
	return nil
}

func validateStoredMergeGate(record runRecord, candidate Candidate) error {
	if record.NonLocal.CIStatus != string(CISuccess) || record.NonLocal.PullRequest == nil {
		return errors.New("completed merge lacks previously proved exact-head required CI")
	}
	result := evaluateCIPolicy(
		record.Repository, candidate.CommitSHA, record.NonLocal.PullRequest.BaseSHA,
		record.NonLocal.Checks,
	)
	if result.State != CISuccess {
		return errors.New("completed merge does not match the exact proved required-CI gate")
	}
	return nil
}

func mergeProofFromObservation(observation MergeObservation, adoptedAt time.Time) *MergeProof {
	return &MergeProof{
		PullRequest: observation.PullRequest, URL: observation.URL,
		HeadSHA: observation.HeadSHA, MergeCommitSHA: observation.MergeCommitSHA,
		MergedAt: observation.MergedAt, AdoptedAt: adoptedAt.UTC().Format(timeFormat),
	}
}

func sameMergeProof(proof MergeProof, observation MergeObservation) bool {
	return proof.PullRequest == observation.PullRequest && proof.URL == observation.URL &&
		proof.HeadSHA == observation.HeadSHA &&
		proof.MergeCommitSHA == observation.MergeCommitSHA &&
		proof.MergedAt == observation.MergedAt
}
