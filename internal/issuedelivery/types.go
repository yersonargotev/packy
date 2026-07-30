package issuedelivery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	DecisionAdjudicateFindings    DecisionKind = "adjudicate-review-findings"
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
	RepositoryPath          string
	IssueNumber             int
	Decision                *Decision
	Repair                  *RepairDecision
	QualificationReview     *QualificationReview
	QualificationCorrection *QualificationCorrection
	NonLocal                *NonLocalAuthorization
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
	RunID                    string
	State                    State
	Reason                   string
	SupersedesRunID          string
	Decision                 *DecisionRequest
	Evidence                 *deliveryevidence.Bundle
	Observations             Observations
	Candidate                *Candidate
	Repair                   *RepairDecisionRequest
	QualificationCorrection  *QualificationCorrectionRequest
	QualificationApproved    bool
	QualificationReviews     []QualificationReview
	QualificationCorrections []QualificationCorrection
	LocalReadiness           *LocalReadiness
	NonLocal                 *NonLocalDelivery
	Timing                   []Timing
}

type AuthorityItem struct {
	Text         string
	EvidenceLink string
	Plan         *QualificationPlan
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
	Branch          string
}

type TrackerObservation struct {
	Repository    deliveryevidence.RepositoryIdentity
	Issue         deliveryevidence.IssueIdentity
	Specification *SpecificationObservation
	Title         string
	Body          string
	State         string
	Labels        []string
	Criteria      []AuthorityItem
	Exclusions    []AuthorityItem
	Dependencies  []DependencyObservation
	References    []ReferenceObservation
	Ambiguities   []AuthorityItem
}

type SpecificationObservation struct {
	Identity deliveryevidence.SpecIdentity
	Title    string
	Body     string
	State    string
	URL      string
	Labels   []string
}

type GitObserver interface {
	ObserveGit(context.Context, string) (GitObservation, error)
}

type GitHubObserver interface {
	ObserveIssue(context.Context, GitObservation, int) (TrackerObservation, error)
}

type NonLocalGateway interface {
	ObserveNonLocal(context.Context, NonLocalObserveRequest) (NonLocalObservation, error)
	PushIssueBranch(context.Context, PushIssueBranchRequest) error
	EnsurePullRequest(context.Context, EnsurePullRequestRequest) error
	RetryInfrastructureCheck(context.Context, RetryInfrastructureCheckRequest) error
	EnsureMerge(context.Context, EnsureMergeRequest) error
	EnsureRemoteIssueBranchAbsent(context.Context, DeleteRemoteIssueBranchRequest) error
}

type LocalCompletionGateway interface {
	ObserveLocalCompletion(context.Context, LocalCompletionObserveRequest) (LocalCompletionObservation, error)
	EnsureManagedWorktreeAbsent(context.Context, RemoveManagedWorktreeRequest) error
	EnsureLocalIssueBranchAbsent(context.Context, DeleteLocalIssueBranchRequest) error
	EnsureLocalMainFastForward(context.Context, FastForwardLocalMainRequest) error
}

type Clock interface {
	Now() time.Time
}

type ReviewExecutor interface {
	Review(context.Context, ReviewRequest) (CandidateReview, error)
}

type CandidateRiskObserver interface {
	ObserveCandidateRisk(context.Context, CandidateRiskRequest) (CandidateRiskObservation, error)
}

type SpecialistReviewExecutor interface {
	ReviewSpecialist(context.Context, SpecialistReviewRequest) (SpecialistReview, error)
}

type BoundaryValidationExecutor interface {
	ValidateBoundary(context.Context, BoundaryValidationRequest) (BoundaryValidationResult, error)
}

type ValidationExecutor interface {
	Focused(context.Context, ValidationRequest) (ValidationResult, error)
	Exhaustive(context.Context, ValidationRequest) (ValidationResult, error)
}

type CandidateRiskRequest struct {
	RunID           string
	CandidateID     string
	RepositoryPath  string
	StartingBaseSHA string
	CommitSHA       string
	TreeSHA         string
}

type CandidateRiskObservation struct {
	CandidateID string              `json:"candidate_id"`
	CommitSHA   string              `json:"commit_sha"`
	TreeSHA     string              `json:"tree_sha"`
	Effects     []EffectObservation `json:"effects"`
	Completed   bool                `json:"completed"`
}

