package issuedelivery

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

const (
	requiredCIValidate   = "Validate Packy-owned code"
	requiredCIClaude     = "Claude 2.1.203 package smoke"
	requiredCIGovernance = "Governance / Validate authorization"
	requiredCICodeQL     = "CodeQL"

	trustedGitHubActionsAppID = int64(15368)
	trustedGitHubActionsSlug  = "github-actions"
)

type CIFailureAttribution string

const (
	FailureInfrastructure CIFailureAttribution = "infrastructure"
	FailureCandidate      CIFailureAttribution = "candidate"
	FailureUnknown        CIFailureAttribution = "unknown"
)

type TrustedPublisherEvidence struct {
	AppID   int64  `json:"app_id,omitempty"`
	Slug    string `json:"slug,omitempty"`
	Login   string `json:"login,omitempty"`
	ID      int64  `json:"id,omitempty"`
	Type    string `json:"type,omitempty"`
	HTMLURL string `json:"html_url,omitempty"`
}

type CIStatusKind string

const (
	CIKindCheckRun     CIStatusKind = "check-run"
	CIKindCommitStatus CIStatusKind = "commit-status"
)

type CIWorkflowEvidence struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	RunID         int64  `json:"run_id"`
	DefinitionRef string `json:"definition_ref"`
	DefinitionSHA string `json:"definition_sha"`
}

type CICheckObservation struct {
	deliveryevidence.RequiredCheck
	StatusKind         CIStatusKind             `json:"status_kind"`
	Publisher          TrustedPublisherEvidence `json:"publisher"`
	Source             TrustedPublisherEvidence `json:"source,omitempty"`
	Workflow           CIWorkflowEvidence       `json:"workflow"`
	RunID              int64                    `json:"run_id"`
	DetailsURL         string                   `json:"details_url"`
	FailureAttribution CIFailureAttribution     `json:"failure_attribution,omitempty"`
}

type CIPolicyState string

const (
	CIPending               CIPolicyState = "pending"
	CISuccess               CIPolicyState = "success"
	CIInfrastructureFailure CIPolicyState = "infrastructure-failure"
	CICandidateFailure      CIPolicyState = "candidate-failure"
	CIDecisionRequired      CIPolicyState = "decision-required"
	CIBlocked               CIPolicyState = "blocked"
)

type CIPolicyResult struct {
	State   CIPolicyState                    `json:"state"`
	Checks  []deliveryevidence.RequiredCheck `json:"checks"`
	Reasons []string                         `json:"reasons,omitempty"`
}

func requiredCIContexts() []string {
	return []string{requiredCIClaude, requiredCICodeQL, requiredCIGovernance, requiredCIValidate}
}

