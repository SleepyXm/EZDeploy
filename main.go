package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"EZDeploy/UI"
)

func main() {
	p := tea.NewProgram(UI.NewModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running TUI:", err)
		os.Exit(1)
	}
}