type ReviewRequest struct {
	RunID       string
	CandidateID string
	Repository  deliveryevidence.RepositoryIdentity
	Issue       deliveryevidence.IssueIdentity
	Axis        deliveryevidence.ReviewAxis
	BaseSHA     string
	CommitSHA   string
	TreeSHA     string
}

type CandidateReview struct {
	CandidateID string                           `json:"candidate_id"`
	Axis        deliveryevidence.ReviewAxis      `json:"axis"`
	Findings    []deliveryevidence.ReviewFinding `json:"findings"`
	Completed   bool                             `json:"completed"`
}

type SpecialistReviewRequest struct {
	RunID       string
	CandidateID string
	Repository  deliveryevidence.RepositoryIdentity
	Issue       deliveryevidence.IssueIdentity
	Boundary    SensitiveBoundary
	Specialist  string
	BaseSHA     string
	CommitSHA   string
	TreeSHA     string
}

type SpecialistFinding struct {
	ID       string                           `json:"id"`
	Severity deliveryevidence.FindingSeverity `json:"severity"`
	Citation string                           `json:"citation"`
	Location string                           `json:"location"`
	Evidence string                           `json:"evidence"`
}

type SpecialistReview struct {
	CandidateID string              `json:"candidate_id"`
	Boundary    SensitiveBoundary   `json:"boundary"`
	Specialist  string              `json:"specialist"`
	Findings    []SpecialistFinding `json:"findings"`
	Completed   bool                `json:"completed"`
}

type ValidationRequest struct {
	RunID          string
	CandidateID    string
	Repository     deliveryevidence.RepositoryIdentity
	Issue          deliveryevidence.IssueIdentity
	CommitSHA      string
	TreeSHA        string
	HomeRoot       string
	ConfigRoot     string
	AcceptanceRows []deliveryevidence.AcceptanceRow
	Profile        deliveryevidence.DeliveryRiskProfile
	Boundaries     []SensitiveBoundary
}

type ValidationResult struct {
	CommitSHA                  string            `json:"commit_sha"`
	TreeSHA                    string            `json:"tree_sha"`
	Command                    string            `json:"command"`
	HomeRoot                   string            `json:"home_root"`
	ConfigRoot                 string            `json:"config_root"`
	CheckoutSHA256             string            `json:"checkout_sha256,omitempty"`
	ValidatorIdentity          string            `json:"validator_identity,omitempty"`
	ValidatorSHA256            string            `json:"validator_sha256,omitempty"`
	ValidatorIdentityExpiresAt string            `json:"validator_identity_expires_at,omitempty"`
	WorkspaceClean             bool              `json:"workspace_clean"`
	Sandboxed                  bool              `json:"sandboxed"`
	Succeeded                  bool              `json:"succeeded"`
	Completed                  bool              `json:"completed"`
	Acceptance                 []AcceptanceProof `json:"acceptance,omitempty"`
}

type AcceptanceProof struct {
	Identity              string `json:"identity"`
	PositiveEvidence      string `json:"positive_evidence"`
	NegativeEvidence      string `json:"negative_evidence"`
	FailureEvidence       string `json:"failure_evidence"`
	MutationEvidence      string `json:"mutation_evidence"`
	CompatibilityEvidence string `json:"compatibility_evidence"`
	PreservationEvidence  string `json:"preservation_evidence"`
	MigrationEvidence     string `json:"migration_evidence"`
}

type BoundaryValidationRequest struct {
	RunID       string
	CandidateID string
	Repository  deliveryevidence.RepositoryIdentity
	Issue       deliveryevidence.IssueIdentity
	Boundary    SensitiveBoundary
	CommitSHA   string
	TreeSHA     string
	HomeRoot    string
	ConfigRoot  string
}

type BoundaryValidationResult struct {
	CandidateID               string            `json:"candidate_id"`
	Boundary                  SensitiveBoundary `json:"boundary"`
	CommitSHA                 string            `json:"commit_sha"`
	TreeSHA                   string            `json:"tree_sha"`
	Command                   string            `json:"command"`
	ToolIdentity              string            `json:"tool_identity"`
	ToolSHA256                string            `json:"tool_sha256"`
	HomeRoot                  string            `json:"home_root"`
	ConfigRoot                string            `json:"config_root"`
	OperatorStateBeforeSHA256 string            `json:"operator_state_before_sha256"`
	OperatorStateAfterSHA256  string            `json:"operator_state_after_sha256"`
	WriteManifestSHA256       string            `json:"write_manifest_sha256"`
	Evidence                  string            `json:"evidence"`
	Sandboxed                 bool              `json:"sandboxed"`
	Succeeded                 bool              `json:"succeeded"`
	Completed                 bool              `json:"completed"`
}

