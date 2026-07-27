package deliveryevidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type CheckClassification string

const (
	CheckRequiredSuccess CheckClassification = "required-success"
	CheckExpectedSkip    CheckClassification = "expected-skip"
	CheckPending         CheckClassification = "pending"
	CheckAbsent          CheckClassification = "absent"
	CheckFailed          CheckClassification = "failed"
	CheckCancelled       CheckClassification = "cancelled"
	CheckConflict        CheckClassification = "duplicate-or-conflicting-identity"
	CheckStaleHead       CheckClassification = "stale-head"
	CheckUnavailable     CheckClassification = "unavailable"
)

type LifecyclePhase string

const (
	PhaseQualification  LifecyclePhase = "qualification"
	PhaseImplementation LifecyclePhase = "implementation"
	PhaseReview         LifecyclePhase = "review"
	PhaseValidation     LifecyclePhase = "validation"
	PhaseCI             LifecyclePhase = "ci"
	PhaseMerge          LifecyclePhase = "merge"
	PhaseCleanup        LifecyclePhase = "cleanup"
)

var lifecyclePhases = []LifecyclePhase{PhaseQualification, PhaseImplementation, PhaseReview, PhaseValidation, PhaseCI, PhaseMerge, PhaseCleanup}

type RequiredCheck struct {
	Identity       string              `json:"identity"`
	Conclusion     string              `json:"conclusion,omitempty"`
	HeadSHA        string              `json:"head_sha,omitempty"`
	Classification CheckClassification `json:"classification"`
}

type PullRequestObservation struct {
	Repository    RepositoryIdentity `json:"repository"`
	Issue         IssueIdentity      `json:"issue"`
	Spec          SpecIdentity       `json:"spec"`
	IssueSHA256   string             `json:"issue_sha256"`
	SpecSHA256    string             `json:"spec_sha256"`
	IssueEligible bool               `json:"issue_eligible"`
	SpecEligible  bool               `json:"spec_eligible"`
	Branch        string             `json:"branch"`
	Number        int                `json:"number"`
	URL           string             `json:"url"`
	HeadSHA       string             `json:"head_sha"`
	BaseSHA       string             `json:"base_sha"`
	Mergeability  string             `json:"mergeability"`
	Required      []RequiredCheck    `json:"required_checks"`
	ObservedAt    string             `json:"observed_at"`
	Available     bool               `json:"available"`
}

type ReadinessCode string

const (
	ReadinessReady          ReadinessCode = "ready"
	ReadinessLocalGate      ReadinessCode = "local-gate-required"
	ReadinessForeign        ReadinessCode = "foreign-evidence"
	ReadinessStaleAuthority ReadinessCode = "stale-authority"
	ReadinessStaleHead      ReadinessCode = "stale-head"
	ReadinessNotMergeable   ReadinessCode = "not-mergeable"
	ReadinessChecks         ReadinessCode = "required-checks-not-ready"
	ReadinessUnavailable    ReadinessCode = "unavailable"
)

type ReadinessReport struct {
	Code            ReadinessCode      `json:"code"`
	Repository      RepositoryIdentity `json:"repository"`
	Issue           IssueIdentity      `json:"issue"`
	Spec            SpecIdentity       `json:"spec"`
	Branch          string             `json:"branch"`
	PullRequest     int                `json:"pull_request"`
	URL             string             `json:"url"`
	HeadSHA         string             `json:"head_sha"`
	BaseSHA         string             `json:"base_sha"`
	Mergeability    string             `json:"mergeability"`
	RequiredChecks  []RequiredCheck    `json:"required_checks"`
	ObservedAt      string             `json:"observed_at"`
	LocalBundleHash string             `json:"local_bundle_sha256"`
	Ready           bool               `json:"ready"`
}

type ReadinessError struct {
	Code   ReadinessCode
	Detail string
	Report ReadinessReport
}

func (e *ReadinessError) Error() string {
	return fmt.Sprintf("non-local readiness %s: %s", e.Code, e.Detail)
}

