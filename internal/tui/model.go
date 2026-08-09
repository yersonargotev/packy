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
// state and explicitly initializes Packy's Installed Source.
type Backend interface {
	Load(context.Context) (Dashboard, error)
	Initialize(context.Context, func(string)) error
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
	ID              string
	Version         string
	Description     string
	Surfaces        []string
	Requirements    []string
	Resources       []Resource
	Exclusions      []Exclusion
	SurfaceStatuses []SurfaceStatus
}

type Resource struct {
	Identity     string
	Description  string
	Role         string
	Requirements []string
	Conflicts    []string
}

type Exclusion struct {
	ID      string
	Surface string
	Mode    string
	Code    string
	Reason  string
}

type SurfaceStatus struct {
	Name           string
	Supported      bool
	Configured     string
	Authorized     string
	Usable         string
	Ownership      int
	Drift          int
	Blockers       []string
	PendingActions []string
	Evidence       []string
}

type Scope struct {
	Available bool
	Root      string
	Packs     []Pack
}

type Dashboard struct {
	Health  Health
	Setup   Setup
	Global  Scope
	Project Scope
}

type Setup struct {
	Initialized             bool
	InitializationAvailable bool
	Blockers                []SetupBlocker
}

type SetupBlocker struct {
	Cause           string
	AffectedActions []string
}

type loadResult struct {
	dashboard Dashboard
	err       error
}

type initializationProgress struct {
	detail string
}

type initializationFinished struct {
	err error
}

type Model struct {
	backend                Backend
	ctx                    context.Context
	dashboard              Dashboard
	err                    error
	loaded                 bool
	project                bool
	globalRow              int
	projectRow             int
	width                  int
	showHelp               bool
	inspecting             bool
	filtering              bool
	filter                 string
	initializing           bool
	initializationResult   bool
	initializationErr      error
	initializationProgress []string
	initializationEvents   chan tea.Msg
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
	case initializationProgress:
		m.initializationProgress = append(m.initializationProgress, message.detail)
		return m, waitForInitialization(m.initializationEvents)
	case initializationFinished:
		m.initializing = false
		m.initializationResult = true
		m.initializationErr = message.err
		m.initializationEvents = nil
		return m, m.Init()
	case tea.KeyPressMsg:
		if m.initializing {
			return m, nil
		}
		if m.initializationResult {
			switch {
			case key.Matches(message, dashboardKeys.Quit):
				return m, tea.Quit
			case key.Matches(message, dashboardKeys.Back):
				m.initializationResult = false
				return m, nil
			case key.Matches(message, dashboardKeys.Inspect):
				if m.initializationErr != nil && m.dashboard.Setup.InitializationAvailable {
					return m.startInitialization()
				}
				m.initializationResult = false
				return m, nil
			}
			return m, nil
		}
		if m.filtering {
			switch message.Code {
			case tea.KeyEnter:
				m.filtering = false
			case tea.KeyEscape:
				m.filtering, m.filter = false, ""
			case tea.KeyBackspace:
				characters := []rune(m.filter)
				if len(characters) > 0 {
					m.filter = string(characters[:len(characters)-1])
				}
			default:
				if message.Text != "" {
					m.filter += message.Text
				}
			}
			m.globalRow = boundedRow(m.globalRow, len(filteredPacks(m.dashboard.Global.Packs, m.filter)))
			m.projectRow = boundedRow(m.projectRow, len(filteredPacks(m.dashboard.Project.Packs, m.filter)))
			return m, nil
		}
		switch {
		case key.Matches(message, dashboardKeys.Quit):
			return m, tea.Quit
		case key.Matches(message, dashboardKeys.Reload):
			m.loaded, m.err, m.inspecting = false, nil, false
			return m, m.Init()
		case key.Matches(message, dashboardKeys.Help):
			m.showHelp = !m.showHelp
		case key.Matches(message, dashboardKeys.Filter):
			m.filtering, m.inspecting = true, false
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
			if m.dashboard.Setup.InitializationAvailable {
				return m.startInitialization()
			}
			m.inspecting = m.selectedPack() != nil
		case key.Matches(message, dashboardKeys.Down):
			m.inspecting = false
			if m.project {
				m.projectRow = nextRow(m.projectRow, len(filteredPacks(m.dashboard.Project.Packs, m.filter)), 1)
			} else {
				m.globalRow = nextRow(m.globalRow, len(filteredPacks(m.dashboard.Global.Packs, m.filter)), 1)
			}
		case key.Matches(message, dashboardKeys.Up):
			m.inspecting = false
			if m.project {
				m.projectRow = nextRow(m.projectRow, len(filteredPacks(m.dashboard.Project.Packs, m.filter)), -1)
			} else {
				m.globalRow = nextRow(m.globalRow, len(filteredPacks(m.dashboard.Global.Packs, m.filter)), -1)
			}
		}
	}
	return m, nil
}