type BoundaryProof struct {
	Result      BoundaryValidationResult `json:"result"`
	CompletedAt string                   `json:"completed_at"`
}

type RepairClass string

const (
	RepairBounded           RepairClass = "bounded"
	RepairCandidateChanging RepairClass = "candidate-changing"
)

type FindingDisposition string

const (
	FindingAccepted FindingDisposition = "accepted"
	FindingRejected FindingDisposition = "rejected-with-evidence"
)

type FindingDecision struct {
	FindingID   string             `json:"finding_id"`
	Disposition FindingDisposition `json:"disposition"`
	Evidence    string             `json:"evidence"`
}

type RepairDecision struct {
	CandidateID string            `json:"candidate_id"`
	Class       RepairClass       `json:"class"`
	Findings    []FindingDecision `json:"findings"`
}

type RepairDecisionRequest struct {
	ID          string        `json:"id"`
	CandidateID string        `json:"candidate_id"`
	FindingIDs  []string      `json:"finding_ids"`
	Options     []RepairClass `json:"options"`
}

type ValidationProof struct {
	Kind        string           `json:"kind"`
	Result      ValidationResult `json:"result"`
	CompletedAt string           `json:"completed_at"`
}

type Candidate struct {
	ID                  string                               `json:"id"`
	BaseSHA             string                               `json:"base_sha"`
	CommitSHA           string                               `json:"commit_sha"`
	TreeSHA             string                               `json:"tree_sha"`
	RepairClass         RepairClass                          `json:"repair_class,omitempty"`
	ObservedFloor       deliveryevidence.DeliveryRiskProfile `json:"observed_floor"`
	Profile             deliveryevidence.DeliveryRiskProfile `json:"profile"`
	Effects             []EffectObservation                  `json:"effects"`
	Boundaries          []SensitiveBoundary                  `json:"boundaries"`
	RequiredReviews     []deliveryevidence.ReviewAxis        `json:"required_reviews"`
	Reviews             []CandidateReview                    `json:"reviews"`
	RequiredSpecialists []SensitiveBoundary                  `json:"required_specialists"`
	SpecialistReviews   []SpecialistReview                   `json:"specialist_reviews"`
	BoundaryProofs      []BoundaryProof                      `json:"boundary_proofs"`
	Acceptance          []AcceptanceProof                    `json:"acceptance,omitempty"`
	Focused             *ValidationProof                     `json:"focused,omitempty"`
	Exhaustive          *ValidationProof                     `json:"exhaustive,omitempty"`
	RepairDecision      *RepairDecision                      `json:"repair_decision,omitempty"`
}

type ProfileTransition struct {
	Sequence         int                                  `json:"sequence"`
	CandidateID      string                               `json:"candidate_id"`
	ObservedFloor    deliveryevidence.DeliveryRiskProfile `json:"observed_floor"`
	EffectiveProfile deliveryevidence.DeliveryRiskProfile `json:"effective_profile"`
	Boundaries       []SensitiveBoundary                  `json:"boundaries"`
	ObservedAt       string                               `json:"observed_at"`
}

type LocalReadiness struct {
	CandidateID   string `json:"candidate_id"`
	CommitSHA     string `json:"commit_sha"`
	TreeSHA       string `json:"tree_sha"`
	AuthorityHash string `json:"authority_sha256"`
	Branch        string `json:"branch"`
	ReadyAt       string `json:"ready_at"`
}

type NonLocalAuthorization struct {
	RunID        string `json:"run_id"`
	CandidateID  string `json:"candidate_id"`
	CommitSHA    string `json:"commit_sha"`
	TreeSHA      string `json:"tree_sha"`
	Branch       string `json:"branch"`
	LocalReadyAt string `json:"local_ready_at"`
}

type NonLocalObserveRequest struct {
	RunID       string
	Repository  deliveryevidence.RepositoryIdentity
	Issue       deliveryevidence.IssueIdentity
	CandidateID string
	Branch      string
	BaseRef     string
	HeadSHA     string
}

type RemoteBranchObservation struct {
	Name    string `json:"name"`
	HeadSHA string `json:"head_sha"`
}

