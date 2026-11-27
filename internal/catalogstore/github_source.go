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

const (
	maxAPIResponseBytes  = 1 << 20
	maxAttestationsBytes = 8 << 20
)

// AttestationVerifier verifies the GitHub build-provenance bundles of an
// archive and binds the subject digest to the expected repository and signer
// workflow.
type AttestationVerifier interface {
	Verify(ctx context.Context, bundles []json.RawMessage, repository, subjectSHA256, signerWorkflow string) error
}

// AttestationRejectedError reports that none of the attestation bundles of a
// catalog's archive verified, as ErrNoAttestationVerified marks. A failure to
// fetch the bundles or to load a Sigstore trust root is not a rejection.
type AttestationRejectedError struct {
	Repository string
	Err        error
}

func (e *AttestationRejectedError) Error() string {
	return fmt.Sprintf("verify catalog %s artifact attestation: %v", e.Repository, e.Err)
}

func (e *AttestationRejectedError) Unwrap() error { return e.Err }

// Credential returns the GitHub token that reads a private catalog.
type Credential func(context.Context) (githubauth.Token, error)

// NewGitHubSource constructs the production GitHub release source. The caller
// owns HTTP timeouts. Acquisition of an attested catalog fails closed when
// verifier is nil or cannot verify the archive's build-provenance attestation.
// Acquisition of an access-controlled catalog always calls credential. An
// attested catalog calls it only after an anonymous read of the release
// answers 404, as GitHub does for a private repository, and then reads its
// release, assets, and attestations with the token. Only requests to
// api.github.com carry the token. NewGitHubSource replaces the CheckRedirect
// of client with one that enforces that rule.
func NewGitHubSource(client *http.Client, verifier AttestationVerifier, credential Credential) Source {
	if client == nil {
		client = http.DefaultClient
	}
	guarded := *client
	guarded.CheckRedirect = keepTokenOnAPI
	return &gitHubSource{client: &guarded, verifier: verifier, credential: credential}
}

// keepTokenOnAPI refuses a redirect of a request that carries the token unless
// it stays on https://api.github.com, because http.Client keeps Authorization
// on a downgrade to http and on a subdomain. It repeats the 10-redirect limit
// of http.Client, because net/http does not export its default policy.
func keepTokenOnAPI(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if via[0].Header.Get("Authorization") != "" && (request.URL.Scheme != "https" || request.URL.Host != "api.github.com") {
		return errors.New("refused to send the GitHub token to a redirect off https://api.github.com")
	}
	return nil
}

type gitHubSource struct {
	client     *http.Client
	verifier   AttestationVerifier
	credential Credential
}

