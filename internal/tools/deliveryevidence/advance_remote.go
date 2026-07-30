package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
	"github.com/yersonargotev/packy/internal/issuedelivery"
)

type productionNonLocalGateway struct {
	runner       Runner
	repository   string
	attributions []advanceCIFailureAttribution
}

type remoteRepository struct {
	ID            string `json:"id"`
	NameWithOwner string `json:"nameWithOwner"`
}

type remotePullRequest struct {
	Number         int    `json:"number"`
	URL            string `json:"url"`
	State          string `json:"state"`
	BaseRefName    string `json:"baseRefName"`
	BaseRefOID     string `json:"baseRefOid"`
	HeadRefName    string `json:"headRefName"`
	HeadRefOID     string `json:"headRefOid"`
	HeadRepository struct {
		ID string `json:"id"`
	} `json:"headRepository"`
	ClosingIssuesReferences []struct {
		Number int    `json:"number"`
		ID     string `json:"id"`
	} `json:"closingIssuesReferences"`
	MergedAt    string `json:"mergedAt"`
	MergeCommit *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
}

type checkRunsResponse struct {
	CheckRuns []struct {
		Name       string `json:"name"`
		HeadSHA    string `json:"head_sha"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		DetailsURL string `json:"details_url"`
		App        struct {
			ID   int64  `json:"id"`
			Slug string `json:"slug"`
		} `json:"app"`
	} `json:"check_runs"`
}

type commitStatus struct {
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"target_url"`
	Creator   struct {
		Login   string `json:"login"`
		ID      int64  `json:"id"`
		Type    string `json:"type"`
		HTMLURL string `json:"html_url"`
	} `json:"creator"`
}

type workflowRun struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	HeadSHA string `json:"head_sha"`
	HTMLURL string `json:"html_url"`
	Actor   struct {
		Login   string `json:"login"`
		ID      int64  `json:"id"`
		Type    string `json:"type"`
		HTMLURL string `json:"html_url"`
	} `json:"actor"`
}

const (
	checkRunsProjection   = `{check_runs:[.check_runs[]|{name,head_sha,status,conclusion,details_url,app:{id:.app.id,slug:.app.slug}}]}`
	statusesProjection    = `[.[]|{context,state,target_url,creator:{login:.creator.login,id:.creator.id,type:.creator.type,html_url:.creator.html_url}}]`
	workflowRunProjection = `{id,name,path,head_sha,html_url,actor:{login:.actor.login,id:.actor.id,type:.actor.type,html_url:.actor.html_url}}`
)

