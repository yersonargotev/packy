package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/packy/internal/issuedelivery"
)

const packyRepositorySlug = "yersonargotev/packy"

type productionGitObserver struct {
	runner Runner
}

func (o productionGitObserver) ObserveGit(ctx context.Context, repository string) (issuedelivery.GitObservation, error) {
	if o.runner == nil {
		return issuedelivery.GitObservation{}, errors.New("Git runner is required")
	}
	common, err := gitOutput(ctx, o.runner, repository, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return issuedelivery.GitObservation{}, fmt.Errorf("observe Git common directory: %w", err)
	}
	worktree, err := gitOutput(ctx, o.runner, repository, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return issuedelivery.GitObservation{}, fmt.Errorf("observe Git worktree: %w", err)
	}
	origin, err := gitOutput(ctx, o.runner, repository, "remote", "get-url", "origin")
	if err != nil {
		return issuedelivery.GitObservation{}, fmt.Errorf("observe Git origin: %w", err)
	}
	slug, err := githubSlug(origin)
	if err != nil || slug != packyRepositorySlug {
		return issuedelivery.GitObservation{}, errors.New("origin is not the Packy repository")
	}
	base, err := gitOutput(ctx, o.runner, repository, "rev-parse", "origin/main^{commit}")
	if err != nil {
		return issuedelivery.GitObservation{}, fmt.Errorf("observe origin/main: %w", err)
	}
	head, err := gitOutput(ctx, o.runner, repository, "rev-parse", "HEAD^{commit}")
	if err != nil {
		return issuedelivery.GitObservation{}, fmt.Errorf("observe HEAD: %w", err)
	}
	tree, err := gitOutput(ctx, o.runner, repository, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return issuedelivery.GitObservation{}, fmt.Errorf("observe HEAD tree: %w", err)
	}
	status, err := gitRaw(ctx, o.runner, repository, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return issuedelivery.GitObservation{}, fmt.Errorf("observe workspace status: %w", err)
	}
	branch, err := gitOutput(ctx, o.runner, repository, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return issuedelivery.GitObservation{}, errors.New("detached HEAD is not a delivery workspace")
	}
	return issuedelivery.GitObservation{
		CommonDir: filepath.Clean(common), Worktree: filepath.Clean(worktree), OriginURL: origin,
		Owner: "yersonargotev", Name: "packy", StartingBaseSHA: base, HeadSHA: head,
		TreeSHA: tree, WorkspaceClean: len(status) == 0, Branch: branch,
	}, nil
}

type productionCandidateRiskObserver struct {
	runner Runner
}

func (o productionCandidateRiskObserver) ObserveCandidateRisk(
	ctx context.Context,
	request issuedelivery.CandidateRiskRequest,
) (issuedelivery.CandidateRiskObservation, error) {
	if o.runner == nil {
		return issuedelivery.CandidateRiskObservation{}, errors.New("Git runner is required")
	}
	head, err := gitOutput(ctx, o.runner, request.RepositoryPath, "rev-parse", request.CommitSHA+"^{commit}")
	if err != nil || head != request.CommitSHA {
		return issuedelivery.CandidateRiskObservation{}, errors.New("candidate commit is absent or inexact")
	}
	tree, err := gitOutput(ctx, o.runner, request.RepositoryPath, "rev-parse", request.CommitSHA+"^{tree}")
	if err != nil || tree != request.TreeSHA {
		return issuedelivery.CandidateRiskObservation{}, errors.New("candidate tree is absent or inexact")
	}
	raw, err := gitRaw(ctx, o.runner, request.RepositoryPath, "diff", "--name-status", "-z",
		request.StartingBaseSHA+".."+request.CommitSHA, "--")
	if err != nil {
		return issuedelivery.CandidateRiskObservation{}, fmt.Errorf("observe candidate diff: %w", err)
	}
	paths, complete := diffPaths(raw)
	effects := classifyCandidatePaths(paths)
	if len(paths) == 0 {
		complete = false
		effects = []issuedelivery.EffectObservation{{
			Effect: issuedelivery.EffectGovernance, Evidence: "candidate diff is empty or unavailable", Complete: false,
		}}
	}
	for index := range effects {
		effects[index].Complete = effects[index].Complete && complete
	}
	return issuedelivery.CandidateRiskObservation{
		CandidateID: request.CandidateID, CommitSHA: head, TreeSHA: tree,
		Effects: effects, Completed: complete,
	}, nil
}

