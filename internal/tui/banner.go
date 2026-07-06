package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// bannerHeight is the number of rows the top banner occupies.
const bannerHeight = 1

const bannerInterval = 150 * time.Millisecond

type tickMsg time.Time

// tick schedules the next banner animation frame.
func tick() tea.Cmd {
	return tea.Tick(bannerInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// bannerPalette is a cyan sweep the wordmark shimmers through.
var bannerPalette = []lipgloss.Color{"45", "51", "87", "123", "159", "123", "87", "51"}

var bannerTagline = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("  · coding agent")

// banner renders the animated "Σ sigma" wordmark; frame advances the color
// sweep so each redraw shifts the gradient one step.
func banner(frame int) string {
	var b strings.Builder
	b.WriteByte(' ')
	for i, r := range []rune("Σ sigma") {
		c := bannerPalette[(i+frame)%len(bannerPalette)]
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(c).Render(string(r)))
	}
	b.WriteString(bannerTagline)
	return b.String()
}
