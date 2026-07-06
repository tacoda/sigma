package tui

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
)

// fmtTokens humanizes a token count (12400 -> "12.4k").
func fmtTokens(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return strconv.FormatFloat(float64(n)/1000, 'f', 1, 64) + "k"
}

func scrollPct(vp viewport.Model) string {
	return strconv.Itoa(int(vp.ScrollPercent()*100)) + "%"
}

// kbd styles a key hint.
func kbd(s string) string { return hintKey.Render(s) }

// spinnerFrames is a braille spinner advanced by the banner tick.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func spinnerFrame(f int) string { return spinnerFrames[f%len(spinnerFrames)] }

// renderMarkdown renders committed assistant text with glamour's dark theme,
// word-wrapped to width. On any error it falls back to the raw text.
func renderMarkdown(md string, width int) string {
	if width < 20 {
		width = 20
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return strings.TrimRight(out, "\n")
}

// toolEntry is a tool call awaiting its result.
type toolEntry struct {
	name, input string
	start       time.Time
}

func cardWidth(termW int) int {
	w := termW - 6 // margin(2) + border(2) + padding(2)
	if w < 12 {
		w = 12
	}
	return w
}

// toolCardRunning renders the live card for an in-flight tool.
func toolCardRunning(t toolEntry, frame, termW int) string {
	w := cardWidth(termW)
	title := cardTitleRun.Render(t.name) + cardMeta.Render("  "+spinnerFrame(frame)+" running")
	body := cardBody.Render(clip(toolPreview(t.name, t.input), w))
	return cardRunning.Width(w).Render(title + "\n" + body)
}

// toolCardDone renders the finished card with status, duration, and a short
// output preview.
func toolCardDone(t toolEntry, output string, isErr bool, dur time.Duration, termW int) string {
	w := cardWidth(termW)
	style, title, status := cardOK, cardTitleOK, "✓ ok"
	if isErr {
		style, title, status = cardErr, cardTitleErr, "✗ error"
	}
	head := title.Render(t.name) + cardMeta.Render("  "+status+" · "+dur.Round(time.Millisecond).String())
	lines := []string{head, cardBody.Render(clip(toolPreview(t.name, t.input), w))}
	if preview := outputPreview(output, w, 6); preview != "" {
		lines = append(lines, cardMeta.Render(preview))
	}
	return style.Width(w).Render(strings.Join(lines, "\n"))
}

// toolPreview is a one-line summary of a tool's input: the shell command for
// bash, otherwise the compacted JSON.
func toolPreview(name, input string) string {
	if name == "bash" {
		var a struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(input), &a) == nil && a.Command != "" {
			return "$ " + a.Command
		}
	}
	return strings.Join(strings.Fields(input), " ")
}

// outputPreview returns up to maxLines clipped lines of output, noting any
// remainder.
func outputPreview(output string, width, maxLines int) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	extra := 0
	if len(lines) > maxLines {
		extra = len(lines) - maxLines
		lines = lines[:maxLines]
	}
	for i, l := range lines {
		lines[i] = clip(l, width)
	}
	out := strings.Join(lines, "\n")
	if extra > 0 {
		out += "\n… " + strconv.Itoa(extra) + " more lines"
	}
	return out
}

func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 1 {
		return ""
	}
	return string(r[:max-1]) + "…"
}
