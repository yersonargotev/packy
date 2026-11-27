package catalogstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/githubauth"
)

type recordingVerifier struct {
	bundles                      []string
	repository, digest, workflow string
	err                          error
}

func (v *recordingVerifier) Verify(_ context.Context, bundles []json.RawMessage, repository, digest, workflow string) error {
	for _, bundle := range bundles {
		v.bundles = append(v.bundles, string(bundle))
	}
	v.repository, v.digest, v.workflow = repository, digest, workflow
	return v.err
}

const testToken = "test-token-value"

func TestGitHubSourceDownloadsExactReleaseAssetsAndVerifiesArchiveAttestation(t *testing.T) {
	for _, test := range []struct {
		name         string
		registration Registration
		signer       string
	}{
		{"official", Official(), "yersonargotev/packy-catalog/.github/workflows/publish.yml"},
		{"third party", Registration{Repository: mustRepository(t, "Acme/Packs"), Trust: Attested{Workflow: ".github/workflows/release.yaml"}}, "acme/packs/.github/workflows/release.yaml"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := test.registration.Repository.String()
			github := newFakeGitHub(t, repository, false)
			verifier := &recordingVerifier{}
			release, err := NewGitHubSource(github.client(), verifier, github.credential).Latest(context.Background(), test.registration)
			if err != nil {
				t.Fatal(err)
			}
			if !release.AttestationVerified || release.Repository != repository || verifier.repository != repository || verifier.digest != digest(github.archive) || verifier.workflow != test.signer || !slices.Equal(verifier.bundles, []string{fakeBundle}) {
				t.Fatalf("release/verifier = %#v / %#v", release, verifier)
			}
			if github.credentialCalls != 0 {
				t.Fatalf("an attested catalog read a credential %d times", github.credentialCalls)
			}
			for _, request := range github.requests {
				if request.authorization != "" {
					t.Fatalf("an attested catalog request to %s carried a token", request.url)
				}
			}
		})
	}
}

func TestGitHubSourceMarksOnlyUnverifiedAttestationsAsRejected(t *testing.T) {
	registration := Registration{Repository: mustRepository(t, "acme/packs"), Trust: Attested{Workflow: DefaultWorkflow}}
	unverified := fmt.Errorf("%w: signed by another workflow", ErrNoAttestationVerified)
	_, err := NewGitHubSource(newFakeGitHub(t, "acme/packs", false).client(), &recordingVerifier{err: unverified}, nil).Latest(context.Background(), registration)
	var rejected *AttestationRejectedError
	if !errors.As(err, &rejected) || rejected.Repository != "acme/packs" || !strings.Contains(err.Error(), "verify catalog acme/packs artifact attestation: no catalog attestation verified: signed by another workflow") {
		t.Fatalf("Latest() error = %v", err)
	}
	failure := fmt.Errorf("initialize Sigstore trust metadata: %w", context.DeadlineExceeded)
	_, err = NewGitHubSource(newFakeGitHub(t, "acme/packs", false).client(), &recordingVerifier{err: failure}, nil).Latest(context.Background(), registration)
	if errors.As(err, &rejected) || !errors.Is(err, failure) || !strings.Contains(err.Error(), "verify catalog acme/packs artifact attestation") {
		t.Fatalf("Latest() with %v = %v", failure, err)
	}
	github := newFakeGitHub(t, "acme/packs", false)
	github.attestationStatus = http.StatusServiceUnavailable
	verifier := &recordingVerifier{}
	_, err = NewGitHubSource(github.client(), verifier, nil).Latest(context.Background(), registration)
	if errors.As(err, &rejected) || verifier.repository != "" || !strings.Contains(err.Error(), "verify catalog acme/packs artifact attestation: fetch catalog artifact attestations: GitHub returned 503") {
		t.Fatalf("Latest() with unavailable attestations = %v", err)
	}
}

