package githubproposal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

// Command is one argument-safe git or gh invocation. Environment entries are
// overrides applied to the publication process allowlist.
type Command struct {
	Directory   string
	Executable  string
	Arguments   []string
	Environment []string
}

type Runner interface {
	Run(context.Context, Command) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, command Command) (string, error) {
	process := exec.CommandContext(ctx, command.Executable, command.Arguments...)
	process.Dir = command.Directory
	process.Env = publicationEnvironment(os.Environ(), command.Environment)
	output, err := process.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w: %s", command.Executable, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

var publicationEnvironmentKeys = map[string]bool{
	"GCM_INTERACTIVE":         true,
	"GH_CONFIG_DIR":           true,
	"GH_ENTERPRISE_TOKEN":     true,
	"GH_HOST":                 true,
	"GH_PROMPT_DISABLED":      true,
	"GH_REPO":                 true,
	"GH_TOKEN":                true,
	"GITHUB_ENTERPRISE_TOKEN": true,
	"GITHUB_TOKEN":            true,
	"GIT_AUTHOR_EMAIL":        true,
	"GIT_AUTHOR_NAME":         true,
	"GIT_COMMITTER_EMAIL":     true,
	"GIT_COMMITTER_NAME":      true,
	"GIT_CONFIG_GLOBAL":       true,
	"GIT_CONFIG_NOSYSTEM":     true,
	"GIT_TERMINAL_PROMPT":     true,
	"HOME":                    true,
	"LANG":                    true,
	"LC_ALL":                  true,
	"LC_CTYPE":                true,
	"PATH":                    true,
	"SSH_AUTH_SOCK":           true,
	"SSL_CERT_DIR":            true,
	"SSL_CERT_FILE":           true,
	"TMPDIR":                  true,
	"XDG_CONFIG_HOME":         true,
}

func publicationEnvironment(inherited, overrides []string) []string {
	values := map[string]string{
		"GCM_INTERACTIVE":     "Never",
		"GH_PROMPT_DISABLED":  "1",
		"GIT_TERMINAL_PROMPT": "0",
	}
	for _, entry := range append(append([]string(nil), inherited...), overrides...) {
		key, value, ok := strings.Cut(entry, "=")
		if ok && publicationEnvironmentKeys[key] {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment
}

// NewCLI constructs the production git/gh-backed Publisher.
func NewCLI() *Adapter {
	return NewCLIWithRunner(ExecRunner{})
}

// NewCLIWithRunner exposes the argument-safe command boundary for hermetic
// adapter tests.
func NewCLIWithRunner(runner Runner) *Adapter {
	return New(&cliGateway{runner: runner})
}

type cliGateway struct {
	runner Runner
}

func (gateway *cliGateway) VerifyCandidate(ctx context.Context, candidate managedpackpromotion.Candidate) error {
	output, err := gateway.run(ctx, candidate.RepositoryRoot, "git", "show", "-s", "--format=%T%x00%P", candidate.HeadSHA)
	if err != nil {
		return err
	}
	fields := strings.Split(strings.TrimSpace(output), "\x00")
	if len(fields) != 2 || fields[0] != candidate.ResultTreeSHA {
		return errors.New("candidate commit does not have the sealed result tree")
	}
	parents := strings.Fields(fields[1])
	if len(parents) != 1 || parents[0] != candidate.BaseSHA {
		return errors.New("candidate commit is not based directly on the sealed origin/main commit")
	}
	return nil
}

func (gateway *cliGateway) Observe(ctx context.Context, candidate managedpackpromotion.Candidate) (Observation, error) {
	actor, err := gateway.run(ctx, candidate.RepositoryRoot, "gh", "api", "user", "--jq", ".login")
	if err != nil {
		return Observation{}, err
	}
	actor = strings.TrimSpace(actor)

	refs, err := gateway.run(ctx, candidate.RepositoryRoot, "git", "ls-remote", "origin", "refs/heads/main", "refs/heads/"+candidate.Branch)
	if err != nil {
		return Observation{}, err
	}
	base, branchHead, err := parseRemoteRefs(refs, candidate.Branch)
	if err != nil {
		return Observation{}, err
	}

	pullRequests, err := gateway.observePullRequests(ctx, candidate)
	if err != nil {
		return Observation{}, err
	}
	observation := Observation{Actor: actor, BaseSHA: base, PullRequests: pullRequests}
	if branchHead == "" {
		return observation, nil
	}

	if _, err := gateway.run(ctx, candidate.RepositoryRoot, "git", "fetch", "--no-tags", "origin", "refs/heads/"+candidate.Branch); err != nil {
		return Observation{}, err
	}
	fetched, err := gateway.run(ctx, candidate.RepositoryRoot, "git", "rev-parse", "FETCH_HEAD^{commit}")
	if err != nil {
		return Observation{}, err
	}
	if strings.TrimSpace(fetched) != branchHead {
		return Observation{}, errors.New("remote proposal branch moved during observation")
	}

	baseAncestor := true
	if _, err := gateway.run(ctx, candidate.RepositoryRoot, "git", "merge-base", "--is-ancestor", base, branchHead); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
			baseAncestor = false
		} else {
			return Observation{}, err
		}
	}
	commitsOutput, err := gateway.run(ctx, candidate.RepositoryRoot, "git", "log", "--format=%H%x1f%P%x1f%T%x1f%an%x1f%ae%x1f%cn%x1f%ce%x1f%B%x1e", base+".."+branchHead)
	if err != nil {
		return Observation{}, err
	}
	commits, err := parseCommits(commitsOutput)
	if err != nil {
		return Observation{}, err
	}
	tree := ""
	if len(commits) > 0 {
		tree = commits[0].TreeSHA
	}
	observation.Branch = &Branch{
		Name: candidate.Branch, HeadSHA: branchHead, TreeSHA: tree,
		BaseAncestor: baseAncestor, Commits: commits,
	}
	return observation, nil
}

func (gateway *cliGateway) Commit(ctx context.Context, request CommitRequest) (string, error) {
	if len(request.ParentSHAs) == 0 || len(request.ParentSHAs) > 2 {
		return "", errors.New("publication commit requires one or two parents")
	}
	arguments := []string{"commit-tree", request.TreeSHA}
	for _, parent := range request.ParentSHAs {
		if !sha1Pattern.MatchString(parent) {
			return "", errors.New("publication commit has a malformed parent identity")
		}
		arguments = append(arguments, "-p", parent)
	}
	arguments = append(arguments, "-m", request.Message)
	output, err := gateway.runWithEnvironment(ctx, request.RepositoryRoot, []string{
		"GIT_AUTHOR_NAME=" + botAuthorName,
		"GIT_AUTHOR_EMAIL=" + botAuthorEmail,
		"GIT_COMMITTER_NAME=" + botAuthorName,
		"GIT_COMMITTER_EMAIL=" + botAuthorEmail,
	}, "git", arguments...)
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(output)
	if !sha1Pattern.MatchString(head) {
		return "", errors.New("git commit-tree returned a malformed commit identity")
	}
	verify, err := gateway.run(ctx, request.RepositoryRoot, "git", "show", "-s", "--format=%T%x00%P", head)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(verify) != request.TreeSHA+"\x00"+strings.Join(request.ParentSHAs, " ") {
		return "", errors.New("created publication commit does not have the requested tree and parents")
	}
	return head, nil
}

func (gateway *cliGateway) FastForwardPush(ctx context.Context, repositoryRoot, head, branch string) error {
	_, err := gateway.run(ctx, repositoryRoot, "git", "push", "origin", head+":refs/heads/"+branch)
	return err
}

func (gateway *cliGateway) CreatePullRequest(ctx context.Context, mutation PullRequestMutation) error {
	_, err := gateway.run(ctx, mutation.RepositoryRoot, "gh", "pr", "create",
		"--base", mutation.BaseBranch, "--head", mutation.HeadBranch,
		"--title", mutation.Title, "--body", mutation.Body)
	return err
}

func (gateway *cliGateway) EditPullRequest(ctx context.Context, number int, mutation PullRequestMutation) error {
	_, err := gateway.run(ctx, mutation.RepositoryRoot, "gh", "pr", "edit", strconv.Itoa(number),
		"--title", mutation.Title, "--body", mutation.Body)
	return err
}

type listedPullRequest struct {
	Number           int             `json:"number"`
	URL              string          `json:"url"`
	State            string          `json:"state"`
	Draft            bool            `json:"isDraft"`
	BaseBranch       string          `json:"baseRefName"`
	HeadBranch       string          `json:"headRefName"`
	HeadSHA          string          `json:"headRefOid"`
	Title            string          `json:"title"`
	Body             string          `json:"body"`
	AutoMergeRequest json.RawMessage `json:"autoMergeRequest"`
}

func (gateway *cliGateway) observePullRequests(ctx context.Context, candidate managedpackpromotion.Candidate) ([]PullRequest, error) {
	output, err := gateway.run(ctx, candidate.RepositoryRoot, "gh", "pr", "list", "--state", "all", "--head", candidate.Branch,
		"--limit", "1000", "--json", "number,url,state,isDraft,baseRefName,headRefName,headRefOid,title,body,autoMergeRequest")
	if err != nil {
		return nil, err
	}
	var listed []listedPullRequest
	if err := json.Unmarshal([]byte(output), &listed); err != nil {
		return nil, fmt.Errorf("decode gh pr list: %w", err)
	}
	if len(listed) == 0 {
		return []PullRequest{}, nil
	}

	nameWithOwner, err := gateway.run(ctx, candidate.RepositoryRoot, "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return nil, err
	}
	owner, name, ok := strings.Cut(strings.TrimSpace(nameWithOwner), "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return nil, errors.New("gh repo view returned an invalid repository identity")
	}

	pullRequests := make([]PullRequest, 0, len(listed))
	for _, item := range listed {
		author, editor, err := gateway.observeEditors(ctx, candidate.RepositoryRoot, owner, name, item.Number)
		if err != nil {
			return nil, err
		}
		autoMerge := len(item.AutoMergeRequest) != 0 && string(item.AutoMergeRequest) != "null"
		pullRequests = append(pullRequests, PullRequest{
			Number: item.Number, URL: item.URL, State: item.State, Draft: item.Draft, AutoMerge: autoMerge,
			BaseBranch: item.BaseBranch, HeadBranch: item.HeadBranch, HeadSHA: item.HeadSHA,
			Title: item.Title, Body: item.Body, Author: author, LastEditor: editor,
		})
	}
	return pullRequests, nil
}

const pullRequestEditorsQuery = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){pullRequest(number:$number){author{login}userContentEdits(last:1){nodes{editor{login}}}}}}`

const unavailableEditor = "<unavailable-user-content-editor>"

func (gateway *cliGateway) observeEditors(ctx context.Context, repositoryRoot, owner, name string, number int) (string, string, error) {
	output, err := gateway.run(ctx, repositoryRoot, "gh", "api", "graphql",
		"-F", "owner="+owner, "-F", "name="+name, "-F", "number="+strconv.Itoa(number), "-f", "query="+pullRequestEditorsQuery)
	if err != nil {
		return "", "", err
	}
	var response struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					Author *struct {
						Login string `json:"login"`
					} `json:"author"`
					UserContentEdits struct {
						Nodes []struct {
							Editor *struct {
								Login string `json:"login"`
							} `json:"editor"`
						} `json:"nodes"`
					} `json:"userContentEdits"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return "", "", fmt.Errorf("decode pull request editor observation: %w", err)
	}
	if response.Data.Repository.PullRequest.Author == nil || response.Data.Repository.PullRequest.Author.Login == "" {
		return "", "", errors.New("pull request has no observable author")
	}
	editor := ""
	edits := response.Data.Repository.PullRequest.UserContentEdits.Nodes
	if len(edits) > 0 {
		last := edits[len(edits)-1]
		if last.Editor == nil || last.Editor.Login == "" {
			editor = unavailableEditor
		} else {
			editor = last.Editor.Login
		}
	}
	return response.Data.Repository.PullRequest.Author.Login, editor, nil
}

