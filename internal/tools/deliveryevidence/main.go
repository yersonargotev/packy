// deliveryevidence is a private, read-only observation adapter. The only write
// it performs is the explicitly selected evidence file.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
)

type Runner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}
type execRunner struct{}

func (execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, name, args...)
	return c.Output()
}

type command struct {
	Git    Runner
	GitHub Runner
}

type qualification struct {
	Schema     string `json:"schema"`
	Repository struct {
		Owner string `json:"owner"`
		Name  string `json:"name"`
	} `json:"repository"`
	IssueNumber           int                                      `json:"issue_number"`
	SpecNumber            int                                      `json:"spec_number"`
	DependencyDisposition []deliveryevidence.DependencyDisposition `json:"dependency_disposition"`
	Scope                 deliveryevidence.ScopeLedger             `json:"scope"`
	AcceptanceCriteria    []string                                 `json:"acceptance_criteria"`
	AcceptanceMatrix      []deliveryevidence.AcceptanceRow         `json:"acceptance_matrix"`
	StartingBaseSHA       string                                   `json:"starting_base_sha"`
	Iterations            []deliveryevidence.Iteration             `json:"iterations"`
}
type repoObservation struct {
	NameWithOwner string `json:"nameWithOwner"`
	ID            string `json:"id"`
}
type labelObservation struct {
	Name string `json:"name"`
}
type blockedObservation struct {
	Number int    `json:"number"`
	ID     string `json:"id"`
	State  string `json:"state"`
}
type issueObservation struct {
	Number    int                `json:"number"`
	ID        string             `json:"id"`
	Title     string             `json:"title"`
	Body      string             `json:"body"`
	State     string             `json:"state"`
	Labels    []labelObservation `json:"labels"`
	BlockedBy struct {
		Nodes      []blockedObservation `json:"nodes"`
		TotalCount int                  `json:"totalCount"`
	} `json:"blockedBy"`
}

