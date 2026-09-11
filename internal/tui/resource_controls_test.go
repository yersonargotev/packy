package tui_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yersonargotev/packy/internal/tui"
)

func openResourceConfiguration(t *testing.T, backend *fakeBackend, project bool) tui.Model {
	t.Helper()
	model := loadModel(t, backend)
	if project {
		model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	}
	for range 3 {
		model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	}
	return model.(tui.Model)
}

func TestNewPackOpensResourceChecklistWithoutSelectionModeStep(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{{
		ID: "resource-list", Resources: []tui.Resource{{Identity: "skill:review", Description: "Review proposed changes", Role: "operational"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true}},
	}}}}}
	model := loadModel(t, backend)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"[x] skill:review", "Review proposed changes", "Apply changes", "1 of 1 selected"} {
		if !strings.Contains(view, want) {
			t.Fatalf("direct checklist missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Selection mode") || len(backend.previewRequests) != 0 {
		t.Fatal("new Pack still needs a mode-selection step or created a preview immediately")
	}
}

func TestResourceConfigurationPreservesSelectionModeWithoutEdits(t *testing.T) {
	for _, mode := range []string{"all", "custom"} {
		for _, project := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/project=%t", mode, project), func(t *testing.T) {
				selection := tui.Selection{Mode: mode}
				if mode == "custom" {
					selection.Roots = []string{"skill:a", "skill:b"}
				}
				pack := tui.Pack{ID: "resource-controls", Resources: []tui.Resource{{Identity: "skill:a", Role: "operational"}, {Identity: "skill:b", Role: "operational"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, Installation: "installed", Selection: selection}}}
				scope := tui.Scope{Available: true, Root: "/project", Packs: []tui.Pack{pack}}
				backend := &fakeBackend{dashboard: tui.Dashboard{Global: scope, Project: scope}}
				var model tea.Model = openResourceConfiguration(t, backend, project)
				model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
				runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
				if len(backend.previewRequests) != 1 {
					t.Fatalf("expected one preview: %#v", backend.previewRequests)
				}
				got := backend.previewRequests[0].Selection
				if got.Mode != selection.Mode || !slices.Equal(got.Roots, selection.Roots) {
					t.Fatalf("unedited selection changed: got %#v, want %#v", got, selection)
				}
			})
		}
	}
}

func TestResourceConfigurationStagesChangesAndDeselectsConsumers(t *testing.T) {
	pack := tui.Pack{ID: "resource-controls", Version: "1.0.0", Resources: []tui.Resource{
		{Identity: "skill:writer", Role: "operational", SelectionClosures: map[string][]string{"codex": {"skill:writer", "skill:helper", "notice:mit"}}},
		{Identity: "skill:helper", Role: "operational"},
		{Identity: "mcp_server:search", Role: "operational"},
		{Identity: "notice:mit", Role: "notice"},
	}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, Selection: tui.Selection{Mode: "custom", Roots: []string{"skill:writer"}}}}}
	backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{pack}}}}
	model := openResourceConfiguration(t, backend, false)
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"[x] skill:writer", "[x] skill:helper", "[ ] mcp_server:search", "notice:mit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("current selection missing %q:\n%s", want, view)
		}
	}
	var next tea.Model = model
	for _, code := range []rune{tea.KeyDown, tea.KeyDown, tea.KeyEnter, tea.KeyUp, tea.KeyEnter} {
		next, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: code}))
	}
	view = ansi.Strip(next.View().Content)
	for _, want := range []string{"[ ] skill:writer", "[ ] skill:helper", "[x] mcp_server:search", "Also deselected dependent resources: skill:writer"} {
		if !strings.Contains(view, want) {
			t.Fatalf("staged selection missing %q:\n%s", want, view)
		}
	}
	if len(backend.previewRequests) != 0 || len(backend.applyRequests) != 0 {
		t.Fatal("editing selection caused lifecycle effects before Apply changes")
	}
	next, _ = next.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	runModelMessage(t, next, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Operation != "configure" || !slices.Equal(backend.previewRequests[0].Selection.Roots, []string{"mcp_server:search"}) {
		t.Fatalf("desired selection was not previewed exactly: %#v", backend.previewRequests)
	}
}

