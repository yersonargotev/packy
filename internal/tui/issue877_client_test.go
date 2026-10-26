package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestAgentsClientSelectionIsExplicitAndRetainedInPreview(t *testing.T) {
	for _, scenario := range []struct {
		name, installedClient, wantClient string
		toggle                            bool
	}{
		{name: "shared default"},
		{name: "explicit codex", toggle: true, wantClient: "codex"},
		{name: "retained codex", installedClient: "codex", wantClient: "codex"},
		{name: "clear explicit client", installedClient: "codex", toggle: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			pack := tui.Pack{ID: "reviewed", Resources: []tui.Resource{{Identity: "skill:review", Role: "operational"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "agents", Supported: true, Active: true, Selection: tui.Selection{Mode: "all", Client: scenario.installedClient}}}}
			backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{pack}}}}
			backend.previewFor = func(request tui.PreviewRequest) (tui.Preview, error) {
				return tui.Preview{ID: "exact", Digest: "reviewed", Operation: request.Operation, PackID: request.PackID, Surface: request.Surface, Scope: request.Scope, Selection: request.Selection, Disposition: "applicable"}, nil
			}
			var model tea.Model = openResourceConfiguration(t, backend, false)
			if scenario.toggle {
				model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Text: "c"}))
			}
			if len(backend.previewRequests) != 0 || len(backend.applyRequests) != 0 {
				t.Fatal("client selection performed lifecycle effects")
			}
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
			model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			if len(backend.previewRequests) != 1 || backend.previewRequests[0].Selection.Client != scenario.wantClient {
				t.Fatalf("preview did not preserve explicit client: %#v", backend.previewRequests)
			}
			view := ansi.Strip(model.View().Content)
			if scenario.wantClient == "codex" && !strings.Contains(view, "Custom-agent client: codex · scope: global") {
				t.Fatalf("preview omitted concrete client/scope:\n%s", view)
			}
		})
	}
}
