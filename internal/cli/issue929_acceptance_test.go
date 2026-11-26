package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/packy/internal/cataloglayout"
	"github.com/yersonargotev/packy/internal/githubfake"
	"github.com/yersonargotev/packy/internal/testprocess"
)

const issue929OfficialCatalog = "yersonargotev/packy-catalog"

func TestIssue929(t *testing.T) {
	var transport string
	binaries := t.TempDir()
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
		{"A11", "An incompatible snapshot is rejected whole and the previous one stays usable", "P2 for official, P4 for others", testIssue929IncompatibleOfficialSnapshot},
		{"A12", "Full lifecycle of a non-official Pack, own-catalog updates, uninstall then install from another catalog", "P4", nil},
		{"A13", "Removal refused while a receipt references it, and removal keeps snapshots", "P4", nil},
		{"A14", "A project lock naming an unregistered catalog fails and names it", "P4", nil},
		{"A15", "Offline operation from retained snapshots", "P3 for official, P4 for others", nil},
		{"A16", "Earlier store, receipt, and lock schemas are rejected with adoption guidance", "P2 and P3", testIssue929EarlierStoreLayout},
		{"A17", "The TUI shows catalogs, trust, snapshots, Pack catalogs, and refreshes", "P6", nil},
	} {
		t.Run(scenario.id, func(t *testing.T) {
			if scenario.run == nil {
				t.Skipf("%s turns green in %s", scenario.name, scenario.greenIn)
			}
			if transport == "" {
				transport = buildIssue929Transport(t, binaries)
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

func testIssue929IncompatibleOfficialSnapshot(t *testing.T, transport string) {
	good, incompatible := strings.Repeat("a", 40), strings.Repeat("b", 40)
	fixtures := t.TempDir()
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, good, githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	environment := startIssue929GitHub(t, fixtures)
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil {
		t.Fatalf("catalog refresh: %v\n%s", err, out)
	}
	selection := filepath.Join(issue929Home(t, environment), ".local", "share", "packy", "catalog", "v2", "yersonargotev", "packy-catalog", "selected.json")
	before, err := os.ReadFile(selection)
	if err != nil {
		t.Fatal(err)
	}

	dist := buildIssue929Dist(t, transport, issue929OfficialCatalog, incompatible, githubfake.Pack{ID: "fixture", Version: "2.0.0"}, githubfake.Pack{ID: "other", Version: "1.0.0"})
	rewriteIssue929Manifest(t, dist, "other", func(manifest string) string {
		return strings.Replace(manifest, `"description": "Fixture Pack",`, `"description": "Fixture Pack", "future_engine_field": true,`, 1)
	})
	if err := githubfake.Publish(fixtures, issue929OfficialCatalog, incompatible, dist); err != nil {
		t.Fatal(err)
	}
	out, err := runIssue929(t, transport, environment, "catalog", "refresh")
	if err == nil || !strings.Contains(out, `Pack "other" requires a newer Packy engine`) {
		t.Fatalf("catalog refresh of an incompatible snapshot: %v\n%s", err, out)
	}
	if after, err := os.ReadFile(selection); err != nil || !bytes.Equal(after, before) {
		t.Fatalf("selection changed after a refused refresh: %s %v", after, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(selection), "snapshots", incompatible)); !os.IsNotExist(err) {
		t.Fatalf("incompatible snapshot was retained: %v", err)
	}
	if packs := listIssue929Packs(t, transport, environment); packs != "fixture@1.0.0" {
		t.Fatalf("list after a refused refresh = %s", packs)
	}
}

func testIssue929EarlierStoreLayout(t *testing.T, transport string) {
	commit := strings.Repeat("a", 40)
	fixtures := t.TempDir()
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, commit, githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	environment := startIssue929GitHub(t, fixtures)
	catalog := filepath.Join(issue929Home(t, environment), ".local", "share", "packy", "catalog")
	earlier := map[string]string{
		"selected.json": `{"schema_version":1,"snapshot_id":"` + commit + `"}` + "\n",
		filepath.Join("snapshots", commit, "catalog-index.json"): "{}\n",
	}
	for name, content := range earlier {
		path := filepath.Join(catalog, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if out, err := runIssue929(t, transport, environment, "list", "--json"); err == nil || !strings.Contains(out, "docs/catalog-adoption.md") {
		t.Fatalf("list over the earlier store layout: %v\n%s", err, out)
	}
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil {
		t.Fatalf("catalog refresh beside the earlier store layout: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(catalog, "v2", "yersonargotev", "packy-catalog", "selected.json")); err != nil {
		t.Fatal(err)
	}
	if packs := listIssue929Packs(t, transport, environment); packs != "fixture@1.0.0" {
		t.Fatalf("list after refresh = %s", packs)
	}
	for name, content := range earlier {
		if data, err := os.ReadFile(filepath.Join(catalog, name)); err != nil || string(data) != content {
			t.Fatalf("earlier store file %s changed: %q %v", name, data, err)
		}
	}
}

func listIssue929Packs(t *testing.T, transport string, environment []string) string {
	t.Helper()
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
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("list --json = %s (%v)", out, err)
	}
	packs := []string{}
	for _, pack := range report.Packs {
		packs = append(packs, pack.ID+"@"+pack.Version)
	}
	return strings.Join(packs, ",")
}

func issue929Home(t *testing.T, environment []string) string {
	t.Helper()
	for _, entry := range environment {
		if home, ok := strings.CutPrefix(entry, "HOME="); ok {
			return home
		}
	}
	t.Fatal("test process environment has no HOME")
	return ""
}

// rewriteIssue929Manifest edits one Pack manifest of a built snapshot and
// recomputes the index and checksum, as a newer Packy build would publish them.
func rewriteIssue929Manifest(t *testing.T, dist, packID string, mutate func(string) string) {
	t.Helper()
	archivePath := filepath.Join(dist, "catalog-snapshot.tar.gz")
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(gz)
	type entry struct {
		header *tar.Header
		data   []byte
	}
	var entries []entry
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry{header, content})
	}
	sum := func(data []byte) string { digest := sha256.Sum256(data); return hex.EncodeToString(digest[:]) }
	manifestPath := "packs/" + packID + "/pack.json"
	var manifest []byte
	for i := range entries {
		if entries[i].header.Name == manifestPath {
			entries[i].data = []byte(mutate(string(entries[i].data)))
			manifest = entries[i].data
		}
	}
	for i := range entries {
		if entries[i].header.Name != "catalog-index.json" {
			continue
		}
		var index cataloglayout.CatalogSnapshotIndex
		if err := json.Unmarshal(entries[i].data, &index); err != nil {
			t.Fatal(err)
		}
		for p := range index.Packs {
			pack := &index.Packs[p]
			if pack.ID != packID {
				continue
			}
			closure := sha256.New()
			for f := range pack.Files {
				if pack.Files[f].Path == "pack.json" {
					pack.Files[f].SHA256 = sum(manifest)
					pack.ManifestSHA256 = sum(manifest)
				}
				fmt.Fprintf(closure, "%s\x00%s\x00%s\n", pack.Files[f].Path, pack.Files[f].Mode, pack.Files[f].SHA256)
			}
			pack.ClosureSHA256 = hex.EncodeToString(closure.Sum(nil))
		}
		packs, err := json.Marshal(index.Packs)
		if err != nil {
			t.Fatal(err)
		}
		index.CatalogSHA256 = sum(packs)
		if entries[i].data, err = json.Marshal(index); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	gzw := gzip.NewWriter(&output)
	writer := tar.NewWriter(gzw)
	for _, entry := range entries {
		entry.header.Size = int64(len(entry.data))
		if err := writer.WriteHeader(entry.header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, output.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "SHA256SUMS"), []byte(sum(output.Bytes())+"  catalog-snapshot.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func buildIssue929Transport(t *testing.T, directory string) string {
	t.Helper()
	binary := filepath.Join(directory, "packy")
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
	if err := githubfake.Publish(fixtures, repository, commit, buildIssue929Dist(t, transport, repository, commit, packs...)); err != nil {
		t.Fatal(err)
	}
}

func buildIssue929Dist(t *testing.T, transport, repository, commit string, packs ...githubfake.Pack) string {
	t.Helper()
	project, dist := t.TempDir(), filepath.Join(t.TempDir(), "dist")
	if err := githubfake.WriteCatalogProject(project, packs...); err != nil {
		t.Fatal(err)
	}
	if out, err := runIssue929(t, transport, testprocess.Env(t), "catalog", "build", "--project", project, "--source-repository", repository, "--source-commit", commit, "--out-dir", dist); err != nil {
		t.Fatalf("catalog build: %v\n%s", err, out)
	}
	return dist
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