type RemotePullRequestObservation struct {
	Number               int    `json:"number"`
	URL                  string `json:"url"`
	State                string `json:"state"`
	BaseRef              string `json:"base_ref"`
	BaseSHA              string `json:"base_sha"`
	HeadBranch           string `json:"head_branch"`
	HeadSHA              string `json:"head_sha"`
	HeadRepositoryNodeID string `json:"head_repository_node_id"`
	ClosingIssue         int    `json:"closing_issue"`
}

type NonLocalObservation struct {
	Branch       *RemoteBranchObservation
	PullRequests []RemotePullRequestObservation
	Checks       []CICheckObservation
	Merge        *MergeObservation
	OriginMain   *OriginMainObservation
}

type PushIssueBranchRequest struct {
	RunID       string
	Repository  deliveryevidence.RepositoryIdentity
	CandidateID string
	Branch      string
	HeadSHA     string
}

type EnsurePullRequestRequest struct {
	RunID          string
	Repository     deliveryevidence.RepositoryIdentity
	Issue          deliveryevidence.IssueIdentity
	CandidateID    string
	IdempotencyKey string
	BaseRef        string
	HeadBranch     string
	HeadSHA        string
	Title          string
	Body           string
}

type PullRequestIntent struct {
	IdempotencyKey string `json:"idempotency_key"`
	PreparedAt     string `json:"prepared_at"`
	DispatchedAt   string `json:"dispatched_at,omitempty"`
}

type RetryInfrastructureCheckRequest struct {
	RunID         string
	Repository    deliveryevidence.RepositoryIdentity
	PullRequest   int
	CandidateID   string
	HeadSHA       string
	CheckIdentity string
	FailedRunID   int64
}

type EnsureMergeRequest struct {
	RunID          string
	Repository     deliveryevidence.RepositoryIdentity
	Issue          deliveryevidence.IssueIdentity
	CandidateID    string
	IdempotencyKey string
	PullRequest    int
	HeadSHA        string
	BaseSHA        string
	Method         string
}

type DeleteRemoteIssueBranchRequest struct {
	RunID       string
	Repository  deliveryevidence.RepositoryIdentity
	CandidateID string
	Branch      string
	HeadSHA     string
}

type MergeObservation struct {
	PullRequest    int    `json:"pull_request"`
	URL            string `json:"url"`
	BaseRef        string `json:"base_ref"`
	HeadSHA        string `json:"head_sha"`
	MergeCommitSHA string `json:"merge_commit_sha"`
	MergedAt       string `json:"merged_at"`
}

type OriginMainObservation struct {
	HeadSHA               string `json:"head_sha"`
	MergeCommitSHA        string `json:"merge_commit_sha"`
	CandidateHeadSHA      string `json:"candidate_head_sha"`
	ContainsMergeCommit   bool   `json:"contains_merge_commit"`
	ContainsCandidateHead bool   `json:"contains_candidate_head"`
}

type MergeIntent struct {
	IdempotencyKey      string `json:"idempotency_key"`
	PullRequest         int    `json:"pull_request"`
	HeadSHA             string `json:"head_sha"`
	BaseSHA             string `json:"base_sha"`
	Method              string `json:"method"`
	OperatorStateSHA256 string `json:"operator_state_sha256"`
	PreparedAt          string `json:"prepared_at"`
	DispatchedAt        string `json:"dispatched_at,omitempty"`
}

type MergeProof struct {
	PullRequest    int    `json:"pull_request"`
	URL            string `json:"url"`
	HeadSHA        string `json:"head_sha"`
	MergeCommitSHA string `json:"merge_commit_sha"`
	MergedAt       string `json:"merged_at"`
	AdoptedAt      string `json:"adopted_at"`
}

type ManagedWorktreeObservation struct {
	Path        string `json:"path"`
	Branch      string `json:"branch"`
	HeadSHA     string `json:"head_sha"`
	RunID       string `json:"run_id"`
	CandidateID string `json:"candidate_id"`
	Clean       bool   `json:"clean"`
}

type LocalBranchObservation struct {
	Name    string `json:"name"`
	HeadSHA string `json:"head_sha"`
}

type IntegrationWorkspaceObservation struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Clean  bool   `json:"clean"`
}

type LocalMainRelation string

