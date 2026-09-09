package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestOlderReceiptStatusPresentationsBeforeApply(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{"codex", "opencode", "claude"} {
		t.Run(surface, func(t *testing.T) {
			pack := testsupport.PortableAllSurfaces("historical-status")
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack, testsupport.PortableAllSurfaces("unrelated-current"))
			opts := fixture.options
			outside := t.TempDir()
			opts.Getwd = func() (string, error) { return outside, nil }
			if out, err := executeCommand(t, NewRootCommand(opts), "activate", pack.ID(), "--surface", surface); err != nil {
				t.Fatalf("activate: %v\n%s", err, out)
			}
			candidate := pack.Candidate().WithExactCopyBytes(pack.OperationalResource().String(), ".", []byte("# Updated guidance\n\nNew catalog content.\n"))
			if err := candidate.WriteBundle(fixture.bundleRoot); err != nil {
				t.Fatal(err)
			}
			before := snapshotTree(t, fixture.home)
			out, err := executeCommand(t, NewRootCommand(opts), "status", pack.ID(), "--surface", surface, "--json")
			if err != nil {
				t.Fatalf("status JSON: %v\n%s", err, out)
			}
			assertStructuredOutput(t, repoRoot, "pack-status.schema.json", out)
			var report capabilitypack.JSONStatusReport
			if err := json.Unmarshal([]byte(out), &report); err != nil {
				t.Fatal(err)
			}
			if len(report.Entries) != 1 {
				t.Fatalf("entries: %#v", report)
			}
			entry := report.Entries[0]
			if entry.PackVersion != candidate.CurrentVersion() || entry.Intent.Version != pack.CurrentVersion() || !entry.UpdateAvailable || entry.HistoricalEvidence.Available || entry.Contract != nil || entry.HistoricalEvidence.Message == "" || len(entry.Intent.Resources) == 0 || len(entry.Resources) != 0 || entry.Projections.Drifted != 0 || entry.Projections.Verified == 0 {
				t.Fatalf("historical receipt facts lost: %#v", entry)
			}
			human, err := executeCommand(t, NewRootCommand(opts), "status", pack.ID(), "--surface", surface)
			if err != nil {
				t.Fatalf("status: %v\n%s", err, human)
			}
			for _, want := range []string{"Update available: yes (" + pack.CurrentVersion() + " -> " + candidate.CurrentVersion() + ")", "Historical evidence: unavailable", "Resources: 2 selected", "Drift: 0 projections"} {
				if !strings.Contains(human, want) {
					t.Fatalf("missing %q:\n%s", want, human)
				}
			}
			for _, args := range [][]string{{"doctor"}, {"doctor", "--json"}} {
				doctor, err := executeCommand(t, NewRootCommand(opts), args...)
				if err != nil {
					t.Fatalf("doctor: %v\n%s", err, doctor)
				}
				for _, want := range []string{"an update is available", "packy update " + pack.ID() + " --surface " + surface, entry.HistoricalEvidence.Message} {
					if !strings.Contains(doctor, want) {
						t.Fatalf("doctor missing %q:\n%s", want, doctor)
					}
				}
				if strings.Contains(doctor, "inspection failed") || strings.Contains(doctor, "no confirmed health problems") {
					t.Fatalf("doctor lost update meaning:\n%s", doctor)
				}
			}
			overview, err := executeCommand(t, NewRootCommand(opts), "status", "--json")
			if err != nil {
				t.Fatalf("multi-Pack overview: %v\n%s", err, overview)
			}
			assertStructuredOutput(t, repoRoot, "pack-status.schema.json", overview)
			var all capabilitypack.JSONStatusReport
			if err := json.Unmarshal([]byte(overview), &all); err != nil {
				t.Fatal(err)
			}
			if !slices.ContainsFunc(all.Entries, func(value capabilitypack.JSONStatusEntry) bool {
				return value.Pack == pack.ID() && string(value.Surface) == surface && value.Intent.Version == pack.CurrentVersion() && value.UpdateAvailable
			}) {
				t.Fatalf("multi-Pack overview dropped historical receipt: %#v", all)
			}
			backendOpts := opts.withDefaults()
			dashboard, err := newTUIBackend(backendOpts, newWorkstationResolver(backendOpts)).Load(context.Background())
			if err != nil || len(dashboard.Setup.Blockers) != 0 {
				t.Fatalf("multi-Pack dashboard blocked: %v %#v", err, dashboard.Setup)
			}
			installed := findTUIPack(dashboard.Global.Packs, pack.ID())
			if installed == nil || !slices.ContainsFunc(installed.SurfaceStatuses, func(value tui.SurfaceStatus) bool {
				return value.Name == surface && value.InstalledVersion == pack.CurrentVersion() && value.UpdateAvailable && value.HistoricalEvidenceMessage != ""
			}) {
				t.Fatalf("multi-Pack dashboard lost receipt: %#v", installed)
			}
			manifest := candidate.Manifest()
			manifest.Surfaces = slices.DeleteFunc(manifest.Surfaces, func(value testsupport.Surface) bool { return string(value) == surface })
			for i := range manifest.Resources {
				manifest.Resources[i].Bindings = slices.DeleteFunc(manifest.Resources[i].Bindings, func(value testsupport.Binding) bool { return string(value.Surface) == surface })
			}
			manifestBytes, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(fixture.bundleRoot, "packs", pack.ID(), "pack.json"), manifestBytes, 0o644); err != nil {
				t.Fatal(err)
			}
			doctor, err := executeCommand(t, NewRootCommand(opts), "doctor")
			if err != nil {
				t.Fatalf("removed surface doctor: %v\n%s", err, doctor)
			}
			if strings.Contains(doctor, "packy update") || !strings.Contains(doctor, "cannot be applied on this surface") || !strings.Contains(doctor, "an update is available") {
				t.Fatalf("removed surface doctor recommends unavailable mutation:\n%s", doctor)
			}
			overview, err = executeCommand(t, NewRootCommand(opts), "status", "--json")
			if err != nil {
				t.Fatalf("removed-surface multi-Pack overview: %v\n%s", err, overview)
			}
			assertStructuredOutput(t, repoRoot, "pack-status.schema.json", overview)
			if err := json.Unmarshal([]byte(overview), &all); err != nil {
				t.Fatal(err)
			}
			if !slices.ContainsFunc(all.Entries, func(value capabilitypack.JSONStatusEntry) bool {
				return value.Pack == pack.ID() && string(value.Surface) == surface && value.Intent.Version == pack.CurrentVersion() && value.UpdateAvailable
			}) {
				t.Fatalf("removed-surface overview dropped receipt: %#v", all)
			}
			dashboard, err = newTUIBackend(backendOpts, newWorkstationResolver(backendOpts)).Load(context.Background())
			if err != nil || len(dashboard.Setup.Blockers) != 0 {
				t.Fatalf("removed-surface dashboard blocked: %v %#v", err, dashboard.Setup)
			}
			installed = findTUIPack(dashboard.Global.Packs, pack.ID())
			if installed == nil || !slices.ContainsFunc(installed.SurfaceStatuses, func(value tui.SurfaceStatus) bool {
				return value.Name == surface && !value.Supported && value.InstalledVersion == pack.CurrentVersion() && value.CatalogUpdateAvailable && !value.UpdateAvailable && value.HistoricalEvidenceMessage != ""
			}) {
				t.Fatalf("removed-surface dashboard lost evidence or enabled unavailable update: %#v", installed)
			}
			if after := snapshotTree(t, fixture.home); after != before {
				t.Fatal("read-only inspection changed receipt or projections")
			}
		})
	}
}

func TestTUICatalogPreservesReceiptWhenCurrentSurfaceIsRemoved(t *testing.T) {
	observed := tui.SurfaceStatus{Name: "claude", Supported: true, Active: true, InstalledVersion: "1.0.0", UpdateAvailable: true, HistoricalEvidenceMessage: "Historical manifest unavailable", Ownership: 2, Drift: 1}
	packs := catalogPacksForTUI([]capabilitypack.CatalogDetail{{Pack: capabilitypack.Pack{ID: "removed-surface", Version: "1.0.1", Surfaces: []capabilitypack.Surface{capabilitypack.SurfaceCodex}}}}, map[string]map[string]tui.SurfaceStatus{"removed-surface": {"claude": observed}})
	for _, status := range packs[0].SurfaceStatuses {
		if status.Name != "claude" {
			continue
		}
		if status.Supported || !status.Active || status.InstalledVersion != observed.InstalledVersion || status.HistoricalEvidenceMessage != observed.HistoricalEvidenceMessage || status.Ownership != 2 || status.Drift != 1 || !status.UpdateAvailable {
			t.Fatalf("lost receipt evidence or invented current support: %#v", status)
		}
		return
	}
	t.Fatal("installed surface disappeared")
}
