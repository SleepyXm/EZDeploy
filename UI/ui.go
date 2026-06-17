package UI

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const signature = `
 ███████╗███████╗██████╗ ███████╗██████╗ ██╗      ██████╗ ██╗   ██╗
 ██╔════╝╚══███╔╝██╔══██╗██╔════╝██╔══██╗██║     ██╔═══██╗╚██╗ ██╔╝
 █████╗    ███╔╝ ██║  ██║█████╗  ██████╔╝██║     ██║   ██║ ╚████╔╝
 ██╔══╝   ███╔╝  ██║  ██║██╔══╝  ██╔══██╗██║     ██║   ██║  ╚██╔╝
 ███████╗███████╗██████╗ ███████╗██║  ██║███████╗╚██████╔╝   ██║
 ╚══════╝╚══════╝╚═════╝ ╚══════╝╚═╝  ╚═╝╚══════╝ ╚═════╝    ╚═╝
`

var (
	signatureStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7FF")).
			Bold(true)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true).
			MarginBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#3C3C3C")).
			Bold(true)

	nameStyle   = lipgloss.NewStyle().Width(20)
	lockedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))

	validStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87"))
	invalidStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F"))
	unknownStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			MarginTop(1)

	resultOKStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87"))
	resultErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F"))

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00D7FF")).
			Bold(true).
			MarginBottom(1)

	fieldLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	fieldAutoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87"))
	confirmKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Width(16)
	confirmValStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	projectBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D7FF")).MarginBottom(1)
)

// entry pairs a Tool with its last-known validity check result.
type entry struct {
	tool   Tool
	status ToolStatus
	detail string
}

// view identifies which screen the model is currently rendering.
type view int

const (
	viewList view = iota
	viewInput
	viewConfirm
	viewResult
)

// Model is the top-level Bubble Tea model: signature + tool registry +
// step-by-step argument collection + confirmation, wired to core functions.
type Model struct {
	entries []entry
	cursor  int
	view    view

	project *Project // active project context, set once CloneRepo succeeds

	// step-by-step input state
	activeTool  *Tool
	pendingFlds []Field // fields still needing user input (autofilled ones excluded)
	fieldIdx    int
	collected   map[string]string
	input       textinput.Model

	// multi-select state, active when pendingFlds[fieldIdx].Kind == FieldMultiSelect
	msCursor   int
	msSelected map[int]bool

	lastMsg  string
	lastOK   bool
	quitting bool
}

// NewModel builds the initial model, running validity checks for every
// registered tool up front.
func NewModel() Model {
	tools := Registry()
	entries := make([]entry, len(tools))
	for i, t := range tools {
		status, detail := t.Validate()
		entries[i] = entry{tool: t, status: status, detail: detail}
	}
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 40
	return Model{entries: entries, view: viewList, input: ti}
}

func (m Model) Init() tea.Cmd {
	return nil
}

// isAutoFilled reports whether field key was resolved from project context
// rather than prompted for, for the currently active tool.
func (m Model) isAutoFilled(key string) bool {
	for _, pf := range m.pendingFlds {
		if pf.Key == key {
			return false
		}
	}
	return true
}

// enterField configures input state for pendingFlds[idx], branching on
// field kind (text input vs multi-select checklist).
func (m *Model) enterField(idx int) {
	m.fieldIdx = idx
	f := m.pendingFlds[idx]

	if f.Kind == FieldMultiSelect {
		m.msCursor = 0
		m.msSelected = map[int]bool{}
		// restore prior selection if the user backed into this field again
		if prev, ok := m.collected[f.Key]; ok && prev != "" {
			chosen := make(map[string]bool)
			for _, v := range splitCSV(prev) {
				chosen[v] = true
			}
			for i, opt := range f.Options {
				if chosen[opt] {
					m.msSelected[i] = true
				}
			}
		}
		return
	}

	m.input.SetValue(m.collected[f.Key])
	m.input.Placeholder = f.Placeholder
	m.input.Focus()
}

func (m *Model) startTool(t Tool) {
	m.activeTool = &t
	m.collected = map[string]string{}
	m.pendingFlds = nil

	for _, f := range t.Fields {
		if f.AutoFill != nil {
			if val, ok := f.AutoFill(m.project); ok {
				m.collected[f.Key] = val
				continue
			}
		}
		m.pendingFlds = append(m.pendingFlds, f)
	}

	if len(m.pendingFlds) == 0 {
		m.view = viewConfirm
		return
	}
	m.view = viewInput
	m.enterField(0)
}

