package agent

import (
	"context"

	"github.com/tacoda/sigma/internal/hooks"
)

// The turn spine: a middleware stack around one user turn. Layers wrap the core
// loop, outer to inner:
//
//	compaction -> prompt-gate -> loop
//
// The response and stop gates are per-iteration loop control, not turn wraps, so
// they stay in loop().

type Turn func(ctx context.Context, input string) error

type TurnLayer func(next Turn) Turn

func buildTurn(a *Agent) Turn {
	t := Turn(a.loop)
	// outermost first; applied inner -> outer.
	stack := []TurnLayer{a.compactionLayer(), a.promptGateLayer()}
	for i := len(stack) - 1; i >= 0; i-- {
		t = stack[i](t)
	}
	return t
}

// TurnStack returns the turn spine's layer names, outer to inner.
func TurnStack() []string {
	return []string{"compaction", "prompt-gate", "loop"}
}

// compactionLayer summarizes prior history at the turn boundary when the
// history has grown past the threshold.
func (a *Agent) compactionLayer() TurnLayer {
	return func(next Turn) Turn {
		return func(ctx context.Context, input string) error {
			if a.shouldCompact() {
				a.compact(ctx)
			}
			return next(ctx, input)
		}
	}
}

// promptGateLayer emits UserPromptSubmit; a blocking outcome rejects the prompt
// before any model call.
func (a *Agent) promptGateLayer() TurnLayer {
	return func(next Turn) Turn {
		return func(ctx context.Context, input string) error {
			if o := a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.UserPrompt, Prompt: input}); o.Block {
				a.cfg.UI.Text(promptRejected + o.Reason)
				return nil
			}
			return next(ctx, input)
		}
	}
}
