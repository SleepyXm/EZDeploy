package UI

import (
	tea "github.com/charmbracelet/bubbletea"

	"EZDeploy/core"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if done, ok := msg.(commandDoneMsg); ok {
		m.refresh()
		m.lastOK = done.err == nil
		if done.err != nil {
			m.lastMsg = done.err.Error()
		} else {
			m.lastMsg = "Command completed"
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" || key.String() == "q" && m.view == viewProjects {
		m.quitting = true
		return m, tea.Quit
	}
	if key.String() == "esc" {
		return m.back(), nil
	}
	switch m.view {
	case viewProjects:
		return m.updateProjects(key)
	case viewProject:
		return m.updateProject(key)
	case viewReleases:
		return m.updateReleases(key)
	case viewLogs:
		return m.updateLogs(key)
	case viewNetwork:
		if key.String() == "r" {
			m.network = core.NetworkDiagnostics(m.project())
		}
	case viewSystem:
		return m.updateSystem(key)
	case viewInstall:
		return m.updateInstall(key)
	case viewConfirm:
		return m.updateConfirm(key)
	}
	return m, nil
}

func (m Model) updateProjects(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.move(key, len(m.projects)+1)
	if key.String() != "enter" {
		return m, nil
	}
	if m.cursor == len(m.projects) {
		m.view, m.cursor = viewSystem, 0
	} else {
		m.selected, m.view, m.cursor = m.projects[m.cursor], viewProject, 0
	}
	return m, nil
}

func (m Model) updateProject(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.move(key, len(projectActions))
	if key.String() != "enter" {
		return m, nil
	}
	switch m.cursor {
	case 0:
		m.view = viewOverview
	case 1:
		return m, m.external("redeploy", m.project().RepoURL)
	case 2:
		m.view, m.cursor = viewReleases, 0
	case 3:
		m.view, m.cursor = viewLogs, 0
	case 4:
		m.network, m.view = core.NetworkDiagnostics(m.project()), viewNetwork
	case 5, 6, 7:
		action := []string{"start", "stop", "restart"}[m.cursor-5]
		return m, m.external("__service", action, m.selected)
	case 8:
		m.confirm, m.view = "remove", viewConfirm
	case 9:
		return m.back(), nil
	}
	return m, nil
}

func (m Model) updateReleases(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	releases := m.project().Releases
	m.move(key, len(releases)+1)
	if key.String() != "enter" {
		return m, nil
	}
	if m.cursor == len(releases) {
		return m.back(), nil
	}
	m.releaseID = releases[len(releases)-1-m.cursor].ID
	m.confirm, m.view = "rollback", viewConfirm
	return m, nil
}

func (m Model) updateLogs(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	managed := m.project().ManagedServices(m.selected)
	m.move(key, len(managed)+2)
	if key.String() != "enter" && key.String() != "f" {
		return m, nil
	}
	if m.cursor == len(managed)+1 {
		return m.back(), nil
	}
	args := []string{"logs", m.selected, "--source", "deployment", "--lines", "100"}
	if m.cursor > 0 {
		args = []string{"logs", m.selected, "--source", "runtime", "--service", managed[m.cursor-1].Name, "--lines", "100"}
	}
	if key.String() == "f" {
		args = append(args, "--follow")
	}
	return m, m.external(args...)
}

func (m Model) updateSystem(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.move(key, 3)
	if key.String() != "enter" {
		return m, nil
	}
	switch m.cursor {
	case 0:
		m.view, m.cursor, m.chosen = viewInstall, 0, map[int]bool{}
	case 1:
		return m, m.external("__metrics")
	default:
		return m.back(), nil
	}
	return m, nil
}

func (m Model) updateConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.String() != "enter" {
		return m, nil
	}
	if m.confirm == "rollback" {
		m.view, m.confirm = viewReleases, ""
		return m, m.external("rollback", m.selected, "--release", m.releaseID, "--yes")
	}
	name := m.selected
	m.view, m.selected, m.cursor, m.confirm = viewProjects, "", 0, ""
	return m, m.external("__remove", name)
}

func (m Model) updateInstall(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.move(key, len(installItems))
	switch key.String() {
	case " ":
		m.chosen[m.cursor] = !m.chosen[m.cursor]
	case "enter":
		args := []string{"__system-install"}
		for index, item := range installItems {
			if m.chosen[index] {
				args = append(args, item)
			}
		}
		if len(args) == 1 {
			m.lastOK, m.lastMsg = false, "Select at least one component"
			return m, nil
		}
		return m, m.external(args...)
	}
	return m, nil
}

func (m *Model) move(key tea.KeyMsg, count int) {
	if count == 0 {
		m.cursor = 0
		return
	}
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < count-1 {
			m.cursor++
		}
	}
}

func (m Model) back() Model {
	m.cursor, m.confirm = 0, ""
	switch m.view {
	case viewProject, viewSystem:
		m.view, m.selected = viewProjects, ""
	case viewOverview, viewReleases, viewLogs, viewNetwork:
		m.view = viewProject
	case viewInstall:
		m.view = viewSystem
	case viewConfirm:
		if m.confirm == "rollback" {
			m.view = viewReleases
		} else {
			m.view = viewProject
		}
	}
	return m
}
