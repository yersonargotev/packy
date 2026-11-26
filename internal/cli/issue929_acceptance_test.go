package cli

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/githubfake"
	"github.com/yersonargotev/packy/internal/testprocess"
)

const issue929OfficialCatalog = "yersonargotev/packy-catalog"

func TestIssue929(t *testing.T) {
	var transport string
	for _, scenario := range []struct {
		id, name, greenIn string
		run               func(*testing.T, string)
	}{
		{"A0", "Refresh the official catalog through the controlled transport", "P1", testIssue929OfficialRefresh},
		{"A1", "Register a public attested catalog", "P4", nil},
		{"A2", "Register a private access-controlled catalog with a token", "P5", nil},
		{"A3", "Registration with an invalid snapshot leaves no residue", "P4", nil},
		{"A4", "Registration with a Pack ID collision leaves no residue", "P4", nil},
		{"A5", "access-controlled on a public repository is refused", "P5", nil},
		{"A6", "A missing token is refused with an explanation", "P5", nil},
		{"A7", "A rejected token is refused", "P5", nil},
		{"A8", "A failed attestation is refused", "P4", nil},
		{"A9", "Refresh all catalogs and refresh one", "P4", nil},
		{"A10", "One catalog fails while another succeeds, exit 1, previous selection usable", "P4", nil},
		{"A11", "An incompatible snapshot is rejected whole and the previous one stays usable", "P2 for official, P4 for others", nil},
		{"A12", "Full lifecycle of a non-official Pack, own-catalog updates, uninstall then install from another catalog", "P4", nil},
		{"A13", "Removal refused while a receipt references it, and removal keeps snapshots", "P4", nil},
		{"A14", "A project lock naming an unregistered catalog fails and names it", "P4", nil},
		{"A15", "Offline operation from retained snapshots", "P3 for official, P4 for others", nil},
		{"A16", "Earlier store, receipt, and lock schemas are rejected with adoption guidance", "P2 and P3", nil},
		{"A17", "The TUI shows catalogs, trust, snapshots, Pack catalogs, and refreshes", "P6", nil},
	} {
		t.Run(scenario.id, func(t *testing.T) {
			if scenario.run == nil {
				t.Skipf("%s turns green in %s", scenario.name, scenario.greenIn)
			}
			if transport == "" {
				transport = buildIssue929Transport(t)
			}
			scenario.run(t, transport)
		})
	}
}

func testIssue929OfficialRefresh(t *testing.T, transport string) {
	commit := strings.Repeat("a", 40)
	fixtures := t.TempDir()
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, commit, githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	environment := startIssue929GitHub(t, fixtures)

	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil || !strings.Contains(out, "selected official Catalog Snapshot "+commit) {
		t.Fatalf("catalog refresh: %v\n%s", err, out)
	}
	out, err := runIssue929(t, transport, environment, "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	var report struct {
		Packs []struct {
			ID      string `json:"id"`
			Version string `json:"version"`
		} `json:"packs"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil || len(report.Packs) != 1 || report.Packs[0].ID != "fixture" || report.Packs[0].Version != "1.0.0" {
		t.Fatalf("list --json = %s (%v)", out, err)
	}
}

func buildIssue929Transport(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "packy")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-ldflags=-X github.com/yersonargotev/packy/internal/version.SourceCommit="+strings.Repeat("2", 40), "-o", binary, "./internal/cli/testdata/packytransport")
	build.Dir = filepath.Join("..", "..")
	build.Env = testprocess.GoOfflineEnv(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build transport executable: %v\n%s", err, out)
	}
	return binary
}

func publishIssue929Fixture(t *testing.T, transport, fixtures, repository, commit string, packs ...githubfake.Pack) {
	t.Helper()
	project, dist := t.TempDir(), filepath.Join(t.TempDir(), "dist")
	if err := githubfake.WriteCatalogProject(project, packs...); err != nil {
		t.Fatal(err)
	}
	if out, err := runIssue929(t, transport, testprocess.Env(t), "catalog", "build", "--project", project, "--source-repository", repository, "--source-commit", commit, "--out-dir", dist); err != nil {
		t.Fatalf("catalog build: %v\n%s", err, out)
	}
	if err := githubfake.Publish(fixtures, repository, commit, dist); err != nil {
		t.Fatal(err)
	}
}

func startIssue929GitHub(t *testing.T, fixtures string) []string {
	t.Helper()
	server := httptest.NewTLSServer(githubfake.Handler(fixtures))
	t.Cleanup(server.Close)
	addressFile := filepath.Join(t.TempDir(), "addr")
	if err := githubfake.AddressOf(server).Write(addressFile); err != nil {
		t.Fatal(err)
	}
	return testprocess.Env(t, "PACKY_GITHUBFAKE_ADDR_FILE="+addressFile)
}

func runIssue929(t *testing.T, transport string, environment []string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, transport, args...)
	command.Dir = t.TempDir()
	command.Env = environment
	out, err := command.CombinedOutput()
	return string(out), err
}
