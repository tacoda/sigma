package hooks

import (
	"strings"
	"testing"
)

func TestPreToolBlocksOnNonZeroExit(t *testing.T) {
	r := New(map[string][]string{"PreToolUse": {"echo nope; exit 1"}})
	block, reason := r.PreTool("bash", "ls")
	if !block {
		t.Fatal("expected block on non-zero exit")
	}
	if !strings.Contains(reason, "nope") {
		t.Errorf("reason = %q", reason)
	}
}

func TestPreToolAllowsOnZeroExit(t *testing.T) {
	r := New(map[string][]string{"PreToolUse": {"true"}})
	if block, _ := r.PreTool("bash", "ls"); block {
		t.Error("zero-exit hook should not block")
	}
}

func TestPreToolReceivesPayload(t *testing.T) {
	// The hook fails only if stdin contains the tool name, proving the payload
	// reaches the command.
	r := New(map[string][]string{"PreToolUse": {`grep -q write_file && exit 1 || exit 0`}})
	if block, _ := r.PreTool("write_file", "x"); !block {
		t.Error("hook should see tool name in stdin payload and block")
	}
	if block, _ := r.PreTool("read_file", "x"); block {
		t.Error("hook should not block a non-matching tool")
	}
}

func TestNoHooksNeverBlock(t *testing.T) {
	r := New(nil)
	if block, _ := r.PreTool("bash", "x"); block {
		t.Error("no hooks should never block")
	}
	r.PostTool("bash", "out") // must not panic
}
