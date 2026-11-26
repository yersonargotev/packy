package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
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
		{"A1", "Register a public attested catalog", "P4", testIssue929RegisterCatalog},
		{"A2", "Register a private access-controlled catalog with a token", "P5", nil},
		{"A3", "Registration with an invalid snapshot leaves no residue", "P4", testIssue929InvalidRegistration},
		{"A4", "Registration with a Pack ID collision leaves no residue", "P4", testIssue929PackIDCollision},
		{"A5", "access-controlled on a public repository is refused", "P5", nil},
		{"A6", "A missing token is refused with an explanation", "P5", nil},
		{"A7", "A rejected token is refused", "P5", nil},
		{"A8", "A failed attestation is refused", "P4", testIssue929FailedAttestation},
		{"A9", "Refresh all catalogs and refresh one", "P4", testIssue929RefreshAllAndOne},
		{"A10", "One catalog fails while another succeeds, exit 1, previous selection usable", "P4", testIssue929PartialRefresh},
		{"A11", "An incompatible snapshot is rejected whole and the previous one stays usable", "P2 for official, P4 for others", testIssue929IncompatibleSnapshots},
		{"A12", "Full lifecycle of a non-official Pack, own-catalog updates, uninstall then install from another catalog", "P4", testIssue929OtherCatalogLifecycle},
		{"A13", "Removal refused while a receipt references it, and removal keeps snapshots", "P4", testIssue929RemoveCatalog},
		{"A14", "A project lock naming an unregistered catalog fails and names it", "P4", testIssue929UnregisteredLock},
		{"A15", "Offline operation from retained snapshots", "P3 for official, P4 for others", testIssue929Offline},
		{"A16", "Earlier store, receipt, and lock schemas are rejected with adoption guidance", "P2 and P3", testIssue929EarlierSchemas},
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

	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil || !strings.Contains(out, "selected Catalog Snapshot "+commit+" of "+issue929OfficialCatalog) {
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

func testIssue929IncompatibleSnapshots(t *testing.T, transport string) {
	t.Run("official", func(t *testing.T) { testIssue929IncompatibleOfficialSnapshot(t, transport) })
	t.Run("other catalog", func(t *testing.T) { testIssue929IncompatibleOtherSnapshot(t, transport) })
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
	if !issue929Exit(err, 1) || !strings.Contains(out, `Pack "other" requires a newer Packy engine`) {
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

func testIssue929Offline(t *testing.T, transport string) {
	t.Run("official", func(t *testing.T) { testIssue929OfficialOffline(t, transport) })
	t.Run("other catalog", func(t *testing.T) { testIssue929OtherCatalogOffline(t, transport) })
}

// publishIssue929Portable publishes Packs whose skill also declares on-demand
// access and a plugin export, so get skill and export can read them.
func publishIssue929Portable(t *testing.T, transport, fixtures, repository, commit string, packs ...githubfake.Pack) {
	t.Helper()
	project := t.TempDir()
	if err := githubfake.WriteCatalogProject(project, packs...); err != nil {
		t.Fatal(err)
	}
	for _, pack := range packs {
		manifest := filepath.Join(project, "packs", pack.ID, "pack.json")
		data, err := os.ReadFile(manifest)
		if err != nil {
			t.Fatal(err)
		}
		portable := `"surface_exclusions": [], "on_demand": {"protocol": "skill-tree-v1", "prerequisites": []}, "plugin_exports": [{"format": "agent-plugins", "name": "` + pack.ID + `", "prerequisites": [], "references": [], "roles": []}]`
		if !bytes.Contains(data, []byte(`"surface_exclusions": []`)) {
			t.Fatalf("fixture manifest has no surface_exclusions to extend:\n%s", data)
		}
		if err := os.WriteFile(manifest, bytes.Replace(data, []byte(`"surface_exclusions": []`), []byte(portable), 1), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := githubfake.Publish(fixtures, repository, commit, buildIssue929DistFrom(t, transport, project, repository, commit)); err != nil {
		t.Fatal(err)
	}
}

func testIssue929OfficialOffline(t *testing.T, transport string) {
	commit := strings.Repeat("a", 40)
	fixtures := t.TempDir()
	publishIssue929Portable(t, transport, fixtures, issue929OfficialCatalog, commit, githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	online := startIssue929GitHub(t, fixtures)
	if out, err := runIssue929(t, transport, online, "catalog", "refresh"); err != nil {
		t.Fatalf("catalog refresh: %v\n%s", err, out)
	}
	offline := issue929Offline(t, online)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	approveIssue929(t, transport, offline, project, "activate", "fixture", "--surface", "agents")
	approveIssue929(t, transport, offline, project, "install", "fixture", "--surface", "agents")
	exported := filepath.Join(t.TempDir(), "plugin")
	if err := os.Mkdir(exported, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"list", "--json"}, {"show", "fixture"}, {"status"}, {"status", "--project"}, {"export", "fixture", "--surface", "agents", "--output", exported}} {
		if out, err := runIssue929At(t, transport, offline, project, args...); err != nil {
			t.Fatalf("offline %v: %v\n%s", args, err, out)
		}
	}
	out, err := runIssue929At(t, transport, offline, project, "get", "skill", "fixture:fixture", "--surface", "agents", "--json")
	var access struct {
		EntryDocument string `json:"entry_document"`
	}
	if err != nil || json.Unmarshal([]byte(out), &access) != nil || !strings.Contains(access.EntryDocument, filepath.Join("skill-access", "v3", "yersonargotev", "packy-catalog", commit)) {
		t.Fatalf("offline get skill: %v\n%s", err, out)
	}

	want := map[string]string{"catalog_repository": issue929OfficialCatalog, "catalog_snapshot": commit}
	for _, path := range []string{filepath.Join(issue929Home(t, offline), ".packy", "packs.json"), filepath.Join(project, "packy.lock.json")} {
		var document struct {
			SchemaVersion int `json:"schema_version"`
			Receipts      []struct {
				Pack map[string]string `json:"pack"`
			} `json:"receipts"`
		}
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, &document) != nil || document.SchemaVersion != 2 || len(document.Receipts) != 1 || document.Receipts[0].Pack["catalog_repository"] != want["catalog_repository"] || document.Receipts[0].Pack["catalog_snapshot"] != want["catalog_snapshot"] {
			t.Fatalf("%s does not bind its receipt to %v: %s %v", path, want, data, err)
		}
	}
	lock, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	assertProjectStructuredOutput(t, filepath.Join("..", ".."), "project-lock.schema.json", string(lock))
	for name, broken := range map[string][]byte{
		"repository without snapshot": bytes.Replace(lock, []byte(`"catalog_snapshot": "`+commit+`"`), []byte(`"unused": ""`), 1),
		"dot-segment repository":      bytes.Replace(lock, []byte(issue929OfficialCatalog), []byte("../.."), 1),
	} {
		var document map[string]any
		if err := json.Unmarshal(broken, &document); err != nil {
			t.Fatal(err)
		}
		pack := document["receipts"].([]any)[0].(map[string]any)["pack"].(map[string]any)
		delete(pack, "unused")
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateProjectStructuredOutput(t, filepath.Join("..", ".."), "project-lock.schema.json", string(encoded)); err == nil {
			t.Fatalf("project lock schema accepted a receipt with a %s", name)
		}
	}
	var provenance map[string]any
	data, err := os.ReadFile(filepath.Join(exported, "packy-provenance.json"))
	if err != nil || json.Unmarshal(data, &provenance) != nil || provenance["catalog_repository"] != issue929OfficialCatalog || provenance["snapshot_id"] != commit {
		t.Fatalf("plugin provenance = %s %v", data, err)
	}
}

func testIssue929EarlierSchemas(t *testing.T, transport string) {
	t.Run("store", func(t *testing.T) { testIssue929EarlierStoreLayout(t, transport) })
	t.Run("receipts and locks", func(t *testing.T) { testIssue929EarlierReceiptsAndLocks(t, transport) })
}

func testIssue929EarlierReceiptsAndLocks(t *testing.T, transport string) {
	commit := strings.Repeat("a", 40)
	fixtures := t.TempDir()
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, commit, githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	environment := startIssue929GitHub(t, fixtures)
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil {
		t.Fatalf("catalog refresh: %v\n%s", err, out)
	}
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	approveIssue929(t, transport, environment, project, "activate", "fixture", "--surface", "agents")
	approveIssue929(t, transport, environment, project, "install", "fixture", "--surface", "agents")

	for _, earlier := range []struct {
		path string
		args []string
	}{
		{filepath.Join(issue929Home(t, environment), ".packy", "packs.json"), []string{"status"}},
		{filepath.Join(project, "packy.lock.json"), []string{"status", "--project"}},
	} {
		data, err := os.ReadFile(earlier.path)
		if err != nil {
			t.Fatal(err)
		}
		data = bytes.Replace(data, []byte(`"schema_version": 2`), []byte(`"schema_version": 1`), 1)
		data = bytes.ReplaceAll(data, []byte(`"catalog_repository": "`+issue929OfficialCatalog+`",`), nil)
		if err := os.WriteFile(earlier.path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := runIssue929At(t, transport, environment, project, earlier.args...)
		if !issue929Exit(err, 1) || !strings.Contains(out, "docs/catalog-adoption.md") {
			t.Fatalf("%v over an earlier %s: %v\n%s", earlier.args, filepath.Base(earlier.path), err, out)
		}
		if after, err := os.ReadFile(earlier.path); err != nil || !bytes.Equal(after, data) {
			t.Fatalf("earlier %s changed: %s %v", filepath.Base(earlier.path), after, err)
		}
	}
	if out, err := runIssue929At(t, transport, environment, project, "doctor"); !issue929Exit(err, 1) || !strings.Contains(out, "could not be observed: read capability-pack state") || !strings.Contains(out, "docs/catalog-adoption.md") {
		t.Fatalf("doctor over earlier receipts: %v\n%s", err, out)
	}
	out, err := runIssue929At(t, transport, environment, project, "install", "fixture", "--surface", "agents", "--dry-run")
	if err == nil || !strings.Contains(out, "uninstall the project Packs with the previous Packy") || strings.Contains(out, "remove the invalid project lock") {
		t.Fatalf("install over an earlier lock: %v\n%s", err, out)
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

	for _, args := range [][]string{{"list", "--json"}, {"get", "skill", "fixture:fixture", "--surface", "agents"}, {"export", "fixture", "--surface", "agents", "--output", t.TempDir()}} {
		if out, err := runIssue929(t, transport, environment, args...); !issue929Exit(err, 1) || !strings.Contains(out, "https://github.com/yersonargotev/packy/blob/main/docs/catalog-adoption.md") || strings.Contains(out, "packy init") {
			t.Fatalf("%v over the earlier store layout: %v\n%s", args, err, out)
		}
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

func issue929Exit(err error, code int) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == code
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
					pack.Files[f].SHA256 = catalogFixtureDigest(manifest)
					pack.ManifestSHA256 = catalogFixtureDigest(manifest)
				}
				fmt.Fprintf(closure, "%s\x00%s\x00%s\n", pack.Files[f].Path, pack.Files[f].Mode, pack.Files[f].SHA256)
			}
			pack.ClosureSHA256 = hex.EncodeToString(closure.Sum(nil))
		}
		packs, err := json.Marshal(index.Packs)
		if err != nil {
			t.Fatal(err)
		}
		index.CatalogSHA256 = catalogFixtureDigest(packs)
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
	if err := os.WriteFile(filepath.Join(dist, "SHA256SUMS"), []byte(catalogFixtureDigest(output.Bytes())+"  catalog-snapshot.tar.gz\n"), 0o644); err != nil {
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
	project := t.TempDir()
	if err := githubfake.WriteCatalogProject(project, packs...); err != nil {
		t.Fatal(err)
	}
	return buildIssue929DistFrom(t, transport, project, repository, commit)
}

func buildIssue929DistFrom(t *testing.T, transport, project, repository, commit string) string {
	t.Helper()
	dist := filepath.Join(t.TempDir(), "dist")
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
	return runIssue929At(t, transport, environment, t.TempDir(), args...)
}

func runIssue929At(t *testing.T, transport string, environment []string, directory string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, transport, args...)
	command.Dir = directory
	command.Env = environment
	out, err := command.CombinedOutput()
	return string(out), err
}

// approveIssue929 answers the apply prompt through a terminal, because Packy
// applies a lifecycle plan only after interactive approval.
func approveIssue929(t *testing.T, transport string, environment []string, directory string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, transport, args...)
	command.Dir, command.Env = directory, environment
	terminal, stdin, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer stdin.Close()
	var output bytes.Buffer
	command.Stdin, command.Stdout, command.Stderr = stdin, &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := terminal.Write([]byte("y\n")); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, output.String())
	}
}

// issue929Offline points the transport at an address file that does not
// exist, so every GitHub request fails as it would with the network blocked.
func issue929Offline(t *testing.T, environment []string) []string {
	t.Helper()
	offline := []string{"PACKY_GITHUBFAKE_ADDR_FILE=" + filepath.Join(t.TempDir(), "offline")}
	for _, entry := range environment {
		if !strings.HasPrefix(entry, "PACKY_GITHUBFAKE_ADDR_FILE=") {
			offline = append(offline, entry)
		}
	}
	return offline
}

const issue929Acme = "acme/packs"

// startIssue929Catalogs serves the official catalog with Pack fixture and
// acme/packs with Pack acme, refreshes the official catalog, and registers
// acme/packs.
func startIssue929Catalogs(t *testing.T, transport string) (fixtures string, environment []string) {
	t.Helper()
	fixtures = t.TempDir()
	publishIssue929Portable(t, transport, fixtures, issue929OfficialCatalog, strings.Repeat("a", 40), githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	publishIssue929Portable(t, transport, fixtures, issue929Acme, strings.Repeat("c", 40), githubfake.Pack{ID: "acme", Version: "1.0.0"})
	environment = startIssue929GitHub(t, fixtures)
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil {
		t.Fatalf("catalog refresh: %v\n%s", err, out)
	}
	if out, err := runIssue929(t, transport, environment, "catalog", "add", issue929Acme); err != nil || !strings.Contains(out, "registered acme/packs (attested by .github/workflows/publish.yml) and selected Catalog Snapshot "+strings.Repeat("c", 40)) {
		t.Fatalf("catalog add: %v\n%s", err, out)
	}
	return fixtures, environment
}

func issue929CatalogDirectory(t *testing.T, environment []string, repository string) string {
	t.Helper()
	return filepath.Join(issue929Home(t, environment), ".local", "share", "packy", "catalog", "v2", filepath.FromSlash(repository))
}

func issue929Selected(t *testing.T, environment []string, repository string) string {
	t.Helper()
	var selection struct {
		SnapshotID string `json:"snapshot_id"`
	}
	data, err := os.ReadFile(filepath.Join(issue929CatalogDirectory(t, environment, repository), "selected.json"))
	if err != nil || json.Unmarshal(data, &selection) != nil {
		t.Fatalf("selection of %s: %s %v", repository, data, err)
	}
	return selection.SnapshotID
}

// issue929StoreTree lists every file below the Catalog Store, so a refused
// command can be shown to leave it unchanged.
func issue929StoreTree(t *testing.T, environment []string) string {
	t.Helper()
	root := filepath.Join(issue929Home(t, environment), ".local", "share", "packy", "catalog")
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		files = append(files, relative)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(files, "\n")
}

type issue929CatalogList struct {
	Catalogs []struct {
		Repository string  `json:"repository"`
		Official   bool    `json:"official"`
		Trust      string  `json:"trust"`
		Workflow   string  `json:"workflow"`
		SnapshotID *string `json:"snapshot_id"`
	} `json:"catalogs"`
}

func listIssue929Catalogs(t *testing.T, transport string, environment []string) issue929CatalogList {
	t.Helper()
	out, err := runIssue929(t, transport, environment, "catalog", "list", "--json")
	if err != nil {
		t.Fatalf("catalog list --json: %v\n%s", err, out)
	}
	assertStructuredOutput(t, filepath.Join("..", ".."), "catalog-list.schema.json", out)
	var report issue929CatalogList
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	return report
}

// issue929Receipt returns the version and catalog pair of the receipt for
// packID in ~/.packy/packs.json or packy.lock.json.
func issue929Receipt(t *testing.T, path, packID string) (version, repository, snapshot string) {
	t.Helper()
	var document struct {
		Receipts []struct {
			Pack map[string]string `json:"pack"`
		} `json:"receipts"`
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &document) != nil {
		t.Fatalf("read %s: %s %v", path, data, err)
	}
	for _, receipt := range document.Receipts {
		if receipt.Pack["id"] == packID {
			return receipt.Pack["version"], receipt.Pack["catalog_repository"], receipt.Pack["catalog_snapshot"]
		}
	}
	t.Fatalf("%s has no receipt for %s:\n%s", path, packID, data)
	return "", "", ""
}

func testIssue929RegisterCatalog(t *testing.T, transport string) {
	_, environment := startIssue929Catalogs(t, transport)
	report := listIssue929Catalogs(t, transport, environment)
	if len(report.Catalogs) != 2 {
		t.Fatalf("catalog list = %+v", report)
	}
	for i, want := range []struct {
		repository, snapshot string
		official             bool
	}{{issue929OfficialCatalog, strings.Repeat("a", 40), true}, {issue929Acme, strings.Repeat("c", 40), false}} {
		got := report.Catalogs[i]
		if got.Repository != want.repository || got.Official != want.official || got.Trust != "attested" || got.Workflow != ".github/workflows/publish.yml" || got.SnapshotID == nil || *got.SnapshotID != want.snapshot {
			t.Fatalf("catalog list entry %d = %+v, want %+v", i, got, want)
		}
	}
	out, err := runIssue929(t, transport, environment, "catalog", "list")
	if err != nil || !strings.Contains(out, "acme/packs                   attested  .github/workflows/publish.yml  "+strings.Repeat("c", 40)) {
		t.Fatalf("catalog list: %v\n%s", err, out)
	}
	out, err = runIssue929(t, transport, environment, "list", "--json")
	if err != nil {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	assertStructuredOutput(t, filepath.Join("..", ".."), "pack-list.schema.json", out)
	if !strings.Contains(out, `"id":"acme","version":"1.0.0","description":"Fixture Pack","surfaces":["agents"],"catalog_repository":"acme/packs"`) || !strings.Contains(out, `"id":"fixture","version":"1.0.0","description":"Fixture Pack","surfaces":["agents"],"catalog_repository":"yersonargotev/packy-catalog"`) {
		t.Fatalf("list --json does not name each Pack's catalog:\n%s", out)
	}
	if out, err := runIssue929(t, transport, environment, "list"); err != nil || !strings.Contains(out, "acme/packs") {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(issue929CatalogDirectory(t, environment, issue929Acme), "registration.json")); err != nil {
		t.Fatal(err)
	}
	for _, refused := range []struct {
		args []string
		want string
	}{
		{[]string{"catalog", "add", issue929Acme}, "already registered"},
		{[]string{"catalog", "add", "acme/private", "--trust", "access-controlled"}, "arrives with private catalog support"},
		{[]string{"catalog", "add", issue929OfficialCatalog}, "built in"},
		{[]string{"catalog", "add", "https://example.com/acme"}, "not a GitHub owner/repo"},
	} {
		before := issue929StoreTree(t, environment)
		if out, err := runIssue929(t, transport, environment, refused.args...); !issue929Exit(err, 1) || !strings.Contains(out, refused.want) {
			t.Fatalf("%v: %v\n%s", refused.args, err, out)
		}
		if after := issue929StoreTree(t, environment); after != before {
			t.Fatalf("%v changed the Catalog Store:\n%s\nwant:\n%s", refused.args, after, before)
		}
	}
}

// assertIssue929RefusedRegistration adds a catalog that must be refused and
// checks that neither a registration nor a retained snapshot remains.
func assertIssue929RefusedRegistration(t *testing.T, transport string, environment []string, repository string, want ...string) {
	t.Helper()
	before := issue929StoreTree(t, environment)
	out, err := runIssue929(t, transport, environment, "catalog", "add", repository)
	if !issue929Exit(err, 1) {
		t.Fatalf("catalog add %s: %v\n%s", repository, err, out)
	}
	for _, fragment := range want {
		if !strings.Contains(out, fragment) {
			t.Fatalf("catalog add %s does not say %q:\n%s", repository, fragment, out)
		}
	}
	if after := issue929StoreTree(t, environment); after != before {
		t.Fatalf("refused registration left residue:\n%s\nwant:\n%s", after, before)
	}
	if report := listIssue929Catalogs(t, transport, environment); len(report.Catalogs) != 1 {
		t.Fatalf("catalog list after a refused registration = %+v", report)
	}
}

func startIssue929Official(t *testing.T, transport, fixtures string) []string {
	t.Helper()
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, strings.Repeat("a", 40), githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	environment := startIssue929GitHub(t, fixtures)
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh"); err != nil {
		t.Fatalf("catalog refresh: %v\n%s", err, out)
	}
	return environment
}

func testIssue929InvalidRegistration(t *testing.T, transport string) {
	fixtures := t.TempDir()
	environment := startIssue929Official(t, transport, fixtures)
	commit := strings.Repeat("c", 40)
	if err := githubfake.Publish(fixtures, issue929Acme, commit, buildIssue929Dist(t, transport, "acme/elsewhere", commit, githubfake.Pack{ID: "acme", Version: "1.0.0"})); err != nil {
		t.Fatal(err)
	}
	assertIssue929RefusedRegistration(t, transport, environment, issue929Acme, "register catalog acme/packs", "index names acme/elsewhere")
}

func testIssue929PackIDCollision(t *testing.T, transport string) {
	fixtures := t.TempDir()
	environment := startIssue929Official(t, transport, fixtures)
	publishIssue929Fixture(t, transport, fixtures, "evil/packs", strings.Repeat("e", 40), githubfake.Pack{ID: "fixture", Version: "9.0.0"})
	assertIssue929RefusedRegistration(t, transport, environment, "evil/packs", `introduces Pack "fixture"`, "yersonargotev/packy-catalog already provides")

	publishIssue929Fixture(t, transport, fixtures, issue929Acme, strings.Repeat("c", 40), githubfake.Pack{ID: "acme", Version: "1.0.0"})
	if out, err := runIssue929(t, transport, environment, "catalog", "add", issue929Acme); err != nil {
		t.Fatalf("catalog add: %v\n%s", err, out)
	}
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, strings.Repeat("b", 40), githubfake.Pack{ID: "acme", Version: "2.0.0"}, githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	out, err := runIssue929(t, transport, environment, "catalog", "refresh", issue929OfficialCatalog)
	if !issue929Exit(err, 1) || !strings.Contains(out, `introduces Pack "acme"`) || !strings.Contains(out, "acme/packs already provides") {
		t.Fatalf("official refresh that reuses a registered catalog's Pack ID: %v\n%s", err, out)
	}
	if selected := issue929Selected(t, environment, issue929OfficialCatalog); selected != strings.Repeat("a", 40) {
		t.Fatalf("official selection after a refused refresh = %s", selected)
	}
	if _, err := os.Stat(filepath.Join(issue929CatalogDirectory(t, environment, issue929OfficialCatalog), "snapshots", strings.Repeat("b", 40))); !os.IsNotExist(err) {
		t.Fatalf("refused official snapshot was retained: %v", err)
	}
}

func testIssue929FailedAttestation(t *testing.T, transport string) {
	fixtures := t.TempDir()
	environment := startIssue929Official(t, transport, fixtures)
	publishIssue929Fixture(t, transport, fixtures, issue929Acme, strings.Repeat("c", 40), githubfake.Pack{ID: "acme", Version: "1.0.0"})
	if err := githubfake.WriteRepository(fixtures, issue929Acme, githubfake.Repository{AttestationWorkflow: "acme/packs/.github/workflows/other.yml"}); err != nil {
		t.Fatal(err)
	}
	assertIssue929RefusedRegistration(t, transport, environment, issue929Acme, "register catalog acme/packs", "attestation")
}

func testIssue929RefreshAllAndOne(t *testing.T, transport string) {
	fixtures, environment := startIssue929Catalogs(t, transport)
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, strings.Repeat("b", 40), githubfake.Pack{ID: "fixture", Version: "2.0.0"})
	publishIssue929Fixture(t, transport, fixtures, issue929Acme, strings.Repeat("d", 40), githubfake.Pack{ID: "acme", Version: "2.0.0"})

	out, err := runIssue929(t, transport, environment, "catalog", "refresh", issue929Acme)
	if err != nil || out != "packy catalog refresh: selected Catalog Snapshot "+strings.Repeat("d", 40)+" of acme/packs\n" {
		t.Fatalf("catalog refresh acme/packs: %v\n%s", err, out)
	}
	if official := issue929Selected(t, environment, issue929OfficialCatalog); official != strings.Repeat("a", 40) {
		t.Fatalf("refreshing acme/packs changed the official selection to %s", official)
	}
	if packs := listIssue929Packs(t, transport, environment); packs != "acme@2.0.0,fixture@1.0.0" {
		t.Fatalf("list after refreshing acme/packs = %s", packs)
	}
	out, err = runIssue929(t, transport, environment, "catalog", "refresh", "--json")
	if err != nil {
		t.Fatalf("catalog refresh --json: %v\n%s", err, out)
	}
	assertStructuredOutput(t, filepath.Join("..", ".."), "catalog-refresh.schema.json", out)
	if !strings.Contains(out, `{"repository":"yersonargotev/packy-catalog","result":"selected","snapshot_id":"`+strings.Repeat("b", 40)+`"},{"repository":"acme/packs","result":"selected","snapshot_id":"`+strings.Repeat("d", 40)+`"}`) {
		t.Fatalf("catalog refresh --json:\n%s", out)
	}
	if packs := listIssue929Packs(t, transport, environment); packs != "acme@2.0.0,fixture@2.0.0" {
		t.Fatalf("list after refreshing every catalog = %s", packs)
	}
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh", "nobody/packs"); !issue929Exit(err, 1) || !strings.Contains(out, "catalog nobody/packs is not registered") {
		t.Fatalf("catalog refresh of an unregistered catalog: %v\n%s", err, out)
	}
}