func (source *gitHubSource) Release(ctx context.Context, registration Registration, commit string) (Release, error) {
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
	endpoint, name := "/releases/latest", "latest catalog release"
	if commit != "" {
		endpoint, name = "/releases/tags/catalog-"+commit, "catalog release catalog-"+commit
	}
	address := "https://api.github.com/repos/" + repository + endpoint
	err := source.getJSON(ctx, address, token, &wire, maxAPIResponseBytes)
	var status *statusErr
	if token == nil && errors.As(err, &status) && status.code == http.StatusNotFound {
		found, tokenErr := source.token(ctx)
		if tokenErr != nil {
			return Release{}, fmt.Errorf("read %s of %s: %w; if the repository is private, %w", name, repository, err, tokenErr)
		}
		token = &found
		err = source.getJSON(ctx, address, token, &wire, maxAPIResponseBytes)
	}
	if err != nil {
		return Release{}, fmt.Errorf("read %s of %s: %w", name, repository, err)
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
	subject := digest(assets[archiveAsset].Data)
	var attestations struct {
		Attestations []struct {
			Bundle json.RawMessage `json:"bundle"`
		} `json:"attestations"`
	}
	if err := source.getJSON(ctx, "https://api.github.com/repos/"+repository+"/attestations/sha256:"+subject, token, &attestations, maxAttestationsBytes); err != nil {
		return Release{}, fmt.Errorf("verify catalog %s artifact attestation: fetch catalog artifact attestations: %w", repository, err)
	}
	bundles := make([]json.RawMessage, 0, len(attestations.Attestations))
	for _, attestation := range attestations.Attestations {
		bundles = append(bundles, attestation.Bundle)
	}
	if err := source.verifier.Verify(ctx, bundles, repository, subject, repository+"/"+attested.Workflow); err != nil {
		if errors.Is(err, ErrNoAttestationVerified) {
			return Release{}, &AttestationRejectedError{Repository: repository, Err: err}
		}
		return Release{}, fmt.Errorf("verify catalog %s artifact attestation: %w", repository, err)
	}
	release.AttestationVerified = true
	return release, nil
}

func (source *gitHubSource) token(ctx context.Context) (githubauth.Token, error) {
	if source.credential == nil {
		return githubauth.Token{}, errors.New("GitHub catalog Source requires a credential for private catalogs")
	}
	token, err := source.credential(ctx)
	if err != nil {
		return githubauth.Token{}, fmt.Errorf("a private catalog needs a GitHub token that can read its repository: %w; see %s", err, privateCatalogGuide)
	}
	return token, nil
}

func (source *gitHubSource) privateAccess(ctx context.Context, repository string) (githubauth.Token, error) {
	token, err := source.token(ctx)
	if err != nil {
		return githubauth.Token{}, err
	}
	var visibility struct {
		Private *bool `json:"private"`
	}
	if err := source.getJSON(ctx, "https://api.github.com/repos/"+repository, &token, &visibility, maxAPIResponseBytes); err != nil {
		return githubauth.Token{}, fmt.Errorf("read repository visibility: %w", err)
	}
	if visibility.Private == nil {
		return githubauth.Token{}, errors.New("GitHub did not report whether the repository is private")
	}
	if !*visibility.Private {
		return githubauth.Token{}, errors.New("the repository is public, and access-controlled is only for private repositories; a public repository uses --trust attested")
	}
	return token, nil
}

func (source *gitHubSource) getJSON(ctx context.Context, url string, token *githubauth.Token, result any, limit int64) error {
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
	decoder := json.NewDecoder(io.LimitReader(response.Body, limit))
	if err := decoder.Decode(result); err != nil {
		return err
	}
	return nil
}

// download follows the asset API redirect without the token, because the
// token must not leave api.github.com and GitHub's signed asset host refuses a
// request that carries one. It sends both requests through the transport,
// because http.Client quotes an unparseable Location, signature and all, in
// its error.
func (source *gitHubSource) download(ctx context.Context, assetURL string, token *githubauth.Token) ([]byte, error) {
	if source.client.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, source.client.Timeout)
		defer cancel()
	}
	transport := source.client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
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
	response, err := transport.RoundTrip(request)
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
		return nil, errors.New("GitHub redirected the asset download to an invalid location")
	}
	if !trustedAssetHost(location) {
		return nil, fmt.Errorf("GitHub redirected the asset download to untrusted host %q", location.Host)
	}
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, location.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "packy-catalog-consumer")
	redirected, err := transport.RoundTrip(request)
	if err != nil {
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

// statusErr keeps the status of a failed GitHub response, because GitHub
// answers 404, not 403, for a private repository the caller cannot see.
type statusErr struct {
	code int
	err  error
}

func (e *statusErr) Error() string { return e.err.Error() }

func (e *statusErr) Unwrap() error { return e.err }

func statusError(response *http.Response, token *githubauth.Token) error {
	err := fmt.Errorf("GitHub returned %s", response.Status)
	switch {
	case response.Header.Get("X-RateLimit-Remaining") == "0":
		err = fmt.Errorf("%w; the GitHub API rate limit is exhausted, so try again later", err)
	case token == nil:
	case response.StatusCode == http.StatusUnauthorized:
		err = fmt.Errorf("%w; GitHub rejected the token from %s, so %s; see %s", err, token, token.Renewal(), privateCatalogGuide)
	case response.StatusCode == http.StatusForbidden, response.StatusCode == http.StatusNotFound:
		err = fmt.Errorf("%w; the token from %s needs read access to the repository and its contents; see %s", err, token, privateCatalogGuide)
	}
	return &statusErr{code: response.StatusCode, err: err}
}

func validateReleaseWithoutAttestation(registration Registration, release Release) (map[string]Asset, error) {
	release.AttestationVerified = true
	return validateRelease(registration, release)
}
