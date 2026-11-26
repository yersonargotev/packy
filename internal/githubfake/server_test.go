package githubfake_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/githubfake"
)

const (
	official = "yersonargotev/packy-catalog"
	private  = "example/private-catalog"
)

var commit = strings.Repeat("c", 40)

func startFake(t *testing.T) (string, *http.Client) {
	t.Helper()
	root := t.TempDir()
	dist := t.TempDir()
	archive := []byte("archive bytes\n")
	sum := sha256.Sum256(archive)
	if err := os.WriteFile(filepath.Join(dist, "catalog-snapshot.tar.gz"), archive, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "SHA256SUMS"), []byte(hex.EncodeToString(sum[:])+"  catalog-snapshot.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, repository := range []string{official, private} {
		if err := githubfake.Publish(root, repository, commit, dist); err != nil {
			t.Fatal(err)
		}
	}
	if err := githubfake.WriteRepository(root, private, githubfake.Repository{Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(githubfake.Handler(root))
	t.Cleanup(server.Close)
	addressFile := filepath.Join(t.TempDir(), "addr")
	if err := githubfake.AddressOf(server).Write(addressFile); err != nil {
		t.Fatal(err)
	}
	return root, githubfake.NewClient(addressFile)
}

func get(t *testing.T, client *http.Client, url, authorization string) (int, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, body
}

func TestHandlerRoutesGitHubEndpointsFromFixtures(t *testing.T) {
	_, client := startFake(t)
	status, body := get(t, client, "https://api.github.com/repos/"+official+"/releases/latest", "")
	var release struct {
		Tag    string `json:"tag_name"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
			URL    string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); status != http.StatusOK || err != nil {
		t.Fatalf("releases/latest = %d %s (%v)", status, body, err)
	}
	if release.Tag != "catalog-"+commit || release.Author.Login != "github-actions[bot]" || len(release.Assets) != 2 {
		t.Fatalf("release = %+v", release)
	}
	for _, asset := range release.Assets {
		status, data := get(t, client, asset.URL, "")
		sum := sha256.Sum256(data)
		if status != http.StatusOK || asset.Digest != "sha256:"+hex.EncodeToString(sum[:]) {
			t.Fatalf("download %s = %d with digest %x, want %s", asset.URL, status, sum, asset.Digest)
		}
	}
	if status, body := get(t, client, "https://api.github.com/repos/"+official, ""); status != http.StatusOK || !strings.Contains(string(body), `"private":false`) {
		t.Fatalf("repository metadata = %d %s", status, body)
	}
	if status, body := get(t, client, "https://api.github.com/repos/"+official+"/attestations/"+release.Assets[0].Digest, ""); status != http.StatusOK || !strings.Contains(string(body), official+"/.github/workflows/publish.yml") {
		t.Fatalf("attestations = %d %s", status, body)
	}
}

func TestHandlerAnswersUnknownPathsWith404(t *testing.T) {
	_, client := startFake(t)
	for _, url := range []string{
		"https://api.github.com/repos/example/missing/releases/latest",
		"https://api.github.com/repos/" + official + "/releases",
		"https://api.github.com/repos/" + official + "/attestations/sha256:" + strings.Repeat("0", 64),
		"https://github.com/" + official + "/releases/download/catalog-other/SHA256SUMS",
		"https://github.com/" + official + "/releases/download/catalog-" + commit + "/release.json",
		"https://objects.githubusercontent.com/anything",
	} {
		if status, body := get(t, client, url, ""); status != http.StatusNotFound {
			t.Errorf("GET %s = %d %s, want 404", url, status, body)
		}
	}
}

func TestHandlerHidesPrivateFixturesWithoutTheirToken(t *testing.T) {
	_, client := startFake(t)
	for _, url := range []string{
		"https://api.github.com/repos/" + private,
		"https://api.github.com/repos/" + private + "/releases/latest",
		"https://github.com/" + private + "/releases/download/catalog-" + commit + "/SHA256SUMS",
	} {
		if status, _ := get(t, client, url, ""); status != http.StatusNotFound {
			t.Errorf("GET %s without a token = %d, want 404", url, status)
		}
		if status, _ := get(t, client, url, "Bearer wrong"); status != http.StatusUnauthorized {
			t.Errorf("GET %s with a wrong token = %d, want 401", url, status)
		}
		if status, _ := get(t, client, url, "Bearer secret"); status != http.StatusOK {
			t.Errorf("GET %s with the token = %d, want 200", url, status)
		}
	}
	if _, body := get(t, client, "https://api.github.com/repos/"+private, "token secret"); !strings.Contains(string(body), `"private":true`) {
		t.Fatalf("private repository metadata = %s", body)
	}
}

func TestClientRefusesHostsOutsideGitHub(t *testing.T) {
	_, client := startFake(t)
	response, err := client.Get("https://tuf-repo-cdn.sigstore.dev/root.json")
	if err == nil {
		response.Body.Close()
		t.Fatal("client reached a host outside GitHub")
	}
}

func TestProductionGitHubSourceAcquiresThroughTheFake(t *testing.T) {
	root, client := startFake(t)
	source := catalogstore.NewGitHubSource(client, githubfake.NewAttestationVerifier(client))
	release, err := source.Latest(context.Background(), catalogstore.Official())
	if err != nil {
		t.Fatal(err)
	}
	if !release.AttestationVerified || release.Commit != commit || len(release.Assets) != 2 {
		t.Fatalf("release = %+v", release)
	}

	if err := githubfake.WriteRepository(root, official, githubfake.Repository{AttestationWorkflow: "example/fork/.github/workflows/publish.yml"}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Latest(context.Background(), catalogstore.Official()); err == nil || !strings.Contains(err.Error(), "attestation") {
		t.Fatalf("Latest() with a foreign signer workflow = %v", err)
	}

	if err := githubfake.WriteRepository(root, official, githubfake.Repository{ReleaseStatus: http.StatusInternalServerError}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Latest(context.Background(), catalogstore.Official()); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("Latest() with a failing releases/latest = %v", err)
	}
}
