package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yersonargotev/packy/internal/deliveryevidence"
	"github.com/yersonargotev/packy/internal/issuedelivery"
)

type productionTrackerObserver struct {
	runner              Runner
	specificationNumber int
}

type suppliedReviewExecutor struct {
	reviews []issuedelivery.CandidateReview
}

type suppliedSpecialistExecutor struct {
	reviews []issuedelivery.SpecialistReview
}

type productionValidationExecutor struct {
	repository string
	runner     Runner
	focused    ValidationRunner
	exhaustive ValidationRunner
	now        func() time.Time
	acceptance []issuedelivery.AcceptanceProof
}

type execFocusedValidationRunner struct{}

type productionBoundaryExecutor struct {
	repository string
	runner     Runner
	validation ValidationRunner
	mu         *sync.Mutex
}

func newProductionAdvancer(options advanceOptions) (issueDeliveryAdvancer, error) {
	runner := execRunner{}
	sandboxRoot, err := productionSandboxRoot(
		context.Background(), runner, options.RepositoryPath, options.IssueNumber, options.SandboxRoot,
	)
	if err != nil {
		return nil, err
	}
	validation := execValidationRunner{}
	return issuedelivery.New(issuedelivery.Config{
		Git: productionGitObserver{runner: runner},
		GitHub: productionTrackerObserver{
			runner: runner, specificationNumber: options.SpecificationNumber,
		},
		Review: suppliedReviewExecutor{reviews: options.Reviews},
		Validation: productionValidationExecutor{
			repository: options.RepositoryPath, runner: runner,
			focused: execFocusedValidationRunner{}, exhaustive: validation, now: time.Now,
			acceptance: append([]issuedelivery.AcceptanceProof(nil), options.Acceptance...),
		},
		Risk:       productionCandidateRiskObserver{runner: runner},
		Specialist: suppliedSpecialistExecutor{reviews: options.Specialists},
		Boundary: productionBoundaryExecutor{
			repository: options.RepositoryPath, runner: runner,
			validation: validation, mu: &sync.Mutex{},
		},
		NonLocal: productionNonLocalGateway{
			runner: runner, repository: options.RepositoryPath,
			attributions: options.CIFailureAttributions,
		},
		LocalCompletion: productionLocalCompletionGateway{runner: runner},
		SandboxRoot:     sandboxRoot,
		DeclaredProfile: options.DeclaredProfile,
	})
}

func productionSandboxRoot(
	ctx context.Context,
	runner Runner,
	repository string,
	issue int,
	override string,
) (string, error) {
	root := strings.TrimSpace(override)
	if root == "" {
		raw, err := runner.Output(ctx, "git", "-C", repository, "rev-parse", "--git-common-dir")
		if err != nil {
			return "", fmt.Errorf("observe Git common directory for sandbox: %w", err)
		}
		common := strings.TrimSpace(string(raw))
		if !filepath.IsAbs(common) {
			common = filepath.Join(repository, common)
		}
		common, err = filepath.Abs(common)
		if err != nil {
			return "", err
		}
		root = filepath.Join(common, "packy", "issue-delivery", fmt.Sprintf("issue-%d-sandbox", issue))
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	if absolute == string(filepath.Separator) {
		return "", errors.New("validation sandbox cannot be the filesystem root")
	}
	for _, path := range []string{absolute, filepath.Join(absolute, "home"), filepath.Join(absolute, "config")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return "", fmt.Errorf("create validation sandbox %q: %w", path, err)
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("validation sandbox %q is not a real directory", path)
		}
	}
	return absolute, nil
}

