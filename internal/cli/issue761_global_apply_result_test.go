package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
	"github.com/yersonargotev/packy/internal/capabilitypack/testsupport"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestIssue761GlobalApplyReportsSelectedPackProjectionCount(t *testing.T) {
	first := testsupport.PortableAllSurfaces("issue761-first")
	second := testsupport.ExternalTool("issue761-second")
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, first, second)
	makeSyntheticExternalToolObservable(t, &fixture.options)
	layout := newCLITestFixture(t, fixture.options)

	if output, err := executeCommand(t, NewRootCommand(fixture.options), "activate", first.ID(), "--surface", "codex"); err != nil {
		t.Fatalf("activate first Pack: %v\n%s", err, output)
	}
	output, err := executeCommand(t, NewRootCommand(fixture.options), "activate", second.ID(), "--surface", "codex")
	if err != nil {
		t.Fatalf("activate second Pack: %v\n%s", err, output)
	}
	for _, fact := range []string{"1 Codex projections owned by " + second.ID(), "projections=1"} {
		if !strings.Contains(output, fact) {
			t.Fatalf("second Pack activation omitted Pack-scoped fact %q:\n%s", fact, output)
		}
	}
	state, err := capabilitypack.NewFileActivationStore(layout.packState.File()).LoadSnapshot(context.Background(), capabilitypack.SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Ownership) != 2 {
		t.Fatalf("multi-Pack activation ownership = %#v, want two Pack-scoped facts", state.Ownership)
	}
	if err := second.Candidate().WriteBundle(fixture.bundleRoot); err != nil {
		t.Fatal(err)
	}
	output, err = executeCommand(t, NewRootCommand(fixture.options), "update", second.ID(), "--surface", "codex")
	if err != nil {
		t.Fatalf("update second Pack: %v\n%s", err, output)
	}
	for _, fact := range []string{"1 Codex projections owned by " + second.ID(), "projections=1"} {
		if !strings.Contains(output, fact) {
			t.Fatalf("second Pack update omitted Pack-scoped fact %q:\n%s", fact, output)
		}
	}

	output, err = executeCommand(t, NewRootCommand(fixture.options), "deactivate", second.ID(), "--surface", "codex", "--json")
	if err != nil {
		t.Fatalf("deactivate second Pack: %v\n%s", err, output)
	}
	decoder := json.NewDecoder(strings.NewReader(output))
	var preview capabilitypack.JSONLifecyclePlan
	var applied capabilitypack.JSONApplyResult
	if err := decoder.Decode(&preview); err != nil {
		t.Fatalf("decode deactivation preview: %v\n%s", err, output)
	}
	if err := decoder.Decode(&applied); err != nil {
		t.Fatalf("decode deactivation result: %v\n%s", err, output)
	}
	if applied.Projections != 0 {
		t.Fatalf("deactivated Pack projections = %d, want 0\n%s", applied.Projections, output)
	}

	state, err = capabilitypack.NewFileActivationStore(layout.packState.File()).LoadSnapshot(context.Background(), capabilitypack.SurfaceCodex)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Ownership) != 1 || state.Ownership[0].PackID != first.ID() {
		t.Fatalf("deactivation changed unrelated Pack ownership: %#v", state.Ownership)
	}

	opts := fixture.options.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	tuiPreview, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "activate", PackID: second.ID(), Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	tuiResult, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: tuiPreview, ApprovedPhases: requiredTUIPhases(tuiPreview)}, func(tui.ApplyProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tuiResult.Details, []string{"1 projections owned"}) {
		t.Fatalf("TUI activation details = %#v, want Pack-scoped count", tuiResult.Details)
	}
}

func TestIssue761PartialDeactivationReportsSelectedPackRemainingProjections(t *testing.T) {
	unrelated := testsupport.ExternalTool("issue761-unrelated")
	selected := testsupport.CapabilityRich("issue761-partial")
	fixture := newSyntheticCLIFixture(t, &fakeTerminal{interactive: true, approve: true}, unrelated, selected)
	makeSyntheticExternalToolObservable(t, &fixture.options)
	layout := newCLITestFixture(t, fixture.options)

	if output, err := executeCommand(t, NewRootCommand(fixture.options), "activate", unrelated.ID(), "--surface", "codex"); err != nil {
		t.Fatalf("activate unrelated Pack: %v\n%s", err, output)
	}
	opts := fixture.options.withDefaults()
	backend := newTUIBackend(opts, newWorkstationResolver(opts))
	activate, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "activate", PackID: selected.ID(), Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: activate, ApprovedPhases: requiredTUIPhases(activate)}, func(tui.ApplyProgress) {}); err != nil {
		t.Fatal(err)
	}

	resource := syntheticResource(t, selected, "instruction", "guidance")
	partial, err := backend.Preview(context.Background(), tui.PreviewRequest{
		Operation: "deactivate", PackID: selected.ID(), Surface: "codex", Scope: "global",
		Selection: tui.Selection{Mode: "custom", Roots: []string{resource.Kind + ":" + resource.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := backend.Apply(context.Background(), tui.ApplyRequest{Preview: partial, ApprovedPhases: requiredTUIPhases(partial)}, func(tui.ApplyProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	want := issue761ReceiptProjectionCount(t, layout.packState.File(), selected.ID())
	if want == 0 {
		t.Fatal("partial deactivation removed every selected-Pack projection")
	}
	if !slices.Equal(result.Details, []string{fmt.Sprintf("%d projections owned", want)}) {
		t.Fatalf("partial deactivation details = %#v, want %d selected-Pack projections", result.Details, want)
	}
	if got := issue761ReceiptProjectionCount(t, layout.packState.File(), unrelated.ID()); got != 1 {
		t.Fatalf("partial deactivation changed unrelated Pack receipt projections = %d, want 1", got)
	}
}

func issue761ReceiptProjectionCount(t *testing.T, path, packID string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Receipts []struct {
			Pack struct {
				ID string `json:"id"`
			} `json:"pack"`
			Projections []json.RawMessage `json:"projections"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode installed receipts: %v\n%s", err, data)
	}
	for _, receipt := range document.Receipts {
		if receipt.Pack.ID == packID {
			return len(receipt.Projections)
		}
	}
	t.Fatalf("installed receipts omitted Pack %q: %s", packID, data)
	return 0
}
