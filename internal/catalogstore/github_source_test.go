package catalogstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/yersonargotev/packy/internal/githubauth"
)

type recordingVerifier struct {
	repository, digest, workflow string
	err                          error
}

func (v *recordingVerifier) Verify(_ context.Context, repository, digest, workflow string) error {
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
			if !release.AttestationVerified || release.Repository != repository || verifier.repository != repository || verifier.digest != digest(github.archive) || verifier.workflow != test.signer {
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
		{"missing token", true, func(g *fakeGitHub) { g.credentialErr = githubauth.ErrMissing }, []string{"an access-controlled catalog needs a GitHub token", "GH_TOKEN or GITHUB_TOKEN", "gh auth login", privateCatalogGuide}},
		{"rejected token", true, func(g *fakeGitHub) { g.status = http.StatusUnauthorized }, []string{"401", "GitHub rejected the token from GH_TOKEN", "gh auth login"}},
		{"insufficient token", true, func(g *fakeGitHub) { g.status = http.StatusNotFound }, []string{"404", "the token from GH_TOKEN needs read access"}},
		{"public repository", false, func(*fakeGitHub) {}, []string{"the repository is public", "use --trust attested"}},
		{"untrusted redirect", true, func(g *fakeGitHub) { g.redirectHost = "example.com" }, []string{"untrusted host \"example.com\""}},
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
			if strings.Contains(err.Error(), testToken) {
				t.Fatalf("Latest() error reveals the token: %v", err)
			}
			for _, request := range github.requests {
				if strings.Contains(request.url, "example.com") {
					t.Fatalf("Latest() followed a redirect to %s", request.url)
				}
			}
		})
	}
}

func TestGitHubSourcePointsAnAnonymous404ToPrivateCatalogs(t *testing.T) {
	github := newFakeGitHub(t, "team/private", true)
	_, err := NewGitHubSource(github.client(), &recordingVerifier{}, github.credential).Latest(context.Background(), Registration{Repository: mustRepository(t, "team/private"), Trust: Attested{Workflow: DefaultWorkflow}})
	if err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "--trust access-controlled") {
		t.Fatalf("Latest() error = %v", err)
	}
}

type fakeRequest struct{ url, authorization, accept string }

// fakeGitHub answers as GitHub does for one repository whose latest release
// holds a valid archive and checksum. A private repository answers 404 to a
// request without the token.
type fakeGitHub struct {
	repository      string
	private         bool
	archive         []byte
	responses       map[string]string
	status          int
	redirectHost    string
	credentialErr   error
	credentialCalls int
	mu              sync.Mutex
	requests        []fakeRequest
}

func newFakeGitHub(t *testing.T, repository string, private bool) *fakeGitHub {
	t.Helper()
	commit := strings.Repeat("a", 40)
	archive := []byte("archive")
	checksum := []byte(digest(archive) + "  " + archiveAsset + "\n")
	api := "https://api.github.com/repos/" + repository
	return &fakeGitHub{repository: repository, private: private, archive: archive, redirectHost: "release-assets.githubusercontent.com", responses: map[string]string{
		api: fmt.Sprintf(`{"full_name":%q,"private":%t}`, repository, private),
		api + "/releases/latest": fmt.Sprintf(
			`{"tag_name":"catalog-%s","target_commitish":"%s","draft":false,"prerelease":false,"immutable":true,"published_at":"2026-09-14T00:00:00Z","author":{"login":"github-actions[bot]"},"assets":[{"id":11,"name":"%s","digest":"sha256:%s","size":%d},{"id":12,"name":"%s","digest":"sha256:%s","size":%d}]}`,
			commit, commit, archiveAsset, digest(archive), len(archive), checksumAsset, digest(checksum), len(checksum)),
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
	g.mu.Lock()
	g.requests = append(g.requests, fakeRequest{url: request.URL.String(), authorization: request.Header.Get("Authorization"), accept: request.Header.Get("Accept")})
	g.mu.Unlock()
	url := request.URL.String()
	api := "https://api.github.com/repos/" + g.repository
	if strings.HasPrefix(url, api) {
		switch authorization := request.Header.Get("Authorization"); {
		case g.status != 0 && authorization != "":
			return response(g.status, nil, ""), nil
		case g.private && authorization != "Bearer "+testToken:
			return response(http.StatusNotFound, nil, `{"message":"Not Found"}`), nil
		}
	}
	if id, ok := strings.CutPrefix(url, api+"/releases/assets/"); ok {
		return response(http.StatusFound, http.Header{"Location": {"https://" + g.redirectHost + "/signed/" + id + "?sig=x"}}, ""), nil
	}
	if id, ok := strings.CutPrefix(request.URL.Path, "/signed"); ok && request.URL.Host == "release-assets.githubusercontent.com" {
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
