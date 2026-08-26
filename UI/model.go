package UI

import (
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"EZDeploy/core"
)

type view int

const (
	viewProjects view = iota
	viewProject
	viewOverview
	viewReleases
	viewLogs
	viewNetwork
	viewSystem
	viewInstall
	viewConfirm
)

var projectActions = []string{"Overview", "Redeploy", "Releases and rollback", "Runtime/deployment logs", "Network and DNS", "Start", "Stop", "Restart", "Project removal", "Back"}
var installItems = []string{"go", "node", "python", "rust", "nginx", "certbot", "docker"}

type commandDoneMsg struct{ err error }

// Model keeps navigation state only; deployment work remains in the CLI pipeline.
type Model struct {
	registry  core.Registry
	projects  []string
	view      view
	cursor    int
	selected  string
	confirm   string
	releaseID string
	chosen    map[int]bool
	network   core.NetworkReport
	lastMsg   string
	lastOK    bool
	quitting  bool
}

func NewModel() Model {
	m := Model{}
	m.refresh()
	return m
}

func (m *Model) refresh() {
	registry, err := core.GetRegistry()
	if err != nil {
		m.registry, m.projects, m.lastMsg, m.lastOK = core.Registry{}, nil, err.Error(), false
		return
	}
	m.registry, m.projects = registry, core.SortedProjects(registry)
	if _, exists := registry[m.selected]; !exists {
		m.selected = ""
	}
}

func (m Model) project() core.Project { return m.registry[m.selected] }
func (m Model) Init() tea.Cmd         { return nil }

func (m Model) external(args ...string) tea.Cmd {
	executable, err := os.Executable()
	if err != nil {
		return func() tea.Msg { return commandDoneMsg{err} }
	}
	command := exec.Command(executable, args...)
	command.Env = os.Environ()
	return tea.ExecProcess(command, func(err error) tea.Msg { return commandDoneMsg{err} })
}
