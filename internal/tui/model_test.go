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
	dashboard       tui.Dashboard
	err             error
	loads           int
	initializations int
	initialize      func(func(string)) error
}

func (b *fakeBackend) Initialize(_ context.Context, progress func(string)) error {
	b.initializations++
	if b.initialize != nil {
		return b.initialize(progress)
	}
	return nil
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
	if !strings.Contains(view, "› matty") || strings.Contains(view, "Product review") {
		t.Fatalf("down navigation did not select the next Pack:\n%s", view)
	}
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(current.View().Content)
	for _, want := range []string{"Pack details", "Product review"} {
		if !strings.Contains(view, want) {
			t.Fatalf("Enter did not inspect the selected Pack; missing %q:\n%s", want, view)
		}
	}
	for _, line := range strings.Split(current.View().Content, "\n") {
		if width := ansi.StringWidth(line); width > 64 {
			t.Fatalf("narrow line width = %d, want <= 64: %q", width, ansi.Strip(line))
		}
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

func TestCatalogCanBeFilteredAndOpensCompletePackDetail(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy"},
		Global: tui.Scope{Available: true, Packs: []tui.Pack{
			{ID: "argote", Version: "1.2.0", Description: "Agent guidance"},
			{
				ID: "orchestrate", Version: "1.0.0", Description: "Coordination workflow",
				Requirements: []string{"git"},
				Resources:    []tui.Resource{{Identity: "skill:orchestrate", Description: "Coordinate agents", Role: "operational", Requirements: []string{"notice:mit"}, Conflicts: []string{"skill:legacy"}}},
				Exclusions: []tui.Exclusion{
					{ID: "windows", Reason: "POSIX shell required"},
					{ID: "skill:orchestrate", Surface: "claude", Mode: "unsupported", Code: "surface-unsupported", Reason: "Codex only"},
				},
				SurfaceStatuses: []tui.SurfaceStatus{
					{Name: "claude", Supported: false},
					{Name: "codex", Supported: true, Configured: "yes", Authorized: "yes", Usable: "no", Ownership: 2, Drift: 1, Blockers: []string{"runtime unavailable"}, PendingActions: []string{"install helper"}, Evidence: []string{"projection verified with a deliberately long host-owned fingerprint"}},
					{Name: "opencode", Supported: false},
				},
			},
		}},
	}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	current, _ = current.Update(tea.WindowSizeMsg{Width: 64, Height: 30})

	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}))
	for _, char := range "orch" {
		current, _ = current.Update(tea.KeyPressMsg(tea.Key{Text: string(char), Code: char}))
	}
	view := ansi.Strip(current.View().Content)
	if strings.Contains(view, "argote") || !strings.Contains(view, "orchestrate") || !strings.Contains(view, "Filter: orch") {
		t.Fatalf("catalog filter did not narrow the visible list:\n%s", view)
	}

	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(current.View().Content)
	for _, want := range []string{
		"Pack details · Workstation · global", "orchestrate 1.0.0", "Coordination workflow",
		"skill:orchestrate", "Coordinate agents", "requires notice:mit", "conflicts skill:legacy",
		"Requirements: git", "windows — POSIX shell required",
		"skill:orchestrate (surface=claude, mode=unsupported", "code=surface-unsupported) — Codex only",
		"claude: unsupported", "codex: supported", "configured=yes authorized=yes usable=no",
		"Ownership: 2 projected paths", "Drift: 1 projections", "runtime unavailable",
		"install helper", "projection verified", "opencode: unsupported",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("Pack detail missing %q:\n%s", want, view)
		}
	}
	for _, line := range strings.Split(current.View().Content, "\n") {
		if width := ansi.StringWidth(line); width > 64 {
			t.Fatalf("detail line width = %d, want <= 64: %q", width, ansi.Strip(line))
		}
	}
}

func TestScopeStatusCannotBeMistakenInPackDetail(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health:  tui.Health{Status: "healthy"},
		Global:  tui.Scope{Available: true, Packs: []tui.Pack{{ID: "matty", Version: "1.0.0"}}},
		Project: tui.Scope{Available: true, Root: "/workspace/project", Packs: []tui.Pack{{ID: "matty", Version: "1.0.0"}}},
	}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"Pack details · Current project", "/workspace/project"} {
		if !strings.Contains(view, want) {
			t.Fatalf("project detail missing scope marker %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Pack details · Workstation · global") {
		t.Fatalf("project detail was mislabeled as global:\n%s", view)
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

func TestReloadReflectsDynamicCatalogChanges(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy"},
		Global: tui.Scope{Available: true, Packs: []tui.Pack{{ID: "first", Version: "1.0.0"}}},
	}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())

	backend.dashboard.Global.Packs = []tui.Pack{{ID: "newly-reviewed", Version: "2.0.0"}}
	current, reload := current.Update(tea.KeyPressMsg(tea.Key{Text: "r", Code: 'r'}))
	current, _ = current.Update(reload())
	view := ansi.Strip(current.View().Content)
	if strings.Contains(view, "first") || !strings.Contains(view, "newly-reviewed") {
		t.Fatalf("reload retained a hard-coded catalog instead of backend data:\n%s", view)
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

func TestUninitializedDashboardRequiresExplicitFocusedInitialization(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy", Passes: 1},
		Setup: tui.Setup{
			InitializationAvailable: true,
			Blockers: []tui.SetupBlocker{{
				Cause:           "Installed Source is missing",
				AffectedActions: []string{"Pack catalog inspection", "Pack lifecycle actions"},
			}},
		},
	}}
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"healthy", "Installed Source is missing", "Affected actions: Pack catalog inspection, Pack lifecycle actions", "Initialize Packy", "selected", "Enter initialize"} {
		if !strings.Contains(view, want) {
			t.Fatalf("uninitialized dashboard missing %q:\n%s", want, view)
		}
	}
	if backend.initializations != 0 {
		t.Fatalf("entering the dashboard initialized Packy %d times", backend.initializations)
	}

	current, command := current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if command == nil || backend.initializations != 0 {
		t.Fatalf("focused initialization was not scheduled explicitly: command=%v initializations=%d", command != nil, backend.initializations)
	}
	if view := ansi.Strip(current.View().Content); !strings.Contains(view, "Initialization in progress") {
		t.Fatalf("initialization did not enter progress state:\n%s", view)
	}
	current, quit := current.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if quit != nil || !strings.Contains(ansi.Strip(current.View().Content), "Initialization in progress") {
		t.Fatal("active initialization allowed ordinary exit or stopped rendering progress")
	}
	current, _ = current.Update(tea.WindowSizeMsg{Width: 48, Height: 20})
	for _, line := range strings.Split(current.View().Content, "\n") {
		if width := ansi.StringWidth(line); width > 48 {
			t.Fatalf("active initialization froze responsive rendering: width=%d line=%q", width, ansi.Strip(line))
		}
	}
}

