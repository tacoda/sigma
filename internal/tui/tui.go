// Package tui is an interactive multi-turn chat front-end built on Bubble Tea.
//
// The agent runs in a background goroutine. It talks to the UI through a bridge
// that satisfies agent.UI and agent.PermissionPolicy: streamed text and tool calls are
// pushed into the Bubble Tea event loop via Program.Send, and permission
// requests block the agent goroutine on a reply channel until the user answers.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/agents"
	"github.com/tacoda/sigma/internal/commands"
	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/permission"
	"github.com/tacoda/sigma/internal/tools"
	"github.com/tacoda/sigma/internal/workflows"
	"github.com/tacoda/sigma/internal/workspace"
)

// SessionStore persists the conversation so a session can be resumed.
type SessionStore interface {
	Load() ([]message.Message, bool, error)
	Save([]message.Message) error
}

// shortCWD is the current directory's base name, for the status line.
func shortCWD() string {
	wd, err := os.Getwd()
	if err != nil {
		return "?"
	}
	return filepath.Base(wd)
}

// --- messages pushed into the Bubble Tea loop ---

type textMsg string
type toolMsg struct{ name, input string }
type toolResultMsg struct {
	name, output string
	isErr        bool
}
type usageMsg struct{ in, out int }
type doneMsg struct{ err error }

type askReply struct{ allow, always bool }
type askMsg struct {
	name, detail string
	reply        chan askReply
}

// bridge connects the agent goroutine to the Bubble Tea program.
type bridge struct {
	prog    *tea.Program
	session map[string]bool    // tools approved for the session
	cancel  context.CancelFunc // cancels the in-flight turn, if any
}

func (b *bridge) Text(delta string)        { b.prog.Send(textMsg(delta)) }
func (b *bridge) ToolCall(name, in string) { b.prog.Send(toolMsg{name, in}) }
func (b *bridge) ToolResult(name, output string, isErr bool) {
	b.prog.Send(toolResultMsg{name: name, output: output, isErr: isErr})
}
func (b *bridge) Usage(in, out int) { b.prog.Send(usageMsg{in, out}) }
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
	Client      agent.LLM
	NewTools    func(root string) []tools.Tool
	Types       agents.Set
	Workflows   workflows.Set
	Isolate     bool
	Hooks       hooks.Bus
	Allowed     []string
	Model       string
	System      string
	Store       SessionStore
	Mode        permission.Mode
	CompactAt   int
	TokenBudget int
	LLMRetries  int
	Resume      bool
}

// Run starts the interactive chat session and blocks until the user quits.
func Run(cfg Config) error {
	if cfg.Hooks == nil {
		cfg.Hooks = hooks.Nop{}
	}
	b := &bridge{session: map[string]bool{}}
	b.preApprove(cfg.Allowed)

	base := agent.Config{
		Client:      cfg.Client,
		Permission:  permission.ForMode(cfg.Mode, b),
		Hooks:       cfg.Hooks,
		Model:       cfg.Model,
		System:      cfg.System,
		CompactAt:   cfg.CompactAt,
		TokenBudget: cfg.TokenBudget,
		LLMRetries:  cfg.LLMRetries,
	}
	base.Tools = agent.WithSubagent(base, agent.SubagentOptions{
		Tools:     cfg.NewTools,
		Types:     cfg.Types,
		Workflows: cfg.Workflows,
		Isolate:   cfg.Isolate,
		Workspace: workspace.Git{},
	})
	base.UI = b
	a := agent.New(base)

	m := newModel(a, b, commands.Load())
	m.modelName = cfg.Model
	m.cwd = shortCWD()
	m.store = cfg.Store
	if cfg.Resume && m.store != nil {
		if msgs, ok, err := m.store.Load(); err == nil && ok {
			a.Restore(msgs)
			m.transcript = noteStyle.Render(fmt.Sprintf("(resumed %d messages)", len(msgs))) + "\n"
		}
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	b.prog = p

	cfg.Hooks.Emit(context.Background(), hooks.Event{Kind: hooks.SessionStart})
	_, err := p.Run()
	cfg.Hooks.Emit(context.Background(), hooks.Event{Kind: hooks.SessionEnd})
	return err
}
