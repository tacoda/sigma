package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/tacoda/sigma/internal/message"
)

// The model spine: a middleware stack around the LLM port. Layers wrap the
// client used for every model call (the turn loop and compaction).
//
//	budget -> retry -> llm
//
// Both layers are opt-in (config), so the default stack is just the client.
//
// Note: PreLLMRequest/PostLLMResponse emits and the response gate stay in the
// turn loop — the gate acts on the PostLLM outcome, which the LLM interface
// (result + error only) can't carry. Prompt caching lives in the anthropic
// adapter. Those are turn-spine / adapter concerns, not model layers.

type LLMLayer func(LLM) LLM

// buildLLM composes the model spine for a config.
func buildLLM(cfg Config) LLM {
	llm := cfg.Client
	if llm == nil {
		return nil
	}
	if cfg.LLMRetries > 0 {
		llm = withRetry(cfg.LLMRetries)(llm)
	}
	if cfg.TokenBudget > 0 {
		llm = withBudget(cfg.TokenBudget)(llm)
	}
	return llm
}

// ModelStack returns the model spine's layer names, outer to inner.
func ModelStack(cfg Config) []string {
	var s []string
	if cfg.TokenBudget > 0 {
		s = append(s, "budget")
	}
	if cfg.LLMRetries > 0 {
		s = append(s, "retry")
	}
	return append(s, "llm")
}

// withRetry retries a failed Stream up to n extra times (immediate; not for
// context cancellation).
func withRetry(n int) LLMLayer {
	return func(next LLM) LLM { return retryLLM{next: next, n: n} }
}

type retryLLM struct {
	next LLM
	n    int
}

func (r retryLLM) Stream(ctx context.Context, req message.Request, onText func(string)) (*message.Result, error) {
	var res *message.Result
	var err error
	for i := 0; i <= r.n; i++ {
		res, err = r.next.Stream(ctx, req, onText)
		if err == nil || ctx.Err() != nil {
			return res, err
		}
	}
	return res, err
}

// withBudget short-circuits once cumulative tokens (in+out) reach limit.
func withBudget(limit int) LLMLayer {
	return func(next LLM) LLM { return &budgetLLM{next: next, limit: limit} }
}

type budgetLLM struct {
	next  LLM
	limit int
	mu    sync.Mutex
	spent int
}

func (b *budgetLLM) Stream(ctx context.Context, req message.Request, onText func(string)) (*message.Result, error) {
	b.mu.Lock()
	over := b.spent >= b.limit
	b.mu.Unlock()
	if over {
		return nil, fmt.Errorf("token budget exceeded (%d tokens)", b.limit)
	}
	res, err := b.next.Stream(ctx, req, onText)
	if err == nil && res != nil {
		b.mu.Lock()
		b.spent += res.Usage.InputTokens + res.Usage.OutputTokens
		b.mu.Unlock()
	}
	return res, err
}