// commitField writes the current field's value (text or multi-select) into
// m.collected based on its kind.
func (m *Model) commitField() {
	f := m.pendingFlds[m.fieldIdx]
	if f.Kind == FieldMultiSelect {
		var chosen []string
		for i, opt := range f.Options {
			if m.msSelected[i] {
				chosen = append(chosen, opt)
			}
		}
		m.collected[f.Key] = strings.Join(chosen, ",")
		return
	}
	m.collected[f.Key] = strings.TrimSpace(m.input.Value())
}

func (m *Model) advanceField() {
	m.commitField()

	if m.fieldIdx == len(m.pendingFlds)-1 {
		m.view = viewConfirm
		return
	}
	m.enterField(m.fieldIdx + 1)
}

func (m *Model) retreatField() {
	if m.fieldIdx == 0 {
		m.view = viewList
		m.activeTool = nil
		return
	}
	m.enterField(m.fieldIdx - 1)
}

func (m *Model) runActiveTool() {
	t := *m.activeTool
	err := t.Run(m.collected)
	if err != nil {
		m.lastOK = false
		m.lastMsg = fmt.Sprintf("%s failed: %v", t.Name, err)
	} else {
		m.lastOK = true
		m.lastMsg = fmt.Sprintf("%s completed", t.Name)

		// CloneRepo establishes the active project context for downstream tools.
		if t.Name == "CloneRepo" {
			repoURL := m.collected["repoURL"]
			name := repoNameFromURL(repoURL)
			m.project = &Project{
				Path:    cloneDirJoin(name),
				Name:    name,
				RepoURL: repoURL,
			}
		}
	}
	m.view = viewResult
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch m.view {
	case viewList:
		return m.updateList(keyMsg)
	case viewInput:
		return m.updateInput(keyMsg)
	case viewConfirm:
		return m.updateConfirm(keyMsg)
	case viewResult:
		return m.updateResult(keyMsg)
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
		}
	case "r":
		for i := range m.entries {
			status, detail := m.entries[i].tool.Validate()
			m.entries[i].status = status
			m.entries[i].detail = detail
		}
	case "enter":
		sel := m.entries[m.cursor]
		if sel.status == StatusInvalid {
			m.lastOK = false
			m.lastMsg = fmt.Sprintf("%s skipped: %s", sel.tool.Name, sel.detail)
			return m, nil
		}
		if sel.tool.RequiresProject && m.project == nil {
			m.lastOK = false
			m.lastMsg = fmt.Sprintf("%s locked: clone a repo first (select CloneRepo)", sel.tool.Name)
			return m, nil
		}
		m.startTool(sel.tool)
	}
	return m, nil
}

func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.pendingFlds[m.fieldIdx]
	if f.Kind == FieldMultiSelect {
		return m.updateMultiSelect(msg, f)
	}

	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.retreatField()
		return m, nil
	case "enter":
		m.advanceField()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateMultiSelect(msg tea.KeyMsg, f Field) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.retreatField()
		return m, nil
	case "up", "k":
		if m.msCursor > 0 {
			m.msCursor--
		}
	case "down", "j":
		if m.msCursor < len(f.Options)-1 {
			m.msCursor++
		}
	case " ":
		m.msSelected[m.msCursor] = !m.msSelected[m.msCursor]
	case "enter":
		m.advanceField()
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		if len(m.pendingFlds) > 0 {
			m.fieldIdx = len(m.pendingFlds) - 1
			m.input.SetValue(m.collected[m.pendingFlds[m.fieldIdx].Key])
			m.input.Placeholder = m.pendingFlds[m.fieldIdx].Placeholder
			m.view = viewInput
		} else {
			m.view = viewList
			m.activeTool = nil
		}
		return m, nil
	case "enter":
		m.runActiveTool()
		return m, nil
	}
	return m, nil
}

