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

// ToolCall is a request to run a tool, flowing through the tool spine.
type ToolCall struct {
	Name  string
	Input json.RawMessage
}

// Invoker runs a tool call. Layers wrap it.
type Invoker interface {
	Invoke(ctx context.Context, c ToolCall) (string, error)
}

// InvokerFunc adapts a function to Invoker (for layer authors).
type InvokerFunc func(ctx context.Context, c ToolCall) (string, error)

func (f InvokerFunc) Invoke(ctx context.Context, c ToolCall) (string, error) { return f(ctx, c) }

// ToolLayer wraps an Invoker with cross-cutting behavior. Plugins and charters
// contribute these; a layer may observe, transform, short-circuit, or wrap.
type ToolLayer func(next Invoker) Invoker

// buildInvoker composes the tool spine: the built-in stack, then any extra
// layers (from plugins/charter) wrapping it outermost, in order.
func buildInvoker(cfg Config) Invoker {
	inv := execLayer(cfg.Tools)
	builtin := []ToolLayer{
		uiCall(cfg.UI),
		preHook(cfg.Hooks),
		permissionLayer(cfg.Tools, cfg.Permission),
		uiResult(cfg.UI),
		postHook(cfg.Hooks),
	}
	for i := len(builtin) - 1; i >= 0; i-- {
		inv = builtin[i](inv)
	}
	// cfg.ToolLayers[0] is the outermost wrapper.
	for i := len(cfg.ToolLayers) - 1; i >= 0; i-- {
		inv = cfg.ToolLayers[i](inv)
	}
	return inv
}

// ToolStack returns the tool spine's layer names, outer to inner, for
// introspection (`sigma layers`).
func ToolStack() []string {
	return []string{"ui", "hooks", "permission", "exec"}
}

func execLayer(reg *tools.Registry) Invoker {
	return InvokerFunc(func(ctx context.Context, c ToolCall) (string, error) {
		return reg.Run(ctx, c.Name, c.Input)
	})
}

func uiCall(ui UI) ToolLayer {
	return func(next Invoker) Invoker {
		return InvokerFunc(func(ctx context.Context, c ToolCall) (string, error) {
			ui.ToolCall(c.Name, string(c.Input))
			return next.Invoke(ctx, c)
		})
	}
}

func uiResult(ui UI) ToolLayer {
	return func(next Invoker) Invoker {
		return InvokerFunc(func(ctx context.Context, c ToolCall) (string, error) {
			out, err := next.Invoke(ctx, c)
			ui.ToolResult(c.Name, out, err != nil)
			return out, err
		})
	}
}

// preHook emits PreToolUse; a blocking outcome short-circuits with errBlocked.
func preHook(bus hooks.Bus) ToolLayer {
	return func(next Invoker) Invoker {
		return InvokerFunc(func(ctx context.Context, c ToolCall) (string, error) {
			if o := bus.Emit(ctx, hooks.Event{Kind: hooks.PreTool, Tool: c.Name, Input: string(c.Input)}); o.Block {
				return o.Reason, errBlocked
			}
			return next.Invoke(ctx, c)
		})
	}
}

// postHook emits PostToolUse (and ToolError on failure) after execution.
func postHook(bus hooks.Bus) ToolLayer {
	return func(next Invoker) Invoker {
		return InvokerFunc(func(ctx context.Context, c ToolCall) (string, error) {
			out, err := next.Invoke(ctx, c)
			bus.Emit(ctx, hooks.Event{Kind: hooks.PostTool, Tool: c.Name, Output: out})
			if err != nil {
				bus.Emit(ctx, hooks.Event{Kind: hooks.ToolError, Tool: c.Name, Output: out})
			}
			return out, err
		})
	}
}

// permissionLayer denies a mutating tool the policy rejects. A read-only tool or
// a nil policy passes through.
func permissionLayer(reg *tools.Registry, p PermissionPolicy) ToolLayer {
	return func(next Invoker) Invoker {
		return InvokerFunc(func(ctx context.Context, c ToolCall) (string, error) {
			if !reg.ReadOnly(c.Name) && p != nil && !p.Allow(c.Name, string(c.Input)) {
				return "", errDenied
			}
			return next.Invoke(ctx, c)
		})
	}
}
