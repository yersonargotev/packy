package catalogstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type recordingVerifier struct {
	repository, digest, workflow string
	err                          error
}

func (v *recordingVerifier) Verify(_ context.Context, repository, digest, workflow string) error {
	v.repository, v.digest, v.workflow = repository, digest, workflow
	return v.err
}

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
			client, archive := releaseClient(t, repository)
			verifier := &recordingVerifier{}
			release, err := NewGitHubSource(client, verifier).Latest(context.Background(), test.registration)
			if err != nil {
				t.Fatal(err)
			}
			if !release.AttestationVerified || release.Repository != repository || verifier.repository != repository || verifier.digest != digest(archive) || verifier.workflow != test.signer {
				t.Fatalf("release/verifier = %#v / %#v", release, verifier)
			}
		})
	}
}

func TestGitHubSourceFailsClosedWithoutAttestationVerifier(t *testing.T) {
	_, err := NewGitHubSource(http.DefaultClient, nil).Latest(context.Background(), Official())
	if err == nil || !strings.Contains(err.Error(), "requires an artifact attestation verifier") {
		t.Fatalf("Latest() error = %v", err)
	}
}

func TestGitHubSourceRefusesAccessControlledCatalogs(t *testing.T) {
	client, _ := releaseClient(t, "acme/private")
	registration := Registration{Repository: mustRepository(t, "acme/private"), Trust: AccessControlled{}}
	if _, err := NewGitHubSource(client, &recordingVerifier{}).Latest(context.Background(), registration); err == nil || !strings.Contains(err.Error(), "only attested") {
		t.Fatalf("Latest() error = %v", err)
	}
}

func releaseClient(t *testing.T, repository string) (*http.Client, []byte) {
	t.Helper()
	commit := strings.Repeat("a", 40)
	archive := []byte("archive")
	checksum := []byte(digest(archive) + "  " + archiveAsset + "\n")
	responses := map[string]string{
		"https://api.github.com/repos/" + repository + "/releases/latest": fmt.Sprintf(
			`{"tag_name":"catalog-%s","target_commitish":"%s","draft":false,"prerelease":false,"immutable":true,"published_at":"2026-09-14T00:00:00Z","author":{"login":"github-actions[bot]"},"assets":[{"name":"%s","digest":"sha256:%s","browser_download_url":"https://github.com/archive","size":%d},{"name":"%s","digest":"sha256:%s","browser_download_url":"https://github.com/checksum","size":%d}]}`,
			commit, commit, archiveAsset, digest(archive), len(archive), checksumAsset, digest(checksum), len(checksum)),
		"https://github.com/archive":  string(archive),
		"https://github.com/checksum": string(checksum),
	}
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, ok := responses[request.URL.String()]
		if !ok {
			return nil, fmt.Errorf("unexpected URL %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}, archive
}

func mustRepository(t *testing.T, value string) Repository {
	t.Helper()
	repository, err := ParseRepository(value)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