func testIssue929PartialRefresh(t *testing.T, transport string) {
	fixtures, environment := startIssue929Catalogs(t, transport)
	publishIssue929Fixture(t, transport, fixtures, issue929OfficialCatalog, strings.Repeat("b", 40), githubfake.Pack{ID: "fixture", Version: "2.0.0"})
	if err := githubfake.WriteRepository(fixtures, issue929Acme, githubfake.Repository{ReleaseStatus: 500}); err != nil {
		t.Fatal(err)
	}
	out, err := runIssue929(t, transport, environment, "catalog", "refresh")
	if !issue929Exit(err, 1) || !strings.Contains(out, "selected Catalog Snapshot "+strings.Repeat("b", 40)+" of yersonargotev/packy-catalog") || !strings.Contains(out, "acme/packs failed and keeps Catalog Snapshot "+strings.Repeat("c", 40)) || !strings.Contains(out, "1 of 2 catalogs failed to refresh") {
		t.Fatalf("catalog refresh with a failing catalog: %v\n%s", err, out)
	}
	if acme := issue929Selected(t, environment, issue929Acme); acme != strings.Repeat("c", 40) {
		t.Fatalf("failed refresh changed the acme/packs selection to %s", acme)
	}
	if packs := listIssue929Packs(t, transport, environment); packs != "acme@1.0.0,fixture@2.0.0" {
		t.Fatalf("list after a partial refresh = %s", packs)
	}
}

