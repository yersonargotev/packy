package catalogstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/yersonargotev/packy/internal/githubauth"
)

const privateCatalogGuide = "https://github.com/yersonargotev/packy/blob/main/docs/private-catalogs.md"

var assetHosts = map[string]bool{"release-assets.githubusercontent.com": true, "objects.githubusercontent.com": true}

// AttestationVerifier verifies GitHub build-provenance cryptography and binds
// the subject digest to the expected repository and signer workflow.
type AttestationVerifier interface {
	Verify(ctx context.Context, repository, subjectSHA256, signerWorkflow string) error
}

// Credential returns the GitHub token that reads an access-controlled catalog.
type Credential func(context.Context) (githubauth.Token, error)

// NewGitHubSource constructs the production GitHub release source. The caller
// owns HTTP timeouts. Acquisition of an attested catalog fails closed when
// verifier is nil or cannot verify the archive's build-provenance attestation.
// Only acquisition of an access-controlled catalog calls credential, and only
// requests to api.github.com carry its token.
func NewGitHubSource(client *http.Client, verifier AttestationVerifier, credential Credential) Source {
	if client == nil {
		client = http.DefaultClient
	}
	return &gitHubSource{client: client, verifier: verifier, credential: credential}
}

type gitHubSource struct {
	client     *http.Client
	verifier   AttestationVerifier
	credential Credential
}

func (source *gitHubSource) Latest(ctx context.Context, registration Registration) (Release, error) {
	repository := registration.Repository.String()
	var token *githubauth.Token
	switch registration.Trust.(type) {
	case Attested:
		if source.verifier == nil {
			return Release{}, errors.New("GitHub catalog Source requires an artifact attestation verifier")
		}
	case AccessControlled:
		found, err := source.privateAccess(ctx, repository)
		if err != nil {
			return Release{}, err
		}
		token = &found
	default:
		return Release{}, fmt.Errorf("catalog %s has no supported trust policy", repository)
	}
	var wire struct {
		Tag        string `json:"tag_name"`
		Target     string `json:"target_commitish"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Immutable  bool   `json:"immutable"`
		Published  string `json:"published_at"`
		Author     struct {
			Login string `json:"login"`
		} `json:"author"`
		Assets []struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Digest string `json:"digest"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if err := source.getJSON(ctx, "https://api.github.com/repos/"+repository+"/releases/latest", token, &wire); err != nil {
		return Release{}, fmt.Errorf("read latest catalog release of %s: %w", repository, err)
	}
	release := Release{
		Repository: repository, Tag: wire.Tag, Commit: wire.Target, Publisher: wire.Author.Login,
		Published: wire.Published != "", Draft: wire.Draft, Prerelease: wire.Prerelease, Immutable: wire.Immutable,
		Assets: make([]Asset, 0, len(wire.Assets)),
	}
	for _, candidate := range wire.Assets {
		if candidate.Size < 0 || candidate.Size > maxArchiveBytes {
			return Release{}, fmt.Errorf("catalog asset %q exceeds size limit", candidate.Name)
		}
		data, err := source.download(ctx, "https://api.github.com/repos/"+repository+"/releases/assets/"+strconv.FormatInt(candidate.ID, 10), token)
		if err != nil {
			return Release{}, fmt.Errorf("download catalog asset %q: %w", candidate.Name, err)
		}
		release.Assets = append(release.Assets, Asset{Name: candidate.Name, SHA256: candidate.Digest, Data: data})
	}
	attested, ok := registration.Trust.(Attested)
	if !ok {
		return release, nil
	}
	assets, err := validateReleaseWithoutAttestation(registration, release)
	if err != nil {
		return Release{}, err
	}
	if err := source.verifier.Verify(ctx, repository, digest(assets[archiveAsset].Data), repository+"/"+attested.Workflow); err != nil {
		return Release{}, fmt.Errorf("verify catalog %s artifact attestation: %w", repository, err)
	}
	release.AttestationVerified = true
	return release, nil
}

func (source *gitHubSource) privateAccess(ctx context.Context, repository string) (githubauth.Token, error) {
	if source.credential == nil {
		return githubauth.Token{}, errors.New("an access-controlled catalog needs a GitHub token, and this Packy reads none")
	}
	token, err := source.credential(ctx)
	if err != nil {
		return githubauth.Token{}, fmt.Errorf("an access-controlled catalog needs a GitHub token that can read its repository: %w; see %s", err, privateCatalogGuide)
	}
	var visibility struct {
		Private *bool `json:"private"`
	}
	if err := source.getJSON(ctx, "https://api.github.com/repos/"+repository, &token, &visibility); err != nil {
		return githubauth.Token{}, fmt.Errorf("read repository visibility: %w", err)
	}
	if visibility.Private == nil {
		return githubauth.Token{}, errors.New("GitHub did not report whether the repository is private")
	}
	if !*visibility.Private {
		return githubauth.Token{}, errors.New("the repository is public, and access-controlled is only for private repositories; use --trust attested")
	}
	return token, nil
}

func (source *gitHubSource) getJSON(ctx context.Context, url string, token *githubauth.Token, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "packy-catalog-consumer")
	if token != nil {
		token.Authorize(request)
	}
	response, err := source.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return statusError(response, token)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(result); err != nil {
		return err
	}
	return nil
}

