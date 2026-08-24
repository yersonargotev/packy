package githubsource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	gitHubAPIVersion        = "2022-11-28"
	maxGitTreeResponseBytes = 256 << 20
	maxGitTreeExpandedBytes = 1 << 30
	maxGitTreeFileBytes     = 128 << 20
	maxGitTreeEntries       = 100_000
	maxGitTreePathDepth     = 64
	maxAnnotatedTagObjects  = 32
)

type gitTreeLimits struct {
	maxExpandedBytes int64
	maxFileBytes     int64
	maxEntries       int
	maxPathDepth     int
}

var defaultGitTreeLimits = gitTreeLimits{
	maxExpandedBytes: maxGitTreeExpandedBytes,
	maxFileBytes:     maxGitTreeFileBytes,
	maxEntries:       maxGitTreeEntries,
	maxPathDepth:     maxGitTreePathDepth,
}

// Client acquires public GitHub release metadata and exact Git objects with a
// caller-supplied HTTP client. It never adds credentials or chooses retry
// policy for its caller.
type Client struct {
	httpClient *http.Client
	apiBase    string
	rawBase    string
	treeLimits gitTreeLimits
}

// HTTPError preserves rate-limit and service response metadata for the
// workflow-owned retry policy.
type HTTPError struct {
	Operation          string
	StatusCode         int
	Status             string
	RetryAfter         string
	RateLimitRemaining string
	RateLimitReset     string
}

func (failure HTTPError) Error() string {
	return fmt.Sprintf("%s: GitHub returned %s", failure.Operation, failure.Status)
}

func New(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		httpClient: httpClient,
		apiBase:    "https://api.github.com",
		rawBase:    "https://raw.githubusercontent.com",
		treeLimits: defaultGitTreeLimits,
	}
}

func newClient(httpClient *http.Client, apiBase string) *Client {
	base := strings.TrimRight(apiBase, "/")
	return &Client{httpClient: httpClient, apiBase: base, rawBase: base, treeLimits: defaultGitTreeLimits}
}

