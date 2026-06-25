package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tacoda/sigma/internal/tools"
)

const subagentNote = "\n\nYou are a sub-agent handling a self-contained subtask. Work autonomously and return a concise final answer; you cannot ask the user questions."

// WithSubagent returns a registry containing childTools plus a `task` tool that
// spawns a fresh sub-agent over those same childTools (the sub-agent itself has
// no `task` tool, preventing unbounded recursion). The sub-agent inherits base
// (client, approver, hooks, model, system); its Tools and UI are set internally.
func WithSubagent(base Config, childTools []tools.Tool) *tools.Registry {
	tmpl := base
	tmpl.Tools = tools.NewRegistry(childTools...)
	tmpl.System = base.System + subagentNote

	child := subagentTool{tmpl: tmpl}
	all := append(append([]tools.Tool{}, childTools...), child)
	return tools.NewRegistry(all...)
}

// subagentTool delegates a subtask to a fresh agent and returns its output.
type subagentTool struct {
	tmpl Config // UI is filled in per call
}

func (subagentTool) Name() string   { return "task" }
func (subagentTool) ReadOnly() bool { return false }
func (subagentTool) Description() string {
	return "Delegate a self-contained subtask to a fresh sub-agent with its own context. Provide a complete, standalone prompt; the sub-agent returns a final answer."
}

func (subagentTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {"type": "string", "description": "The complete task for the sub-agent"}
		},
		"required": ["prompt"]
	}`)
}

func (s subagentTool) Run(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return "", fmt.Errorf("prompt is required")
	}
	var buf strings.Builder
	cfg := s.tmpl
	cfg.UI = &captureUI{out: &buf}
	child := New(cfg)
	if err := child.Run(ctx, args.Prompt); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// captureUI collects a sub-agent's text output instead of displaying it.
type captureUI struct{ out *strings.Builder }

func (c *captureUI) Text(delta string)       { c.out.WriteString(delta) }
func (c *captureUI) ToolCall(string, string) {}
