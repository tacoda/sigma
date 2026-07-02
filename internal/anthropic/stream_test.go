package anthropic

import (
	"strings"
	"testing"
)

// canned SSE stream: one text block, one tool_use block, stop_reason tool_use.
const sample = `data: {"type":"message_start","message":{"usage":{"input_tokens":10}}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

data: {"type":"content_block_stop","index":0}

data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu_1","name":"read_file"}}

data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}

data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"go.mod\"}"}}

data: {"type":"content_block_stop","index":1}

data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}

data: {"type":"message_stop"}
`

func TestParseStream(t *testing.T) {
	var got strings.Builder
	r, err := parseStream(strings.NewReader(sample), func(s string) { got.WriteString(s) })
	if err != nil {
		t.Fatal(err)
	}
	if r.Text() != "Hello world" {
		t.Errorf("text = %q", r.Text())
	}
	if got.String() != "Hello world" {
		t.Errorf("streamed text = %q", got.String())
	}
	if r.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q", r.StopReason)
	}
	if r.Usage.InputTokens != 10 || r.Usage.OutputTokens != 7 {
		t.Errorf("usage = %+v", r.Usage)
	}
	uses := r.ToolUses()
	if len(uses) != 1 {
		t.Fatalf("tool uses = %d", len(uses))
	}
	if uses[0].Name != "read_file" || uses[0].ID != "tu_1" {
		t.Errorf("tool = %+v", uses[0])
	}
	if string(uses[0].Input) != `{"path":"go.mod"}` {
		t.Errorf("input = %s", uses[0].Input)
	}
}

func TestParseStreamErrorEvent(t *testing.T) {
	const stream = `data: {"type":"message_start","message":{"usage":{"input_tokens":10}}}

data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}
`
	_, err := parseStream(strings.NewReader(stream), nil)
	if err == nil {
		t.Fatal("want error from error event, got nil")
	}
	if !strings.Contains(err.Error(), "Overloaded") {
		t.Errorf("err = %v, want it to contain %q", err, "Overloaded")
	}
}
