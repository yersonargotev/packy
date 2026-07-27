package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

type fakeRunner struct {
	outputs [][]byte
	calls   []string
	failAt  int
}

type environmentRunner struct {
	environment []string
}

func (r environmentRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = r.environment
	return cmd.Output()
}

type fakeValidationRunner struct {
	calls  []deliveryevidence.SandboxFacts
	fail   bool
	mutate func()
}

func (f *fakeValidationRunner) Run(_ context.Context, _ string, sandbox deliveryevidence.SandboxFacts) error {
	f.calls = append(f.calls, sandbox)
	if f.mutate != nil {
		f.mutate()
	}
	if f.fail {
		return errors.New("validator failed")
	}
	return nil
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if f.failAt > 0 && len(f.calls) == f.failAt {
		return nil, errors.New("missing commit")
	}
	out := f.outputs[0]
	f.outputs = f.outputs[1:]
	return out, nil
}

func TestValidationRunnerEnvironmentReplacesOperatorRoots(t *testing.T) {
	got := replacedEnvironment(
		[]string{"HOME=/operator", "XDG_CONFIG_HOME=/operator/config", "PATH=/bin"},
		map[string]string{"HOME": "/sandbox/home", "XDG_CONFIG_HOME": "/sandbox/config"},
	)
	joined := "\n" + strings.Join(got, "\n") + "\n"
	for _, want := range []string{"\nHOME=/sandbox/home\n", "\nXDG_CONFIG_HOME=/sandbox/config\n", "\nPATH=/bin\n"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("replacement environment missing %q: %v", want, got)
		}
	}
	for _, forbidden := range []string{"HOME=/operator\n", "XDG_CONFIG_HOME=/operator/config\n"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("operator root survived replacement: %v", got)
		}
	}
}