func (gateway productionNonLocalGateway) ObserveNonLocal(ctx context.Context, request issuedelivery.NonLocalObserveRequest) (issuedelivery.NonLocalObservation, error) {
	if err := gateway.validateObserveRequest(ctx, request); err != nil {
		return issuedelivery.NonLocalObservation{}, err
	}
	repo := repositoryName(request.Repository)
	if _, err := gateway.output(
		ctx, "git", "fetch", "--prune", "--no-tags", "origin",
		"refs/heads/"+request.BaseRef+":refs/remotes/origin/"+request.BaseRef,
	); err != nil {
		return issuedelivery.NonLocalObservation{}, fmt.Errorf("refresh exact origin/%s: %w", request.BaseRef, err)
	}
	refsRaw, err := gateway.output(ctx, "git", "ls-remote", "--refs", "origin",
		"refs/heads/"+request.Branch, "refs/heads/"+request.BaseRef)
	if err != nil {
		return issuedelivery.NonLocalObservation{}, fmt.Errorf("observe remote refs: %w", err)
	}
	refs, err := parseRemoteRefs(refsRaw)
	if err != nil {
		return issuedelivery.NonLocalObservation{}, err
	}
	baseHead := refs["refs/heads/"+request.BaseRef]
	if baseHead == "" {
		return issuedelivery.NonLocalObservation{}, errors.New("remote base branch is absent")
	}
	trackingRaw, err := gateway.output(
		ctx, "git", "rev-parse", "refs/remotes/origin/"+request.BaseRef+"^{commit}",
	)
	if err != nil || strings.TrimSpace(string(trackingRaw)) != baseHead {
		return issuedelivery.NonLocalObservation{}, errors.New("refreshed origin base does not match remote observation")
	}
	observation := issuedelivery.NonLocalObservation{
		PullRequests: []issuedelivery.RemotePullRequestObservation{},
		Checks:       []issuedelivery.CICheckObservation{},
	}
	if head := refs["refs/heads/"+request.Branch]; head != "" {
		observation.Branch = &issuedelivery.RemoteBranchObservation{Name: request.Branch, HeadSHA: head}
	}

	prRaw, err := gateway.output(ctx, "gh", "pr", "list", "--repo", repo, "--state", "all",
		"--head", request.Branch, "--json",
		"number,url,state,baseRefName,baseRefOid,headRefName,headRefOid,headRepository,closingIssuesReferences,mergedAt,mergeCommit")
	if err != nil {
		return issuedelivery.NonLocalObservation{}, fmt.Errorf("observe pull requests: %w", err)
	}
	var prs []remotePullRequest
	if err := exactJSON(prRaw, &prs); err != nil {
		return issuedelivery.NonLocalObservation{}, fmt.Errorf("decode pull requests: %w", err)
	}
	for _, pr := range prs {
		converted, err := convertPullRequest(pr, request)
		if err != nil {
			return issuedelivery.NonLocalObservation{}, err
		}
		observation.PullRequests = append(observation.PullRequests, converted)
		if pr.MergedAt != "" && pr.MergeCommit != nil {
			if observation.Merge != nil {
				return issuedelivery.NonLocalObservation{}, errors.New("multiple merged pull requests match the delivery branch")
			}
			observation.Merge = &issuedelivery.MergeObservation{
				PullRequest: pr.Number, URL: pr.URL, BaseRef: pr.BaseRefName,
				HeadSHA: pr.HeadRefOID, MergeCommitSHA: pr.MergeCommit.OID, MergedAt: pr.MergedAt,
			}
		}
	}
	sort.Slice(observation.PullRequests, func(i, j int) bool {
		return observation.PullRequests[i].Number < observation.PullRequests[j].Number
	})
	if len(observation.PullRequests) > 0 {
		checks, err := gateway.observeChecks(ctx, request, refs["refs/heads/"+request.BaseRef])
		if err != nil {
			return issuedelivery.NonLocalObservation{}, err
		}
		observation.Checks = checks
	}
	if observation.Merge != nil {
		mainHead := refs["refs/heads/"+request.BaseRef]
		mergeContained, err := gateway.containsOriginMain(ctx, observation.Merge.MergeCommitSHA)
		if err != nil {
			return issuedelivery.NonLocalObservation{}, err
		}
		headContained, err := gateway.containsOriginMain(ctx, request.HeadSHA)
		if err != nil {
			return issuedelivery.NonLocalObservation{}, err
		}
		observation.OriginMain = &issuedelivery.OriginMainObservation{
			HeadSHA: mainHead, MergeCommitSHA: observation.Merge.MergeCommitSHA,
			CandidateHeadSHA: request.HeadSHA, ContainsMergeCommit: mergeContained,
			ContainsCandidateHead: headContained,
		}
	}
	return observation, nil
}