// Releases lists every release so the promotion layer can select exactly one
// immutable pack-v<version> release.
func (client *Client) Releases(ctx context.Context, repository string) ([]Release, error) {
	type releaseResponse struct {
		ID          int64  `json:"id"`
		Tag         string `json:"tag_name"`
		Immutable   bool   `json:"immutable"`
		PublishedAt string `json:"published_at"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
	}
	var releases []Release
	for page := 1; ; page++ {
		var response []releaseResponse
		endpoint := client.repoURL(repository) + "/releases?per_page=100&page=" + strconv.Itoa(page)
		if err := client.getJSON(ctx, endpoint, &response); err != nil {
			return nil, err
		}
		for _, item := range response {
			var published time.Time
			if item.PublishedAt != "" {
				parsed, err := time.Parse(time.RFC3339, item.PublishedAt)
				if err != nil {
					return nil, fmt.Errorf("release %s has invalid published_at: %w", item.Tag, err)
				}
				published = parsed
			}
			releases = append(releases, Release{
				ID: item.ID, Tag: item.Tag, Immutable: item.Immutable,
				PublishedAt: published, Draft: item.Draft, Prerelease: item.Prerelease,
			})
		}
		if len(response) < 100 {
			break
		}
	}
	return releases, nil
}

// ResolveRelease follows the release tag through any bounded annotated-tag
// chain and observes its exact commit and root tree.
func (client *Client) ResolveRelease(ctx context.Context, repository string, release Release) (Candidate, error) {
	if release.Tag == "" {
		return Candidate{}, errors.New("release tag is required")
	}
	candidate, err := client.repositoryCandidate(ctx, repository)
	if err != nil {
		return Candidate{}, err
	}
	tagPath := url.PathEscape(release.Tag)
	var ref struct {
		Object gitObject `json:"object"`
	}
	if err := client.getJSON(ctx, client.repoURL(repository)+"/git/ref/tags/"+tagPath, &ref); err != nil {
		return Candidate{}, fmt.Errorf("resolve exact tag ref: %w", err)
	}
	candidate.Release = &release
	candidate.TagRefName = "refs/tags/" + release.Tag
	candidate.TagRefType = ref.Object.Type
	candidate.TagRefSHA = ref.Object.SHA
	object := ref.Object
	seen := map[string]bool{}
	for object.Type == "tag" {
		if len(candidate.TagObjects) >= maxAnnotatedTagObjects {
			return Candidate{}, fmt.Errorf("annotated tag object limit of %d exceeded", maxAnnotatedTagObjects)
		}
		if object.SHA == "" || seen[object.SHA] {
			return Candidate{}, errors.New("tag chain is empty or cyclic")
		}
		seen[object.SHA] = true
		var tag struct {
			SHA    string    `json:"sha"`
			Object gitObject `json:"object"`
		}
		if err := client.getJSON(ctx, client.repoURL(repository)+"/git/tags/"+object.SHA, &tag); err != nil {
			return Candidate{}, fmt.Errorf("peel tag object: %w", err)
		}
		candidate.TagObjects = append(candidate.TagObjects, TagObject{
			SHA: tag.SHA, TargetSHA: tag.Object.SHA, TargetType: tag.Object.Type,
		})
		object = tag.Object
	}
	if object.Type != "commit" || len(object.SHA) != 40 {
		return Candidate{}, errors.New("release tag did not peel to one full commit SHA")
	}
	if err := client.addCommit(ctx, repository, object.SHA, &candidate); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}

// ResolveCommit observes one exact lowercase commit and its root tree.
func (client *Client) ResolveCommit(ctx context.Context, repository, sha string) (Candidate, error) {
	if !fullGitObjectID(sha) {
		return Candidate{}, errors.New("commit acquisition requires a full lowercase 40-character SHA")
	}
	candidate, err := client.repositoryCandidate(ctx, repository)
	if err != nil {
		return Candidate{}, err
	}
	if err := client.addCommit(ctx, repository, sha, &candidate); err != nil {
		return Candidate{}, err
	}
	if candidate.Commit != sha {
		return Candidate{}, errors.New("GitHub returned a different commit SHA")
	}
	return candidate, nil
}

type gitObject struct {
	SHA  string `json:"sha"`
	Type string `json:"type"`
}

func (client *Client) repositoryCandidate(ctx context.Context, repository string) (Candidate, error) {
	var response struct {
		ID         int64  `json:"id"`
		FullName   string `json:"full_name"`
		Visibility string `json:"visibility"`
		Private    bool   `json:"private"`
	}
	if err := client.getJSON(ctx, client.repoURL(repository), &response); err != nil {
		return Candidate{}, fmt.Errorf("observe repository identity: %w", err)
	}
	return Candidate{
		Repository: response.FullName, RepositoryID: response.ID,
		Public: !response.Private && response.Visibility == "public",
	}, nil
}

func (client *Client) addCommit(ctx context.Context, repository, sha string, candidate *Candidate) error {
	var commit struct {
		SHA  string    `json:"sha"`
		Tree gitObject `json:"tree"`
	}
	if err := client.getJSON(ctx, client.repoURL(repository)+"/git/commits/"+sha, &commit); err != nil {
		return fmt.Errorf("observe exact commit: %w", err)
	}
	candidate.Commit = commit.SHA
	candidate.Tree = commit.Tree.SHA
	return nil
}

func (client *Client) repoURL(repository string) string {
	return client.apiBase + "/repos/" + repository
}

func (client *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	request, err := client.request(ctx, endpoint)
	if err != nil {
		return err
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return newHTTPError("read GitHub API", response)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func newHTTPError(operation string, response *http.Response) HTTPError {
	return HTTPError{
		Operation: operation, StatusCode: response.StatusCode, Status: response.Status,
		RetryAfter:         response.Header.Get("Retry-After"),
		RateLimitRemaining: response.Header.Get("X-RateLimit-Remaining"),
		RateLimitReset:     response.Header.Get("X-RateLimit-Reset"),
	}
}

func (client *Client) request(ctx context.Context, endpoint string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", gitHubAPIVersion)
	request.Header.Set("User-Agent", "packy-managed-pack-promotion")
	return request, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	read, err := reader.reader.Read(buffer)
	if contextErr := reader.ctx.Err(); contextErr != nil {
		return 0, contextErr
	}
	return read, err
}

func safeGitTreePath(value string) bool {
	return value != "" && value != "." && !path.IsAbs(value) && path.Clean(value) == value && !strings.HasPrefix(value, "../") && !strings.Contains(value, "\\")
}

func emptyDirectory(name string) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("caller-supplied acquisition area must be a real directory")
	}
	entries, err := os.ReadDir(name)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("caller-supplied acquisition area must be empty")
	}
	return nil
}

func cleanDirectory(name string) error {
	entries, err := os.ReadDir(name)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(name, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
