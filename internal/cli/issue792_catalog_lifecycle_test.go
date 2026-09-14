package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/managedpack"
)

type catalogSourceFixture struct {
	release catalogstore.Release
	calls   int
	err     error
}

func (s *catalogSourceFixture) Latest(context.Context, string) (catalogstore.Release, error) {
	s.calls++
	return s.release, s.err
}

type catalogOriginFixture map[string]string

func (r catalogOriginFixture) Resolve(_ context.Context, origin managedpack.Origin) (string, error) {
	return r[origin.ID], nil
}

func TestIssue792CatalogRefreshKeepsActivationsOnRetainedSnapshotsUntilSelectedUpdate(t *testing.T) {
	first := testsupport.PortableAllSurfaces("catalog-first")
	second := testsupport.PortableAllSurfaces("catalog-second")
	initialCommit := strings.Repeat("a", 40)
	updatedCommit := strings.Repeat("b", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, initialCommit, first, second)}
	terminal := &fakeTerminal{interactive: true, approve: true}
	fixture := newSyntheticCLIFixture(t, terminal, first, second)
	opts := fixture.options
	env := MapEnv{}
	for key, value := range opts.Env.(MapEnv) {
		env[key] = value
	}
	delete(env, "PACKY_SKILLS_SOURCE")
	opts.Env = env
	opts.CatalogSource = source

	if out, err := executeCommand(t, NewRootCommand(opts), "init"); err != nil || !strings.Contains(out, initialCommit) {
		t.Fatalf("init Catalog Snapshot: %v\n%s", err, out)
	}
	activations := []struct {
		pack    testsupport.Fixture
		surface string
	}{{first, "codex"}, {second, "claude"}}
	for _, activation := range activations {
		if out, err := executeCommand(t, NewRootCommand(opts), "activate", activation.pack.ID(), "--surface", activation.surface); err != nil {
			t.Fatalf("activate %s: %v\n%s", activation.pack.ID(), err, out)
		}
	}
	beforeReceipts, err := os.ReadFile(filepath.Join(env["HOME"], ".packy", "packs.json"))
	if err != nil {
		t.Fatal(err)
	}
	beforeCodex := snapshotTree(t, filepath.Join(env["HOME"], ".codex"))
	beforeClaude := snapshotTree(t, filepath.Join(env["HOME"], ".claude"))

	updatedFirst := first.Candidate().WithExactCopyBytes("instruction:guidance", ".", []byte("# Updated catalog guidance\n"))
	newPack := testsupport.PortableAllSurfaces("catalog-new")
	source.release = catalogReleaseFixture(t, updatedCommit, updatedFirst, second, newPack)
	if out, err := executeCommand(t, NewRootCommand(opts), "catalog", "refresh"); err != nil || !strings.Contains(out, updatedCommit) {
		t.Fatalf("catalog refresh: %v\n%s", err, out)
	}
	afterReceipts, err := os.ReadFile(filepath.Join(env["HOME"], ".packy", "packs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterReceipts) != string(beforeReceipts) || snapshotTree(t, filepath.Join(env["HOME"], ".codex")) != beforeCodex || snapshotTree(t, filepath.Join(env["HOME"], ".claude")) != beforeClaude {
		t.Fatal("Catalog Refresh changed activation state or surface projections")
	}

	callsAfterRefresh := source.calls
	for _, args := range [][]string{{"list"}, {"status", first.ID(), "--surface", "codex"}, {"status", second.ID(), "--surface", "claude"}, {"show", newPack.ID()}} {
		if out, err := executeCommand(t, NewRootCommand(opts), args...); err != nil {
			t.Fatalf("offline %v: %v\n%s", args, err, out)
		}
	}
	if source.calls != callsAfterRefresh {
		t.Fatalf("downloaded-content commands discovered catalog updates online: calls %d -> %d", callsAfterRefresh, source.calls)
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "update", first.ID(), "--surface", "codex"); err != nil {
		t.Fatalf("selected update: %v\n%s", err, out)
	}
	receipts, err := os.ReadFile(filepath.Join(env["HOME"], ".packy", "packs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Receipts []struct {
			Pack struct {
				ID              string `json:"id"`
				Version         string `json:"version"`
				CatalogSnapshot string `json:"catalog_snapshot"`
			} `json:"pack"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(receipts, &state); err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		first.ID():  {updatedFirst.CurrentVersion(), updatedCommit},
		second.ID(): {second.CurrentVersion(), initialCommit},
	}
	for _, receipt := range state.Receipts {
		identity, ok := want[receipt.Pack.ID]
		if !ok {
			continue
		}
		if receipt.Pack.Version != identity[0] || receipt.Pack.CatalogSnapshot != identity[1] {
			t.Fatalf("Pack %s receipt = %s at %s, want %s at %s", receipt.Pack.ID, receipt.Pack.Version, receipt.Pack.CatalogSnapshot, identity[0], identity[1])
		}
		delete(want, receipt.Pack.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing Pack receipts %v:\n%s", want, receipts)
	}
	if _, err := os.Stat(filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "catalog", "snapshots", initialCommit)); err != nil {
		t.Fatalf("previous Catalog Snapshot was garbage-collected: %v", err)
	}
}

func TestIssue792TUIInitializationAcquiresSnapshotThenLoadsOffline(t *testing.T) {
	pack := testsupport.PortableAllSurfaces("catalog-tui")
	commit := strings.Repeat("d", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, commit, pack)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, pack)
	opts := fixture.options
	env := MapEnv{}
	for key, value := range opts.Env.(MapEnv) {
		env[key] = value
	}
	delete(env, "PACKY_SKILLS_SOURCE")
	opts.Env = env
	opts.CatalogSource = source
	backend := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults()))
	var progress []string
	if err := backend.Initialize(context.Background(), func(message string) { progress = append(progress, message) }); err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || !strings.Contains(progress[0], commit) {
		t.Fatalf("Initialize() progress = %v", progress)
	}
	callsAfterInitialization := source.calls
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if findTUIPack(dashboard.Global.Packs, pack.ID()) == nil {
		t.Fatalf("TUI catalog omitted initialized Pack: %#v", dashboard.Global.Packs)
	}
	if source.calls != callsAfterInitialization {
		t.Fatalf("TUI load discovered catalog updates online: calls %d -> %d", callsAfterInitialization, source.calls)
	}
}

func catalogReleaseFixture(t *testing.T, commit string, fixtures ...testsupport.Fixture) catalogstore.Release {
	t.Helper()
	projectRoot := t.TempDir()
	bundleRoot := filepath.Join(projectRoot, "bundle")
	origins := catalogOriginFixture{}
	for _, fixture := range fixtures {
		if err := fixture.WriteBundle(bundleRoot); err != nil {
			t.Fatal(err)
		}
		resolved, err := fixture.WriteProject(t.TempDir(), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		for id, root := range resolved {
			origins[id] = root
		}
	}
	validation, err := managedpack.ValidateCatalogProject(context.Background(), projectRoot, "", origins)
	if err != nil {
		t.Fatal(err)
	}
	result, err := managedpack.BuildCatalogSnapshot(context.Background(), projectRoot, validation, managedpack.CatalogSnapshotSource{
		Repository: "yersonargotev/packy-catalog", Commit: commit, Builder: "yersonargotev/packy@" + strings.Repeat("c", 40),
	}, filepath.Join(t.TempDir(), "snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(result.ArchivePath)
	if err != nil {
		t.Fatal(err)
	}
	checksum, err := os.ReadFile(result.ChecksumPath)
	if err != nil {
		t.Fatal(err)
	}
	return catalogstore.Release{
		Repository: "yersonargotev/packy-catalog", Tag: "catalog-" + commit, Commit: commit,
		Publisher: "github-actions[bot]", Published: true, Immutable: true, AttestationVerified: true,
		Assets: []catalogstore.Asset{
			{Name: "catalog-snapshot.tar.gz", SHA256: catalogFixtureDigest(archive), Data: archive},
			{Name: "SHA256SUMS", SHA256: catalogFixtureDigest(checksum), Data: checksum},
		},
	}
}

func catalogFixtureDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
