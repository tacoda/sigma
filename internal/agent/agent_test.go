package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/tools"
)

// fakeClient returns canned results in sequence; once exhausted it repeats the
// last one. It records how many times Stream was called.
type fakeClient struct {
	results []*message.Result
	calls   int
}

func (f *fakeClient) Stream(_ context.Context, _ message.Request, _ func(string)) (*message.Result, error) {
	f.calls++
	i := f.calls - 1
	if i >= len(f.results) {
		i = len(f.results) - 1
	}
	return f.results[i], nil
}

func textResult(s string) *message.Result {
	return &message.Result{
		Content:    []message.Block{{Type: "text", Text: s}},
		StopReason: "end_turn",
	}
}

func toolUseResult(name string) *message.Result {
	return &message.Result{
		Content:    []message.Block{{Type: "tool_use", ID: "u1", Name: name, Input: json.RawMessage(`{}`)}},
		StopReason: "tool_use",
	}
}

// recordTool is a no-op read-only tool that records each call.
type recordTool struct{ runs *int }

func (recordTool) Name() string            { return "noop" }
func (recordTool) Description() string     { return "noop" }
func (recordTool) ReadOnly() bool          { return true }
func (recordTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t recordTool) Run(context.Context, json.RawMessage) (string, error) {
	*t.runs++
	return "ok", nil
}

func newAgent(t *testing.T, client LLM, runs *int) *Agent {
	t.Helper()
	return New(Config{
		Client: client,
		Tools:  tools.NewRegistry(recordTool{runs: runs}),
		UI:     noopUI{},
	})
}

type noopUI struct{}

func (noopUI) Text(string)                     {}
func (noopUI) ToolCall(string, string)         {}
func (noopUI) ToolResult(string, string, bool) {}
func (noopUI) Usage(int, int)                  {}

func TestRunLoop(t *testing.T) {
	cases := []struct {
		name                string
		results             []*message.Result
		wantCalls, wantRuns int
	}{
		{"final answer, no tools", []*message.Result{textResult("done")}, 1, 0},
		{"tool then finish", []*message.Result{toolUseResult("noop"), textResult("done")}, 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var runs int
			client := &fakeClient{results: tc.results}
			a := newAgent(t, client, &runs)

			if err := a.Run(context.Background(), "hi"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if client.calls != tc.wantCalls {
				t.Errorf("calls = %d, want %d", client.calls, tc.wantCalls)
			}
			if runs != tc.wantRuns {
				t.Errorf("tool runs = %d, want %d", runs, tc.wantRuns)
			}
		})
	}
}

func TestRunCapsToolIterations(t *testing.T) {
	var runs int
	client := &fakeClient{results: []*message.Result{toolUseResult("noop")}} // always tool_use
	a := newAgent(t, client, &runs)

	err := a.Run(context.Background(), "hi")
	if err == nil {
		t.Fatal("Run: want error at iteration cap, got nil")
	}
	if client.calls != maxIterations {
		t.Errorf("calls = %d, want %d", client.calls, maxIterations)
	}
}

func TestRunRespectsContextCancellation(t *testing.T) {
	var runs int
	client := &fakeClient{results: []*message.Result{toolUseResult("noop")}}
	a := newAgent(t, client, &runs)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := a.Run(ctx, "hi")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if client.calls != 0 {
		t.Errorf("calls = %d, want 0 (cancelled before any request)", client.calls)
	}
}