func TestGitHubSourceFailsClosedWithoutAttestationVerifier(t *testing.T) {
	_, err := NewGitHubSource(http.DefaultClient, nil, nil).Latest(context.Background(), Official())
	if err == nil || !strings.Contains(err.Error(), "requires an artifact attestation verifier") {
		t.Fatalf("Latest() error = %v", err)
	}
}

func TestGitHubSourceReadsAPrivateCatalogWithTheTokenAndKeepsItFromTheAssetHost(t *testing.T) {
	github := newFakeGitHub(t, "team/private", true)
	verifier := &recordingVerifier{}
	registration := Registration{Repository: mustRepository(t, "team/private"), Trust: AccessControlled{}}
	release, err := NewGitHubSource(github.client(), verifier, github.credential).Latest(context.Background(), registration)
	if err != nil {
		t.Fatal(err)
	}
	if release.AttestationVerified || verifier.repository != "" || len(release.Assets) != 2 {
		t.Fatalf("release/verifier = %#v / %#v", release, verifier)
	}
	if _, err := validateRelease(registration, release); err != nil {
		t.Fatalf("validateRelease() = %v", err)
	}
	assetHost := 0
	for _, request := range github.requests {
		switch {
		case strings.HasPrefix(request.url, "https://api.github.com/"):
			if request.authorization != "Bearer "+testToken {
				t.Errorf("API request %s carried authorization %q", request.url, request.authorization)
			}
		case strings.HasPrefix(request.url, "https://release-assets.githubusercontent.com/"):
			assetHost++
			if request.authorization != "" {
				t.Errorf("asset host request %s carried the token", request.url)
			}
		default:
			t.Errorf("unexpected request %s", request.url)
		}
		if strings.Contains(request.url, "/releases/assets/") && request.accept != "application/octet-stream" {
			t.Errorf("asset API request %s accepted %q", request.url, request.accept)
		}
	}
	if assetHost != 2 {
		t.Fatalf("asset host received %d downloads, want 2", assetHost)
	}
}

func TestGitHubSourceExplainsRefusedPrivateAccess(t *testing.T) {
	registration := func(t *testing.T) Registration {
		return Registration{Repository: mustRepository(t, "team/private"), Trust: AccessControlled{}}
	}
	for _, test := range []struct {
		name    string
		private bool
		prepare func(*fakeGitHub)
		want    []string
	}{
		{"missing token", true, func(g *fakeGitHub) { g.credentialErr = githubauth.ErrMissing }, []string{"a private catalog needs a GitHub token", "GH_TOKEN or GITHUB_TOKEN", "gh auth login", privateCatalogGuide}},
		{"rejected token", true, func(g *fakeGitHub) { g.status = http.StatusUnauthorized }, []string{"401", "GitHub rejected the token from GH_TOKEN, so replace or unset GH_TOKEN", privateCatalogGuide}},
		{"insufficient token", true, func(g *fakeGitHub) { g.status = http.StatusNotFound }, []string{"404", "the token from GH_TOKEN needs read access", privateCatalogGuide}},
		{"contents hidden from the token", true, func(g *fakeGitHub) { g.releaseStatus = http.StatusNotFound }, []string{"read latest catalog release", "404", "the token from GH_TOKEN needs read access", privateCatalogGuide}},
		{"rate limited", true, func(g *fakeGitHub) { g.status, g.rateLimited = http.StatusForbidden, true }, []string{"403", "rate limit is exhausted"}},
		{"asset host unreachable", true, func(g *fakeGitHub) { g.assetHostDown = true }, []string{"download from release-assets.githubusercontent.com: asset host is down"}},
		{"public repository", false, func(*fakeGitHub) {}, []string{"the repository is public", "a public repository uses --trust attested"}},
		{"redirect outside GitHub", true, func(g *fakeGitHub) { g.redirectHost = "example.com" }, []string{"untrusted host \"example.com\""}},
		{"redirect to user content", true, func(g *fakeGitHub) { g.redirectHost = "raw.githubusercontent.com" }, []string{"untrusted host \"raw.githubusercontent.com\""}},
		{"plaintext redirect", true, func(g *fakeGitHub) { g.redirectScheme = "http" }, []string{"untrusted host"}},
		{"redirect with userinfo", true, func(g *fakeGitHub) { g.redirectHost = "u:p@release-assets.githubusercontent.com" }, []string{"untrusted host"}},
		{"unparseable redirect", true, func(g *fakeGitHub) { g.redirectHost = "release-assets.githubusercontent.com/%zz" }, []string{"redirected the asset download to an invalid location"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			github := newFakeGitHub(t, "team/private", test.private)
			test.prepare(github)
			_, err := NewGitHubSource(github.client(), &recordingVerifier{}, github.credential).Latest(context.Background(), registration(t))
			if err == nil {
				t.Fatal("Latest() accepted the catalog")
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Latest() error = %v, want %q", err, want)
				}
			}
			if strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), "sig=") {
				t.Fatalf("Latest() error reveals a credential: %v", err)
			}
			if strings.Count(err.Error(), privateCatalogGuide) > 1 {
				t.Fatalf("Latest() error repeats the guide: %v", err)
			}
			for _, request := range github.requests {
				if strings.Contains(request.url, "example.com") || strings.Contains(request.url, "raw.githubusercontent.com") || strings.HasPrefix(request.url, "http://") || strings.Contains(request.url, "u:p@") {
					t.Fatalf("Latest() followed a redirect to %s", request.url)
				}
			}
		})
	}
}