func TestResourceConfigurationAppliesOnlyAfterAllConsentAndReloads(t *testing.T) {
	for _, scope := range []string{"global", "project"} {
		t.Run(scope, func(t *testing.T) {
			pack := tui.Pack{ID: "resource-controls", Resources: []tui.Resource{{Identity: "skill:a", Role: "operational"}, {Identity: "skill:b", Role: "operational"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, Installation: "installed", Selection: tui.Selection{Mode: "all"}}}}
			installed := tui.Scope{Available: true, Root: "/project", Packs: []tui.Pack{pack}}
			disposition := "applicable"
			if scope == "project" {
				disposition = "previewable"
			}
			backend := &fakeBackend{
				dashboard: tui.Dashboard{Global: installed, Project: installed},
				preview: tui.Preview{ID: "configure-1", Digest: "exact", Operation: "configure", Scope: scope, PackID: "resource-controls", Surface: "codex", Disposition: disposition, Phases: []tui.PreviewPhase{
					{Kind: "reversible-local", ApprovalRequired: true},
					{Kind: "destructive-cleanup", ApprovalRequired: true},
				}},
				applyResult: tui.ApplyResult{Stage: "verification", Verified: true, Summary: "Configured resource-controls"},
			}
			var model tea.Model = openResourceConfiguration(t, backend, scope == "project")
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
			model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Enter continue to consent") {
				t.Fatalf("resource configuration cannot continue:\n%s", view)
			}
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			if len(backend.applyRequests) != 0 {
				t.Fatal("configuration applied before destructive consent")
			}
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
			model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			if len(backend.applyRequests) != 1 || !slices.Equal(backend.applyRequests[0].ApprovedPhases, []string{"reversible-local", "destructive-cleanup"}) {
				t.Fatalf("configuration did not apply approved phases: %#v", backend.applyRequests)
			}
			if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Resource configuration succeeded") || !strings.Contains(view, "Fresh Pack status reloaded") {
				t.Fatalf("configuration did not reach verified result:\n%s", view)
			}
		})
	}
}

func TestClearingResourceConfigurationPreviewsWholeRemovalAndCanBeCancelled(t *testing.T) {
	for _, project := range []bool{false, true} {
		t.Run(fmt.Sprintf("project=%t", project), func(t *testing.T) {
			pack := tui.Pack{ID: "resource-controls", Resources: []tui.Resource{{Identity: "command:review", Role: "operational"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, Installation: "installed", Runtime: "active", Selection: tui.Selection{Mode: "all"}}}}
			scope := tui.Scope{Available: true, Root: "/project", Packs: []tui.Pack{pack}}
			backend := &fakeBackend{dashboard: tui.Dashboard{Global: scope, Project: scope}}
			backend.previewFor = func(request tui.PreviewRequest) (tui.Preview, error) {
				return tui.Preview{Operation: request.Operation, Scope: request.Scope, PackID: request.PackID, Disposition: "applicable"}, nil
			}
			var model tea.Model = openResourceConfiguration(t, backend, project)
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			consequence, operation := "deactivate the complete Pack", "deactivate"
			if project {
				consequence, operation = "uninstall the Pack", "uninstall"
			}
			if view := ansi.Strip(model.View().Content); !strings.Contains(view, consequence) {
				t.Fatalf("missing empty-selection consequence:\n%s", view)
			}
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
			model = runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			if len(backend.previewRequests) != 1 || backend.previewRequests[0].Operation != operation || len(backend.previewRequests[0].Selection.Roots) != 0 {
				t.Fatalf("wrong whole-Pack removal: %#v", backend.previewRequests)
			}
			model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
			if len(backend.applyRequests) != 0 {
				t.Fatal("cancelling removal preview applied changes")
			}
		})
	}
}

func TestResourceConfigurationLoadsSelectionForChangedSurface(t *testing.T) {
	pack := tui.Pack{ID: "resource-controls", Resources: []tui.Resource{{Identity: "skill:a", Role: "operational"}, {Identity: "skill:b", Role: "operational"}}, SurfaceStatuses: []tui.SurfaceStatus{
		{Name: "codex", Supported: true, Active: true, Selection: tui.Selection{Mode: "custom", Roots: []string{"skill:a"}}},
		{Name: "opencode", Supported: true, Active: true, Selection: tui.Selection{Mode: "custom", Roots: []string{"skill:b"}}},
		{Name: "claude", Supported: true},
	}}
	backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{pack}}}}
	var model tea.Model = openResourceConfiguration(t, backend, false)
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "[ ] skill:a") || !strings.Contains(view, "[x] skill:b") {
		t.Fatalf("surface selection leaked:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || backend.previewRequests[0].Operation != "activate" || backend.previewRequests[0].Surface != "claude" {
		t.Fatalf("inactive surface did not use fresh activation: %#v", backend.previewRequests)
	}
}