func testIssue929IncompatibleOtherSnapshot(t *testing.T, transport string) {
	fixtures, environment := startIssue929Catalogs(t, transport)
	incompatible := strings.Repeat("d", 40)
	dist := buildIssue929Dist(t, transport, issue929Acme, incompatible, githubfake.Pack{ID: "acme", Version: "2.0.0"}, githubfake.Pack{ID: "acme-extra", Version: "1.0.0"})
	rewriteIssue929Manifest(t, dist, "acme-extra", func(manifest string) string {
		return strings.Replace(manifest, `"description": "Fixture Pack",`, `"description": "Fixture Pack", "future_engine_field": true,`, 1)
	})
	if err := githubfake.Publish(fixtures, issue929Acme, incompatible, dist); err != nil {
		t.Fatal(err)
	}
	out, err := runIssue929(t, transport, environment, "catalog", "refresh", issue929Acme)
	if !issue929Exit(err, 1) || !strings.Contains(out, `Pack "acme-extra" requires a newer Packy engine`) {
		t.Fatalf("catalog refresh of an incompatible acme/packs snapshot: %v\n%s", err, out)
	}
	if acme := issue929Selected(t, environment, issue929Acme); acme != strings.Repeat("c", 40) {
		t.Fatalf("incompatible refresh changed the acme/packs selection to %s", acme)
	}
	if _, err := os.Stat(filepath.Join(issue929CatalogDirectory(t, environment, issue929Acme), "snapshots", incompatible)); !os.IsNotExist(err) {
		t.Fatalf("incompatible snapshot was retained: %v", err)
	}
	if packs := listIssue929Packs(t, transport, environment); packs != "acme@1.0.0,fixture@1.0.0" {
		t.Fatalf("list after a refused acme/packs refresh = %s", packs)
	}
}