func TestSetupBlockersDisableOnlyAffectedActions(t *testing.T) {
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "warnings", Warnings: 1},
		Setup: tui.Setup{Blockers: []tui.SetupBlocker{{
			Cause:           "project status is unavailable",
			AffectedActions: []string{"Current-project status", "Project Pack lifecycle actions"},
		}}},
		Global: tui.Scope{Available: true, Packs: []tui.Pack{{ID: "argote", Version: "1.0.0", Description: "Agent guidance"}}},
	}}
	current := loadModel(t, backend)
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"project status is unavailable", "Affected actions: Current-project status, Project Pack lifecycle actions", "argote"} {
		if !strings.Contains(view, want) {
			t.Fatalf("blocked dashboard missing %q:\n%s", want, view)
		}
	}
	current, _ = current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if view := ansi.Strip(current.View().Content); !strings.Contains(view, "Pack details") || !strings.Contains(view, "Agent guidance") {
		t.Fatalf("setup blocker disabled unaffected global inspection:\n%s", view)
	}
}

func TestInitializationProgressResultReloadAndRetryAreRecoverable(t *testing.T) {
	attempt := 0
	backend := &fakeBackend{dashboard: tui.Dashboard{
		Health: tui.Health{Status: "healthy", Passes: 1},
		Setup:  tui.Setup{InitializationAvailable: true},
	}}
	backend.initialize = func(progress func(string)) error {
		attempt++
		progress("cloning Installed Source")
		if attempt == 1 {
			return errors.New("network unavailable")
		}
		backend.dashboard.Setup = tui.Setup{}
		backend.dashboard.Global = tui.Scope{Available: true, Packs: []tui.Pack{{ID: "argote", Version: "1.0.0"}}}
		return nil
	}

	current := loadModel(t, backend)
	current = runModelMessage(t, current, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view := ansi.Strip(current.View().Content)
	for _, want := range []string{"Initialization failed", "network unavailable", "cloning Installed Source", "Enter retry", "Esc dashboard"} {
		if !strings.Contains(view, want) {
			t.Fatalf("failed result missing %q:\n%s", want, view)
		}
	}
	if backend.loads != 2 {
		t.Fatalf("failed initialization loads = %d, want initial load plus fresh diagnosis", backend.loads)
	}

	current = runModelMessage(t, current, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(current.View().Content)
	for _, want := range []string{"Initialization succeeded", "cloning Installed Source", "Enter dashboard"} {
		if !strings.Contains(view, want) {
			t.Fatalf("successful result missing %q:\n%s", want, view)
		}
	}
	if backend.initializations != 2 || backend.loads != 3 {
		t.Fatalf("retry/reload counts = initializations %d, loads %d; want 2 and 3", backend.initializations, backend.loads)
	}

	updated, _ := current.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	view = ansi.Strip(updated.View().Content)
	if !strings.Contains(view, "argote") || strings.Contains(view, "Initialization succeeded") {
		t.Fatalf("successful result did not continue to reloaded dashboard:\n%s", view)
	}
}

func loadModel(t *testing.T, backend *fakeBackend) tea.Model {
	t.Helper()
	model := tui.NewModel(backend)
	current, _ := model.Update(model.Init()())
	return current
}

func runModelMessage(t *testing.T, model tea.Model, message tea.Msg) tea.Model {
	t.Helper()
	current, command := model.Update(message)
	return runModelCommand(t, current, command)
}

func runModelCommand(t *testing.T, model tea.Model, command tea.Cmd) tea.Model {
	t.Helper()
	if command == nil {
		t.Fatal("expected command")
	}
	queue := []tea.Cmd{command}
	current := model
	for len(queue) > 0 {
		command, queue = queue[0], queue[1:]
		message := command()
		if batch, ok := message.(tea.BatchMsg); ok {
			queue = append(queue, []tea.Cmd(batch)...)
			continue
		}
		if message == nil {
			continue
		}
		var next tea.Cmd
		current, next = current.Update(message)
		if next != nil {
			queue = append(queue, next)
		}
	}
	return current
}
