package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSinkRecordsEvents(t *testing.T) {
	var buf bytes.Buffer
	s := NewSink(&buf)
	s.now = func() time.Time { return time.Unix(0, 0).UTC() }

	if s.Emit(context.Background(), Event{Kind: PostTool, Tool: "bash", Output: "hello"}).Block {
		t.Error("sink must never block")
	}
	s.Emit(context.Background(), Event{Kind: PostLLM, InTokens: 100, OutTokens: 20})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}

	var r0 sinkRecord
	if err := json.Unmarshal([]byte(lines[0]), &r0); err != nil {
		t.Fatal(err)
	}
	if r0.Event != "PostToolUse" || r0.Tool != "bash" || r0.Bytes != 5 {
		t.Errorf("record 0 = %+v", r0)
	}

	var r1 sinkRecord
	if err := json.Unmarshal([]byte(lines[1]), &r1); err != nil {
		t.Fatal(err)
	}
	if r1.Event != "PostLLMResponse" || r1.InTokens != 100 || r1.OutTokens != 20 {
		t.Errorf("record 1 = %+v", r1)
	}
}
