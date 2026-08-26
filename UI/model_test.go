package UI

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"EZDeploy/core"
)

func TestProjectDashboardSortingNavigationAndRefresh(t *testing.T) {
	t.Chdir(t.TempDir())
	writeTestRegistry(t, core.Registry{
		"zeta":  {Domain: "z.example.com", Revision: "2222222222222222", Services: []core.Service{{Name: "api", Unit: "z-api"}, {Name: "worker", Unit: "z-worker"}}},
		"alpha": {Domain: "a.example.com", Revision: "1111111111111111", Services: []core.Service{{Name: "api", Unit: "a-api"}}},
	})
	m := NewModel()
	if strings.Join(m.projects, ",") != "alpha,zeta" {
		t.Fatalf("project order = %v", m.projects)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.view != viewProject || m.selected != "alpha" {
		t.Fatalf("selection = view %d project %q", m.view, m.selected)
	}
	m.selected, m.view, m.cursor = "zeta", viewLogs, 2
	_, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("service log selection did not hand control to the CLI")
	}
	writeTestRegistry(t, core.Registry{"beta": {Domain: "b.example.com"}})
	updated, _ = m.Update(commandDoneMsg{})
	m = updated.(Model)
	if strings.Join(m.projects, ",") != "beta" || m.selected != "" {
		t.Fatalf("refreshed state = %v selected %q", m.projects, m.selected)
	}
}

func TestNoProjectDashboardStillExposesSystem(t *testing.T) {
	t.Chdir(t.TempDir())
	m := NewModel()
	view := m.View()
	if !strings.Contains(view, "No projects registered") || !strings.Contains(view, "System") {
		t.Fatalf("empty dashboard = %q", view)
	}
}

func writeTestRegistry(t *testing.T, registry core.Registry) {
	t.Helper()
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("registry.json", data, 0o600); err != nil {
		t.Fatal(err)
	}
}