func TestResourceSearchPreservesHiddenSelectionsAndAcceptsQuitLetter(t *testing.T) {
	pack := tui.Pack{ID: "resource-list", Resources: []tui.Resource{
		{Identity: "skill:query", Role: "operational", Description: "Query the project"},
		{Identity: "command:report", Role: "operational", Description: "Write a report"},
	}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, Selection: tui.Selection{Mode: "all"}}}}
	backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{pack}}}}
	var model tea.Model = openResourceConfiguration(t, backend, false)
	model = resourceListMessage(t, model, tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	model = resourceListMessage(t, model, tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "skill:query") || strings.Contains(view, "command:report") {
		t.Fatalf("resource search did not filter the list:\n%s", view)
	}
	model = resourceListMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = resourceListMessage(t, model, tea.KeyPressMsg(tea.Key{Code: ' ', Text: " "}))
	model = resourceListMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "[ ] skill:query") || !strings.Contains(view, "[x] command:report") || !strings.Contains(view, "1 of 2 selected") {
		t.Fatalf("filtering lost or toggled a hidden selection:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	runModelMessage(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if len(backend.previewRequests) != 1 || !slices.Equal(backend.previewRequests[0].Selection.Roots, []string{"command:report"}) {
		t.Fatalf("filtered edit sent the wrong desired resources: %#v", backend.previewRequests)
	}
}

// Deliver asynchronous list results without advancing recurring cursor timers.
func resourceListMessage(t *testing.T, model tea.Model, message tea.Msg) tea.Model {
	t.Helper()
	model, command := model.Update(message)
	queue := []tea.Cmd{command}
	for len(queue) > 0 {
		command, queue = queue[0], queue[1:]
		if command == nil {
			continue
		}
		message := command()
		if batch, ok := message.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		if _, quit := message.(tea.QuitMsg); quit {
			t.Fatal("typing into the resource filter quit the application")
		}
		model, _ = model.Update(message)
	}
	return model
}

func TestResourceListKeepsActionsVisibleAtMinimumTerminalSize(t *testing.T) {
	pack := tui.Pack{ID: "resource-list", Resources: []tui.Resource{{Identity: "skill:a", Role: "operational"}, {Identity: "notice:license", Role: "notice"}}, SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, Selection: tui.Selection{Mode: "all"}}}}
	backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{pack}}}}
	var model tea.Model = openResourceConfiguration(t, backend, false)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 48, Height: 14})
	for _, message := range []tea.Msg{tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}), tea.KeyPressMsg(tea.Key{Code: tea.KeyTab})} {
		model, _ = model.Update(message)
		view := ansi.Strip(model.View().Content)
		if len(strings.Split(view, "\n")) > 14 || !strings.Contains(view, "Apply changes") {
			t.Fatalf("minimum-size selection lost its fixed actions:\n%s", view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > 48 {
				t.Fatalf("selection exceeds terminal width: %q", line)
			}
		}
	}
}

func TestResourceConfigurationKeepsFocusedResourceAndApplyVisible(t *testing.T) {
	pack := tui.Pack{ID: "resource-controls", SurfaceStatuses: []tui.SurfaceStatus{{Name: "codex", Supported: true, Active: true, Selection: tui.Selection{Mode: "all"}}}}
	for index := range 40 {
		pack.Resources = append(pack.Resources, tui.Resource{Identity: fmt.Sprintf("skill:item-%02d", index), Role: "operational"})
	}
	backend := &fakeBackend{dashboard: tui.Dashboard{Global: tui.Scope{Available: true, Packs: []tui.Pack{pack}}}}
	var model tea.Model = openResourceConfiguration(t, backend, false)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for range 39 {
		model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "› [x] skill:item-39") {
		t.Fatalf("focused resource is off screen:\n%s", view)
	}
	model, _ = model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "› [ Apply changes") {
		t.Fatalf("Apply control is off screen:\n%s", view)
	}
}