func main() {
	if err := (command{execRunner{}, execRunner{}}).run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (c command) run(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("command is required: initialize or status")
	}
	switch args[0] {
	case "initialize":
		return c.initialize(ctx, args[1:], stdout)
	case "status":
		return c.status(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (c command) initialize(ctx context.Context, args []string, stdout io.Writer) error {
	f := flag.NewFlagSet("deliveryevidence initialize", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var input, output, repo string
	f.StringVar(&input, "qualified-bundle", "", "caller-qualified canonical bundle")
	f.StringVar(&output, "out", "", "absolute disposable evidence override")
	f.StringVar(&repo, "repository", ".", "repository to observe")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || input == "" {
		return errors.New("qualified-bundle is required and positional arguments are forbidden")
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	var q qualification
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&q); err != nil {
		return fmt.Errorf("decode qualification: %w", err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return errors.New("qualification must contain exactly one JSON value")
	}
	if q.Schema != deliveryevidence.SchemaV1 || q.IssueNumber <= 0 || q.SpecNumber <= 0 || q.IssueNumber == q.SpecNumber {
		return errors.New("qualification schema and distinct positive issue/spec numbers are required")
	}
	if c.Git == nil || c.GitHub == nil {
		return errors.New("Git and GitHub read-only runners are required")
	}
	common, err := c.Git.Output(ctx, "git", "-C", repo, "rev-parse", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("observe Git common directory: %w", err)
	}
	commonPath := strings.TrimSpace(string(common))
	if !filepath.IsAbs(commonPath) {
		commonPath = filepath.Join(repo, commonPath)
	}
	commonPath, err = filepath.Abs(commonPath)
	if err != nil {
		return err
	}
	slug := q.Repository.Owner + "/" + q.Repository.Name
	repoRaw, err := c.GitHub.Output(ctx, "gh", "repo", "view", slug, "--json", "nameWithOwner,id")
	if err != nil {
		return fmt.Errorf("observe GitHub repository: %w", err)
	}
	var ro repoObservation
	if err = json.Unmarshal(repoRaw, &ro); err != nil {
		return err
	}
	if ro.NameWithOwner != slug || ro.ID == "" {
		return errors.New("foreign repository identity")
	}
	observe := func(number int) (issueObservation, error) {
		raw, e := c.GitHub.Output(ctx, "gh", "issue", "view", fmt.Sprint(number), "--repo", slug, "--json", "number,id,title,body,state,labels,blockedBy")
		if e != nil {
			return issueObservation{}, e
		}
		var o issueObservation
		e = json.Unmarshal(raw, &o)
		return o, e
	}
	issue, err := observe(q.IssueNumber)
	if err != nil {
		return fmt.Errorf("observe issue: %w", err)
	}
	spec, err := observe(q.SpecNumber)
	if err != nil {
		return fmt.Errorf("observe spec: %w", err)
	}
	if issue.Number != q.IssueNumber || spec.Number != q.SpecNumber || issue.ID == "" || spec.ID == "" {
		return errors.New("foreign issue or spec identity")
	}
	if !strings.EqualFold(issue.State, "OPEN") || !strings.EqualFold(spec.State, "OPEN") {
		return errors.New("issue and accepted spec must be open")
	}
	labels := labelNames(issue.Labels)
	if !hasLabel(labels, "status:approved") && !hasLabel(labels, "status:needs-review") {
		return errors.New("issue is not eligible: approved or needs-review status is required")
	}
	specLabels := labelNames(spec.Labels)
	if !hasLabel(specLabels, "status:accepted") && !hasLabel(specLabels, "status:approved") {
		return errors.New("spec authority is not accepted")
	}
	if err = matchDependencies(q.DependencyDisposition, issue.BlockedBy.Nodes); err != nil {
		return err
	}
	issueHash, err := deliveryevidence.TypedObservationHash("github-issue", fmt.Sprintf("%s#%d:%s", slug, issue.Number, issue.ID), normalizedIssue(issue))
	if err != nil {
		return err
	}
	specHash, err := deliveryevidence.TypedObservationHash("github-spec", fmt.Sprintf("%s#%d:%s", slug, spec.Number, spec.ID), normalizedIssue(spec))
	if err != nil {
		return err
	}
	bundle := deliveryevidence.Bundle{Schema: deliveryevidence.SchemaV1, Repository: deliveryevidence.RepositoryIdentity{Owner: q.Repository.Owner, Name: q.Repository.Name, NodeID: ro.ID}, Issue: deliveryevidence.IssueIdentity{Number: issue.Number, NodeID: issue.ID}, Spec: deliveryevidence.SpecIdentity{Number: spec.Number, NodeID: spec.ID}, Authority: deliveryevidence.Authority{IssueSHA256: issueHash, SpecSHA256: specHash, Labels: labels, DependencyDisposition: q.DependencyDisposition, AcceptanceCriteria: q.AcceptanceCriteria}, Scope: q.Scope, AcceptanceMatrix: q.AcceptanceMatrix, StartingBaseSHA: q.StartingBaseSHA, Iterations: q.Iterations}
	path, err := deliveryevidence.ResolvePath(commonPath, output, bundle.Issue.Number)
	if err != nil {
		return err
	}
	result, err := deliveryevidence.InitializeOrResume(path, bundle)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s %s\n", result.State, result.Path)
	return err
}

func labelNames(in []labelObservation) []string {
	out := make([]string, 0, len(in))
	for _, l := range in {
		out = append(out, l.Name)
	}
	sort.Strings(out)
	return out
}
func hasLabel(labels []string, want string) bool {
	for _, v := range labels {
		if v == want {
			return true
		}
	}
	return false
}

type canonicalIssue struct {
	Number    int                  `json:"number"`
	ID        string               `json:"id"`
	Title     string               `json:"title"`
	Body      string               `json:"body"`
	State     string               `json:"state"`
	Labels    []string             `json:"labels"`
	BlockedBy []blockedObservation `json:"blocked_by"`
}

func normalizedIssue(o issueObservation) canonicalIssue {
	blocked := append([]blockedObservation(nil), o.BlockedBy.Nodes...)
	sort.Slice(blocked, func(i, j int) bool {
		if blocked[i].Number == blocked[j].Number {
			return blocked[i].ID < blocked[j].ID
		}
		return blocked[i].Number < blocked[j].Number
	})
	return canonicalIssue{o.Number, o.ID, o.Title, o.Body, strings.ToUpper(o.State), labelNames(o.Labels), blocked}
}

func matchDependencies(qualified []deliveryevidence.DependencyDisposition, observed []blockedObservation) error {
	want := map[string]string{}
	for _, d := range qualified {
		want[d.Identity] = d.Disposition
	}
	if len(want) != len(observed) {
		return errors.New("dependency disposition changed")
	}
	for _, d := range observed {
		id := fmt.Sprintf("#%d", d.Number)
		disposition := "blocking"
		if strings.EqualFold(d.State, "CLOSED") {
			disposition = "satisfied"
		}
		if want[id] != disposition {
			return errors.New("dependency disposition changed")
		}
	}
	return nil
}

func (c command) status(args []string, stdout io.Writer) error {
	f := flag.NewFlagSet("deliveryevidence status", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var path string
	f.StringVar(&path, "bundle", "", "canonical evidence bundle")
	if err := f.Parse(args); err != nil {
		return err
	}
	if path == "" || f.NArg() != 0 {
		return errors.New("bundle is required")
	}
	b, _, err := deliveryevidence.Load(path)
	if err != nil {
		return err
	}
	s, err := deliveryevidence.RenderStatus(b)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, s)
	return err
}
