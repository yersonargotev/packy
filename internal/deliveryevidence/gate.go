package deliveryevidence

import (
	"errors"
	"fmt"
	"strings"
)

// LocalGateObservation is a caller-provided, read-only snapshot of the facts
// needed to decide whether a qualified issue is ready to leave the local loop.
// It contains identities and digests, never tracker bodies or checkout paths.
type LocalGateObservation struct {
	Repository     RepositoryIdentity
	Issue          IssueIdentity
	Spec           SpecIdentity
	IssueSHA256    string
	SpecSHA256     string
	IssueEligible  bool
	SpecEligible   bool
	ExpectedBranch string
	CurrentBranch  string
	HeadSHA        string
	TreeSHA        string
	OrderedCommits []string
	Validation     ValidationObservation
	ObservedAt     string
}

type LocalGateErrorCode string

const (
	LocalGateForeignEvidence         LocalGateErrorCode = "foreign-evidence"
	LocalGateTrackerAuthorityChanged LocalGateErrorCode = "tracker-authority-changed"
	LocalGateQualificationInvalid    LocalGateErrorCode = "qualification-incomplete"
	LocalGateAcceptanceUnproved      LocalGateErrorCode = "acceptance-incomplete"
	LocalGateReviewGap               LocalGateErrorCode = "review-gap"
	LocalGateUnresolvedFindings      LocalGateErrorCode = "unresolved-findings"
	LocalGateStaleValidation         LocalGateErrorCode = "stale-validation"
	LocalGateDirtyWorkspace          LocalGateErrorCode = "dirty-workspace"
	LocalGateWrongBranch             LocalGateErrorCode = "wrong-branch"
	LocalGateUnrecordedDelta         LocalGateErrorCode = "unrecorded-delta"
)

// LocalGateError is stable for automation: callers should branch on Code and
// treat Detail as explanatory text only.
type LocalGateError struct {
	Code   LocalGateErrorCode
	Detail string
	Report LocalGateReport
}

func (e *LocalGateError) Error() string {
	return fmt.Sprintf("local delivery gate %s: %s", e.Code, e.Detail)
}

// NewLocalGateFailure preserves every currently available fact for a
// deterministic exception report before the complete evaluator can run.
func NewLocalGateFailure(bundle Bundle, observation LocalGateObservation, code LocalGateErrorCode, detail string) *LocalGateError {
	return &LocalGateError{Code: code, Detail: detail, Report: localGateReport(bundle, observation)}
}

// LocalGateReport is emitted only for a passing observation.
type LocalGateReport struct {
	Repository            RepositoryIdentity
	Issue                 IssueIdentity
	Spec                  SpecIdentity
	IssueNumber           int
	Branch                string
	StartingBaseSHA       string
	HeadSHA               string
	TreeSHA               string
	Owned                 int
	Deferred              int
	Forbidden             int
	Prerequisites         int
	AcceptanceProved      int
	Iterations            int
	Reviews               int
	Adjudications         int
	ReviewedCommits       int
	ValidationCompletedAt string
	BundleSHA256          string
}

func gateFailure(report LocalGateReport, code LocalGateErrorCode, format string, args ...any) (LocalGateReport, error) {
	return LocalGateReport{}, &LocalGateError{Code: code, Detail: fmt.Sprintf(format, args...), Report: report}
}

// EvaluateLocalGate composes the bundle, review, and exhaustive-validation
// authorities. It performs no I/O and never changes the supplied bundle.
func EvaluateLocalGate(bundle Bundle, observation LocalGateObservation) (LocalGateReport, error) {
	report := localGateReport(bundle, observation)
	if observation.Repository != bundle.Repository || observation.Issue != bundle.Issue || observation.Spec != bundle.Spec {
		return gateFailure(report, LocalGateForeignEvidence, "observation identities do not belong to the bundle")
	}
	if !observation.IssueEligible || !observation.SpecEligible {
		return gateFailure(report, LocalGateTrackerAuthorityChanged, "issue or specification is no longer eligible")
	}
	if observation.IssueSHA256 != bundle.Authority.IssueSHA256 || observation.SpecSHA256 != bundle.Authority.SpecSHA256 {
		return gateFailure(report, LocalGateTrackerAuthorityChanged, "qualified tracker authority has changed")
	}
	if err := Validate(bundle); err != nil {
		return gateFailure(report, LocalGateQualificationInvalid, "%v", err)
	}
	var unproved []string
	for _, row := range bundle.AcceptanceMatrix {
		if row.State != AcceptanceProved {
			unproved = append(unproved, row.Identity)
		}
	}
	if len(unproved) > 0 {
		return gateFailure(report, LocalGateAcceptanceUnproved, "acceptance rows are not proved: %s", strings.Join(unproved, ", "))
	}
	if len(bundle.Iterations) == 0 {
		return gateFailure(report, LocalGateUnrecordedDelta, "at least one contiguous iteration is required")
	}
	if !deliveryBranchForIssue(observation.ExpectedBranch, bundle.Issue.Number) {
		return gateFailure(report, LocalGateWrongBranch, "expected branch %q is not an issue #%d delivery branch", observation.ExpectedBranch, bundle.Issue.Number)
	}
	if observation.CurrentBranch != observation.ExpectedBranch {
		return gateFailure(report, LocalGateWrongBranch, "expected branch %q, observed %q", observation.ExpectedBranch, observation.CurrentBranch)
	}
	if !observation.Validation.WorkspaceClean {
		return gateFailure(report, LocalGateDirtyWorkspace, "workspace is not clean")
	}
	last := bundle.Iterations[len(bundle.Iterations)-1]
	if last.HeadSHA != observation.HeadSHA {
		return gateFailure(report, LocalGateUnrecordedDelta, "final iteration head %s does not equal current head %s", last.HeadSHA, observation.HeadSHA)
	}
	status, err := QueryReviews(bundle, observation.HeadSHA, observation.OrderedCommits)
	if err != nil {
		return gateFailure(report, LocalGateUnrecordedDelta, "%v", err)
	}
	if len(status.UnreviewedDeltas) > 0 {
		return gateFailure(report, LocalGateReviewGap, "iterations lack both review axes: %s", strings.Join(status.UnreviewedDeltas, ", "))
	}
	if len(status.UncoveredCommits) > 0 {
		return gateFailure(report, LocalGateUnrecordedDelta, "commits lack recorded iteration coverage: %s", strings.Join(status.UncoveredCommits, ", "))
	}
	if len(status.UnresolvedFindings) > 0 {
		return gateFailure(report, LocalGateUnresolvedFindings, "findings remain unresolved: %s", strings.Join(status.UnresolvedFindings, ", "))
	}
	if observation.Validation.CommitSHA != observation.HeadSHA || observation.Validation.TreeSHA != observation.TreeSHA {
		return gateFailure(report, LocalGateStaleValidation, "current validation does not seal the observed head and tree")
	}
	receipt, err := ReusableExhaustiveValidation(bundle, observation.Validation, observation.ObservedAt)
	if err != nil {
		return gateFailure(report, LocalGateStaleValidation, "%v", err)
	}
	if report.BundleSHA256 == "" {
		return gateFailure(report, LocalGateQualificationInvalid, "canonical bundle digest is unavailable")
	}
	report.ValidationCompletedAt = receipt.CompletedAt
	return report, nil
}