func testIssue929OtherCatalogLifecycle(t *testing.T, transport string) {
	t.Run("own catalog", func(t *testing.T) { testIssue929OwnCatalogLifecycle(t, transport) })
	t.Run("Pack ID moves to another registered catalog", func(t *testing.T) { testIssue929PackMovesBetweenCatalogs(t, transport) })
}

func testIssue929PackMovesBetweenCatalogs(t *testing.T, transport string) {
	fixtures, environment := startIssue929Catalogs(t, transport)
	project, outside := t.TempDir(), t.TempDir()
	writeTestGitWorktree(t, project)
	receipts, lock := filepath.Join(issue929Home(t, environment), ".packy", "packs.json"), filepath.Join(project, "packy.lock.json")
	approveIssue929(t, transport, environment, outside, "activate", "acme", "--surface", "agents")
	approveIssue929(t, transport, environment, project, "install", "acme", "--surface", "agents")

	publishIssue929Portable(t, transport, fixtures, issue929Acme, strings.Repeat("d", 40), githubfake.Pack{ID: "acme-next", Version: "1.0.0"})
	publishIssue929Portable(t, transport, fixtures, issue929OfficialCatalog, strings.Repeat("b", 40), githubfake.Pack{ID: "acme", Version: "5.0.0"}, githubfake.Pack{ID: "fixture", Version: "1.0.0"})
	for _, repository := range []string{issue929Acme, issue929OfficialCatalog} {
		if out, err := runIssue929(t, transport, environment, "catalog", "refresh", repository); err != nil {
			t.Fatalf("catalog refresh %s: %v\n%s", repository, err, out)
		}
	}
	out, err := runIssue929At(t, transport, environment, outside, "update", "acme", "--surface", "agents")
	if !issue929Exit(err, 1) || !strings.Contains(out, "installed from catalog acme/packs, and updates look only in that catalog") || !strings.Contains(out, "packy deactivate acme --surface agents") {
		t.Fatalf("update of a Pack ID that moved to another catalog: %v\n%s", err, out)
	}
	if out, err := runIssue929At(t, transport, environment, outside, "status"); err != nil || !regexp.MustCompile(`acme\s+acme/packs\s+agents\s+active at version 1\.0\.0.*none`).MatchString(out) {
		t.Fatalf("status follows the Pack ID into another catalog: %v\n%s", err, out)
	}
	if out, err := runIssue929At(t, transport, environment, project, "install", "acme", "--surface", "agents", "--dry-run"); !issue929Exit(err, 1) || !strings.Contains(out, "catalog_conflict") {
		t.Fatalf("install over a lock receipt from another catalog: %v\n%s", err, out)
	}

	approveIssue929(t, transport, environment, outside, "deactivate", "acme", "--surface", "agents")
	approveIssue929(t, transport, environment, outside, "activate", "acme", "--surface", "agents")
	approveIssue929(t, transport, environment, project, "uninstall", "acme", "--surface", "agents")
	approveIssue929(t, transport, environment, project, "install", "acme", "--surface", "agents")
	for _, path := range []string{receipts, lock} {
		if version, repository, snapshot := issue929Receipt(t, path, "acme"); version != "5.0.0" || repository != issue929OfficialCatalog || snapshot != strings.Repeat("b", 40) {
			t.Fatalf("%s receipt after the move = %s %s@%s", path, version, repository, snapshot)
		}
	}
}

