package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestGlobalUpdateContractDiffPresentations(t *testing.T) {
	for _, surface := range []string{"codex", "opencode", "claude"} {
		t.Run(surface, func(t *testing.T) {
			pack := testsupport.PortableAllSurfaces("truthful-diff")
			fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, pack)
			opts := fixture.options
			if out, err := executeCommand(t, NewRootCommand(opts), "activate", pack.ID(), "--surface", surface); err != nil {
				t.Fatalf("activate: %v\n%s", err, out)
			}
			for _, historical := range []bool{false, true} {
				if historical {
					if err := pack.Candidate().WriteBundle(fixture.bundleRoot); err != nil {
						t.Fatal(err)
					}
				}
				before := snapshotTree(t, fixture.home)
				out, err := executeCommand(t, NewRootCommand(opts), "update", pack.ID(), "--surface", surface, "--dry-run", "--json")
				if historical && surface == "claude" {
					// Claude requires an exact historical mutation contract before previewing.
					if err == nil || !strings.Contains(out, "no exact registered adapter contract") {
						t.Fatalf("lost Claude ownership guard: %v\n%s", err, out)
					}
					if snapshotTree(t, fixture.home) != before {
						t.Fatal("failed preview mutated installed state")
					}
					continue
				}
				if err != nil {
					t.Fatalf("preview: %v\n%s", err, out)
				}
				var report capabilitypack.JSONLifecyclePlan
				if err := json.Unmarshal([]byte(out), &report); err != nil {
					t.Fatal(err)
				}
				diff := report.ContractDiff
				if diff.BaselineAvailable == historical || (historical && diff.UnavailableReason != "historical_contract_unavailable") || (!historical && diff.UnavailableReason != "") {
					t.Fatalf("wrong comparison provenance: %#v", diff)
				}
				if len(diff.Added) != 0 || len(diff.Changed) != 0 || len(diff.Removed) != 0 {
					t.Fatalf("fabricated changes (historical=%v): %#v", historical, diff)
				}
				if !historical && len(diff.Retained) != 2 {
					t.Fatalf("converged diff = %#v, want two retained resources", diff)
				}
				if historical && len(diff.Retained) != 0 {
					t.Fatalf("fabricated retention: %#v", diff)
				}
				root, err := filepath.Abs(filepath.Join("..", ".."))
				if err != nil {
					t.Fatal(err)
				}
				assertStructuredOutput(t, root, "pack-lifecycle.schema.json", out)
				human, err := executeCommand(t, NewRootCommand(opts), "update", pack.ID(), "--surface", surface, "--dry-run")
				if err != nil {
					t.Fatalf("human preview: %v\n%s", err, human)
				}
				want := "Contract diff baseline: available"
				if historical {
					want = "Contract diff baseline: unavailable (historical_contract_unavailable)"
				}
				if !strings.Contains(human, want) {
					t.Fatalf("human preview missing %q:\n%s", want, human)
				}
				configured := opts.withDefaults()
				backend := newTUIBackend(configured, newWorkstationResolver(configured))
				preview, err := backend.Preview(context.Background(), tui.PreviewRequest{Operation: "update", PackID: pack.ID(), Surface: surface, Scope: "global"})
				if err != nil {
					t.Fatal(err)
				}
				if preview.Diff.BaselineAvailable != diff.BaselineAvailable || preview.Diff.UnavailableReason != diff.UnavailableReason || !reflect.DeepEqual(preview.Diff.Added, diff.Added) || !reflect.DeepEqual(preview.Diff.Changed, diff.Changed) || !reflect.DeepEqual(preview.Diff.Removed, diff.Removed) || !reflect.DeepEqual(preview.Diff.Retained, diff.Retained) {
					t.Fatalf("TUI differs from JSON: %#v / %#v", preview.Diff, diff)
				}
				if snapshotTree(t, fixture.home) != before {
					t.Fatal("preview mutated installed state")
				}
			}
		})
	}
}
