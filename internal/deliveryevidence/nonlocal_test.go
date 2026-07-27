package deliveryevidence

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyRequiredChecksFailClosed(t *testing.T) {
	head := strings.Repeat("a", 40)
	cases := []struct {
		name     string
		observed []RequiredCheck
		want     CheckClassification
	}{
		{"success", []RequiredCheck{{Identity: "ci", Conclusion: "success", HeadSHA: head}}, CheckRequiredSuccess},
		{"expected skip", []RequiredCheck{{Identity: "ci", Conclusion: "skipped", HeadSHA: head}}, CheckExpectedSkip},
		{"pending", []RequiredCheck{{Identity: "ci", HeadSHA: head}}, CheckPending},
		{"absent", nil, CheckAbsent},
		{"failed", []RequiredCheck{{Identity: "ci", Conclusion: "failure", HeadSHA: head}}, CheckFailed},
		{"cancelled", []RequiredCheck{{Identity: "ci", Conclusion: "cancelled", HeadSHA: head}}, CheckCancelled},
		{"conflict", []RequiredCheck{{Identity: "ci", Conclusion: "success"}, {Identity: "ci", Conclusion: "failure"}}, CheckConflict},
		{"stale", []RequiredCheck{{Identity: "ci", Conclusion: "success", HeadSHA: strings.Repeat("b", 40)}}, CheckStaleHead},
		{"unavailable", []RequiredCheck{{Identity: "ci", Conclusion: "mystery", HeadSHA: head}}, CheckUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectedSkips := map[string]bool{}
			if tc.name == "expected skip" {
				expectedSkips["ci"] = true
			}
			got := ClassifyRequiredChecks([]string{"ci"}, expectedSkips, tc.observed, head)
			if len(got) != 1 || got[0].Classification != tc.want {
				t.Fatalf("got %+v, want %s", got, tc.want)
			}
		})
	}
}

func TestReadinessBindsPassingExactLocalGate(t *testing.T) {
	bundle := reviewFixture()
	head := bundle.Iterations[len(bundle.Iterations)-1].HeadSHA
	digest, err := Digest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	local := LocalGateReport{Repository: bundle.Repository, Issue: bundle.Issue, Spec: bundle.Spec, Branch: "feat/issue-42-ready", StartingBaseSHA: bundle.StartingBaseSHA, HeadSHA: head, TreeSHA: strings.Repeat("f", 40), AcceptanceProved: len(bundle.AcceptanceMatrix), ValidationCompletedAt: "2026-07-27T11:59:00Z", BundleSHA256: digest}
	observation := PullRequestObservation{Repository: bundle.Repository, Issue: bundle.Issue, Spec: bundle.Spec, IssueSHA256: bundle.Authority.IssueSHA256, SpecSHA256: bundle.Authority.SpecSHA256, IssueEligible: true, SpecEligible: true, Branch: local.Branch, Number: 7, URL: "https://github.com/o/r/pull/7", HeadSHA: head, BaseSHA: bundle.StartingBaseSHA, Mergeability: "mergeable", Available: true, ObservedAt: "2026-07-27T12:00:00Z", Required: []RequiredCheck{{Identity: "validate", Conclusion: "success", HeadSHA: head}}}
	report, err := EvaluateReadiness(bundle, local, observation, []string{"validate"}, nil)
	if err != nil || !report.Ready {
		t.Fatalf("not ready: %+v %v", report, err)
	}
	for name, mutate := range map[string]func(*PullRequestObservation){"foreign branch": func(o *PullRequestObservation) { o.Branch = "feat/issue-42-other" }, "stale head": func(o *PullRequestObservation) { o.HeadSHA = strings.Repeat("c", 40) }, "pending": func(o *PullRequestObservation) { o.Required[0].Conclusion = "" }} {
		t.Run(name, func(t *testing.T) {
			bad := observation
			bad.Required = append([]RequiredCheck(nil), observation.Required...)
			mutate(&bad)
			if _, err := EvaluateReadiness(bundle, local, bad, []string{"validate"}, nil); err == nil {
				t.Fatal("failure passed")
			}
		})
	}
	changed := bundle
	changed.Authority.IssueSHA256 = strings.Repeat("f", 64)
	if _, err := EvaluateReadiness(changed, local, observation, []string{"validate"}, nil); err == nil {
		t.Fatal("changed authority passed")
	}
}

