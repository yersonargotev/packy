package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
	"github.com/yersonargotev/packy/internal/issuedelivery"
)

type remoteRunnerCall struct {
	name string
	args []string
}

type fakeRemoteRunner struct {
	outputs [][]byte
	errs    []error
	calls   []remoteRunnerCall
}

func (runner *fakeRemoteRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.calls = append(runner.calls, remoteRunnerCall{name: name, args: append([]string(nil), args...)})
	index := len(runner.calls) - 1
	if index >= len(runner.outputs) {
		return nil, errors.New("unexpected command")
	}
	var err error
	if index < len(runner.errs) {
		err = runner.errs[index]
	}
	return runner.outputs[index], err
}

func TestProductionNonLocalPushObservesThenUsesExactRefspec(t *testing.T) {
	head := strings.Repeat("b", 40)
	runner := &fakeRemoteRunner{outputs: [][]byte{
		[]byte(`{"id":"R1","nameWithOwner":"yersonargotev/packy"}`),
		nil,
		nil,
	}}
	gateway := productionNonLocalGateway{runner: runner}
	err := gateway.PushIssueBranch(context.Background(), issuedelivery.PushIssueBranchRequest{
		Repository: packyRemoteRepository(), Branch: "chore/issue-361-remote-adapter", HeadSHA: head,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []remoteRunnerCall{
		{name: "gh", args: []string{"repo", "view", "yersonargotev/packy", "--json", "id,nameWithOwner"}},
		{name: "git", args: []string{"ls-remote", "--refs", "origin", "refs/heads/chore/issue-361-remote-adapter"}},
		{name: "git", args: []string{"push", "origin", head + ":refs/heads/chore/issue-361-remote-adapter"}},
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("commands differ\n got: %#v\nwant: %#v", runner.calls, want)
	}
}

func TestProductionNonLocalObservationRefreshesAndBindsOriginMain(t *testing.T) {
	head, main := strings.Repeat("b", 40), strings.Repeat("a", 40)
	runner := &fakeRemoteRunner{outputs: [][]byte{
		[]byte(`{"id":"R1","nameWithOwner":"yersonargotev/packy"}`),
		nil,
		[]byte(head + "\trefs/heads/chore/issue-361-remote-adapter\n" +
			main + "\trefs/heads/main\n"),
		[]byte(main + "\n"),
		[]byte(`[]`),
	}}
	gateway := productionNonLocalGateway{runner: runner}
	observation, err := gateway.ObserveNonLocal(
		context.Background(),
		issuedelivery.NonLocalObserveRequest{
			Repository: packyRemoteRepository(),
			Issue:      deliveryevidence.IssueIdentity{Number: 361, NodeID: "I361"},
			Branch:     "chore/issue-361-remote-adapter", BaseRef: "main", HeadSHA: head,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Branch == nil || observation.Branch.HeadSHA != head {
		t.Fatalf("remote observation = %#v", observation)
	}
	if got := runner.calls[1]; got.name != "git" || !reflect.DeepEqual(got.args, []string{
		"fetch", "--prune", "--no-tags", "origin",
		"refs/heads/main:refs/remotes/origin/main",
	}) {
		t.Fatalf("origin/main refresh = %#v", got)
	}
}

func TestProductionNonLocalChecksProjectGitHubResponsesBeforeStrictDecode(t *testing.T) {
	head, base := strings.Repeat("b", 40), strings.Repeat("a", 40)
	runner := &fakeRemoteRunner{outputs: [][]byte{
		[]byte(`{"check_runs":[{"name":"Validate Packy-owned code","head_sha":"` + head + `","status":"completed","conclusion":"success","details_url":"https://github.com/yersonargotev/packy/actions/runs/42/job/7","app":{"id":15368,"slug":"github-actions"}}]}`),
		[]byte(`[]`),
		[]byte(`{"id":42,"name":"CI","path":".github/workflows/ci.yml","head_sha":"` + head + `","html_url":"https://github.com/yersonargotev/packy/actions/runs/42","actor":{"login":"maintainer","id":1,"type":"User","html_url":"https://github.com/maintainer"}}`),
		[]byte(head),
	}}
	gateway := productionNonLocalGateway{runner: runner}
	checks, err := gateway.observeChecks(context.Background(), issuedelivery.NonLocalObserveRequest{
		Repository: packyRemoteRepository(), HeadSHA: head,
	}, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].Identity != "Validate Packy-owned code" {
		t.Fatalf("checks = %#v", checks)
	}
	if got := runner.calls[0]; got.name != "gh" || !reflect.DeepEqual(got.args, []string{
		"api", "-H", "Accept: application/vnd.github+json",
		"repos/yersonargotev/packy/commits/" + head + "/check-runs?filter=latest",
		"--jq", checkRunsProjection,
	}) {
		t.Fatalf("check-runs command = %#v", got)
	}
	if got := runner.calls[1]; got.name != "gh" || !reflect.DeepEqual(got.args, []string{
		"api", "-H", "Accept: application/vnd.github+json",
		"repos/yersonargotev/packy/commits/" + head + "/statuses",
		"--jq", statusesProjection,
	}) {
		t.Fatalf("statuses command = %#v", got)
	}
	if got := runner.calls[2]; got.name != "gh" || !reflect.DeepEqual(got.args, []string{
		"api", "-H", "Accept: application/vnd.github+json",
		"repos/yersonargotev/packy/actions/runs/42",
		"--jq", workflowRunProjection,
	}) {
		t.Fatalf("workflow command = %#v", got)
	}
}

func TestProductionNonLocalMergeUsesMatchHeadCommit(t *testing.T) {
	head, base := strings.Repeat("b", 40), strings.Repeat("a", 40)
	runner := &fakeRemoteRunner{outputs: [][]byte{
		[]byte(`{"id":"R1","nameWithOwner":"yersonargotev/packy"}`),
		[]byte(`{"number":17,"state":"OPEN","baseRefOid":"` + base + `","headRefOid":"` + head + `","closingIssuesReferences":[{"number":361,"id":"I1"}],"mergedAt":""}`),
		nil,
	}}
	gateway := productionNonLocalGateway{runner: runner}
	err := gateway.EnsureMerge(context.Background(), issuedelivery.EnsureMergeRequest{
		Repository: packyRemoteRepository(), Issue: deliveryevidence.IssueIdentity{Number: 361, NodeID: "I1"},
		PullRequest: 17, HeadSHA: head, BaseSHA: base, Method: "merge",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := runner.calls[2]
	want := remoteRunnerCall{name: "gh", args: []string{
		"pr", "merge", "17", "--repo", "yersonargotev/packy", "--merge",
		"--match-head-commit", head,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge command = %#v, want %#v", got, want)
	}
}

func TestProductionNonLocalCleanupRejectsMovedBranchWithoutDeletion(t *testing.T) {
	head, moved := strings.Repeat("b", 40), strings.Repeat("c", 40)
	runner := &fakeRemoteRunner{outputs: [][]byte{
		[]byte(`{"id":"R1","nameWithOwner":"yersonargotev/packy"}`),
		[]byte(moved + "\trefs/heads/fix/issue-361-adapter\n"),
	}}
	gateway := productionNonLocalGateway{runner: runner}
	err := gateway.EnsureRemoteIssueBranchAbsent(context.Background(), issuedelivery.DeleteRemoteIssueBranchRequest{
		Repository: packyRemoteRepository(), Branch: "fix/issue-361-adapter", HeadSHA: head,
	})
	if err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("unexpected mutation after incompatible observation: %#v", runner.calls)
	}
}

func TestProductionNonLocalCleanupUsesExactLease(t *testing.T) {
	head := strings.Repeat("b", 40)
	runner := &fakeRemoteRunner{outputs: [][]byte{
		[]byte(`{"id":"R1","nameWithOwner":"yersonargotev/packy"}`),
		[]byte(head + "\trefs/heads/fix/issue-361-adapter\n"),
		nil,
	}}
	gateway := productionNonLocalGateway{runner: runner}
	err := gateway.EnsureRemoteIssueBranchAbsent(
		context.Background(),
		issuedelivery.DeleteRemoteIssueBranchRequest{
			Repository: packyRemoteRepository(),
			Branch:     "fix/issue-361-adapter", HeadSHA: head,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := runner.calls[2]
	wantLease := "--force-with-lease=refs/heads/fix/issue-361-adapter:" + head
	if got.name != "git" || !reflect.DeepEqual(got.args, []string{
		"push", wantLease, "origin", ":refs/heads/fix/issue-361-adapter",
	}) {
		t.Fatalf("leased cleanup command = %#v", got)
	}
}

func TestProductionNonLocalCleanupRejectsConcurrentBranchMoveAtLease(t *testing.T) {
	head := strings.Repeat("b", 40)
	runner := &fakeRemoteRunner{
		outputs: [][]byte{
			[]byte(`{"id":"R1","nameWithOwner":"yersonargotev/packy"}`),
			[]byte(head + "\trefs/heads/fix/issue-361-adapter\n"),
			nil,
		},
		errs: []error{nil, nil, errors.New("stale info")},
	}
	err := (productionNonLocalGateway{runner: runner}).EnsureRemoteIssueBranchAbsent(
		context.Background(),
		issuedelivery.DeleteRemoteIssueBranchRequest{
			Repository: packyRemoteRepository(),
			Branch:     "fix/issue-361-adapter", HeadSHA: head,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "delete exact remote") {
		t.Fatalf("concurrent remote branch movement was not rejected: %v", err)
	}
}

func TestProductionNonLocalRejectsForeignRepositoryBeforeMutation(t *testing.T) {
	runner := &fakeRemoteRunner{}
	gateway := productionNonLocalGateway{runner: runner}
	err := gateway.PushIssueBranch(context.Background(), issuedelivery.PushIssueBranchRequest{
		Repository: deliveryevidence.RepositoryIdentity{Owner: "other", Name: "packy", NodeID: "R1"},
		Branch:     "chore/issue-361-adapter", HeadSHA: strings.Repeat("b", 40),
	})
	if err == nil || len(runner.calls) != 0 {
		t.Fatalf("foreign identity was not rejected before commands: err=%v calls=%#v", err, runner.calls)
	}
}

func TestTrustedCheckIdentityRejectsIncompatibleWorkflow(t *testing.T) {
	if trustedCheckIdentity("CodeQL", "CI", ".github/workflows/ci.yml") {
		t.Fatal("CodeQL must not accept CI workflow provenance")
	}
	if trustedCheckIdentity("foreign", "Security", ".github/workflows/security-pr.yml") {
		t.Fatal("foreign required check accepted")
	}
}

func TestApplyCIFailureAttributionRequiresExactFailedRun(t *testing.T) {
	head := strings.Repeat("b", 40)
	checks := []issuedelivery.CICheckObservation{{
		RequiredCheck: deliveryevidence.RequiredCheck{
			Identity: "Validate Packy-owned code", Conclusion: "failure", HeadSHA: head,
		},
		RunID: 42, DetailsURL: "https://github.com/yersonargotev/packy/actions/runs/42",
	}}
	decision := advanceCIFailureAttribution{
		CheckIdentity: checks[0].Identity, RunID: checks[0].RunID,
		HeadSHA: head, DetailsURL: checks[0].DetailsURL,
		Attribution: issuedelivery.FailureCandidate,
	}
	got, err := applyCIFailureAttributions(checks, []advanceCIFailureAttribution{decision})
	if err != nil || got[0].FailureAttribution != issuedelivery.FailureCandidate {
		t.Fatalf("exact attribution = %#v, err=%v", got, err)
	}
	decision.RunID++
	if _, err = applyCIFailureAttributions(checks, []advanceCIFailureAttribution{decision}); err == nil {
		t.Fatal("foreign failed run attribution was accepted")
	}
}

func packyRemoteRepository() deliveryevidence.RepositoryIdentity {
	return deliveryevidence.RepositoryIdentity{Owner: "yersonargotev", Name: "packy", NodeID: "R1"}
}