const (
	LocalMainSynced   LocalMainRelation = "synced"
	LocalMainBehind   LocalMainRelation = "behind"
	LocalMainAhead    LocalMainRelation = "ahead"
	LocalMainDiverged LocalMainRelation = "diverged"
)

type LocalMainObservation struct {
	Exists        bool              `json:"exists"`
	HeadSHA       string            `json:"head_sha"`
	OriginHeadSHA string            `json:"origin_head_sha"`
	Relation      LocalMainRelation `json:"relation"`
	Clean         bool              `json:"clean"`
}

type LocalCompletionObserveRequest struct {
	RunID          string
	CandidateID    string
	CommonDir      string
	RepositoryPath string
	IssueBranch    string
	CandidateHead  string
	MergeCommitSHA string
}

type LocalCompletionObservation struct {
	OperatorStateSHA256 string                          `json:"operator_state_sha256"`
	Integration         IntegrationWorkspaceObservation `json:"integration"`
	Worktrees           []ManagedWorktreeObservation    `json:"worktrees"`
	LocalBranch         *LocalBranchObservation         `json:"local_branch,omitempty"`
	LocalMain           LocalMainObservation            `json:"local_main"`
}

type RemoveManagedWorktreeRequest struct {
	RunID          string
	CandidateID    string
	CommonDir      string
	RepositoryPath string
	Path           string
	Branch         string
	HeadSHA        string
}

type DeleteLocalIssueBranchRequest struct {
	RunID          string
	CandidateID    string
	CommonDir      string
	RepositoryPath string
	Branch         string
	HeadSHA        string
}

type FastForwardLocalMainRequest struct {
	RunID          string
	CommonDir      string
	RepositoryPath string
	ExpectedOldSHA string
	OriginMainSHA  string
	MergeCommitSHA string
}

type CompletionReport struct {
	IssueClosed         bool                            `json:"issue_closed"`
	OriginMain          OriginMainObservation           `json:"origin_main"`
	RemoteBranchAbsent  bool                            `json:"remote_branch_absent"`
	WorktreesAbsent     bool                            `json:"worktrees_absent"`
	LocalBranchAbsent   bool                            `json:"local_branch_absent"`
	LocalMain           LocalMainObservation            `json:"local_main"`
	Integration         IntegrationWorkspaceObservation `json:"integration"`
	OperatorStateSHA256 string                          `json:"operator_state_sha256"`
	CompletedAt         string                          `json:"completed_at"`
}

type CIRetry struct {
	CheckIdentity string `json:"check_identity"`
	FailedRunID   int64  `json:"failed_run_id"`
	RetriedAt     string `json:"retried_at"`
}

type CandidateCIFailure struct {
	CheckIdentity string `json:"check_identity"`
	RunID         int64  `json:"run_id"`
	DetailsURL    string `json:"details_url"`
	ObservedAt    string `json:"observed_at"`
}

type NonLocalDelivery struct {
	Authorization     NonLocalAuthorization         `json:"authorization"`
	BaseRef           string                        `json:"base_ref"`
	Branch            *RemoteBranchObservation      `json:"branch,omitempty"`
	PullRequestIntent *PullRequestIntent            `json:"pull_request_intent,omitempty"`
	PullRequest       *RemotePullRequestObservation `json:"pull_request,omitempty"`
	Checks            []CICheckObservation          `json:"checks"`
	Retries           []CIRetry                     `json:"retries"`
	CandidateFailure  *CandidateCIFailure           `json:"candidate_failure,omitempty"`
	CIStatus          string                        `json:"ci_status,omitempty"`
	MergeIntent       *MergeIntent                  `json:"merge_intent,omitempty"`
	Merge             *MergeProof                   `json:"merge,omitempty"`
	Completion        *CompletionReport             `json:"completion,omitempty"`
}

type Config struct {
	Git             GitObserver
	GitHub          GitHubObserver
	Clock           Clock
	Review          ReviewExecutor
	Validation      ValidationExecutor
	Risk            CandidateRiskObserver
	Specialist      SpecialistReviewExecutor
	Boundary        BoundaryValidationExecutor
	NonLocal        NonLocalGateway
	LocalCompletion LocalCompletionGateway
	SandboxRoot     string
	DeclaredProfile deliveryevidence.DeliveryRiskProfile
	AllowLegacyV1   bool
}

