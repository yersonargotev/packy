package issuedelivery

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

const ciPolicyTestHead = "1111111111111111111111111111111111111111"
const ciPolicyTestBase = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var ciPolicyTestRepository = deliveryevidence.RepositoryIdentity{
	Owner: "yersonargotev", Name: "packy", NodeID: "R1",
}

func TestRequiredCIContextsAreTheFourUniversalADR0018Contexts(t *testing.T) {
	want := []string{
		"Claude 2.1.203 package smoke",
		"CodeQL",
		"Governance / Validate authorization",
		"Validate Packy-owned code",
	}
	if got := requiredCIContexts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("required CI contexts = %#v, want %#v", got, want)
	}
}

func TestEvaluateCIPolicyClassifiesExactHeadObservations(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func([]CICheckObservation) []CICheckObservation
		want        CIPolicyState
		wantClass   deliveryevidence.CheckClassification
		wantReasons bool
	}{
		{
			name: "all exact successes",
			mutate: func(in []CICheckObservation) []CICheckObservation {
				return in
			},
			want:      CISuccess,
			wantClass: deliveryevidence.CheckRequiredSuccess,
		},
		{
			name: "absent waits",
			mutate: func(in []CICheckObservation) []CICheckObservation {
				return in[1:]
			},
			want:      CIPending,
			wantClass: deliveryevidence.CheckAbsent,
		},
		{
			name: "pending waits",
			mutate: func(in []CICheckObservation) []CICheckObservation {
				in[0].Conclusion = ""
				return in
			},
			want:      CIPending,
			wantClass: deliveryevidence.CheckPending,
		},
		{
			name: "stale head waits",
			mutate: func(in []CICheckObservation) []CICheckObservation {
				in[0].HeadSHA = "2222222222222222222222222222222222222222"
				return in
			},
			want:      CIPending,
			wantClass: deliveryevidence.CheckStaleHead,
		},
		{
			name: "unavailable blocks",
			mutate: func(in []CICheckObservation) []CICheckObservation {
				in[0].Conclusion = "neutral"
				return in
			},
			want:        CIBlocked,
			wantClass:   deliveryevidence.CheckUnavailable,
			wantReasons: true,
		},
		{
			name: "duplicate conflicts block",
			mutate: func(in []CICheckObservation) []CICheckObservation {
				return append(in, in[0])
			},
			want:        CIBlocked,
			wantClass:   deliveryevidence.CheckConflict,
			wantReasons: true,
		},
		{
			name: "infrastructure failure is retryable",
			mutate: func(in []CICheckObservation) []CICheckObservation {
				in[0].Conclusion = "failure"
				in[0].FailureAttribution = FailureInfrastructure
				return in
			},
			want:      CIInfrastructureFailure,
			wantClass: deliveryevidence.CheckFailed,
		},
		{
			name: "candidate failure requires repair",
			mutate: func(in []CICheckObservation) []CICheckObservation {
				in[0].Conclusion = "failed"
				in[0].FailureAttribution = FailureCandidate
				return in
			},
			want:      CICandidateFailure,
			wantClass: deliveryevidence.CheckFailed,
		},
		{
			name: "cancelled unknown requires decision",
			mutate: func(in []CICheckObservation) []CICheckObservation {
				in[0].Conclusion = "cancelled"
				in[0].FailureAttribution = FailureUnknown
				return in
			},
			want:        CIDecisionRequired,
			wantClass:   deliveryevidence.CheckCancelled,
			wantReasons: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluateCIPolicy(
				ciPolicyTestRepository, ciPolicyTestHead, ciPolicyTestBase,
				tt.mutate(successfulCIChecks()),
			)
			if result.State != tt.want {
				t.Fatalf("state = %q, want %q; result=%#v", result.State, tt.want, result)
			}
			if !containsCIClassification(result.Checks, tt.wantClass) {
				t.Fatalf("checks = %#v, want classification %q", result.Checks, tt.wantClass)
			}
			if tt.wantReasons && len(result.Reasons) == 0 {
				t.Fatalf("result %#v has no explanatory reason", result)
			}
		})
	}
}

func TestEvaluateCIPolicyBlocksUntrustedOrUnsafePublisherEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CICheckObservation)
	}{
		{"wrong app", func(o *CICheckObservation) { o.Publisher.AppID = 1 }},
		{"wrong slug", func(o *CICheckObservation) { o.Publisher.Slug = "lookalike" }},
		{"HTTP URL", func(o *CICheckObservation) { o.DetailsURL = "http://github.com/run/1" }},
		{"foreign URL", func(o *CICheckObservation) { o.DetailsURL = "https://example.com/run/1" }},
		{"other GitHub repository", func(o *CICheckObservation) {
			o.DetailsURL = "https://github.com/other/fork/actions/runs/1"
		}},
		{"URL userinfo", func(o *CICheckObservation) { o.DetailsURL = "https://token@github.com/run/1" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks := successfulCIChecks()
			tt.mutate(&checks[0])
			result := evaluateCIPolicy(
				ciPolicyTestRepository, ciPolicyTestHead, ciPolicyTestBase, checks,
			)
			if result.State != CIBlocked || len(result.Reasons) == 0 {
				t.Fatalf("result = %#v, want explained blocked state", result)
			}
		})
	}
}