func parseRemoteRefs(output, branch string) (string, string, error) {
	refs := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || !sha1Pattern.MatchString(fields[0]) {
			return "", "", errors.New("git ls-remote returned malformed output")
		}
		if _, duplicate := refs[fields[1]]; duplicate {
			return "", "", errors.New("git ls-remote returned a duplicate ref")
		}
		refs[fields[1]] = fields[0]
	}
	base := refs["refs/heads/main"]
	if base == "" {
		return "", "", errors.New("origin/main is not observable")
	}
	return base, refs["refs/heads/"+branch], nil
}

func parseCommits(output string) ([]Commit, error) {
	records := strings.Split(output, "\x1e")
	commits := make([]Commit, 0, len(records))
	for _, raw := range records {
		raw = strings.TrimPrefix(raw, "\n")
		if strings.TrimSpace(raw) == "" {
			continue
		}
		fields := strings.SplitN(raw, "\x1f", 8)
		if len(fields) != 8 || !sha1Pattern.MatchString(fields[0]) || !sha1Pattern.MatchString(fields[2]) {
			return nil, errors.New("git log returned malformed publication history")
		}
		parents := strings.Fields(fields[1])
		for _, parent := range parents {
			if !sha1Pattern.MatchString(parent) {
				return nil, errors.New("git log returned a malformed parent identity")
			}
		}
		commits = append(commits, Commit{
			SHA: fields[0], Parents: parents, TreeSHA: fields[2], AuthorName: fields[3], AuthorEmail: fields[4],
			CommitterName: fields[5], CommitterEmail: fields[6], Message: strings.TrimRight(fields[7], "\n"),
		})
	}
	return commits, nil
}

func (gateway *cliGateway) run(ctx context.Context, directory, executable string, arguments ...string) (string, error) {
	return gateway.runWithEnvironment(ctx, directory, nil, executable, arguments...)
}

func (gateway *cliGateway) runWithEnvironment(ctx context.Context, directory string, environment []string, executable string, arguments ...string) (string, error) {
	if gateway == nil || gateway.runner == nil {
		return "", errors.New("command runner is required")
	}
	return gateway.runner.Run(ctx, Command{
		Directory: directory, Executable: executable,
		Arguments: append([]string(nil), arguments...), Environment: append([]string(nil), environment...),
	})
}
