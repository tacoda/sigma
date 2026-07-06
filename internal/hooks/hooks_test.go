package hooks

import (
	"context"
	"strings"
	"testing"
)

func emit(b Bus, ev Event) Outcome { return b.Emit(context.Background(), ev) }

func TestShellBlocksOnNonZeroExit(t *testing.T) {
	s := NewShell(map[string][]string{"PreToolUse": {"echo nope; exit 1"}})
	o := emit(s, Event{Kind: PreTool, Tool: "bash", Input: "ls"})
	if !o.Block {
		t.Fatal("expected block on non-zero exit")
	}
	if !strings.Contains(o.Reason, "nope") {
		t.Errorf("reason = %q", o.Reason)
	}
}

func TestShellAllowsOnZeroExit(t *testing.T) {
	s := NewShell(map[string][]string{"PreToolUse": {"true"}})
	if emit(s, Event{Kind: PreTool, Tool: "bash"}).Block {
		t.Error("zero-exit hook should not block")
	}
}

func TestShellReceivesPayload(t *testing.T) {
	// The hook blocks only if stdin carries the tool name, proving the payload
	// reaches the command.
	s := NewShell(map[string][]string{"PreToolUse": {`grep -q write_file && exit 1 || exit 0`}})
	if !emit(s, Event{Kind: PreTool, Tool: "write_file"}).Block {
		t.Error("hook should see tool name in stdin payload and block")
	}
	if emit(s, Event{Kind: PreTool, Tool: "read_file"}).Block {
		t.Error("hook should not block a non-matching tool")
	}
}

func TestNopNeverBlocks(t *testing.T) {
	if emit(Nop{}, Event{Kind: PreTool}).Block {
		t.Error("nop should never block")
	}
}

func TestCallbacksDispatchByKind(t *testing.T) {
	var pre, post int
	c := NewCallbacks().
		On(PreTool, func(context.Context, Event) Outcome { pre++; return Outcome{} }).
		On(PostTool, func(context.Context, Event) Outcome { post++; return Outcome{} })

	emit(c, Event{Kind: PreTool})
	emit(c, Event{Kind: Stop}) // no handler
	if pre != 1 || post != 0 {
		t.Errorf("pre=%d post=%d, want 1 0", pre, post)
	}
}

func TestCallbackCanBlock(t *testing.T) {
	c := NewCallbacks().On(PreTool, func(context.Context, Event) Outcome {
		return Outcome{Block: true, Reason: "denied"}
	})
	if o := emit(c, Event{Kind: PreTool}); !o.Block || o.Reason != "denied" {
		t.Errorf("got %+v, want block/denied", o)
	}
}

func TestOnAllTapsEveryKind(t *testing.T) {
	var seen []Kind
	c := NewCallbacks().OnAll(func(_ context.Context, ev Event) Outcome {
		seen = append(seen, ev.Kind)
		return Outcome{}
	})
	for _, k := range []Kind{SessionStart, PreLLM, ToolError, Stop} {
		emit(c, Event{Kind: k})
	}
	if len(seen) != 4 {
		t.Errorf("OnAll saw %v, want 4 events", seen)
	}
}

func TestMultiFirstBlockWins(t *testing.T) {
	calls := 0
	count := NewCallbacks().On(PreTool, func(context.Context, Event) Outcome { calls++; return Outcome{} })
	blocker := NewCallbacks().On(PreTool, func(context.Context, Event) Outcome { return Outcome{Block: true} })
	after := NewCallbacks().On(PreTool, func(context.Context, Event) Outcome { calls++; return Outcome{} })

	m := Multi{count, blocker, after}
	if !emit(m, Event{Kind: PreTool}).Block {
		t.Fatal("expected block")
	}
	if calls != 1 {
		t.Errorf("calls=%d, want 1 (bus after the blocker must not run)", calls)
	}
}
