package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/commands"
	"github.com/tacoda/sigma/internal/session"
)

const maxInputHeight = 6

type model struct {
	agent    *agent.Agent
	bridge   *bridge
	commands commands.Set

	input textarea.Model
	vp    viewport.Model

	termW, termH int
	inputH       int // current textarea height in rows

	// Plain strings (not strings.Builder): Bubble Tea copies the model on every
	// Update, and a copied Builder panics.
	transcript string // committed conversation
	cur        string // streaming assistant text, not yet committed
	busy       bool
	pending    *askMsg

	history []string // past submitted lines, oldest first
	histIdx int      // cursor into history; == len(history) means "new line"

	frame int // banner animation frame
	ready bool
}

func newModel(a *agent.Agent, b *bridge, cmds commands.Set) model {
	ta := textarea.New()
	ta.Placeholder = "message  (shift+enter for newline, /help, ctrl+c to quit)"
	ta.Prompt = "› "
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	// enter submits; newline moves to a dedicated chord.
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "alt+enter", "ctrl+j"))
	ta.Focus()
	return model{agent: a, bridge: b, commands: cmds, input: ta, inputH: 1}
}

func (m model) Init() tea.Cmd { return tea.Batch(textarea.Blink, tick()) }

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
	case tea.MouseMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg) // wheel scroll, anytime
		return m, cmd
	case askMsg:
		m.pending = &msg
		m.refresh()
		return m, nil
	case doneMsg:
		return m.onDone(msg), nil
	case tickMsg:
		m.frame++
		return m, tick()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// onDone commits the finished turn, reporting cancellation or error, and
// autosaves the session.
func (m model) onDone(msg doneMsg) model {
	m.commitAssistant()
	switch {
	case msg.err == nil:
	case errors.Is(msg.err, context.Canceled):
		m.transcript += noteStyle.Render("  ⊘ cancelled") + "\n"
	default:
		m.transcript += errStyle.Render("  ✗ error: "+msg.err.Error()) + "\n"
	}
	m.busy = false
	_ = session.Save(m.agent.Snapshot()) // best-effort autosave
	m.refresh()
	return m
}

func (m model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m.quit()
	}
	if isScrollKey(key) {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg) // scrollback works anytime, mid-turn included
		return m, cmd
	}
	if m.pending != nil {
		return m.answerPending(key), nil
	}
	if m.busy {
		if key == "esc" && m.bridge.cancel != nil {
			m.bridge.cancel() // cancel the turn; stay in the session
		}
		return m, nil
	}
	return m.editKey(msg)
}

// quit tears down any in-flight turn or pending prompt, then exits.
func (m model) quit() (tea.Model, tea.Cmd) {
	if m.pending != nil {
		m.pending.reply <- askReply{}
	}
	if m.bridge.cancel != nil {
		m.bridge.cancel()
	}
	return m, tea.Quit
}

func isScrollKey(key string) bool {
	switch key {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		return true
	}
	return false
}

// editKey handles keys while the input is live (not busy, nothing pending).
func (m model) editKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m.submit()
	case "tab":
		return m.complete(), nil
	case "up":
		return m.navUp(msg)
	case "down":
		return m.navDown(msg)
	}
	return m.typeInto(msg)
}

// navUp/navDown recall history on a single line; on a multi-line draft they
// fall through to the textarea so the cursor moves between lines instead.
func (m model) navUp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.input.LineCount() <= 1 {
		return m.recall(-1), nil
	}
	return m.typeInto(msg)
}

func (m model) navDown(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.input.LineCount() <= 1 {
		return m.recall(+1), nil
	}
	return m.typeInto(msg)
}

// typeInto forwards a key to the textarea and syncs the layout.
func (m model) typeInto(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.syncHeight()
	m.refresh()
	return m, cmd
}

// recall steps the history cursor and loads that line into the input. Stepping
// past the newest entry clears the input back to a fresh line.
func (m model) recall(dir int) model {
	if len(m.history) == 0 {
		return m
	}
	m.histIdx += dir
	if m.histIdx < 0 {
		m.histIdx = 0
	}
	if m.histIdx >= len(m.history) {
		m.histIdx = len(m.history)
		m.input.SetValue("")
	} else {
		m.input.SetValue(m.history[m.histIdx])
		m.input.CursorEnd()
	}
	m.syncHeight()
	m.refresh()
	return m
}

// remember appends a submitted line and resets the recall cursor.
func (m *model) remember(line string) {
	m.history = append(m.history, line)
	m.histIdx = len(m.history)
}

var builtinCommands = []string{"help", "clear", "quit", "exit"}

// complete tab-completes a partial "/command". A unique match is filled in with
// a trailing space; multiple matches fill the common prefix and list options.
func (m model) complete() model {
	val := m.input.Value()
	prefix, ok := strings.CutPrefix(val, "/")
	if !ok || strings.Contains(prefix, " ") {
		return m // not a bare command token
	}
	matches := m.matchCommands(prefix)
	switch len(matches) {
	case 0:
		return m
	case 1:
		m.input.SetValue("/" + matches[0] + " ")
	default:
		m.input.SetValue("/" + commonPrefix(matches))
		m.transcript += noteStyle.Render("  "+strings.Join(matches, "  ")) + "\n"
		m.refresh()
	}
	m.input.CursorEnd()
	return m
}

// matchCommands returns built-in and loaded command names sharing the prefix.
func (m model) matchCommands(prefix string) []string {
	var out []string
	for _, name := range append(builtinCommands, m.commands.Names()...) {
		if strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	return out
}

func commonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	p := ss[0]
	for _, s := range ss[1:] {
		for !strings.HasPrefix(s, p) {
			p = p[:len(p)-1]
		}
	}
	return p
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
	m.syncHeight()
	m.remember(val)
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
	m.termW, m.termH = w, h
	if !m.ready {
		m.vp = viewport.New(w, h)
		m.ready = true
	} else {
		m.vp.Width = w
	}
	m.input.SetWidth(w - 4)
	m.refresh()
}

// syncHeight grows the input box with its content, up to maxInputHeight.
func (m *model) syncHeight() {
	n := m.input.LineCount()
	if n < 1 {
		n = 1
	}
	if n > maxInputHeight {
		n = maxInputHeight
	}
	m.inputH = n
	m.input.SetHeight(n)
}

// footerRows is the height reserved below the transcript: a single status line
// while busy or awaiting approval, otherwise the (possibly multi-line) input.
func (m model) footerRows() int {
	if m.busy || m.pending != nil {
		return 1
	}
	return m.inputH
}

func (m *model) refresh() {
	if !m.ready {
		return
	}
	m.vp.Height = m.termH - m.footerRows() - 1 - bannerHeight // -1 separator, banner on top
	if m.vp.Height < 1 {
		m.vp.Height = 1
	}
	atBottom := m.vp.AtBottom()
	m.vp.SetContent(m.transcript + m.cur)
	if atBottom {
		m.vp.GotoBottom() // follow new output only when not scrolled up
	}
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
		footer = busyStyle.Render("…working (esc to cancel, pgup/pgdn to scroll, ctrl+c to quit)")
	default:
		footer = m.input.View()
	}
	return banner(m.frame) + "\n" + m.vp.View() + "\n" + footer
}