func TestGitHubSourceFollowsAnAuthenticatedRedirectOnlyWithinTheAPI(t *testing.T) {
	registration := Registration{Repository: mustRepository(t, "team/private"), Trust: AccessControlled{}}
	redirected := func(target string) (*fakeGitHub, error) {
		github := newFakeGitHub(t, "team/private", true)
		github.apiRedirect = target
		github.responses[target] = `{"private":true}`
		_, err := NewGitHubSource(github.client(), nil, github.credential).Latest(context.Background(), registration)
		return github, err
	}
	moved := "https://api.github.com/repositories/1"
	github, err := redirected(moved)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(github.requests, fakeRequest{url: moved, authorization: "Bearer " + testToken, accept: "application/vnd.github+json"}) {
		t.Fatalf("the moved repository request did not carry the token: %#v", github.requests)
	}
	for _, target := range []string{
		"http://api.github.com/repositories/1",
		"https://sub.api.github.com/repositories/1",
		"https://api.github.com.evil.example/repositories/1",
		"https://evil.example/repositories/1",
	} {
		github, err := redirected(target)
		if err == nil || !strings.Contains(err.Error(), "refused to send the GitHub token to a redirect off https://api.github.com") || strings.Contains(err.Error(), testToken) {
			t.Errorf("Latest() redirected to %s: %v", target, err)
		}
		for _, request := range github.requests {
			if request.url == target {
				t.Errorf("Latest() followed the redirect to %s", target)
			}
		}
	}
}

func TestGitHubSourceWithoutACredentialRefusesAccessControlledCatalogs(t *testing.T) {
	github := newFakeGitHub(t, "team/private", true)
	_, err := NewGitHubSource(github.client(), &recordingVerifier{}, nil).Latest(context.Background(), Registration{Repository: mustRepository(t, "team/private"), Trust: AccessControlled{}})
	if err == nil || !strings.Contains(err.Error(), "requires a credential for private catalogs") || len(github.requests) != 0 {
		t.Fatalf("Latest() error = %v after %d requests", err, len(github.requests))
	}
}

func TestGitHubSourceRefusesAnUnknownTrustPolicy(t *testing.T) {
	github := newFakeGitHub(t, "team/private", false)
	_, err := NewGitHubSource(github.client(), &recordingVerifier{}, github.credential).Latest(context.Background(), Registration{Repository: mustRepository(t, "team/private")})
	if err == nil || !strings.Contains(err.Error(), "has no supported trust policy") || len(github.requests) != 0 {
		t.Fatalf("Latest() error = %v after %d requests", err, len(github.requests))
	}
}

