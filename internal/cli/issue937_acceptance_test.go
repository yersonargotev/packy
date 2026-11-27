package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yersonargotev/packy/internal/githubfake"
	"github.com/yersonargotev/packy/internal/testprocess"
)

const (
	issue937Private = "team/attested-packs"
	issue937Token   = "issue937-private-token"
)

var issue937Snapshot = strings.Repeat("f", 40)

func TestIssue937(t *testing.T) {
	transport := buildIssue929Transport(t, t.TempDir())
	issue929Dists.root = t.TempDir()
	issue929Dists.built.Clear()
	for _, scenario := range []struct {
		id  string
		run func(*testing.T, string)
	}{
		{"B1", testIssue937RegisterPrivateAttestedCatalog},
		{"B2", testIssue937MissingToken},
		{"B3", testIssue937ForeignWorkflow},
	} {
		t.Run(scenario.id, func(t *testing.T) {
			t.Parallel()
			scenario.run(t, transport)
		})
	}
}

type issue937Request struct {
	host, path string
	authorized bool
}

type issue937Requests struct {
	mu      sync.Mutex
	entries []issue937Request
}

func (r *issue937Requests) all() []issue937Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]issue937Request(nil), r.entries...)
}

// startIssue937 serves the official catalog and the private attested catalog
// that only issue937Token reads, refreshes the official catalog, and returns
// an environment that carries no token.
func startIssue937(t *testing.T, transport string, repository githubfake.Repository) (fixtures string, environment []string, requests *issue937Requests) {
	t.Helper()
	fixtures = t.TempDir()
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, strings.Repeat("a", 40), githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	publishIssue929Fixture(t, transport, fixtures, issue937Private, issue937Snapshot, githubfake.Pack{ID: "vault", Version: "1.0.0"})
	repository.Token = issue937Token
	if err := githubfake.WriteRepository(fixtures, issue937Private, repository); err != nil {
		t.Fatal(err)
	}
	requests = &issue937Requests{}
	handler := githubfake.Handler(fixtures)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.mu.Lock()
		requests.entries = append(requests.entries, issue937Request{host: r.Host, path: r.URL.Path, authorized: r.Header.Get("Authorization") != ""})
		requests.mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	addressFile := filepath.Join(t.TempDir(), "addr")
	if err := githubfake.AddressOf(server).Write(addressFile); err != nil {
		t.Fatal(err)
	}
	environment = testprocess.Env(t, "PACKY_GITHUBFAKE_ADDR_FILE="+addressFile, "PATH="+t.TempDir())
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil {
		t.Fatalf("catalog refresh: %v\n%s", err, out)
	}
	return fixtures, environment, requests
}

func testIssue937RegisterPrivateAttestedCatalog(t *testing.T, transport string) {
	_, environment, requests := startIssue937(t, transport, githubfake.Repository{})
	withToken := issue929Setenv(environment, "GH_TOKEN="+issue937Token)
	out, err := runIssue929(t, transport, withToken, "catalog", "add", issue937Private)
	if err != nil || !strings.Contains(out, "registered team/attested-packs (attested by .github/workflows/publish.yml) and selected Catalog Snapshot "+issue937Snapshot) {
		t.Fatalf("catalog add of the private attested catalog: %v\n%s", err, out)
	}
	report := listIssue929Catalogs(t, transport, environment)
	if len(report.Catalogs) != 2 {
		t.Fatalf("catalog list = %+v", report)
	}
	if got := report.Catalogs[1]; got.Repository != issue937Private || got.Trust != "attested" || got.Workflow != ".github/workflows/publish.yml" || got.SnapshotID == nil || *got.SnapshotID != issue937Snapshot {
		t.Fatalf("catalog list entry of the private attested catalog = %+v", got)
	}
	if out, err := runIssue929(t, transport, environment, "catalog", "list"); err != nil || strings.Contains(out, "weaker trust level") {
		t.Fatalf("catalog list marks the private attested catalog as weaker: %v\n%s", err, out)
	}
	if out, err := runIssue929(t, transport, withToken, "catalog", "refresh", issue937Private); err != nil || !strings.Contains(out, "selected Catalog Snapshot "+issue937Snapshot+" of "+issue937Private) {
		t.Fatalf("catalog refresh of the private attested catalog: %v\n%s", err, out)
	}
	out, err = runIssue929(t, transport, environment, "catalog", "refresh", issue937Private)
	if !issue929Exit(err, 1) || !strings.Contains(out, "team/attested-packs failed and keeps Catalog Snapshot "+issue937Snapshot) || !strings.Contains(out, "gh auth login") {
		t.Fatalf("catalog refresh without a token: %v\n%s", err, out)
	}

	attestations := 0
	for _, request := range requests.all() {
		if request.authorized && request.host != "api.github.com" {
			t.Errorf("request to %s%s carried Authorization", request.host, request.path)
		}
		if request.authorized && strings.HasPrefix(request.path, "/repos/"+issue929OfficialCatalog+"/") {
			t.Errorf("request to the official catalog %s carried Authorization", request.path)
		}
		if request.path == "/repos/"+issue937Private {
			t.Errorf("an attested catalog read the repository visibility")
		}
		if request.authorized && strings.HasPrefix(request.path, "/repos/"+issue937Private+"/attestations/sha256:") {
			attestations++
		}
	}
	if attestations != 2 {
		t.Fatalf("the private attestations API received %d authorized requests, want one for add and one for refresh", attestations)
	}
	assertIssue929NoToken(t, filepath.Dir(issue929Home(t, environment)), issue937Token)
}

func testIssue937MissingToken(t *testing.T, transport string) {
	_, environment, _ := startIssue937(t, transport, githubfake.Repository{})
	assertIssue929RefusedAdd(t, transport, environment, []string{"catalog", "add", issue937Private, "--trust", "attested"}, "packy catalog add: read latest catalog release of team/attested-packs: GitHub returned 404 Not Found; if the repository is private, a private catalog needs a GitHub token", "GH_TOKEN", "gh auth login", "docs/private-catalogs.md")
}

func testIssue937ForeignWorkflow(t *testing.T, transport string) {
	_, environment, _ := startIssue937(t, transport, githubfake.Repository{AttestationWorkflow: issue937Private + "/.github/workflows/other.yml"})
	withToken := issue929Setenv(environment, "GH_TOKEN="+issue937Token)
	assertIssue929RefusedAdd(t, transport, withToken, []string{"catalog", "add", issue937Private}, "packy catalog add: verify catalog team/attested-packs artifact attestation", "if team/attested-packs publishes from another workflow, run `packy catalog add team/attested-packs --workflow <path>`")
}
