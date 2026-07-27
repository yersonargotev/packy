// deliveryevidence is a private, read-only observation adapter. The only write
// it performs is the explicitly selected evidence file.
package main

import (
	"context"
	"crypto/sha256"
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
	"time"

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

type ValidationRunner interface {
	Run(context.Context, string, deliveryevidence.SandboxFacts) error
}
type execValidationRunner struct{}

func (execValidationRunner) Run(ctx context.Context, repository string, sandbox deliveryevidence.SandboxFacts) error {
	c := exec.CommandContext(ctx, "./scripts/validate-packy.sh")
	c.Dir = repository
	c.Env = replacedEnvironment(os.Environ(), map[string]string{
		"HOME":                         sandbox.HomeRoot,
		"XDG_CONFIG_HOME":              sandbox.ConfigHomeRoot,
		"PACKY_VALIDATION_HOME":        sandbox.HomeRoot,
		"PACKY_VALIDATION_CONFIG_HOME": sandbox.ConfigHomeRoot,
	})
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func replacedEnvironment(current []string, replacements map[string]string) []string {
	out := make([]string, 0, len(current)+len(replacements))
	for _, entry := range current {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := replacements[key]; !replaced {
			out = append(out, entry)
		}
	}
	for key, value := range replacements {
		out = append(out, key+"="+value)
	}
	return out
}

type command struct {
	Git        Runner
	GitHub     Runner
	Validation ValidationRunner
	Now        func() time.Time
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
	if err := (command{Git: execRunner{}, GitHub: execRunner{}, Validation: execValidationRunner{}, Now: time.Now}).run(context.Background(), os.Args[1:], os.Stdout); err != nil {
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
	case "record-iteration":
		return c.recordIteration(ctx, args[1:], stdout)
	case "record-review":
		return c.recordReview(args[1:], stdout)
	case "record-adjudication":
		return c.recordAdjudication(args[1:], stdout)
	case "review-status":
		return c.reviewStatus(ctx, args[1:], stdout)
	case "record-exhaustive-validation":
		return c.recordExhaustiveValidation(ctx, args[1:], stdout)
	case "validation-status":
		return c.validationStatus(ctx, args[1:], stdout)
	case "record-focused-validation":
		return c.recordFocusedValidation(args[1:], stdout)
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
	topRaw, err := c.Git.Output(ctx, "git", "-C", repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("observe Git worktree: %w", err)
	}
	worktree, err := filepath.Abs(strings.TrimSpace(string(topRaw)))
	if err != nil {
		return err
	}
	originRaw, err := c.Git.Output(ctx, "git", "-C", repo, "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("observe origin: %w", err)
	}
	baseRaw, err := c.Git.Output(ctx, "git", "-C", repo, "rev-parse", "origin/main^{commit}")
	if err != nil {
		return fmt.Errorf("observe exact origin/main: %w", err)
	}
	if strings.TrimSpace(string(baseRaw)) != q.StartingBaseSHA {
		return errors.New("qualification starting base does not match origin/main")
	}
	for _, iteration := range q.Iterations {
		for _, sha := range []string{iteration.BaseSHA, iteration.HeadSHA} {
			if _, err = c.Git.Output(ctx, "git", "-C", repo, "cat-file", "-e", sha+"^{commit}"); err != nil {
				return fmt.Errorf("iteration %d references a foreign or missing commit: %w", iteration.Sequence, err)
			}
		}
	}
	slug := q.Repository.Owner + "/" + q.Repository.Name
	localSlug, err := githubSlug(strings.TrimSpace(string(originRaw)))
	if err != nil {
		return err
	}
	if localSlug != slug {
		return errors.New("qualification repository does not match local origin")
	}
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
	bundle := deliveryevidence.Bundle{Schema: deliveryevidence.SchemaV1, Repository: deliveryevidence.RepositoryIdentity{Owner: q.Repository.Owner, Name: q.Repository.Name, NodeID: ro.ID}, Issue: deliveryevidence.IssueIdentity{Number: issue.Number, NodeID: issue.ID}, Spec: deliveryevidence.SpecIdentity{Number: spec.Number, NodeID: spec.ID}, Authority: deliveryevidence.Authority{IssueSHA256: issueHash, SpecSHA256: specHash, Labels: labels, DependencyDisposition: q.DependencyDisposition, AcceptanceCriteria: q.AcceptanceCriteria}, Scope: q.Scope, AcceptanceMatrix: q.AcceptanceMatrix, StartingBaseSHA: q.StartingBaseSHA, Iterations: q.Iterations, ReviewReceipts: []deliveryevidence.ReviewReceipt{}, Adjudications: []deliveryevidence.Adjudication{}, ValidationReceipts: []deliveryevidence.ValidationReceipt{}, FocusedValidation: []deliveryevidence.FocusedValidationEvidence{}}
	path, err := deliveryevidence.ResolvePath(commonPath, output, bundle.Issue.Number)
	if err != nil {
		return err
	}
	if output != "" {
		inside, err := pathWithin(path, worktree)
		if err != nil {
			return err
		}
		if inside {
			return errors.New("delivery evidence override must be outside the worktree")
		}
	}
	result, err := deliveryevidence.InitializeOrResume(path, bundle)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s %s\n", result.State, result.Path)
	return err
}

func githubSlug(origin string) (string, error) {
	s := strings.TrimSuffix(origin, ".git")
	if strings.HasPrefix(s, "git@github.com:") {
		s = strings.TrimPrefix(s, "git@github.com:")
	} else if strings.HasPrefix(s, "ssh://git@github.com/") {
		s = strings.TrimPrefix(s, "ssh://git@github.com/")
	} else if strings.HasPrefix(s, "https://github.com/") {
		s = strings.TrimPrefix(s, "https://github.com/")
	} else {
		return "", errors.New("origin is not a canonical GitHub SSH/HTTPS URL")
	}
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", errors.New("origin does not identify one GitHub repository")
	}
	return s, nil
}
func pathWithin(path, root string) (bool, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	parent := filepath.Dir(path)
	suffix := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			parent = resolved
			break
		}
		next := filepath.Dir(parent)
		if next == parent {
			return false, err
		}
		suffix = append([]string{filepath.Base(parent)}, suffix...)
		parent = next
	}
	candidate := filepath.Join(append([]string{parent}, append(suffix, filepath.Base(path))...)...)
	rel, err := filepath.Rel(resolvedRoot, candidate)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
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
		want[d.Identity] = string(d.Disposition)
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

func (c command) recordIteration(ctx context.Context, args []string, stdout io.Writer) error {
	f := flag.NewFlagSet("deliveryevidence record-iteration", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var bundlePath, inputPath, repo string
	f.StringVar(&bundlePath, "bundle", "", "canonical evidence bundle")
	f.StringVar(&inputPath, "iteration", "", "canonical iteration input")
	f.StringVar(&repo, "repository", ".", "repository to observe")
	if err := f.Parse(args); err != nil {
		return err
	}
	if bundlePath == "" || inputPath == "" || f.NArg() != 0 {
		return errors.New("bundle and iteration are required")
	}
	if c.Git == nil {
		return errors.New("Git read-only runner is required")
	}
	bundle, _, err := deliveryevidence.Load(bundlePath)
	if err != nil {
		return err
	}
	var iteration deliveryevidence.Iteration
	if err = decodeFile(inputPath, &iteration); err != nil {
		return err
	}
	for _, sha := range []string{iteration.BaseSHA, iteration.HeadSHA} {
		if _, err = c.Git.Output(ctx, "git", "-C", repo, "cat-file", "-e", sha+"^{commit}"); err != nil {
			return fmt.Errorf("iteration references a foreign or missing commit: %w", err)
		}
	}
	bundle, err = deliveryevidence.RecordIteration(bundle, iteration)
	if err != nil {
		return err
	}
	if err = deliveryevidence.StoreAtomic(bundlePath, bundle); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "recorded iteration %s\n", iteration.Identity)
	return err
}

func (c command) recordReview(args []string, stdout io.Writer) error {
	var record struct {
		Receipt       deliveryevidence.ReviewReceipt  `json:"receipt"`
		Adjudications []deliveryevidence.Adjudication `json:"adjudications"`
	}
	bundle, path, err := recordInput(args, "receipt", &record)
	if err != nil {
		return err
	}
	if record.Adjudications == nil {
		return errors.New("review record requires an explicit adjudications array")
	}
	bundle, err = deliveryevidence.RecordReview(bundle, record.Receipt, record.Adjudications...)
	if err != nil {
		return err
	}
	if err = deliveryevidence.StoreAtomic(path, bundle); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "recorded %s review for %s\n", record.Receipt.Axis, record.Receipt.Iteration)
	return err
}

