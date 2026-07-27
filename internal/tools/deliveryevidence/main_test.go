package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
