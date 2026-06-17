package UI

import (
	"fmt"
	"strings"

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

	nameStyle = lipgloss.NewStyle().Width(20)

	validStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87"))
	invalidStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F"))
	unknownStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	descStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			MarginTop(1)

	resultOKStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87"))
	resultErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F"))
)

// entry pairs a Tool with its last-known validity check result.
type entry struct {
	tool   Tool
	status ToolStatus
	detail string
}

// Model is the top-level Bubble Tea model for the signature + tool registry view.
type Model struct {
	entries  []entry
	cursor   int
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
	return Model{entries: entries}
}

func (m Model) Init() tea.Cmd {
	return nil
}

// runResultMsg carries the outcome of executing a tool's Run function.
type runResultMsg struct {
	ok  bool
	msg string
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			// Re-run validity checks for all entries.
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
			if err := sel.tool.Run(map[string]string{}); err != nil {
				m.lastOK = false
				m.lastMsg = fmt.Sprintf("%s failed: %v", sel.tool.Name, err)
			} else {
				m.lastOK = true
				m.lastMsg = fmt.Sprintf("%s completed", sel.tool.Name)
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(signatureStyle.Render(signature))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("deployment toolkit — tool registry"))
	b.WriteString("\n\n")

	for i, e := range m.entries {
		cursor := "  "
		line := nameStyle.Render(e.tool.Name)

		switch e.status {
		case StatusValid:
			line += validStyle.Render("● valid  ")
		case StatusInvalid:
			line += invalidStyle.Render("● invalid")
		default:
			line += unknownStyle.Render("● unknown")
		}
		line += "  " + descStyle.Render(e.tool.Description)

		if i == m.cursor {
			cursor = "> "
			line = selectedStyle.Render(cursor + line)
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

	b.WriteString(footerStyle.Render("↑/↓ navigate · enter run · r re-validate · q quit"))

	return b.String()
}
