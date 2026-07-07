package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/agents"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/tools"
)

// constClient returns the same end-turn result for every call and is safe for
// concurrent use (no mutable state) — needed for the parallel fanout test.
type constClient struct{ text string }

func (c constClient) Stream(_ context.Context, _ message.Request, onText func(string)) (*message.Result, error) {
	if onText != nil {
		onText(c.text)
	}
	return textResult(c.text), nil
}

type nameTool struct{ n string }

func (t nameTool) Name() string                                       { return t.n }
func (nameTool) Description() string                                  { return "" }
func (nameTool) ReadOnly() bool                                       { return true }
func (nameTool) Schema() json.RawMessage                              { return json.RawMessage(`{}`) }
func (nameTool) Run(context.Context, json.RawMessage) (string, error) { return "", nil }

func TestFilterTools(t *testing.T) {
	all := []tools.Tool{nameTool{"read_file"}, nameTool{"bash"}, nameTool{"grep"}}
	got := filterTools(all, []string{"read_file", "grep"})
	if len(got) != 2 || got[0].Name() != "read_file" || got[1].Name() != "grep" {
		t.Errorf("filterTools = %v, want [read_file grep]", names(got))
	}
	if len(filterTools(all, nil)) != 3 {
		t.Error("empty filter should keep all tools")
	}
}

func names(ts []tools.Tool) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Name())
	}
	return out
}

func TestTaskUnknownTypeErrors(t *testing.T) {
	reg := WithSubagent(
		Config{Client: constClient{"x"}},
		SubagentOptions{Tools: func(string) []tools.Tool { return nil }, Types: agents.Set{}},
	)
	if _, err := reg.Run(context.Background(), "task", json.RawMessage(`{"prompt":"p","type":"ghost"}`)); err == nil {
		t.Error("unknown agent type should error")
	}
}

func TestFanoutCombinesParallelResults(t *testing.T) {
	reg := WithSubagent(
		Config{Client: constClient{"SUBRESULT"}},
		SubagentOptions{Tools: func(string) []tools.Tool { return nil }},
	)
	out, err := reg.Run(context.Background(), "fanout",
		json.RawMessage(`{"tasks":[{"prompt":"a"},{"prompt":"b","type":""}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## task 1", "## task 2", "SUBRESULT"} {
		if !strings.Contains(out, want) {
			t.Errorf("fanout output missing %q:\n%s", want, out)
		}
	}
}
