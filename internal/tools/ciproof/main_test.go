package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/yersonargotev/packy/internal/testprocess"
	"strings"
	"testing"
	"time"
)

const testSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const validYAML = "jobs:\n  validate:\n    name: Validate Packy-owned code\n    steps:\n      - name: Run CI validation\n        run: ./scripts/validate-packy.sh --ci\n"

type fixture struct {
	runs      []run
	direct    run
	attempt   run
	jobs      []job
	source    string
	repo      repository
	workflow  workflow
	fail      string
	malformed string
	reads     int
	race      bool
}

func newFixture() *fixture {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := repository{canonical.repositoryID, canonical.repository}
	r := run{ID: 10, WorkflowID: canonical.workflowID, Path: canonical.workflowPath, HeadSHA: testSHA, HeadBranch: "main", Event: "push", Status: "completed", Conclusion: "success", Attempt: 1, CreatedAt: now, Repository: repo, HeadRepository: repo}
	j := job{ID: 20, RunID: 10, RunAttempt: 1, HeadSHA: testSHA, HeadBranch: "main", Name: "Validate Packy-owned code", Status: "completed", Conclusion: "success", Steps: []step{{Name: "Run CI validation", Number: 3, Status: "completed", Conclusion: "success", StartedAt: &now, CompletedAt: &now}}}
	return &fixture{runs: []run{r}, direct: r, attempt: r, jobs: []job{j}, source: validYAML, repo: repo, workflow: workflow{canonical.workflowID, canonical.workflowPath, "active"}}
}
func (f *fixture) read(_ context.Context, path string) ([]byte, error) {
	if f.fail != "" && strings.Contains(path, f.fail) {
		return nil, errors.New("API failure")
	}
	if f.malformed != "" && strings.Contains(path, f.malformed) {
		return []byte("{"), nil
	}
	var out any
	switch {
	case path == "repos/"+canonical.repository:
		out = f.repo
	case strings.Contains(path, "/contents/"):
		blob := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(f.source), f.source)))
		out = map[string]any{"type": "file", "path": canonical.workflowPath, "sha": fmt.Sprintf("%x", blob), "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(f.source))}
	case strings.Contains(path, "/jobs?"):
		page := 1
		fmt.Sscanf(path[strings.LastIndex(path, "page="):], "page=%d", &page)
		start := (page - 1) * 100
		end := min(start+100, len(f.jobs))
		if start > len(f.jobs) {
			start = len(f.jobs)
		}
		out = map[string]any{"total_count": len(f.jobs), "jobs": f.jobs[start:end]}
	case strings.Contains(path, "/attempts/"):
		out = f.attempt
	case strings.Contains(path, "/runs?"):
		f.reads++
		runs := f.runs
		if f.race && f.reads > 1 {
			runs = append(append([]run{}, runs...), f.direct)
			runs[len(runs)-1].ID++
		}
		page := 1
		fmt.Sscanf(path[strings.LastIndex(path, "page="):], "page=%d", &page)
		start := (page - 1) * 100
		end := min(start+100, len(runs))
		if start > len(runs) {
			start = len(runs)
		}
		out = map[string]any{"total_count": len(runs), "workflow_runs": runs[start:end]}
	case strings.Contains(path, "/runs/"):
		out = f.direct
	case strings.Contains(path, "/actions/workflows/"):
		out = f.workflow
	default:
		return nil, fmt.Errorf("unexpected path %s", path)
	}
	return json.Marshal(out)
}
func TestProofAcceptsTrustedExactCommit(t *testing.T) {
	f := newFixture()
	p, err := verify(context.Background(), testSHA, f.read)
	if err != nil {
		t.Fatal(err)
	}
	if p.RunID != 10 || p.Attempt != 1 || p.JobID != 20 || !fullSHA.MatchString(p.WorkflowBlobSHA) || p.StepNumber != 3 || !strings.Contains(p.RunURL, "/runs/10") {
		t.Fatalf("unexpected proof: %+v", p)
	}
}
func TestProofRejectsUntrustedEvidence(t *testing.T) {
	cases := map[string]func(*fixture){
		"absent":          func(f *fixture) { f.runs = []run{} },
		"pending":         func(f *fixture) { f.direct.Status = "in_progress" },
		"failed":          func(f *fixture) { f.direct.Conclusion = "failure" },
		"cancelled":       func(f *fixture) { f.direct.Conclusion = "cancelled" },
		"skipped step":    func(f *fixture) { f.jobs[0].Steps[0].Conclusion = "skipped" },
		"repository":      func(f *fixture) { f.repo.ID++ },
		"workflow":        func(f *fixture) { f.workflow.ID++ },
		"run workflow":    func(f *fixture) { f.runs[0].WorkflowID++ },
		"wrong sha":       func(f *fixture) { f.runs[0].HeadSHA = testSHA[:39] + "b" },
		"attempt":         func(f *fixture) { f.attempt.Attempt++ },
		"failed attempt":  func(f *fixture) { f.attempt.Conclusion = "failure" },
		"job identity":    func(f *fixture) { f.jobs[0].RunID++ },
		"selective rerun": func(f *fixture) { f.jobs = []job{} },
		"api failure":     func(f *fixture) { f.fail = "/contents/" },
		"malformed":       func(f *fixture) { f.malformed = "/contents/" },
		"missing invocation": func(f *fixture) {
			f.source = strings.ReplaceAll(f.source, "./scripts/validate-packy.sh --ci", "echo ok")
		},
		"masked invocation": func(f *fixture) { f.source = strings.ReplaceAll(f.source, "--ci", "--ci || true") },
		"step environment":  func(f *fixture) { f.source += "        env:\n          BASH_ENV: /tmp/mask\n" },
		"continue on error": func(f *fixture) { f.source += "        continue-on-error: true\n" },
		"newer failed": func(f *fixture) {
			r := f.direct
			r.ID++
			r.CreatedAt = r.CreatedAt.Add(time.Second)
			r.Conclusion = "failure"
			f.runs = append(f.runs, r)
		},
		"race":           func(f *fixture) { f.race = true },
		"duplicate runs": func(f *fixture) { f.runs = append(f.runs, f.runs[0]) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			mutate(f)
			if _, err := verify(context.Background(), testSHA, f.read); err == nil {
				t.Fatal("accepted invalid proof")
			}
		})
	}
}
func TestProofReadsEveryPage(t *testing.T) {
	f := newFixture()
	newest := f.runs[0]
	newest.CreatedAt = newest.CreatedAt.Add(time.Hour)
	newest.ID = 200
	f.direct = newest
	f.attempt = newest
	f.jobs[0].RunID = 200
	f.runs = nil
	for i := int64(1); i <= 100; i++ {
		r := newest
		r.ID = i
		r.CreatedAt = r.CreatedAt.Add(-time.Hour)
		f.runs = append(f.runs, r)
	}
	f.runs = append(f.runs, newest)
	for i := int64(21); i < 121; i++ {
		f.jobs = append(f.jobs, job{ID: i, Name: "other"})
	}
	f.jobs = append(f.jobs[1:], f.jobs[0])
	p, err := verify(context.Background(), testSHA, f.read)
	if err != nil || p.RunID != 200 {
		t.Fatalf("proof %+v: %v", p, err)
	}
}