func (gateway productionNonLocalGateway) PushIssueBranch(ctx context.Context, request issuedelivery.PushIssueBranchRequest) error {
	if err := gateway.validateRepository(ctx, request.Repository); err != nil {
		return err
	}
	if err := validBranchAndHead(request.Branch, request.HeadSHA); err != nil {
		return err
	}
	raw, err := gateway.output(ctx, "git", "ls-remote", "--refs", "origin", "refs/heads/"+request.Branch)
	if err != nil {
		return fmt.Errorf("observe issue branch before push: %w", err)
	}
	refs, err := parseRemoteRefs(raw)
	if err != nil {
		return err
	}
	if current := refs["refs/heads/"+request.Branch]; current != "" && current != request.HeadSHA {
		_, err = gateway.output(ctx, "git", "merge-base", "--is-ancestor", current, request.HeadSHA)
		if err != nil {
			return errors.New("remote issue branch is not an ancestor of the authorized candidate")
		}
	}
	_, err = gateway.output(ctx, "git", "push", "origin",
		request.HeadSHA+":refs/heads/"+request.Branch)
	if err != nil {
		return fmt.Errorf("push exact issue branch: %w", err)
	}
	return nil
}

func (gateway productionNonLocalGateway) EnsurePullRequest(ctx context.Context, request issuedelivery.EnsurePullRequestRequest) error {
	observe := issuedelivery.NonLocalObserveRequest{
		RunID: request.RunID, Repository: request.Repository, Issue: request.Issue,
		CandidateID: request.CandidateID, Branch: request.HeadBranch,
		BaseRef: request.BaseRef, HeadSHA: request.HeadSHA,
	}
	state, err := gateway.ObserveNonLocal(ctx, observe)
	if err != nil {
		return err
	}
	for _, pr := range state.PullRequests {
		if pr.HeadBranch == request.HeadBranch && pr.HeadSHA == request.HeadSHA &&
			pr.BaseRef == request.BaseRef && pr.ClosingIssue == request.Issue.Number {
			return nil
		}
		return errors.New("an incompatible pull request already owns the delivery branch")
	}
	_, err = gateway.output(ctx, "gh", "pr", "create", "--repo", repositoryName(request.Repository),
		"--base", request.BaseRef, "--head", request.HeadBranch,
		"--title", request.Title, "--body", request.Body)
	if err != nil {
		return fmt.Errorf("create exact pull request: %w", err)
	}
	return nil
}

func (gateway productionNonLocalGateway) RetryInfrastructureCheck(ctx context.Context, request issuedelivery.RetryInfrastructureCheckRequest) error {
	if err := gateway.validateRepository(ctx, request.Repository); err != nil {
		return err
	}
	if request.FailedRunID <= 0 || !validSHA(request.HeadSHA) {
		return errors.New("retry requires an exact failed run and candidate head")
	}
	run, err := gateway.workflowRun(ctx, request.Repository, request.FailedRunID)
	if err != nil {
		return err
	}
	if run.HeadSHA != request.HeadSHA || !trustedCheckIdentity(request.CheckIdentity, run.Name, run.Path) {
		return errors.New("failed workflow run identity is incompatible with the authorized retry")
	}
	_, err = gateway.output(ctx, "gh", "run", "rerun", strconv.FormatInt(request.FailedRunID, 10),
		"--repo", repositoryName(request.Repository), "--failed")
	if err != nil {
		return fmt.Errorf("retry failed infrastructure jobs: %w", err)
	}
	return nil
}

func (gateway productionNonLocalGateway) EnsureMerge(ctx context.Context, request issuedelivery.EnsureMergeRequest) error {
	if err := gateway.validateRepository(ctx, request.Repository); err != nil {
		return err
	}
	if request.Method != "merge" || request.PullRequest <= 0 || !validSHA(request.HeadSHA) || !validSHA(request.BaseSHA) {
		return errors.New("merge request has an incompatible exact identity or method")
	}
	raw, err := gateway.output(ctx, "gh", "pr", "view", strconv.Itoa(request.PullRequest),
		"--repo", repositoryName(request.Repository), "--json",
		"number,state,baseRefOid,headRefOid,closingIssuesReferences,mergedAt")
	if err != nil {
		return fmt.Errorf("observe pull request before merge: %w", err)
	}
	var pr remotePullRequest
	if err := exactJSON(raw, &pr); err != nil {
		return err
	}
	if pr.Number != request.PullRequest || pr.State != "OPEN" || pr.HeadRefOID != request.HeadSHA ||
		pr.BaseRefOID != request.BaseSHA || closingIssue(pr.ClosingIssuesReferences, request.Issue) == 0 {
		return errors.New("pull request changed before merge")
	}
	_, err = gateway.output(ctx, "gh", "pr", "merge", strconv.Itoa(request.PullRequest),
		"--repo", repositoryName(request.Repository), "--merge",
		"--match-head-commit", request.HeadSHA)
	if err != nil {
		return fmt.Errorf("merge exact pull request: %w", err)
	}
	return nil
}

