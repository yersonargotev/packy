// Package tui presents Packy's read-only application state in a terminal UI.
package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Backend is the presentation-neutral seam through which the TUI loads Packy
// state. Implementations must not mutate Pack state.
type Backend interface {
	Load(context.Context) (Dashboard, error)
}

type Health struct {
	Status   string
	Passes   int
	Warnings int
	Failures int
	Checks   []HealthCheck
}

type HealthCheck struct {
	Name     string
	Severity string
	Detail   string
}

type Pack struct {
	ID          string
	Version     string
	Description string
	Surfaces    []string
}

type Scope struct {
	Available bool
	Root      string
	Packs     []Pack
}

type Dashboard struct {
	Health  Health
	Global  Scope
	Project Scope
}

type loadResult struct {
	dashboard Dashboard
	err       error
}

type Model struct {
	backend    Backend
	ctx        context.Context
	dashboard  Dashboard
	err        error
	loaded     bool
	project    bool
	globalRow  int
	projectRow int
	width      int
	showHelp   bool
	inspecting bool
}

func NewModel(backend Backend) Model {
	return newModel(context.Background(), backend)
}

func newModel(ctx context.Context, backend Backend) Model {
	return Model{backend: backend, ctx: ctx}
}

// Run executes the full-screen dashboard with caller-owned process I/O.
func Run(ctx context.Context, backend Backend, input io.Reader, output io.Writer) error {
	program := tea.NewProgram(newModel(ctx, backend), tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output))
	_, err := program.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		dashboard, err := m.backend.Load(m.ctx)
		return loadResult{dashboard: dashboard, err: err}
	}
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case loadResult:
		m.loaded = true
		m.dashboard = message.dashboard
		m.err = message.err
		m.globalRow = boundedRow(m.globalRow, len(m.dashboard.Global.Packs))
		m.projectRow = boundedRow(m.projectRow, len(m.dashboard.Project.Packs))
	case tea.WindowSizeMsg:
		m.width = message.Width
	case tea.KeyPressMsg:
		switch {
		case key.Matches(message, dashboardKeys.Quit):
			return m, tea.Quit
		case key.Matches(message, dashboardKeys.Reload):
			m.loaded, m.err = false, nil
			return m, m.Init()
		case key.Matches(message, dashboardKeys.Help):
			m.showHelp = !m.showHelp
		case key.Matches(message, dashboardKeys.NextScope):
			if m.dashboard.Project.Available {
				m.project = true
				m.inspecting = false
			}
		case key.Matches(message, dashboardKeys.PreviousScope):
			m.project = false
			m.inspecting = false
		case key.Matches(message, dashboardKeys.Back):
			if m.inspecting {
				m.inspecting = false
			} else {
				m.project = false
			}
		case key.Matches(message, dashboardKeys.Inspect):
			m.inspecting = m.selectedPack() != nil
		case key.Matches(message, dashboardKeys.Down):
			m.inspecting = false
			if m.project {
				m.projectRow = nextRow(m.projectRow, len(m.dashboard.Project.Packs), 1)
			} else {
				m.globalRow = nextRow(m.globalRow, len(m.dashboard.Global.Packs), 1)
			}
		case key.Matches(message, dashboardKeys.Up):
			m.inspecting = false
			if m.project {
				m.projectRow = nextRow(m.projectRow, len(m.dashboard.Project.Packs), -1)
			} else {
				m.globalRow = nextRow(m.globalRow, len(m.dashboard.Global.Packs), -1)
			}
		}
	}
	return m, nil
}

func (m Model) selectedPack() *Pack {
	packs, row := m.dashboard.Global.Packs, m.globalRow
	if m.project {
		packs, row = m.dashboard.Project.Packs, m.projectRow
	}
	if row < 0 || row >= len(packs) {
		return nil
	}
	pack := packs[row]
	return &pack
}

func boundedRow(row, length int) int {
	if length == 0 || row >= length {
		return 0
	}
	return row
}

func nextRow(row, length, direction int) int {
	if length == 0 {
		return 0
	}
	return (row + direction + length) % length
}

type keyMap struct {
	Up, Down, NextScope, PreviousScope, Inspect, Back, Help, Reload, Quit key.Binding
}