func evaluateCIPolicy(
	repository deliveryevidence.RepositoryIdentity,
	head, base string,
	observations []CICheckObservation,
) CIPolicyResult {
	canonical := append([]CICheckObservation(nil), observations...)
	sort.Slice(canonical, func(i, j int) bool {
		left, right := canonical[i], canonical[j]
		if left.Identity != right.Identity {
			return left.Identity < right.Identity
		}
		if left.HeadSHA != right.HeadSHA {
			return left.HeadSHA < right.HeadSHA
		}
		if left.Conclusion != right.Conclusion {
			return left.Conclusion < right.Conclusion
		}
		if left.RunID != right.RunID {
			return left.RunID < right.RunID
		}
		return left.DetailsURL < right.DetailsURL
	})

	required := requiredCIContexts()
	allowed := make(map[string]bool, len(required))
	for _, identity := range required {
		allowed[identity] = true
	}
	var raw []deliveryevidence.RequiredCheck
	attribution := make(map[string]CIFailureAttribution, len(canonical))
	var reasons []string
	if !validCIHead(head) {
		reasons = append(reasons, "exact CI head identity is malformed")
	}
	if !validCIHead(base) {
		reasons = append(reasons, "exact pull-request base identity is malformed")
	}
	for _, observation := range canonical {
		raw = append(raw, observation.RequiredCheck)
		if !allowed[observation.Identity] {
			reasons = append(reasons, fmt.Sprintf("foreign required check %q", observation.Identity))
		}
		if observation.RunID <= 0 {
			reasons = append(reasons, fmt.Sprintf("required check %q has no workflow run identity", observation.Identity))
		}
		if !trustedGitHubRunURL(observation.DetailsURL, repository, observation.RunID) {
			reasons = append(reasons, fmt.Sprintf("required check %q has an unsafe details URL", observation.Identity))
		}
		if reason := validateCIProvenance(head, base, observation); reason != "" {
			reasons = append(reasons, reason)
		}
		attribution[observation.Identity] = observation.FailureAttribution
	}

	checks := deliveryevidence.ClassifyRequiredChecks(required, nil, raw, head)
	result := CIPolicyResult{Checks: checks}
	pending, infrastructure, candidate, decision := false, false, false, false
	blocked := len(reasons) > 0
	for _, check := range checks {
		switch check.Classification {
		case deliveryevidence.CheckRequiredSuccess:
		case deliveryevidence.CheckAbsent, deliveryevidence.CheckPending, deliveryevidence.CheckStaleHead:
			pending = true
		case deliveryevidence.CheckConflict, deliveryevidence.CheckUnavailable:
			blocked = true
			reasons = append(reasons, fmt.Sprintf("required check %q is %s", check.Identity, check.Classification))
		case deliveryevidence.CheckFailed, deliveryevidence.CheckCancelled:
			switch attribution[check.Identity] {
			case FailureInfrastructure:
				infrastructure = true
			case FailureCandidate:
				candidate = true
			default:
				decision = true
				reasons = append(reasons, fmt.Sprintf("required check %q failure attribution is unknown", check.Identity))
			}
		default:
			blocked = true
			reasons = append(reasons, fmt.Sprintf("required check %q has unsupported classification %q", check.Identity, check.Classification))
		}
	}
	result.Reasons = canonicalStrings(reasons)
	switch {
	case blocked:
		result.State = CIBlocked
	case decision:
		result.State = CIDecisionRequired
	case candidate:
		result.State = CICandidateFailure
	case infrastructure:
		result.State = CIInfrastructureFailure
	case pending:
		result.State = CIPending
	default:
		result.State = CISuccess
	}
	return result
}

func validCIHead(value string) bool {
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

func trustedGitHubRunURL(
	value string,
	repository deliveryevidence.RepositoryIdentity,
	runID int64,
) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() != "github.com" ||
		parsed.Port() != "" || parsed.User != nil || runID <= 0 {
		return false
	}
	needle := "/" + repository.Owner + "/" + repository.Name +
		"/actions/runs/" + strconv.FormatInt(runID, 10)
	tail := parsed.Path
	if tail == needle {
		return true
	}
	jobID := strings.TrimPrefix(tail, needle+"/job/")
	if jobID == tail || jobID == "" {
		return false
	}
	for _, character := range jobID {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validateCIProvenance(head, base string, observation CICheckObservation) string {
	workflowName, workflowPath := "", ""
	switch observation.Identity {
	case requiredCIValidate, requiredCIClaude:
		workflowName, workflowPath = "CI", ".github/workflows/ci.yml"
	case requiredCICodeQL:
		workflowName, workflowPath = "Security", ".github/workflows/security-pr.yml"
	case requiredCIGovernance:
		workflowName, workflowPath = "Governance", ".github/workflows/governance.yml"
	default:
		return fmt.Sprintf("required check %q has no trusted provenance policy", observation.Identity)
	}
	workflow := observation.Workflow
	if workflow.Name != workflowName || workflow.Path != workflowPath ||
		workflow.RunID != observation.RunID || !validCIHead(workflow.DefinitionSHA) {
		return fmt.Sprintf("required check %q has incompatible workflow provenance", observation.Identity)
	}
	if observation.Identity == requiredCIGovernance {
		if observation.StatusKind != CIKindCommitStatus ||
			observation.Publisher.Login != "github-actions[bot]" ||
			observation.Publisher.ID != 41898282 ||
			observation.Publisher.Type != "Bot" ||
			observation.Publisher.HTMLURL != "https://github.com/apps/github-actions" ||
			observation.Source.AppID != trustedGitHubActionsAppID ||
			observation.Source.Slug != trustedGitHubActionsSlug ||
			workflow.DefinitionRef != base {
			return fmt.Sprintf("required check %q is not the trusted Governance commit status", observation.Identity)
		}
		return ""
	}
	if observation.StatusKind != CIKindCheckRun ||
		observation.Publisher.AppID != trustedGitHubActionsAppID ||
		observation.Publisher.Slug != trustedGitHubActionsSlug ||
		workflow.DefinitionRef != head {
		return fmt.Sprintf("required check %q is not the trusted Actions check-run", observation.Identity)
	}
	return ""
}

func canonicalStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}