func ClassifyRequiredChecks(expected []string, expectedSkips map[string]bool, observed []RequiredCheck, prHead string) []RequiredCheck {
	byID := map[string][]RequiredCheck{}
	for _, item := range observed {
		byID[item.Identity] = append(byID[item.Identity], item)
	}
	out := make([]RequiredCheck, 0, len(expected))
	for _, identity := range expected {
		items := byID[identity]
		item := RequiredCheck{Identity: identity, Classification: CheckAbsent}
		if len(items) == 1 {
			item = items[0]
			switch {
			case (item.Conclusion == "success" || (expectedSkips[item.Identity] && item.Conclusion == "skipped")) && item.HeadSHA == "":
				item.Classification = CheckUnavailable
			case item.HeadSHA != "" && item.HeadSHA != prHead:
				item.Classification = CheckStaleHead
			case item.Conclusion == "":
				item.Classification = CheckPending
			case expectedSkips[item.Identity] && item.Conclusion == "skipped":
				item.Classification = CheckExpectedSkip
			case item.Conclusion == "success":
				item.Classification = CheckRequiredSuccess
			case item.Conclusion == "cancelled":
				item.Classification = CheckCancelled
			case item.Conclusion == "failure" || item.Conclusion == "failed":
				item.Classification = CheckFailed
			default:
				item.Classification = CheckUnavailable
			}
		} else if len(items) > 1 {
			item = items[0]
			item.Classification = CheckConflict
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Identity < out[j].Identity })
	return out
}

func validateHTTPS(name, value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("%s must be a credential-free HTTPS URL", name)
	}
	return safeText(name, value)
}

func validateExpectedChecks(expected, expectedSkips []string, observed []RequiredCheck) (map[string]bool, error) {
	if len(expected) == 0 {
		return nil, errors.New("at least one required check identity is required")
	}
	allowed := map[string]bool{}
	for _, id := range expected {
		if id == "" || strings.TrimSpace(id) != id || allowed[id] || safeText("required check identity", id) != nil {
			return nil, fmt.Errorf("invalid or duplicate required check identity %q", id)
		}
		allowed[id] = true
	}
	skips := map[string]bool{}
	for _, id := range expectedSkips {
		if !allowed[id] || skips[id] {
			return nil, fmt.Errorf("invalid, duplicate, or non-required expected skip identity %q", id)
		}
		skips[id] = true
	}
	for _, item := range observed {
		if !allowed[item.Identity] {
			return nil, fmt.Errorf("foreign or empty observed check identity %q", item.Identity)
		}
		if item.Conclusion != "" && safeText("required check conclusion", item.Conclusion) != nil {
			return nil, fmt.Errorf("invalid conclusion for required check %q", item.Identity)
		}
		if item.HeadSHA != "" && !gitSHA(item.HeadSHA) {
			return nil, fmt.Errorf("required check %q has malformed head identity", item.Identity)
		}
	}
	return skips, nil
}

