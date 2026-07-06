package tui

import "github.com/charmbracelet/lipgloss"

// Palette: dark gray + mint as the primary hues, with muted, hue-appropriate
// accents for status (rose for errors, amber for notices).
const (
	mint       = lipgloss.Color("#7EE0C0") // primary accent
	mintBright = lipgloss.Color("#A8F0D8")
	mintDim    = lipgloss.Color("#4FB89A")

	surface = lipgloss.Color("#242424") // panel background
	border  = lipgloss.Color("#3D3D3D") // idle borders
	fg      = lipgloss.Color("#D6D6D6") // normal text
	faint   = lipgloss.Color("#6A6A6A") // hints, secondary text
	dim     = lipgloss.Color("#8A8A8A") // tool/meta text

	rose  = lipgloss.Color("#E39A93") // errors
	amber = lipgloss.Color("#E3C892") // notices
)

// Transcript markers and inline styles.
var (
	userLabel  = lipgloss.NewStyle().Bold(true).Foreground(mint)
	userText   = lipgloss.NewStyle().Foreground(fg)
	toolStyle  = lipgloss.NewStyle().Foreground(dim)
	okStyle    = lipgloss.NewStyle().Foreground(mint)
	errStyle   = lipgloss.NewStyle().Foreground(rose)
	noteStyle  = lipgloss.NewStyle().Foreground(amber)
	busyStyle  = lipgloss.NewStyle().Foreground(mint)
	spinStyle  = lipgloss.NewStyle().Foreground(mintBright)
	promptText = lipgloss.NewStyle().Foreground(fg)
)

// Chrome: input box, status line, hint bar.
var (
	inputBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mintDim).
			Padding(0, 1)

	statusBar = lipgloss.NewStyle().
			Background(surface).
			Foreground(dim)

	statusKey = lipgloss.NewStyle().Background(surface).Foreground(mint)
	statusVal = lipgloss.NewStyle().Background(surface).Foreground(fg)

	hintBar  = lipgloss.NewStyle().Foreground(faint)
	hintKey  = lipgloss.NewStyle().Foreground(dim)
	sepStyle = lipgloss.NewStyle().Foreground(faint)
)

// Tool card borders keyed by state.
var (
	cardRunning = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(mintDim).Padding(0, 1).MarginLeft(2)
	cardOK      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1).MarginLeft(2)
	cardErr     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(rose).Padding(0, 1).MarginLeft(2)

	cardTitleRun = lipgloss.NewStyle().Bold(true).Foreground(mint)
	cardTitleOK  = lipgloss.NewStyle().Bold(true).Foreground(dim)
	cardTitleErr = lipgloss.NewStyle().Bold(true).Foreground(rose)
	cardBody     = lipgloss.NewStyle().Foreground(dim)
	cardMeta     = lipgloss.NewStyle().Foreground(faint)
)
