package githubproposal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

func TestPublisherCreatesOneReadyProposalFromASealedCandidate(t *testing.T) {
	candidate := testCandidate()
	gateway := newFakeGateway(candidate)
	publisher := New(gateway)

	publication, err := publisher.Publish(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if publication.Proposal == nil || publication.Proposal.Branch != candidate.Branch || publication.Proposal.Number != 17 {
		t.Fatalf("Publish() = %+v", publication)
	}
	if strings.Join(gateway.mutations, ",") != "push,create-pr" {
		t.Fatalf("mutations = %v", gateway.mutations)
	}
	if gateway.pullRequests[0].Draft || gateway.pullRequests[0].AutoMerge {
		t.Fatalf("proposal must be ready without auto-merge: %+v", gateway.pullRequests[0])
	}
	wantRecord := proposalRecord{
		Branch: "promote/addy-1.2.3", Coordinate: "addy@1.2.3", Owner: "packy-bot",
		Project: "yersonargotev/skills-addy", Version: 1,
	}
	record, summary, err := parseBody(gateway.pullRequests[0].Body)
	if err != nil || record != wantRecord || summary != candidate.Summary {
		t.Fatalf("immutable proposal body = record %+v, summary %q, error %v", record, summary, err)
	}
	for _, mutableSeal := range []string{candidate.ID, candidate.BaseSHA, candidate.HeadSHA, candidate.ResultTreeSHA} {
		if strings.Contains(gateway.pullRequests[0].Body, mutableSeal) {
			t.Fatalf("immutable proposal body contains candidate-specific seal %q", mutableSeal)
		}
	}
}

func TestPublisherReturnsNoChangeForTheExactUntouchedProposal(t *testing.T) {
	candidate, gateway := publishedFixture(t)

	publication, err := New(gateway).Publish(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if publication.Proposal != nil || publication.NoChangeReason == "" {
		t.Fatalf("Publish() = %+v", publication)
	}
	if len(gateway.mutations) != 0 {
		t.Fatalf("no-change mutations = %v", gateway.mutations)
	}
}

func TestPublisherAppendsAnUntouchedProposalWithoutRewritingHistory(t *testing.T) {
	old, gateway := publishedFixture(t)
	oldHead := gateway.branch.HeadSHA
	oldTitle, oldBody := gateway.pullRequests[0].Title, gateway.pullRequests[0].Body
	candidate := old
	candidate.ID = strings.Repeat("2", 64)
	candidate.HeadSHA = strings.Repeat("e", 40)
	candidate.ResultTreeSHA = strings.Repeat("f", 40)

	publication, err := New(gateway).Publish(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if publication.Proposal == nil || publication.Proposal.HeadSHA == oldHead {
		t.Fatalf("Publish() = %+v", publication)
	}
	if strings.Join(gateway.mutations, ",") != "commit,push" {
		t.Fatalf("mutations = %v", gateway.mutations)
	}
	if strings.Join(gateway.lastCommit.ParentSHAs, ",") != oldHead {
		t.Fatalf("append parents = %q, want %q", gateway.lastCommit.ParentSHAs, oldHead)
	}
	if gateway.pullRequests[0].Title != oldTitle || gateway.pullRequests[0].Body != oldBody || len(gateway.pullRequests[0].ContentEdits) != 0 {
		t.Fatalf("immutable proposal metadata changed: %+v", gateway.pullRequests[0])
	}
}

func TestPublisherDoesNotOverwriteAHumanEditRacingAnOrdinaryUpdate(t *testing.T) {
	old, gateway := publishedFixture(t)
	candidate := old
	candidate.ID = strings.Repeat("2", 64)
	candidate.HeadSHA = strings.Repeat("e", 40)
	candidate.ResultTreeSHA = strings.Repeat("f", 40)
	gateway.afterObserve = func(gateway *fakeGateway, count int) {
		if count == 2 {
			gateway.pullRequests[0].Body += "\nhuman note"
			gateway.pullRequests[0].LastEditor = "maintainer"
		}
	}

	_, err := New(gateway).Publish(context.Background(), candidate)
	assertRejectionGate(t, err, managedpackpromotion.GateProposalOwnership)
	if strings.Join(gateway.mutations, ",") != "commit,push" {
		t.Fatalf("mutations = %v, want append and push without metadata edit", gateway.mutations)
	}
}

func TestPublisherRegeneratesAnUntouchedStaleProposalOnTheNewBaseWithAMergeCommit(t *testing.T) {
	old, gateway := publishedFixture(t)
	oldHead := gateway.branch.HeadSHA
	oldTitle, oldBody := gateway.pullRequests[0].Title, gateway.pullRequests[0].Body
	candidate := rebuiltCandidate(old, gateway)

	publication, err := New(gateway).Publish(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if publication.Proposal == nil || publication.Proposal.HeadSHA == oldHead {
		t.Fatalf("Publish() = %+v", publication)
	}
	if strings.Join(gateway.mutations, ",") != "commit,push" {
		t.Fatalf("mutations = %v", gateway.mutations)
	}
	wantParents := []string{oldHead, candidate.BaseSHA}
	if strings.Join(gateway.lastCommit.ParentSHAs, ",") != strings.Join(wantParents, ",") {
		t.Fatalf("regeneration parents = %v, want %v", gateway.lastCommit.ParentSHAs, wantParents)
	}
	if gateway.pullRequests[0].Title != oldTitle || gateway.pullRequests[0].Body != oldBody || len(gateway.pullRequests[0].ContentEdits) != 0 {
		t.Fatalf("immutable proposal metadata changed: %+v", gateway.pullRequests[0])
	}

	publication, err = New(gateway).Publish(context.Background(), candidate)
	if err != nil {
		t.Fatalf("second Publish() error = %v", err)
	}
	if publication.NoChangeReason == "" || strings.Join(gateway.mutations, ",") != "commit,push" {
		t.Fatalf("second publication = %+v, mutations = %v", publication, gateway.mutations)
	}
}

func TestPublisherDoesNotOverwriteAHumanEditRacingAStaleBaseRegeneration(t *testing.T) {
	old, gateway := publishedFixture(t)
	candidate := rebuiltCandidate(old, gateway)
	gateway.afterObserve = func(gateway *fakeGateway, count int) {
		if count == 2 {
			gateway.pullRequests[0].Title = "Human title"
			gateway.pullRequests[0].LastEditor = "maintainer"
		}
	}

	_, err := New(gateway).Publish(context.Background(), candidate)
	assertRejectionGate(t, err, managedpackpromotion.GateProposalOwnership)
	if strings.Join(gateway.mutations, ",") != "commit,push" {
		t.Fatalf("mutations = %v, want regeneration and push without metadata edit", gateway.mutations)
	}
}

func TestPublisherRejectsStaleProposalsWithHumanOrForeignState(t *testing.T) {
	tests := map[string]func(*fakeGateway){
		"human-edited metadata": func(gateway *fakeGateway) {
			gateway.pullRequests[0].Body += "\nhuman note"
			gateway.pullRequests[0].LastEditor = "maintainer"
		},
		"foreign branch history": func(gateway *fakeGateway) {
			gateway.branch.Commits[len(gateway.branch.Commits)-1].Message = "Promote foreign-1.2.3"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			old, gateway := publishedFixture(t)
			candidate := rebuiltCandidate(old, gateway)
			mutate(gateway)

			_, err := New(gateway).Publish(context.Background(), candidate)
			assertRejectionGate(t, err, managedpackpromotion.GateProposalOwnership)
			if len(gateway.mutations) != 0 {
				t.Fatalf("mutations = %v", gateway.mutations)
			}
		})
	}
}

func TestPublisherRejectsHumanProposalEdits(t *testing.T) {
	tests := map[string]func(*fakeGateway){
		"body": func(gateway *fakeGateway) {
			gateway.pullRequests[0].Body += "\nhuman note"
			gateway.pullRequests[0].LastEditor = "maintainer"
		},
		"title": func(gateway *fakeGateway) {
			gateway.pullRequests[0].Title = "Edited title"
			gateway.pullRequests[0].LastEditor = "maintainer"
		},
		"bot content edit": func(gateway *fakeGateway) {
			gateway.pullRequests[0].LastEditor = gateway.actor
			gateway.pullRequests[0].ContentEdits = []ContentEdit{{ID: "edit-1", Editor: gateway.actor}}
		},
		"branch head": func(gateway *fakeGateway) {
			head := strings.Repeat("9", 40)
			human := Commit{
				SHA: head, Parents: []string{gateway.branch.HeadSHA}, TreeSHA: gateway.branch.TreeSHA,
				AuthorName: "Maintainer", AuthorEmail: "maintainer@example.test",
				CommitterName: "Maintainer", CommitterEmail: "maintainer@example.test", Message: "manual edit",
			}
			gateway.branch.HeadSHA = head
			gateway.branch.Commits = append([]Commit{human}, gateway.branch.Commits...)
			gateway.pullRequests[0].HeadSHA = head
		},
	}

	for name, edit := range tests {
		t.Run(name, func(t *testing.T) {
			candidate, gateway := publishedFixture(t)
			edit(gateway)

			_, err := New(gateway).Publish(context.Background(), candidate)
			assertRejectionGate(t, err, managedpackpromotion.GateProposalOwnership)
			if len(gateway.mutations) != 0 {
				t.Fatalf("mutations after human edit = %v", gateway.mutations)
			}
		})
	}
}

func TestPublisherRejectsAStaleBaseBeforeMutation(t *testing.T) {
	candidate := testCandidate()
	gateway := newFakeGateway(candidate)
	gateway.base = strings.Repeat("8", 40)

	_, err := New(gateway).Publish(context.Background(), candidate)
	assertRejectionGate(t, err, managedpackpromotion.GateFreshness)
	if len(gateway.mutations) != 0 {
		t.Fatalf("stale-base mutations = %v", gateway.mutations)
	}
}

func TestPublisherRejectsCompetingAndClosedPullRequests(t *testing.T) {
	tests := map[string]func(*fakeGateway){
		"competing": func(gateway *fakeGateway) {
			competing := gateway.pullRequests[0]
			competing.Number = 18
			gateway.pullRequests = append(gateway.pullRequests, competing)
		},
		"closed": func(gateway *fakeGateway) { gateway.pullRequests[0].State = "CLOSED" },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			candidate, gateway := publishedFixture(t)
			change(gateway)
			_, err := New(gateway).Publish(context.Background(), candidate)
			assertRejectionGate(t, err, managedpackpromotion.GateProposalOwnership)
			if len(gateway.mutations) != 0 {
				t.Fatalf("mutations = %v", gateway.mutations)
			}
		})
	}
}

func TestPublisherRecoversAnAppendCompletedBeforeThePriorProcessStopped(t *testing.T) {
	old, gateway := publishedFixture(t)
	candidate := old
	candidate.ID = strings.Repeat("2", 64)
	candidate.HeadSHA = strings.Repeat("e", 40)
	candidate.ResultTreeSHA = strings.Repeat("f", 40)
	record := recordFor(candidate, gateway.actor)
	head, err := gateway.Commit(context.Background(), CommitRequest{
		RepositoryRoot: candidate.RepositoryRoot,
		ParentSHAs:     []string{gateway.branch.HeadSHA},
		TreeSHA:        candidate.ResultTreeSHA,
		Message:        commitMessage(candidate, record),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.FastForwardPush(context.Background(), candidate.RepositoryRoot, head, candidate.Branch); err != nil {
		t.Fatal(err)
	}
	gateway.mutations = nil

	publication, err := New(gateway).Publish(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if publication.NoChangeReason == "" || len(gateway.mutations) != 0 {
		t.Fatalf("publication = %+v, mutations = %v", publication, gateway.mutations)
	}
}

func TestPublisherRecoversTheExactOwnedBranchWhenPullRequestProjectionIsAbsent(t *testing.T) {
	candidate := testCandidate()
	gateway := newFakeGateway(candidate)
	record := recordFor(candidate, gateway.actor)
	head, err := gateway.Commit(context.Background(), CommitRequest{
		RepositoryRoot: candidate.RepositoryRoot, ParentSHAs: []string{candidate.BaseSHA},
		TreeSHA: candidate.ResultTreeSHA, Message: commitMessage(candidate, record),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.FastForwardPush(context.Background(), candidate.RepositoryRoot, head, candidate.Branch); err != nil {
		t.Fatal(err)
	}
	gateway.mutations = nil

	publication, err := New(gateway).Publish(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if publication.Proposal == nil || strings.Join(gateway.mutations, ",") != "create-pr" {
		t.Fatalf("publication = %+v, mutations = %v", publication, gateway.mutations)
	}
}

func TestCLIPublisherUsesNormalPushAndObservesAuthorAndEditorThroughGraphQL(t *testing.T) {
	candidate := testCandidate()
	runner := &publicationRunner{candidate: candidate, publishedHead: candidate.HeadSHA}

	publication, err := NewCLIWithRunner(runner).Publish(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if publication.Proposal == nil || publication.Proposal.HeadSHA != runner.publishedHead {
		t.Fatalf("Publish() = %+v", publication)
	}
	if !runner.sawGraphQL {
		t.Fatal("publisher did not observe pull request author/editor through GraphQL")
	}
	for _, command := range runner.commands {
		for _, argument := range command.Arguments {
			if argument == "--force" || argument == "--force-with-lease" || strings.HasPrefix(argument, "+") {
				t.Fatalf("force-capable argument in command: %+v", command)
			}
		}
	}
}

func TestCLIGatewayFailsClosedWhenTheLastContentEditorsIdentityIsUnavailable(t *testing.T) {
	runner := &publicationRunner{
		candidate: testCandidate(),
		graphqlResponse: `{"data":{"repository":{"pullRequest":{"author":{"login":"packy-bot"},` +
			`"userContentEdits":{"totalCount":1,"pageInfo":{"hasNextPage":false},` +
			`"nodes":[{"id":"edit-1","editor":null}]}}}}}`,
	}
	author, edits, err := (&cliGateway{runner: runner}).observeEditors(context.Background(), "/tmp/packy", "yersonargotev", "packy", 17)
	if err != nil {
		t.Fatalf("observeEditors() error = %v", err)
	}
	if author != "packy-bot" || len(edits) != 1 || edits[0] != (ContentEdit{ID: "edit-1", Editor: unavailableEditor}) {
		t.Fatalf("author/edits = %q/%+v", author, edits)
	}
}

func TestCLIPublisherRejectsAHumanEditHiddenByALaterBotEdit(t *testing.T) {
	candidate := testCandidate()
	runner := &publicationRunner{
		candidate: candidate, publishedHead: candidate.HeadSHA, branchExists: true, prExists: true,
		prTitle: titleFor(candidate), prBody: bodyFor(candidate, "packy-bot"),
		graphqlResponse: `{"data":{"repository":{"pullRequest":{"author":{"login":"packy-bot"},` +
			`"userContentEdits":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[` +
			`{"id":"edit-human","editor":{"login":"maintainer"}},` +
			`{"id":"edit-bot","editor":{"login":"packy-bot"}}]}}}}}`,
	}

	_, err := NewCLIWithRunner(runner).Publish(context.Background(), candidate)
	assertRejectionGate(t, err, managedpackpromotion.GateProposalOwnership)
	if commandNamed(runner.commands, "gh", "pr", "edit") {
		t.Fatal("publisher edited metadata after observing a human content edit")
	}
}

func TestCLIPublisherFailsClosedWhenContentEditHistoryIsTruncated(t *testing.T) {
	candidate := testCandidate()
	runner := &publicationRunner{
		candidate: candidate, publishedHead: candidate.HeadSHA, branchExists: true, prExists: true,
		prTitle: titleFor(candidate), prBody: bodyFor(candidate, "packy-bot"),
		graphqlResponse: `{"data":{"repository":{"pullRequest":{"author":{"login":"packy-bot"},` +
			`"userContentEdits":{"totalCount":101,"pageInfo":{"hasNextPage":true},"nodes":[]}}}}}`,
	}

	_, err := NewCLIWithRunner(runner).Publish(context.Background(), candidate)
	assertRejectionGate(t, err, managedpackpromotion.GatePublication)
	if commandNamed(runner.commands, "gh", "pr", "edit") {
		t.Fatal("publisher edited metadata with incomplete content edit history")
	}
	var query string
	for _, command := range runner.commands {
		if commandNamed([]Command{command}, "gh", "api", "graphql") {
			query = flagValue(command.Arguments, "-f")
		}
	}
	for _, required := range []string{"userContentEdits(first:100)", "totalCount", "hasNextPage", "nodes{id editor{login}}"} {
		if !strings.Contains(query, required) {
			t.Fatalf("GraphQL query %q lacks %q", query, required)
		}
	}
}

func TestCLIGatewayCreatesARegenerationCommitWithTheExactParentSet(t *testing.T) {
	tree := strings.Repeat("c", 40)
	oldHead := strings.Repeat("d", 40)
	newBase := strings.Repeat("e", 40)
	created := strings.Repeat("f", 40)
	var commands []Command
	runner := runnerFunc(func(_ context.Context, command Command) (string, error) {
		commands = append(commands, command)
		switch command.Arguments[0] {
		case "commit-tree":
			return created + "\n", nil
		case "show":
			return tree + "\x00" + oldHead + " " + newBase + "\n", nil
		case "push":
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %+v", command)
		}
	})

	gateway := &cliGateway{runner: runner}
	head, err := gateway.Commit(context.Background(), CommitRequest{
		RepositoryRoot: "/tmp/packy", ParentSHAs: []string{oldHead, newBase}, TreeSHA: tree, Message: "regenerate",
	})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if err := gateway.FastForwardPush(context.Background(), "/tmp/packy", head, "promote/addy-1.2.3"); err != nil {
		t.Fatalf("FastForwardPush() error = %v", err)
	}
	if head != created || len(commands) != 3 {
		t.Fatalf("head = %q, commands = %+v", head, commands)
	}
	want := []string{"commit-tree", tree, "-p", oldHead, "-p", newBase, "-m", "regenerate"}
	if strings.Join(commands[0].Arguments, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("commit-tree arguments = %v, want %v", commands[0].Arguments, want)
	}
	if strings.Join(commands[1].Arguments, " ") != "show -s --format=%T%x00%P "+created {
		t.Fatalf("verification command = %v", commands[1].Arguments)
	}
	wantPush := "push origin " + created + ":refs/heads/promote/addy-1.2.3"
	if strings.Join(commands[2].Arguments, " ") != wantPush {
		t.Fatalf("push command = %v, want %q", commands[2].Arguments, wantPush)
	}
}

func TestPublicationProcessReceivesOnlyDeliveryAuthority(t *testing.T) {
	environment := publicationEnvironment([]string{
		"PATH=/safe/bin",
		"HOME=/safe/home",
		"XDG_CONFIG_HOME=/safe/config",
		"GH_TOKEN=publication-token",
		"SSH_AUTH_SOCK=/safe/agent.sock",
		"AWS_SECRET_ACCESS_KEY=unrelated-secret",
		"PACKY_ACQUIRED_PROJECT=/untrusted/project",
		"HTTP_PROXY=http://credential:secret@proxy.test",
		"GIT_AUTHOR_NAME=ambient-author",
	}, []string{
		"GIT_AUTHOR_NAME=" + botAuthorName,
		"GIT_AUTHOR_EMAIL=" + botAuthorEmail,
	})

	want := []string{
		"GCM_INTERACTIVE=Never",
		"GH_PROMPT_DISABLED=1",
		"GH_TOKEN=publication-token",
		"GIT_AUTHOR_EMAIL=" + botAuthorEmail,
		"GIT_AUTHOR_NAME=" + botAuthorName,
		"GIT_TERMINAL_PROMPT=0",
		"HOME=/safe/home",
		"PATH=/safe/bin",
		"SSH_AUTH_SOCK=/safe/agent.sock",
		"XDG_CONFIG_HOME=/safe/config",
	}
	if !slices.Equal(environment, want) {
		t.Fatalf("publication environment = %q, want %q", environment, want)
	}
}

func testCandidate() managedpackpromotion.Candidate {
	return managedpackpromotion.Candidate{
		ID:             strings.Repeat("1", 64),
		Summary:        "Admit addy 1.2.3 after every promotion gate passed.",
		Coordinate:     managedpackpromotion.Coordinate{PackID: "addy", Version: "1.2.3"},
		Project:        "yersonargotev/skills-addy",
		RepositoryRoot: "/tmp/packy",
		BaseSHA:        strings.Repeat("a", 40),
		HeadSHA:        strings.Repeat("b", 40),
		ResultTreeSHA:  strings.Repeat("c", 40),
		Branch:         "promote/addy-1.2.3",
	}
}

func rebuiltCandidate(old managedpackpromotion.Candidate, gateway *fakeGateway) managedpackpromotion.Candidate {
	candidate := old
	candidate.ID = strings.Repeat("2", 64)
	candidate.BaseSHA = strings.Repeat("8", 40)
	candidate.HeadSHA = strings.Repeat("e", 40)
	candidate.ResultTreeSHA = strings.Repeat("f", 40)
	gateway.base = candidate.BaseSHA
	gateway.branch.BaseAncestor = false
	return candidate
}

type fakeGateway struct {
	candidate    managedpackpromotion.Candidate
	actor        string
	base         string
	branch       *Branch
	pullRequests []PullRequest
	mutations    []string
	commitCount  int
	commits      map[string]Commit
	lastCommit   CommitRequest
	observeCount int
	afterObserve func(*fakeGateway, int)
}

func newFakeGateway(candidate managedpackpromotion.Candidate) *fakeGateway {
	gateway := &fakeGateway{
		candidate: candidate, actor: "packy-bot", base: candidate.BaseSHA,
		commits: make(map[string]Commit),
	}
	gateway.commits[candidate.HeadSHA] = Commit{
		SHA: candidate.HeadSHA, Parents: []string{candidate.BaseSHA}, TreeSHA: candidate.ResultTreeSHA,
		AuthorName: botAuthorName, AuthorEmail: botAuthorEmail,
		CommitterName: botAuthorName, CommitterEmail: botAuthorEmail,
		Message: "Promote " + candidate.Coordinate.String(),
	}
	return gateway
}

func (gateway *fakeGateway) VerifyCandidate(context.Context, managedpackpromotion.Candidate) error {
	return nil
}

func (gateway *fakeGateway) Observe(context.Context, managedpackpromotion.Candidate) (Observation, error) {
	gateway.observeCount++
	observed := Observation{Actor: gateway.actor, BaseSHA: gateway.base, Branch: gateway.branch, PullRequests: append([]PullRequest(nil), gateway.pullRequests...)}
	if gateway.afterObserve != nil {
		gateway.afterObserve(gateway, gateway.observeCount)
	}
	return observed, nil
}

func (gateway *fakeGateway) Commit(_ context.Context, request CommitRequest) (string, error) {
	gateway.mutations = append(gateway.mutations, "commit")
	gateway.commitCount++
	gateway.lastCommit = request
	head := strings.Repeat(string(rune('d'+gateway.commitCount-1)), 40)
	gateway.commits[head] = Commit{
		SHA: head, Parents: append([]string(nil), request.ParentSHAs...), TreeSHA: request.TreeSHA,
		AuthorName: botAuthorName, AuthorEmail: botAuthorEmail,
		CommitterName: botAuthorName, CommitterEmail: botAuthorEmail, Message: request.Message,
	}
	return head, nil
}

func publishedFixture(t *testing.T) (managedpackpromotion.Candidate, *fakeGateway) {
	t.Helper()
	candidate := testCandidate()
	gateway := newFakeGateway(candidate)
	if _, err := New(gateway).Publish(context.Background(), candidate); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	gateway.mutations = nil
	gateway.observeCount = 0
	return candidate, gateway
}

func assertRejectionGate(t *testing.T, err error, want managedpackpromotion.Gate) {
	t.Helper()
	var rejection *managedpackpromotion.RejectionError
	if !errors.As(err, &rejection) || rejection.Gate != want {
		t.Fatalf("error = %v, want rejection at %s", err, want)
	}
}

type publicationRunner struct {
	candidate       managedpackpromotion.Candidate
	publishedHead   string
	branchExists    bool
	prExists        bool
	commitMessage   string
	prTitle         string
	prBody          string
	commands        []Command
	sawGraphQL      bool
	graphqlResponse string
}

type runnerFunc func(context.Context, Command) (string, error)

func (run runnerFunc) Run(ctx context.Context, command Command) (string, error) {
	return run(ctx, command)
}

func (runner *publicationRunner) Run(_ context.Context, command Command) (string, error) {
	runner.commands = append(runner.commands, command)
	args := command.Arguments
	if command.Executable == "git" && len(args) >= 1 {
		switch args[0] {
		case "show":
			return runner.candidate.ResultTreeSHA + "\x00" + runner.candidate.BaseSHA + "\n", nil
		case "ls-remote":
			output := runner.candidate.BaseSHA + "\trefs/heads/main\n"
			if runner.branchExists {
				output += runner.publishedHead + "\trefs/heads/" + runner.candidate.Branch + "\n"
			}
			return output, nil
		case "commit-tree":
			runner.commitMessage = flagValue(args, "-m")
			return runner.publishedHead + "\n", nil
		case "push":
			runner.branchExists = true
			return "", nil
		case "fetch", "merge-base":
			return "", nil
		case "rev-parse":
			return runner.publishedHead + "\n", nil
		case "log":
			message := runner.commitMessage
			if message == "" {
				message = "Promote " + runner.candidate.Coordinate.String()
			}
			return strings.Join([]string{
				runner.publishedHead,
				runner.candidate.BaseSHA,
				runner.candidate.ResultTreeSHA,
				botAuthorName,
				botAuthorEmail,
				botAuthorName,
				botAuthorEmail,
				message,
			}, "\x1f") + "\x1e\n", nil
		}
	}
	if command.Executable == "gh" && len(args) >= 2 {
		switch args[0] + " " + args[1] {
		case "api user":
			return "packy-bot\n", nil
		case "pr list":
			if !runner.prExists {
				return "[]", nil
			}
			encoded, _ := json.Marshal([]map[string]any{{
				"number": 17, "url": "https://github.test/pull/17", "state": "OPEN", "isDraft": false,
				"baseRefName": "main", "headRefName": runner.candidate.Branch, "headRefOid": runner.publishedHead,
				"title": runner.prTitle, "body": runner.prBody, "autoMergeRequest": nil,
			}})
			return string(encoded), nil
		case "pr create":
			runner.prExists = true
			runner.prTitle = flagValue(args, "--title")
			runner.prBody = flagValue(args, "--body")
			return "https://github.test/pull/17\n", nil
		case "repo view":
			return "yersonargotev/packy\n", nil
		case "api graphql":
			runner.sawGraphQL = true
			if runner.graphqlResponse != "" {
				return runner.graphqlResponse, nil
			}
			return `{"data":{"repository":{"pullRequest":{"author":{"login":"packy-bot"},` +
				`"userContentEdits":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}}}}`, nil
		}
	}
	return "", fmt.Errorf("unexpected command: %s %v", command.Executable, args)
}

func flagValue(arguments []string, flag string) string {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == flag {
			return arguments[index+1]
		}
	}
	return ""
}

func commandNamed(commands []Command, executable string, arguments ...string) bool {
	for _, command := range commands {
		if command.Executable != executable || len(command.Arguments) < len(arguments) {
			continue
		}
		matched := true
		for index := range arguments {
			if command.Arguments[index] != arguments[index] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func (gateway *fakeGateway) FastForwardPush(_ context.Context, _ string, head, branch string) error {
	gateway.mutations = append(gateway.mutations, "push")
	commit, ok := gateway.commits[head]
	if !ok {
		return errors.New("unknown commit")
	}
	if gateway.branch != nil && (len(commit.Parents) == 0 || commit.Parents[0] != gateway.branch.HeadSHA) {
		return errors.New("push is not a fast-forward")
	}
	commits := []Commit{commit}
	if gateway.branch != nil {
		commits = append(commits, gateway.branch.Commits...)
	}
	gateway.branch = &Branch{Name: branch, HeadSHA: head, TreeSHA: commit.TreeSHA, BaseAncestor: true, Commits: commits}
	for index := range gateway.pullRequests {
		gateway.pullRequests[index].HeadSHA = head
	}
	return nil
}

func (gateway *fakeGateway) CreatePullRequest(_ context.Context, mutation PullRequestMutation) error {
	gateway.mutations = append(gateway.mutations, "create-pr")
	gateway.pullRequests = []PullRequest{{
		Number: 17, URL: "https://github.test/pull/17", State: "OPEN", BaseBranch: mutation.BaseBranch,
		HeadBranch: mutation.HeadBranch, HeadSHA: gateway.branch.HeadSHA, Title: mutation.Title,
		Body: mutation.Body, Author: gateway.actor,
	}}
	return nil
}
