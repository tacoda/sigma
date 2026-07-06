package agent_test

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tacoda/sigma/internal/message"
)

// fakeStreamer replays a scripted sequence of results, one per Stream call, and
// records the requests it received. It simulates streaming by pushing each text
// block through onText before returning.
type fakeStreamer struct {
	script []*message.Result
	calls  int
	reqs   []message.Request
}

func (f *fakeStreamer) Stream(_ context.Context, req message.Request, onText func(string)) (*message.Result, error) {
	f.reqs = append(f.reqs, req)
	if f.calls >= len(f.script) {
		return nil, fmt.Errorf("fakeStreamer: no scripted result for call %d", f.calls+1)
	}
	res := f.script[f.calls]
	f.calls++
	if onText != nil {
		for _, b := range res.Content {
			if b.Type == "text" && b.Text != "" {
				onText(b.Text)
			}
		}
	}
	return res, nil
}

// fakeUI records everything the agent emits.
type fakeUI struct {
	text  string
	calls []string // "name input" per ToolCall
}

func (u *fakeUI) Text(delta string)               { u.text += delta }
func (u *fakeUI) ToolCall(name, input string)     { u.calls = append(u.calls, name+" "+input) }
func (u *fakeUI) ToolResult(string, string, bool) {}
func (u *fakeUI) Usage(int, int)                  {}

// allowApprover approves every tool.
type allowApprover struct{ asked []string }

func (a *allowApprover) Allow(name, _ string) bool {
	a.asked = append(a.asked, name)
	return true
}

// echoTool is a mutating tool that returns its "msg" argument, recording inputs.
type echoTool struct{ inputs []string }

func (t *echoTool) Name() string        { return "echo" }
func (t *echoTool) Description() string { return "echoes the msg argument" }
func (t *echoTool) ReadOnly() bool      { return false }
func (t *echoTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}},"required":["msg"]}`)
}

func (t *echoTool) Run(_ context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	t.inputs = append(t.inputs, in.Msg)
	return in.Msg, nil
}

// helpers to build scripted results.

func toolUseResult(text, toolID, toolName, inputJSON string) *message.Result {
	content := []message.Block{}
	if text != "" {
		content = append(content, message.Block{Type: "text", Text: text})
	}
	content = append(content, message.Block{
		Type:  "tool_use",
		ID:    toolID,
		Name:  toolName,
		Input: json.RawMessage(inputJSON),
	})
	return &message.Result{Content: content, StopReason: "tool_use"}
}

func endTurnResult(text string) *message.Result {
	return &message.Result{
		Content:    []message.Block{{Type: "text", Text: text}},
		StopReason: "end_turn",
	}
}
