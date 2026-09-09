package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
)

func TestOlderReceiptStatusPresentationsBeforeApply(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{"codex", "opencode", "claude"} {
		t.Run(surface, func(t *testing.T) {
			pack := testsupport.PortableAllSurfaces("historical-status")
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
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
			if after := snapshotTree(t, fixture.home); after != before {
				t.Fatal("read-only inspection changed receipt or projections")
			}
		})
	}
}
