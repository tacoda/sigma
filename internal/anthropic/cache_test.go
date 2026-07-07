package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/tacoda/sigma/internal/message"
)

func TestWireOfMarksCacheBreakpoints(t *testing.T) {
	w := wireOf(message.Request{
		System: "project rules",
		Tools: []message.Tool{
			{Name: "read_file", InputSchema: json.RawMessage(`{}`)},
			{Name: "bash", InputSchema: json.RawMessage(`{}`)},
		},
	})

	// The last system block (the rules) is the breakpoint; the CLI identity is not.
	if w.System[0].CacheControl != nil {
		t.Error("first system block should not be a breakpoint")
	}
	if w.System[len(w.System)-1].CacheControl == nil {
		t.Error("last system block should be a cache breakpoint")
	}

	// Only the last tool is the breakpoint.
	if w.Tools[0].CacheControl != nil {
		t.Error("non-last tool should not be a breakpoint")
	}
	if w.Tools[len(w.Tools)-1].CacheControl == nil {
		t.Error("last tool should be a cache breakpoint")
	}
}

func TestWireOfNoToolsNoPanic(t *testing.T) {
	w := wireOf(message.Request{})
	if len(w.Tools) != 0 {
		t.Error("no tools expected")
	}
	// System still has the CLI identity, marked as a breakpoint.
	if w.System[len(w.System)-1].CacheControl == nil {
		t.Error("system breakpoint should be set even without extra system text")
	}
}