func EvaluateReadiness(bundle Bundle, local LocalGateReport, observation PullRequestObservation, expectedChecks, expectedSkips []string) (ReadinessReport, error) {
	report := ReadinessReport{Repository: observation.Repository, Issue: observation.Issue, Spec: observation.Spec, Branch: observation.Branch, PullRequest: observation.Number, URL: observation.URL, HeadSHA: observation.HeadSHA, BaseSHA: observation.BaseSHA, Mergeability: observation.Mergeability, ObservedAt: observation.ObservedAt, LocalBundleHash: local.BundleSHA256}
	fail := func(code ReadinessCode, detail string) (ReadinessReport, error) {
		report.Code = code
		return report, &ReadinessError{Code: code, Detail: detail, Report: report}
	}
	if local.Repository != bundle.Repository || local.Issue != bundle.Issue || local.Spec != bundle.Spec || local.Branch == "" ||
		!gitSHA(local.StartingBaseSHA) || local.StartingBaseSHA != bundle.StartingBaseSHA || !gitSHA(local.HeadSHA) || !gitSHA(local.TreeSHA) ||
		local.AcceptanceProved != len(bundle.AcceptanceMatrix) || !digest(local.BundleSHA256) {
		return fail(ReadinessLocalGate, "successful LOCAL gate report is incomplete or does not seal the current delivery facts")
	}
	if len(bundle.Iterations) == 0 || local.HeadSHA != bundle.Iterations[len(bundle.Iterations)-1].HeadSHA {
		return fail(ReadinessLocalGate, "LOCAL gate head does not equal the final recorded Bundle iteration")
	}
	if _, err := parseTimestamp("LOCAL validation completion", local.ValidationCompletedAt); err != nil {
		return fail(ReadinessLocalGate, err.Error())
	}
	currentDigest, err := Digest(bundle)
	if err != nil || currentDigest != local.BundleSHA256 {
		return fail(ReadinessStaleAuthority, "LOCAL gate does not seal the current issue and specification evidence")
	}
	if observation.Repository != local.Repository || observation.Issue != local.Issue || observation.Spec != local.Spec || observation.Branch != local.Branch {
		return fail(ReadinessForeign, "pull request identity does not match the LOCAL gate")
	}
	if !observation.IssueEligible || !observation.SpecEligible || observation.IssueSHA256 != bundle.Authority.IssueSHA256 || observation.SpecSHA256 != bundle.Authority.SpecSHA256 {
		return fail(ReadinessStaleAuthority, "re-observed issue or specification authority changed")
	}
	if !observation.Available || observation.Number <= 0 || !gitSHA(observation.HeadSHA) || !gitSHA(observation.BaseSHA) {
		return fail(ReadinessUnavailable, "pull request facts are incomplete or unavailable")
	}
	if observation.BaseSHA != local.StartingBaseSHA {
		return fail(ReadinessStaleHead, "pull request base changed after the successful LOCAL gate")
	}
	if safeText("pull request branch", observation.Branch) != nil || safeText("pull request mergeability", observation.Mergeability) != nil {
		return fail(ReadinessUnavailable, "pull request branch or mergeability is unsafe")
	}
	if _, err := parseTimestamp("pull request observation time", observation.ObservedAt); err != nil {
		return fail(ReadinessUnavailable, err.Error())
	}
	if err := validateHTTPS("pull request URL", observation.URL); err != nil {
		return fail(ReadinessUnavailable, err.Error())
	}
	skips, err := validateExpectedChecks(expectedChecks, expectedSkips, observation.Required)
	if err != nil {
		return fail(ReadinessChecks, err.Error())
	}
	if observation.HeadSHA != local.HeadSHA {
		return fail(ReadinessStaleHead, "pull request head does not equal the successful LOCAL gate head")
	}
	if observation.Mergeability != "mergeable" {
		return fail(ReadinessNotMergeable, "pull request is not known mergeable")
	}
	report.RequiredChecks = ClassifyRequiredChecks(expectedChecks, skips, observation.Required, observation.HeadSHA)
	for _, check := range report.RequiredChecks {
		if check.Classification != CheckRequiredSuccess && check.Classification != CheckExpectedSkip {
			return fail(ReadinessChecks, fmt.Sprintf("required check %q is %s", check.Identity, check.Classification))
		}
	}
	report.Code, report.Ready = ReadinessReady, true
	return report, nil
}

type FinalOutcomeObservation struct {
	Repository           RepositoryIdentity `json:"repository"`
	Issue                IssueIdentity      `json:"issue"`
	PullRequest          int                `json:"pull_request"`
	PullRequestURL       string             `json:"pull_request_url"`
	PullRequestHeadSHA   string             `json:"pull_request_head_sha"`
	MergeCommitSHA       string             `json:"merge_commit_sha"`
	Merged               bool               `json:"merged"`
	MergeContainedOnMain bool               `json:"merge_contained_on_origin_main"`
	IssueClosed          bool               `json:"issue_closed"`
	RemoteBranchAbsent   bool               `json:"remote_branch_absent"`
	LocalMainSHA         string             `json:"local_main_sha"`
	OriginMainSHA        string             `json:"origin_main_sha"`
	LocalBranchAbsent    bool               `json:"local_branch_absent"`
	WorktreeClean        bool               `json:"worktree_clean"`
	PreservedStateBefore string             `json:"preserved_state_before_sha256"`
	PreservedStateAfter  string             `json:"preserved_state_after_sha256"`
	ObservedAt           string             `json:"observed_at"`
	MatrixURL            string             `json:"matrix_url"`
	ReviewsURL           string             `json:"reviews_url"`
	ValidationURL        string             `json:"validation_url"`
	CIURL                string             `json:"ci_url"`
	CleanupURL           string             `json:"cleanup_url"`
}

type OutcomeCode string