func TestGitHubSourceReadsAPrivateAttestedCatalogWithTheToken(t *testing.T) {
	github := newFakeGitHub(t, "team/private", true)
	verifier := &recordingVerifier{}
	release, err := NewGitHubSource(github.client(), verifier, github.credential).Latest(context.Background(), Registration{Repository: mustRepository(t, "team/private"), Trust: Attested{Workflow: DefaultWorkflow}})
	if err != nil {
		t.Fatal(err)
	}
	if !release.AttestationVerified || github.credentialCalls != 1 || verifier.digest != digest(github.archive) || !slices.Equal(verifier.bundles, []string{fakeBundle}) {
		t.Fatalf("release/verifier = %#v / %#v after %d credential calls", release, verifier, github.credentialCalls)
	}
	api := "https://api.github.com/repos/team/private"
	if first := github.requests[0]; first.url != api+"/releases/latest" || first.authorization != "" {
		t.Fatalf("first request = %#v, want an anonymous read of the latest release", first)
	}
	attestations := 0
	for _, request := range github.requests[1:] {
		switch {
		case request.url == api:
			t.Errorf("an attested catalog read the repository visibility")
		case strings.HasPrefix(request.url, "https://api.github.com/"):
			if request.authorization != "Bearer "+testToken {
				t.Errorf("API request %s carried authorization %q", request.url, request.authorization)
			}
			if strings.HasPrefix(request.url, api+"/attestations/sha256:") {
				attestations++
			}
		case request.authorization != "":
			t.Errorf("request %s off the API carried the token", request.url)
		}
	}
	if attestations != 1 {
		t.Fatalf("the attestations API received %d authorized requests, want 1", attestations)
	}
}

func TestGitHubSourceExplainsARefusedPrivateAttestedCatalog(t *testing.T) {
	registration := Registration{Repository: mustRepository(t, "team/private"), Trust: Attested{Workflow: DefaultWorkflow}}
	github := newFakeGitHub(t, "team/private", true)
	github.credentialErr = githubauth.ErrMissing
	_, err := NewGitHubSource(github.client(), &recordingVerifier{}, github.credential).Latest(context.Background(), registration)
	if err == nil || len(github.requests) != 1 || strings.Count(err.Error(), privateCatalogGuide) != 1 {
		t.Fatalf("Latest() without a token = %v after %d requests", err, len(github.requests))
	}
	for _, want := range []string{"read latest catalog release of team/private", "404", "if the repository is private, a private catalog needs a GitHub token", "gh auth login"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Latest() without a token = %v, want %q", err, want)
		}
	}

	github = newFakeGitHub(t, "team/private", true)
	github.status = http.StatusNotFound
	_, err = NewGitHubSource(github.client(), &recordingVerifier{}, github.credential).Latest(context.Background(), registration)
	if err == nil || !strings.Contains(err.Error(), "the token from GH_TOKEN needs read access") || strings.Contains(err.Error(), testToken) {
		t.Fatalf("Latest() with an insufficient token = %v", err)
	}
}

type fakeRequest struct{ url, authorization, accept string }

const fakeBundle = `{"fixture":"bundle"}`

// fakeGitHub answers as GitHub does for one repository whose latest release
// holds a valid archive and checksum. A private repository answers 404 to a
// request without the token.
type fakeGitHub struct {
	repository        string
	private           bool
	archive           []byte
	responses         map[string]string
	status            int
	releaseStatus     int
	attestationStatus int
	rateLimited       bool
	assetHostDown     bool
	redirectScheme    string
	redirectHost      string
	apiRedirect       string
	credentialErr     error
	credentialCalls   int
	requests          []fakeRequest
}