func (gateway productionNonLocalGateway) EnsureRemoteIssueBranchAbsent(ctx context.Context, request issuedelivery.DeleteRemoteIssueBranchRequest) error {
	if err := gateway.validateRepository(ctx, request.Repository); err != nil {
		return err
	}
	if err := validBranchAndHead(request.Branch, request.HeadSHA); err != nil {
		return err
	}
	raw, err := gateway.output(ctx, "git", "ls-remote", "--refs", "origin", "refs/heads/"+request.Branch)
	if err != nil {
		return fmt.Errorf("observe issue branch before deletion: %w", err)
	}
	refs, err := parseRemoteRefs(raw)
	if err != nil {
		return err
	}
	if current := refs["refs/heads/"+request.Branch]; current == "" {
		return nil
	} else if current != request.HeadSHA {
		return errors.New("remote issue branch head is incompatible with authorized cleanup")
	}
	lease := "--force-with-lease=refs/heads/" + request.Branch + ":" + request.HeadSHA
	_, err = gateway.output(
		ctx, "git", "push", lease, "origin", ":refs/heads/"+request.Branch,
	)
	if err != nil {
		return fmt.Errorf("delete exact remote issue branch: %w", err)
	}
	return nil
}

func (gateway productionNonLocalGateway) observeChecks(ctx context.Context, request issuedelivery.NonLocalObserveRequest, baseSHA string) ([]issuedelivery.CICheckObservation, error) {
	repo := repositoryName(request.Repository)
	raw, err := gateway.output(ctx, "gh", "api", "-H", "Accept: application/vnd.github+json",
		"repos/"+repo+"/commits/"+request.HeadSHA+"/check-runs?filter=latest",
		"--jq", checkRunsProjection)
	if err != nil {
		return nil, fmt.Errorf("observe check runs: %w", err)
	}
	var runs checkRunsResponse
	if err := exactJSON(raw, &runs); err != nil {
		return nil, err
	}
	statusRaw, err := gateway.output(ctx, "gh", "api", "-H", "Accept: application/vnd.github+json",
		"repos/"+repo+"/commits/"+request.HeadSHA+"/statuses",
		"--jq", statusesProjection)
	if err != nil {
		return nil, fmt.Errorf("observe commit statuses: %w", err)
	}
	var statuses []commitStatus
	if err := exactJSON(statusRaw, &statuses); err != nil {
		return nil, err
	}
	var checks []issuedelivery.CICheckObservation
	for _, run := range runs.CheckRuns {
		if !isRequiredCheck(run.Name) {
			continue
		}
		runID, err := githubRunID(run.DetailsURL, request.Repository)
		if err != nil {
			return nil, err
		}
		workflow, err := gateway.workflowRun(ctx, request.Repository, runID)
		if err != nil {
			return nil, err
		}
		definitionSHA, err := gateway.definitionSHA(ctx, request.HeadSHA, workflow.Path)
		if err != nil {
			return nil, err
		}
		checks = append(checks, issuedelivery.CICheckObservation{
			RequiredCheck: deliveryevidence.RequiredCheck{
				Identity: run.Name, Conclusion: checkConclusion(run.Status, run.Conclusion), HeadSHA: run.HeadSHA,
			},
			StatusKind: issuedelivery.CIKindCheckRun,
			Publisher:  issuedelivery.TrustedPublisherEvidence{AppID: run.App.ID, Slug: run.App.Slug},
			Workflow: issuedelivery.CIWorkflowEvidence{
				Name: workflow.Name, Path: workflow.Path, RunID: runID,
				DefinitionRef: request.HeadSHA, DefinitionSHA: definitionSHA,
			},
			RunID: runID, DetailsURL: run.DetailsURL,
		})
	}
	for _, status := range statuses {
		if status.Context != "Governance / Validate authorization" {
			continue
		}
		runID, err := githubRunID(status.TargetURL, request.Repository)
		if err != nil {
			return nil, err
		}
		workflow, err := gateway.workflowRun(ctx, request.Repository, runID)
		if err != nil {
			return nil, err
		}
		definitionSHA, err := gateway.definitionSHA(ctx, baseSHA, workflow.Path)
		if err != nil {
			return nil, err
		}
		checks = append(checks, issuedelivery.CICheckObservation{
			RequiredCheck: deliveryevidence.RequiredCheck{
				Identity: status.Context, Conclusion: statusConclusion(status.State), HeadSHA: request.HeadSHA,
			},
			StatusKind: issuedelivery.CIKindCommitStatus,
			Publisher: issuedelivery.TrustedPublisherEvidence{
				Login: status.Creator.Login, ID: status.Creator.ID,
				Type: status.Creator.Type, HTMLURL: status.Creator.HTMLURL,
			},
			Source: issuedelivery.TrustedPublisherEvidence{AppID: 15368, Slug: "github-actions"},
			Workflow: issuedelivery.CIWorkflowEvidence{
				Name: workflow.Name, Path: workflow.Path, RunID: runID,
				DefinitionRef: baseSHA, DefinitionSHA: definitionSHA,
			},
			RunID: runID, DetailsURL: status.TargetURL,
		})
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].Identity < checks[j].Identity })
	return applyCIFailureAttributions(checks, gateway.attributions)
}