func (c command) recordAdjudication(args []string, stdout io.Writer) error {
	var adjudication deliveryevidence.Adjudication
	bundle, path, err := recordInput(args, "adjudication", &adjudication)
	if err != nil {
		return err
	}
	bundle, err = deliveryevidence.RecordAdjudication(bundle, adjudication)
	if err != nil {
		return err
	}
	if err = deliveryevidence.StoreAtomic(path, bundle); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "recorded adjudication %d for %s\n", adjudication.Sequence, adjudication.FindingID)
	return err
}

func recordInput[T any](args []string, inputName string, value *T) (deliveryevidence.Bundle, string, error) {
	f := flag.NewFlagSet("deliveryevidence record-"+inputName, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var bundlePath, inputPath string
	f.StringVar(&bundlePath, "bundle", "", "canonical evidence bundle")
	f.StringVar(&inputPath, inputName, "", "canonical record input")
	if err := f.Parse(args); err != nil {
		return deliveryevidence.Bundle{}, "", err
	}
	if bundlePath == "" || inputPath == "" || f.NArg() != 0 {
		return deliveryevidence.Bundle{}, "", fmt.Errorf("bundle and %s are required", inputName)
	}
	bundle, _, err := deliveryevidence.Load(bundlePath)
	if err != nil {
		return deliveryevidence.Bundle{}, "", err
	}
	if err = decodeFile(inputPath, value); err != nil {
		return deliveryevidence.Bundle{}, "", err
	}
	return bundle, bundlePath, nil
}

func decodeFile(path string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return errors.New("record input must contain exactly one JSON value")
	}
	return nil
}

