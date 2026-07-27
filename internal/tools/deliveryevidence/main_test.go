package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

type fakeRunner struct {
	outputs [][]byte
	calls   []string
	failAt  int
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
	if _, err := run("record-review", "--bundle", bundlePath, "--receipt", write("standards-1", standards)); err != nil {
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
	if _, err := run("record-review", "--bundle", bundlePath, "--receipt", write("stale", stale)); err == nil || !strings.Contains(err.Error(), "exact delta") {
		t.Fatalf("stale receipt accepted: %v", err)
	}
	after, _ := os.ReadFile(bundlePath)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected stale receipt changed canonical evidence")
	}

	specFinding := deliveryevidence.ReviewFinding{ID: "SPEC-1", Axis: deliveryevidence.ReviewSpec, Severity: deliveryevidence.SeverityP1, Authority: deliveryevidence.AuthoritySpecRequirement, Citation: "issue#277:AC-05", Location: "internal/deliveryevidence/review.go", Evidence: "repair pairing is missing"}
	rejectedFinding := deliveryevidence.ReviewFinding{ID: "SPEC-2", Axis: deliveryevidence.ReviewSpec, Severity: deliveryevidence.SeverityP3, Authority: deliveryevidence.AuthoritySpecRequirement, Citation: "issue#277:AC-10", Location: "internal/tools/deliveryevidence/main.go", Evidence: "mutation concern is not reachable"}
	spec := deliveryevidence.ReviewReceipt{IssueNumber: 277, Iteration: "iteration-1", BaseSHA: base, HeadSHA: head1, Axis: deliveryevidence.ReviewSpec, Findings: []deliveryevidence.ReviewFinding{specFinding, rejectedFinding}}
	if _, err := run("record-review", "--bundle", bundlePath, "--receipt", write("spec-1", spec)); err != nil {
		t.Fatal(err)
	}
	iteration2 := deliveryevidence.Iteration{Sequence: 2, Identity: "iteration-2", BaseSHA: head1, HeadSHA: head2, EvidenceSHA256: strings.Repeat("2", 64)}
	if _, err := run("record-iteration", "--bundle", bundlePath, "--iteration", write("iteration-2", iteration2)); err != nil {
		t.Fatal(err)
	}
	for _, axis := range []deliveryevidence.ReviewAxis{deliveryevidence.ReviewStandards, deliveryevidence.ReviewSpec} {
		receipt := deliveryevidence.ReviewReceipt{IssueNumber: 277, Iteration: "iteration-2", BaseSHA: head1, HeadSHA: head2, Axis: axis, Findings: []deliveryevidence.ReviewFinding{}}
		if _, err := run("record-review", "--bundle", bundlePath, "--receipt", write(string(axis)+"-2", receipt)); err != nil {
			t.Fatal(err)
		}
	}
	adjudications := []deliveryevidence.Adjudication{
		{Sequence: 1, FindingID: "SPEC-1", Disposition: deliveryevidence.DispositionAccepted, Evidence: "repair is owned by iteration-2", RepairIteration: "iteration-2"},
		{Sequence: 2, FindingID: "SPEC-1", Disposition: deliveryevidence.DispositionRepairedByLaterIteration, Evidence: "paired review proves the repair", RepairIteration: "iteration-2"},
		{Sequence: 3, FindingID: "SPEC-2", Disposition: deliveryevidence.DispositionRejectedWithEvidence, Evidence: "the private adapter exposes no GitHub mutation"},
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

func reviewBundleFixture(base string) deliveryevidence.Bundle {
	row := deliveryevidence.AcceptanceRow{Identity: "AC-01", Criterion: "structured review receipts", OwningSeam: "delivery evidence", PositiveEvidence: "paired receipts", NegativeEvidence: "foreign receipts rejected", FailureEvidence: "stale receipts rejected", MutationEvidence: "evidence file only", CompatibilityEvidence: "canonical schema", PreservationEvidence: "scope preserved", MigrationEvidence: "N/A: additive", State: deliveryevidence.AcceptancePlanned}
	return deliveryevidence.Bundle{
		Schema:           deliveryevidence.SchemaV1,
		Repository:       deliveryevidence.RepositoryIdentity{Owner: "yersonargotev", Name: "packy", NodeID: "R1"},
		Issue:            deliveryevidence.IssueIdentity{Number: 277, NodeID: "I277"},
		Spec:             deliveryevidence.SpecIdentity{Number: 275, NodeID: "I275"},
		Authority:        deliveryevidence.Authority{IssueSHA256: strings.Repeat("d", 64), SpecSHA256: strings.Repeat("e", 64), Labels: []string{"status:approved"}, DependencyDisposition: []deliveryevidence.DependencyDisposition{{Identity: "#276", Disposition: deliveryevidence.DependencySatisfied}}, AcceptanceCriteria: []string{"AC-01"}},
		Scope:            deliveryevidence.ScopeLedger{OwnedNow: []deliveryevidence.LedgerEntry{{Identity: "O1", Requirement: "reviews", EvidenceLink: "issue#277"}}, Deferred: []deliveryevidence.DeferredEntry{}, Forbidden: []deliveryevidence.LedgerEntry{}, Prerequisites: []deliveryevidence.PrerequisiteEntry{}},
		AcceptanceMatrix: []deliveryevidence.AcceptanceRow{row},
		StartingBaseSHA:  base,
		Iterations:       []deliveryevidence.Iteration{},
		ReviewReceipts:   []deliveryevidence.ReviewReceipt{},
		Adjudications:    []deliveryevidence.Adjudication{},
	}
}