func applyCIFailureAttributions(
	checks []issuedelivery.CICheckObservation,
	attributions []advanceCIFailureAttribution,
) ([]issuedelivery.CICheckObservation, error) {
	applied := make(map[int]bool, len(attributions))
	for index := range checks {
		check := &checks[index]
		for decisionIndex, decision := range attributions {
			if decision.CheckIdentity != check.Identity || decision.RunID != check.RunID ||
				decision.HeadSHA != check.HeadSHA || decision.DetailsURL != check.DetailsURL {
				continue
			}
			if applied[decisionIndex] {
				return nil, errors.New("CI failure attribution matches more than one observed check")
			}
			conclusion := strings.ToLower(strings.TrimSpace(check.Conclusion))
			if conclusion != "failure" && conclusion != "failed" && conclusion != "cancelled" {
				return nil, errors.New("CI attribution applies only to an exact failed check")
			}
			if decision.Attribution != issuedelivery.FailureInfrastructure &&
				decision.Attribution != issuedelivery.FailureCandidate {
				return nil, errors.New("CI failure attribution must be infrastructure or candidate")
			}
			check.FailureAttribution = decision.Attribution
			applied[decisionIndex] = true
		}
	}
	for index := range attributions {
		if !applied[index] {
			return nil, errors.New("CI failure attribution does not match an exact observed failed run")
		}
	}
	return checks, nil
}

func (gateway productionNonLocalGateway) validateObserveRequest(ctx context.Context, request issuedelivery.NonLocalObserveRequest) error {
	if err := gateway.validateRepository(ctx, request.Repository); err != nil {
		return err
	}
	if request.Issue.Number <= 0 || request.Issue.NodeID == "" || request.BaseRef != "main" ||
		!strings.Contains(request.Branch, "issue-"+strconv.Itoa(request.Issue.Number)+"-") {
		return errors.New("non-local observation identity is incompatible with Packy delivery")
	}
	return validBranchAndHead(request.Branch, request.HeadSHA)
}

