package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/message"
)

// ReplayRunner runs the agent with a fake LLM that replays a recorded
// transcript, so runs are deterministic and free. Transcripts live at
// <Base>/transcripts/<variant>-<case>.json (a JSON array of message results).
type ReplayRunner struct {
	Base string
}

func (rr ReplayRunner) Run(ctx context.Context, v Variant, c Case) (Result, error) {
	path := filepath.Join(rr.Base, "transcripts", v.Name+"-"+c.Name+".json")
	transcript, err := loadTranscript(path)
	if err != nil {
		return Result{}, fmt.Errorf("transcript %s: %w", path, err)
	}
	// Build the real agent from the variant's charter, then replay the model
	// through it — so charter differences (hooks, guards, prompt) manifest
	// deterministically on the same transcript.
	return runAgent(ctx, &replayLLM{results: transcript}, v.Charter, c)
}

// replayLLM returns recorded results in order.
type replayLLM struct {
	results []*message.Result
	i       int
}

func (r *replayLLM) Stream(_ context.Context, _ message.Request, onText func(string)) (*message.Result, error) {
	if r.i >= len(r.results) {
		return nil, fmt.Errorf("replay transcript exhausted (%d results)", len(r.results))
	}
	res := r.results[r.i]
	r.i++
	if onText != nil {
		for _, blk := range res.Content {
			if blk.Type == "text" {
				onText(blk.Text)
			}
		}
	}
	return res, nil
}

func loadTranscript(path string) ([]*message.Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var results []*message.Result
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// traceRecorder captures the event stream (a sensor; never blocks).
type traceRecorder struct {
	mu     sync.Mutex
	events []hooks.Event
}

func (t *traceRecorder) Emit(_ context.Context, ev hooks.Event) hooks.Outcome {
	t.mu.Lock()
	t.events = append(t.events, ev)
	t.mu.Unlock()
	return hooks.Outcome{}
}

// captureUI collects streamed text.
type captureUI struct{ b *strings.Builder }

func (c *captureUI) Text(s string)                   { c.b.WriteString(s) }
func (c *captureUI) ToolCall(string, string)         {}
func (c *captureUI) ToolResult(string, string, bool) {}
func (c *captureUI) Usage(int, int)                  {}

// scratch makes a temp workspace, seeding it from setup if given.
func scratch(setup string) (string, error) {
	dir, err := os.MkdirTemp("", "sigma-eval-")
	if err != nil {
		return "", err
	}
	if setup != "" {
		if err := copyTree(setup, dir); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
