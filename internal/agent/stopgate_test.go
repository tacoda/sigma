package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/tools"
)

// stopGate blocks the first `block` Stop events, then allows.
type stopGate struct {
	block, seen int
}

func (g *stopGate) Emit(_ context.Context, ev hooks.Event) hooks.Outcome {
	if ev.Kind == hooks.Stop {
		g.seen++
		if g.seen <= g.block {
			return hooks.Outcome{Block: true, Reason: "tests failed"}
		}
	}
	return hooks.Outcome{}
}

func stopGateAgent(gate hooks.Bus) (*Agent, *fakeClient) {
	// fakeClient repeats its last result, so every turn ends (end_turn).
	client := &fakeClient{results: []*message.Result{textResult("done")}}
	a := New(Config{Client: client, Tools: tools.NewRegistry(), UI: noopUI{}, Hooks: gate})
	return a, client
}

func TestStopGateRetriesThenPasses(t *testing.T) {
	a, client := stopGateAgent(&stopGate{block: 2})

	if err := a.Run(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	// 2 blocked stops force 2 extra turns: 3 model calls total.
	if client.calls != 3 {
		t.Errorf("Stream calls = %d, want 3", client.calls)
	}
	// The failure reason was fed back to the model.
	var fed int
	for _, m := range a.Snapshot() {
		if m.Role == "user" && strings.Contains(m.Content[0].Text, "tests failed") {
			fed++
		}
	}
	if fed != 2 {
		t.Errorf("validation feedback injected %d times, want 2", fed)
	}
}

func TestStopGateGivesUpAfterCap(t *testing.T) {
	a, client := stopGateAgent(&stopGate{block: 99})

	err := a.Run(context.Background(), "hi")
	if err == nil {
		t.Fatal("want error when the gate never passes")
	}
	if !strings.Contains(err.Error(), "validation gate") {
		t.Errorf("err = %v, want validation-gate message", err)
	}
	// initial turn + maxStopRetries retries.
	if client.calls != maxStopRetries+1 {
		t.Errorf("Stream calls = %d, want %d", client.calls, maxStopRetries+1)
	}
}