var dashboardKeys = keyMap{
	Up:            key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:          key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	NextScope:     key.NewBinding(key.WithKeys("tab", "right"), key.WithHelp("tab/→", "next scope")),
	PreviousScope: key.NewBinding(key.WithKeys("shift+tab", "left"), key.WithHelp("shift+tab/←", "previous scope")),
	Inspect:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "inspect")),
	Back:          key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Help:          key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Reload:        key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
	Quit:          key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	view.WindowTitle = "Packy"
	return view
}

var (
	mochaBase  = lipgloss.Color("#1e1e2e")
	mochaText  = lipgloss.Color("#cdd6f4")
	mochaBlue  = lipgloss.Color("#89b4fa")
	mochaGreen = lipgloss.Color("#a6e3a1")

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(mochaBlue)
	goodStyle  = lipgloss.NewStyle().Bold(true).Foreground(mochaGreen)
	bodyStyle  = lipgloss.NewStyle().Foreground(mochaText).Background(mochaBase)
)

func (m Model) render() string {
	if !m.loaded {
		return m.renderBody(titleStyle.Render("Packy health") + "\n\nLoading Packy health…")
	}
	if m.err != nil {
		return m.renderBody(titleStyle.Render("Packy health") + "\n\nUnable to load dashboard\n" + m.err.Error() + "\n\nr reload · q quit")
	}

	health := fmt.Sprintf("%s · %d pass · %d warnings · %d failures", m.dashboard.Health.Status, m.dashboard.Health.Passes, m.dashboard.Health.Warnings, m.dashboard.Health.Failures)
	if m.dashboard.Health.Status == "healthy" {
		health = goodStyle.Render(health)
	}
	healthLines := []string{health}
	for _, check := range m.dashboard.Health.Checks {
		healthLines = append(healthLines, fmt.Sprintf("  %s  %s — %s", check.Severity, check.Name, check.Detail))
	}
	global := m.renderScope("Workstation · global", m.dashboard.Global.Packs, m.globalRow, !m.project, !m.project && m.inspecting, "No reviewed Packs are available")
	project := "Current project\nNo Git project\nGlobal inspection remains available"
	if m.dashboard.Project.Available {
		project = m.renderScope("Current project", m.dashboard.Project.Packs, m.projectRow, m.project, m.project && m.inspecting, "No Packs are installed in this project") + "\n" + m.dashboard.Project.Root
	}
	scopes := global + "\n\n" + project
	if m.width >= 96 {
		scopes = lipgloss.JoinHorizontal(lipgloss.Top, global, "    ", project)
	}
	help := "↑/k ↓/j navigate · tab switch scope · enter inspect · ? help · r reload · q quit"
	if m.showHelp {
		help = "arrows/j/k navigate · Tab/Shift+Tab switch scope · Enter inspect · Esc back · ? hide help · r reload · q quit · Ctrl+C quit"
	}
	return m.renderBody(strings.Join([]string{
		titleStyle.Render("Packy health"),
		strings.Join(healthLines, "\n"),
		"",
		scopes,
		"",
		help,
	}, "\n"))
}

func (m Model) renderBody(content string) string {
	style := bodyStyle
	if m.width > 0 {
		style = style.Width(m.width)
	}
	return style.Render(content)
}

func (m Model) renderScope(title string, packs []Pack, row int, selected, inspecting bool, empty string) string {
	if selected {
		title += " · selected"
	}
	lines := []string{title}
	if len(packs) == 0 {
		return strings.Join(append(lines, "  "+empty), "\n")
	}
	for index, pack := range packs {
		marker := "  "
		if selected && index == row {
			marker = "› "
		}
		lines = append(lines, fmt.Sprintf("%s%s  %s", marker, pack.ID, pack.Version))
	}
	if selected && inspecting && row < len(packs) {
		lines = append(lines, "", "Pack details")
		if packs[row].Description != "" {
			lines = append(lines, "  "+packs[row].Description)
		}
		if len(packs[row].Surfaces) != 0 {
			lines = append(lines, "  Available on: "+strings.Join(packs[row].Surfaces, ", "))
		}
	}
	return strings.Join(lines, "\n")
}
