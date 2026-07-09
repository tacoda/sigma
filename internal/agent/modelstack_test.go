package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/message"
)

// flaky fails its first `fails` calls, then succeeds.
type flaky struct {
	fails, calls int
}

func (f *flaky) Stream(_ context.Context, _ message.Request, _ func(string)) (*message.Result, error) {
	f.calls++
	if f.calls <= f.fails {
		return nil, errors.New("boom")
	}
	return textResult("ok"), nil
}

func TestRetryLayer(t *testing.T) {
	// 2 failures then success; 2 retries (3 attempts) recovers.
	c := &flaky{fails: 2}
	res, err := withRetry(2)(c).Stream(context.Background(), message.Request{}, nil)
	if err != nil || res.Text() != "ok" || c.calls != 3 {
		t.Fatalf("retry recover: err=%v calls=%d", err, c.calls)
	}
	// Not enough retries exhausts to error.
	c2 := &flaky{fails: 2}
	if _, err := withRetry(1)(c2).Stream(context.Background(), message.Request{}, nil); err == nil {
		t.Error("expected error when retries are exhausted")
	}
}

type usageLLM struct{ out int }

func (u usageLLM) Stream(_ context.Context, _ message.Request, _ func(string)) (*message.Result, error) {
	return &message.Result{
		Content:    []message.Block{{Type: "text", Text: "x"}},
		StopReason: "end_turn",
		Usage:      message.Usage{OutputTokens: u.out},
	}, nil
}

func TestBudgetLayer(t *testing.T) {
	llm := withBudget(10)(usageLLM{out: 6})
	ctx := context.Background()
	if _, err := llm.Stream(ctx, message.Request{}, nil); err != nil { // spent 6
		t.Fatal(err)
	}
	if _, err := llm.Stream(ctx, message.Request{}, nil); err != nil { // spent 12
		t.Fatal(err)
	}
	// Now over budget: next call is refused before running.
	if _, err := llm.Stream(ctx, message.Request{}, nil); err == nil {
		t.Error("expected token budget to be exceeded")
	}
}

func TestModelStackNames(t *testing.T) {
	if got := strings.Join(ModelStack(Config{}), ","); got != "llm" {
		t.Errorf("default stack = %q, want llm", got)
	}
	got := strings.Join(ModelStack(Config{TokenBudget: 100, LLMRetries: 2}), ",")
	if got != "budget,retry,llm" {
		t.Errorf("full stack = %q, want budget,retry,llm", got)
	}
}
