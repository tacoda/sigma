package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tacoda/sigma/internal/agents"
	"github.com/tacoda/sigma/internal/tools"
	"github.com/tacoda/sigma/internal/workspace"
)

const subagentNote = "\n\nYou are a sub-agent handling a self-contained subtask. Work autonomously and return a concise final answer; you cannot ask the user questions."

// SubagentOptions configures the `task` and `fanout` tools.
type SubagentOptions struct {
	// Tools builds the sub-agent's tools for a workspace root ("" = cwd). Called
	// once per task so isolated tasks get tools rooted at their worktree.
	Tools func(root string) []tools.Tool
	// Types are named sub-agent configurations selectable by name.
	Types agents.Set
	// Isolate runs each `task` in a fresh worktree, merged on success.
	Isolate bool
	// Workspace creates the worktrees when Isolate is set.
	Workspace workspace.Workspace
}

// WithSubagent returns a registry of the (cwd-rooted) tools plus `task` (one
// sub-agent, optionally isolated) and `fanout` (several in parallel). Sub-agents
// have neither tool, preventing unbounded recursion.
func WithSubagent(base Config, opt SubagentOptions) *tools.Registry {
	tmpl := base
	tmpl.System = base.System + subagentNote
	s := spawner{tmpl: tmpl, opt: opt}
	all := append(append([]tools.Tool{}, opt.Tools("")...), taskTool{s}, fanoutTool{s})
	return tools.NewRegistry(all...)
}

var taskCounter atomic.Int64

func nextTaskID() string { return "task-" + strconv.FormatInt(taskCounter.Add(1), 10) }

// spawner builds and runs sub-agents. Shared by the task and fanout tools.
type spawner struct {
	tmpl Config
	opt  SubagentOptions
}

// runChild runs one sub-agent and returns its output. typeName, if set, selects
// an agent type (custom system prompt + tool subset). isolate runs it in a
// worktree, merged on success / discarded on error.
func (s spawner) runChild(ctx context.Context, prompt, typeName string, isolate bool) (string, error) {
	cfg := s.tmpl
	toolset := s.opt.Tools
	if typeName != "" {
		t, ok := s.opt.Types[typeName]
		if !ok {
			return "", fmt.Errorf("unknown agent type %q", typeName)
		}
		if t.System != "" {
			cfg.System = t.System + subagentNote
		}
		allowed := t.Tools
		toolset = func(root string) []tools.Tool { return filterTools(s.opt.Tools(root), allowed) }
	}

	root, wtID, err := s.setup(ctx, isolate)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	cfg.Tools = tools.NewRegistry(toolset(root)...)
	cfg.UI = &captureUI{out: &buf}
	runErr := New(cfg).Run(ctx, prompt)
	return s.teardown(ctx, isolate, wtID, buf.String(), runErr)
}

func (s spawner) isolating(isolate bool) bool {
	return isolate && s.opt.Workspace != nil
}

func (s spawner) setup(ctx context.Context, isolate bool) (root, wtID string, err error) {
	if !s.isolating(isolate) {
		return "", "", nil
	}
	wtID = nextTaskID()
	h, err := s.opt.Workspace.Create(ctx, wtID)
	if err != nil {
		return "", "", fmt.Errorf("create worktree: %w", err)
	}
	return h.Dir, wtID, nil
}

func (s spawner) teardown(ctx context.Context, isolate bool, wtID, out string, runErr error) (string, error) {
	if !s.isolating(isolate) {
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

// runParallel runs each spec's sub-agent concurrently (no isolation) and returns
// the labeled, combined output.
func (s spawner) runParallel(ctx context.Context, specs []taskSpec) string {
	outs := make([]string, len(specs))
	errs := make([]error, len(specs))
	var wg sync.WaitGroup
	for i := range specs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outs[i], errs[i] = s.runChild(ctx, specs[i].Prompt, specs[i].Type, false)
		}(i)
	}
	wg.Wait()

	var b strings.Builder
	for i, spec := range specs {
		label := "task " + strconv.Itoa(i+1)
		if spec.Type != "" {
			label += " [" + spec.Type + "]"
		}
		b.WriteString("## " + label + "\n")
		if errs[i] != nil {
			fmt.Fprintf(&b, "error: %v\n\n", errs[i])
		} else {
			b.WriteString(outs[i] + "\n\n")
		}
	}
	return b.String()
}

func filterTools(all []tools.Tool, names []string) []tools.Tool {
	if len(names) == 0 {
		return all
	}
	keep := make(map[string]bool, len(names))
	for _, n := range names {
		keep[n] = true
	}
	var out []tools.Tool
	for _, t := range all {
		if keep[t.Name()] {
			out = append(out, t)
		}
	}
	return out
}

// taskTool delegates one subtask to a fresh sub-agent.
type taskTool struct{ s spawner }

func (taskTool) Name() string   { return "task" }
func (taskTool) ReadOnly() bool { return false }
func (taskTool) Description() string {
	return "Delegate a self-contained subtask to a fresh sub-agent with its own context. Provide a complete, standalone prompt. Optionally set `type` to a named agent type (custom prompt + tool subset)."
}

func (taskTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {"type": "string", "description": "The complete task for the sub-agent"},
			"type": {"type": "string", "description": "Optional named agent type"}
		},
		"required": ["prompt"]
	}`)
}

func (t taskTool) Run(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Prompt string `json:"prompt"`
		Type   string `json:"type"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Prompt) == "" {
		return "", fmt.Errorf("prompt is required")
	}
	return t.s.runChild(ctx, args.Prompt, args.Type, t.s.opt.Isolate)
}

type taskSpec struct {
	Prompt string `json:"prompt"`
	Type   string `json:"type"`
}

// fanoutTool runs several subtasks in parallel and combines their results.
type fanoutTool struct{ s spawner }

func (fanoutTool) Name() string   { return "fanout" }
func (fanoutTool) ReadOnly() bool { return false }
func (fanoutTool) Description() string {
	return "Run several self-contained subtasks in parallel, each in its own fresh sub-agent, and return their combined results. Best for independent read/analysis work; use read-only agent types to avoid concurrent writes. Does not isolate in worktrees."
}

func (fanoutTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"tasks": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"prompt": {"type": "string"},
						"type": {"type": "string", "description": "Optional named agent type"}
					},
					"required": ["prompt"]
				}
			}
		},
		"required": ["tasks"]
	}`)
}

func (t fanoutTool) Run(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Tasks []taskSpec `json:"tasks"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if len(args.Tasks) == 0 {
		return "", fmt.Errorf("tasks is required")
	}
	for i, spec := range args.Tasks {
		if strings.TrimSpace(spec.Prompt) == "" {
			return "", fmt.Errorf("task %d: prompt is required", i)
		}
	}
	return t.s.runParallel(ctx, args.Tasks), nil
}

// captureUI collects a sub-agent's text output instead of displaying it.
type captureUI struct{ out *strings.Builder }

func (c *captureUI) Text(delta string)               { c.out.WriteString(delta) }
func (c *captureUI) ToolCall(string, string)         {}
func (c *captureUI) ToolResult(string, string, bool) {}
func (c *captureUI) Usage(int, int)                  {}