func TestEvaluateCIPolicyRequiresGovernanceCommitStatusProvenance(t *testing.T) {
	checks := successfulCIChecks()
	for index := range checks {
		if checks[index].Identity == requiredCIGovernance {
			checks[index].StatusKind = CIKindCheckRun
			checks[index].Publisher = TrustedPublisherEvidence{
				AppID: trustedGitHubActionsAppID, Slug: trustedGitHubActionsSlug,
			}
		}
	}
	result := evaluateCIPolicy(
		ciPolicyTestRepository, ciPolicyTestHead, ciPolicyTestBase, checks,
	)
	if result.State != CIBlocked || len(result.Reasons) == 0 {
		t.Fatalf("Governance check-run result = %#v, want blocked", result)
	}
}

func TestEvaluateCIPolicyBindsGovernanceDefinitionToExactPullRequestBase(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CIWorkflowEvidence)
	}{
		{
			name: "different valid base SHA",
			mutate: func(workflow *CIWorkflowEvidence) {
				workflow.DefinitionRef = strings.Repeat("b", 40)
			},
		},
		{
			name: "malformed definition blob SHA",
			mutate: func(workflow *CIWorkflowEvidence) {
				workflow.DefinitionSHA = "workflow-blob"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checks := successfulCIChecks()
			for index := range checks {
				if checks[index].Identity == requiredCIGovernance {
					tt.mutate(&checks[index].Workflow)
				}
			}
			result := evaluateCIPolicy(
				ciPolicyTestRepository, ciPolicyTestHead, ciPolicyTestBase, checks,
			)
			if result.State != CIBlocked || len(result.Reasons) == 0 {
				t.Fatalf("result=%#v, want blocked", result)
			}
		})
	}
}

func TestEvaluateCIPolicyCanonicalizesInputOrder(t *testing.T) {
	checks := successfulCIChecks()
	reversed := append([]CICheckObservation(nil), checks...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	if got, want := evaluateCIPolicy(
		ciPolicyTestRepository, ciPolicyTestHead, ciPolicyTestBase, reversed,
	), evaluateCIPolicy(
		ciPolicyTestRepository, ciPolicyTestHead, ciPolicyTestBase, checks,
	); !reflect.DeepEqual(got, want) {
		t.Fatalf("reordered result = %#v, want %#v", got, want)
	}
}

func successfulCIChecks() []CICheckObservation {
	contexts := requiredCIContexts()
	checks := make([]CICheckObservation, 0, len(contexts))
	for index, identity := range contexts {
		runID := int64(index + 1)
		check := CICheckObservation{
			RequiredCheck: deliveryevidence.RequiredCheck{
				Identity:   identity,
				Conclusion: "success",
				HeadSHA:    ciPolicyTestHead,
			},
			StatusKind: CIKindCheckRun,
			Publisher: TrustedPublisherEvidence{
				AppID: 15368,
				Slug:  "github-actions",
			},
			Workflow: CIWorkflowEvidence{
				Name: "CI", Path: ".github/workflows/ci.yml", RunID: runID,
				DefinitionRef: ciPolicyTestHead, DefinitionSHA: strings.Repeat("f", 40),
			},
			RunID:      runID,
			DetailsURL: "https://github.com/yersonargotev/packy/actions/runs/" + string(rune('1'+index)),
		}
		switch identity {
		case requiredCICodeQL:
			check.Workflow.Name = "Security"
			check.Workflow.Path = ".github/workflows/security-pr.yml"
		case requiredCIGovernance:
			check.StatusKind = CIKindCommitStatus
			check.Publisher = TrustedPublisherEvidence{
				Login: "github-actions[bot]", ID: 41898282, Type: "Bot",
				HTMLURL: "https://github.com/apps/github-actions",
			}
			check.Source = TrustedPublisherEvidence{
				AppID: trustedGitHubActionsAppID, Slug: trustedGitHubActionsSlug,
			}
			check.Workflow.Name = "Governance"
			check.Workflow.Path = ".github/workflows/governance.yml"
			check.Workflow.DefinitionRef = ciPolicyTestBase
		}
		checks = append(checks, check)
	}
	return checks
}

func containsCIClassification(checks []deliveryevidence.RequiredCheck, classification deliveryevidence.CheckClassification) bool {
	for _, check := range checks {
		if check.Classification == classification {
			return true
		}
	}
	return false
}
