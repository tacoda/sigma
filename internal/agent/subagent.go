package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/tacoda/sigma/internal/tools"
	"github.com/tacoda/sigma/internal/workspace"
)

const subagentNote = "\n\nYou are a sub-agent handling a self-contained subtask. Work autonomously and return a concise final answer; you cannot ask the user questions."

// SubagentOptions configures the `task` tool.
type SubagentOptions struct {
	// Tools builds the sub-agent's tools for a workspace root ("" means the
	// process cwd). It is called once per task so isolated tasks get tools
	// rooted at their worktree.
	Tools func(root string) []tools.Tool
	// Isolate runs each task in a fresh git worktree, merged back on success
	// and discarded on error.
	Isolate bool
	// Workspace creates the worktrees when Isolate is set.
	Workspace workspace.Workspace
}

// WithSubagent returns a registry containing the (cwd-rooted) tools plus a
// `task` tool that spawns a fresh sub-agent. The sub-agent has no `task` tool,
// preventing unbounded recursion, and inherits base (client, permission, hooks,
// model, system); its Tools and UI are set per task.
func WithSubagent(base Config, opt SubagentOptions) *tools.Registry {
	tmpl := base
	tmpl.System = base.System + subagentNote

	child := subagentTool{tmpl: tmpl, opt: opt}
	all := append(append([]tools.Tool{}, opt.Tools("")...), child)
	return tools.NewRegistry(all...)
}

var taskCounter atomic.Int64

func nextTaskID() string { return "task-" + strconv.FormatInt(taskCounter.Add(1), 10) }

// subagentTool delegates a subtask to a fresh agent and returns its output.
type subagentTool struct {
	tmpl Config
	opt  SubagentOptions
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

	root, wtID, err := s.setup(ctx)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	cfg := s.tmpl
	cfg.Tools = tools.NewRegistry(s.opt.Tools(root)...)
	cfg.UI = &captureUI{out: &buf}
	runErr := New(cfg).Run(ctx, args.Prompt)

	return s.teardown(ctx, wtID, buf.String(), runErr)
}

// setup creates a worktree when isolating and returns its root and id.
func (s subagentTool) setup(ctx context.Context) (root, wtID string, err error) {
	if !s.isolating() {
		return "", "", nil
	}
	wtID = nextTaskID()
	h, err := s.opt.Workspace.Create(ctx, wtID)
	if err != nil {
		return "", "", fmt.Errorf("create worktree: %w", err)
	}
	return h.Dir, wtID, nil
}

// teardown merges the worktree on success or discards it on error.
func (s subagentTool) teardown(ctx context.Context, wtID, out string, runErr error) (string, error) {
	if !s.isolating() {
		if runErr != nil {
			return "", runErr
		}
		return out, nil
	}
	if runErr != nil {
		_ = s.opt.Workspace.Discard(ctx, wtID)
		return "", runErr
	}
	if err := s.opt.Workspace.Merge(ctx, wtID); err != nil {
		return out + fmt.Sprintf("\n\n[worktree %s left for manual merge: %v]", wtID, err), nil
	}
	return out, nil
}

func (s subagentTool) isolating() bool { return s.opt.Isolate && s.opt.Workspace != nil }

// captureUI collects a sub-agent's text output instead of displaying it.
type captureUI struct{ out *strings.Builder }

func (c *captureUI) Text(delta string)               { c.out.WriteString(delta) }
func (c *captureUI) ToolCall(string, string)         {}
func (c *captureUI) ToolResult(string, string, bool) {}
func (c *captureUI) Usage(int, int)                  {}
