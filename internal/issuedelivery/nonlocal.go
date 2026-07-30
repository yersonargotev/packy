package issuedelivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

const nonLocalBaseRef = "main"

func (m *Module) advanceNonLocal(
	ctx context.Context,
	store lockedIssueStore,
	record runRecord,
	git GitObservation,
	tracker TrackerObservation,
	compiled compiledAuthority,
	request Request,
) (Outcome, error) {
	if record.Schema == legacyRunSchema {
		return outcomeFromRecord(record), nil
	}
	candidate := latestCandidate(&record)
	if err := validateNonLocalFreshness(record, candidate, git, tracker, compiled); err != nil {
		return m.persistAssuranceTransition(
			store, record, StateBlocked, err.Error(), "non-local-freshness",
		)
	}
	if record.NonLocal == nil {
		if request.NonLocal == nil {
			return outcomeWithReason(
				record, StateWaiting,
				"exact local readiness is proved; explicit non-local authorization is required",
			), nil
		}
		if err := validateNonLocalAuthorization(*request.NonLocal, record, *candidate); err != nil {
			return Outcome{}, err
		}
		record.NonLocal = &NonLocalDelivery{
			Authorization: *request.NonLocal, BaseRef: nonLocalBaseRef,
			Checks: []CICheckObservation{}, Retries: []CIRetry{},
		}
		return m.persistAssuranceTransition(
			store, record, StateWaiting, "exact non-local delivery authority is recorded",
			"non-local-authorization",
		)
	}
	if request.NonLocal != nil && *request.NonLocal != record.NonLocal.Authorization {
		return Outcome{}, errors.New("non-local authorization does not match the active delivery candidate")
	}

	observeRequest := NonLocalObserveRequest{
		RunID: record.ID, Repository: record.Repository, Issue: record.Issue,
		CandidateID: candidate.ID, Branch: record.LocalReadiness.Branch,
		BaseRef: nonLocalBaseRef, HeadSHA: candidate.CommitSHA,
	}
	observation, err := m.nonlocal.ObserveNonLocal(ctx, observeRequest)
	if err != nil {
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"non-local observation failed; retry without changing the candidate",
			"non-local-observation",
		)
	}
	if observation.PullRequests == nil || observation.Checks == nil {
		return m.persistAssuranceTransition(
			store, record, StateBlocked,
			"non-local observation is incomplete; reacquire branch, pull request, and CI state",
			"non-local-observation",
		)
	}

	if observation.Branch == nil {
		if err := m.nonlocal.PushIssueBranch(ctx, PushIssueBranchRequest{
			RunID: record.ID, Repository: record.Repository, CandidateID: candidate.ID,
			Branch: record.LocalReadiness.Branch, HeadSHA: candidate.CommitSHA,
		}); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"issue branch push failed; retry without changing the candidate",
				"branch-push",
			)
		}
		observation, err = m.nonlocal.ObserveNonLocal(ctx, observeRequest)
		if err != nil || observation.Branch == nil {
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"issue branch push is not yet observable; retry observation",
				"branch-push",
			)
		}
	} else if observation.Branch.Name != record.LocalReadiness.Branch {
		return m.persistAssuranceTransition(
			store, record, StateBlocked,
			"remote issue branch name is incompatible; inspect it and requalify without mutation",
			"branch-push",
		)
	} else if observation.Branch.HeadSHA != candidate.CommitSHA {
		if !priorCandidateCommit(record, observation.Branch.HeadSHA) {
			return m.persistAssuranceTransition(
				store, record, StateBlocked,
				"remote issue branch has an incompatible HEAD; inspect or rename it, then requalify",
				"branch-push",
			)
		}
		if err := m.nonlocal.PushIssueBranch(ctx, PushIssueBranchRequest{
			RunID: record.ID, Repository: record.Repository, CandidateID: candidate.ID,
			Branch: record.LocalReadiness.Branch, HeadSHA: candidate.CommitSHA,
		}); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"fast-forward issue branch push failed; retry without changing the candidate",
				"branch-push",
			)
		}
		observation, err = m.nonlocal.ObserveNonLocal(ctx, observeRequest)
		if err != nil || observation.Branch == nil {
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"updated issue branch is not yet observable; retry observation",
				"branch-push",
			)
		}
	}
	if err := validateRemoteBranch(*observation.Branch, record.LocalReadiness.Branch, candidate.CommitSHA); err != nil {
		return m.persistAssuranceTransition(store, record, StateBlocked, err.Error(), "branch-push")
	}
	if record.NonLocal.Branch == nil || *record.NonLocal.Branch != *observation.Branch {
		branch := *observation.Branch
		record.NonLocal.Branch = &branch
		return m.persistAssuranceTransition(
			store, record, StateWaiting, "exact remote issue branch is proved", "branch-push",
		)
	}

	pullRequest, err := selectExactPullRequest(
		observation.PullRequests, record.Repository, record.Issue.Number,
		record.LocalReadiness.Branch, candidate.CommitSHA,
	)
	if err != nil {
		return m.persistAssuranceTransition(store, record, StateBlocked, err.Error(), "pull-request")
	}
	if pullRequest == nil {
		if record.NonLocal.PullRequestIntent == nil {
			record.NonLocal.PullRequestIntent = &PullRequestIntent{
				IdempotencyKey: pullRequestIdempotencyKey(record, *candidate),
				PreparedAt:     m.clock.Now().UTC().Format(timeFormat),
			}
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"exact pull-request creation intent is recorded before mutation",
				"pull-request",
			)
		}
		intent := record.NonLocal.PullRequestIntent
		if intent.DispatchedAt != "" {
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"pull-request creation was dispatched; wait to adopt its exact observable identity",
				"pull-request",
			)
		}
		if err := m.nonlocal.EnsurePullRequest(ctx, EnsurePullRequestRequest{
			RunID: record.ID, Repository: record.Repository, Issue: record.Issue,
			CandidateID: candidate.ID, IdempotencyKey: intent.IdempotencyKey,
			BaseRef:    nonLocalBaseRef,
			HeadBranch: record.LocalReadiness.Branch, HeadSHA: candidate.CommitSHA,
			Title: tracker.Title, Body: fmt.Sprintf("Closes #%d", record.Issue.Number),
		}); err != nil {
			return m.persistAssuranceTransition(
				store, record, StateWaiting,
				"pull-request creation failed; retry without changing the candidate",
				"pull-request",
			)
		}
		intent.DispatchedAt = m.clock.Now().UTC().Format(timeFormat)
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"idempotent pull-request creation was dispatched; wait for exact observation",
			"pull-request",
		)
	}
	if record.NonLocal.PullRequest == nil || *record.NonLocal.PullRequest != *pullRequest {
		value := *pullRequest
		record.NonLocal.PullRequest = &value
		return m.persistAssuranceTransition(
			store, record, StateWaiting, "one exact-head pull request is proved", "pull-request",
		)
	}

	confirmation, err := m.nonlocal.ObserveNonLocal(ctx, observeRequest)
	if err != nil || confirmation.PullRequests == nil || confirmation.Checks == nil {
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"pull-request head/base confirmation is not yet available after CI collection",
			"ci-wait",
		)
	}
	confirmedPullRequest, err := selectExactPullRequest(
		confirmation.PullRequests, record.Repository, record.Issue.Number,
		record.LocalReadiness.Branch, candidate.CommitSHA,
	)
	if err != nil {
		return m.persistAssuranceTransition(store, record, StateBlocked, err.Error(), "ci-wait")
	}
	if confirmedPullRequest == nil || *confirmedPullRequest != *record.NonLocal.PullRequest {
		return m.persistAssuranceTransition(
			store, record, StateBlocked,
			"pull-request head or base changed during CI collection; reobserve and requalify",
			"ci-wait",
		)
	}
	return m.advanceRequiredCI(
		ctx, store, record, candidate, git, request.RepositoryPath,
		confirmedPullRequest.BaseSHA, confirmation.Checks,
	)
}

