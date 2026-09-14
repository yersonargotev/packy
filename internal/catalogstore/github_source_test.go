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
	commit := strings.Repeat("a", 40)
	archive := []byte("archive")
	checksum := []byte(digest(archive) + "  " + archiveAsset + "\n")
	responses := map[string]string{
		"https://api.github.com/repos/" + officialRepository + "/releases/latest": fmt.Sprintf(
			`{"tag_name":"catalog-%s","target_commitish":"%s","draft":false,"prerelease":false,"immutable":true,"published_at":"2026-09-14T00:00:00Z","author":{"login":"github-actions[bot]"},"assets":[{"name":"%s","digest":"sha256:%s","browser_download_url":"https://github.com/archive","size":%d},{"name":"%s","digest":"sha256:%s","browser_download_url":"https://github.com/checksum","size":%d}]}`,
			commit, commit, archiveAsset, digest(archive), len(archive), checksumAsset, digest(checksum), len(checksum)),
		"https://github.com/archive":  string(archive),
		"https://github.com/checksum": string(checksum),
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, ok := responses[request.URL.String()]
		if !ok {
			return nil, fmt.Errorf("unexpected URL %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	verifier := &recordingVerifier{}
	release, err := NewGitHubSource(client, verifier).Latest(context.Background(), officialRepository)
	if err != nil {
		t.Fatal(err)
	}
	if !release.AttestationVerified || verifier.repository != officialRepository || verifier.digest != digest(archive) || verifier.workflow != officialSignerWorkflow {
		t.Fatalf("release/verifier = %#v / %#v", release, verifier)
	}
}

func TestGitHubSourceFailsClosedWithoutAttestationVerifier(t *testing.T) {
	_, err := NewGitHubSource(http.DefaultClient, nil).Latest(context.Background(), officialRepository)
	if err == nil || !strings.Contains(err.Error(), "requires an artifact attestation verifier") {
		t.Fatalf("Latest() error = %v", err)
	}
}