func TestCommandUsesReadOnlyGitHubTransport(t *testing.T) {
	dir := t.TempDir()
	f := newFixture()
	responses := map[string][]byte{}
	_, err := verify(context.Background(), testSHA, func(ctx context.Context, path string) ([]byte, error) {
		b, e := f.read(ctx, path)
		responses[path] = b
		return b, e
	})
	if err != nil {
		t.Fatal(err)
	}
	var script strings.Builder
	script.WriteString("#!/bin/sh\n[ \"$1 $2 $3 $4 $5\" = 'api --hostname github.com --method GET' ] || exit 19\nfor last do :; done\ncase \"$last\" in\n")
	i := 0
	for path, b := range responses {
		i++
		file := filepath.Join(dir, fmt.Sprintf("response-%d", i))
		if err := os.WriteFile(file, b, 0600); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&script, "'%s') cat '%s';;\n", path, file)
	}
	script.WriteString("*) exit 20;;\nesac\n")
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script.String()), 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "ciproof")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Env = testprocess.GoOfflineEnv(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	run := func(sha string) ([]byte, string, error) {
		cmd := exec.Command(binary, "--sha", sha)
		cmd.Env = testprocess.Env(t, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		return out, stderr.String(), err
	}
	out, stderr, err := run(testSHA)
	if err != nil {
		t.Fatalf("command: %v: %s", err, stderr)
	}
	var p proof
	if err := json.Unmarshal(out, &p); err != nil || p.RunID != 10 || !strings.Contains(stderr, "Accepted main CI proof") {
		t.Fatalf("stdout %q stderr %q error %v", out, stderr, err)
	}
	out, stderr, err = run(strings.Repeat("b", 40))
	if err == nil || len(out) != 0 || !strings.Contains(stderr, "CI proof rejected") || !strings.Contains(stderr, "rerun all jobs") {
		t.Fatalf("stdout %q stderr %q error %v", out, stderr, err)
	}
}

func TestProofRejectsIncompleteResponses(t *testing.T) {
	for name, rewrite := range map[string]func(string, []byte) []byte{
		"truncated runs": func(path string, b []byte) []byte {
			if strings.Contains(path, "/runs?") {
				return []byte(`{"total_count":2,"workflow_runs":[]}`)
			}
			return b
		},
		"run ceiling": func(path string, b []byte) []byte {
			if strings.Contains(path, "/runs?") {
				return []byte(`{"total_count":1000,"workflow_runs":[]}`)
			}
			return b
		},
		"truncated jobs": func(path string, b []byte) []byte {
			if strings.Contains(path, "/jobs?") {
				return []byte(`{"total_count":2,"jobs":[]}`)
			}
			return b
		},
		"blob mismatch": func(path string, b []byte) []byte {
			if strings.Contains(path, "/contents/") {
				var v map[string]any
				_ = json.Unmarshal(b, &v)
				v["sha"] = testSHA
				b, _ = json.Marshal(v)
			}
			return b
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			_, err := verify(context.Background(), testSHA, func(ctx context.Context, path string) ([]byte, error) {
				b, e := f.read(ctx, path)
				return rewrite(path, b), e
			})
			if err == nil {
				t.Fatal("accepted incomplete response")
			}
		})
	}
}

func TestNewerAttemptMustSucceed(t *testing.T) {
	f := newFixture()
	f.runs[0].Attempt = 2
	f.direct.Attempt = 2
	f.attempt.Attempt = 2
	f.attempt.Conclusion = "failure"
	if _, err := verify(context.Background(), testSHA, f.read); err == nil {
		t.Fatal("accepted failed latest attempt")
	}
}