const (
	OutcomeSucceeded      OutcomeCode = "succeeded"
	OutcomeNotMerged      OutcomeCode = "merge-unproved"
	OutcomeNotContained   OutcomeCode = "merge-not-contained"
	OutcomeIssueOpen      OutcomeCode = "issue-open"
	OutcomeRemoteBranch   OutcomeCode = "remote-branch-present"
	OutcomeUnsynchronized OutcomeCode = "local-main-unsynchronized"
	OutcomeLocalBranch    OutcomeCode = "local-branch-present"
	OutcomeDirty          OutcomeCode = "dirty-worktree"
	OutcomeStateChanged   OutcomeCode = "preserved-state-changed"
	OutcomeUnavailable    OutcomeCode = "unavailable"
)

type PhaseReceipt struct {
	Phase        LifecyclePhase `json:"phase"`
	StartedAt    string         `json:"started_at"`
	CompletedAt  string         `json:"completed_at"`
	FindingCount int            `json:"finding_count,omitempty"`
}
type OutcomeTelemetry struct {
	PhaseDurationsSeconds   map[string]int64 `json:"phase_durations_seconds"`
	Iterations              int              `json:"iterations"`
	StandardsFindings       int              `json:"standards_findings"`
	SpecFindings            int              `json:"spec_findings"`
	LocalValidationFindings int              `json:"local_validation_findings"`
	CIFindings              int              `json:"ci_findings"`
}
type FinalOutcome struct {
	Code        OutcomeCode             `json:"code"`
	Observation FinalOutcomeObservation `json:"observation"`
	Telemetry   OutcomeTelemetry        `json:"telemetry"`
	Successful  bool                    `json:"successful"`
}
type OutcomeError struct {
	Code    OutcomeCode
	Detail  string
	Outcome FinalOutcome
}

func (e *OutcomeError) Error() string {
	return fmt.Sprintf("final delivery outcome %s: %s", e.Code, e.Detail)
}

func DeriveTelemetry(bundle Bundle, receipts []PhaseReceipt) (OutcomeTelemetry, error) {
	t := OutcomeTelemetry{PhaseDurationsSeconds: map[string]int64{}, Iterations: len(bundle.Iterations)}
	required := map[LifecyclePhase]bool{}
	for _, phase := range lifecyclePhases {
		required[phase] = true
	}
	for _, review := range bundle.ReviewReceipts {
		if review.Axis == ReviewStandards {
			t.StandardsFindings += len(review.Findings)
		}
		if review.Axis == ReviewSpec {
			t.SpecFindings += len(review.Findings)
		}
	}
	for _, receipt := range receipts {
		if !required[receipt.Phase] || receipt.FindingCount < 0 || safeText("telemetry phase", string(receipt.Phase)) != nil {
			return t, errors.New("lifecycle phase identity is invalid")
		}
		start, startErr := parseTimestamp(fmt.Sprintf("%s phase start", receipt.Phase), receipt.StartedAt)
		end, endErr := parseTimestamp(fmt.Sprintf("%s phase completion", receipt.Phase), receipt.CompletedAt)
		if startErr != nil {
			return t, startErr
		}
		if endErr != nil {
			return t, endErr
		}
		if end.Before(start) {
			return t, fmt.Errorf("telemetry receipt %q has invalid time ordering", receipt.Phase)
		}
		if _, exists := t.PhaseDurationsSeconds[string(receipt.Phase)]; exists {
			return t, fmt.Errorf("duplicate telemetry phase %q", receipt.Phase)
		}
		t.PhaseDurationsSeconds[string(receipt.Phase)] = int64(end.Sub(start) / time.Second)
		switch receipt.Phase {
		case PhaseValidation:
			t.LocalValidationFindings = receipt.FindingCount
		case PhaseCI:
			t.CIFindings = receipt.FindingCount
		}
	}
	for phase := range required {
		if _, exists := t.PhaseDurationsSeconds[string(phase)]; !exists {
			return t, fmt.Errorf("lifecycle phase %q is required", phase)
		}
	}
	return t, nil
}

