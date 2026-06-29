package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/commands"
	"github.com/tacoda/sigma/internal/session"
)

const footerHeight = 2

type model struct {
	agent    *agent.Agent
	bridge   *bridge
	commands commands.Set

	input textinput.Model
	vp    viewport.Model

	// Plain strings (not strings.Builder): Bubble Tea copies the model on every
	// Update, and a copied Builder panics.
	transcript string // committed conversation
	cur        string // streaming assistant text, not yet committed
	busy       bool
	pending    *askMsg

	ready bool
}

func newModel(a *agent.Agent, b *bridge, cmds commands.Set) model {
	ti := textinput.New()
	ti.Placeholder = "message  (/help for commands, ctrl+c to quit)"
	ti.Prompt = "› "
	ti.Focus()
	return model{agent: a, bridge: b, commands: cmds, input: ti}
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		return m.onKey(msg)
	case textMsg:
		m.cur += string(msg)
		m.refresh()
		return m, nil
	case toolMsg:
		m.commitAssistant()
		m.transcript += toolStyle.Render("  ⚙ "+msg.name+" "+msg.input) + "\n"
		m.refresh()
		return m, nil
	case askMsg:
		m.pending = &msg
		m.refresh()
		return m, nil
	case doneMsg:
		m.commitAssistant()
		if msg.err != nil {
			m.transcript += errStyle.Render("  ✗ error: "+msg.err.Error()) + "\n"
		}
		m.busy = false
		_ = session.Save(m.agent.Snapshot()) // best-effort autosave
		m.refresh()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		if m.pending != nil {
			m.pending.reply <- askReply{}
		}
		if m.bridge.cancel != nil {
			m.bridge.cancel() // stop any in-flight request/tool before exit
		}
		return m, tea.Quit
	}
	if m.pending != nil {
		return m.answerPending(msg.String()), nil
	}
	if m.busy {
		return m, nil
	}
	if msg.String() == "enter" {
		return m.submit()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) answerPending(key string) model {
	var r askReply
	switch strings.ToLower(key) {
	case "y":
		r = askReply{allow: true}
	case "a":
		r = askReply{allow: true, always: true}
	}
	m.pending.reply <- r
	if r.allow {
		m.transcript += okStyle.Render("  → allowed "+m.pending.name) + "\n"
	} else {
		m.transcript += errStyle.Render("  → denied "+m.pending.name) + "\n"
	}
	m.pending = nil
	m.refresh()
	return m
}

func (m model) submit() (tea.Model, tea.Cmd) {
	val := strings.TrimSpace(m.input.Value())
	if val == "" {
		return m, nil
	}
	m.input.Reset()
	if strings.HasPrefix(val, "/") {
		return m.dispatch(val)
	}
	m.transcript += "\n" + userStyle.Render("› "+val) + "\n\n"
	m.busy = true
	m.refresh()
	return m, m.startTurn(val)
}

// dispatch handles slash commands: built-ins and loaded templates.
func (m model) dispatch(line string) (tea.Model, tea.Cmd) {
	name, args, _ := strings.Cut(strings.TrimPrefix(line, "/"), " ")
	switch name {
	case "help":
		m.transcript += m.helpText()
		m.refresh()
		return m, nil
	case "clear":
		m.agent.Reset()
		m.transcript = ""
		m.cur = ""
		m.refresh()
		return m, nil
	case "quit", "exit":
		return m, tea.Quit
	}
	body, ok := m.commands[name]
	if !ok {
		m.transcript += errStyle.Render("  unknown command: /"+name) + "\n"
		m.refresh()
		return m, nil
	}
	m.transcript += "\n" + userStyle.Render("› /"+name) + noteStyle.Render(" "+args) + "\n\n"
	m.busy = true
	m.refresh()
	return m, m.startTurn(commands.Expand(body, args))
}

func (m model) helpText() string {
	out := noteStyle.Render("built-in: /help  /clear  /quit") + "\n"
	if len(m.commands) > 0 {
		out += noteStyle.Render("commands: /"+strings.Join(m.commands.Names(), "  /")) + "\n"
	}
	return out
}

// startTurn runs one agent turn in a goroutine, signalling completion via Send.
// The turn's context is stored on the bridge so ctrl+c can cancel it.
func (m model) startTurn(input string) tea.Cmd {
	a, b := m.agent, m.bridge
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		err := a.Run(ctx, input)
		b.prog.Send(doneMsg{err: err})
		return nil
	}
}

// commitAssistant flushes streamed text into the transcript.
func (m *model) commitAssistant() {
	if m.cur == "" {
		return
	}
	m.transcript += m.cur + "\n"
	m.cur = ""
}

func (m *model) resize(w, h int) {
	if !m.ready {
		m.vp = viewport.New(w, h-footerHeight)
		m.ready = true
	} else {
		m.vp.Width = w
		m.vp.Height = h - footerHeight
	}
	m.input.Width = w - 4
	m.refresh()
}

func (m *model) refresh() {
	if !m.ready {
		return
	}
	m.vp.SetContent(m.transcript + m.cur)
	m.vp.GotoBottom()
}

func (m model) View() string {
	if !m.ready {
		return "loading…"
	}
	var footer string
	switch {
	case m.pending != nil:
		footer = promptStyle.Render(fmt.Sprintf("allow %s? %s  [y]es / [a]lways / [N]o",
			m.pending.name, m.pending.detail))
	case m.busy:
		footer = busyStyle.Render("…working (ctrl+c to quit)")
	default:
		footer = m.input.View()
	}
	return m.vp.View() + "\n" + footer
}
