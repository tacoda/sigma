package eval

import (
	"context"
	"strings"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/app"
	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/permission"
	"github.com/tacoda/sigma/internal/tools"
	"github.com/tacoda/sigma/internal/workspace"
)

// runAgent builds the agent from the variant's charter (via the app composition
// root, so plugins/hooks/prompt from that charter apply) and runs it against the
// case with the given client in a fresh scratch workspace, capturing output and
// the event trace. Replay passes a transcript-replaying client; live passes a
// recording wrapper over the real model.
func runAgent(ctx context.Context, client agent.LLM, charter string, c Case) (Result, error) {
	dir, err := scratch(c.Setup)
	if err != nil {
		return Result{}, err
	}
	d, err := app.Build(app.Options{Client: client, ConfigRoot: charter})
	if err != nil {
		return Result{Dir: dir}, err
	}
	defer d.Cleanup()

	tr := &traceRecorder{}
	var out strings.Builder
	base := agent.Config{
		Client:      client,
		Permission:  permission.ForMode(permission.Bypass, nil),
		Hooks:       append(hooks.Multi{tr}, d.Bus), // capture the trace + run the charter's hooks/guards
		Model:       d.Model,
		System:      d.System,
		CompactAt:   d.CompactAt,
		TokenBudget: d.TokenBudget,
		LLMRetries:  d.LLMRetries,
		ToolLayers:  d.ToolLayers,
		UI:          &captureUI{b: &out},
	}
	newTools := func(root string) []tools.Tool {
		if root == "" {
			root = dir // root the top agent's file tools at the scratch workspace
		}
		return d.NewTools(root)
	}
	base.Tools = agent.WithSubagent(base, agent.SubagentOptions{
		Tools:     newTools,
		Types:     d.Types,
		Workflows: d.Workflows,
		Isolate:   false, // deterministic scratch, not worktree-isolated
		Workspace: workspace.Git{},
	})
	a := agent.New(base)

	base.Hooks.Emit(ctx, hooks.Event{Kind: hooks.SessionStart})
	runErr := a.Run(ctx, c.Prompt)
	base.Hooks.Emit(ctx, hooks.Event{Kind: hooks.SessionEnd})

	return Result{Output: out.String(), Trace: tr.events, Dir: dir, Err: runErr}, nil
}