func EvaluateFinalOutcome(bundle Bundle, readiness ReadinessReport, observation FinalOutcomeObservation, receipts []PhaseReceipt) (FinalOutcome, error) {
	telemetry, err := DeriveTelemetry(bundle, receipts)
	out := FinalOutcome{Observation: observation, Telemetry: telemetry}
	fail := func(code OutcomeCode, detail string) (FinalOutcome, error) {
		out.Code = code
		return out, &OutcomeError{Code: code, Detail: detail, Outcome: out}
	}
	currentDigest, digestErr := Digest(bundle)
	if digestErr != nil || !readiness.Ready || readiness.Code != ReadinessReady ||
		readiness.Repository != bundle.Repository || readiness.Issue != bundle.Issue ||
		readiness.LocalBundleHash != currentDigest ||
		readiness.PullRequest != observation.PullRequest || readiness.URL != observation.PullRequestURL ||
		readiness.HeadSHA != observation.PullRequestHeadSHA {
		return fail(OutcomeUnavailable, "final outcome is not bound to a successful exact-head readiness report")
	}
	if err != nil || observation.Repository != bundle.Repository || observation.Issue != bundle.Issue || observation.PullRequest <= 0 || observation.ObservedAt == "" {
		return fail(OutcomeUnavailable, "final evidence is incomplete or telemetry is invalid")
	}
	if _, err := parseTimestamp("final observation time", observation.ObservedAt); err != nil {
		return fail(OutcomeUnavailable, err.Error())
	}
	for name, value := range map[string]string{"pull request URL": observation.PullRequestURL, "matrix URL": observation.MatrixURL, "reviews URL": observation.ReviewsURL, "validation URL": observation.ValidationURL, "CI URL": observation.CIURL, "cleanup URL": observation.CleanupURL} {
		if err := validateHTTPS(name, value); err != nil {
			return fail(OutcomeUnavailable, err.Error())
		}
	}
	if !gitSHA(observation.PullRequestHeadSHA) || !gitSHA(observation.MergeCommitSHA) || !gitSHA(observation.LocalMainSHA) || !gitSHA(observation.OriginMainSHA) ||
		!digest(observation.PreservedStateBefore) || !digest(observation.PreservedStateAfter) {
		return fail(OutcomeUnavailable, "final Git or preservation identities are malformed")
	}
	if !observation.Merged {
		return fail(OutcomeNotMerged, "merge is not proved")
	}
	if !observation.MergeContainedOnMain {
		return fail(OutcomeNotContained, "merge commit is not contained on origin/main")
	}
	if !observation.IssueClosed {
		return fail(OutcomeIssueOpen, "issue remains open")
	}
	if !observation.RemoteBranchAbsent {
		return fail(OutcomeRemoteBranch, "remote delivery branch remains")
	}
	if observation.LocalMainSHA == "" || observation.LocalMainSHA != observation.OriginMainSHA {
		return fail(OutcomeUnsynchronized, "local main is not synchronized with origin/main")
	}
	if !observation.LocalBranchAbsent {
		return fail(OutcomeLocalBranch, "local delivery branch remains")
	}
	if !observation.WorktreeClean {
		return fail(OutcomeDirty, "local worktree is not clean")
	}
	if observation.PreservedStateBefore == "" || observation.PreservedStateBefore != observation.PreservedStateAfter {
		return fail(OutcomeStateChanged, "declared operator state was not preserved")
	}
	out.Code, out.Successful = OutcomeSucceeded, true
	return out, nil
}

func RenderSuccessBrief(out FinalOutcome) (string, error) {
	if !out.Successful || out.Code != OutcomeSucceeded {
		return "", errors.New("success brief requires a successful final outcome")
	}
	o := out.Observation
	links := []struct{ name, url string }{{"Issue", fmt.Sprintf("https://github.com/%s/%s/issues/%d", o.Repository.Owner, o.Repository.Name, o.Issue.Number)}, {"Pull request", o.PullRequestURL}, {"Merge", fmt.Sprintf("https://github.com/%s/%s/commit/%s", o.Repository.Owner, o.Repository.Name, o.MergeCommitSHA)}, {"Matrix", o.MatrixURL}, {"Reviews", o.ReviewsURL}, {"Validation", o.ValidationURL}, {"CI", o.CIURL}, {"Cleanup", o.CleanupURL}}
	var b strings.Builder
	b.WriteString("## Delivery succeeded\n\n")
	for _, link := range links {
		if link.url == "" {
			return "", fmt.Errorf("success brief link %s is unavailable", link.name)
		}
		fmt.Fprintf(&b, "- [%s](%s)\n", link.name, link.url)
	}
	return b.String(), nil
}

func CanonicalReport(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