// download follows the asset API redirect without the token, because the
// token must not leave api.github.com and GitHub's signed asset host refuses a
// request that carries one.
func (source *gitHubSource) download(ctx context.Context, assetURL string, token *githubauth.Token) ([]byte, error) {
	client := *source.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "packy-catalog-consumer")
	if token != nil {
		token.Authorize(request)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
		return readAsset(response)
	case http.StatusFound, http.StatusMovedPermanently, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
	default:
		return nil, statusError(response, token)
	}
	location, err := response.Location()
	if err != nil {
		return nil, fmt.Errorf("GitHub redirected the asset download without a location: %w", err)
	}
	if !trustedAssetHost(location) {
		return nil, fmt.Errorf("GitHub redirected the asset download to untrusted host %q", location.Host)
	}
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, location.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "packy-catalog-consumer")
	redirected, err := client.Do(request)
	if err != nil {
		var requestErr *url.Error
		if errors.As(err, &requestErr) {
			err = requestErr.Err
		}
		return nil, fmt.Errorf("download from %s: %w", location.Host, err)
	}
	defer redirected.Body.Close()
	if redirected.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", location.Host, redirected.Status)
	}
	return readAsset(redirected)
}

func trustedAssetHost(location *url.URL) bool {
	return location.Scheme == "https" && location.User == nil && assetHosts[location.Host]
}

func readAsset(response *http.Response) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(response.Body, maxArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxArchiveBytes {
		return nil, errors.New("download exceeds size limit")
	}
	return data, nil
}

// statusError points an anonymous 404 to private catalog registration, because
// GitHub answers 404, not 403, for a private repository the caller cannot see.
func statusError(response *http.Response, token *githubauth.Token) error {
	err := fmt.Errorf("GitHub returned %s", response.Status)
	if response.Header.Get("X-RateLimit-Remaining") == "0" {
		return fmt.Errorf("%w; the GitHub API rate limit is exhausted, so try again later", err)
	}
	if token == nil {
		if response.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w; if the repository is private, register it with --trust access-controlled and a GitHub token, as %s explains", err, privateCatalogGuide)
		}
		return err
	}
	switch response.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w; GitHub rejected the token from %s, so %s; see %s", err, token, token.Renewal(), privateCatalogGuide)
	case http.StatusForbidden, http.StatusNotFound:
		return fmt.Errorf("%w; the token from %s needs read access to the repository and its contents; see %s", err, token, privateCatalogGuide)
	}
	return err
}

func validateReleaseWithoutAttestation(registration Registration, release Release) (map[string]Asset, error) {
	release.AttestationVerified = true
	return validateRelease(registration, release)
}
