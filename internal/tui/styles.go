package tui

import "github.com/charmbracelet/lipgloss"

// Styles for transcript markers and the footer. ANSI 8-15 (bright) colors keep
// it readable across terminal themes.
var (
	userStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")) // bright blue
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // gray
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))            // green
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))             // red
	noteStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))            // yellow
	busyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Italic(true)
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14")) // cyan
)
