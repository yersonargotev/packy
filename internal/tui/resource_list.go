package tui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/paginator"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type resourceItem struct{ Resource }

func (item resourceItem) FilterValue() string {
	return item.Identity + " " + item.Description
}

type resourceDelegate struct {
	selected, initial map[string]bool
	focused, compact  bool
}

func (d resourceDelegate) Height() int {
	if d.compact {
		return 1
	}
	return 2
}

func (d resourceDelegate) Spacing() int                        { return 0 }
func (d resourceDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d resourceDelegate) Render(writer io.Writer, model list.Model, index int, value list.Item) {
	item := value.(resourceItem)
	enabled := d.selected[item.Identity]
	marker, state := "[ ]", "off"
	style := mutedStyle.Background(mochaMantle)
	if enabled {
		marker, state, style = "[x]", "on", resourceEnabledStyle.Background(mochaMantle)
	}
	if !selectableResource(item.Resource) {
		marker, state, style = " · ", "auto · off", dimStyle.Background(mochaMantle)
		if enabled {
			state = "auto · on"
		}
	} else if enabled != d.initial[item.Identity] {
		state, style = "+ enable", resourcePendingStyle.Background(mochaMantle)
		if !enabled {
			state = "− disable"
		}
	}
	focus := "  "
	if d.focused && index == model.Index() {
		focus, style = "› ", selectedRowStyle
	}
	width := model.Width()
	title := focus + marker + " " + item.Identity
	stateWidth := lipgloss.Width(state) + 2
	title = ansi.Truncate(title, max(width-stateWidth, 1), "…")
	row := title + strings.Repeat(" ", max(width-lipgloss.Width(title)-lipgloss.Width(state), 1)) + state
	fmt.Fprint(writer, style.Width(width).Render(row))
	if !d.compact {
		description := item.Description
		if !selectableResource(item.Resource) {
			description = "Included automatically with the resources that need it."
		}
		if description == "" {
			description = "Space to select · changes are staged until you apply."
		}
		descriptionStyle := dimStyle.Background(mochaMantle)
		if d.focused && index == model.Index() {
			descriptionStyle = resourceFocusedDescriptionStyle
		}
		fmt.Fprint(writer, "\n", descriptionStyle.Width(width).Render(ansi.Truncate("      "+description, width, "…")))
	}
}

func (m *Model) beginSelection() {
	m.selecting = true
	m.selectionPreviewFocus = false
	m.selectionNotice = ""
	m.selectedRoots = make(map[string]bool)
	pack := m.selectedPack()
	for _, resource := range operationalRoots(*pack) {
		m.selectedRoots[resource.Identity] = true
	}
	if m.operation == "configure" {
		if status := m.selectedSurfaceStatus(); status != nil && status.Selection.Mode == "custom" {
			m.selectedRoots = make(map[string]bool)
			for _, identity := range status.Selection.Roots {
				m.selectedRoots[identity] = true
			}
		}
	}
	m.initialSelected = make(map[string]bool)
	if m.operation == "configure" {
		for _, resource := range pack.Resources {
			m.initialSelected[resource.Identity] = m.resourceSelected(resource.Identity)
		}
	}
	items := make([]list.Item, 0, len(pack.Resources))
	// Keep the operational choices first; supporting content stays discoverable.
	for _, selectable := range []bool{true, false} {
		for _, resource := range pack.Resources {
			if selectableResource(resource) == selectable {
				items = append(items, resourceItem{resource})
			}
		}
	}
	m.resourceList = list.New(items, resourceDelegate{}, 80, 20)
	m.resourceList.SetShowTitle(false)
	m.resourceList.SetShowStatusBar(true)
	m.resourceList.SetStatusBarItemName("resource", "resources")
	m.resourceList.SetShowHelp(false)
	m.resourceList.DisableQuitKeybindings()
	m.resourceList.InfiniteScrolling = true
	m.resourceList.KeyMap.PrevPage.SetKeys("pgup")
	m.resourceList.KeyMap.NextPage.SetKeys("pgdown")
	m.resourceList.Paginator.Type = paginator.Arabic
	m.resourceList.Paginator.ArabicFormat = "Page %d / %d"
	m.resourceList.FilterInput.Prompt = "Search resources: "
	m.resourceList.Styles.TitleBar = lipgloss.NewStyle()
	m.resourceList.Styles.StatusBar = mutedStyle.Background(mochaMantle)
	m.resourceList.Styles.StatusEmpty = dimStyle
	m.resourceList.Styles.StatusBarActiveFilter = resourceEnabledStyle
	m.resourceList.Styles.StatusBarFilterCount = mutedStyle
	m.resourceList.Styles.NoItems = mutedStyle
	m.resourceList.Styles.PaginationStyle = dimStyle.Background(mochaMantle)
	m.resourceList.Styles.ArabicPagination = dimStyle
	m.syncResourceList()
}

