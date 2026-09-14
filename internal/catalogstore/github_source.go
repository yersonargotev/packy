package catalogstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const officialSignerWorkflow = "yersonargotev/packy-catalog/.github/workflows/publish.yml"

// AttestationVerifier verifies GitHub build-provenance cryptography and binds
// the subject digest to the expected repository and signer workflow.
type AttestationVerifier interface {
	Verify(ctx context.Context, repository, subjectSHA256, signerWorkflow string) error
}

// NewGitHubSource constructs the production GitHub release source. The caller
// owns HTTP timeouts and authentication. Acquisition fails closed when verifier
// is nil or cannot verify the archive's build-provenance attestation.
func NewGitHubSource(client *http.Client, verifier AttestationVerifier) Source {
	if client == nil {
		client = http.DefaultClient
	}
	return &gitHubSource{client: client, verifier: verifier}
}

type gitHubSource struct {
	client   *http.Client
	verifier AttestationVerifier
}

func (source *gitHubSource) Latest(ctx context.Context, repository string) (Release, error) {
	if repository != officialRepository {
		return Release{}, fmt.Errorf("GitHub catalog Source only supports %s", officialRepository)
	}
	if source.verifier == nil {
		return Release{}, errors.New("GitHub catalog Source requires an artifact attestation verifier")
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
			Name   string `json:"name"`
			Digest string `json:"digest"`
			URL    string `json:"browser_download_url"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if err := source.getJSON(ctx, "https://api.github.com/repos/"+repository+"/releases/latest", &wire); err != nil {
		return Release{}, fmt.Errorf("read latest official catalog release: %w", err)
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
		data, err := source.download(ctx, candidate.URL)
		if err != nil {
			return Release{}, fmt.Errorf("download catalog asset %q: %w", candidate.Name, err)
		}
		release.Assets = append(release.Assets, Asset{Name: candidate.Name, SHA256: candidate.Digest, Data: data})
	}
	assets, err := validateReleaseWithoutAttestation(release)
	if err != nil {
		return Release{}, err
	}
	archiveDigest := digest(assets[archiveAsset].Data)
	if err := source.verifier.Verify(ctx, repository, archiveDigest, officialSignerWorkflow); err != nil {
		return Release{}, fmt.Errorf("verify official catalog artifact attestation: %w", err)
	}
	release.AttestationVerified = true
	return release, nil
}

func (source *gitHubSource) getJSON(ctx context.Context, url string, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "packy-catalog-consumer")
	response, err := source.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return fmt.Errorf("GitHub returned %s", response.Status)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(result); err != nil {
		return err
	}
	return nil
}

func (source *gitHubSource) download(ctx context.Context, url string) ([]byte, error) {
	if !strings.HasPrefix(url, "https://github.com/") && !strings.HasPrefix(url, "https://objects.githubusercontent.com/") {
		return nil, errors.New("GitHub asset URL is not trusted")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "packy-catalog-consumer")
	response, err := source.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxArchiveBytes {
		return nil, errors.New("download exceeds size limit")
	}
	return data, nil
}

func validateReleaseWithoutAttestation(release Release) (map[string]Asset, error) {
	release.AttestationVerified = true
	return validateRelease(release)
}
