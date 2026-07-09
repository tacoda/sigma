package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/tools"
)

type logUI struct{ log *[]string }

func (u logUI) Text(string) {}
func (u logUI) ToolCall(name, _ string) {
	*u.log = append(*u.log, "call:"+name)
}
func (u logUI) ToolResult(name string, _ string, isErr bool) {
	*u.log = append(*u.log, "result:"+name)
}
func (u logUI) Usage(int, int) {}

type logBus struct {
	log       *[]string
	blockKind hooks.Kind
}

func (b logBus) Emit(_ context.Context, ev hooks.Event) hooks.Outcome {
	*b.log = append(*b.log, "hook:"+string(ev.Kind))
	if ev.Kind == b.blockKind {
		return hooks.Outcome{Block: true, Reason: "nope"}
	}
	return hooks.Outcome{}
}

type spyPerm struct{ allow bool }

func (p spyPerm) Allow(string, string) bool { return p.allow }

type mutTool struct{ ran *bool }

func (mutTool) Name() string            { return "mut" }
func (mutTool) Description() string     { return "" }
func (mutTool) ReadOnly() bool          { return false }
func (mutTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t mutTool) Run(context.Context, json.RawMessage) (string, error) {
	*t.ran = true
	return "ok", nil
}

func invokeMut(cfg Config) (string, error) {
	return buildInvoker(cfg).invoke(context.Background(), toolCall{name: "mut", input: json.RawMessage(`{}`)})
}

func TestToolSpineNormalOrder(t *testing.T) {
	var log []string
	ran := false
	out, err := invokeMut(Config{
		Tools:      tools.NewRegistry(mutTool{&ran}),
		UI:         logUI{&log},
		Hooks:      logBus{log: &log},
		Permission: spyPerm{allow: true},
	})
	if err != nil || out != "ok" || !ran {
		t.Fatalf("normal run: out=%q err=%v ran=%v", out, err, ran)
	}
	got := strings.Join(log, ",")
	want := "call:mut,hook:PreToolUse,hook:PostToolUse,result:mut"
	if got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
}

func TestToolSpineHookBlock(t *testing.T) {
	var log []string
	ran := false
	out, err := invokeMut(Config{
		Tools:      tools.NewRegistry(mutTool{&ran}),
		UI:         logUI{&log},
		Hooks:      logBus{log: &log, blockKind: hooks.PreTool},
		Permission: spyPerm{allow: true},
	})
	if !errors.Is(err, errBlocked) || ran {
		t.Fatalf("block: err=%v ran=%v", err, ran)
	}
	if out != "nope" {
		t.Errorf("block reason = %q", out)
	}
	// No PostToolUse, no result after a block.
	if strings.Contains(strings.Join(log, ","), "PostToolUse") || strings.Contains(strings.Join(log, ","), "result:") {
		t.Errorf("block should skip post-hook and ui result: %v", log)
	}
}

func TestToolSpinePermissionDeny(t *testing.T) {
	var log []string
	ran := false
	_, err := invokeMut(Config{
		Tools:      tools.NewRegistry(mutTool{&ran}),
		UI:         logUI{&log},
		Hooks:      logBus{log: &log},
		Permission: spyPerm{allow: false},
	})
	if !errors.Is(err, errDenied) || ran {
		t.Fatalf("deny: err=%v ran=%v", err, ran)
	}
	// PreToolUse fired, but no PostToolUse and no ui result on deny.
	j := strings.Join(log, ",")
	if !strings.Contains(j, "PreToolUse") || strings.Contains(j, "PostToolUse") || strings.Contains(j, "result:") {
		t.Errorf("deny path wrong: %v", log)
	}
}

func TestToolSpineReadOnlyBypassesPermission(t *testing.T) {
	var log []string
	var runs int
	cfg := Config{
		Tools:      tools.NewRegistry(recordTool{runs: &runs}),
		UI:         logUI{&log},
		Hooks:      logBus{log: &log},
		Permission: spyPerm{allow: false}, // would deny, but read-only bypasses
	}
	_, err := buildInvoker(cfg).invoke(context.Background(), toolCall{name: "noop", input: json.RawMessage(`{}`)})
	if err != nil || runs != 1 {
		t.Errorf("read-only tool should run despite deny: err=%v runs=%d", err, runs)
	}
}