func (m Model) updateSelection(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if message.String() == "ctrl+c" {
		return m.quit()
	}
	if m.resourceList.SettingFilter() {
		return m.updateResourceList(message)
	}
	if key.Matches(message, dashboardKeys.Quit) {
		return m.quit()
	}
	if key.Matches(message, dashboardKeys.Back) {
		if m.resourceList.IsFiltered() {
			return m.updateResourceList(message)
		}
		m.selecting = false
		return m, nil
	}
	pack := m.selectedPack()
	if pack == nil {
		return m, nil
	}
	surfaces := supportedSurfaces(*pack)
	if (message.Code == tea.KeyRight || message.Code == tea.KeyLeft) && len(surfaces) > 0 {
		direction := 1
		if message.Code == tea.KeyLeft {
			direction = -1
		}
		m.surfaceIndex = nextRow(m.surfaceIndex, len(surfaces), direction)
		if m.project {
			m.operation = projectLifecycleOperation(m.selectedSurfaceStatus())
			m.selecting, m.choosingAction, m.actionChoice = false, true, 0
		} else if status := m.selectedSurfaceStatus(); status != nil && status.Active {
			m.selecting, m.choosingAction, m.actionChoice = false, true, 0
			m.operation = firstLifecycleAction(*status)
		} else {
			m.operation = "activate"
			m.beginSelection()
		}
		return m, nil
	}
	if message.Code == tea.KeyTab {
		m.selectionPreviewFocus = !m.selectionPreviewFocus
		m.syncResourceList()
		return m, nil
	}
	if m.selectionPreviewFocus {
		if key.Matches(message, dashboardKeys.Inspect) {
			return m.startPreview()
		}
		return m, nil
	}
	switch {
	case message.Text == "a":
		for _, resource := range operationalRoots(*pack) {
			m.selectedRoots[resource.Identity] = true
		}
		m.selectionNotice = "All resources selected. Review before applying."
	case message.Text == "n":
		m.selectedRoots = make(map[string]bool)
		m.selectionNotice = "All resources deselected."
	case message.Text == " " || key.Matches(message, dashboardKeys.Inspect):
		if item, ok := m.resourceList.SelectedItem().(resourceItem); ok {
			if selectableResource(item.Resource) {
				m.toggleResource(item.Identity)
			} else {
				m.selectionNotice = "Supporting files and legal notices follow their resources automatically."
			}
		}
	default:
		return m.updateResourceList(message)
	}
	m.syncResourceList()
	return m, nil
}

func (m Model) updateResourceList(message tea.Msg) (tea.Model, tea.Cmd) {
	var command tea.Cmd
	m.resourceList, command = m.resourceList.Update(message)
	m.syncResourceList()
	return m, command
}

