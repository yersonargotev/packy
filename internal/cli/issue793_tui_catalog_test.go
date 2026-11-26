package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestIssue793TUIRefreshUsesCatalogStoreAndPreservesActivePack(t *testing.T) {
	pack := testsupport.SkillVariants("catalog-tui-refresh")
	initialCommit := strings.Repeat("f", 40)
	updatedCommit := strings.Repeat("1", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, initialCommit, pack)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{}, pack)
	opts := fixture.options
	env := MapEnv{}
	for key, value := range opts.Env.(MapEnv) {
		env[key] = value
	}
	opts.catalogRootOverride = ""
	opts.Env = env
	opts.CatalogSource = source
	backend := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults()))
	if err := backend.Initialize(context.Background(), func(string) {}); err != nil {
		t.Fatal(err)
	}
	preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "activate", PackID: pack.ID(), Surface: "agents", Scope: "global", Selection: tui.Selection{Mode: "all"}})
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
	beforeAgents := snapshotTree(t, filepath.Join(env["HOME"], ".agents"))

	updated := pack.Candidate().WithExactCopyBytes("skill:guide", "SKILL.md", []byte("---\nname: guide\ndescription: Updated guide.\n---\n\nTUI refreshed catalog guidance\n"))
	newPack := testsupport.SkillVariants("catalog-tui-new")
	source.release = catalogReleaseFixture(t, updatedCommit, updated, newPack)
	refreshes, err := backend.RefreshCatalog(context.Background(), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if want := []tui.CatalogRefresh{{Repository: "yersonargotev/packy-catalog", SnapshotID: updatedCommit}}; !reflect.DeepEqual(refreshes, want) {
		t.Fatalf("RefreshCatalog() = %#v, want %#v", refreshes, want)
	}
	afterReceipts, err := os.ReadFile(filepath.Join(env["HOME"], ".packy", "packs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterReceipts) != string(beforeReceipts) || snapshotTree(t, filepath.Join(env["HOME"], ".agents")) != beforeAgents {
		t.Fatal("TUI Catalog Refresh changed active Pack state or projections")
	}
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	current := findTUIPack(dashboard.Global.Packs, pack.ID())
	added := findTUIPack(dashboard.Global.Packs, newPack.ID())
	if len(dashboard.Catalogs) != 1 || dashboard.Catalogs[0].SnapshotID != updatedCommit || dashboard.Catalogs[0].Packs != 2 || current == nil || current.Version != updated.CurrentVersion() || added == nil {
		t.Fatalf("refreshed TUI availability = catalogs %#v, packs %#v", dashboard.Catalogs, dashboard.Global.Packs)
	}
	statusIndex := slices.IndexFunc(current.SurfaceStatuses, func(status tui.SurfaceStatus) bool { return status.Name == "agents" })
	if statusIndex < 0 {
		t.Fatalf("refreshed Pack omitted agents status: %#v", current.SurfaceStatuses)
	}
	status := current.SurfaceStatuses[statusIndex]
	if !status.Active || !status.UpdateAvailable || status.InstalledVersion != pack.CurrentVersion() {
		t.Fatalf("refresh did not preserve active version with selected update available: %#v", status)
	}

	source.err = errors.New("Catalog Snapshot requires a newer Packy engine; update Packy and retry")
	refreshes, err = backend.RefreshCatalog(context.Background(), func(string) {})
	if err != nil || len(refreshes) != 1 || refreshes[0].SnapshotID != updatedCommit || refreshes[0].Err == nil || !strings.Contains(refreshes[0].Err.Error(), "update Packy and retry") {
		t.Fatalf("incompatible refresh = %#v, %v", refreshes, err)
	}
	dashboard, err = backend.Load(context.Background())
	if err != nil || len(dashboard.Catalogs) != 1 || dashboard.Catalogs[0].SnapshotID != updatedCommit || findTUIPack(dashboard.Global.Packs, newPack.ID()) == nil {
		t.Fatalf("failed refresh replaced usable catalog: catalogs=%#v err=%v", dashboard.Catalogs, err)
	}
}
