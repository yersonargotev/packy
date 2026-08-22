// Package githubproposal publishes fully gated Managed Pack candidates as
// automation-owned GitHub pull requests.
package githubproposal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

const (
	botAuthorName  = "Packy Promotion Bot"
	botAuthorEmail = "packy-promotion@users.noreply.github.com"
	markerPrefix   = "<!-- packy-managed-pack-promotion: "
	markerSuffix   = " -->"
	commitPrefix   = "Packy-Managed-Pack-Promotion: "
)

var (
	sha1Pattern    = regexp.MustCompile(`^[0-9a-f]{40}$`)
	projectPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	branchPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

// Gateway is the least-privilege publication edge. Its push operation has no
// force option by design: every update must be a normal fast-forward push.
type Gateway interface {
	VerifyCandidate(context.Context, managedpackpromotion.Candidate) error
	Observe(context.Context, managedpackpromotion.Candidate) (Observation, error)
	Commit(context.Context, CommitRequest) (string, error)
	FastForwardPush(context.Context, string, string, string) error
	CreatePullRequest(context.Context, PullRequestMutation) error
	EditPullRequest(context.Context, int, PullRequestMutation) error
}

// Observation is one detached snapshot of every Git and GitHub identity used
// to decide whether publication may mutate anything.
type Observation struct {
	Actor        string
	BaseSHA      string
	Branch       *Branch
	PullRequests []PullRequest
}

type Branch struct {
	Name         string
	HeadSHA      string
	TreeSHA      string
	BaseAncestor bool
	Commits      []Commit
}

type Commit struct {
	SHA            string
	Parents        []string
	TreeSHA        string
	AuthorName     string
	AuthorEmail    string
	CommitterName  string
	CommitterEmail string
	Message        string
}

type PullRequest struct {
	Number     int
	URL        string
	State      string
	Draft      bool
	AutoMerge  bool
	BaseBranch string
	HeadBranch string
	HeadSHA    string
	Title      string
	Body       string
	Author     string
	LastEditor string
}

type CommitRequest struct {
	RepositoryRoot string
	ParentSHAs     []string
	TreeSHA        string
	Message        string
}

type PullRequestMutation struct {
	RepositoryRoot string
	BaseBranch     string
	HeadBranch     string
	Title          string
	Body           string
}

// Adapter implements managedpackpromotion.Publisher.
type Adapter struct {
	gateway Gateway
}

var _ managedpackpromotion.Publisher = (*Adapter)(nil)

func New(gateway Gateway) *Adapter {
	return &Adapter{gateway: gateway}
}

func (adapter *Adapter) Publish(ctx context.Context, candidate managedpackpromotion.Candidate) (managedpackpromotion.Publication, error) {
	if err := validateCandidate(candidate); err != nil {
		return managedpackpromotion.Publication{}, err
	}
	if adapter == nil || adapter.gateway == nil {
		return managedpackpromotion.Publication{}, errors.New("GitHub proposal gateway is required")
	}
	if err := adapter.gateway.VerifyCandidate(ctx, candidate); err != nil {
		return rejectPublication("verify sealed candidate", err)
	}

	observed, err := adapter.observe(ctx, candidate)
	if err != nil {
		return managedpackpromotion.Publication{}, err
	}
	if observed.BaseSHA != candidate.BaseSHA {
		return reject(managedpackpromotion.GateFreshness, "origin/main moved after the candidate was validated")
	}
	if observed.Branch == nil {
		if len(observed.PullRequests) != 0 {
			return reject(managedpackpromotion.GateProposalOwnership, "a pull request already claims the deterministic branch without its exact remote branch")
		}
		return adapter.create(ctx, candidate, observed)
	}
	return adapter.adopt(ctx, candidate, observed)
}

func (adapter *Adapter) create(ctx context.Context, candidate managedpackpromotion.Candidate, observed Observation) (managedpackpromotion.Publication, error) {
	head := candidate.HeadSHA
	if err := adapter.gateway.FastForwardPush(ctx, candidate.RepositoryRoot, head, candidate.Branch); err != nil {
		return rejectPublication("push automation-owned proposal branch", err)
	}

	afterPush, err := adapter.observe(ctx, candidate)
	if err != nil {
		return managedpackpromotion.Publication{}, err
	}
	if afterPush.BaseSHA != candidate.BaseSHA {
		return reject(managedpackpromotion.GateFreshness, "origin/main moved while the proposal branch was being created")
	}
	if reason := exactPublishedBranch(afterPush.Branch, candidate, observed.Actor, head); reason != "" {
		return reject(managedpackpromotion.GatePublication, reason)
	}
	if len(afterPush.PullRequests) != 0 {
		return adapter.adopt(ctx, candidate, afterPush)
	}
	return adapter.createPullRequest(ctx, candidate, afterPush, head)
}

func (adapter *Adapter) adopt(ctx context.Context, candidate managedpackpromotion.Candidate, observed Observation) (managedpackpromotion.Publication, error) {
	branch := observed.Branch
	if reason := ownedBranch(branch, candidate, observed.Actor); reason != "" {
		return adapter.regenerateStale(ctx, candidate, observed, reason)
	}
	if len(observed.PullRequests) == 0 {
		if !latestCommitMatches(branch, candidate, observed.Actor) {
			return reject(managedpackpromotion.GateProposalOwnership, "the deterministic branch has no pull request and no exact publication record for this candidate")
		}
		return adapter.createPullRequest(ctx, candidate, observed, branch.HeadSHA)
	}
	if len(observed.PullRequests) != 1 {
		return reject(managedpackpromotion.GateProposalOwnership, "multiple pull requests claim the deterministic proposal branch")
	}

	pr := observed.PullRequests[0]
	record, summary, reason := ownedPullRequest(pr, candidate, observed.Actor)
	if reason != "" {
		return reject(managedpackpromotion.GateProposalOwnership, reason)
	}
	if pr.HeadSHA != branch.HeadSHA {
		return reject(managedpackpromotion.GateProposalOwnership, "the pull request head does not equal the observed remote branch head")
	}

	if record.HeadSHA != branch.HeadSHA {
		if !isRecoverableMetadataLag(branch, record, candidate, observed.Actor) {
			return reject(managedpackpromotion.GateProposalOwnership, "proposal metadata does not describe the current automation-owned branch head")
		}
		return adapter.editAndVerify(ctx, candidate, observed, pr, branch.HeadSHA)
	}
	if record.TreeSHA != branch.TreeSHA {
		return reject(managedpackpromotion.GateProposalOwnership, "proposal metadata does not describe the current branch tree")
	}
	if record.matchesCandidate(candidate) {
		if summary != candidate.Summary {
			return reject(managedpackpromotion.GateProposalOwnership, "the sealed proposal summary differs from the published automation metadata")
		}
		return managedpackpromotion.Publication{NoChangeReason: "the exact automation-owned proposal already exists"}, nil
	}

	return adapter.update(ctx, candidate, observed, pr, []string{branch.HeadSHA})
}

func (adapter *Adapter) regenerateStale(ctx context.Context, candidate managedpackpromotion.Candidate, observed Observation, branchReason string) (managedpackpromotion.Publication, error) {
	if observed.Branch == nil || len(observed.PullRequests) != 1 {
		return reject(managedpackpromotion.GateProposalOwnership, branchReason)
	}
	pr := observed.PullRequests[0]
	record, _, reason := ownedPullRequestOnAnyBase(pr, candidate, observed.Actor)
	if reason != "" || pr.HeadSHA != observed.Branch.HeadSHA || record.HeadSHA != observed.Branch.HeadSHA ||
		record.TreeSHA != observed.Branch.TreeSHA || record.BaseSHA == candidate.BaseSHA ||
		!latestCommitMatchesRecord(observed.Branch, record, observed.Actor) {
		return reject(managedpackpromotion.GateProposalOwnership, branchReason)
	}
	if reason := ownedBranchAtBase(observed.Branch, candidate, observed.Actor, record.BaseSHA, false); reason != "" {
		return reject(managedpackpromotion.GateProposalOwnership, reason)
	}
	return adapter.update(ctx, candidate, observed, pr, []string{observed.Branch.HeadSHA, candidate.BaseSHA})
}

func (adapter *Adapter) update(ctx context.Context, candidate managedpackpromotion.Candidate, observed Observation, pr PullRequest, parents []string) (managedpackpromotion.Publication, error) {
	record := recordFor(candidate, observed.Actor, "")
	head, err := adapter.gateway.Commit(ctx, CommitRequest{
		RepositoryRoot: candidate.RepositoryRoot,
		ParentSHAs:     append([]string(nil), parents...),
		TreeSHA:        candidate.ResultTreeSHA,
		Message:        commitMessage(candidate, record),
	})
	if err != nil {
		return rejectPublication("append automation-owned proposal commit", err)
	}
	if !sha1Pattern.MatchString(head) {
		return reject(managedpackpromotion.GatePublication, "git commit-tree returned an invalid commit identity")
	}
	if err := adapter.gateway.FastForwardPush(ctx, candidate.RepositoryRoot, head, candidate.Branch); err != nil {
		return rejectPublication("fast-forward the automation-owned proposal branch", err)
	}

	afterPush, err := adapter.observe(ctx, candidate)
	if err != nil {
		return managedpackpromotion.Publication{}, err
	}
	if afterPush.BaseSHA != candidate.BaseSHA {
		return reject(managedpackpromotion.GateFreshness, "origin/main moved while the proposal branch was being updated")
	}
	if reason := exactPublishedBranch(afterPush.Branch, candidate, observed.Actor, head); reason != "" {
		return reject(managedpackpromotion.GatePublication, reason)
	}
	if len(afterPush.PullRequests) != 1 || afterPush.PullRequests[0].Number != pr.Number {
		return reject(managedpackpromotion.GateProposalOwnership, "pull request identity changed while the proposal branch was being updated")
	}
	if afterPush.PullRequests[0].Body == bodyFor(candidate, observed.Actor, head) && afterPush.PullRequests[0].Title == titleFor(candidate) {
		return adapter.verifyFinal(candidate, afterPush, pr.Number, head)
	}
	oldRecord, _, reason := ownedPullRequestOnAnyBase(afterPush.PullRequests[0], candidate, observed.Actor)
	if reason != "" {
		return reject(managedpackpromotion.GateProposalOwnership, reason)
	}
	if !isRecoverableMetadataLag(afterPush.Branch, oldRecord, candidate, observed.Actor) {
		return reject(managedpackpromotion.GateProposalOwnership, "pull request metadata changed while the proposal branch was being updated")
	}
	return adapter.editAndVerify(ctx, candidate, afterPush, pr, head)
}

func (adapter *Adapter) createPullRequest(ctx context.Context, candidate managedpackpromotion.Candidate, observed Observation, head string) (managedpackpromotion.Publication, error) {
	mutation := mutationFor(candidate, observed.Actor, head)
	if err := adapter.gateway.CreatePullRequest(ctx, mutation); err != nil {
		return rejectPublication("create ready pull request", err)
	}
	final, err := adapter.observe(ctx, candidate)
	if err != nil {
		return managedpackpromotion.Publication{}, err
	}
	return adapter.verifyFinal(candidate, final, 0, head)
}

func (adapter *Adapter) editAndVerify(ctx context.Context, candidate managedpackpromotion.Candidate, observed Observation, pr PullRequest, head string) (managedpackpromotion.Publication, error) {
	if err := adapter.gateway.EditPullRequest(ctx, pr.Number, mutationFor(candidate, observed.Actor, head)); err != nil {
		return rejectPublication("update automation-owned pull request metadata", err)
	}
	final, err := adapter.observe(ctx, candidate)
	if err != nil {
		return managedpackpromotion.Publication{}, err
	}
	return adapter.verifyFinal(candidate, final, pr.Number, head)
}

func (adapter *Adapter) verifyFinal(candidate managedpackpromotion.Candidate, observed Observation, expectedNumber int, expectedHead string) (managedpackpromotion.Publication, error) {
	if observed.BaseSHA != candidate.BaseSHA {
		return reject(managedpackpromotion.GateFreshness, "origin/main moved before publication could be verified")
	}
	if reason := exactPublishedBranch(observed.Branch, candidate, observed.Actor, expectedHead); reason != "" {
		return reject(managedpackpromotion.GatePublication, reason)
	}
	if len(observed.PullRequests) != 1 {
		return reject(managedpackpromotion.GatePublication, "the exact pull request was not observable after publication")
	}
	pr := observed.PullRequests[0]
	if expectedNumber != 0 && pr.Number != expectedNumber {
		return reject(managedpackpromotion.GateProposalOwnership, "pull request number changed during publication")
	}
	record, summary, reason := ownedPullRequest(pr, candidate, observed.Actor)
	if reason != "" {
		return reject(managedpackpromotion.GateProposalOwnership, reason)
	}
	if pr.HeadSHA != expectedHead || record.HeadSHA != expectedHead || !record.matchesCandidate(candidate) || summary != candidate.Summary {
		return reject(managedpackpromotion.GatePublication, "published pull request does not equal the sealed candidate")
	}
	return managedpackpromotion.Publication{Proposal: &managedpackpromotion.Proposal{
		Branch: candidate.Branch, Number: pr.Number, URL: pr.URL, HeadSHA: expectedHead,
	}}, nil
}

func (adapter *Adapter) observe(ctx context.Context, candidate managedpackpromotion.Candidate) (Observation, error) {
	observed, err := adapter.gateway.Observe(ctx, candidate)
	if err != nil {
		_, rejection := rejectPublication("observe GitHub proposal state", err)
		return Observation{}, rejection
	}
	if strings.TrimSpace(observed.Actor) == "" {
		return Observation{}, managedpackpromotion.Reject(managedpackpromotion.GatePublication, "GitHub did not identify the authenticated publication actor")
	}
	return observed, nil
}

type publicationRecord struct {
	BaseSHA      string `json:"base"`
	Branch       string `json:"branch"`
	CandidateSHA string `json:"candidate"`
	CandidateID  string `json:"candidate_id"`
	Coordinate   string `json:"coordinate"`
	HeadSHA      string `json:"head"`
	Owner        string `json:"owner"`
	Project      string `json:"project"`
	TreeSHA      string `json:"tree"`
	Version      int    `json:"version"`
}

func recordFor(candidate managedpackpromotion.Candidate, owner, head string) publicationRecord {
	return publicationRecord{
		BaseSHA: candidate.BaseSHA, Branch: candidate.Branch, CandidateSHA: candidate.HeadSHA,
		CandidateID: candidate.ID, Coordinate: candidate.Coordinate.String(), HeadSHA: head,
		Owner: owner, Project: candidate.Project, TreeSHA: candidate.ResultTreeSHA, Version: 1,
	}
}

func (record publicationRecord) matchesCandidate(candidate managedpackpromotion.Candidate) bool {
	return record.BaseSHA == candidate.BaseSHA && record.Branch == candidate.Branch &&
		record.CandidateSHA == candidate.HeadSHA && record.CandidateID == candidate.ID &&
		record.Coordinate == candidate.Coordinate.String() && record.Project == candidate.Project &&
		record.TreeSHA == candidate.ResultTreeSHA && record.Version == 1
}

func titleFor(candidate managedpackpromotion.Candidate) string {
	return "Promote Managed Pack " + candidate.Coordinate.String()
}

func bodyFor(candidate managedpackpromotion.Candidate, owner, head string) string {
	record := recordFor(candidate, owner, head)
	encoded, _ := json.Marshal(record)
	return candidate.Summary + "\n\n" + markerPrefix + string(encoded) + markerSuffix
}

func mutationFor(candidate managedpackpromotion.Candidate, owner, head string) PullRequestMutation {
	return PullRequestMutation{
		RepositoryRoot: candidate.RepositoryRoot, BaseBranch: "main", HeadBranch: candidate.Branch,
		Title: titleFor(candidate), Body: bodyFor(candidate, owner, head),
	}
}

func commitMessage(candidate managedpackpromotion.Candidate, record publicationRecord) string {
	record.HeadSHA = ""
	encoded, _ := json.Marshal(record)
	return titleFor(candidate) + "\n\n" + commitPrefix + string(encoded)
}

func parseBody(body string) (publicationRecord, string, error) {
	separator := "\n\n" + markerPrefix
	index := strings.LastIndex(body, separator)
	if index < 0 || !strings.HasSuffix(body, markerSuffix) {
		return publicationRecord{}, "", errors.New("missing canonical ownership marker")
	}
	summary := body[:index]
	encoded := body[index+len(separator) : len(body)-len(markerSuffix)]
	record, err := parseRecord(encoded)
	if err != nil {
		return publicationRecord{}, "", err
	}
	canonical, _ := json.Marshal(record)
	if encoded != string(canonical) || body != summary+separator+string(canonical)+markerSuffix {
		return publicationRecord{}, "", errors.New("ownership marker is not canonical")
	}
	return record, summary, nil
}

func parseCommitRecord(message string) (publicationRecord, error) {
	index := strings.LastIndex(message, "\n\n"+commitPrefix)
	if index < 0 {
		return publicationRecord{}, errors.New("missing commit ownership record")
	}
	encoded := message[index+len("\n\n"+commitPrefix):]
	record, err := parseRecord(encoded)
	if err != nil {
		return publicationRecord{}, err
	}
	canonical, _ := json.Marshal(record)
	if encoded != string(canonical) || record.HeadSHA != "" {
		return publicationRecord{}, errors.New("commit ownership record is not canonical")
	}
	return record, nil
}

func parseRecord(encoded string) (publicationRecord, error) {
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var record publicationRecord
	if err := decoder.Decode(&record); err != nil {
		return publicationRecord{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return publicationRecord{}, errors.New("ownership marker has trailing JSON")
	}
	coordinate, coordinateErr := managedpackpromotion.ParseCoordinate(record.Coordinate)
	if record.Version != 1 || strings.TrimSpace(record.Owner) == "" || strings.TrimSpace(record.CandidateID) == "" ||
		coordinateErr != nil || coordinate.String() != record.Coordinate || !projectPattern.MatchString(record.Project) || !validBranch(record.Branch) ||
		!sha1Pattern.MatchString(record.BaseSHA) || !sha1Pattern.MatchString(record.CandidateSHA) ||
		!sha1Pattern.MatchString(record.TreeSHA) || (record.HeadSHA != "" && !sha1Pattern.MatchString(record.HeadSHA)) {
		return publicationRecord{}, errors.New("ownership marker has invalid fields")
	}
	return record, nil
}

func ownedPullRequest(pr PullRequest, candidate managedpackpromotion.Candidate, actor string) (publicationRecord, string, string) {
	record, summary, reason := ownedPullRequestOnAnyBase(pr, candidate, actor)
	if reason != "" {
		return publicationRecord{}, "", reason
	}
	if record.BaseSHA != candidate.BaseSHA {
		return publicationRecord{}, "", "pull request ownership metadata belongs to a different proposal base"
	}
	return record, summary, ""
}

func ownedPullRequestOnAnyBase(pr PullRequest, candidate managedpackpromotion.Candidate, actor string) (publicationRecord, string, string) {
	if pr.State != "OPEN" {
		return publicationRecord{}, "", "the deterministic branch has a closed or merged pull request"
	}
	if pr.Draft {
		return publicationRecord{}, "", "the automation-owned pull request was changed to draft"
	}
	if pr.AutoMerge {
		return publicationRecord{}, "", "auto-merge is enabled on the automation-owned pull request"
	}
	if pr.BaseBranch != "main" || pr.HeadBranch != candidate.Branch {
		return publicationRecord{}, "", "pull request base or head identity differs from the deterministic proposal"
	}
	if pr.Author != actor || (pr.LastEditor != "" && pr.LastEditor != actor) {
		return publicationRecord{}, "", "pull request author or last editor is not the publication actor"
	}
	if pr.Title != titleFor(candidate) {
		return publicationRecord{}, "", "pull request title was edited"
	}
	record, summary, err := parseBody(pr.Body)
	if err != nil {
		return publicationRecord{}, "", "pull request body was edited or has invalid ownership metadata"
	}
	if record.Owner != actor || record.Coordinate != candidate.Coordinate.String() || record.Project != candidate.Project ||
		record.Branch != candidate.Branch {
		return publicationRecord{}, "", "pull request ownership metadata belongs to a different proposal"
	}
	return record, summary, ""
}

func ownedBranch(branch *Branch, candidate managedpackpromotion.Candidate, actor string) string {
	return ownedBranchAtBase(branch, candidate, actor, candidate.BaseSHA, true)
}

func ownedBranchAtBase(branch *Branch, candidate managedpackpromotion.Candidate, actor, expectedBase string, requireBaseAncestor bool) string {
	if branch == nil || branch.Name != candidate.Branch || (requireBaseAncestor && !branch.BaseAncestor) {
		return "the deterministic branch is missing or is not descended from the sealed base"
	}
	if len(branch.Commits) == 0 || branch.Commits[0].SHA != branch.HeadSHA {
		return "the deterministic branch history cannot be proven from the sealed base"
	}
	if branch.TreeSHA != branch.Commits[0].TreeSHA {
		return "the deterministic branch tree does not equal its observed head commit"
	}

	oldest := branch.Commits[len(branch.Commits)-1]
	if !automationCommit(oldest) || len(oldest.Parents) != 1 {
		return "the deterministic branch does not begin with its automation-owned candidate"
	}
	activeBase := oldest.Parents[0]
	if strings.TrimSpace(oldest.Message) != "Promote "+candidate.Coordinate.String() {
		record, err := parseCommitRecord(oldest.Message)
		if err != nil || record.Owner != actor || record.Coordinate != candidate.Coordinate.String() ||
			record.Project != candidate.Project || record.Branch != candidate.Branch || record.TreeSHA != oldest.TreeSHA ||
			record.BaseSHA != activeBase {
			return "the deterministic branch does not begin with its automation-owned candidate"
		}
	}
	for index := len(branch.Commits) - 2; index >= 0; index-- {
		commit := branch.Commits[index]
		previous := branch.Commits[index+1]
		if !automationCommit(commit) || len(commit.Parents) < 1 || len(commit.Parents) > 2 || commit.Parents[0] != previous.SHA {
			return "the deterministic branch contains non-append-only history"
		}
		record, err := parseCommitRecord(commit.Message)
		if err != nil || record.Owner != actor || record.Coordinate != candidate.Coordinate.String() ||
			record.Project != candidate.Project || record.Branch != candidate.Branch || record.TreeSHA != commit.TreeSHA {
			return "the deterministic branch contains a commit without matching automation ownership"
		}
		if len(commit.Parents) == 1 {
			if record.BaseSHA != activeBase {
				return "the deterministic branch contains an append commit for a different base"
			}
		} else {
			if commit.Parents[1] != record.BaseSHA || record.BaseSHA == activeBase {
				return "the deterministic branch contains an invalid base-regeneration merge"
			}
			activeBase = record.BaseSHA
		}
	}
	if activeBase != expectedBase {
		return "the deterministic branch history belongs to a different base"
	}
	return ""
}

func automationCommit(commit Commit) bool {
	return commit.AuthorName == botAuthorName && commit.AuthorEmail == botAuthorEmail &&
		commit.CommitterName == botAuthorName && commit.CommitterEmail == botAuthorEmail
}

func latestCommitMatchesRecord(branch *Branch, record publicationRecord, actor string) bool {
	if branch == nil || len(branch.Commits) == 0 || branch.HeadSHA != record.HeadSHA || branch.TreeSHA != record.TreeSHA {
		return false
	}
	latest := branch.Commits[0]
	if latest.SHA == record.CandidateSHA {
		return automationCommit(latest) && latest.TreeSHA == record.TreeSHA && len(latest.Parents) == 1 &&
			latest.Parents[0] == record.BaseSHA && strings.TrimSpace(latest.Message) == "Promote "+record.Coordinate
	}
	commitRecord, err := parseCommitRecord(latest.Message)
	if err != nil {
		return false
	}
	commitRecord.HeadSHA = record.HeadSHA
	return commitRecord == record && record.Owner == actor
}

func exactPublishedBranch(branch *Branch, candidate managedpackpromotion.Candidate, actor, head string) string {
	if reason := ownedBranch(branch, candidate, actor); reason != "" {
		return reason
	}
	if branch.HeadSHA != head || branch.TreeSHA != candidate.ResultTreeSHA || !latestCommitMatches(branch, candidate, actor) {
		return "the remote proposal branch does not equal the exact published candidate"
	}
	return ""
}

func latestCommitMatches(branch *Branch, candidate managedpackpromotion.Candidate, actor string) bool {
	if branch == nil || len(branch.Commits) == 0 || branch.TreeSHA != candidate.ResultTreeSHA {
		return false
	}
	if branch.HeadSHA == candidate.HeadSHA && isDetachedCandidateCommit(branch.Commits[0], candidate) {
		return true
	}
	record, err := parseCommitRecord(branch.Commits[0].Message)
	return err == nil && record.Owner == actor && record.matchesCandidate(candidate)
}

func isDetachedCandidateCommit(commit Commit, candidate managedpackpromotion.Candidate) bool {
	return commit.SHA == candidate.HeadSHA && commit.TreeSHA == candidate.ResultTreeSHA && isOwnedDetachedCandidateCommit(commit, candidate)
}

func isOwnedDetachedCandidateCommit(commit Commit, candidate managedpackpromotion.Candidate) bool {
	return len(commit.Parents) == 1 && commit.Parents[0] == candidate.BaseSHA &&
		strings.TrimSpace(commit.Message) == "Promote "+candidate.Coordinate.String()
}

func isRecoverableMetadataLag(branch *Branch, old publicationRecord, candidate managedpackpromotion.Candidate, actor string) bool {
	if !latestCommitMatches(branch, candidate, actor) || len(branch.Commits[0].Parents) < 1 ||
		len(branch.Commits[0].Parents) > 2 || branch.Commits[0].Parents[0] != old.HeadSHA {
		return false
	}
	return old.Owner == actor && old.Coordinate == candidate.Coordinate.String() && old.Project == candidate.Project &&
		old.Branch == candidate.Branch
}

func validateCandidate(candidate managedpackpromotion.Candidate) error {
	coordinate, err := managedpackpromotion.ParseCoordinate(candidate.Coordinate.String())
	if err != nil || coordinate != candidate.Coordinate {
		return errors.New("publisher received an invalid Managed Pack coordinate")
	}
	if strings.TrimSpace(candidate.ID) == "" || strings.TrimSpace(candidate.Summary) == "" ||
		!projectPattern.MatchString(candidate.Project) || strings.TrimSpace(candidate.RepositoryRoot) == "" ||
		!sha1Pattern.MatchString(candidate.BaseSHA) || !sha1Pattern.MatchString(candidate.HeadSHA) ||
		!sha1Pattern.MatchString(candidate.ResultTreeSHA) || !validBranch(candidate.Branch) {
		return errors.New("publisher received an incomplete or invalid sealed candidate")
	}
	return nil
}

func validBranch(branch string) bool {
	return branchPattern.MatchString(branch) && !strings.Contains(branch, "..") && !strings.Contains(branch, "@{") &&
		!strings.Contains(branch, "//") && !strings.HasSuffix(branch, ".") && !strings.HasSuffix(branch, "/")
}

func reject(gate managedpackpromotion.Gate, reason string) (managedpackpromotion.Publication, error) {
	return managedpackpromotion.Publication{}, managedpackpromotion.Reject(gate, reason)
}

func rejectPublication(operation string, err error) (managedpackpromotion.Publication, error) {
	return reject(managedpackpromotion.GatePublication, fmt.Sprintf("%s: %v", operation, err))
}
