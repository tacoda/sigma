// Package dryrun is a plugin that adds a tool-spine layer turning mutating tool
// calls into no-ops that report what they would do — a plan preview. Enable it
// with plugins:["dryrun"]. Demonstrates plugins contributing layers, not just
// tools/hooks/sources.
package dryrun

import (
	"context"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/plugin"
)

func init() { plugin.Register(plug{}) }

type plug struct{}

func (plug) Name() string { return "dryrun" }

// mutating tools whose effects dry-run suppresses.
var mutating = map[string]bool{
	"write_file": true,
	"edit_file":  true,
	"bash":       true,
	"worktree":   true,
}

func (plug) Register(h *plugin.Host, _ plugin.Config) error {
	h.AddToolLayer("dry-run", func(next agent.Invoker) agent.Invoker {
		return agent.InvokerFunc(func(ctx context.Context, c agent.ToolCall) (string, error) {
			if mutating[c.Name] {
				return "[dry-run] skipped " + c.Name + " (no changes made)", nil
			}
			return next.Invoke(ctx, c)
		})
	})
	return nil
}