func (o productionTrackerObserver) ObserveIssue(
	ctx context.Context,
	git issuedelivery.GitObservation,
	number int,
) (issuedelivery.TrackerObservation, error) {
	if o.runner == nil {
		return issuedelivery.TrackerObservation{}, errors.New("GitHub runner is required")
	}
	slug := git.Owner + "/" + git.Name
	repositoryRaw, err := o.runner.Output(ctx, "gh", "repo", "view", slug, "--json", "nameWithOwner,id")
	if err != nil {
		return issuedelivery.TrackerObservation{}, fmt.Errorf("observe GitHub repository: %w", err)
	}
	var repository repoObservation
	if err = json.Unmarshal(repositoryRaw, &repository); err != nil {
		return issuedelivery.TrackerObservation{}, fmt.Errorf("decode GitHub repository: %w", err)
	}
	if repository.NameWithOwner != slug || repository.ID == "" {
		return issuedelivery.TrackerObservation{}, errors.New("GitHub repository identity is incompatible with origin")
	}
	issueRaw, err := o.runner.Output(
		ctx, "gh", "issue", "view", fmt.Sprint(number), "--repo", slug,
		"--json", "number,id,title,body,state,url,labels,blockedBy",
	)
	if err != nil {
		return issuedelivery.TrackerObservation{}, fmt.Errorf("observe GitHub issue: %w", err)
	}
	var issue struct {
		Number    int                `json:"number"`
		ID        string             `json:"id"`
		Title     string             `json:"title"`
		Body      string             `json:"body"`
		State     string             `json:"state"`
		URL       string             `json:"url"`
		Labels    []labelObservation `json:"labels"`
		BlockedBy struct {
			Nodes      []blockedObservation `json:"nodes"`
			TotalCount int                  `json:"totalCount"`
		} `json:"blockedBy"`
	}
	if err = json.Unmarshal(issueRaw, &issue); err != nil {
		return issuedelivery.TrackerObservation{}, fmt.Errorf("decode GitHub issue: %w", err)
	}
	if issue.Number != number || issue.ID == "" || issue.URL != "https://github.com/"+slug+"/issues/"+fmt.Sprint(number) {
		return issuedelivery.TrackerObservation{}, errors.New("GitHub issue identity is incompatible with the requested Packy issue")
	}
	criteria, exclusions, ambiguities, references := parseIssueAuthority(issue.Body, issue.URL, slug)
	labels := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		labels = append(labels, label.Name)
	}
	sort.Strings(labels)
	dependencies := make([]issuedelivery.DependencyObservation, 0, len(issue.BlockedBy.Nodes))
	if issue.BlockedBy.TotalCount != len(issue.BlockedBy.Nodes) {
		return issuedelivery.TrackerObservation{}, errors.New("GitHub issue dependency observation is incomplete")
	}
	for _, dependency := range issue.BlockedBy.Nodes {
		dependencies = append(dependencies, issuedelivery.DependencyObservation{
			Identity: fmt.Sprintf("issue-%d", dependency.Number), Number: dependency.Number,
			Title: fmt.Sprintf("GitHub issue #%d", dependency.Number), State: dependency.State,
			URL: fmt.Sprintf("https://github.com/%s/issues/%d", slug, dependency.Number),
		})
	}
	sort.Slice(dependencies, func(i, j int) bool { return dependencies[i].Number < dependencies[j].Number })
	var specification *issuedelivery.SpecificationObservation
	if o.specificationNumber > 0 {
		specificationRaw, observeErr := o.runner.Output(
			ctx, "gh", "issue", "view", fmt.Sprint(o.specificationNumber), "--repo", slug,
			"--json", "number,id,title,body,state,url,labels",
		)
		if observeErr != nil {
			return issuedelivery.TrackerObservation{}, fmt.Errorf("observe GitHub specification: %w", observeErr)
		}
		var observed struct {
			Number int                `json:"number"`
			ID     string             `json:"id"`
			Title  string             `json:"title"`
			Body   string             `json:"body"`
			State  string             `json:"state"`
			URL    string             `json:"url"`
			Labels []labelObservation `json:"labels"`
		}
		if err = json.Unmarshal(specificationRaw, &observed); err != nil {
			return issuedelivery.TrackerObservation{}, fmt.Errorf("decode GitHub specification: %w", err)
		}
		expectedURL := fmt.Sprintf(
			"https://github.com/%s/issues/%d", slug, o.specificationNumber,
		)
		if observed.Number != o.specificationNumber || observed.ID == "" || observed.URL != expectedURL {
			return issuedelivery.TrackerObservation{}, errors.New(
				"GitHub specification identity is incompatible with the requested Packy specification",
			)
		}
		specificationLabels := make([]string, 0, len(observed.Labels))
		for _, label := range observed.Labels {
			specificationLabels = append(specificationLabels, label.Name)
		}
		sort.Strings(specificationLabels)
		specification = &issuedelivery.SpecificationObservation{
			Identity: deliveryevidence.SpecIdentity{Number: observed.Number, NodeID: observed.ID},
			Title:    observed.Title, Body: observed.Body, State: observed.State,
			URL: observed.URL, Labels: specificationLabels,
		}
	}
	return issuedelivery.TrackerObservation{
		Repository: deliveryevidence.RepositoryIdentity{
			Owner: git.Owner, Name: git.Name, NodeID: repository.ID,
		},
		Issue:         deliveryevidence.IssueIdentity{Number: issue.Number, NodeID: issue.ID},
		Specification: specification,
		Title:         issue.Title, Body: issue.Body, State: issue.State, Labels: labels,
		Criteria: criteria, Exclusions: exclusions, Dependencies: dependencies,
		References: references, Ambiguities: ambiguities,
	}, nil
}

