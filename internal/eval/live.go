package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/message"
)

// LiveRunner runs the real agent against a live model, building it from the
// variant's charter, and records the transcript so the run can later be
// replayed for free.
type LiveRunner struct {
	Client    agent.LLM
	RecordDir string // if set, transcripts are written under <RecordDir>/transcripts
}

func (lr LiveRunner) Run(ctx context.Context, v Variant, c Case) (Result, error) {
	rec := &recordingLLM{inner: lr.Client}
	r, err := runAgent(ctx, rec, v.Charter, c)
	if err == nil && lr.RecordDir != "" {
		_ = saveTranscript(lr.RecordDir, v.Name, c.Name, rec.results)
	}
	return r, err
}

// recordingLLM wraps a client and captures every result for later replay.
type recordingLLM struct {
	inner   agent.LLM
	mu      sync.Mutex
	results []*message.Result
}

func (r *recordingLLM) Stream(ctx context.Context, req message.Request, onText func(string)) (*message.Result, error) {
	res, err := r.inner.Stream(ctx, req, onText)
	if err == nil {
		r.mu.Lock()
		r.results = append(r.results, res)
		r.mu.Unlock()
	}
	return res, err
}

func saveTranscript(base, variant, caseName string, rs []*message.Result) error {
	dir := filepath.Join(base, "transcripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, variant+"-"+caseName+".json"), data, 0o644)
}