func TestInitializeResumeAndFreshAuthorityInvalidation(t *testing.T) {
	root := t.TempDir()
	common := filepath.Join(root, "common")
	if err := os.MkdirAll(common, 0700); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work")
	home := filepath.Join(root, "home")
	xdg := filepath.Join(root, "xdg")
	for _, p := range []string{work, home, xdg} {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "sentinel"), []byte("unchanged"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	q := qualification{Schema: deliveryevidence.SchemaV1, IssueNumber: 276, SpecNumber: 99, DependencyDisposition: []deliveryevidence.DependencyDisposition{}, Scope: deliveryevidence.ScopeLedger{OwnedNow: []deliveryevidence.LedgerEntry{{Identity: "O1", Requirement: "owned", EvidenceLink: "issue#276"}}, Deferred: []deliveryevidence.DeferredEntry{}, Forbidden: []deliveryevidence.LedgerEntry{}, Prerequisites: []deliveryevidence.PrerequisiteEntry{}}, AcceptanceCriteria: []string{"AC1"}, AcceptanceMatrix: []deliveryevidence.AcceptanceRow{{Identity: "AC1", Criterion: "criterion", OwningSeam: "module", PositiveEvidence: "positive", NegativeEvidence: "negative", FailureEvidence: "failure", MutationEvidence: "mutation", CompatibilityEvidence: "compatible", PreservationEvidence: "preserved", MigrationEvidence: "N/A: no migration", State: "proved"}}, StartingBaseSHA: strings.Repeat("a", 40), Iterations: []deliveryevidence.Iteration{}}
	q.Repository.Owner = "owner"
	q.Repository.Name = "repo"
	input := filepath.Join(root, "qualification.json")
	raw, _ := json.Marshal(q)
	if err := os.WriteFile(input, raw, 0600); err != nil {
		t.Fatal(err)
	}
	repo := []byte(`{"nameWithOwner":"owner/repo","id":"R1"}`)
	issue := func(body string) []byte {
		return []byte(`{"number":276,"id":"I1","title":"issue","body":` + strconvQuote(body) + `,"state":"OPEN","labels":[{"name":"status:approved","id":"x","description":"","color":""}],"blockedBy":{"nodes":[],"totalCount":0}}`)
	}
	spec := func(body string) []byte {
		return []byte(`{"number":99,"id":"S1","title":"spec","body":` + strconvQuote(body) + `,"state":"OPEN","labels":[{"name":"status:accepted"}],"blockedBy":{"nodes":[],"totalCount":0}}`)
	}
	run := func(body, specBody string) (string, []byte) {
		git := &fakeRunner{outputs: [][]byte{[]byte(common + "\n"), []byte(work + "\n"), []byte("git@github.com:owner/repo.git\n"), []byte(strings.Repeat("a", 40) + "\n")}}
		gh := &fakeRunner{outputs: [][]byte{repo, issue(body), spec(specBody)}}
		var stdout bytes.Buffer
		err := (command{Git: git, GitHub: gh}).run(context.Background(), []string{"initialize", "--qualified-bundle", input, "--repository", work}, &stdout)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range append(git.calls, gh.calls...) {
			for _, bad := range []string{" add ", " commit ", " push ", " edit ", " create "} {
				if strings.Contains(" "+call+" ", bad) {
					t.Fatalf("mutating call: %s", call)
				}
			}
		}
		path := filepath.Join(common, "packy", "issue-delivery", "issue-276.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return stdout.String(), data
	}
	out, old := run("original", "accepted")
	if !strings.HasPrefix(out, "initialized ") {
		t.Fatal(out)
	}
	out, same := run("original", "accepted")
	if !strings.HasPrefix(out, "resumed ") || !bytes.Equal(old, same) {
		t.Fatalf("resume changed evidence: %s", out)
	}
	out, stale := run("changed", "accepted")
	if !strings.HasPrefix(out, "stale ") || !bytes.Equal(old, stale) {
		t.Fatalf("stale authority overwrote evidence: %s", out)
	}
	out, stale = run("original", "changed spec")
	if !strings.HasPrefix(out, "stale ") || !bytes.Equal(old, stale) {
		t.Fatalf("changed spec overwrote evidence: %s", out)
	}
	override := filepath.Join(root, "override", "evidence.json")
	gitOverride := &fakeRunner{outputs: [][]byte{[]byte(common + "\n"), []byte(work + "\n"), []byte("git@github.com:owner/repo.git\n"), []byte(strings.Repeat("a", 40) + "\n")}}
	needsReview := bytes.Replace(issue("original"), []byte("status:approved"), []byte("status:needs-review"), 1)
	ghOverride := &fakeRunner{outputs: [][]byte{repo, needsReview, spec("accepted")}}
	if err := (command{Git: gitOverride, GitHub: ghOverride}).run(context.Background(), []string{"initialize", "--qualified-bundle", input, "--repository", work, "--out", override}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(override); err != nil {
		t.Fatal("absolute override not written:", err)
	}
	foreignGit := &fakeRunner{outputs: [][]byte{[]byte(common + "\n"), []byte(work + "\n"), []byte("https://github.com/other/repo.git\n"), []byte(strings.Repeat("a", 40) + "\n")}}
	err := (command{Git: foreignGit, GitHub: &fakeRunner{}}).run(context.Background(), []string{"initialize", "--qualified-bundle", input, "--repository", work, "--out", filepath.Join(root, "foreign.json")}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "does not match local origin") {
		t.Fatalf("foreign local repository accepted: %v", err)
	}
	wrongBase := &fakeRunner{outputs: [][]byte{[]byte(common + "\n"), []byte(work + "\n"), []byte("git@github.com:owner/repo.git\n"), []byte(strings.Repeat("b", 40) + "\n")}}
	err = (command{Git: wrongBase, GitHub: &fakeRunner{}}).run(context.Background(), []string{"initialize", "--qualified-bundle", input, "--repository", work}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "starting base does not match") {
		t.Fatalf("foreign starting base accepted: %v", err)
	}
	qIteration := q
	qIteration.Iterations = []deliveryevidence.Iteration{{Sequence: 1, Identity: "iteration-1", BaseSHA: q.StartingBaseSHA, HeadSHA: strings.Repeat("b", 40), EvidenceSHA256: strings.Repeat("c", 64)}}
	iterationInput := filepath.Join(root, "iteration.json")
	iterationRaw, _ := json.Marshal(qIteration)
	if err := os.WriteFile(iterationInput, iterationRaw, 0600); err != nil {
		t.Fatal(err)
	}
	missingCommit := &fakeRunner{outputs: [][]byte{[]byte(common + "\n"), []byte(work + "\n"), []byte("git@github.com:owner/repo.git\n"), []byte(q.StartingBaseSHA + "\n")}, failAt: 5}
	err = (command{Git: missingCommit, GitHub: &fakeRunner{}}).run(context.Background(), []string{"initialize", "--qualified-bundle", iterationInput, "--repository", work}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "foreign or missing commit") {
		t.Fatalf("missing iteration commit accepted: %v", err)
	}
	inside := filepath.Join(work, "evidence.json")
	gitInside := &fakeRunner{outputs: [][]byte{[]byte(common + "\n"), []byte(work + "\n"), []byte("https://github.com/owner/repo.git\n"), []byte(strings.Repeat("a", 40) + "\n")}}
	ghInside := &fakeRunner{outputs: [][]byte{repo, issue("original"), spec("accepted")}}
	err = (command{Git: gitInside, GitHub: ghInside}).run(context.Background(), []string{"initialize", "--qualified-bundle", input, "--repository", work, "--out", inside}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "outside the worktree") {
		t.Fatalf("worktree override accepted: %v", err)
	}
	link := filepath.Join(root, "work-link")
	if err := os.Symlink(work, link); err != nil {
		t.Fatal(err)
	}
	gitLink := &fakeRunner{outputs: [][]byte{[]byte(common + "\n"), []byte(work + "\n"), []byte("git@github.com:owner/repo.git\n"), []byte(strings.Repeat("a", 40) + "\n")}}
	ghLink := &fakeRunner{outputs: [][]byte{repo, issue("original"), spec("accepted")}}
	err = (command{Git: gitLink, GitHub: ghLink}).run(context.Background(), []string{"initialize", "--qualified-bundle", input, "--repository", work, "--out", filepath.Join(link, "linked.json")}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "outside the worktree") {
		t.Fatalf("symlink worktree override accepted: %v", err)
	}
	trailing := filepath.Join(root, "trailing.json")
	if err := os.WriteFile(trailing, append(raw, []byte("{}")...), 0600); err != nil {
		t.Fatal(err)
	}
	err = (command{}).run(context.Background(), []string{"initialize", "--qualified-bundle", trailing}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "exactly one JSON value") {
		t.Fatalf("trailing qualification accepted: %v", err)
	}
	for _, p := range []string{work, home, xdg} {
		got, err := os.ReadFile(filepath.Join(p, "sentinel"))
		if err != nil || string(got) != "unchanged" {
			t.Fatalf("sandbox mutated: %s", p)
		}
	}
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestGitHubSlugRealShapes(t *testing.T) {
	for input, want := range map[string]string{"git@github.com:owner/repo.git": "owner/repo", "ssh://git@github.com/owner/repo.git": "owner/repo", "https://github.com/owner/repo.git": "owner/repo"} {
		got, err := githubSlug(input)
		if err != nil || got != want {
			t.Fatalf("githubSlug(%q)=%q,%v", input, got, err)
		}
	}
	if _, err := githubSlug("https://example.com/owner/repo.git"); err == nil {
		t.Fatal("foreign host accepted")
	}
}

func TestReviewCommandsAtSandboxedHighestSeam(t *testing.T) {
	root := t.TempDir()
	sentinel := filepath.Join(root, "operator-sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	base := strings.Repeat("a", 40)
	head1 := strings.Repeat("b", 40)
	head2 := strings.Repeat("c", 40)
	bundlePath := filepath.Join(root, "evidence.json")
	bundle := reviewBundleFixture(base)
	if err := deliveryevidence.StoreAtomic(bundlePath, bundle); err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) string {
		t.Helper()
		path := filepath.Join(root, name+".json")
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		var stdout bytes.Buffer
		c := command{}
		if len(args) > 0 && args[0] == "record-iteration" {
			c.Git = &fakeRunner{outputs: [][]byte{[]byte("commit\n"), []byte("commit\n")}}
		}
		err := c.run(context.Background(), args, &stdout)
		return stdout.String(), err
	}

	iteration1 := deliveryevidence.Iteration{Sequence: 1, Identity: "iteration-1", BaseSHA: base, HeadSHA: head1, EvidenceSHA256: strings.Repeat("1", 64)}
	if _, err := run("record-iteration", "--bundle", bundlePath, "--iteration", write("iteration-1", iteration1)); err != nil {
		t.Fatal(err)
	}
	standards := deliveryevidence.ReviewReceipt{IssueNumber: 277, Iteration: "iteration-1", BaseSHA: base, HeadSHA: head1, Axis: deliveryevidence.ReviewStandards, Findings: []deliveryevidence.ReviewFinding{}}
	if _, err := run("record-review", "--bundle", bundlePath, "--receipt", write("standards-1", map[string]any{"receipt": standards, "adjudications": []deliveryevidence.Adjudication{}})); err != nil {
		t.Fatal(err)
	}
	git := &fakeRunner{outputs: [][]byte{[]byte(head1 + "\n"), []byte(head1 + "\n")}}
	var missing bytes.Buffer
	if err := (command{Git: git}).run(context.Background(), []string{"review-status", "--bundle", bundlePath, "--repository", root}, &missing); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(missing.String(), "Unreviewed deltas: iteration-1") || !strings.Contains(missing.String(), "Uncovered commits: "+head1) {
		t.Fatalf("missing paired review was not reported:\n%s", missing.String())
	}

	before, _ := os.ReadFile(bundlePath)
	stale := standards
	stale.Axis = deliveryevidence.ReviewSpec
	stale.HeadSHA = head2
	if _, err := run("record-review", "--bundle", bundlePath, "--receipt", write("stale", map[string]any{"receipt": stale, "adjudications": []deliveryevidence.Adjudication{}})); err == nil || !strings.Contains(err.Error(), "exact delta") {
		t.Fatalf("stale receipt accepted: %v", err)
	}
	after, _ := os.ReadFile(bundlePath)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected stale receipt changed canonical evidence")
	}

	specFinding := deliveryevidence.ReviewFinding{ID: "SPEC-1", Axis: deliveryevidence.ReviewSpec, Severity: deliveryevidence.SeverityP1, Authority: deliveryevidence.AuthoritySpecRequirement, Citation: "issue#277:AC-05", Location: "internal/deliveryevidence/review.go", Evidence: "repair pairing is missing"}
	rejectedFinding := deliveryevidence.ReviewFinding{ID: "SPEC-2", Axis: deliveryevidence.ReviewSpec, Severity: deliveryevidence.SeverityP3, Authority: deliveryevidence.AuthoritySpecRequirement, Citation: "issue#277:AC-10", Location: "internal/tools/deliveryevidence/main.go", Evidence: "mutation concern is not reachable"}
	spec := deliveryevidence.ReviewReceipt{IssueNumber: 277, Iteration: "iteration-1", BaseSHA: base, HeadSHA: head1, Axis: deliveryevidence.ReviewSpec, Findings: []deliveryevidence.ReviewFinding{specFinding, rejectedFinding}}
	initialAdjudications := []deliveryevidence.Adjudication{
		{Sequence: 1, FindingID: "SPEC-1", Disposition: deliveryevidence.DispositionAccepted, Evidence: "repair is owned by iteration-2", RepairIteration: "iteration-2"},
		{Sequence: 2, FindingID: "SPEC-2", Disposition: deliveryevidence.DispositionRejectedWithEvidence, Evidence: "the private adapter exposes no GitHub mutation"},
	}
	if _, err := run("record-review", "--bundle", bundlePath, "--receipt", write("spec-1", map[string]any{"receipt": spec, "adjudications": initialAdjudications})); err != nil {
		t.Fatal(err)
	}
	iteration2 := deliveryevidence.Iteration{Sequence: 2, Identity: "iteration-2", BaseSHA: head1, HeadSHA: head2, EvidenceSHA256: strings.Repeat("2", 64)}
	if _, err := run("record-iteration", "--bundle", bundlePath, "--iteration", write("iteration-2", iteration2)); err != nil {
		t.Fatal(err)
	}
	for _, axis := range []deliveryevidence.ReviewAxis{deliveryevidence.ReviewStandards, deliveryevidence.ReviewSpec} {
		receipt := deliveryevidence.ReviewReceipt{IssueNumber: 277, Iteration: "iteration-2", BaseSHA: head1, HeadSHA: head2, Axis: axis, Findings: []deliveryevidence.ReviewFinding{}}
		if _, err := run("record-review", "--bundle", bundlePath, "--receipt", write(string(axis)+"-2", map[string]any{"receipt": receipt, "adjudications": []deliveryevidence.Adjudication{}})); err != nil {
			t.Fatal(err)
		}
	}
	adjudications := []deliveryevidence.Adjudication{
		{Sequence: 3, FindingID: "SPEC-1", Disposition: deliveryevidence.DispositionRepairedByLaterIteration, Evidence: "paired review proves the repair", RepairIteration: "iteration-2"},
	}
	for _, adjudication := range adjudications {
		if _, err := run("record-adjudication", "--bundle", bundlePath, "--adjudication", write(fmt.Sprintf("adjudication-%d", adjudication.Sequence), adjudication)); err != nil {
			t.Fatal(err)
		}
	}
	git = &fakeRunner{outputs: [][]byte{[]byte(head2 + "\n"), []byte(head1 + "\n" + head2 + "\n")}}
	var repaired bytes.Buffer
	if err := (command{Git: git}).run(context.Background(), []string{"review-status", "--bundle", bundlePath, "--repository", root}, &repaired); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Uncovered commits: none", "Unreviewed deltas: none", "Unresolved findings: none", "SPEC-1", "repaired-by-later-iteration", "SPEC-2", "rejected-with-evidence"} {
		if !strings.Contains(repaired.String(), want) {
			t.Fatalf("review report missing %q:\n%s", want, repaired.String())
		}
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "unchanged" {
		t.Fatalf("sandbox sentinel changed: %q %v", got, err)
	}
	for _, call := range git.calls {
		if !strings.Contains(call, " rev-parse ") && !strings.Contains(call, " rev-list ") {
			t.Fatalf("unexpected external authority: %s", call)
		}
	}
}

func TestValidationCommandsAtSandboxedHighestSeam(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "checkout")
	if err := os.MkdirAll(filepath.Join(repository, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	validatorPath := filepath.Join(repository, "scripts", "validate-packy.sh")
	if err := os.WriteFile(validatorPath, []byte("#!/usr/bin/env bash\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(root, "evidence.json")
	bundle := reviewBundleFixture(strings.Repeat("a", 40))
	if err := deliveryevidence.StoreAtomic(bundlePath, bundle); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "validation-home")
	config := filepath.Join(root, "validation-config")
	head := strings.Repeat("b", 40)
	tree := strings.Repeat("c", 40)
	gitObservation := func(observedTree string, dirty bool) [][]byte {
		status := []byte{}
		if dirty {
			status = []byte(" M tracked.go\n")
		}
		return [][]byte{
			[]byte("git@github.com:yersonargotev/packy.git\n"),
			[]byte(head + "\n"),
			[]byte(observedTree + "\n"),
			status,
		}
	}
	clock := func() time.Time { return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC) }
	repositoryObservation := []byte(`{"nameWithOwner":"yersonargotev/packy","id":"R1"}`)
	args := []string{
		"record-exhaustive-validation",
		"--bundle", bundlePath,
		"--repository", repository,
		"--sandbox-home", home,
		"--sandbox-config-home", config,
		"--validator-identity-expires-at", "2026-07-28T00:00:00Z",
	}
	git := &fakeRunner{outputs: append(gitObservation(tree, false), gitObservation(tree, false)...)}
	github := &fakeRunner{outputs: [][]byte{repositoryObservation, repositoryObservation}}
	validation := &fakeValidationRunner{}
	var stdout bytes.Buffer
	if err := (command{Git: git, GitHub: github, Validation: validation, Now: clock}).run(context.Background(), args, &stdout); err != nil {
		t.Fatal(err)
	}
	if len(validation.calls) != 1 || validation.calls[0].HomeRoot != home || validation.calls[0].ConfigHomeRoot != config {
		t.Fatalf("validator did not receive exact sandbox facts: %+v", validation.calls)
	}
	recorded, _, err := deliveryevidence.Load(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded.ValidationReceipts) != 1 || !recorded.ValidationReceipts[0].Succeeded {
		t.Fatalf("authoritative receipt not recorded: %+v", recorded.ValidationReceipts)
	}

	statusArgs := append([]string{"validation-status"}, args[1:]...)
	assertStatus := func(outputs [][]byte, commandArgs []string, now func() time.Time, repositoryIdentity []byte, want string) {
		t.Helper()
		var output bytes.Buffer
		err := (command{Git: &fakeRunner{outputs: outputs}, GitHub: &fakeRunner{outputs: [][]byte{repositoryIdentity}}, Now: now}).run(context.Background(), commandArgs, &output)
		if want == "" {
			if err != nil || !strings.Contains(output.String(), "reusable exhaustive validation") {
				t.Fatalf("exact receipt not reusable: %q %v", output.String(), err)
			}
			return
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("wanted %q, got %v", want, err)
		}
	}
	assertStatus(gitObservation(tree, false), statusArgs, clock, repositoryObservation, "")
	changedCommit := gitObservation(tree, false)
	changedCommit[1] = []byte(strings.Repeat("d", 40) + "\n")
	assertStatus(changedCommit, statusArgs, clock, repositoryObservation, "no exhaustive validation receipt matches")
	assertStatus(gitObservation(strings.Repeat("d", 40), false), statusArgs, clock, repositoryObservation, "no exhaustive validation receipt matches")
	assertStatus(gitObservation(tree, true), statusArgs, clock, repositoryObservation, "clean workspace")
	alteredSandbox := append([]string(nil), statusArgs...)
	for i := range alteredSandbox {
		if alteredSandbox[i] == config {
			alteredSandbox[i] = config + "-changed"
		}
	}
	assertStatus(gitObservation(tree, false), alteredSandbox, clock, repositoryObservation, "no exhaustive validation receipt matches")
	changedCommand := append([]string(nil), statusArgs...)
	changedCommand = append(changedCommand, "--required-command", "./scripts/validate-packy.sh --changed")
	assertStatus(gitObservation(tree, false), changedCommand, clock, repositoryObservation, "no exhaustive validation receipt matches")
	foreignRepository := []byte(`{"nameWithOwner":"yersonargotev/packy","id":"R-foreign"}`)
	assertStatus(gitObservation(tree, false), statusArgs, clock, foreignRepository, "repository identity does not match")
	expiredClock := func() time.Time { return time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC) }
	assertStatus(gitObservation(tree, false), statusArgs, expiredClock, repositoryObservation, "validator identity is expired")
	incompleteIdentity := []string{"validation-status", "--bundle", bundlePath, "--repository", repository, "--sandbox-home", home, "--sandbox-config-home", config}
	assertStatus(nil, incompleteIdentity, clock, repositoryObservation, "validator-identity-expires-at are required")

	if err := os.WriteFile(validatorPath, []byte("#!/usr/bin/env bash\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	assertStatus(gitObservation(tree, false), statusArgs, clock, repositoryObservation, "no exhaustive validation receipt matches")
	if err := os.WriteFile(validatorPath, []byte("#!/usr/bin/env bash\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(root, "foreign-checkout")
	if err := os.MkdirAll(filepath.Join(foreign, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "scripts", "validate-packy.sh"), []byte("#!/usr/bin/env bash\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	foreignArgs := append([]string(nil), statusArgs...)
	for i := range foreignArgs {
		if foreignArgs[i] == repository {
			foreignArgs[i] = foreign
		}
	}
	assertStatus(gitObservation(tree, false), foreignArgs, clock, repositoryObservation, "no exhaustive validation receipt matches")

	failingPath := filepath.Join(root, "failed.json")
	if err := deliveryevidence.StoreAtomic(failingPath, bundle); err != nil {
		t.Fatal(err)
	}
	_, beforeFailure, err := deliveryevidence.Load(failingPath)
	if err != nil {
		t.Fatal(err)
	}
	failingArgs := append([]string(nil), args...)
	for i := range failingArgs {
		if failingArgs[i] == bundlePath {
			failingArgs[i] = failingPath
		}
	}
	failedValidation := &fakeValidationRunner{fail: true}
	err = (command{Git: &fakeRunner{outputs: gitObservation(tree, false)}, GitHub: &fakeRunner{outputs: [][]byte{repositoryObservation}}, Validation: failedValidation, Now: clock}).run(context.Background(), failingArgs, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "exhaustive validation failed") {
		t.Fatalf("failed validation accepted: %v", err)
	}
	_, afterFailure, err := deliveryevidence.Load(failingPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeFailure, afterFailure) {
		t.Fatal("failed validation changed canonical evidence")
	}

	focusedPath := filepath.Join(root, "focused.json")
	if err := deliveryevidence.StoreAtomic(focusedPath, bundle); err != nil {
		t.Fatal(err)
	}
	focusedInput := filepath.Join(root, "focused-input.json")
	focused := deliveryevidence.FocusedValidationEvidence{Identity: "changed-impact", Command: "./scripts/validate-changed.sh", CommitSHA: head, TreeSHA: tree, CompletedAt: "2026-07-27T11:00:00Z", Succeeded: true, Completed: true}
	raw, _ := json.Marshal(focused)
	if err := os.WriteFile(focusedInput, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := (command{}).run(context.Background(), []string{"record-focused-validation", "--bundle", focusedPath, "--evidence", focusedInput}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	focusedStatus := append([]string(nil), statusArgs...)
	for i := range focusedStatus {
		if focusedStatus[i] == bundlePath {
			focusedStatus[i] = focusedPath
		}
	}
	assertStatus(gitObservation(tree, false), focusedStatus, clock, repositoryObservation, "no exhaustive validation receipt matches")
}

type localGateFixture struct {
	root        string
	repository  string
	bundlePath  string
	home        string
	config      string
	branch      string
	issue       issueObservation
	spec        issueObservation
	environment []string
}

func newLocalGateFixture(t *testing.T, iterations int) localGateFixture {
	t.Helper()
	root := t.TempDir()
	repository := filepath.Join(root, "checkout")
	operatorHome := filepath.Join(root, "operator-home")
	operatorConfig := filepath.Join(root, "operator-config")
	environment := replacedEnvironment(os.Environ(), map[string]string{
		"HOME":                operatorHome,
		"XDG_CONFIG_HOME":     operatorConfig,
		"GIT_CONFIG_GLOBAL":   filepath.Join(root, "empty-gitconfig"),
		"GIT_CONFIG_NOSYSTEM": "1",
	})
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repository
		cmd.Env = environment
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.MkdirAll(filepath.Join(repository, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	run("git", "init", "-b", "feat/issue-279-local-delivery-gate")
	run("git", "config", "user.name", "Gate Test")
	run("git", "config", "user.email", "gate@example.com")
	run("git", "remote", "add", "origin", "git@github.com:yersonargotev/packy.git")
	validator := []byte("#!/usr/bin/env bash\nexit 0\n")
	if err := os.WriteFile(filepath.Join(repository, "scripts", "validate-packy.sh"), validator, 0700); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "scripts/validate-packy.sh")
	run("git", "commit", "-m", "base")
	base := run("git", "rev-parse", "HEAD")

	var recorded []deliveryevidence.Iteration
	previous := base
	for i := 1; i <= iterations; i++ {
		path := filepath.Join(repository, fmt.Sprintf("change-%d.txt", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("change %d\n", i)), 0600); err != nil {
			t.Fatal(err)
		}
		run("git", "add", filepath.Base(path))
		run("git", "commit", "-m", fmt.Sprintf("iteration %d", i))
		head := run("git", "rev-parse", "HEAD")
		recorded = append(recorded, deliveryevidence.Iteration{Sequence: i, Identity: fmt.Sprintf("iteration-%d", i), BaseSHA: previous, HeadSHA: head, EvidenceSHA256: strings.Repeat(fmt.Sprint(i), 64)})
		previous = head
	}
	head := previous
	tree := run("git", "rev-parse", "HEAD^{tree}")
	issue := issueObservation{Number: 279, ID: "I279", Title: "LOCAL gate", Body: "immutable issue", State: "OPEN", Labels: []labelObservation{{Name: "status:approved"}}}
	spec := issueObservation{Number: 275, ID: "I275", Title: "accepted spec", Body: "immutable spec", State: "OPEN", Labels: []labelObservation{{Name: "status:approved"}}}
	issueSHA, err := deliveryevidence.TypedObservationHash("github-issue", "yersonargotev/packy#279:I279", normalizedIssue(issue))
	if err != nil {
		t.Fatal(err)
	}
	specSHA, err := deliveryevidence.TypedObservationHash("github-spec", "yersonargotev/packy#275:I275", normalizedIssue(spec))
	if err != nil {
		t.Fatal(err)
	}
	row := deliveryevidence.AcceptanceRow{Identity: "AC-01", Criterion: "complete local gate", OwningSeam: "delivery evidence", PositiveEvidence: "passing report", NegativeEvidence: "fail closed", FailureEvidence: "distinct diagnostics", MutationEvidence: "read only", CompatibilityEvidence: "additive", PreservationEvidence: "workflow preserved", MigrationEvidence: "N/A: additive", State: deliveryevidence.AcceptanceProved}
	bundle := deliveryevidence.Bundle{
		Schema:           deliveryevidence.SchemaV1,
		Repository:       deliveryevidence.RepositoryIdentity{Owner: "yersonargotev", Name: "packy", NodeID: "R1"},
		Issue:            deliveryevidence.IssueIdentity{Number: 279, NodeID: "I279"},
		Spec:             deliveryevidence.SpecIdentity{Number: 275, NodeID: "I275"},
		Authority:        deliveryevidence.Authority{IssueSHA256: issueSHA, SpecSHA256: specSHA, Labels: []string{"status:approved"}, DependencyDisposition: []deliveryevidence.DependencyDisposition{}, AcceptanceCriteria: []string{"AC-01"}},
		Scope:            deliveryevidence.ScopeLedger{OwnedNow: []deliveryevidence.LedgerEntry{{Identity: "O1", Requirement: "complete local gate", EvidenceLink: "issue#279"}}, Deferred: []deliveryevidence.DeferredEntry{}, Forbidden: []deliveryevidence.LedgerEntry{}, Prerequisites: []deliveryevidence.PrerequisiteEntry{}},
		AcceptanceMatrix: []deliveryevidence.AcceptanceRow{row},
		StartingBaseSHA:  base,
		Iterations:       recorded,
		ReviewReceipts:   []deliveryevidence.ReviewReceipt{},
		Adjudications:    []deliveryevidence.Adjudication{},
	}
	for _, iteration := range recorded {
		for _, axis := range []deliveryevidence.ReviewAxis{deliveryevidence.ReviewStandards, deliveryevidence.ReviewSpec} {
			bundle.ReviewReceipts = append(bundle.ReviewReceipts, deliveryevidence.ReviewReceipt{IssueNumber: 279, Iteration: iteration.Identity, BaseSHA: iteration.BaseSHA, HeadSHA: iteration.HeadSHA, Axis: axis, Findings: []deliveryevidence.ReviewFinding{}})
		}
	}
	resolvedRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	checkoutDigest := sha256.Sum256([]byte(resolvedRepository))
	validatorDigest := sha256.Sum256(validator)
	home := filepath.Join(root, "sandbox-home")
	config := filepath.Join(root, "sandbox-config")
	observation := deliveryevidence.ValidationObservation{
		Repository:                 bundle.Repository,
		CheckoutSHA256:             fmt.Sprintf("%x", checkoutDigest),
		CommitSHA:                  head,
		TreeSHA:                    tree,
		WorkspaceClean:             true,
		ValidatorIdentity:          "scripts/validate-packy.sh",
		ValidatorSHA256:            fmt.Sprintf("%x", validatorDigest),
		ValidatorIdentityExpiresAt: "2026-07-29T00:00:00Z",
		RequiredCommand:            exhaustiveValidationCommand,
		Sandbox:                    deliveryevidence.SandboxFacts{HomeRoot: home, ConfigHomeRoot: config, Sandboxed: true},
	}
	bundle, err = deliveryevidence.RecordExhaustiveValidation(bundle, deliveryevidence.ExhaustiveValidationResult{Observation: observation, CompletedAt: "2026-07-27T12:00:00Z", Succeeded: true, Completed: true})
	if err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(root, "evidence.json")
	if err = deliveryevidence.StoreAtomic(bundlePath, bundle); err != nil {
		t.Fatal(err)
	}
	return localGateFixture{root: root, repository: repository, bundlePath: bundlePath, home: home, config: config, branch: "feat/issue-279-local-delivery-gate", issue: issue, spec: spec, environment: environment}
}

func (f localGateFixture) run(t *testing.T, issue issueObservation, expectedBranch string, extra ...string) (string, error) {
	t.Helper()
	issueRaw, _ := json.Marshal(issue)
	specRaw, _ := json.Marshal(f.spec)
	repositoryRaw := []byte(`{"nameWithOwner":"yersonargotev/packy","id":"R1"}`)
	github := &fakeRunner{outputs: [][]byte{issueRaw, specRaw, repositoryRaw}}
	args := []string{"local-gate", "--bundle", f.bundlePath, "--repository", f.repository, "--delivery-branch", expectedBranch, "--sandbox-home", f.home, "--sandbox-config-home", f.config, "--validator-identity-expires-at", "2026-07-29T00:00:00Z"}
	args = append(args, extra...)
	var stdout bytes.Buffer
	err := (command{Git: environmentRunner{environment: f.environment}, GitHub: github, Now: func() time.Time {
		return time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC)
	}}).run(context.Background(), args, &stdout)
	for _, call := range github.calls {
		if !strings.HasPrefix(call, "gh issue view ") && !strings.HasPrefix(call, "gh repo view ") {
			t.Fatalf("LOCAL gate used mutating or foreign GitHub authority: %s", call)
		}
	}
	return stdout.String(), err
}

func TestLocalGateCommandAtSandboxedHighestSeam(t *testing.T) {
	for _, iterations := range []int{1, 2} {
		t.Run(fmt.Sprintf("%d-iterations", iterations), func(t *testing.T) {
			fixture := newLocalGateFixture(t, iterations)
			before, err := os.ReadFile(fixture.bundlePath)
			if err != nil {
				t.Fatal(err)
			}
			output, err := fixture.run(t, fixture.issue, fixture.branch)
			if err != nil || !strings.Contains(output, "LOCAL delivery gate: PASS") || !strings.Contains(output, fmt.Sprintf("Iterations: %d", iterations)) {
				t.Fatalf("gate did not pass: %q %v", output, err)
			}
			after, err := os.ReadFile(fixture.bundlePath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("read-only gate changed canonical evidence")
			}
		})
	}
}

func TestLocalGateCommandFailureClasses(t *testing.T) {
	t.Run("required-arguments", func(t *testing.T) {
		var output bytes.Buffer
		err := (command{}).run(context.Background(), []string{"local-gate"}, &output)
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateQualificationInvalid)) || !strings.Contains(output.String(), "LOCAL delivery gate: FAIL") {
			t.Fatalf("unexpected result: %q %v", output.String(), err)
		}
	})
	t.Run("authority-observation", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		github := &fakeRunner{outputs: [][]byte{[]byte("{}")}, failAt: 1}
		args := []string{"local-gate", "--bundle", f.bundlePath, "--repository", f.repository, "--delivery-branch", f.branch, "--sandbox-home", f.home, "--sandbox-config-home", f.config, "--validator-identity-expires-at", "2026-07-29T00:00:00Z"}
		var output bytes.Buffer
		err := (command{Git: environmentRunner{environment: f.environment}, GitHub: github}).run(context.Background(), args, &output)
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateTrackerAuthorityChanged)) || !strings.Contains(output.String(), "Issue: #279") {
			t.Fatalf("unexpected result: %q %v", output.String(), err)
		}
	})
	t.Run("missing-acceptance-row", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		b, _, _ := deliveryevidence.Load(f.bundlePath)
		b.AcceptanceMatrix = []deliveryevidence.AcceptanceRow{}
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(f.bundlePath, raw, 0600); err != nil {
			t.Fatal(err)
		}
		output, err := f.run(t, f.issue, f.branch)
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateQualificationInvalid)) || !strings.Contains(err.Error(), "missing acceptance matrix row AC-01") || !strings.Contains(output, "LOCAL delivery gate: FAIL") {
			t.Fatalf("unexpected result: %q %v", output, err)
		}
	})
	t.Run("tracker-authority-changed", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		changed := f.issue
		changed.Body = "changed"
		output, err := f.run(t, changed, f.branch)
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateTrackerAuthorityChanged)) || !strings.Contains(output, "Issue: #279") {
			t.Fatalf("unexpected result: %q %v", output, err)
		}
	})
	t.Run("acceptance-unproved", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		b, _, _ := deliveryevidence.Load(f.bundlePath)
		b.AcceptanceMatrix[0].State = deliveryevidence.AcceptancePlanned
		if err := deliveryevidence.StoreAtomic(f.bundlePath, b); err != nil {
			t.Fatal(err)
		}
		_, err := f.run(t, f.issue, f.branch)
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateAcceptanceUnproved)) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("review-gap", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		b, _, _ := deliveryevidence.Load(f.bundlePath)
		b.ReviewReceipts = b.ReviewReceipts[:1]
		if err := deliveryevidence.StoreAtomic(f.bundlePath, b); err != nil {
			t.Fatal(err)
		}
		_, err := f.run(t, f.issue, f.branch)
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateReviewGap)) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("unresolved-findings", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		b, _, _ := deliveryevidence.Load(f.bundlePath)
		axis := b.ReviewReceipts[0].Axis
		authority := deliveryevidence.AuthorityDocumentedStandard
		if axis == deliveryevidence.ReviewSpec {
			authority = deliveryevidence.AuthoritySpecRequirement
		}
		finding := deliveryevidence.ReviewFinding{ID: "FINDING-1", Axis: axis, Severity: deliveryevidence.SeverityP1, Authority: authority, Citation: "issue#279:AC-07", Location: "gate.go", Evidence: "repair required"}
		b.ReviewReceipts[0].Findings = []deliveryevidence.ReviewFinding{finding}
		b.Adjudications = []deliveryevidence.Adjudication{{Sequence: 1, FindingID: finding.ID, Disposition: deliveryevidence.DispositionAccepted, Evidence: "owned repair", RepairIteration: "iteration-2"}}
		if err := deliveryevidence.StoreAtomic(f.bundlePath, b); err != nil {
			t.Fatal(err)
		}
		_, err := f.run(t, f.issue, f.branch)
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateUnresolvedFindings)) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("stale-validation", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		_, err := f.run(t, f.issue, f.branch, "--required-command", "./scripts/validate-packy.sh --changed")
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateStaleValidation)) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("dirty-workspace", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		if err := os.WriteFile(filepath.Join(f.repository, "dirty.txt"), []byte("dirty"), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := f.run(t, f.issue, f.branch)
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateDirtyWorkspace)) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("wrong-branch", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		_, err := f.run(t, f.issue, "feat/issue-279-other")
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateWrongBranch)) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("self-asserted-main", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		_, err := f.run(t, f.issue, "main")
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateWrongBranch)) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("foreign-evidence", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		foreign := f.issue
		foreign.ID = "I-foreign"
		_, err := f.run(t, foreign, f.branch)
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateForeignEvidence)) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("unrecorded-delta", func(t *testing.T) {
		f := newLocalGateFixture(t, 1)
		cmd := exec.Command("git", "commit", "--allow-empty", "-m", "extra")
		cmd.Dir = f.repository
		cmd.Env = f.environment
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("extra commit: %v\n%s", err, out)
		}
		_, err := f.run(t, f.issue, f.branch)
		if err == nil || !strings.Contains(err.Error(), string(deliveryevidence.LocalGateUnrecordedDelta)) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func reviewBundleFixture(base string) deliveryevidence.Bundle {
	row := deliveryevidence.AcceptanceRow{Identity: "AC-01", Criterion: "structured review receipts", OwningSeam: "delivery evidence", PositiveEvidence: "paired receipts", NegativeEvidence: "foreign receipts rejected", FailureEvidence: "stale receipts rejected", MutationEvidence: "evidence file only", CompatibilityEvidence: "canonical schema", PreservationEvidence: "scope preserved", MigrationEvidence: "N/A: additive", State: deliveryevidence.AcceptancePlanned}
	return deliveryevidence.Bundle{
		Schema:             deliveryevidence.SchemaV1,
		Repository:         deliveryevidence.RepositoryIdentity{Owner: "yersonargotev", Name: "packy", NodeID: "R1"},
		Issue:              deliveryevidence.IssueIdentity{Number: 277, NodeID: "I277"},
		Spec:               deliveryevidence.SpecIdentity{Number: 275, NodeID: "I275"},
		Authority:          deliveryevidence.Authority{IssueSHA256: strings.Repeat("d", 64), SpecSHA256: strings.Repeat("e", 64), Labels: []string{"status:approved"}, DependencyDisposition: []deliveryevidence.DependencyDisposition{{Identity: "#276", Disposition: deliveryevidence.DependencySatisfied}}, AcceptanceCriteria: []string{"AC-01"}},
		Scope:              deliveryevidence.ScopeLedger{OwnedNow: []deliveryevidence.LedgerEntry{{Identity: "O1", Requirement: "reviews", EvidenceLink: "issue#277"}}, Deferred: []deliveryevidence.DeferredEntry{}, Forbidden: []deliveryevidence.LedgerEntry{}, Prerequisites: []deliveryevidence.PrerequisiteEntry{}},
		AcceptanceMatrix:   []deliveryevidence.AcceptanceRow{row},
		StartingBaseSHA:    base,
		Iterations:         []deliveryevidence.Iteration{},
		ReviewReceipts:     []deliveryevidence.ReviewReceipt{},
		Adjudications:      []deliveryevidence.Adjudication{},
		ValidationReceipts: []deliveryevidence.ValidationReceipt{},
		FocusedValidation:  []deliveryevidence.FocusedValidationEvidence{},
	}
}

func TestNonLocalCommandsReadOnlyGreenAndRed(t *testing.T) {
	root := t.TempDir()
	base, head, tree := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	bundle := reviewBundleFixture(base)
	bundle.AcceptanceMatrix[0].State = deliveryevidence.AcceptanceProved
	bundle.Iterations = []deliveryevidence.Iteration{{Sequence: 1, Identity: "iteration-1", BaseSHA: base, HeadSHA: head, EvidenceSHA256: strings.Repeat("1", 64)}}
	bundlePath := filepath.Join(root, "bundle.json")
	if err := deliveryevidence.StoreAtomic(bundlePath, bundle); err != nil {
		t.Fatal(err)
	}
	digest, err := deliveryevidence.Digest(bundle)
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) string {
		t.Helper()
		path := filepath.Join(root, name+".json")
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	local := deliveryevidence.LocalGateReport{
		Repository: bundle.Repository, Issue: bundle.Issue, Spec: bundle.Spec,
		Branch: "feat/issue-277-non-local", StartingBaseSHA: base, HeadSHA: head,
		TreeSHA: tree, AcceptanceProved: 1, ValidationCompletedAt: "2026-07-27T11:59:00Z",
		BundleSHA256: digest,
	}
	pr := deliveryevidence.PullRequestObservation{
		Repository: bundle.Repository, Issue: bundle.Issue, Spec: bundle.Spec,
		IssueSHA256: bundle.Authority.IssueSHA256, SpecSHA256: bundle.Authority.SpecSHA256,
		IssueEligible: true, SpecEligible: true, Branch: local.Branch, Number: 7,
		URL: "https://github.com/yersonargotev/packy/pull/7", HeadSHA: head, BaseSHA: base,
		Mergeability: "mergeable", Required: []deliveryevidence.RequiredCheck{{Identity: "validate", Conclusion: "success", HeadSHA: head}},
		ObservedAt: "2026-07-27T12:00:00Z", Available: true,
	}
	before, _ := os.ReadFile(bundlePath)
	var stdout bytes.Buffer
	args := []string{"non-local-readiness", "--bundle", bundlePath, "--local-report", write("local", local), "--observation", write("pr", pr), "--required-checks", "validate"}
	if err := (command{}).run(context.Background(), args, &stdout); err != nil || !strings.Contains(stdout.String(), `"ready": true`) {
		t.Fatalf("green readiness: %s %v", stdout.String(), err)
	}
	var readiness deliveryevidence.ReadinessReport
	if err := json.Unmarshal(stdout.Bytes(), &readiness); err != nil {
		t.Fatal(err)
	}
	readinessPath := write("readiness", readiness)
	pr.HeadSHA = strings.Repeat("d", 40)
	stdout.Reset()
	args[6] = write("stale-pr", pr)
	err = (command{}).run(context.Background(), args, &stdout)
	var readinessErr *deliveryevidence.ReadinessError
	if !errors.As(err, &readinessErr) || readinessErr.Code != deliveryevidence.ReadinessStaleHead {
		t.Fatalf("stale readiness classification: %v", err)
	}
	outcome := deliveryevidence.FinalOutcomeObservation{
		Repository: bundle.Repository, Issue: bundle.Issue, PullRequest: 7,
		PullRequestURL: "https://github.com/yersonargotev/packy/pull/7", PullRequestHeadSHA: head, MergeCommitSHA: head,
		Merged: true, MergeContainedOnMain: true, IssueClosed: true, RemoteBranchAbsent: true,
		LocalMainSHA: head, OriginMainSHA: head, LocalBranchAbsent: true, WorktreeClean: true,
		PreservedStateBefore: strings.Repeat("2", 64), PreservedStateAfter: strings.Repeat("2", 64),
		ObservedAt: "2026-07-27T12:05:00Z", MatrixURL: "https://e/matrix", ReviewsURL: "https://e/reviews",
		ValidationURL: "https://e/validation", CIURL: "https://e/ci", CleanupURL: "https://e/cleanup",
	}
	var receipts []deliveryevidence.PhaseReceipt
	for _, phase := range []string{"qualification", "implementation", "review", "validation", "ci", "merge", "cleanup"} {
		receipts = append(receipts, deliveryevidence.PhaseReceipt{Phase: phase, StartedAt: "2026-07-27T12:00:00Z", CompletedAt: "2026-07-27T12:05:00Z"})
	}
	stdout.Reset()
	if err = (command{}).run(context.Background(), []string{"final-outcome", "--bundle", bundlePath, "--readiness-report", readinessPath, "--observation", write("outcome", outcome), "--phase-receipts", write("receipts", receipts)}, &stdout); err != nil || !strings.Contains(stdout.String(), `"successful": true`) {
		t.Fatalf("green outcome: %s %v", stdout.String(), err)
	}
	staleReadiness := readiness
	staleReadiness.HeadSHA = strings.Repeat("f", 40)
	err = (command{}).run(context.Background(), []string{"final-outcome", "--bundle", bundlePath, "--readiness-report", write("stale-readiness", staleReadiness), "--observation", write("outcome-stale", outcome), "--phase-receipts", write("receipts-stale", receipts)}, io.Discard)
	var staleOutcome *deliveryevidence.OutcomeError
	if !errors.As(err, &staleOutcome) || staleOutcome.Code != deliveryevidence.OutcomeUnavailable {
		t.Fatalf("stale readiness outcome classification: %v", err)
	}
	outcome.WorktreeClean = false
	err = (command{}).run(context.Background(), []string{"final-outcome", "--bundle", bundlePath, "--readiness-report", readinessPath, "--observation", write("dirty", outcome), "--phase-receipts", write("receipts-2", receipts)}, io.Discard)
	var outcomeErr *deliveryevidence.OutcomeError
	if !errors.As(err, &outcomeErr) || outcomeErr.Code != deliveryevidence.OutcomeDirty {
		t.Fatalf("dirty outcome classification: %v", err)
	}
	after, _ := os.ReadFile(bundlePath)
	if !bytes.Equal(before, after) {
		t.Fatal("read-only NON-LOCAL commands changed canonical evidence")
	}
}
