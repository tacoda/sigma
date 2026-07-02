package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tacoda/sigma/internal/commands"
)

// Pre-approved tools must short-circuit before any Program.Send, so Allow is
// safe to call with no running program.
func TestBridgePreApprove(t *testing.T) {
	b := &bridge{session: map[string]bool{}}
	b.preApprove([]string{"bash", "grep"})

	if !b.Allow("bash", "ls") {
		t.Error("pre-approved bash should allow without prompting")
	}
	if !b.Allow("grep", "x") {
		t.Error("pre-approved grep should allow without prompting")
	}
}

// esc while busy cancels the in-flight turn but keeps the session running;
// busy stays true until the agent goroutine reports back via doneMsg.
func TestEscCancelsTurnStaysInSession(t *testing.T) {
	called := false
	b := &bridge{session: map[string]bool{}, cancel: func() { called = true }}
	m := model{bridge: b, busy: true}

	next, _ := m.onKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !called {
		t.Error("esc should cancel the in-flight turn")
	}
	if !next.(model).busy {
		t.Error("session should stay busy until the turn actually finishes")
	}
}

// up/down walk history; stepping past the newest entry clears the input.
func TestHistoryRecall(t *testing.T) {
	m := newModel(nil, &bridge{session: map[string]bool{}}, nil)
	m.remember("first")
	m.remember("second")

	if got := m.recall(-1).input.Value(); got != "second" {
		t.Errorf("up once = %q, want %q", got, "second")
	}
	if got := m.recall(-1).recall(-1).input.Value(); got != "first" {
		t.Errorf("up twice = %q, want %q", got, "first")
	}
	// past the oldest stays put; forward past newest clears.
	if got := m.recall(-1).recall(-1).recall(-1).input.Value(); got != "first" {
		t.Errorf("up past oldest = %q, want %q", got, "first")
	}
	if got := m.recall(+1).input.Value(); got != "" {
		t.Errorf("down from newest = %q, want empty", got)
	}
}

// tab fills a unique match with a trailing space and a shared prefix otherwise.
func TestSlashComplete(t *testing.T) {
	cmds := commands.Set{"review": "", "run": ""}
	m := newModel(nil, &bridge{session: map[string]bool{}}, cmds)

	m.input.SetValue("/cl") // unique built-in
	if got := m.complete().input.Value(); got != "/clear " {
		t.Errorf("complete /cl = %q, want %q", got, "/clear ")
	}

	m.input.SetValue("/r") // review + run -> common prefix only
	if got := m.complete().input.Value(); got != "/r" {
		t.Errorf("complete /r = %q, want %q", got, "/r")
	}

	m.input.SetValue("/rev") // unique loaded command
	if got := m.complete().input.Value(); got != "/review " {
		t.Errorf("complete /rev = %q, want %q", got, "/review ")
	}
}

// input box grows with its content and clamps at maxInputHeight.
func TestInputHeightGrowsAndClamps(t *testing.T) {
	m := newModel(nil, &bridge{session: map[string]bool{}}, nil)

	m.input.SetValue("a\nb\nc")
	m.syncHeight()
	if m.inputH != 3 {
		t.Errorf("3 lines -> inputH %d, want 3", m.inputH)
	}

	m.input.SetValue(strings.Repeat("x\n", 20))
	m.syncHeight()
	if m.inputH != maxInputHeight {
		t.Errorf("20 lines -> inputH %d, want clamp %d", m.inputH, maxInputHeight)
	}
}

// refresh follows new output only when the viewport is already at the bottom;
// once the user scrolls up, incoming text must not yank them back down.
func TestScrollbackHoldsPosition(t *testing.T) {
	m := newModel(nil, &bridge{session: map[string]bool{}}, nil)
	m.resize(40, 20)

	m.transcript = strings.Repeat("line\n", 100)
	m.refresh()
	if !m.vp.AtBottom() {
		t.Fatal("fresh output should follow to the bottom")
	}

	m.vp.GotoTop() // user scrolls up
	m.transcript += "incoming\n"
	m.refresh()
	if m.vp.AtBottom() {
		t.Error("scrolled-up view should hold position when new output arrives")
	}
}