func (c command) reviewStatus(ctx context.Context, args []string, stdout io.Writer) error {
	f := flag.NewFlagSet("deliveryevidence review-status", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var bundlePath, repo string
	f.StringVar(&bundlePath, "bundle", "", "canonical evidence bundle")
	f.StringVar(&repo, "repository", ".", "repository to observe")
	if err := f.Parse(args); err != nil {
		return err
	}
	if bundlePath == "" || f.NArg() != 0 {
		return errors.New("bundle is required")
	}
	if c.Git == nil {
		return errors.New("Git read-only runner is required")
	}
	bundle, _, err := deliveryevidence.Load(bundlePath)
	if err != nil {
		return err
	}
	headRaw, err := c.Git.Output(ctx, "git", "-C", repo, "rev-parse", "HEAD^{commit}")
	if err != nil {
		return fmt.Errorf("observe current head: %w", err)
	}
	head := strings.TrimSpace(string(headRaw))
	commitsRaw, err := c.Git.Output(ctx, "git", "-C", repo, "rev-list", "--reverse", "--ancestry-path", bundle.StartingBaseSHA+".."+head)
	if err != nil {
		return fmt.Errorf("observe delivery commits: %w", err)
	}
	commits := append([]string{bundle.StartingBaseSHA}, strings.Fields(string(commitsRaw))...)
	report, err := deliveryevidence.RenderReviewReport(bundle, head, commits)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, report)
	return err
}

const exhaustiveValidationCommand = "./scripts/validate-packy.sh"

type validationFlags struct {
	bundlePath      string
	repository      string
	sandboxHome     string
	sandboxConfig   string
	identityExpires string
	requiredCommand string
}