func localGateReport(bundle Bundle, observation LocalGateObservation) LocalGateReport {
	digest, _ := Digest(bundle)
	proved := 0
	for _, row := range bundle.AcceptanceMatrix {
		if row.State == AcceptanceProved {
			proved++
		}
	}
	reviewedCommits := len(observation.OrderedCommits) - 1
	if reviewedCommits < 0 {
		reviewedCommits = 0
	}
	return LocalGateReport{Repository: bundle.Repository, Issue: bundle.Issue, Spec: bundle.Spec, IssueNumber: bundle.Issue.Number, Branch: observation.CurrentBranch, StartingBaseSHA: bundle.StartingBaseSHA, HeadSHA: observation.HeadSHA, TreeSHA: observation.TreeSHA, Owned: len(bundle.Scope.OwnedNow), Deferred: len(bundle.Scope.Deferred), Forbidden: len(bundle.Scope.Forbidden), Prerequisites: len(bundle.Scope.Prerequisites), AcceptanceProved: proved, Iterations: len(bundle.Iterations), Reviews: len(bundle.ReviewReceipts), Adjudications: len(bundle.Adjudications), ReviewedCommits: reviewedCommits, BundleSHA256: digest}
}

func deliveryBranchForIssue(branch string, issue int) bool {
	for _, kind := range []string{"feat", "fix", "chore"} {
		prefix := fmt.Sprintf("%s/issue-%d-", kind, issue)
		suffix := strings.TrimPrefix(branch, prefix)
		if suffix == branch || suffix == "" || strings.HasPrefix(suffix, "-") || strings.HasSuffix(suffix, "-") {
			continue
		}
		valid := true
		for _, r := range suffix {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				valid = false
				break
			}
		}
		if valid {
			return true
		}
	}
	return false
}

// RenderLocalGateReport renders a deterministic success report.
func RenderLocalGateReport(report LocalGateReport) string {
	return "LOCAL delivery gate: PASS\n" + renderLocalGateEvidence(report)
}

// RenderLocalGateFailureReport renders deterministic evidence for an exception
// brief without changing or replacing the canonical bundle.
func RenderLocalGateFailureReport(err error) string {
	var gateErr *LocalGateError
	if !errors.As(err, &gateErr) {
		return ""
	}
	out := fmt.Sprintf("LOCAL delivery gate: FAIL\nCode: %s\nDetail: %s\n", gateErr.Code, gateErr.Detail)
	if gateErr.Report.Issue.Number > 0 {
		out += renderLocalGateEvidence(gateErr.Report)
	}
	return out
}

func renderLocalGateEvidence(report LocalGateReport) string {
	return fmt.Sprintf("Repository: %s/%s (%s)\nIssue: #%d (%s)\nSpec: #%d (%s)\nBranch: %s\nStarting base: %s\nHead: %s\nTree: %s\nScope: owned=%d deferred=%d forbidden=%d prerequisites=%d\nAcceptance proved: %d\nIterations: %d\nReview receipts: %d\nAdjudications: %d\nReviewed commits: %d\nExhaustive validation completed: %s\nBundle SHA-256: %s\n", report.Repository.Owner, report.Repository.Name, report.Repository.NodeID, report.Issue.Number, report.Issue.NodeID, report.Spec.Number, report.Spec.NodeID, report.Branch, report.StartingBaseSHA, report.HeadSHA, report.TreeSHA, report.Owned, report.Deferred, report.Forbidden, report.Prerequisites, report.AcceptanceProved, report.Iterations, report.Reviews, report.Adjudications, report.ReviewedCommits, report.ValidationCompletedAt, report.BundleSHA256)
}

// IsLocalGateError reports whether err has the requested stable gate code.
func IsLocalGateError(err error, code LocalGateErrorCode) bool {
	var target *LocalGateError
	return errors.As(err, &target) && target.Code == code
}
