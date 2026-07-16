package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/message"
)

func TestWireOfFlattensToolResults(t *testing.T) {
	req := message.Request{
		Model:  "gpt-4o",
		System: "be terse",
		Messages: []message.Message{
			message.UserText("hi"),
			{Role: "assistant", Content: []message.Block{
				{Type: "tool_use", ID: "call_1", Name: "ls", Input: json.RawMessage(`{"p":"."}`)},
			}},
			{Role: "user", Content: []message.Block{
				{Type: "tool_result", ToolUseID: "call_1", Content: "file.go"},
			}},
		},
	}
	w := wireOf(req)

	// system + user + assistant(tool_calls) + tool
	if len(w.Messages) != 4 {
		t.Fatalf("got %d messages, want 4", len(w.Messages))
	}
	if w.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want system", w.Messages[0].Role)
	}
	asst := w.Messages[2]
	if asst.Role != "assistant" || len(asst.ToolCalls) != 1 || asst.ToolCalls[0].ID != "call_1" {
		t.Errorf("assistant tool call not converted: %+v", asst)
	}
	tool := w.Messages[3]
	if tool.Role != "tool" || tool.ToolCallID != "call_1" || tool.Content != "file.go" {
		t.Errorf("tool_result not converted: %+v", tool)
	}
}

func TestParseStreamAccumulatesToolCall(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hi "},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"content":"there"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_9","function":{"name":"ls","arguments":"{\"p\""}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\".\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		"data: [DONE]",
	}, "\n")

	res, err := parseStream(strings.NewReader(sse), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "hi there" {
		t.Errorf("text = %q, want %q", res.Text(), "hi there")
	}
	if res.StopReason != "tool_use" {
		t.Errorf("stop reason = %q, want tool_use", res.StopReason)
	}
	uses := res.ToolUses()
	if len(uses) != 1 || uses[0].Name != "ls" || string(uses[0].Input) != `{"p":"."}` {
		t.Errorf("tool use not accumulated: %+v", uses)
	}
	if res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want in=10 out=5", res.Usage)
	}
}
