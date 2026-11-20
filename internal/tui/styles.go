package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	activePaneStyle = paneStyle.
			BorderForeground(lipgloss.Color("63"))

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("86"))

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	autoBadgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("42"))

	manualBadgeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("214"))

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	startButtonStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("232")).
				Background(lipgloss.Color("42")).
				Padding(1, 4).
				MarginTop(1)

	stopButtonStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("196")).
			Padding(1, 4).
			MarginTop(1)

	scanButtonStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("63")).
			Padding(1, 3).
			MarginTop(1)
)
