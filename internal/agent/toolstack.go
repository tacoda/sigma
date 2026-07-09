package agent

import (
	"context"
	"encoding/json"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/tools"
)

// The tool spine: a middleware stack around tool execution. Each layer wraps the
// next; the innermost (exec) dispatches to the registry. Execution order,
// outer -> inner:
//
//	ui:call -> hooks:pre -> permission -> ui:result -> hooks:post -> exec
//
// so a hook block skips permission and execution, and a permission deny skips
// execution, PostToolUse, and the UI result — matching the pre-layer behavior.

type toolCall struct {
	name  string
	input json.RawMessage
}

type invoker interface {
	invoke(ctx context.Context, c toolCall) (string, error)
}

type invokerFunc func(ctx context.Context, c toolCall) (string, error)

func (f invokerFunc) invoke(ctx context.Context, c toolCall) (string, error) { return f(ctx, c) }

type toolLayer func(next invoker) invoker

// buildInvoker composes the tool spine for a config.
func buildInvoker(cfg Config) invoker {
	inv := execLayer(cfg.Tools)
	// outermost first; applied inner -> outer.
	stack := []toolLayer{
		uiCall(cfg.UI),
		preHook(cfg.Hooks),
		permissionLayer(cfg.Tools, cfg.Permission),
		uiResult(cfg.UI),
		postHook(cfg.Hooks),
	}
	for i := len(stack) - 1; i >= 0; i-- {
		inv = stack[i](inv)
	}
	return inv
}

// ToolStack returns the tool spine's layer names, outer to inner, for
// introspection (`sigma layers`).
func ToolStack() []string {
	return []string{"ui", "hooks", "permission", "exec"}
}

func execLayer(reg *tools.Registry) invoker {
	return invokerFunc(func(ctx context.Context, c toolCall) (string, error) {
		return reg.Run(ctx, c.name, c.input)
	})
}

func uiCall(ui UI) toolLayer {
	return func(next invoker) invoker {
		return invokerFunc(func(ctx context.Context, c toolCall) (string, error) {
			ui.ToolCall(c.name, string(c.input))
			return next.invoke(ctx, c)
		})
	}
}

func uiResult(ui UI) toolLayer {
	return func(next invoker) invoker {
		return invokerFunc(func(ctx context.Context, c toolCall) (string, error) {
			out, err := next.invoke(ctx, c)
			ui.ToolResult(c.name, out, err != nil)
			return out, err
		})
	}
}

// preHook emits PreToolUse; a blocking outcome short-circuits with errBlocked.
func preHook(bus hooks.Bus) toolLayer {
	return func(next invoker) invoker {
		return invokerFunc(func(ctx context.Context, c toolCall) (string, error) {
			if o := bus.Emit(ctx, hooks.Event{Kind: hooks.PreTool, Tool: c.name, Input: string(c.input)}); o.Block {
				return o.Reason, errBlocked
			}
			return next.invoke(ctx, c)
		})
	}
}

// postHook emits PostToolUse (and ToolError on failure) after execution.
func postHook(bus hooks.Bus) toolLayer {
	return func(next invoker) invoker {
		return invokerFunc(func(ctx context.Context, c toolCall) (string, error) {
			out, err := next.invoke(ctx, c)
			bus.Emit(ctx, hooks.Event{Kind: hooks.PostTool, Tool: c.name, Output: out})
			if err != nil {
				bus.Emit(ctx, hooks.Event{Kind: hooks.ToolError, Tool: c.name, Output: out})
			}
			return out, err
		})
	}
}

// permissionLayer denies a mutating tool the policy rejects. A read-only tool or
// a nil policy passes through.
func permissionLayer(reg *tools.Registry, p PermissionPolicy) toolLayer {
	return func(next invoker) invoker {
		return invokerFunc(func(ctx context.Context, c toolCall) (string, error) {
			if !reg.ReadOnly(c.name) && p != nil && !p.Allow(c.name, string(c.input)) {
				return "", errDenied
			}
			return next.invoke(ctx, c)
		})
	}
}