func validateNonLocalFreshness(
	record runRecord,
	candidate *Candidate,
	git GitObservation,
	tracker TrackerObservation,
	compiled compiledAuthority,
) error {
	if candidate == nil || record.LocalReadiness == nil ||
		record.LocalReadiness.CandidateID != candidate.ID ||
		record.LocalReadiness.CommitSHA != candidate.CommitSHA ||
		record.LocalReadiness.TreeSHA != candidate.TreeSHA ||
		record.LocalReadiness.AuthorityHash != compiled.hash ||
		record.AuthoritySHA256 != compiled.hash ||
		git.HeadSHA != candidate.CommitSHA || git.TreeSHA != candidate.TreeSHA ||
		git.Branch != record.LocalReadiness.Branch || !git.WorkspaceClean ||
		!deliveryBranch(git.Branch, tracker.Issue.Number) {
		return errors.New("non-local delivery freshness changed; restore exact local readiness or requalify")
	}
	if record.PendingDecision != nil || record.PendingRepair != nil ||
		candidate.Exhaustive == nil || len(candidate.Acceptance) == 0 ||
		len(missingReviewAxes(candidate)) != 0 ||
		len(missingSpecialistBoundaries(candidate)) != 0 ||
		len(missingBoundaryProofs(candidate)) != 0 {
		return errors.New("non-local delivery requires complete candidate assurance and exact validation")
	}
	return nil
}

