package UI

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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

	project string // project name selected from the deployment registry

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

	return Model{
		entries: entries,
		view:    viewList,
		input:   ti,
		project: activeProjectFromRegistry(),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}