func parseValidationFlags(name string, args []string) (validationFlags, error) {
	f := flag.NewFlagSet("deliveryevidence "+name, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var values validationFlags
	f.StringVar(&values.bundlePath, "bundle", "", "canonical evidence bundle")
	f.StringVar(&values.repository, "repository", ".", "repository to observe")
	f.StringVar(&values.sandboxHome, "sandbox-home", "", "absolute disposable HOME used by validation")
	f.StringVar(&values.sandboxConfig, "sandbox-config-home", "", "absolute disposable configuration root used by validation")
	f.StringVar(&values.identityExpires, "validator-identity-expires-at", "", "canonical RFC3339 validator identity expiry")
	f.StringVar(&values.requiredCommand, "required-command", exhaustiveValidationCommand, "exact validation authority command to observe")
	if err := f.Parse(args); err != nil {
		return validationFlags{}, err
	}
	if values.bundlePath == "" || values.sandboxHome == "" || values.sandboxConfig == "" || values.identityExpires == "" || f.NArg() != 0 {
		return validationFlags{}, errors.New("bundle, sandbox-home, sandbox-config-home, and validator-identity-expires-at are required")
	}
	return values, nil
}

func (c command) recordExhaustiveValidation(ctx context.Context, args []string, stdout io.Writer) error {
	values, err := parseValidationFlags("record-exhaustive-validation", args)
	if err != nil {
		return err
	}
	if c.Validation == nil {
		return errors.New("validation runner is required")
	}
	if values.requiredCommand != exhaustiveValidationCommand {
		return errors.New("recording supports only the exhaustive validation authority command")
	}
	bundle, _, err := deliveryevidence.Load(values.bundlePath)
	if err != nil {
		return err
	}
	before, err := c.validationObservation(ctx, bundle, values)
	if err != nil {
		return err
	}
	if err = c.Validation.Run(ctx, values.repository, before.Sandbox); err != nil {
		return fmt.Errorf("exhaustive validation failed: %w", err)
	}
	after, err := c.validationObservation(ctx, bundle, values)
	if err != nil {
		return err
	}
	if before != after {
		return errors.New("repository, validator, or sandbox facts changed during exhaustive validation")
	}
	completedAt := c.now().Format(time.RFC3339Nano)
	bundle, err = deliveryevidence.RecordExhaustiveValidation(bundle, deliveryevidence.ExhaustiveValidationResult{
		Observation: after,
		CompletedAt: completedAt,
		Succeeded:   true,
		Completed:   true,
	})
	if err != nil {
		return err
	}
	if err = deliveryevidence.StoreAtomic(values.bundlePath, bundle); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "recorded exhaustive validation for %s\n", after.TreeSHA)
	return err
}

func (c command) validationStatus(ctx context.Context, args []string, stdout io.Writer) error {
	values, err := parseValidationFlags("validation-status", args)
	if err != nil {
		return err
	}
	bundle, _, err := deliveryevidence.Load(values.bundlePath)
	if err != nil {
		return err
	}
	current, err := c.validationObservation(ctx, bundle, values)
	if err != nil {
		return err
	}
	receipt, err := deliveryevidence.ReusableExhaustiveValidation(bundle, current, c.now().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "reusable exhaustive validation completed at %s\n", receipt.CompletedAt)
	return err
}

func (c command) recordFocusedValidation(args []string, stdout io.Writer) error {
	var evidence deliveryevidence.FocusedValidationEvidence
	bundle, path, err := recordInput(args, "evidence", &evidence)
	if err != nil {
		return err
	}
	bundle, err = deliveryevidence.RecordFocusedValidation(bundle, evidence)
	if err != nil {
		return err
	}
	if err = deliveryevidence.StoreAtomic(path, bundle); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "recorded non-authoritative focused validation %s\n", evidence.Identity)
	return err
}

