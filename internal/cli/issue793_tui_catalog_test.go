package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestIssue793TUIRefreshUsesCatalogStoreAndPreservesActivePack(t *testing.T) {
	pack := testsupport.PortableAllSurfaces("catalog-tui-refresh")
	initialCommit := strings.Repeat("f", 40)
	updatedCommit := strings.Repeat("1", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, initialCommit, pack)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, pack)
	opts := fixture.options
	env := MapEnv{}
	for key, value := range opts.Env.(MapEnv) {
		env[key] = value
	}
	opts.skillSourceRoot = ""
	opts.Env = env
	opts.CatalogSource = source
	backend := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults()))
	if err := backend.Initialize(context.Background(), func(string) {}); err != nil {
		t.Fatal(err)
	}
	preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "activate", PackID: pack.ID(), Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: preview, ApprovedPhases: requiredTUIPhases(preview)}, func(tui.ApplyProgress) {}); err != nil {
		t.Fatal(err)
	}
	beforeReceipts, err := os.ReadFile(filepath.Join(env["HOME"], ".packy", "packs.json"))
	if err != nil {
		t.Fatal(err)
	}
	beforeCodex := snapshotTree(t, filepath.Join(env["HOME"], ".codex"))

	updated := pack.Candidate().WithExactCopyBytes("instruction:guidance", ".", []byte("# TUI refreshed catalog guidance\n"))
	newPack := testsupport.PortableAllSurfaces("catalog-tui-new")
	source.release = catalogReleaseFixture(t, updatedCommit, updated, newPack)
	var progress []string
	if err := backend.RefreshCatalog(context.Background(), func(message string) { progress = append(progress, message) }); err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || !strings.Contains(progress[0], updatedCommit) {
		t.Fatalf("RefreshCatalog() progress = %v", progress)
	}
	afterReceipts, err := os.ReadFile(filepath.Join(env["HOME"], ".packy", "packs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterReceipts) != string(beforeReceipts) || snapshotTree(t, filepath.Join(env["HOME"], ".codex")) != beforeCodex {
		t.Fatal("TUI Catalog Refresh changed active Pack state or projections")
	}
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	current := findTUIPack(dashboard.Global.Packs, pack.ID())
	added := findTUIPack(dashboard.Global.Packs, newPack.ID())
	if dashboard.Catalog.SnapshotID != updatedCommit || dashboard.Catalog.Packs != 2 || current == nil || current.Version != updated.CurrentVersion() || added == nil {
		t.Fatalf("refreshed TUI availability = catalog %#v, packs %#v", dashboard.Catalog, dashboard.Global.Packs)
	}
	statusIndex := slices.IndexFunc(current.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "codex" })
	if statusIndex < 0 {
		t.Fatalf("refreshed Pack omitted codex status: %#v", current.SurfaceStatuses)
	}
	status := current.SurfaceStatuses[statusIndex]
	if !status.Active || !status.UpdateAvailable || status.InstalledVersion != pack.CurrentVersion() {
		t.Fatalf("refresh did not preserve active version with selected update available: %#v", status)
	}

	source.err = errors.New("Catalog Snapshot requires a newer Packy engine; update Packy and retry")
	if err := backend.RefreshCatalog(context.Background(), func(string) {}); err == nil || !strings.Contains(err.Error(), "update Packy and retry") {
		t.Fatalf("incompatible refresh error = %v", err)
	}
	dashboard, err = backend.Load(context.Background())
	if err != nil || dashboard.Catalog.SnapshotID != updatedCommit || findTUIPack(dashboard.Global.Packs, newPack.ID()) == nil {
		t.Fatalf("failed refresh replaced usable catalog: catalog=%#v err=%v", dashboard.Catalog, err)
	}
}
