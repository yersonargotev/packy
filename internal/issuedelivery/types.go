package issuedelivery

import (
	"context"
	"fmt"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

type State string

const (
	StateNeedsDecision State = "needs-decision"
	StateNeedsReview   State = "needs-review"
	StateWaiting       State = "waiting"
	StateBlocked       State = "blocked"
	StateCompleted     State = "completed"
)

type DecisionKind string

const (
	DecisionClassifyAuthorityItem DecisionKind = "classify-authority-item"
	DecisionSupplyCriterion       DecisionKind = "supply-acceptance-criterion"
)

type DecisionDisposition string

const (
	DecisionOwnedNow  DecisionDisposition = "owned-now"
	DecisionDeferred  DecisionDisposition = "deferred"
	DecisionForbidden DecisionDisposition = "forbidden"
)

type DecisionRequest struct {
	ID       string                `json:"id"`
	Kind     DecisionKind          `json:"kind"`
	Prompt   string                `json:"prompt"`
	Evidence string                `json:"evidence"`
	Options  []DecisionDisposition `json:"options"`
}

type Decision struct {
	RequestID    string              `json:"request_id"`
	Disposition  DecisionDisposition `json:"disposition"`
	Requirement  string              `json:"requirement"`
	EvidenceLink string              `json:"evidence_link"`
	Owner        string              `json:"owner,omitempty"`
}

type Request struct {
	RepositoryPath string
	IssueNumber    int
	Decision       *Decision
}

type Timing struct {
	Sequence    int    `json:"sequence"`
	Phase       string `json:"phase"`
	From        State  `json:"from,omitempty"`
	To          State  `json:"to"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

type Observations struct {
	Repository      deliveryevidence.RepositoryIdentity `json:"repository"`
	Issue           deliveryevidence.IssueIdentity      `json:"issue"`
	AuthoritySHA256 string                              `json:"authority_sha256"`
	CommitSHA       string                              `json:"commit_sha"`
	TreeSHA         string                              `json:"tree_sha"`
	WorkspaceClean  bool                                `json:"workspace_clean"`
}

type Outcome struct {
	RunID           string
	State           State
	Reason          string
	SupersedesRunID string
	Decision        *DecisionRequest
	Evidence        *deliveryevidence.Bundle
	Observations    Observations
	Timing          []Timing
}

type AuthorityItem struct {
	Text         string
	EvidenceLink string
}

type DependencyObservation struct {
	Identity string
	Number   int
	Title    string
	State    string
	URL      string
}

type ReferenceObservation struct {
	Identity string
	URL      string
}

type GitObservation struct {
	CommonDir       string
	Worktree        string
	OriginURL       string
	Owner           string
	Name            string
	StartingBaseSHA string
	HeadSHA         string
	TreeSHA         string
	WorkspaceClean  bool
}

type TrackerObservation struct {
	Repository   deliveryevidence.RepositoryIdentity
	Issue        deliveryevidence.IssueIdentity
	Title        string
	Body         string
	State        string
	Labels       []string
	Criteria     []AuthorityItem
	Exclusions   []AuthorityItem
	Dependencies []DependencyObservation
	References   []ReferenceObservation
	Ambiguities  []AuthorityItem
}

type GitObserver interface {
	ObserveGit(context.Context, string) (GitObservation, error)
}

type GitHubObserver interface {
	ObserveIssue(context.Context, GitObservation, int) (TrackerObservation, error)
}

type Clock interface {
	Now() time.Time
}

type Config struct {
	Git    GitObserver
	GitHub GitHubObserver
	Clock  Clock
}

type Module struct {
	git    GitObserver
	github GitHubObserver
	clock  Clock
	store  fileRunStore
}

func New(config Config) (*Module, error) {
	if config.Git == nil || config.GitHub == nil {
		return nil, fmt.Errorf("Git and GitHub observers are required")
	}
	if config.Clock == nil {
		config.Clock = systemClock{}
	}
	return &Module{git: config.Git, github: config.GitHub, clock: config.Clock}, nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type runRecord struct {
	Schema          string
	ID              string
	Repository      deliveryevidence.RepositoryIdentity
	Issue           deliveryevidence.IssueIdentity
	AuthoritySHA256 string
	State           State
	Reason          string
	SupersedesRunID string
	Evidence        *deliveryevidence.Bundle
	PendingDecision *DecisionRequest
	Decisions       []Decision
	Observations    Observations
	Timing          []Timing
	CreatedAt       string
	UpdatedAt       string
}

type DecisionMismatchError struct {
	Expected string
	Got      string
}

func (e *DecisionMismatchError) Error() string {
	return fmt.Sprintf("delivery decision %q does not match pending request %q", e.Got, e.Expected)
}