func parseIssueAuthority(
	body string,
	issueURL string,
	slug string,
) ([]issuedelivery.AuthorityItem, []issuedelivery.AuthorityItem, []issuedelivery.AuthorityItem, []issuedelivery.ReferenceObservation) {
	var criteria, exclusions, ambiguities []issuedelivery.AuthorityItem
	referencesByURL := map[string]issuedelivery.ReferenceObservation{}
	section := ""
	criterionIndex, exclusionIndex := 0, 0
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			heading := strings.ToLower(strings.TrimSpace(strings.TrimLeft(line, "#")))
			switch {
			case strings.Contains(heading, "acceptance"):
				section = "criteria"
			case strings.Contains(heading, "out of scope") || strings.Contains(heading, "forbidden") ||
				strings.Contains(heading, "must not"):
				section = "exclusions"
			default:
				section = ""
			}
			continue
		}
		item := markdownListText(line)
		if item != "" {
			switch section {
			case "criteria":
				criterionIndex++
				entry := issuedelivery.AuthorityItem{
					Text: item, EvidenceLink: fmt.Sprintf("%s#acceptance-%d", issueURL, criterionIndex),
				}
				if strings.Contains(strings.ToLower(item), "tbd") ||
					strings.Contains(strings.ToLower(item), "decision required") || strings.HasSuffix(item, "?") {
					ambiguities = append(ambiguities, entry)
				} else {
					criteria = append(criteria, entry)
				}
			case "exclusions":
				exclusionIndex++
				exclusions = append(exclusions, issuedelivery.AuthorityItem{
					Text: item, EvidenceLink: fmt.Sprintf("%s#exclusion-%d", issueURL, exclusionIndex),
				})
			}
		}
		for _, match := range issueReferencePattern.FindAllStringSubmatch(line, -1) {
			number := match[1]
			url := "https://github.com/" + slug + "/issues/" + number
			referencesByURL[url] = issuedelivery.ReferenceObservation{Identity: "issue-" + number, URL: url}
		}
	}
	references := make([]issuedelivery.ReferenceObservation, 0, len(referencesByURL))
	for _, reference := range referencesByURL {
		references = append(references, reference)
	}
	sort.Slice(references, func(i, j int) bool { return references[i].URL < references[j].URL })
	return criteria, exclusions, ambiguities, references
}

var issueReferencePattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9])#([1-9][0-9]*)\b`)

