package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/tools"
)

// compactBus counts PreCompact / PostCompact events.
type compactBus struct{ pre, post int }

func (b *compactBus) Emit(_ context.Context, ev hooks.Event) hooks.Outcome {
	switch ev.Kind {
	case hooks.PreCompact:
		b.pre++
	case hooks.PostCompact:
		b.post++
	}
	return hooks.Outcome{}
}

func bigResult(text string, inTokens int) *message.Result {
	return &message.Result{
		Content:    []message.Block{{Type: "text", Text: text}},
		StopReason: "end_turn",
		Usage:      message.Usage{InputTokens: inTokens},
	}
}

func TestCompactsWhenOverThreshold(t *testing.T) {
	// turn 1 reports a large input; the summary call; turn 2.
	client := &fakeClient{results: []*message.Result{
		bigResult("first answer", 500),
		textResult("SUMMARY of prior work"),
		textResult("second answer"),
	}}
	bus := &compactBus{}
	a := New(Config{
		Client: client, Tools: tools.NewRegistry(), UI: noopUI{},
		Hooks: bus, Model: "m", CompactAt: 100,
	})

	if err := a.Run(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	// lastInput (500) >= CompactAt (100): the next turn compacts first.
	if err := a.Run(context.Background(), "second"); err != nil {
		t.Fatal(err)
	}

	if client.calls != 3 {
		t.Errorf("Stream calls = %d, want 3 (turn1 + summary + turn2)", client.calls)
	}
	if bus.pre != 1 || bus.post != 1 {
		t.Errorf("compact events pre=%d post=%d, want 1/1", bus.pre, bus.post)
	}
	// History was replaced with the summary; the original messages are gone.
	msgs := a.Snapshot()
	if !strings.Contains(msgs[0].Content[0].Text, "SUMMARY of prior work") {
		t.Errorf("first message should be the summary, got %q", msgs[0].Content[0].Text)
	}
	for _, m := range msgs {
		if strings.Contains(m.Content[0].Text, "first answer") {
			t.Error("pre-compaction content should be gone from history")
		}
	}
}

func TestNoCompactionUnderThreshold(t *testing.T) {
	client := &fakeClient{results: []*message.Result{
		bigResult("a", 50),
		textResult("b"),
	}}
	bus := &compactBus{}
	a := New(Config{
		Client: client, Tools: tools.NewRegistry(), UI: noopUI{},
		Hooks: bus, Model: "m", CompactAt: 100,
	})

	_ = a.Run(context.Background(), "first")
	_ = a.Run(context.Background(), "second")

	if bus.pre != 0 {
		t.Errorf("should not compact under threshold (pre=%d)", bus.pre)
	}
	if client.calls != 2 {
		t.Errorf("Stream calls = %d, want 2 (no summary call)", client.calls)
	}
}