func (gateway productionNonLocalGateway) validateRepository(ctx context.Context, repository deliveryevidence.RepositoryIdentity) error {
	if gateway.runner == nil {
		return errors.New("non-local runner is required")
	}
	if repository.Owner != "yersonargotev" || repository.Name != "packy" || repository.NodeID == "" {
		return errors.New("non-local repository identity is not Packy")
	}
	raw, err := gateway.output(ctx, "gh", "repo", "view", repositoryName(repository),
		"--json", "id,nameWithOwner")
	if err != nil {
		return fmt.Errorf("observe repository identity: %w", err)
	}
	var observed remoteRepository
	if err := exactJSON(raw, &observed); err != nil {
		return err
	}
	if observed.ID != repository.NodeID || observed.NameWithOwner != repositoryName(repository) {
		return errors.New("observed repository identity is incompatible with Packy")
	}
	return nil
}

func (gateway productionNonLocalGateway) workflowRun(ctx context.Context, repository deliveryevidence.RepositoryIdentity, runID int64) (workflowRun, error) {
	raw, err := gateway.output(ctx, "gh", "api", "-H", "Accept: application/vnd.github+json",
		"repos/"+repositoryName(repository)+"/actions/runs/"+strconv.FormatInt(runID, 10),
		"--jq", workflowRunProjection)
	if err != nil {
		return workflowRun{}, fmt.Errorf("observe workflow run: %w", err)
	}
	var run workflowRun
	if err := exactJSON(raw, &run); err != nil {
		return workflowRun{}, err
	}
	if run.ID != runID {
		return workflowRun{}, errors.New("workflow run identity changed")
	}
	return run, nil
}

func (gateway productionNonLocalGateway) definitionSHA(ctx context.Context, ref, path string) (string, error) {
	if !validSHA(ref) || !trustedWorkflowPath(path) {
		return "", errors.New("workflow definition identity is incompatible")
	}
	raw, err := gateway.output(ctx, "git", "rev-parse", ref+":"+path)
	if err != nil {
		return "", fmt.Errorf("resolve exact workflow definition: %w", err)
	}
	sha := strings.TrimSpace(string(raw))
	if !validSHA(sha) {
		return "", errors.New("workflow definition blob identity is malformed")
	}
	return sha, nil
}

func (gateway productionNonLocalGateway) containsOriginMain(ctx context.Context, sha string) (bool, error) {
	raw, err := gateway.output(ctx, "git", "branch", "-r", "--contains", sha, "--format=%(refname:short)")
	if err != nil {
		return false, fmt.Errorf("observe origin/main containment: %w", err)
	}
	for _, ref := range strings.Fields(string(raw)) {
		if ref == "origin/main" {
			return true, nil
		}
	}
	return false, nil
}

func (gateway productionNonLocalGateway) output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "git" && gateway.repository != "" {
		args = append([]string{"-C", gateway.repository}, args...)
	}
	return gateway.runner.Output(ctx, name, args...)
}

func exactJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("command output contains more than one JSON value")
		}
		return err
	}
	return nil
}

func parseRemoteRefs(raw []byte) (map[string]string, error) {
	refs := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || !validSHA(fields[0]) || !strings.HasPrefix(fields[1], "refs/heads/") {
			return nil, errors.New("remote ref observation is malformed")
		}
		if _, duplicate := refs[fields[1]]; duplicate {
			return nil, errors.New("remote ref observation is ambiguous")
		}
		refs[fields[1]] = fields[0]
	}
	return refs, nil
}