func validateNonLocalAuthorization(
	authorization NonLocalAuthorization,
	record runRecord,
	candidate Candidate,
) error {
	ready := record.LocalReadiness
	if ready == nil || authorization.RunID != record.ID ||
		authorization.CandidateID != candidate.ID ||
		authorization.CommitSHA != candidate.CommitSHA ||
		authorization.TreeSHA != candidate.TreeSHA ||
		authorization.Branch != ready.Branch ||
		authorization.LocalReadyAt != ready.ReadyAt {
		return errors.New("non-local authorization does not match exact local readiness")
	}
	return nil
}

func validateRemoteBranch(branch RemoteBranchObservation, expectedName, expectedHead string) error {
	if branch.Name != expectedName || branch.HeadSHA != expectedHead {
		return errors.New("remote issue branch identity is incompatible; inspect it and requalify without force-pushing")
	}
	return nil
}

func priorCandidateCommit(record runRecord, commit string) bool {
	for _, candidate := range record.Candidates[:len(record.Candidates)-1] {
		if candidate.CommitSHA == commit {
			return true
		}
	}
	return false
}

func selectExactPullRequest(
	observed []RemotePullRequestObservation,
	repository deliveryevidence.RepositoryIdentity,
	issue int,
	branch, head string,
) (*RemotePullRequestObservation, error) {
	if len(observed) == 0 {
		return nil, nil
	}
	if len(observed) != 1 {
		return nil, errors.New("multiple pull requests match the issue branch; preserve them for inspection and requalify")
	}
	item := observed[0]
	parsed, err := url.Parse(item.URL)
	expectedPath := fmt.Sprintf(
		"/%s/%s/pull/%d", repository.Owner, repository.Name, item.Number,
	)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "github.com" ||
		parsed.Port() != "" || parsed.User != nil || parsed.Path != expectedPath ||
		parsed.RawQuery != "" || parsed.Fragment != "" ||
		item.Number <= 0 || item.State != "OPEN" || item.BaseRef != nonLocalBaseRef ||
		!validCIHead(item.BaseSHA) ||
		item.HeadBranch != branch || item.HeadSHA != head ||
		item.ClosingIssue != issue || item.HeadRepositoryNodeID != repository.NodeID {
		return nil, errors.New("pull-request identity is incompatible; inspect it and create no duplicate")
	}
	return &item, nil
}

