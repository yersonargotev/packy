package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/catalogstore"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestIssue797WithdrawnPackRemainsInspectableAndDeactivatable(t *testing.T) {
	withdrawn := testsupport.PortableAllSurfaces("withdrawn-runtime")
	blockedActivation := testsupport.PortableAllSurfaces("withdrawn-inactive-surface")
	remaining := testsupport.PortableAllSurfaces("catalog-current")
	retainedSnapshot := strings.Repeat("7", 40)
	currentSnapshot := strings.Repeat("8", 40)
	source := &catalogSourceFixture{release: catalogReleaseFixture(t, retainedSnapshot, withdrawn, blockedActivation, remaining)}
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, withdrawn, blockedActivation, remaining)
	opts := fixture.options
	env := MapEnv{}
	for key, value := range opts.Env.(MapEnv) {
		env[key] = value
	}
	delete(env, "PACKY_SKILLS_SOURCE")
	opts.Env = env
	opts.CatalogSource = source

	if out, err := executeCommand(t, NewRootCommand(opts), "init"); err != nil {
		t.Fatalf("initialize retained snapshot: %v\n%s", err, out)
	}
	for _, surface := range []string{"claude", "codex", "opencode"} {
		if out, err := executeCommand(t, NewRootCommand(opts), "activate", withdrawn.ID(), "--surface", surface); err != nil {
			t.Fatalf("activate Pack on %s before withdrawal: %v\n%s", surface, err, out)
		}
	}
	source.release = catalogReleaseFixture(t, currentSnapshot, remaining)
	if out, err := executeCommand(t, NewRootCommand(opts), "catalog", "refresh"); err != nil {
		t.Fatalf("select snapshot that withdraws Pack: %v\n%s", err, out)
	}

	beforeRejectedActivation := snapshotTree(t, fixture.home)
	if out, err := executeCommand(t, NewRootCommand(opts), "activate", blockedActivation.ID(), "--surface", "opencode", "--dry-run"); err == nil || !strings.Contains(err.Error(), "current catalog") {
		t.Fatalf("new activation from withdrawn Pack = %v\n%s", err, out)
	}
	if after := snapshotTree(t, fixture.home); after != beforeRejectedActivation {
		t.Fatal("rejected activation mutated Packy Home or host projections")
	}

	for _, surface := range []string{"claude", "codex", "opencode"} {
		statusOutput, err := executeCommand(t, NewRootCommand(opts), "status", withdrawn.ID(), "--surface", surface, "--json")
		if err != nil {
			t.Fatalf("inspect withdrawn Pack status on %s: %v\n%s", surface, err, statusOutput)
		}
		var status capabilitypack.JSONStatusReport
		if err := json.Unmarshal([]byte(statusOutput), &status); err != nil {
			t.Fatal(err)
		}
		if len(status.Entries) != 1 || status.Entries[0].PackVersion != withdrawn.CurrentVersion() || status.Entries[0].Intent.Version != withdrawn.CurrentVersion() || !status.Entries[0].HistoricalEvidence.Available || status.Entries[0].UpdateAvailable {
			t.Fatalf("withdrawn status on %s = %#v", surface, status.Entries)
		}
	}

	showOutput, err := executeCommand(t, NewRootCommand(opts), "show", withdrawn.ID(), "--json")
	if err != nil {
		t.Fatalf("show withdrawn Pack: %v\n%s", err, showOutput)
	}
	var show packShowJSON
	if err := json.Unmarshal([]byte(showOutput), &show); err != nil {
		t.Fatal(err)
	}
	if show.CatalogState != "retained" || show.Version != withdrawn.CurrentVersion() || show.LifecycleAvailability.FreshActivationAvailable || show.LifecycleAvailability.CatalogUpdateAvailable || !show.LifecycleAvailability.LifecycleVerbsAvailable || show.LifecycleAvailability.AutomaticDowngrade {
		t.Fatalf("withdrawn show = %#v", show)
	}

	if out, err := executeCommand(t, NewRootCommand(opts), "update", withdrawn.ID(), "--surface", "codex", "--dry-run"); err == nil || !strings.Contains(err.Error(), "current catalog") {
		t.Fatalf("withdrawn update = %v\n%s", err, out)
	}

	backend := newTUIBackend(opts.withDefaults(), newWorkstationResolver(opts.withDefaults()))
	dashboard, err := backend.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	installed := findTUIPack(dashboard.Global.Packs, withdrawn.ID())
	if installed == nil || installed.Version != withdrawn.CurrentVersion() {
		t.Fatalf("TUI omitted retained Pack: %#v", dashboard.Global.Packs)
	}
	var installedStatus *tui.SurfaceStatus
	for index := range installed.SurfaceStatuses {
		if installed.SurfaceStatuses[index].Name == "codex" {
			installedStatus = &installed.SurfaceStatuses[index]
			break
		}
	}
	if installedStatus == nil || installedStatus.Supported || !installedStatus.Active || installedStatus.UpdateAvailable || installedStatus.CatalogUpdateAvailable || installedStatus.InstalledVersion != withdrawn.CurrentVersion() {
		t.Fatalf("TUI retained status = %#v", installedStatus)
	}

	for _, surface := range []string{"claude", "codex", "opencode"} {
		preview, err := backend.Preview(context.Background(), tui.PreviewRequest{
			Operation: "deactivate", PackID: withdrawn.ID(), Surface: surface, Scope: "global", Selection: tui.Selection{Mode: "all"},
		})
		if err != nil {
			t.Fatalf("preview retained deactivation on %s: %v", surface, err)
		}
		result, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: preview, ApprovedPhases: requiredTUIPhases(preview)}, func(tui.ApplyProgress) {})
		if err != nil || !result.Verified {
			t.Fatalf("apply retained deactivation on %s = %#v, %v", surface, result, err)
		}
	}
	if _, err := os.Stat(filepath.Join(catalogstore.DefaultDataRoot(env["HOME"]), "catalog", "snapshots", retainedSnapshot)); err != nil {
		t.Fatalf("retained Catalog Snapshot was removed after deactivation: %v", err)
	}
}
