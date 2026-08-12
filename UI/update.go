package UI

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if keyMsg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
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
	case "q":
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
		if sel.tool.RequiresProject && m.project == "" {
			m.lastOK = false
			m.lastMsg = fmt.Sprintf("%s locked: deploy a project first", sel.tool.Name)
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
	case "q":
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
