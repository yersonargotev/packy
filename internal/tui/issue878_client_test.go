package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yersonargotev/packy/internal/tui"
)

func TestIssue878ClaudeRoleClientIsFixedAtEachSupportedScope(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			pack := tui.Pack{ID: "reviewed", Resources: []tui.Resource{
				{Identity: "skill:review", Role: "operational", SelectionClosures: map[string][]string{"claude": {"skill:review", "agent:reviewer"}}},
				{Identity: "agent:reviewer", Role: "supporting", ClientProjections: []tui.ClientProjection{{Surface: "claude", Client: "claude", Scopes: []string{"global", "project"}}}},
			}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "claude", Supported: true, Active: true, Installation: "installed", Selection: tui.Selection{Mode: "all"}}}}
			dashboardScope := tui.Scope{Available: true, Root: "/project", Packs: []tui.Pack{pack}}
			backend := &fakeBackend{dashboard: tui.Dashboard{Global: dashboardScope, Project: dashboardScope}}
			var model tea.Model = openResourceConfiguration(t, backend, scope == "project")
			view := ansi.Strip(model.View().Content)
			if !strings.Contains(view, "Custom-agent client: claude · scope: "+scope) || strings.Contains(view, "c to change") {
				t.Errorf("Claude client/scope must be fixed information:\n%s", view)
			}
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Text: "c"}))
			if len(backend.previewRequests) != 0 || len(backend.applyRequests) != 0 {
				t.Fatal("client key performed lifecycle effects")
			}
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
			runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			if len(backend.previewRequests) != 1 || backend.previewRequests[0].Selection.Client != "" {
				t.Fatalf("Claude preview acquired unsupported client selection: %#v", backend.previewRequests)
			}
		})
	}
}