type Module struct {
	git             GitObserver
	github          GitHubObserver
	clock           Clock
	review          ReviewExecutor
	validation      ValidationExecutor
	risk            CandidateRiskObserver
	specialist      SpecialistReviewExecutor
	boundary        BoundaryValidationExecutor
	nonlocal        NonLocalGateway
	localCompletion LocalCompletionGateway
	sandboxRoot     string
	declaredProfile deliveryevidence.DeliveryRiskProfile
	allowLegacyV1   bool
	store           fileRunStore
}

func New(config Config) (*Module, error) {
	if config.Git == nil || config.GitHub == nil {
		return nil, fmt.Errorf("Git and GitHub observers are required")
	}
	if config.Clock == nil {
		config.Clock = systemClock{}
	}
	if config.DeclaredProfile == "" {
		config.DeclaredProfile = deliveryevidence.RiskStandard
	}
	if config.DeclaredProfile != deliveryevidence.RiskLow &&
		config.DeclaredProfile != deliveryevidence.RiskStandard &&
		config.DeclaredProfile != deliveryevidence.RiskHigh {
		return nil, fmt.Errorf("declared delivery risk profile is invalid")
	}
	if (config.Review == nil) != (config.Validation == nil) {
		return nil, fmt.Errorf("review and validation executors must be configured together")
	}
	if config.Review != nil && config.Risk == nil {
		return nil, fmt.Errorf("configured assurance requires a candidate risk observer")
	}
	if (config.Specialist == nil) != (config.Boundary == nil) {
		return nil, fmt.Errorf("specialist review and boundary validation executors must be configured together")
	}
	if config.NonLocal != nil && config.Review == nil {
		return nil, fmt.Errorf("non-local delivery requires configured candidate assurance")
	}
	if config.LocalCompletion != nil && config.NonLocal == nil {
		return nil, fmt.Errorf("local completion requires configured non-local delivery")
	}
	if config.Review != nil &&
		(config.SandboxRoot == "" || !filepath.IsAbs(config.SandboxRoot) ||
			filepath.Clean(config.SandboxRoot) != config.SandboxRoot || config.SandboxRoot == string(filepath.Separator)) {
		return nil, fmt.Errorf("configured assurance requires an absolute canonical validation sandbox root")
	}
	if config.Review != nil {
		if err := validateSandboxRoot(config.SandboxRoot); err != nil {
			return nil, err
		}
	}
	return &Module{
		git: config.Git, github: config.GitHub, clock: config.Clock,
		review: config.Review, validation: config.Validation, sandboxRoot: config.SandboxRoot,
		risk: config.Risk, specialist: config.Specialist, boundary: config.Boundary,
		nonlocal: config.NonLocal, localCompletion: config.LocalCompletion,
		declaredProfile: config.DeclaredProfile, allowLegacyV1: config.AllowLegacyV1,
	}, nil
}

func validateSandboxRoot(root string) error {
	for _, path := range []string{root, filepath.Join(root, "home"), filepath.Join(root, "config")} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("validation sandbox path %q must be a real directory", path)
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || resolved != path {
			return fmt.Errorf("validation sandbox path %q must not traverse symlinks", path)
		}
	}
	return nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type runRecord struct {
	Schema                         string
	ID                             string
	Repository                     deliveryevidence.RepositoryIdentity
	Issue                          deliveryevidence.IssueIdentity
	AuthoritySHA256                string
	State                          State
	Reason                         string
	SupersedesRunID                string
	Evidence                       *deliveryevidence.Bundle
	PendingDecision                *DecisionRequest
	Decisions                      []Decision
	Observations                   Observations
	Candidates                     []Candidate
	PendingRepair                  *RepairDecisionRequest
	PendingQualificationCorrection *QualificationCorrectionRequest
	QualificationApproved          bool
	QualificationReviews           []QualificationReview
	QualificationCorrections       []QualificationCorrection
	LocalReadiness                 *LocalReadiness
	EffectiveProfile               deliveryevidence.DeliveryRiskProfile
	RequiredBoundaries             []SensitiveBoundary
	ProfileHistory                 []ProfileTransition
	NonLocal                       *NonLocalDelivery
	Timing                         []Timing
	CreatedAt                      string
	UpdatedAt                      string
}

type DecisionMismatchError struct {
	Expected string
	Got      string
}

func (e *DecisionMismatchError) Error() string {
	return fmt.Sprintf("delivery decision %q does not match pending request %q", e.Got, e.Expected)
}
