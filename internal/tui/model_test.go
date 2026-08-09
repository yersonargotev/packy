package tui_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yersonargotev/packy/internal/tui"
)

type fakeBackend struct {
	dashboard tui.Dashboard
	err       error
	loads     int
}

func TestRunEntersAndRestoresAlternateScreen(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}}}
	input := bytes.NewBufferString("q")
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := tui.Run(ctx, backend, input, &output); err != nil {
		t.Fatal(err)
	}
	for _, sequence := range []string{"\x1b[?1049h", "\x1b[?1049l"} {
		if !strings.Contains(output.String(), sequence) {
			t.Fatalf("terminal output missing alternate-screen sequence %q: %q", sequence, output.String())
		}
	}
}

func TestDashboardNavigationKeyMapAndNarrowLayout(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "warnings", Passes: 2, Warnings: 1},
		Global: tui.Scope{Packs: []tui.Pack{
			{ID: "argote", Version: "1.2.0", Description: "Agent guidance"},
			{ID: "matty", Version: "1.0.0", Description: "Product review"},
		}},
		Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{ID: "matty", Version: "1.0.0"}}},
	}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	current, _ = current.Update(tea.WindowSizeMsg{Width: 64, Height: 30})

	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Text: "j", Code: 'j'}))
	view := ansi.Strip(current.View().Content)
	if !strings.Contains(view, "› matty") || !strings.Contains(view, "Product review") {
		t.Fatalf("down navigation did not select the next Pack:\n%s", view)
	}

	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	view = ansi.Strip(current.View().Content)
	if !strings.Contains(view, "Current project · selected") || !strings.Contains(view, "/workspace/project") {
		t.Fatalf("tab did not select project scope:\n%s", view)
	}
	if strings.Contains(lineContaining(view, "Workstation"), "Current project") {
		t.Fatalf("narrow layout did not stack scopes:\n%s", view)
	}

	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Text: "?", Code: '?'}))
	view = ansi.Strip(current.View().Content)
	for _, want := range []string{"arrows/j/k", "Tab/Shift+Tab", "Enter", "Esc", "Ctrl+C", "r reload", "q quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded key help missing %q:\n%s", want, view)
		}
	}
}

func lineContaining(value, target string) string {
	for _, line := range strings.Split(value, "\n") {
		if strings.Contains(line, target) {
			return line
		}
	}
	return ""
}

func TestDashboardRendersEmptyAndLoadFailureStates(t *testing.T) {
	tests := []struct {
		name    string
		backend *fakeBackend
		want    []string
	}{
		{
			name:    "empty",
			backend: &fakeBackend{dashboard: tui.Dashboard{Health: tui.Health{Status: "healthy"}}},
			want:    []string{"No reviewed Packs are available", "No Git project", "Global inspection remains available"},
		},
		{
			name:    "failure",
			backend: &fakeBackend{err: errors.New("catalog is unavailable")},
			want:    []string{"Unable to load dashboard", "catalog is unavailable", "r reload", "q quit"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := tui.NewModel(test.backend)
			updated, _ := model.Update(model.Init()())
			view := updated.View().Content
			for _, want := range test.want {
				if !strings.Contains(view, want) {
					t.Fatalf("view missing %q:\n%s", want, view)
				}
			}
		})
	}
}

func TestDashboardReloadsAfterFailureAndAlwaysRequestsAlternateScreen(t *testing.T) {
	backend := &fakeBackend{err: errors.New("temporary failure")}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	if !current.View().AltScreen {
		t.Fatal("dashboard did not request the alternate screen")
	}

	backend.err = nil
	backend.dashboard = tui.Dashboard{Health: tui.Health{Status: "healthy"}, Global: tui.Scope{Packs: []tui.Pack{{ID: "argote", Version: "1.0.0"}}}}
	current, reload := current.Update(tea.KeyPressMsg(tea.Key{Text: "r", Code: 'r'}))
	if reload == nil || !strings.Contains(current.View().Content, "Loading Packy health") {
		t.Fatalf("reload did not return to loading state:\n%s", current.View().Content)
	}
	current, _ = current.Update(reload())
	if backend.loads != 2 || !strings.Contains(current.View().Content, "argote") {
		t.Fatalf("reload did not replace the failure state: loads=%d\n%s", backend.loads, current.View().Content)
	}
}

func (b *fakeBackend) Load(context.Context) (tui.Dashboard, error) {
	b.loads++
	return b.dashboard, b.err
}

func TestDashboardLoadsThroughInjectedBackendOutsideUpdate(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy", Passes: 1, Checks: []tui.HealthCheck{{Name: "packy-core", Severity: "PASS", Detail: "Packy core is available"}}},
		Global: tui.Scope{Packs: []tui.Pack{{ID: "matty", Version: "1.0.0"}}},
	}}
	model := tui.NewModel(backend)

	if backend.loads != 0 {
		t.Fatal("constructing the model performed I/O")
	}
	if view := model.View().Content; !strings.Contains(view, "Loading Packy health") {
		t.Fatalf("loading view = %q", view)
	}

	load := model.Init()
	if load == nil {
		t.Fatal("Init did not schedule loading")
	}
	message := load()
	if backend.loads != 1 {
		t.Fatalf("backend loads = %d, want 1", backend.loads)
	}
	updated, _ := model.Update(message)
	view := updated.View().Content
	for _, want := range []string{"Packy health", "healthy", "packy-core", "PASS", "Packy core is available", "Workstation", "matty", "1.0.0"} {
		if !strings.Contains(view, want) {
			t.Fatalf("ready view missing %q:\n%s", want, view)
		}
	}
}