func (m *Model) syncResourceList() {
	selected := make(map[string]bool)
	if pack := m.selectedPack(); pack != nil {
		for _, resource := range pack.Resources {
			selected[resource.Identity] = m.resourceSelected(resource.Identity)
		}
	}
	width, height := m.selectionSize()
	m.resourceList.SetShowFilter(m.resourceList.SettingFilter())
	m.resourceList.SetShowStatusBar(height >= 20)
	m.resourceList.SetDelegate(resourceDelegate{selected: selected, initial: m.initialSelected, focused: !m.selectionPreviewFocus, compact: height < 20 || width < 64})
	header, footer := m.resourceSelectionFrame(width)
	m.resourceList.SetSize(width-4, max(height-lipgloss.Height(header)-lipgloss.Height(footer)-4, 3))
}

func (m Model) selectionSize() (int, int) {
	width, height := m.width, m.height
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 32
	}
	return width - 4, height
}

func (m Model) resourceSelectionFrame(width int) (string, string) {
	pack := m.selectedPack()
	if pack == nil {
		return "Select Pack resources", "Esc back"
	}
	scope, title := "Workstation · global", "Select Pack resources"
	if m.project {
		scope = "Current project"
	}
	if m.operation == "configure" {
		title = "Configure Pack resources"
	}
	surface := selectedSurface(*pack, m.surfaceIndex)
	if surface == "" {
		surface = "unavailable"
	}
	selected, total, added, removed := 0, 0, 0, 0
	for _, resource := range operationalRoots(*pack) {
		total++
		enabled := m.resourceSelected(resource.Identity)
		if enabled {
			selected++
		}
		if enabled && !m.initialSelected[resource.Identity] {
			added++
		} else if !enabled && m.initialSelected[resource.Identity] {
			removed++
		}
	}
	counts := resourceEnabledStyle.Render(fmt.Sprintf("%d of %d selected", selected, total))
	if added+removed > 0 {
		counts += "  " + resourcePendingStyle.Render(fmt.Sprintf("+%d / −%d pending", added, removed))
	} else {
		counts += "  " + dimStyle.Render("No changes")
	}
	surfaceLine := "CLI surface: " + surface + " · selected (←/→ change surface)"
	if width < 64 {
		surfaceLine = surface + " · ←/→ surface"
	}
	header := strings.Join([]string{
		titleStyle.Render(title),
		mutedStyle.Render(ansi.Truncate(pack.ID+" · "+surface+" · "+scope, width, "…")),
		resourceSurfaceStyle.Render(surfaceLine),
		counts,
	}, "\n")
	button := "  [ Apply changes · preview first ]"
	buttonStyle := resourceActionStyle
	if m.selectionPreviewFocus {
		button, buttonStyle = "› [ Apply changes · preview first ]", selectedRowStyle
	}
	notice := m.selectionNotice
	noticeStyle := mutedStyle
	if notice != "" {
		noticeStyle = resourcePendingStyle
	}
	if selected == 0 && m.operation == "configure" {
		noticeStyle = resourcePendingStyle
		notice = "This will deactivate the complete Pack on this CLI surface."
		if m.project {
			notice = "This will uninstall the Pack from this project's selected CLI surface."
		}
	}
	if notice == "" {
		notice = "Select resources, then review the changes before applying."
	}
	help := "↑/↓ move · Space/Enter toggle · / search · a all · n none · Tab actions · Esc back"
	if m.resourceList.SettingFilter() {
		help = "Type to search · Enter accept · Esc clear · Ctrl+C quit"
	} else if m.selectionPreviewFocus {
		help = "Enter preview changes · Tab resources · Esc back · q quit"
	}
	footer := strings.Join([]string{
		noticeStyle.Render(ansi.Truncate(notice, width, "…")),
		buttonStyle.Render(button),
		mutedStyle.Width(width).Render(help),
	}, "\n")
	return header, footer
}

func (m Model) renderSelection() string {
	m.syncResourceList()
	width, _ := m.selectionSize()
	header, footer := m.resourceSelectionFrame(width)
	body := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(mochaBlue).Padding(0, 1).Background(mochaMantle).Render(m.resourceList.View())
	return m.renderBody(lipgloss.NewStyle().Padding(0, 2).Render(header + "\n" + body + "\n" + footer))
}