func (m Model) updateResult(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "enter", "esc":
		m.activeTool = nil
		m.view = viewList
		// refresh validity since on-disk state (e.g. registry.json) may have changed
		for i := range m.entries {
			status, detail := m.entries[i].tool.Validate()
			m.entries[i].status = status
			m.entries[i].detail = detail
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(signatureStyle.Render(signature))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("deployment toolkit — tool registry"))
	b.WriteString("\n")

	if m.project != nil {
		b.WriteString(projectBarStyle.Render(fmt.Sprintf("active project: %s (%s)", m.project.Name, m.project.Path)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	switch m.view {
	case viewList:
		b.WriteString(m.renderList())
	case viewInput:
		b.WriteString(m.renderInput())
	case viewConfirm:
		b.WriteString(m.renderConfirm())
	case viewResult:
		b.WriteString(m.renderResult())
	}

	return b.String()
}

func (m Model) renderList() string {
	var b strings.Builder
	for i, e := range m.entries {
		cursor := "  "
		locked := e.tool.RequiresProject && m.project == nil

		line := nameStyle.Render(e.tool.Name)
		switch {
		case locked:
			line += lockedStyle.Render("● locked ")
		case e.status == StatusValid:
			line += validStyle.Render("● valid  ")
		case e.status == StatusInvalid:
			line += invalidStyle.Render("● invalid")
		default:
			line += unknownStyle.Render("● unknown")
		}

		desc := e.tool.Description
		if locked {
			desc += " (needs active project — run CloneRepo first)"
		}
		line += "  " + descStyle.Render(desc)

		if i == m.cursor {
			line = selectedStyle.Render("> " + line)
		} else {
			line = cursor + line
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	if m.lastMsg != "" {
		b.WriteString("\n")
		if m.lastOK {
			b.WriteString(resultOKStyle.Render("✓ " + m.lastMsg))
		} else {
			b.WriteString(resultErrStyle.Render("✗ " + m.lastMsg))
		}
		b.WriteString("\n")
	}

	b.WriteString(footerStyle.Render("↑/↓ navigate · enter select · r re-validate · q quit"))
	return b.String()
}

func (m Model) renderInput() string {
	var b strings.Builder
	f := m.pendingFlds[m.fieldIdx]
	b.WriteString(headerStyle.Render(fmt.Sprintf("%s — step %d/%d", m.activeTool.Name, m.fieldIdx+1, len(m.pendingFlds))))
	b.WriteString("\n")

	for _, pf := range m.pendingFlds[:m.fieldIdx] {
		val := m.collected[pf.Key]
		b.WriteString(confirmKeyStyle.Render(pf.Label + ":"))
		b.WriteString(confirmValStyle.Render(val))
		b.WriteString("\n")
	}

	b.WriteString(fieldLabelStyle.Render(f.Label + ":"))
	b.WriteString("\n")

	if f.Kind == FieldMultiSelect {
		for i, opt := range f.Options {
			box := "[ ]"
			if m.msSelected[i] {
				box = "[x]"
			}
			line := fmt.Sprintf("%s %s", box, opt)
			if i == m.msCursor {
				line = selectedStyle.Render("> " + line)
			} else {
				line = "  " + line
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(footerStyle.Render("↑/↓ move · space toggle · enter next (none = defaults) · esc back"))
		return b.String()
	}

	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	b.WriteString(footerStyle.Render("enter next · esc back · ctrl+c quit"))
	return b.String()
}

func (m Model) renderConfirm() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("%s — confirm", m.activeTool.Name)))
	b.WriteString("\n")

	for _, f := range m.activeTool.Fields {
		val := m.collected[f.Key]
		display := val
		if f.Kind == FieldMultiSelect && val == "" {
			display = "(none — nginx + certbot will install as defaults)"
		}
		auto := ""
		if m.isAutoFilled(f.Key) {
			auto = fieldAutoStyle.Render(" (from active project)")
		}
		b.WriteString(confirmKeyStyle.Render(f.Label + ":"))
		b.WriteString(confirmValStyle.Render(display))
		b.WriteString(auto)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("enter run · esc back · ctrl+c quit"))
	return b.String()
}

func (m Model) renderResult() string {
	var b strings.Builder
	if m.lastOK {
		b.WriteString(resultOKStyle.Render("✓ " + m.lastMsg))
	} else {
		b.WriteString(resultErrStyle.Render("✗ " + m.lastMsg))
	}
	b.WriteString("\n\n")
	b.WriteString(footerStyle.Render("enter/esc back to list · q quit"))
	return b.String()
}

// repoNameFromURL mirrors the repo-name derivation in core.CloneRepo so the
// TUI's project context matches the actual on-disk clone path.
func repoNameFromURL(repoURL string) string {
	trimmed := strings.TrimRight(repoURL, "/")
	trimmed = strings.TrimSuffix(trimmed, ".git")
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

// cloneDirJoin builds the on-disk path core.CloneRepo would have used.
// Mirrors core.CloneDir; kept as a literal here to avoid exporting internals.
func cloneDirJoin(repoName string) string {
	return strings.TrimRight(cloneDirConst, "/") + "/" + repoName
}

const cloneDirConst = "/opt/EZDeploy/projects"
