package tui

import "github.com/charmbracelet/lipgloss"

var (
	styleStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	styleStatusItem = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	styleStatusBadgeEOF = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("166")).
				Padding(0, 1)

	styleStatusBadgeLive = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("29")).
				Padding(0, 1)

	styleStatusBadgePretty = lipgloss.NewStyle().
					Foreground(lipgloss.Color("230")).
					Background(lipgloss.Color("22")).
					Padding(0, 1)

	styleStatusBadgeFollowing = lipgloss.NewStyle().
					Foreground(lipgloss.Color("230")).
					Background(lipgloss.Color("25")).
					Padding(0, 1)

	styleSidebarBorder = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	styleSidebarBox = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240"))

	styleInputBox = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62"))

	styleSidebarHeader = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("219"))

	styleRoleUser = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("111"))

	styleRoleAssistant = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("114"))

	styleErr = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))

	styleDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	styleHelp = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	styleSelectedRow = lipgloss.NewStyle().
				Background(lipgloss.Color("237")).
				Foreground(lipgloss.Color("255"))

	styleActivity = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	styleActivityError = lipgloss.NewStyle().
				Foreground(lipgloss.Color("203"))
)
