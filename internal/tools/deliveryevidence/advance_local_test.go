package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/issuedelivery"
)

type localRunnerCall struct {
	name string
	args []string
}

type fakeLocalRunner struct {
	outputs [][]byte
	errors  []error
	calls   []localRunnerCall
}

func (f *fakeLocalRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, localRunnerCall{name: name, args: append([]string{}, args...)})
	if len(f.outputs) == 0 {
		return nil, errors.New("unexpected command")
	}
	output, err := f.outputs[0], f.errors[0]
	f.outputs, f.errors = f.outputs[1:], f.errors[1:]
	return output, err
}

func TestProductionGitObserverObservesExactPackyIdentity(t *testing.T) {
	runner := &fakeLocalRunner{
		outputs: [][]byte{
			[]byte("/repo/.git\n"), []byte("/repo\n"),
			[]byte("git@github.com:yersonargotev/packy.git\n"),
			[]byte(strings.Repeat("a", 40) + "\n"), []byte(strings.Repeat("b", 40) + "\n"),
			[]byte(strings.Repeat("c", 40) + "\n"), nil, []byte("feature/361\n"),
		},
		errors: make([]error, 8),
	}
	got, err := (productionGitObserver{runner: runner}).ObserveGit(context.Background(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner != "yersonargotev" || got.Name != "packy" || got.Worktree != "/repo" ||
		got.CommonDir != "/repo/.git" || !got.WorkspaceClean || got.Branch != "feature/361" {
		t.Fatalf("unexpected observation: %#v", got)
	}
}

func TestProductionCandidateRiskObserverClassifiesConservatively(t *testing.T) {
	head, tree := strings.Repeat("b", 40), strings.Repeat("c", 40)
	runner := &fakeLocalRunner{
		outputs: [][]byte{
			[]byte(head + "\n"), []byte(tree + "\n"),
			[]byte("M\x00docs/readme.md\x00M\x00internal/widget/widget.go\x00M\x00.github/workflows/release.yml\x00"),
		},
		errors: make([]error, 3),
	}
	got, err := (productionCandidateRiskObserver{runner: runner}).ObserveCandidateRisk(
		context.Background(),
		issuedelivery.CandidateRiskRequest{
			CandidateID: "candidate", RepositoryPath: "/repo", StartingBaseSHA: strings.Repeat("a", 40),
			CommitSHA: head, TreeSHA: tree,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantEffects := []issuedelivery.CandidateEffect{
		issuedelivery.EffectOrdinaryBehavior, issuedelivery.EffectPassive, issuedelivery.EffectPublication,
	}
	var effects []issuedelivery.CandidateEffect
	for _, effect := range got.Effects {
		effects = append(effects, effect.Effect)
	}
	if !got.Completed || !reflect.DeepEqual(effects, wantEffects) {
		t.Fatalf("unexpected risk observation: %#v", got)
	}
}

func TestProductionLocalCompletionOmitsUnprovedWorktreeOwnership(t *testing.T) {
	head, origin := strings.Repeat("a", 40), strings.Repeat("b", 40)
	runner := &fakeLocalRunner{
		outputs: [][]byte{
			[]byte("/repo/.git\n"), []byte("/repo\n"), []byte("main\n"), nil,
			[]byte("worktree /repo\x00HEAD " + head + "\x00branch refs/heads/main\x00\x00" +
				"worktree /tmp/candidate\x00HEAD " + head + "\x00branch refs/heads/issue/361\x00\x00"),
			nil, nil,
			[]byte(head + "\n"), []byte(origin + "\n"), []byte(head + "\n"), nil,
			[]byte("main\n"), nil, []byte(head + "\n"),
		},
		errors: []error{
			nil, nil, nil, nil, nil,
			errors.New("unset"), errors.New("unset"),
			nil, nil, nil, nil, nil, nil, nil,
		},
	}
	got, err := (productionLocalCompletionGateway{runner: runner}).ObserveLocalCompletion(
		context.Background(),
		issuedelivery.LocalCompletionObserveRequest{
			RunID: "run", CandidateID: "candidate", CommonDir: "/repo/.git",
			RepositoryPath: "/repo", IssueBranch: "issue/361", CandidateHead: head,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Worktrees == nil || len(got.Worktrees) != 0 {
		t.Fatalf("unproved worktree ownership was inferred: %#v", got.Worktrees)
	}
}

func TestProductionLocalCompletionUsesOnlySafeCleanupCommands(t *testing.T) {
	runner := &fakeLocalRunner{
		outputs: [][]byte{
			[]byte("old\n"), nil, []byte("topic\n"), nil,
			[]byte("old\n"), []byte("new\n"), nil, nil, []byte("topic\n"),
			[]byte("worktree /repo\x00HEAD topic\x00branch refs/heads/topic\x00\x00"), nil,
		},
		errors: make([]error, 11),
	}
	gateway := productionLocalCompletionGateway{runner: runner}
	if err := gateway.EnsureLocalIssueBranchAbsent(context.Background(), issuedelivery.DeleteLocalIssueBranchRequest{
		RepositoryPath: "/repo", Branch: "issue/361", HeadSHA: "old",
	}); err != nil {
		t.Fatal(err)
	}
	if err := gateway.EnsureLocalMainFastForward(context.Background(), issuedelivery.FastForwardLocalMainRequest{
		RepositoryPath: "/repo", ExpectedOldSHA: "old", OriginMainSHA: "new", MergeCommitSHA: "merge",
	}); err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, call := range runner.calls {
		joined += call.name + " " + strings.Join(call.args, " ") + "\n"
	}
	for _, forbidden := range []string{"checkout", "reset", "stash", "--force"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("unsafe Git command used:\n%s", joined)
		}
	}
	if !strings.Contains(joined, "update-ref -d refs/heads/issue/361 old") ||
		!strings.Contains(joined, "update-ref refs/heads/main new old") {
		t.Fatalf("safe cleanup commands absent:\n%s", joined)
	}
}

func TestProductionLocalCompletionMovesExactCheckedOutIssueBranchBeforeDeletion(t *testing.T) {
	runner := &fakeLocalRunner{
		outputs: [][]byte{
			[]byte("head\n"), nil, []byte("issue/361\n"), nil, nil, nil,
		},
		errors: make([]error, 6),
	}
	err := (productionLocalCompletionGateway{runner: runner}).EnsureLocalIssueBranchAbsent(
		context.Background(),
		issuedelivery.DeleteLocalIssueBranchRequest{
			RepositoryPath: "/repo", Branch: "issue/361", HeadSHA: "head",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, call := range runner.calls {
		joined += call.name + " " + strings.Join(call.args, " ") + "\n"
	}
	if !strings.Contains(joined, "switch main") ||
		!strings.Contains(joined, "update-ref -d refs/heads/issue/361 head") {
		t.Fatalf("exact checked-out cleanup sequence absent:\n%s", joined)
	}
}

func TestProductionLocalCompletionFastForwardsCheckedOutMainWithoutDirtyingIt(t *testing.T) {
	runner := &fakeLocalRunner{
		outputs: [][]byte{
			[]byte("old\n"), []byte("new\n"), nil, nil, []byte("main\n"), nil,
		},
		errors: make([]error, 6),
	}
	err := (productionLocalCompletionGateway{runner: runner}).EnsureLocalMainFastForward(
		context.Background(),
		issuedelivery.FastForwardLocalMainRequest{
			RepositoryPath: "/repo", ExpectedOldSHA: "old",
			OriginMainSHA: "new", MergeCommitSHA: "merge",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, call := range runner.calls {
		joined += call.name + " " + strings.Join(call.args, " ") + "\n"
	}
	if !strings.Contains(joined, "merge --ff-only new") ||
		strings.Contains(joined, "update-ref refs/heads/main") {
		t.Fatalf("checked-out main was not advanced through its worktree:\n%s", joined)
	}
}

func TestOperatorStateDigestAllowsOnlyWorkflowOwnedMainHeadChange(t *testing.T) {
	if operatorHeadIdentity("main", "old", "issue/361", "candidate") !=
		operatorHeadIdentity("main", "new", "issue/361", "candidate") {
		t.Fatal("workflow-owned local main advance changed the operator-state identity")
	}
	if operatorHeadIdentity("issue/361", "candidate", "issue/361", "candidate") !=
		operatorHeadIdentity("main", "old", "issue/361", "candidate") {
		t.Fatal("workflow-owned issue-to-main transition changed the operator-state identity")
	}
	if operatorHeadIdentity("topic", "old", "issue/361", "candidate") ==
		operatorHeadIdentity("topic", "new", "issue/361", "candidate") {
		t.Fatal("operator-owned branch head change was ignored")
	}
}