func newFakeGitHub(t *testing.T, repository string, private bool) *fakeGitHub {
	t.Helper()
	commit := strings.Repeat("a", 40)
	archive := []byte("archive")
	checksum := []byte(digest(archive) + "  " + archiveAsset + "\n")
	api := "https://api.github.com/repos/" + repository
	return &fakeGitHub{repository: repository, private: private, archive: archive, redirectScheme: "https", redirectHost: "release-assets.githubusercontent.com", responses: map[string]string{
		api: fmt.Sprintf(`{"full_name":%q,"private":%t}`, repository, private),
		api + "/releases/latest": fmt.Sprintf(
			`{"tag_name":"catalog-%s","target_commitish":"%s","draft":false,"prerelease":false,"immutable":true,"published_at":"2026-09-14T00:00:00Z","author":{"login":"github-actions[bot]"},"assets":[{"id":11,"name":"%s","digest":"sha256:%s","size":%d},{"id":12,"name":"%s","digest":"sha256:%s","size":%d}]}`,
			commit, commit, archiveAsset, digest(archive), len(archive), checksumAsset, digest(checksum), len(checksum)),
		api + "/attestations/sha256:" + digest(archive): `{"attestations":[{"bundle":` + fakeBundle + `}]}`,
		"/11": string(archive),
		"/12": string(checksum),
	}}
}

func (g *fakeGitHub) credential(context.Context) (githubauth.Token, error) {
	g.credentialCalls++
	if g.credentialErr != nil {
		return githubauth.Token{}, g.credentialErr
	}
	return githubauth.Lookup(context.Background(), func(name string) string {
		if name == "GH_TOKEN" {
			return testToken
		}
		return ""
	})
}

func (g *fakeGitHub) client() *http.Client {
	return &http.Client{Transport: roundTripFunc(g.roundTrip)}
}

func (g *fakeGitHub) roundTrip(request *http.Request) (*http.Response, error) {
	g.requests = append(g.requests, fakeRequest{url: request.URL.String(), authorization: request.Header.Get("Authorization"), accept: request.Header.Get("Accept")})
	url := request.URL.String()
	api := "https://api.github.com/repos/" + g.repository
	if url == api && g.apiRedirect != "" {
		return response(http.StatusMovedPermanently, http.Header{"Location": {g.apiRedirect}}, ""), nil
	}
	if strings.HasPrefix(url, api) {
		switch authorization := request.Header.Get("Authorization"); {
		case g.status != 0 && authorization != "":
			header := http.Header{}
			if g.rateLimited {
				header.Set("X-RateLimit-Remaining", "0")
			}
			return response(g.status, header, ""), nil
		case g.private && authorization != "Bearer "+testToken:
			return response(http.StatusNotFound, nil, `{"message":"Not Found"}`), nil
		case g.releaseStatus != 0 && url == api+"/releases/latest":
			return response(g.releaseStatus, nil, `{"message":"Not Found"}`), nil
		case g.attestationStatus != 0 && strings.HasPrefix(url, api+"/attestations/"):
			return response(g.attestationStatus, nil, ""), nil
		}
	}
	if id, ok := strings.CutPrefix(url, api+"/releases/assets/"); ok {
		return response(http.StatusFound, http.Header{"Location": {g.redirectScheme + "://" + g.redirectHost + "/signed/" + id + "?sig=x"}}, ""), nil
	}
	if id, ok := strings.CutPrefix(request.URL.Path, "/signed"); ok && request.URL.Host == "release-assets.githubusercontent.com" {
		if g.assetHostDown {
			return nil, errors.New("asset host is down")
		}
		if request.Header.Get("Authorization") != "" {
			return response(http.StatusBadRequest, nil, ""), nil
		}
		url = id
	}
	body, ok := g.responses[url]
	if !ok {
		return nil, errors.New("unexpected URL " + request.URL.String())
	}
	return response(http.StatusOK, nil, body), nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func response(status int, header http.Header, body string) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func mustRepository(t *testing.T, value string) Repository {
	t.Helper()
	repository, err := ParseRepository(value)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