func (m *Module) advanceRequiredCI(
	ctx context.Context,
	store lockedIssueStore,
	record runRecord,
	candidate *Candidate,
	git GitObservation,
	repositoryPath string,
	baseSHA string,
	observed []CICheckObservation,
) (Outcome, error) {
	result := evaluateCIPolicy(record.Repository, candidate.CommitSHA, baseSHA, observed)
	record.NonLocal.Checks = canonicalCICheckObservations(observed)
	record.NonLocal.CIStatus = string(result.State)
	if result.State != CISuccess {
		record.NonLocal.MergeIntent = nil
	}
	switch result.State {
	case CIPending:
		return m.persistAssuranceTransition(
			store, record, StateWaiting, "required CI is pending for the exact candidate HEAD", "ci-wait",
		)
	case CISuccess:
		return m.advanceMerge(ctx, store, record, *candidate, git.CommonDir, repositoryPath)
	case CIInfrastructureFailure:
		failure := firstFailedAttributedCheck(observed, candidate.CommitSHA, FailureInfrastructure)
		if failure == nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "infrastructure failure evidence is incomplete", "ci-wait",
			)
		}
		if !alreadyRetried(record.NonLocal.Retries, *failure) {
			if err := m.nonlocal.RetryInfrastructureCheck(ctx, RetryInfrastructureCheckRequest{
				RunID: record.ID, Repository: record.Repository,
				PullRequest: record.NonLocal.PullRequest.Number,
				CandidateID: candidate.ID, HeadSHA: candidate.CommitSHA,
				CheckIdentity: failure.Identity, FailedRunID: failure.RunID,
			}); err != nil {
				return m.persistAssuranceTransition(
					store, record, StateWaiting,
					"infrastructure CI retry failed; retry without changing the candidate",
					"ci-retry",
				)
			}
			record.NonLocal.Retries = append(record.NonLocal.Retries, CIRetry{
				CheckIdentity: failure.Identity, FailedRunID: failure.RunID,
				RetriedAt: m.clock.Now().UTC().Format(timeFormat),
			})
		}
		return m.persistAssuranceTransition(
			store, record, StateWaiting,
			"infrastructure CI failure is retrying without changing the candidate",
			"ci-retry",
		)
	case CICandidateFailure:
		failure := firstFailedAttributedCheck(observed, candidate.CommitSHA, FailureCandidate)
		if failure == nil {
			return m.persistAssuranceTransition(
				store, record, StateBlocked, "candidate CI failure evidence is incomplete", "ci-wait",
			)
		}
		record.NonLocal.CandidateFailure = &CandidateCIFailure{
			CheckIdentity: failure.Identity, RunID: failure.RunID,
			DetailsURL: failure.DetailsURL,
			ObservedAt: m.clock.Now().UTC().Format(timeFormat),
		}
		invalidateForCandidateCIFailure(&record, candidate)
		return m.persistAssuranceTransition(
			store, record, StateNeedsReview,
			"change-attributable CI failure requires a candidate-changing repair",
			"ci-candidate-failure",
		)
	case CIDecisionRequired:
		return m.persistAssuranceTransition(
			store, record, StateBlocked,
			"CI failure attribution is unknown; classify the exact failed run before retrying or repairing",
			"ci-wait",
		)
	default:
		reason := "required CI observation is incompatible"
		if len(result.Reasons) > 0 {
			reason += ": " + strings.Join(result.Reasons, "; ")
		}
		return m.persistAssuranceTransition(store, record, StateBlocked, reason, "ci-wait")
	}
}

func canonicalCICheckObservations(values []CICheckObservation) []CICheckObservation {
	out := append([]CICheckObservation{}, values...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Identity != out[j].Identity {
			return out[i].Identity < out[j].Identity
		}
		return out[i].RunID < out[j].RunID
	})
	return out
}

func pullRequestIdempotencyKey(record runRecord, candidate Candidate) string {
	sum := sha256.Sum256([]byte(
		record.ID + "\x00" + candidate.ID + "\x00" + nonLocalBaseRef + "\x00" +
			record.NonLocal.Authorization.Branch + "\x00" + candidate.CommitSHA,
	))
	return hex.EncodeToString(sum[:])
}

func firstFailedAttributedCheck(
	values []CICheckObservation,
	head string,
	attribution CIFailureAttribution,
) *CICheckObservation {
	for _, value := range values {
		conclusion := strings.ToLower(strings.TrimSpace(value.Conclusion))
		if value.HeadSHA == head && value.FailureAttribution == attribution &&
			(conclusion == "failure" || conclusion == "failed" || conclusion == "cancelled") {
			copy := value
			return &copy
		}
	}
	return nil
}

func alreadyRetried(retries []CIRetry, failure CICheckObservation) bool {
	for _, retry := range retries {
		if retry.CheckIdentity == failure.Identity && retry.FailedRunID == failure.RunID {
			return true
		}
	}
	return false
}

func invalidateForCandidateCIFailure(record *runRecord, candidate *Candidate) {
	record.LocalReadiness = nil
	candidate.Acceptance = nil
	candidate.Exhaustive = nil
	record.Evidence.ValidationReceipts = []deliveryevidence.ValidationReceipt{}
	invalidateAcceptance(record.Evidence, record.QualificationCorrections)
}

