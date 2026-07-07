package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/tools"
)

// kindGate blocks the first `block` events of a given kind, then allows.
type kindGate struct {
	kind        hooks.Kind
	block, seen int
	reason      string
}

func (g *kindGate) Emit(_ context.Context, ev hooks.Event) hooks.Outcome {
	if ev.Kind == g.kind {
		g.seen++
		if g.seen <= g.block {
			return hooks.Outcome{Block: true, Reason: g.reason}
		}
	}
	return hooks.Outcome{}
}

func gateAgent(gate hooks.Bus) (*Agent, *fakeClient, *recordUI) {
	client := &fakeClient{results: []*message.Result{textResult("done")}}
	ui := &recordUI{}
	a := New(Config{Client: client, Tools: tools.NewRegistry(), UI: ui, Hooks: gate})
	return a, client, ui
}

// recordUI captures streamed text.
type recordUI struct{ text string }

func (u *recordUI) Text(s string)                   { u.text += s }
func (u *recordUI) ToolCall(string, string)         {}
func (u *recordUI) ToolResult(string, string, bool) {}
func (u *recordUI) Usage(int, int)                  {}

func TestUserPromptGateRejects(t *testing.T) {
	a, client, ui := gateAgent(&kindGate{kind: hooks.UserPrompt, block: 1, reason: "off topic"})

	if err := a.Run(context.Background(), "do bad thing"); err != nil {
		t.Fatal(err)
	}
	if client.calls != 0 {
		t.Errorf("model called %d times, want 0 (prompt rejected before any call)", client.calls)
	}
	if !strings.Contains(ui.text, "off topic") {
		t.Errorf("rejection reason not surfaced: %q", ui.text)
	}
	if len(a.Snapshot()) != 0 {
		t.Errorf("rejected prompt should not enter history, got %d messages", len(a.Snapshot()))
	}
}

func TestResponseGateRetriesThenPasses(t *testing.T) {
	a, client, _ := gateAgent(&kindGate{kind: hooks.PostLLM, block: 2, reason: "too vague"})

	if err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if client.calls != 3 {
		t.Errorf("Stream calls = %d, want 3 (2 rejected responses + 1 accepted)", client.calls)
	}
	var fed int
	for _, m := range a.Snapshot() {
		if m.Role == "user" && strings.Contains(m.Content[0].Text, "too vague") {
			fed++
		}
	}
	if fed != 2 {
		t.Errorf("response feedback injected %d times, want 2", fed)
	}
}

func TestResponseGateGivesUpAfterCap(t *testing.T) {
	a, client, _ := gateAgent(&kindGate{kind: hooks.PostLLM, block: 99, reason: "nope"})

	err := a.Run(context.Background(), "hi")
	if err == nil {
		t.Fatal("want error when response gate never passes")
	}
	if !strings.Contains(err.Error(), "response gate") {
		t.Errorf("err = %v, want response-gate message", err)
	}
	if client.calls != maxGateRetries+1 {
		t.Errorf("Stream calls = %d, want %d", client.calls, maxGateRetries+1)
	}
}