func (m Model) startInitialization() (tea.Model, tea.Cmd) {
	m.initializing = true
	m.initializationResult = false
	m.initializationErr = nil
	m.initializationProgress = nil
	m.initializationEvents = make(chan tea.Msg, 64)
	events := m.initializationEvents
	initialize := func() tea.Msg {
		err := m.backend.Initialize(m.ctx, func(detail string) {
			events <- initializationProgress{detail: detail}
		})
		events <- initializationFinished{err: err}
		close(events)
		return nil
	}
	return m, tea.Batch(initialize, waitForInitialization(events))
}

func waitForInitialization(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

func (m Model) selectedPack() *Pack {
	packs, row := m.dashboard.Global.Packs, m.globalRow
	if m.project {
		packs, row = m.dashboard.Project.Packs, m.projectRow
	}
	packs = filteredPacks(packs, m.filter)
	if row < 0 || row >= len(packs) {
		return nil
	}
	pack := packs[row]
	return &pack
}

func filteredPacks(packs []Pack, filter string) []Pack {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return packs
	}
	result := make([]Pack, 0, len(packs))
	for _, pack := range packs {
		if strings.Contains(strings.ToLower(pack.ID+" "+pack.Description), filter) {
			result = append(result, pack)
		}
	}
	return result
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
	Up, Down, NextScope, PreviousScope, Inspect, Back, Filter, Help, Reload, Quit key.Binding
}

var dashboardKeys = keyMap{
	Up:            key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:          key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	NextScope:     key.NewBinding(key.WithKeys("tab", "right"), key.WithHelp("tab/→", "next scope")),
	PreviousScope: key.NewBinding(key.WithKeys("shift+tab", "left"), key.WithHelp("shift+tab/←", "previous scope")),
	Inspect:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "inspect")),
	Back:          key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Filter:        key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
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
	if m.initializing {
		return m.renderBody(m.renderInitializationProgress())
	}
	if m.initializationResult {
		return m.renderBody(m.renderInitializationResult())
	}
	if m.inspecting {
		return m.renderBody(m.renderDetail())
	}

	health := fmt.Sprintf("%s · %d pass · %d warnings · %d failures", m.dashboard.Health.Status, m.dashboard.Health.Passes, m.dashboard.Health.Warnings, m.dashboard.Health.Failures)
	if m.dashboard.Health.Status == "healthy" {
		health = goodStyle.Render(health)
	}
	healthLines := []string{health}
	for _, check := range m.dashboard.Health.Checks {
		healthLines = append(healthLines, fmt.Sprintf("  %s  %s — %s", check.Severity, check.Name, check.Detail))
	}
	globalEmpty := "No reviewed Packs are available"
	projectEmpty := "No reviewed Packs are available in this project scope"
	if strings.TrimSpace(m.filter) != "" {
		globalEmpty, projectEmpty = "No reviewed Packs match the filter", "No reviewed Packs match the filter"
	}
	global := m.renderScope("Workstation · global", filteredPacks(m.dashboard.Global.Packs, m.filter), m.globalRow, !m.project, globalEmpty)
	project := "Current project\nNo Git project\nGlobal inspection remains available"
	if m.dashboard.Project.Available {
		project = m.renderScope("Current project", filteredPacks(m.dashboard.Project.Packs, m.filter), m.projectRow, m.project, projectEmpty) + "\n" + m.dashboard.Project.Root
	}
	scopes := global + "\n\n" + project
	if m.width >= 96 {
		scopes = lipgloss.JoinHorizontal(lipgloss.Top, global, "    ", project)
	}
	filter := ""
	if m.filtering || m.filter != "" {
		filter = "Filter: " + m.filter
		if m.filtering {
			filter += "_"
		}
	}
	help := "↑/k ↓/j navigate · tab switch scope · / filter · enter inspect · ? help · r reload · q quit"
	setup := m.renderSetup()
	if setup != "" {
		help = "Enter initialize · ? help · r reload · q quit"
	}
	if m.showHelp {
		help = "arrows/j/k navigate · Tab/Shift+Tab switch scope · Enter inspect · Esc back · ? hide help · r reload · q quit · Ctrl+C quit"
	}
	return m.renderBody(strings.Join([]string{
		titleStyle.Render("Packy health"),
		strings.Join(healthLines, "\n"),
		setup,
		"",
		scopes,
		filter,
		"",
		help,
	}, "\n"))
}

