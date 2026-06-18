package UI

import (
	"EZDeploy/core"
	"fmt"
	"strings"
)

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
			path := cloneDirJoin(name)

			m.project = &Project{
				Path:    path,
				Name:    name,
				RepoURL: repoURL,
			}

			if err := core.RegisterProject(name, core.Project{
				Path:    path,
				RepoURL: repoURL,
				Branch:  "main",
				Status:  "cloned",
			}); err != nil {
				m.lastOK = false
				m.lastMsg = fmt.Sprintf("CloneRepo completed but registry update failed: %v", err)
			}
		}
	}
	m.view = viewResult
}
