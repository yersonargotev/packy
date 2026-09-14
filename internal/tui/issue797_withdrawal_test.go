package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestIssue797WithdrawnPackOffersOnlyDeactivation(t *testing.T) {
	backend := &fakeBackend{
		dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Available: true, Packs: []tui.Pack{{
			ID: "withdrawn-runtime", Version: "1.0.0", Description: "Retained installed Pack", CatalogState: "retained",
			SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: false, Active: true, InstalledVersion: "1.0.0"}},
		}}}},
		preview: tui.Preview{ID: "withdrawn-deactivation", Digest: "digest", Operation: "deactivate", Disposition: "applicable", PackID: "withdrawn-runtime", PackVersion: "1.0.0", Surface: "codex", Scope: "global", Selection: tui.Selection{Mode: "all"}},
	}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Choose lifecycle action") || !strings.Contains(view, "Deactivate") || strings.Contains(view, "Configure") || strings.Contains(view, "Update") || strings.Contains(view, "Activate") {
		t.Fatalf("withdrawn Pack actions are not deactivation-only:\n%s", view)
	}
	model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Operation != "deactivate" || backend.previewRequests[0].Surface != "codex" {
		t.Fatalf("withdrawn Pack preview request = %#v", backend.previewRequests)
	}
}