func TestFinalOutcomeTelemetryAndBrief(t *testing.T) {
	bundle := reviewFixture()
	digest, _ := Digest(bundle)
	head := strings.Repeat("c", 40)
	readiness := ReadinessReport{Code: ReadinessReady, Repository: bundle.Repository, Issue: bundle.Issue, Spec: bundle.Spec, PullRequest: 7, URL: "https://github.com/o/r/pull/7", HeadSHA: head, LocalBundleHash: digest, Ready: true}
	o := FinalOutcomeObservation{Repository: bundle.Repository, Issue: bundle.Issue, PullRequest: 7, PullRequestURL: readiness.URL, PullRequestHeadSHA: head, MergeCommitSHA: strings.Repeat("d", 40), Merged: true, MergeContainedOnMain: true, IssueClosed: true, RemoteBranchAbsent: true, LocalMainSHA: strings.Repeat("e", 40), OriginMainSHA: strings.Repeat("e", 40), LocalBranchAbsent: true, WorktreeClean: true, PreservedStateBefore: strings.Repeat("1", 64), PreservedStateAfter: strings.Repeat("1", 64), ObservedAt: "2026-07-27T12:00:00Z", MatrixURL: "https://e/matrix", ReviewsURL: "https://e/reviews", ValidationURL: "https://e/validation", CIURL: "https://e/ci", CleanupURL: "https://e/cleanup"}
	phases := []string{"qualification", "implementation", "review", "validation", "ci", "merge", "cleanup"}
	var receipts []PhaseReceipt
	for _, phase := range phases {
		receipts = append(receipts, PhaseReceipt{Phase: phase, StartedAt: "2026-07-27T11:58:00Z", CompletedAt: "2026-07-27T12:00:00Z"})
	}
	receipts[3].FindingCount, receipts[4].FindingCount = 3, 4
	out, err := EvaluateFinalOutcome(bundle, readiness, o, receipts)
	if err != nil || out.Telemetry.StandardsFindings != 0 || out.Telemetry.SpecFindings != 0 || out.Telemetry.LocalValidationFindings != 3 || out.Telemetry.CIFindings != 4 {
		t.Fatalf("%+v %v", out, err)
	}
	brief, err := RenderSuccessBrief(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Issue", "Pull request", "Merge", "Matrix", "Reviews", "Validation", "CI", "Cleanup"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("brief misses %s", want)
		}
	}
	failures := []struct {
		code   OutcomeCode
		mutate func(*FinalOutcomeObservation)
	}{{OutcomeNotContained, func(x *FinalOutcomeObservation) { x.MergeContainedOnMain = false }}, {OutcomeIssueOpen, func(x *FinalOutcomeObservation) { x.IssueClosed = false }}, {OutcomeRemoteBranch, func(x *FinalOutcomeObservation) { x.RemoteBranchAbsent = false }}, {OutcomeLocalBranch, func(x *FinalOutcomeObservation) { x.LocalBranchAbsent = false }}, {OutcomeDirty, func(x *FinalOutcomeObservation) { x.WorktreeClean = false }}, {OutcomeStateChanged, func(x *FinalOutcomeObservation) { x.PreservedStateAfter = strings.Repeat("2", 64) }}}
	for _, tc := range failures {
		bad := o
		tc.mutate(&bad)
		_, err := EvaluateFinalOutcome(bundle, readiness, bad, receipts)
		var oe *OutcomeError
		if !errors.As(err, &oe) || oe.Code != tc.code {
			t.Fatalf("want %s got %v", tc.code, err)
		}
	}
	for name, bad := range map[string]ReadinessReport{"absent": {}, "stale": func() ReadinessReport { x := readiness; x.LocalBundleHash = strings.Repeat("9", 64); return x }(), "foreign": func() ReadinessReport {
		x := readiness
		x.Issue = IssueIdentity{Number: 999, NodeID: "foreign"}
		return x
	}()} {
		if _, err := EvaluateFinalOutcome(bundle, bad, o, receipts); err == nil {
			t.Fatalf("%s readiness passed", name)
		}
	}
	if _, err := DeriveTelemetry(bundle, receipts[:6]); err == nil {
		t.Fatal("missing cleanup phase passed")
	}
	unsafe := o
	unsafe.CIURL = "https://token@example.com/ci"
	if _, err := EvaluateFinalOutcome(bundle, readiness, unsafe, receipts); err == nil {
		t.Fatal("credential-bearing URL passed")
	}
}