func convertPullRequest(pr remotePullRequest, request issuedelivery.NonLocalObserveRequest) (issuedelivery.RemotePullRequestObservation, error) {
	if pr.Number <= 0 || !safeGitHubURL(pr.URL, request.Repository, "pull", pr.Number) ||
		!validSHA(pr.BaseRefOID) || !validSHA(pr.HeadRefOID) || pr.HeadRepository.ID == "" {
		return issuedelivery.RemotePullRequestObservation{}, errors.New("pull-request observation has an unsafe identity")
	}
	return issuedelivery.RemotePullRequestObservation{
		Number: pr.Number, URL: pr.URL, State: pr.State, BaseRef: pr.BaseRefName,
		BaseSHA: pr.BaseRefOID, HeadBranch: pr.HeadRefName, HeadSHA: pr.HeadRefOID,
		HeadRepositoryNodeID: pr.HeadRepository.ID,
		ClosingIssue:         closingIssue(pr.ClosingIssuesReferences, request.Issue),
	}, nil
}

func closingIssue(values []struct {
	Number int    `json:"number"`
	ID     string `json:"id"`
}, issue deliveryevidence.IssueIdentity) int {
	for _, value := range values {
		if value.Number == issue.Number && value.ID == issue.NodeID {
			return value.Number
		}
	}
	return 0
}

func githubRunID(value string, repository deliveryevidence.RepositoryIdentity) (int64, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return 0, errors.New("workflow run URL is unsafe")
	}
	prefix := "/" + repositoryName(repository) + "/actions/runs/"
	tail := strings.TrimPrefix(parsed.Path, prefix)
	if tail == parsed.Path {
		return 0, errors.New("workflow run URL has a foreign repository")
	}
	idText := strings.Split(tail, "/")[0]
	runID, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || runID <= 0 {
		return 0, errors.New("workflow run URL has no exact run identity")
	}
	return runID, nil
}

func checkConclusion(status, conclusion string) string {
	if strings.EqualFold(status, "completed") {
		return strings.ToLower(conclusion)
	}
	return ""
}

func statusConclusion(state string) string {
	if strings.EqualFold(state, "pending") {
		return ""
	}
	if strings.EqualFold(state, "error") {
		return "failure"
	}
	return strings.ToLower(state)
}

func trustedCheckIdentity(identity, workflowName, workflowPath string) bool {
	switch identity {
	case "Validate Packy-owned code", "Claude 2.1.203 package smoke":
		return workflowName == "CI" && workflowPath == ".github/workflows/ci.yml"
	case "Governance / Validate authorization":
		return workflowName == "Governance" && workflowPath == ".github/workflows/governance.yml"
	case "CodeQL":
		return workflowName == "Security" && workflowPath == ".github/workflows/security-pr.yml"
	default:
		return false
	}
}

func isRequiredCheck(identity string) bool {
	return trustedCheckIdentity(identity, "CI", ".github/workflows/ci.yml") ||
		trustedCheckIdentity(identity, "Security", ".github/workflows/security-pr.yml")
}

func trustedWorkflowPath(path string) bool {
	return path == ".github/workflows/ci.yml" ||
		path == ".github/workflows/governance.yml" ||
		path == ".github/workflows/security-pr.yml"
}

func validBranchAndHead(branch, head string) error {
	if !validSHA(head) || strings.TrimSpace(branch) != branch || branch == "" ||
		strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, " \t\r\n~^:?*[\\") ||
		strings.Contains(branch, "..") || strings.Contains(branch, "@{") {
		return errors.New("branch or candidate head identity is unsafe")
	}
	return nil
}

func validSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func safeGitHubURL(value string, repository deliveryevidence.RepositoryIdentity, kind string, number int) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host == "github.com" &&
		parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" &&
		parsed.Path == fmt.Sprintf("/%s/%s/%d", repositoryName(repository), kind, number)
}

func repositoryName(repository deliveryevidence.RepositoryIdentity) string {
	return repository.Owner + "/" + repository.Name
}