func (m Model) renderSetup() string {
	if !m.dashboard.Setup.InitializationAvailable && len(m.dashboard.Setup.Blockers) == 0 {
		return ""
	}
	lines := []string{"", "Setup"}
	for _, blocker := range m.dashboard.Setup.Blockers {
		lines = append(lines, "  Blocked: "+blocker.Cause)
		if len(blocker.AffectedActions) > 0 {
			lines = append(lines, "  Affected actions: "+strings.Join(blocker.AffectedActions, ", "))
		}
	}
	if m.dashboard.Setup.InitializationAvailable {
		lines = append(lines, "", "› [ Initialize Packy ] · selected")
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderInitializationProgress() string {
	lines := []string{titleStyle.Render("Packy initialization"), "", "Initialization in progress"}
	if len(m.initializationProgress) == 0 {
		lines = append(lines, "Preparing Installed Source…")
	} else {
		for _, detail := range m.initializationProgress {
			lines = append(lines, "  "+detail)
		}
	}
	lines = append(lines, "", "Packy will return when initialization finishes")
	return strings.Join(lines, "\n")
}

func (m Model) renderInitializationResult() string {
	status := "Initialization succeeded"
	action := "Enter dashboard"
	if m.initializationErr != nil {
		status = "Initialization failed"
		action = "Enter retry"
	}
	lines := []string{titleStyle.Render("Packy initialization"), "", status}
	for _, detail := range m.initializationProgress {
		lines = append(lines, "  "+detail)
	}
	if m.initializationErr != nil {
		lines = append(lines, "", m.initializationErr.Error())
	}
	lines = append(lines, "", action+" · Esc dashboard · q quit")
	return strings.Join(lines, "\n")
}

func (m Model) renderBody(content string) string {
	style := bodyStyle
	if m.width > 0 {
		style = style.Width(m.width)
	}
	return style.Render(content)
}

func (m Model) renderScope(title string, packs []Pack, row int, selected bool, empty string) string {
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
	return strings.Join(lines, "\n")
}

func (m Model) renderDetail() string {
	pack := m.selectedPack()
	if pack == nil {
		return titleStyle.Render("Pack details") + "\n\nNo Pack selected\n\nEsc back"
	}
	scope := "Workstation · global"
	root := ""
	if m.project {
		scope = "Current project"
		root = "\nProject root: " + m.dashboard.Project.Root
	}
	lines := []string{
		titleStyle.Render("Pack details · " + scope),
		pack.ID + " " + pack.Version,
		pack.Description + root,
		"Requirements: " + joinOrNone(pack.Requirements),
		"",
		"Resources",
	}
	if len(pack.Resources) == 0 {
		lines = append(lines, "  none")
	}
	for _, resource := range pack.Resources {
		line := "  " + resource.Identity + " [" + resource.Role + "]"
		if resource.Description != "" {
			line += " — " + resource.Description
		}
		lines = append(lines, line)
		if len(resource.Requirements) > 0 {
			lines = append(lines, "    requires "+strings.Join(resource.Requirements, ", "))
		}
		if len(resource.Conflicts) > 0 {
			lines = append(lines, "    conflicts "+strings.Join(resource.Conflicts, ", "))
		}
	}
	lines = append(lines, "", "Conflicts and exclusions")
	if len(pack.Exclusions) == 0 {
		lines = append(lines, "  none")
	}
	for _, exclusion := range pack.Exclusions {
		identity := exclusion.ID
		qualifiers := make([]string, 0, 3)
		if exclusion.Surface != "" {
			qualifiers = append(qualifiers, "surface="+exclusion.Surface)
		}
		if exclusion.Mode != "" {
			qualifiers = append(qualifiers, "mode="+exclusion.Mode)
		}
		if exclusion.Code != "" {
			qualifiers = append(qualifiers, "code="+exclusion.Code)
		}
		if len(qualifiers) > 0 {
			identity += " (" + strings.Join(qualifiers, ", ") + ")"
		}
		lines = append(lines, "  "+identity+" — "+exclusion.Reason)
	}
	lines = append(lines, "", "CLI surfaces and status")
	if len(pack.SurfaceStatuses) == 0 && len(pack.Surfaces) > 0 {
		lines = append(lines, "  Available on: "+strings.Join(pack.Surfaces, ", "))
	}
	for _, status := range pack.SurfaceStatuses {
		if !status.Supported {
			lines = append(lines, "  "+status.Name+": unsupported")
			continue
		}
		lines = append(lines,
			"  "+status.Name+": supported",
			fmt.Sprintf("    configured=%s authorized=%s usable=%s", status.Configured, status.Authorized, status.Usable),
			fmt.Sprintf("    Ownership: %d projected paths", status.Ownership),
			fmt.Sprintf("    Drift: %d projections", status.Drift),
			"    Blockers: "+joinOrNone(status.Blockers),
			"    Pending actions: "+joinOrNone(status.PendingActions),
			"    Evidence: "+joinOrNone(status.Evidence),
		)
	}
	lines = append(lines, "", "Esc back · / filter · r reload · q quit")
	return strings.Join(lines, "\n")
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
