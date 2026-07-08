package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/app"
	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/permission"
	"github.com/tacoda/sigma/internal/tools"
	"github.com/tacoda/sigma/internal/workspace"
)

// LiveRunner runs the real agent against a live model, building it from the
// variant's charter via the app composition root, in a fresh scratch workspace.
// It records the transcript so the run can later be replayed for free.
type LiveRunner struct {
	Client    agent.LLM
	RecordDir string // if set, transcripts are written under <RecordDir>/transcripts
}

func (lr LiveRunner) Run(ctx context.Context, v Variant, c Case) (Result, error) {
	dir, err := scratch(c.Setup)
	if err != nil {
		return Result{}, err
	}

	rec := &recordingLLM{inner: lr.Client}
	d, err := app.Build(app.Options{Client: rec, ConfigRoot: v.Charter})
	if err != nil {
		return Result{Dir: dir}, err
	}
	defer d.Cleanup()

	tr := &traceRecorder{}
	var out strings.Builder
	base := agent.Config{
		Client:     rec,
		Permission: permission.ForMode(permission.Bypass, nil),
		Hooks:      append(hooks.Multi{tr}, d.Bus), // capture trace + run the charter's hooks
		Model:      d.Model,
		System:     d.System,
		CompactAt:  d.CompactAt,
		UI:         &captureUI{b: &out},
	}
	// Root the top agent's file tools at the scratch workspace.
	newTools := func(root string) []tools.Tool {
		if root == "" {
			root = dir
		}
		return d.NewTools(root)
	}
	base.Tools = agent.WithSubagent(base, agent.SubagentOptions{
		Tools:     newTools,
		Types:     d.Types,
		Workflows: d.Workflows,
		Isolate:   false, // eval runs are not worktree-isolated (deterministic scratch)
		Workspace: workspace.Git{},
	})
	a := agent.New(base)

	base.Hooks.Emit(ctx, hooks.Event{Kind: hooks.SessionStart})
	runErr := a.Run(ctx, c.Prompt)
	base.Hooks.Emit(ctx, hooks.Event{Kind: hooks.SessionEnd})

	if lr.RecordDir != "" {
		_ = saveTranscript(lr.RecordDir, v.Name, c.Name, rec.results)
	}
	return Result{Output: out.String(), Trace: tr.events, Dir: dir, Err: runErr}, nil
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
