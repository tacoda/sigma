package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// Sink is a sensor bus: it records every event as one JSON line and never
// blocks. It is the durable signal from the observe layer — an audit trail of
// the loop (events, tools, token usage). Content (prompt/output text) is not
// logged, only sizes, to avoid leaking data into the trail.
type Sink struct {
	mu  sync.Mutex
	w   io.Writer
	now func() time.Time
}

// NewSink writes events to w.
func NewSink(w io.Writer) *Sink { return &Sink{w: w, now: time.Now} }

type sinkRecord struct {
	Time      string `json:"time"`
	Event     string `json:"event"`
	Tool      string `json:"tool,omitempty"`
	Bytes     int    `json:"bytes,omitempty"` // len of tool output
	InTokens  int    `json:"in_tokens,omitempty"`
	OutTokens int    `json:"out_tokens,omitempty"`
}

func (s *Sink) Emit(_ context.Context, ev Event) Outcome {
	rec := sinkRecord{
		Time:      s.now().UTC().Format(time.RFC3339Nano),
		Event:     string(ev.Kind),
		Tool:      ev.Tool,
		Bytes:     len(ev.Output),
		InTokens:  ev.InTokens,
		OutTokens: ev.OutTokens,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return Outcome{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintln(s.w, string(data))
	return Outcome{}
}