func markdownListText(line string) string {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"- [ ] ", "- [x] ", "- [X] ", "* [ ] ", "* [x] ", "* [X] ", "- ", "* "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func (e suppliedReviewExecutor) Review(
	_ context.Context,
	request issuedelivery.ReviewRequest,
) (issuedelivery.CandidateReview, error) {
	for _, review := range e.reviews {
		if review.CandidateID == request.CandidateID && review.Axis == request.Axis {
			if review.Findings == nil {
				review.Findings = []deliveryevidence.ReviewFinding{}
			}
			return review, nil
		}
	}
	return issuedelivery.CandidateReview{
		CandidateID: request.CandidateID, Axis: request.Axis,
		Findings: []deliveryevidence.ReviewFinding{}, Completed: false,
	}, nil
}

func (e suppliedSpecialistExecutor) ReviewSpecialist(
	_ context.Context,
	request issuedelivery.SpecialistReviewRequest,
) (issuedelivery.SpecialistReview, error) {
	for _, review := range e.reviews {
		if review.CandidateID == request.CandidateID &&
			review.Boundary == request.Boundary && review.Specialist == request.Specialist {
			if review.Findings == nil {
				review.Findings = []issuedelivery.SpecialistFinding{}
			}
			return review, nil
		}
	}
	return issuedelivery.SpecialistReview{
		CandidateID: request.CandidateID, Boundary: request.Boundary,
		Specialist: request.Specialist, Findings: []issuedelivery.SpecialistFinding{}, Completed: false,
	}, nil
}

func (e productionValidationExecutor) Focused(
	ctx context.Context,
	request issuedelivery.ValidationRequest,
) (issuedelivery.ValidationResult, error) {
	if err := e.focused.Run(ctx, e.repository, deliveryevidence.SandboxFacts{
		HomeRoot: request.HomeRoot, ConfigHomeRoot: request.ConfigRoot, Sandboxed: true,
	}); err != nil {
		return issuedelivery.ValidationResult{}, fmt.Errorf("run focused checks: %w", err)
	}
	return e.result(ctx, request, "go test ./...", false)
}

func (execFocusedValidationRunner) Run(
	ctx context.Context,
	repository string,
	sandbox deliveryevidence.SandboxFacts,
) error {
	command := exec.CommandContext(ctx, "go", "-C", repository, "test", "./...")
	command.Env = replacedEnvironment(os.Environ(), map[string]string{
		"HOME": sandbox.HomeRoot, "XDG_CONFIG_HOME": sandbox.ConfigHomeRoot,
		"PACKY_VALIDATION_HOME":        sandbox.HomeRoot,
		"PACKY_VALIDATION_CONFIG_HOME": sandbox.ConfigHomeRoot,
	})
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

func (e productionValidationExecutor) Exhaustive(
	ctx context.Context,
	request issuedelivery.ValidationRequest,
) (issuedelivery.ValidationResult, error) {
	if err := validateAcceptanceReviewContent(request, e.acceptance); err != nil {
		return issuedelivery.ValidationResult{}, err
	}
	if err := e.exhaustive.Run(ctx, e.repository, deliveryevidence.SandboxFacts{
		HomeRoot: request.HomeRoot, ConfigHomeRoot: request.ConfigRoot, Sandboxed: true,
	}); err != nil {
		return issuedelivery.ValidationResult{}, fmt.Errorf("run repository validation authority: %w", err)
	}
	return e.result(ctx, request, "./scripts/validate-packy.sh", true)
}

func validateAcceptanceReviewContent(
	request issuedelivery.ValidationRequest,
	proofs []issuedelivery.AcceptanceProof,
) error {
	if len(proofs) != len(request.AcceptanceRows) {
		return errors.New("acceptance review content must prove every compiled row before exhaustive validation")
	}
	rows := make(map[string]bool, len(request.AcceptanceRows))
	for _, row := range request.AcceptanceRows {
		if row.Identity == "" || rows[row.Identity] {
			return errors.New("compiled acceptance rows are incomplete or duplicated")
		}
		rows[row.Identity] = true
	}
	seen := make(map[string]bool, len(proofs))
	for _, proof := range proofs {
		if !rows[proof.Identity] || seen[proof.Identity] {
			return errors.New("acceptance review content has a foreign or duplicate row")
		}
		seen[proof.Identity] = true
		for name, evidence := range map[string]string{
			"positive": proof.PositiveEvidence, "negative": proof.NegativeEvidence,
			"failure": proof.FailureEvidence, "mutation": proof.MutationEvidence,
			"compatibility": proof.CompatibilityEvidence,
			"preservation":  proof.PreservationEvidence,
			"migration":     proof.MigrationEvidence,
		} {
			if !strings.Contains(strings.TrimSpace(evidence), request.CandidateID) {
				return fmt.Errorf(
					"acceptance review %s evidence must bind candidate %s before exhaustive validation",
					name, request.CandidateID,
				)
			}
		}
	}
	return nil
}

func (e productionValidationExecutor) result(
	ctx context.Context,
	request issuedelivery.ValidationRequest,
	command string,
	exhaustive bool,
) (issuedelivery.ValidationResult, error) {
	head, err := e.runner.Output(ctx, "git", "-C", e.repository, "rev-parse", "HEAD^{commit}")
	if err != nil {
		return issuedelivery.ValidationResult{}, err
	}
	tree, err := e.runner.Output(ctx, "git", "-C", e.repository, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return issuedelivery.ValidationResult{}, err
	}
	status, err := e.runner.Output(ctx, "git", "-C", e.repository, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return issuedelivery.ValidationResult{}, err
	}
	result := issuedelivery.ValidationResult{
		CommitSHA: strings.TrimSpace(string(head)), TreeSHA: strings.TrimSpace(string(tree)),
		Command: command, HomeRoot: request.HomeRoot, ConfigRoot: request.ConfigRoot,
		WorkspaceClean: len(status) == 0, Sandboxed: true, Succeeded: true, Completed: true,
	}
	if result.CommitSHA != request.CommitSHA || result.TreeSHA != request.TreeSHA {
		return issuedelivery.ValidationResult{}, errors.New("candidate changed during validation")
	}
	if !exhaustive {
		return result, nil
	}
	repository, err := filepath.Abs(e.repository)
	if err != nil {
		return issuedelivery.ValidationResult{}, err
	}
	validator, err := os.ReadFile(filepath.Join(repository, "scripts", "validate-packy.sh"))
	if err != nil {
		return issuedelivery.ValidationResult{}, err
	}
	checkoutDigest := sha256.Sum256([]byte(repository))
	validatorDigest := sha256.Sum256(validator)
	result.CheckoutSHA256 = fmt.Sprintf("%x", checkoutDigest)
	result.ValidatorIdentity = "scripts/validate-packy.sh"
	result.ValidatorSHA256 = fmt.Sprintf("%x", validatorDigest)
	result.ValidatorIdentityExpiresAt = e.now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	result.Acceptance = append([]issuedelivery.AcceptanceProof(nil), e.acceptance...)
	return result, nil
}

func (e productionBoundaryExecutor) ValidateBoundary(
	ctx context.Context,
	request issuedelivery.BoundaryValidationRequest,
) (issuedelivery.BoundaryValidationResult, error) {
	if e.mu == nil {
		return issuedelivery.BoundaryValidationResult{}, errors.New("boundary validation serialization is unavailable")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	before, err := e.operatorStateDigest(ctx, request.HomeRoot, request.ConfigRoot)
	if err != nil {
		return issuedelivery.BoundaryValidationResult{}, err
	}
	sandboxBefore, err := sandboxSnapshotDigest(request.HomeRoot, request.ConfigRoot)
	if err != nil {
		return issuedelivery.BoundaryValidationResult{}, err
	}
	if err = e.validation.Run(ctx, e.repository, deliveryevidence.SandboxFacts{
		HomeRoot: request.HomeRoot, ConfigHomeRoot: request.ConfigRoot, Sandboxed: true,
	}); err != nil {
		return issuedelivery.BoundaryValidationResult{}, err
	}
	after, err := e.operatorStateDigest(ctx, request.HomeRoot, request.ConfigRoot)
	if err != nil {
		return issuedelivery.BoundaryValidationResult{}, err
	}
	if before != after {
		return issuedelivery.BoundaryValidationResult{}, errors.New(
			"protected repository state changed during sandboxed boundary validation",
		)
	}
	sandboxAfter, err := sandboxSnapshotDigest(request.HomeRoot, request.ConfigRoot)
	if err != nil {
		return issuedelivery.BoundaryValidationResult{}, err
	}
	validator, err := os.ReadFile(filepath.Join(e.repository, "scripts", "validate-packy.sh"))
	if err != nil {
		return issuedelivery.BoundaryValidationResult{}, err
	}
	validatorDigest := sha256.Sum256(validator)
	writeManifest := sha256.Sum256([]byte(strings.Join([]string{
		request.CandidateID, request.CommitSHA, request.TreeSHA,
		request.HomeRoot, sandboxBefore, request.ConfigRoot, sandboxAfter,
	}, "\x00")))
	return issuedelivery.BoundaryValidationResult{
		CandidateID: request.CandidateID, Boundary: request.Boundary,
		CommitSHA: request.CommitSHA, TreeSHA: request.TreeSHA,
		Command: "./scripts/validate-packy.sh", ToolIdentity: "scripts/validate-packy.sh",
		ToolSHA256: fmt.Sprintf("%x", validatorDigest), HomeRoot: request.HomeRoot,
		ConfigRoot: request.ConfigRoot, OperatorStateBeforeSHA256: before,
		OperatorStateAfterSHA256: after, WriteManifestSHA256: fmt.Sprintf("%x", writeManifest),
		Evidence: "repository validation completed with a content-addressed before/after manifest " +
			"of both declared sandbox roots and unchanged protected repository state",
		Sandboxed: true, Succeeded: true, Completed: true,
	}, nil
}

func sandboxSnapshotDigest(roots ...string) (string, error) {
	return filesystemSnapshotDigest(roots, nil)
}

func filesystemSnapshotDigest(roots, excluded []string) (string, error) {
	hash := sha256.New()
	roots = nonOverlappingCanonicalPaths(roots)
	excluded = nonOverlappingCanonicalPaths(excluded)
	for _, root := range roots {
		root = filepath.Clean(root)
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("snapshot sandbox root %q: real directory required", root)
		}
		err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path != root && pathWithinAny(path, excluded) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%d\x00%d\x00", root, relative, info.Mode(), info.Size())
			switch {
			case info.Mode().IsRegular():
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				contentDigest := sha256.Sum256(content)
				_, _ = hash.Write(contentDigest[:])
			case info.Mode()&os.ModeSymlink != 0:
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				_, _ = hash.Write([]byte(target))
			}
			_, _ = hash.Write([]byte{0})
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("snapshot sandbox root %q: %w", root, err)
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func nonOverlappingCanonicalPaths(values []string) []string {
	values = append([]string(nil), values...)
	sort.Slice(values, func(i, j int) bool {
		return len(filepath.Clean(values[i])) < len(filepath.Clean(values[j]))
	})
	var out []string
	for _, value := range values {
		value = filepath.Clean(value)
		covered := false
		for _, existing := range out {
			if value == existing || isDescendantPath(value, existing) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func pathWithinAny(path string, roots []string) bool {
	for _, root := range roots {
		if path == root || isDescendantPath(path, root) {
			return true
		}
	}
	return false
}

func isDescendantPath(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (e productionBoundaryExecutor) operatorStateDigest(
	ctx context.Context,
	sandboxRoots ...string,
) (string, error) {
	status, err := e.runner.Output(ctx, "git", "-C", e.repository, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return "", err
	}
	worktrees, err := e.runner.Output(ctx, "git", "-C", e.repository, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	commonRaw, err := e.runner.Output(
		ctx, "git", "-C", e.repository, "rev-parse", "--path-format=absolute", "--git-common-dir",
	)
	if err != nil {
		return "", err
	}
	common := filepath.Clean(strings.TrimSpace(string(commonRaw)))
	if !filepath.IsAbs(common) {
		return "", errors.New("protected Git common directory is not absolute")
	}
	protected, err := filesystemSnapshotDigest(
		[]string{e.repository, common},
		append(append([]string(nil), sandboxRoots...), filepath.Join(common, "objects")),
	)
	if err != nil {
		return "", err
	}
	payload := strings.Join([]string{string(status), string(worktrees), protected}, "\x00")
	digest := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", digest), nil
}
