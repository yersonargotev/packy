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
	"github.com/yersonargotev/packy/internal/githubauth"
	"github.com/yersonargotev/packy/internal/githubfake"
)

const (
	official = "yersonargotev/packy-catalog"
	private  = "example/private-catalog"
)

var (
	commit  = strings.Repeat("c", 40)
	archive = []byte("archive bytes\n")
)

func startFake(t *testing.T) (string, *http.Client) {
	t.Helper()
	root := t.TempDir()
	dist := t.TempDir()
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
	header := http.Header{}
	if authorization != "" {
		header.Set("Authorization", authorization)
	}
	status, _, body := fetch(t, client, url, header)
	return status, body
}

func fetch(t *testing.T, client *http.Client, url string, header http.Header) (int, http.Header, []byte) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header = header
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header, body
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
			URL    string `json:"url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); status != http.StatusOK || err != nil {
		t.Fatalf("releases/latest = %d %s (%v)", status, body, err)
	}
	if release.Tag != "catalog-"+commit || release.Author.Login != "github-actions[bot]" || len(release.Assets) != 2 {
		t.Fatalf("release = %+v", release)
	}
	for _, asset := range release.Assets {
		status, _, data := fetch(t, client, asset.URL, http.Header{"Accept": {"application/octet-stream"}})
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

func TestHandlerServesEarlierReleasesByTag(t *testing.T) {
	root, client := startFake(t)
	newer := strings.Repeat("d", 40)
	newerArchive := []byte("newer archive bytes\n")
	dist := t.TempDir()
	sum := sha256.Sum256(newerArchive)
	if err := os.WriteFile(filepath.Join(dist, "catalog-snapshot.tar.gz"), newerArchive, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "SHA256SUMS"), []byte(hex.EncodeToString(sum[:])+"  catalog-snapshot.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := githubfake.Publish(root, official, newer, dist); err != nil {
		t.Fatal(err)
	}
	type served struct {
		Tag    string `json:"tag_name"`
		Commit string `json:"target_commitish"`
		Assets []struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Digest string `json:"digest"`
			URL    string `json:"url"`
		} `json:"assets"`
	}
	read := func(path string) served {
		t.Helper()
		status, body := get(t, client, "https://api.github.com/repos/"+official+path, "")
		var release served
		if err := json.Unmarshal(body, &release); status != http.StatusOK || err != nil {
			t.Fatalf("GET %s = %d %s (%v)", path, status, body, err)
		}
		return release
	}
	if latest := read("/releases/latest"); latest.Commit != newer {
		t.Fatalf("releases/latest names %s, want %s", latest.Commit, newer)
	}
	ids := map[int64]bool{}
	for _, want := range []struct {
		commit  string
		archive []byte
	}{{newer, newerArchive}, {commit, archive}} {
		release := read("/releases/tags/catalog-" + want.commit)
		if release.Tag != "catalog-"+want.commit || release.Commit != want.commit || len(release.Assets) != 2 {
			t.Fatalf("release of %s = %+v", want.commit, release)
		}
		for _, asset := range release.Assets {
			if ids[asset.ID] {
				t.Fatalf("asset ID %d repeats across releases", asset.ID)
			}
			ids[asset.ID] = true
			status, _, data := fetch(t, client, asset.URL, http.Header{"Accept": {"application/octet-stream"}})
			sum := sha256.Sum256(data)
			if status != http.StatusOK || asset.Digest != "sha256:"+hex.EncodeToString(sum[:]) {
				t.Fatalf("download %s = %d with digest %x, want %s", asset.URL, status, sum, asset.Digest)
			}
			if asset.Name == "catalog-snapshot.tar.gz" && string(data) != string(want.archive) {
				t.Fatalf("archive of %s = %q", want.commit, data)
			}
			if status, _ := get(t, client, "https://api.github.com/repos/"+official+"/attestations/"+asset.Digest, ""); status != http.StatusOK {
				t.Fatalf("attestations of %s = %d", asset.Name, status)
			}
		}
	}
	if status, body := get(t, client, "https://api.github.com/repos/"+official+"/releases/tags/catalog-"+strings.Repeat("e", 40), ""); status != http.StatusNotFound {
		t.Fatalf("an unknown tag = %d %s, want 404", status, body)
	}
}

func TestHandlerAnswersUnknownPathsWith404(t *testing.T) {
	_, client := startFake(t)
	for _, url := range []string{
		"https://api.github.com/repos/example/missing/releases/latest",
		"https://api.github.com/repos/" + official + "/releases",
		"https://api.github.com/repos/" + official + "/attestations/sha256:" + strings.Repeat("0", 64),
		"https://api.github.com/repos/" + official + "/releases/assets/99",
		"https://" + githubfake.AssetHost + "/" + official + "/catalog-other/SHA256SUMS",
		"https://" + githubfake.AssetHost + "/" + official + "/catalog-" + commit + "/release.json",
		"https://" + githubfake.AssetHost + "/anything",
	} {
		if status, body := get(t, client, url, ""); status != http.StatusNotFound {
			t.Errorf("GET %s = %d %s, want 404", url, status, body)
		}
	}
}

func TestHandlerHidesPrivateFixturesWithoutTheirToken(t *testing.T) {
	_, client := startFake(t)
	sum := sha256.Sum256(archive)
	for _, url := range []string{
		"https://api.github.com/repos/" + private,
		"https://api.github.com/repos/" + private + "/releases/latest",
		"https://api.github.com/repos/" + private + "/releases/assets/1",
		"https://api.github.com/repos/" + private + "/attestations/sha256:" + hex.EncodeToString(sum[:]),
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

func TestAssetAPIRedirectsToAnAssetHostThatRefusesTheToken(t *testing.T) {
	_, client := startFake(t)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	status, header, _ := fetch(t, client, "https://api.github.com/repos/"+private+"/releases/assets/1", http.Header{"Accept": {"application/octet-stream"}, "Authorization": {"Bearer secret"}})
	location := header.Get("Location")
	if status != http.StatusFound || !strings.HasPrefix(location, "https://"+githubfake.AssetHost+"/"+private+"/") {
		t.Fatalf("asset API = %d to %q", status, location)
	}
	if status, _, body := fetch(t, client, location, http.Header{}); status != http.StatusOK || len(body) == 0 {
		t.Fatalf("asset host without a token = %d %s", status, body)
	}
	if status, _, _ := fetch(t, client, location, http.Header{"Authorization": {"Bearer secret"}}); status != http.StatusBadRequest {
		t.Fatalf("asset host with a token = %d, want 400", status)
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
	source := catalogstore.NewGitHubSource(client, githubfake.AttestationVerifier{}, func(ctx context.Context) (githubauth.Token, error) {
		return githubauth.Lookup(ctx, func(name string) string {
			if name == "GH_TOKEN" {
				return "secret"
			}
			return ""
		})
	})
	privateRegistration := catalogstore.Registration{Repository: mustRepository(t, private), Trust: catalogstore.AccessControlled{}}
	if release, err := source.Latest(context.Background(), privateRegistration); err != nil || release.AttestationVerified || release.Commit != commit || len(release.Assets) != 2 {
		t.Fatalf("Latest() of the private catalog = %+v, %v", release, err)
	}
	privateRegistration.Trust = catalogstore.Attested{Workflow: catalogstore.DefaultWorkflow}
	if release, err := source.Latest(context.Background(), privateRegistration); err != nil || !release.AttestationVerified || release.Commit != commit {
		t.Fatalf("Latest() of the private attested catalog = %+v, %v", release, err)
	}
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

func mustRepository(t *testing.T, value string) catalogstore.Repository {
	t.Helper()
	repository, err := catalogstore.ParseRepository(value)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
