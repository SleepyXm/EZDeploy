package UI

import (
	"fmt"
	"strings"
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(signatureStyle.Render(signature))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("deployment toolkit — tool registry"))
	b.WriteString("\n")

	if m.project != "" {
		b.WriteString(projectBarStyle.Render("active project: " + m.project))
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
		locked := e.tool.RequiresProject && m.project == ""

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
			desc += " (needs an existing deployment)"
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
