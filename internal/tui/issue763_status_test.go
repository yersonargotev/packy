package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestOlderReceiptDetailDisplaysUpdateAndUnavailableEvidence(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{{ID: "older", Version: "2.0.0", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, UpdateAvailable: true, InstalledVersion: "1.0.0", HistoricalEvidenceMessage: "Historical resource and contract evidence is unavailable"}}}}}}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"2.0.0", "Installed version: 1.0.0", "Update available", "Historical evidence: unavailable"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}

func TestRemovedSurfaceRetainsVisibleReceiptWithoutEnablingLifecycle(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{{ID: "older", Version: "2.0.0", SurfaceStatuses: []tui.SurfaceStatus{{Name: "claude", Active: true, CatalogUpdateAvailable: true, InstalledVersion: "1.0.0", Ownership: 2, Drift: 1, HistoricalEvidenceMessage: "Historical manifest is unavailable"}}}}}}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"claude: unsupported", "Update available", "Current catalog does not support this surface", "Installed version: 1.0.0", "Historical evidence: unavailable", "Ownership: 2 projected paths", "Drift: 1 projections", "No applicable global action"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	current, cmd := current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil || !strings.Contains(ansi.Strip(current.View().Content), "Pack details") {
		t.Fatal("unsupported catalog surface enabled lifecycle selection")
	}
}