func testIssue929OwnCatalogLifecycle(t *testing.T, transport string) {
	fixtures, environment := startIssue929Catalogs(t, transport)
	project, outside := t.TempDir(), t.TempDir()
	writeTestGitWorktree(t, project)
	receipts, lock := filepath.Join(issue929Home(t, environment), ".packy", "packs.json"), filepath.Join(project, "packy.lock.json")
	approveIssue929(t, transport, environment, outside, "activate", "acme", "--surface", "agents")
	approveIssue929(t, transport, environment, project, "install", "acme", "--surface", "agents")
	first := strings.Repeat("c", 40)
	for _, path := range []string{receipts, lock} {
		if version, repository, snapshot := issue929Receipt(t, path, "acme"); version != "1.0.0" || repository != issue929Acme || snapshot != first {
			t.Fatalf("%s receipt = %s %s@%s", path, version, repository, snapshot)
		}
	}
	if out, err := runIssue929At(t, transport, environment, outside, "status"); err != nil || !regexp.MustCompile(`acme\s+acme/packs\s+agents\s+active`).MatchString(out) {
		t.Fatalf("status does not name the Pack's catalog: %v\n%s", err, out)
	}

	second := strings.Repeat("d", 40)
	publishIssue929Portable(t, transport, fixtures, issue929Acme, second, githubfake.Pack{ID: "acme", Version: "2.0.0"})
	if out, err := runIssue929(t, transport, environment, "catalog", "refresh", issue929Acme); err != nil {
		t.Fatalf("catalog refresh acme/packs: %v\n%s", err, out)
	}
	approveIssue929(t, transport, environment, outside, "update", "acme", "--surface", "agents")
	approveIssue929(t, transport, environment, project, "update", "acme", "--surface", "agents", "--project")
	for _, path := range []string{receipts, lock} {
		if version, repository, snapshot := issue929Receipt(t, path, "acme"); version != "2.0.0" || repository != issue929Acme || snapshot != second {
			t.Fatalf("%s receipt after update = %s %s@%s", path, version, repository, snapshot)
		}
	}
	out, err := runIssue929(t, transport, environment, "get", "skill", "acme:acme", "--surface", "agents", "--snapshot", first, "--json")
	if err != nil || !strings.Contains(out, filepath.Join("skill-access", "v3", "acme", "packs", first)) {
		t.Fatalf("get skill from the retained acme/packs snapshot: %v\n%s", err, out)
	}
	exported := filepath.Join(t.TempDir(), "plugin")
	if out, err := runIssue929(t, transport, environment, "export", "acme", "--surface", "agents", "--snapshot", first, "--output", exported, "--json"); err != nil || !strings.Contains(out, `"snapshot_id":"`+first+`"`) {
		t.Fatalf("export from the retained acme/packs snapshot: %v\n%s", err, out)
	}

	approveIssue929(t, transport, environment, outside, "deactivate", "acme", "--surface", "agents")
	approveIssue929(t, transport, environment, project, "uninstall", "acme", "--surface", "agents")
	if out, err := runIssue929(t, transport, environment, "catalog", "remove", issue929Acme); err != nil {
		t.Fatalf("catalog remove: %v\n%s", err, out)
	}
	publishIssue929Fixture(t, transport, fixtures, "other/packs", strings.Repeat("f", 40), githubfake.Pack{ID: "acme", Version: "3.0.0"})
	if out, err := runIssue929(t, transport, environment, "catalog", "add", "other/packs"); err != nil {
		t.Fatalf("catalog add other/packs: %v\n%s", err, out)
	}
	approveIssue929(t, transport, environment, outside, "activate", "acme", "--surface", "agents")
	approveIssue929(t, transport, environment, project, "install", "acme", "--surface", "agents")
	for _, path := range []string{receipts, lock} {
		if version, repository, snapshot := issue929Receipt(t, path, "acme"); version != "3.0.0" || repository != "other/packs" || snapshot != strings.Repeat("f", 40) {
			t.Fatalf("%s receipt after the move = %s %s@%s", path, version, repository, snapshot)
		}
	}
}

