package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// Catppuccin Mocha: https://github.com/catppuccin/catppuccin
var (
	mochaBase     = lipgloss.Color("#1e1e2e")
	mochaMantle   = lipgloss.Color("#181825")
	mochaSurface0 = lipgloss.Color("#313244")
	mochaSurface1 = lipgloss.Color("#45475a")
	mochaOverlay1 = lipgloss.Color("#7f849c")
	mochaSubtext0 = lipgloss.Color("#a6adc8")
	mochaText     = lipgloss.Color("#cdd6f4")
	mochaBlue     = lipgloss.Color("#89b4fa")
	mochaSapphire = lipgloss.Color("#74c7ec")
	mochaGreen    = lipgloss.Color("#a6e3a1")
	mochaYellow   = lipgloss.Color("#f9e2af")
	mochaRed      = lipgloss.Color("#f38ba8")
	mochaMauve    = lipgloss.Color("#cba6f7")
	mochaLavender = lipgloss.Color("#b4befe")

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(mochaMauve)
	bodyStyle  = lipgloss.NewStyle().Foreground(mochaText).Background(mochaBase)
	mutedStyle = lipgloss.NewStyle().Foreground(mochaSubtext0)
	dimStyle   = lipgloss.NewStyle().Foreground(mochaOverlay1)

	brandStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(mochaMantle).
			Background(mochaMauve).
			Padding(0, 1)
	sectionTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(mochaLavender)
	selectedRowStyle  = lipgloss.NewStyle().Bold(true).Foreground(mochaBase).Background(mochaBlue)
	versionStyle      = lipgloss.NewStyle().Foreground(mochaOverlay1)
)

func newHelpModel() help.Model {
	model := help.New()
	model.ShortSeparator = "  •  "
	model.FullSeparator = "    "
	model.Styles = help.Styles{
		Ellipsis:       dimStyle,
		ShortKey:       lipgloss.NewStyle().Bold(true).Foreground(mochaBlue),
		ShortDesc:      mutedStyle,
		ShortSeparator: lipgloss.NewStyle().Foreground(mochaSurface1),
		FullKey:        lipgloss.NewStyle().Bold(true).Foreground(mochaBlue),
		FullDesc:       mutedStyle,
		FullSeparator:  lipgloss.NewStyle().Foreground(mochaSurface1),
	}
	return model
}

func panel(content string, width int, focused bool) string {
	border := mochaSurface1
	if focused {
		border = mochaBlue
	}
	return lipgloss.NewStyle().
		Width(max(width-4, 1)).
		Padding(0, 1).
		Foreground(mochaText).
		Background(mochaSurface0).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Render(content)
}

func statusBadge(status string) string {
	label, foreground, background := "[INFO] "+status, mochaBase, mochaSapphire
	switch strings.ToLower(status) {
	case "healthy", "pass", "verified", "active", "supported":
		label, background = "[OK] "+status, mochaGreen
	case "warnings", "warn", "warning", "unknown", "unsupported":
		label, background = "[WARN] "+status, mochaYellow
	case "failures", "fail", "failed", "blocked", "unhealthy":
		label, background = "[FAIL] "+status, mochaRed
	}
	return lipgloss.NewStyle().Bold(true).Foreground(foreground).Background(background).Padding(0, 1).Render(label)
}

func sectionHeading(title string, count int) string {
	label := "◆ " + title
	if count > 0 {
		label += fmt.Sprintf("  %d", count)
	}
	return sectionTitleStyle.Render(label)
}

func roleBadge(role string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(mochaSapphire).Render("[" + role + "]")
}

func (m Model) renderViewportScreen(title, subtitle, content, footer string, offset int) string {
	return m.renderFramedScreen(title, subtitle, content, footer, offset, "↑/↓ scroll")
}

func (m Model) renderPagedScreen(title, subtitle, content, footer string) string {
	return m.renderFramedScreen(title, subtitle, content, footer, m.pagedScreenScroll, "PgUp/PgDn scroll")
}

func (m Model) renderFramedScreen(title, subtitle, content, footer string, offset int, scrollHint string) string {
	width := m.width
	if width <= 0 {
		width = 100
	}
	contentWidth := max(width-4, 44)
	lines := m.viewportLines(content)
	visible := m.viewportVisibleLines()
	maxOffset := max(len(lines)-visible, 0)
	offset = min(max(offset, 0), maxOffset)
	end := min(offset+visible, len(lines))
	visibleContent := strings.Join(lines[offset:end], "\n")
	if visibleContent == "" {
		visibleContent = " "
	}
	if maxOffset > 0 {
		footer += fmt.Sprintf("  ·  %s  ·  lines %d–%d of %d", scrollHint, offset+1, end, len(lines))
	}
	header := titleStyle.Render(title)
	if subtitle != "" {
		header += "\n" + mutedStyle.Render(subtitle)
	}
	framed := strings.Join([]string{
		header,
		"",
		panel(visibleContent, contentWidth, false),
		"",
		lipgloss.NewStyle().Width(contentWidth).Foreground(mochaSubtext0).Render(footer),
	}, "\n")
	return m.renderBody(lipgloss.NewStyle().Padding(0, 2).Render(framed))
}

func (m Model) viewportMaxOffset(content string) int {
	return max(len(m.viewportLines(content))-m.viewportVisibleLines(), 0)
}

func (m Model) viewportLines(content string) []string {
	width := m.width
	if width <= 0 {
		width = 100
	}
	// panel width includes its two padding cells and border, leaving ten cells
	// less than the terminal for actual text after the screen margins.
	innerWidth := max(width-10, 38)
	wrapped := lipgloss.NewStyle().Width(innerWidth).Render(content)
	return strings.Split(wrapped, "\n")
}

func (m Model) viewportVisibleLines() int {
	if m.height <= 0 {
		return int(^uint(0) >> 1)
	}
	// Reserve two footer rows so contextual help remains within narrow screens
	// when its scroll position wraps.
	return max(m.height-9, 1)
}

func severityMarker(severity string) string {
	label, color := "[INFO]", mochaSapphire
	switch strings.ToUpper(severity) {
	case "PASS":
		label, color = "[OK]", mochaGreen
	case "WARN", "WARNING":
		label, color = "[WARN]", mochaYellow
	case "FAIL", "FAILURE":
		label, color = "[FAIL]", mochaRed
	}
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(label)
}

func metric(value int, label string, foreground color.Color) string {
	return lipgloss.NewStyle().Bold(true).Foreground(foreground).Render(fmt.Sprintf("%d", value)) + " " + mutedStyle.Render(label)
}

func dashboardHelpBindings() []key.Binding {
	return []key.Binding{
		dashboardKeys.Up,
		dashboardKeys.NextScope,
		dashboardKeys.Inspect,
		dashboardKeys.Filter,
		dashboardKeys.Help,
		dashboardKeys.Reload,
		dashboardKeys.Quit,
	}
}

func setupHelpBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("Enter", "initialize")),
		dashboardKeys.Help,
		dashboardKeys.Reload,
		dashboardKeys.Quit,
	}
}