func diffPaths(raw []byte) ([]string, bool) {
	fields := strings.Split(string(raw), "\x00")
	paths := make([]string, 0, len(fields)/2)
	for index := 0; index < len(fields)-1; {
		status := fields[index]
		index++
		if status == "" || index >= len(fields)-1 {
			return nil, false
		}
		path := fields[index]
		index++
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			if index >= len(fields)-1 {
				return nil, false
			}
			path = fields[index]
			index++
		}
		if path == "" || filepath.IsAbs(path) || strings.HasPrefix(filepath.Clean(path), "..") {
			return nil, false
		}
		paths = append(paths, filepath.ToSlash(filepath.Clean(path)))
	}
	sort.Strings(paths)
	return paths, len(fields) > 1 && fields[len(fields)-1] == ""
}

func classifyCandidatePaths(paths []string) []issuedelivery.EffectObservation {
	grouped := map[issuedelivery.CandidateEffect][]string{}
	for _, path := range paths {
		effect := candidatePathEffect(path)
		grouped[effect] = append(grouped[effect], path)
	}
	effects := make([]issuedelivery.EffectObservation, 0, len(grouped))
	for effect, effectPaths := range grouped {
		effects = append(effects, issuedelivery.EffectObservation{
			Effect: effect, Evidence: strings.Join(effectPaths, ", "), Complete: true,
		})
	}
	sort.Slice(effects, func(i, j int) bool { return effects[i].Effect < effects[j].Effect })
	return effects
}

func candidatePathEffect(path string) issuedelivery.CandidateEffect {
	lower := strings.ToLower(path)
	switch {
	case containsAny(lower, "uninstall", "destructive", "delete_", "remove_"):
		return issuedelivery.EffectDestructive
	case strings.HasPrefix(lower, ".github/workflows/") ||
		containsAny(lower, "release", "publish", "goreleaser", "package"):
		return issuedelivery.EffectPublication
	case containsAny(lower, "security", "auth", "permission", "credential", "secret", "crypto"):
		return issuedelivery.EffectSecurity
	case containsAny(lower, "migration", "migrate"):
		return issuedelivery.EffectMigration
	case containsAny(lower, "schema", "persistent", "serialization", "record_v"):
		return issuedelivery.EffectPersistentFormat
	case strings.HasPrefix(lower, "docs/adr/") || strings.HasSuffix(lower, "agents.md") ||
		containsAny(lower, "codeowners", "governance", "policy"):
		return issuedelivery.EffectGovernance
	case containsAny(lower, "install", "bootstrap", "setup"):
		return issuedelivery.EffectInstallation
	case containsAny(lower, "config", "dotfile", "corelifecycle"):
		return issuedelivery.EffectRealConfiguration
	case strings.HasSuffix(lower, "_test.go") || strings.HasPrefix(lower, "docs/") ||
		strings.HasSuffix(lower, ".md") || strings.HasPrefix(lower, "testdata/") ||
		strings.HasPrefix(lower, "scripts/validate"):
		return issuedelivery.EffectPassive
	case strings.HasSuffix(lower, ".go"):
		return issuedelivery.EffectOrdinaryBehavior
	default:
		return issuedelivery.EffectGovernance
	}
}