func validateNonLocalRecord(record runRecord, candidate Candidate) error {
	remote := record.NonLocal
	if remote == nil {
		if record.State == StateCompleted {
			return errors.New("completed delivery lacks non-local completion proof")
		}
		return nil
	}
	authorization := remote.Authorization
	if authorization.RunID != record.ID ||
		authorization.CandidateID != candidate.ID ||
		authorization.CommitSHA != candidate.CommitSHA ||
		authorization.TreeSHA != candidate.TreeSHA ||
		!deliveryBranch(authorization.Branch, record.Issue.Number) ||
		strings.TrimSpace(authorization.LocalReadyAt) == "" ||
		remote.BaseRef != nonLocalBaseRef ||
		remote.Checks == nil || remote.Retries == nil {
		return errors.New("non-local delivery authority is invalid")
	}
	if _, err := time.Parse(timeFormat, authorization.LocalReadyAt); err != nil {
		return errors.New("non-local delivery authority readiness time is invalid")
	}
	if record.LocalReadiness != nil &&
		(record.LocalReadiness.CandidateID != authorization.CandidateID ||
			record.LocalReadiness.CommitSHA != authorization.CommitSHA ||
			record.LocalReadiness.TreeSHA != authorization.TreeSHA ||
			record.LocalReadiness.Branch != authorization.Branch ||
			record.LocalReadiness.ReadyAt != authorization.LocalReadyAt) {
		return errors.New("non-local delivery authority does not match local readiness")
	}
	if record.LocalReadiness == nil && remote.CandidateFailure == nil {
		return errors.New("non-local delivery authority lacks exact local readiness")
	}
	if remote.Branch != nil {
		if err := validateRemoteBranch(
			*remote.Branch, authorization.Branch, authorization.CommitSHA,
		); err != nil {
			return err
		}
	}
	if remote.PullRequest != nil {
		if remote.Branch == nil {
			return errors.New("non-local pull request lacks an exact remote branch")
		}
		selected, err := selectExactPullRequest(
			[]RemotePullRequestObservation{*remote.PullRequest},
			record.Repository, record.Issue.Number,
			authorization.Branch, authorization.CommitSHA,
		)
		if err != nil || selected == nil {
			return errors.New("non-local pull-request identity is invalid")
		}
	}
	if remote.PullRequestIntent != nil {
		intent := remote.PullRequestIntent
		if intent.IdempotencyKey != pullRequestIdempotencyKey(record, candidate) ||
			!runIDPattern.MatchString(intent.IdempotencyKey) {
			return errors.New("non-local pull-request intent identity is invalid")
		}
		if _, err := time.Parse(timeFormat, intent.PreparedAt); err != nil {
			return errors.New("non-local pull-request intent preparation time is invalid")
		}
		if intent.DispatchedAt != "" {
			if _, err := time.Parse(timeFormat, intent.DispatchedAt); err != nil {
				return errors.New("non-local pull-request dispatch time is invalid")
			}
		}
	}
	if len(remote.Checks) > 0 && remote.PullRequest == nil {
		return errors.New("non-local CI evidence lacks a pull request")
	}
	if !equalCICheckObservations(remote.Checks, canonicalCICheckObservations(remote.Checks)) {
		return errors.New("non-local CI observations are not canonical")
	}
	retryIDs := map[string]bool{}
	for _, retry := range remote.Retries {
		key := fmt.Sprintf("%s\x00%d", retry.CheckIdentity, retry.FailedRunID)
		if retryIDs[key] || retry.FailedRunID <= 0 || strings.TrimSpace(retry.CheckIdentity) == "" ||
			!containsInfrastructureFailure(
				remote.Checks, authorization.CommitSHA, retry.CheckIdentity, retry.FailedRunID,
			) {
			return errors.New("non-local CI retry journal is invalid")
		}
		if _, err := time.Parse(timeFormat, retry.RetriedAt); err != nil {
			return errors.New("non-local CI retry time is invalid")
		}
		retryIDs[key] = true
	}
	switch CIPolicyState(remote.CIStatus) {
	case "", CIPending, CISuccess, CIInfrastructureFailure,
		CICandidateFailure, CIDecisionRequired, CIBlocked:
	default:
		return errors.New("non-local CI status is invalid")
	}
	if remote.CIStatus != "" {
		if remote.PullRequest == nil {
			return errors.New("non-local CI status lacks an exact pull request")
		}
		result := evaluateCIPolicy(
			record.Repository, candidate.CommitSHA, remote.PullRequest.BaseSHA, remote.Checks,
		)
		if result.State != CIPolicyState(remote.CIStatus) {
			return errors.New("non-local CI status does not match its exact observations")
		}
	}
	if remote.MergeIntent != nil {
		intent := remote.MergeIntent
		if remote.PullRequest == nil ||
			intent.IdempotencyKey != mergeIdempotencyKey(record, candidate) ||
			!runIDPattern.MatchString(intent.IdempotencyKey) ||
			intent.PullRequest != remote.PullRequest.Number ||
			intent.HeadSHA != candidate.CommitSHA ||
			intent.BaseSHA != remote.PullRequest.BaseSHA ||
			intent.Method != mergeMethod ||
			!runIDPattern.MatchString(intent.OperatorStateSHA256) {
			return errors.New("non-local merge intent identity is invalid")
		}
		if _, err := time.Parse(timeFormat, intent.PreparedAt); err != nil {
			return errors.New("non-local merge intent preparation time is invalid")
		}
		if intent.DispatchedAt != "" {
			if _, err := time.Parse(timeFormat, intent.DispatchedAt); err != nil {
				return errors.New("non-local merge dispatch time is invalid")
			}
		}
		if err := validateStoredMergeGate(record, candidate); err != nil {
			return err
		}
	}
	if remote.Merge != nil {
		if remote.CandidateFailure != nil ||
			(record.State != StateWaiting && record.State != StateBlocked &&
				record.State != StateCompleted) {
			return errors.New("irreversible merge proof is incompatible with candidate flow")
		}
		merge := remote.Merge
		if err := validateMergeObservation(record, candidate, MergeObservation{
			PullRequest: merge.PullRequest, URL: merge.URL, BaseRef: nonLocalBaseRef,
			HeadSHA: merge.HeadSHA, MergeCommitSHA: merge.MergeCommitSHA, MergedAt: merge.MergedAt,
		}); err != nil {
			return err
		}
		mergedAt, mergedErr := time.Parse(timeFormat, merge.MergedAt)
		adoptedAt, adoptedErr := time.Parse(timeFormat, merge.AdoptedAt)
		if mergedErr != nil || adoptedErr != nil || adoptedAt.Before(mergedAt) {
			return errors.New("irreversible merge adoption time is invalid")
		}
	}
	if remote.CandidateFailure != nil && remote.MergeIntent != nil {
		return errors.New("candidate failure retained stale merge authority")
	}
	if (record.State == StateCompleted) != (remote.Completion != nil) {
		return errors.New("completed state and completion report must coexist")
	}
	if remote.Completion != nil {
		completion := remote.Completion
		if remote.Merge == nil || remote.MergeIntent == nil || !completion.IssueClosed ||
			!completion.RemoteBranchAbsent || !completion.WorktreesAbsent ||
			!completion.LocalBranchAbsent ||
			completion.OperatorStateSHA256 != remote.MergeIntent.OperatorStateSHA256 ||
			!completion.Integration.Clean ||
			completion.Integration.Branch == authorization.Branch ||
			!completion.LocalMain.Exists || !completion.LocalMain.Clean ||
			completion.LocalMain.Relation != LocalMainSynced ||
			completion.LocalMain.HeadSHA != completion.OriginMain.HeadSHA ||
			completion.LocalMain.OriginHeadSHA != completion.OriginMain.HeadSHA ||
			!runIDPattern.MatchString(completion.OperatorStateSHA256) ||
			!safeCompletionPath(completion.Integration.Path) {
			return errors.New("non-local completion report is invalid")
		}
		if err := validateOriginMain(
			completion.OriginMain, *remote.Merge, candidate.CommitSHA,
		); err != nil {
			return err
		}
		if _, err := time.Parse(timeFormat, completion.CompletedAt); err != nil {
			return errors.New("non-local completion time is invalid")
		}
	}
	if remote.CandidateFailure != nil {
		failure := remote.CandidateFailure
		if record.LocalReadiness != nil || candidate.Exhaustive != nil || len(candidate.Acceptance) != 0 ||
			!containsCandidateFailure(remote.Checks, *failure) {
			return errors.New("candidate-attributable CI failure did not invalidate stale readiness")
		}
		if _, err := time.Parse(timeFormat, failure.ObservedAt); err != nil {
			return errors.New("candidate-attributable CI failure time is invalid")
		}
	}
	return nil
}

func containsInfrastructureFailure(
	checks []CICheckObservation,
	head, identity string,
	runID int64,
) bool {
	for _, check := range checks {
		conclusion := strings.ToLower(strings.TrimSpace(check.Conclusion))
		if check.Identity == identity && check.RunID == runID && check.HeadSHA == head &&
			check.FailureAttribution == FailureInfrastructure &&
			(conclusion == "failure" || conclusion == "failed" || conclusion == "cancelled") {
			return true
		}
	}
	return false
}

func equalCICheckObservations(left, right []CICheckObservation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func containsCandidateFailure(checks []CICheckObservation, failure CandidateCIFailure) bool {
	for _, check := range checks {
		if check.Identity == failure.CheckIdentity && check.RunID == failure.RunID &&
			check.DetailsURL == failure.DetailsURL &&
			check.FailureAttribution == FailureCandidate {
			return true
		}
	}
	return false
}
