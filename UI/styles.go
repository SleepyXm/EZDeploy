package UI

import "github.com/charmbracelet/lipgloss"

const signature = `
 ███████╗███████╗██████╗ ███████╗██████╗ ██╗      ██████╗ ██╗   ██╗
 ██╔════╝╚══███╔╝██╔══██╗██╔════╝██╔══██╗██║     ██╔═══██╗╚██╗ ██╔╝
 █████╗    ███╔╝ ██║  ██║█████╗  ██████╔╝██║     ██║   ██║ ╚████╔╝
 ██╔══╝   ███╔╝  ██║  ██║██╔══╝  ██╔═══╝ ██║     ██║   ██║  ╚██╔╝
 ███████╗███████╗██████╗ ███████╗██║     ███████╗╚██████╔╝   ██║
 ╚══════╝╚══════╝╚═════╝ ╚══════╝╚═╝     ╚══════╝ ╚═════╝    ╚═╝
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
