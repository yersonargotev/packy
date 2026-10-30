package main

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type authority struct {
	repository   string
	repositoryID int64
	workflowPath string
	workflowID   int64
}

var canonical = authority{"yersonargotev/packy", 1290275684, ".github/workflows/ci.yml", 307623507}
var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

type proof struct {
	Repository      string    `json:"repository"`
	RepositoryID    int64     `json:"repository_id"`
	WorkflowPath    string    `json:"workflow_path"`
	WorkflowID      int64     `json:"workflow_id"`
	WorkflowBlobSHA string    `json:"workflow_blob_sha"`
	SourceSHA       string    `json:"source_sha"`
	Event           string    `json:"event"`
	Branch          string    `json:"branch"`
	RunID           int64     `json:"run_id"`
	RunCreatedAt    time.Time `json:"run_created_at"`
	Attempt         int       `json:"attempt"`
	JobID           int64     `json:"job_id"`
	JobName         string    `json:"job_name"`
	StepNumber      int       `json:"step_number"`
	StepName        string    `json:"step_name"`
	RunURL          string    `json:"run_url"`
	AttemptURL      string    `json:"attempt_url"`
	JobURL          string    `json:"job_url"`
	ObservedAt      time.Time `json:"observed_at"`
}

type repository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}
type workflow struct {
	ID    int64  `json:"id"`
	Path  string `json:"path"`
	State string `json:"state"`
}
type run struct {
	ID             int64      `json:"id"`
	WorkflowID     int64      `json:"workflow_id"`
	Path           string     `json:"path"`
	HeadSHA        string     `json:"head_sha"`
	HeadBranch     string     `json:"head_branch"`
	Event          string     `json:"event"`
	Status         string     `json:"status"`
	Conclusion     string     `json:"conclusion"`
	Attempt        int        `json:"run_attempt"`
	CreatedAt      time.Time  `json:"created_at"`
	Repository     repository `json:"repository"`
	HeadRepository repository `json:"head_repository"`
}
type step struct {
	Name        string     `json:"name"`
	Number      int        `json:"number"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}
type job struct {
	ID         int64  `json:"id"`
	RunID      int64  `json:"run_id"`
	RunAttempt int    `json:"run_attempt"`
	HeadSHA    string `json:"head_sha"`
	HeadBranch string `json:"head_branch"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Steps      []step `json:"steps"`
}
type readAPI func(context.Context, string) ([]byte, error)

