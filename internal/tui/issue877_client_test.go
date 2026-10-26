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
		{name: "no implicit client"},
		{name: "explicit codex", toggle: true, wantClient: "codex"},
		{name: "retained codex", installedClient: "codex", wantClient: "codex"},
		{name: "clear explicit client", installedClient: "codex", toggle: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			pack := tui.Pack{ID: "reviewed", Resources: []tui.Resource{{Identity: "skill:review", Role: "operational", SelectionClosures: map[string][]string{"agents": {"skill:review", "agent:reviewer"}}}, {Identity: "agent:reviewer", Role: "supporting", ClientProjections: []tui.ClientProjection{{Surface: "agents", Client: "codex", Scopes: []string{"global"}}}}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "agents", Supported: true, Active: true, Selection: tui.Selection{Mode: "all", Client: scenario.installedClient}}}}
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

func TestSharedSkillsDoNotOfferOrSelectRuntimeClient(t *testing.T) {
	pack := tui.Pack{ID: "shared", Resources: []tui.Resource{{Identity: "skill:guide", Role: "operational"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "agents", Supported: true, Active: true, Selection: tui.Selection{Mode: "all"}}}}
	backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{pack}}}}
	var model tea.Model = openResourceConfiguration(t, backend, false)
	if strings.Contains(ansi.Strip(model.View().Content), "Role client:") {
		t.Fatal("shared skill selection offers a runtime client")
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Text: "c"}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Selection.Client != "" {
		t.Fatalf("shared skill acquired client selection: %#v", backend.previewRequests)
	}
}

func TestRemovingLastRoleConsumerClearsClientSelection(t *testing.T) {
	pack := tui.Pack{ID: "reviewed", Resources: []tui.Resource{
		{Identity: "skill:review", Role: "operational", SelectionClosures: map[string][]string{"agents": {"skill:review", "agent:reviewer"}}},
		{Identity: "skill:guide", Role: "operational"},
		{Identity: "agent:reviewer", Role: "supporting", ClientProjections: []tui.ClientProjection{{Surface: "agents", Client: "codex", Scopes: []string{"global"}}}},
	}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "agents", Supported: true, Active: true, Selection: tui.Selection{Mode: "all", Client: "codex"}}}}
	backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{pack}}}}
	var model tea.Model = openResourceConfiguration(t, backend, false)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if strings.Contains(ansi.Strip(model.View().Content), "Role client:") {
		t.Fatal("deselected role still offers a runtime client")
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Selection.Client != "" || strings.Join(backend.previewRequests[0].Selection.Roots, ",") != "skill:guide" {
		t.Fatalf("remaining shared skill retained a runtime client: %#v", backend.previewRequests)
	}
}