func testIssue929RemoveCatalog(t *testing.T, transport string) {
	_, environment := startIssue929Catalogs(t, transport)
	project := t.TempDir()
	approveIssue929(t, transport, environment, project, "activate", "acme", "--surface", "agents")
	directory := issue929CatalogDirectory(t, environment, issue929Acme)
	before := issue929StoreTree(t, environment)
	if out, err := runIssue929(t, transport, environment, "catalog", "remove", issue929Acme); !issue929Exit(err, 1) || !strings.Contains(out, "catalog acme/packs is in use by installed Packs acme on agents") {
		t.Fatalf("catalog remove with an installed Pack: %v\n%s", err, out)
	}
	if after := issue929StoreTree(t, environment); after != before {
		t.Fatalf("refused removal changed the Catalog Store:\n%s\nwant:\n%s", after, before)
	}
	if out, err := runIssue929(t, transport, environment, "catalog", "remove", issue929OfficialCatalog); !issue929Exit(err, 1) || !strings.Contains(out, "cannot be removed") {
		t.Fatalf("catalog remove of the official catalog: %v\n%s", err, out)
	}
	approveIssue929(t, transport, environment, project, "deactivate", "acme", "--surface", "agents")
	if out, err := runIssue929(t, transport, environment, "catalog", "remove", issue929Acme); err != nil || !strings.Contains(out, "removed catalog acme/packs") {
		t.Fatalf("catalog remove: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(directory, "snapshots", strings.Repeat("c", 40), "catalog-index.json")); err != nil {
		t.Fatalf("removal deleted the retained snapshot: %v", err)
	}
	for _, name := range []string{"registration.json", "selected.json"} {
		if _, err := os.Stat(filepath.Join(directory, name)); !os.IsNotExist(err) {
			t.Fatalf("removal kept %s: %v", name, err)
		}
	}
	if report := listIssue929Catalogs(t, transport, environment); len(report.Catalogs) != 1 || report.Catalogs[0].Repository != issue929OfficialCatalog {
		t.Fatalf("catalog list after removal = %+v", report)
	}
	if packs := listIssue929Packs(t, transport, environment); packs != "fixture@1.0.0" {
		t.Fatalf("list after removal = %s", packs)
	}
}

func testIssue929UnregisteredLock(t *testing.T, transport string) {
	fixtures, environment := startIssue929Catalogs(t, transport)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	approveIssue929(t, transport, environment, project, "install", "acme", "--surface", "agents")
	lock, err := os.ReadFile(filepath.Join(project, "packy.lock.json"))
	if err != nil {
		t.Fatal(err)
	}

	elsewhere := testprocess.Env(t, issue929AddressEntry(t, environment))
	if out, err := runIssue929(t, transport, elsewhere, "catalog", "refresh"); err != nil {
		t.Fatalf("catalog refresh: %v\n%s", err, out)
	}
	if out, err := runIssue929At(t, transport, elsewhere, project, "install", "acme", "--surface", "agents", "--dry-run"); !issue929Exit(err, 1) || !strings.Contains(out, "catalog acme/packs, which is not registered on this workstation") || !strings.Contains(out, "packy catalog add acme/packs") {
		t.Fatalf("install of a Pack that only an unregistered lock catalog provides: %v\n%s", err, out)
	}
	out, err := runIssue929At(t, transport, elsewhere, project, "status", "--project")
	if !issue929Exit(err, 1) || !strings.Contains(out, "catalog acme/packs, which is not registered on this workstation") || !strings.Contains(out, "packy catalog add acme/packs") {
		t.Fatalf("status --project over a lock from an unregistered catalog: %v\n%s", err, out)
	}
	publishIssue929Fixture(t, transport, fixtures, "other/packs", strings.Repeat("f", 40), githubfake.Pack{ID: "acme", Version: "3.0.0"})
	if out, err := runIssue929(t, transport, elsewhere, "catalog", "add", "other/packs"); err != nil {
		t.Fatalf("catalog add other/packs: %v\n%s", err, out)
	}
	out, err = runIssue929At(t, transport, elsewhere, project, "install", "acme", "--surface", "agents", "--dry-run")
	if !issue929Exit(err, 1) || !strings.Contains(out, "unregistered_catalog") || !strings.Contains(out, "packy catalog add acme/packs") {
		t.Fatalf("install over a lock from an unregistered catalog: %v\n%s", err, out)
	}
	if after, err := os.ReadFile(filepath.Join(project, "packy.lock.json")); err != nil || !bytes.Equal(after, lock) {
		t.Fatalf("project lock changed: %v", err)
	}
}

func issue929AddressEntry(t *testing.T, environment []string) string {
	t.Helper()
	for _, entry := range environment {
		if strings.HasPrefix(entry, "PACKY_GITHUBFAKE_ADDR_FILE=") {
			return entry
		}
	}
	t.Fatal("test process environment has no fake address")
	return ""
}

func testIssue929OtherCatalogOffline(t *testing.T, transport string) {
	_, online := startIssue929Catalogs(t, transport)
	offline := issue929Offline(t, online)
	project := t.TempDir()
	writeTestGitWorktree(t, project)
	approveIssue929(t, transport, offline, project, "activate", "acme", "--surface", "agents")
	approveIssue929(t, transport, offline, project, "install", "acme", "--surface", "agents")
	exported := filepath.Join(t.TempDir(), "plugin")
	if err := os.Mkdir(exported, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"list", "--json"}, {"show", "acme"}, {"status"}, {"status", "--project"}, {"catalog", "list"}, {"export", "acme", "--surface", "agents", "--output", exported}} {
		if out, err := runIssue929At(t, transport, offline, project, args...); err != nil {
			t.Fatalf("offline %v: %v\n%s", args, err, out)
		}
	}
	out, err := runIssue929At(t, transport, offline, project, "get", "skill", "acme:acme", "--surface", "agents", "--json")
	if err != nil || !strings.Contains(out, filepath.Join("skill-access", "v3", "acme", "packs", strings.Repeat("c", 40))) {
		t.Fatalf("offline get skill: %v\n%s", err, out)
	}
	var provenance map[string]any
	data, err := os.ReadFile(filepath.Join(exported, "packy-provenance.json"))
	if err != nil || json.Unmarshal(data, &provenance) != nil || provenance["catalog_repository"] != issue929Acme || provenance["snapshot_id"] != strings.Repeat("c", 40) {
		t.Fatalf("plugin provenance = %s %v", data, err)
	}
}