func main() {
	sha := flag.String("sha", "", "full source commit SHA")
	flag.Parse()
	if flag.NArg() != 0 || !fullSHA.MatchString(*sha) {
		fmt.Fprintln(os.Stderr, "provide --sha with a full lowercase 40-character commit SHA")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p, err := verify(ctx, *sha, githubRead)
	if err != nil {
		fmt.Fprintf(os.Stderr, "CI proof rejected for %s: %v. Restore API access or rerun all jobs of canonical main CI and wait for success.\n", *sha, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(p); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Accepted main CI proof: %s (attempt %d)\n", p.RunURL, p.Attempt)
}

func githubRead(ctx context.Context, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", "api", "--hostname", "github.com", "--method", "GET", "-H", "X-GitHub-Api-Version: 2022-11-28", path)
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return nil, fmt.Errorf("GitHub API %s: %w: %s", path, err, strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("GitHub API %s: %w", path, err)
	}
	return out, nil
}

func get(ctx context.Context, read readAPI, path string, dst any) error {
	b, err := read(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("malformed GitHub response for %s: %w", path, err)
	}
	return nil
}

func repoOK(r repository) bool {
	return r.ID == canonical.repositoryID && r.FullName == canonical.repository
}
func runOK(r run, sha string) bool {
	return r.ID > 0 && r.WorkflowID == canonical.workflowID && r.Path == canonical.workflowPath && r.HeadSHA == sha && r.HeadBranch == "main" && r.Event == "push" && r.Attempt > 0 && !r.CreatedAt.IsZero() && repoOK(r.Repository) && repoOK(r.HeadRepository)
}
func success(status, conclusion string) bool { return status == "completed" && conclusion == "success" }

func verify(ctx context.Context, sha string, read readAPI) (p proof, resultErr error) {
	if !fullSHA.MatchString(sha) {
		return p, errors.New("invalid source SHA")
	}
	base := "repos/" + canonical.repository
	var repo repository
	if err := get(ctx, read, base, &repo); err != nil {
		return p, err
	}
	if !repoOK(repo) {
		return p, errors.New("canonical repository identity mismatch")
	}
	var wf workflow
	if err := get(ctx, read, fmt.Sprintf("%s/actions/workflows/%d", base, canonical.workflowID), &wf); err != nil {
		return p, err
	}
	if wf.ID != canonical.workflowID || wf.Path != canonical.workflowPath || wf.State != "active" {
		return p, errors.New("canonical workflow identity mismatch or inactive workflow")
	}
	var file struct {
		Type     string `json:"type"`
		Path     string `json:"path"`
		SHA      string `json:"sha"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := get(ctx, read, base+"/contents/"+canonical.workflowPath+"?ref="+sha, &file); err != nil {
		return p, err
	}
	if file.Type != "file" || file.Path != canonical.workflowPath || !fullSHA.MatchString(file.SHA) || file.Encoding != "base64" {
		return p, errors.New("invalid source workflow file identity")
	}
	content, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
	if err != nil {
		return p, fmt.Errorf("invalid source workflow content: %w", err)
	}
	blob := sha1.New()
	fmt.Fprintf(blob, "blob %d\x00", len(content))
	_, _ = blob.Write(content)
	if fmt.Sprintf("%x", blob.Sum(nil)) != file.SHA {
		return p, errors.New("source workflow blob content does not match its identity")
	}
	jobName, stepName, err := validatorIdentity(content)
	if err != nil {
		return p, err
	}
	selected, err := newestRun(ctx, read, sha)
	if err != nil {
		return p, err
	}
	path := fmt.Sprintf("%s/actions/runs/%d", base, selected.ID)
	var latest, attempt run
	defer func() {
		if resultErr != nil {
			resultErr = fmt.Errorf("run %d selected attempt %d (%s/%s), latest attempt %d (%s/%s), numbered attempt %d (%s/%s): %w", selected.ID, selected.Attempt, selected.Status, selected.Conclusion, latest.Attempt, latest.Status, latest.Conclusion, attempt.Attempt, attempt.Status, attempt.Conclusion, resultErr)
		}
	}()
	if err := get(ctx, read, path, &latest); err != nil {
		return p, err
	}
	if !sameRun(selected, latest, sha) || !success(latest.Status, latest.Conclusion) {
		return p, fmt.Errorf("newest run %d is changed or not completed successfully", selected.ID)
	}
	if err := get(ctx, read, fmt.Sprintf("%s/attempts/%d", path, latest.Attempt), &attempt); err != nil {
		return p, err
	}
	if !sameRun(latest, attempt, sha) || !success(attempt.Status, attempt.Conclusion) {
		return p, fmt.Errorf("run %d attempt %d is not a matching successful attempt", latest.ID, latest.Attempt)
	}
	jobs, err := listJobs(ctx, read, path, latest.Attempt)
	if err != nil {
		return p, err
	}
	var found []job
	for _, j := range jobs {
		if j.Name == jobName {
			found = append(found, j)
		}
	}
	if len(found) != 1 {
		return p, fmt.Errorf("run %d attempt %d has %d source jobs; rerun all jobs", latest.ID, latest.Attempt, len(found))
	}
	j := found[0]
	if j.ID <= 0 || j.RunID != latest.ID || j.RunAttempt != latest.Attempt || j.HeadSHA != sha || j.HeadBranch != "main" || !success(j.Status, j.Conclusion) {
		return p, errors.New("source job identity or result mismatch")
	}
	var matched []step
	for _, s := range j.Steps {
		if s.Name == stepName {
			matched = append(matched, s)
		}
	}
	if len(matched) != 1 {
		return p, errors.New("source validator step missing or ambiguous")
	}
	s := matched[0]
	if s.Number <= 0 || !success(s.Status, s.Conclusion) || s.StartedAt == nil || s.CompletedAt == nil || s.StartedAt.IsZero() || s.CompletedAt.IsZero() || s.CompletedAt.Before(*s.StartedAt) {
		return p, errors.New("source validator step did not complete successfully")
	}
	again, err := newestRun(ctx, read, sha)
	if err != nil {
		return p, err
	}
	var final run
	if err := get(ctx, read, path, &final); err != nil {
		return p, err
	}
	if !sameRun(latest, again, sha) || !sameRun(latest, final, sha) || !success(again.Status, again.Conclusion) || !success(final.Status, final.Conclusion) {
		return p, errors.New("newest run or latest attempt changed during verification; retry")
	}
	url := fmt.Sprintf("https://github.com/%s/actions/runs/%d", canonical.repository, latest.ID)
	return proof{
		Repository: canonical.repository, RepositoryID: canonical.repositoryID,
		WorkflowPath: canonical.workflowPath, WorkflowID: canonical.workflowID,
		WorkflowBlobSHA: file.SHA, SourceSHA: sha, Event: "push", Branch: "main",
		RunID: latest.ID, RunCreatedAt: latest.CreatedAt, Attempt: latest.Attempt,
		JobID: j.ID, JobName: j.Name, StepNumber: s.Number, StepName: s.Name,
		RunURL: url, AttemptURL: fmt.Sprintf("%s/attempts/%d", url, latest.Attempt),
		JobURL: fmt.Sprintf("%s/job/%d", url, j.ID), ObservedAt: time.Now().UTC(),
	}, nil
}

func sameRun(a, b run, sha string) bool {
	return runOK(a, sha) && runOK(b, sha) && a.ID == b.ID && a.Attempt == b.Attempt && a.CreatedAt.Equal(b.CreatedAt)
}

func newestRun(ctx context.Context, read readAPI, sha string) (run, error) {
	var newest run
	seen := map[int64]bool{}
	total := -1
	for page := 1; ; page++ {
		var response struct {
			Total *int  `json:"total_count"`
			Runs  []run `json:"workflow_runs"`
		}
		path := fmt.Sprintf("repos/%s/actions/workflows/%d/runs?head_sha=%s&event=push&branch=main&per_page=100&page=%d", canonical.repository, canonical.workflowID, sha, page)
		if err := get(ctx, read, path, &response); err != nil {
			return run{}, err
		}
		if response.Total == nil || *response.Total < 0 || *response.Total >= 1000 || (total >= 0 && total != *response.Total) || response.Runs == nil {
			return run{}, errors.New("incomplete or inconsistent workflow runs response")
		}
		total = *response.Total
		want := min(100, total-len(seen))
		if want < 0 || len(response.Runs) != want {
			return run{}, errors.New("truncated workflow runs response")
		}
		for _, r := range response.Runs {
			if !runOK(r, sha) || seen[r.ID] {
				return run{}, errors.New("invalid or duplicate workflow run identity")
			}
			seen[r.ID] = true
			if newest.ID == 0 || r.CreatedAt.After(newest.CreatedAt) || (r.CreatedAt.Equal(newest.CreatedAt) && r.ID > newest.ID) {
				newest = r
			}
		}
		if len(seen) == total {
			break
		}
	}
	if newest.ID == 0 {
		return run{}, errors.New("no canonical main CI run for this commit")
	}
	return newest, nil
}

func listJobs(ctx context.Context, read readAPI, path string, attempt int) ([]job, error) {
	var jobs []job
	seen := map[int64]bool{}
	total := -1
	for page := 1; ; page++ {
		var response struct {
			Total *int  `json:"total_count"`
			Jobs  []job `json:"jobs"`
		}
		if err := get(ctx, read, fmt.Sprintf("%s/attempts/%d/jobs?per_page=100&page=%d", path, attempt, page), &response); err != nil {
			return nil, err
		}
		if response.Total == nil || *response.Total < 0 || *response.Total >= 1000 || (total >= 0 && total != *response.Total) || response.Jobs == nil {
			return nil, errors.New("incomplete or inconsistent jobs response")
		}
		total = *response.Total
		want := min(100, total-len(jobs))
		if want < 0 || len(response.Jobs) != want {
			return nil, errors.New("truncated jobs response")
		}
		for _, j := range response.Jobs {
			if j.ID <= 0 || seen[j.ID] {
				return nil, errors.New("invalid or duplicate job identity")
			}
			seen[j.ID] = true
			jobs = append(jobs, j)
		}
		if len(jobs) == total {
			return jobs, nil
		}
	}
}

func validatorIdentity(content []byte) (string, string, error) {
	type workflowStep struct {
		Name             string `yaml:"name"`
		Run              string `yaml:"run"`
		If               any    `yaml:"if"`
		Continue         any    `yaml:"continue-on-error"`
		Shell            any    `yaml:"shell"`
		WorkingDirectory any    `yaml:"working-directory"`
		Env              any    `yaml:"env"`
	}
	type workflowJob struct {
		Name     string         `yaml:"name"`
		Uses     string         `yaml:"uses"`
		If       any            `yaml:"if"`
		Continue any            `yaml:"continue-on-error"`
		Defaults any            `yaml:"defaults"`
		Env      any            `yaml:"env"`
		Steps    []workflowStep `yaml:"steps"`
	}
	var wf struct {
		Defaults any                    `yaml:"defaults"`
		Env      any                    `yaml:"env"`
		Jobs     map[string]workflowJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(content, &wf); err != nil {
		return "", "", fmt.Errorf("invalid source workflow: %w", err)
	}
	j, ok := wf.Jobs["validate"]
	if !ok || j.Name == "" || j.Uses != "" || j.If != nil || !safeContinue(j.Continue) || j.Defaults != nil || wf.Defaults != nil || j.Env != nil || wf.Env != nil {
		return "", "", errors.New("source validation job is missing or conditional")
	}
	count := 0
	name := ""
	for key, candidate := range wf.Jobs {
		for _, s := range candidate.Steps {
			if strings.Contains(s.Run, "scripts/validate-packy.sh") {
				count++
				if key != "validate" || strings.TrimSpace(s.Run) != "./scripts/validate-packy.sh --ci" || s.Name == "" || s.If != nil || !safeContinue(s.Continue) || s.Shell != nil || s.WorkingDirectory != nil || s.Env != nil {
					return "", "", errors.New("source validator invocation is indirect or can mask failure")
				}
				name = s.Name
			}
		}
	}
	if count != 1 {
		return "", "", errors.New("source workflow must contain exactly one direct validator invocation")
	}
	return j.Name, name, nil
}

func safeContinue(value any) bool { return value == nil || value == false }