func (c command) validationObservation(ctx context.Context, bundle deliveryevidence.Bundle, values validationFlags) (deliveryevidence.ValidationObservation, error) {
	if c.Git == nil {
		return deliveryevidence.ValidationObservation{}, errors.New("Git read-only runner is required")
	}
	repository, err := filepath.Abs(values.repository)
	if err != nil {
		return deliveryevidence.ValidationObservation{}, err
	}
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		return deliveryevidence.ValidationObservation{}, fmt.Errorf("resolve repository: %w", err)
	}
	sandbox := deliveryevidence.SandboxFacts{
		HomeRoot:       filepath.Clean(values.sandboxHome),
		ConfigHomeRoot: filepath.Clean(values.sandboxConfig),
		Sandboxed:      true,
	}
	for _, root := range []string{sandbox.HomeRoot, sandbox.ConfigHomeRoot} {
		if !filepath.IsAbs(root) {
			return deliveryevidence.ValidationObservation{}, errors.New("sandbox roots must be absolute")
		}
		inside, err := pathWithin(root, repository)
		if err != nil {
			return deliveryevidence.ValidationObservation{}, err
		}
		if inside {
			return deliveryevidence.ValidationObservation{}, errors.New("validation sandbox roots must be outside the repository worktree")
		}
	}
	originRaw, err := c.Git.Output(ctx, "git", "-C", repository, "remote", "get-url", "origin")
	if err != nil {
		return deliveryevidence.ValidationObservation{}, fmt.Errorf("observe origin: %w", err)
	}
	slug, err := githubSlug(strings.TrimSpace(string(originRaw)))
	if err != nil {
		return deliveryevidence.ValidationObservation{}, err
	}
	if slug != bundle.Repository.Owner+"/"+bundle.Repository.Name {
		return deliveryevidence.ValidationObservation{}, errors.New("validation repository does not match bundle authority")
	}
	if c.GitHub == nil {
		return deliveryevidence.ValidationObservation{}, errors.New("GitHub read-only runner is required")
	}
	repositoryRaw, err := c.GitHub.Output(ctx, "gh", "repo", "view", slug, "--json", "nameWithOwner,id")
	if err != nil {
		return deliveryevidence.ValidationObservation{}, fmt.Errorf("observe GitHub repository identity: %w", err)
	}
	var observedRepository repoObservation
	if err = json.Unmarshal(repositoryRaw, &observedRepository); err != nil {
		return deliveryevidence.ValidationObservation{}, err
	}
	if observedRepository.NameWithOwner != slug || observedRepository.ID != bundle.Repository.NodeID {
		return deliveryevidence.ValidationObservation{}, errors.New("validation repository identity does not match bundle authority")
	}
	headRaw, err := c.Git.Output(ctx, "git", "-C", repository, "rev-parse", "HEAD^{commit}")
	if err != nil {
		return deliveryevidence.ValidationObservation{}, fmt.Errorf("observe validation commit: %w", err)
	}
	treeRaw, err := c.Git.Output(ctx, "git", "-C", repository, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return deliveryevidence.ValidationObservation{}, fmt.Errorf("observe validation tree: %w", err)
	}
	statusRaw, err := c.Git.Output(ctx, "git", "-C", repository, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return deliveryevidence.ValidationObservation{}, fmt.Errorf("observe validation workspace: %w", err)
	}
	validatorPath := filepath.Join(repository, "scripts", "validate-packy.sh")
	validatorBytes, err := os.ReadFile(validatorPath)
	if err != nil {
		return deliveryevidence.ValidationObservation{}, fmt.Errorf("read validation authority: %w", err)
	}
	checkoutDigest := sha256.Sum256([]byte(repository))
	validatorDigest := sha256.Sum256(validatorBytes)
	return deliveryevidence.ValidationObservation{
		Repository:                 bundle.Repository,
		CheckoutSHA256:             fmt.Sprintf("%x", checkoutDigest),
		CommitSHA:                  strings.TrimSpace(string(headRaw)),
		TreeSHA:                    strings.TrimSpace(string(treeRaw)),
		WorkspaceClean:             len(statusRaw) == 0,
		ValidatorIdentity:          "scripts/validate-packy.sh",
		ValidatorSHA256:            fmt.Sprintf("%x", validatorDigest),
		ValidatorIdentityExpiresAt: values.identityExpires,
		RequiredCommand:            values.requiredCommand,
		Sandbox:                    sandbox,
	}, nil
}

func (c command) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