func containsAny(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

type productionLocalCompletionGateway struct {
	runner Runner
}

func (g productionLocalCompletionGateway) ObserveLocalCompletion(
	ctx context.Context,
	request issuedelivery.LocalCompletionObserveRequest,
) (issuedelivery.LocalCompletionObservation, error) {
	if g.runner == nil {
		return issuedelivery.LocalCompletionObservation{}, errors.New("Git runner is required")
	}
	common, err := gitOutput(ctx, g.runner, request.RepositoryPath, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || filepath.Clean(common) != filepath.Clean(request.CommonDir) {
		return issuedelivery.LocalCompletionObservation{}, errors.New("Git common directory changed")
	}
	integrationPath, err := gitOutput(ctx, g.runner, request.RepositoryPath, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil || filepath.Clean(integrationPath) != filepath.Clean(request.RepositoryPath) {
		return issuedelivery.LocalCompletionObservation{}, errors.New("integration workspace changed")
	}
	branch, _ := gitOutput(ctx, g.runner, request.RepositoryPath, "symbolic-ref", "--quiet", "--short", "HEAD")
	status, err := gitRaw(ctx, g.runner, request.RepositoryPath, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return issuedelivery.LocalCompletionObservation{}, err
	}
	worktrees, err := g.observeOwnedWorktrees(ctx, request)
	if err != nil {
		return issuedelivery.LocalCompletionObservation{}, err
	}
	localBranch := g.observeLocalBranch(ctx, request.RepositoryPath, request.IssueBranch)
	localMain, err := g.observeLocalMain(ctx, request.RepositoryPath)
	if err != nil {
		return issuedelivery.LocalCompletionObservation{}, err
	}
	operatorDigest, err := g.operatorStateDigest(ctx, request, integrationPath, branch, status)
	if err != nil {
		return issuedelivery.LocalCompletionObservation{}, err
	}
	return issuedelivery.LocalCompletionObservation{
		OperatorStateSHA256: operatorDigest,
		Integration: issuedelivery.IntegrationWorkspaceObservation{
			Path: filepath.Clean(integrationPath), Branch: branch, Clean: len(status) == 0,
		},
		Worktrees: worktrees, LocalBranch: localBranch, LocalMain: localMain,
	}, nil
}

func (g productionLocalCompletionGateway) observeOwnedWorktrees(
	ctx context.Context,
	request issuedelivery.LocalCompletionObserveRequest,
) ([]issuedelivery.ManagedWorktreeObservation, error) {
	raw, err := gitRaw(ctx, g.runner, request.RepositoryPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, fmt.Errorf("observe worktrees: %w", err)
	}
	entries, ok := parseWorktrees(raw)
	if !ok {
		return nil, errors.New("worktree observation is incomplete")
	}
	owned := []issuedelivery.ManagedWorktreeObservation{}
	for _, entry := range entries {
		if entry.path == filepath.Clean(request.RepositoryPath) ||
			entry.branch != request.IssueBranch || entry.head != request.CandidateHead {
			continue
		}
		runID, runErr := gitOutput(ctx, g.runner, entry.path, "config", "--worktree", "--get", "packy.delivery-run-id")
		candidateID, candidateErr := gitOutput(ctx, g.runner, entry.path, "config", "--worktree", "--get", "packy.delivery-candidate-id")
		if runErr != nil || candidateErr != nil || runID != request.RunID || candidateID != request.CandidateID {
			continue
		}
		status, statusErr := gitRaw(ctx, g.runner, entry.path, "status", "--porcelain=v1", "--untracked-files=all")
		if statusErr != nil {
			return nil, statusErr
		}
		owned = append(owned, issuedelivery.ManagedWorktreeObservation{
			Path: entry.path, Branch: entry.branch, HeadSHA: entry.head, RunID: runID,
			CandidateID: candidateID, Clean: len(status) == 0,
		})
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].Path < owned[j].Path })
	return owned, nil
}

type worktreeEntry struct {
	path, branch, head string
}

func parseWorktrees(raw []byte) ([]worktreeEntry, bool) {
	records := strings.Split(string(raw), "\x00")
	entries := []worktreeEntry{}
	var current *worktreeEntry
	for _, record := range records {
		switch {
		case strings.HasPrefix(record, "worktree "):
			if current != nil {
				entries = append(entries, *current)
			}
			current = &worktreeEntry{path: filepath.Clean(strings.TrimPrefix(record, "worktree "))}
		case strings.HasPrefix(record, "HEAD ") && current != nil:
			current.head = strings.TrimPrefix(record, "HEAD ")
		case strings.HasPrefix(record, "branch refs/heads/") && current != nil:
			current.branch = strings.TrimPrefix(record, "branch refs/heads/")
		}
	}
	if current != nil {
		entries = append(entries, *current)
	}
	for _, entry := range entries {
		if !filepath.IsAbs(entry.path) || entry.head == "" {
			return nil, false
		}
	}
	return entries, len(entries) > 0
}

func (g productionLocalCompletionGateway) observeLocalBranch(
	ctx context.Context,
	repository, branch string,
) *issuedelivery.LocalBranchObservation {
	if strings.TrimSpace(branch) == "" {
		return nil
	}
	head, err := gitOutput(ctx, g.runner, repository, "show-ref", "--verify", "--hash", "refs/heads/"+branch)
	if err != nil {
		return nil
	}
	return &issuedelivery.LocalBranchObservation{Name: branch, HeadSHA: head}
}

func (g productionLocalCompletionGateway) observeLocalMain(
	ctx context.Context,
	repository string,
) (issuedelivery.LocalMainObservation, error) {
	origin, err := gitOutput(ctx, g.runner, repository, "rev-parse", "origin/main^{commit}")
	if err != nil {
		return issuedelivery.LocalMainObservation{}, fmt.Errorf("observe origin/main: %w", err)
	}
	head, err := gitOutput(ctx, g.runner, repository, "show-ref", "--verify", "--hash", "refs/heads/main")
	if err != nil {
		return issuedelivery.LocalMainObservation{OriginHeadSHA: origin}, nil
	}
	relation := issuedelivery.LocalMainDiverged
	switch {
	case head == origin:
		relation = issuedelivery.LocalMainSynced
	case gitSucceeds(ctx, g.runner, repository, "merge-base", "--is-ancestor", head, origin):
		relation = issuedelivery.LocalMainBehind
	case gitSucceeds(ctx, g.runner, repository, "merge-base", "--is-ancestor", origin, head):
		relation = issuedelivery.LocalMainAhead
	}
	clean := true
	current, currentErr := gitOutput(ctx, g.runner, repository, "symbolic-ref", "--quiet", "--short", "HEAD")
	if currentErr == nil && current == "main" {
		status, statusErr := gitRaw(ctx, g.runner, repository, "status", "--porcelain=v1", "--untracked-files=all")
		if statusErr != nil {
			return issuedelivery.LocalMainObservation{}, statusErr
		}
		clean = len(status) == 0
	}
	return issuedelivery.LocalMainObservation{
		Exists: true, HeadSHA: head, OriginHeadSHA: origin, Relation: relation, Clean: clean,
	}, nil
}

func (g productionLocalCompletionGateway) operatorStateDigest(
	ctx context.Context,
	request issuedelivery.LocalCompletionObserveRequest,
	integrationPath, branch string,
	status []byte,
) (string, error) {
	head, err := gitOutput(ctx, g.runner, request.RepositoryPath, "rev-parse", "HEAD^{commit}")
	if err != nil {
		return "", err
	}
	payload := strings.Join([]string{
		filepath.Clean(request.CommonDir), filepath.Clean(integrationPath),
		operatorBranchIdentity(branch, head, request.IssueBranch, request.CandidateHead),
		operatorHeadIdentity(branch, head, request.IssueBranch, request.CandidateHead),
		string(status),
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:]), nil
}

func operatorBranchIdentity(branch, head, issueBranch, candidateHead string) string {
	if branch == "main" || branch == issueBranch && head == candidateHead {
		return "workflow-managed-integration"
	}
	return branch
}

func operatorHeadIdentity(branch, head, issueBranch, candidateHead string) string {
	if branch == "main" || branch == issueBranch && head == candidateHead {
		// Moving the exact clean candidate worktree to main and fast-forwarding
		// main are workflow-owned cleanup. Their identities are checked
		// independently, so they must not look like operator mutations.
		return "workflow-managed-main"
	}
	return head
}

func (g productionLocalCompletionGateway) EnsureManagedWorktreeAbsent(
	ctx context.Context,
	request issuedelivery.RemoveManagedWorktreeRequest,
) error {
	observation, err := g.ObserveLocalCompletion(ctx, issuedelivery.LocalCompletionObserveRequest{
		RunID: request.RunID, CandidateID: request.CandidateID, CommonDir: request.CommonDir,
		RepositoryPath: request.RepositoryPath, IssueBranch: request.Branch, CandidateHead: request.HeadSHA,
	})
	if err != nil {
		return err
	}
	found := false
	for _, worktree := range observation.Worktrees {
		if worktree.Path == request.Path && worktree.Branch == request.Branch && worktree.HeadSHA == request.HeadSHA {
			found = true
		}
	}
	if !found {
		return nil
	}
	if _, err := g.runner.Output(ctx, "git", "-C", request.RepositoryPath, "worktree", "remove", request.Path); err != nil {
		return fmt.Errorf("remove exact managed worktree: %w", err)
	}
	return nil
}

func (g productionLocalCompletionGateway) EnsureLocalIssueBranchAbsent(
	ctx context.Context,
	request issuedelivery.DeleteLocalIssueBranchRequest,
) error {
	head, err := gitOutput(ctx, g.runner, request.RepositoryPath, "show-ref", "--verify", "--hash", "refs/heads/"+request.Branch)
	if err != nil {
		return nil
	}
	if head != request.HeadSHA {
		return errors.New("local issue branch changed before cleanup")
	}
	if !gitSucceeds(ctx, g.runner, request.RepositoryPath, "merge-base", "--is-ancestor", request.HeadSHA, "origin/main") {
		return errors.New("local issue branch is not contained by origin/main")
	}
	current, currentErr := gitOutput(
		ctx, g.runner, request.RepositoryPath, "symbolic-ref", "--quiet", "--short", "HEAD",
	)
	if currentErr == nil && current == request.Branch {
		status, statusErr := gitRaw(
			ctx, g.runner, request.RepositoryPath, "status", "--porcelain=v1", "--untracked-files=all",
		)
		if statusErr != nil || len(status) != 0 {
			return errors.New("checked-out local issue branch is not clean for exact cleanup")
		}
		if _, err := g.runner.Output(
			ctx, "git", "-C", request.RepositoryPath, "switch", "main",
		); err != nil {
			return fmt.Errorf("move exact delivery worktree to local main: %w", err)
		}
	}
	if _, err := g.runner.Output(
		ctx, "git", "-C", request.RepositoryPath, "update-ref", "-d",
		"refs/heads/"+request.Branch, request.HeadSHA,
	); err != nil {
		return fmt.Errorf("delete exact merged local issue branch: %w", err)
	}
	return nil
}

func (g productionLocalCompletionGateway) EnsureLocalMainFastForward(
	ctx context.Context,
	request issuedelivery.FastForwardLocalMainRequest,
) error {
	main, err := gitOutput(ctx, g.runner, request.RepositoryPath, "show-ref", "--verify", "--hash", "refs/heads/main")
	if err != nil || main != request.ExpectedOldSHA {
		return errors.New("local main changed before compare-and-swap")
	}
	origin, err := gitOutput(ctx, g.runner, request.RepositoryPath, "rev-parse", "origin/main^{commit}")
	if err != nil || origin != request.OriginMainSHA {
		return errors.New("origin/main changed before compare-and-swap")
	}
	if !gitSucceeds(ctx, g.runner, request.RepositoryPath, "merge-base", "--is-ancestor", request.ExpectedOldSHA, request.OriginMainSHA) ||
		!gitSucceeds(ctx, g.runner, request.RepositoryPath, "merge-base", "--is-ancestor", request.MergeCommitSHA, request.OriginMainSHA) {
		return errors.New("requested local main update is not the verified fast-forward")
	}
	current, currentErr := gitOutput(
		ctx, g.runner, request.RepositoryPath, "symbolic-ref", "--quiet", "--short", "HEAD",
	)
	if currentErr == nil && current == "main" {
		if _, err := g.runner.Output(
			ctx, "git", "-C", request.RepositoryPath, "merge", "--ff-only", request.OriginMainSHA,
		); err != nil {
			return fmt.Errorf("fast-forward checked-out local main: %w", err)
		}
		return nil
	}
	raw, err := gitRaw(ctx, g.runner, request.RepositoryPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return fmt.Errorf("observe local main checkout ownership: %w", err)
	}
	entries, complete := parseWorktrees(raw)
	if !complete {
		return errors.New("local main checkout observation is incomplete")
	}
	for _, entry := range entries {
		if entry.branch == "main" {
			return errors.New("local main is checked out in another worktree; preserve that operator checkout")
		}
	}
	if _, err := g.runner.Output(ctx, "git", "-C", request.RepositoryPath, "update-ref",
		"refs/heads/main", request.OriginMainSHA, request.ExpectedOldSHA); err != nil {
		return fmt.Errorf("compare-and-swap local main: %w", err)
	}
	return nil
}

func gitOutput(ctx context.Context, runner Runner, repository string, args ...string) (string, error) {
	raw, err := gitRaw(ctx, runner, repository, args...)
	return strings.TrimSpace(string(raw)), err
}

func gitRaw(ctx context.Context, runner Runner, repository string, args ...string) ([]byte, error) {
	return runner.Output(ctx, "git", append([]string{"-C", repository}, args...)...)
}

func gitSucceeds(ctx context.Context, runner Runner, repository string, args ...string) bool {
	_, err := gitRaw(ctx, runner, repository, args...)
	return err == nil
}
