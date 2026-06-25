// Package tui is an interactive multi-turn chat front-end built on Bubble Tea.
//
// The agent runs in a background goroutine. It talks to the UI through a bridge
// that satisfies agent.UI and agent.Approver: streamed text and tool calls are
// pushed into the Bubble Tea event loop via Program.Send, and permission
// requests block the agent goroutine on a reply channel until the user answers.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/anthropic"
	"github.com/tacoda/sigma/internal/commands"
	"github.com/tacoda/sigma/internal/session"
	"github.com/tacoda/sigma/internal/tools"
)

// --- messages pushed into the Bubble Tea loop ---

type textMsg string
type toolMsg struct{ name, input string }
type doneMsg struct{ err error }

type askReply struct{ allow, always bool }
type askMsg struct {
	name, detail string
	reply        chan askReply
}

// bridge connects the agent goroutine to the Bubble Tea program.
type bridge struct {
	prog    *tea.Program
	session map[string]bool // tools approved for the session
}

func (b *bridge) Text(delta string)        { b.prog.Send(textMsg(delta)) }
func (b *bridge) ToolCall(name, in string) { b.prog.Send(toolMsg{name, in}) }
func (b *bridge) preApprove(names []string) {
	for _, n := range names {
		b.session[n] = true
	}
}

// Allow runs on the agent goroutine; it blocks until the UI replies.
func (b *bridge) Allow(name, detail string) bool {
	if b.session[name] {
		return true
	}
	reply := make(chan askReply, 1)
	b.prog.Send(askMsg{name: name, detail: detail, reply: reply})
	r := <-reply
	if r.always {
		b.session[name] = true
	}
	return r.allow
}

// Config holds everything needed to start a chat session.
type Config struct {
	Client     *anthropic.Client
	ChildTools []tools.Tool
	Hooks      agent.Hooks
	Allowed    []string
	Model      string
	System     string
	Resume     bool
}

// Run starts the interactive chat session and blocks until the user quits.
func Run(cfg Config) error {
	b := &bridge{session: map[string]bool{}}
	b.preApprove(cfg.Allowed)

	base := agent.Config{
		Client:   cfg.Client,
		Approver: b,
		Hooks:    cfg.Hooks,
		Model:    cfg.Model,
		System:   cfg.System,
	}
	base.Tools = agent.WithSubagent(base, cfg.ChildTools)
	base.UI = b
	a := agent.New(base)

	m := newModel(a, b, commands.Load())
	if cfg.Resume {
		if msgs, err := session.Load(); err == nil {
			a.Restore(msgs)
			m.transcript = noteStyle.Render(fmt.Sprintf("(resumed %d messages)", len(msgs))) + "\n"
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	b.prog = p

	_, err := p.Run()
	return err
}
