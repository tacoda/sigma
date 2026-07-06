package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/tools"
	"github.com/tacoda/sigma/internal/workspace"
)

// recordWS records worktree lifecycle calls.
type recordWS struct{ created, merged, discarded string }

func (w *recordWS) Create(_ context.Context, id string) (workspace.Handle, error) {
	w.created = id
	return workspace.Handle{ID: id, Dir: "/wt/" + id, Branch: "sigma/" + id}, nil
}
func (w *recordWS) Merge(_ context.Context, id string) error   { w.merged = id; return nil }
func (w *recordWS) Discard(_ context.Context, id string) error { w.discarded = id; return nil }
func (w *recordWS) List(context.Context) ([]workspace.Handle, error) {
	return nil, nil
}

func runTask(t *testing.T, reg *tools.Registry, prompt string) (string, error) {
	t.Helper()
	return reg.Run(context.Background(), "task", json.RawMessage(`{"prompt":"`+prompt+`"}`))
}

func TestSubagentIsolationMergesOnSuccess(t *testing.T) {
	ws := &recordWS{}
	var childRoot string
	reg := agent.WithSubagent(
		agent.Config{Client: &fakeStreamer{script: []*message.Result{endTurnResult("done")}}},
		agent.SubagentOptions{
			Tools:     func(root string) []tools.Tool { childRoot = root; return nil },
			Isolate:   true,
			Workspace: ws,
		},
	)

	out, err := runTask(t, reg, "do the thing")
	if err != nil {
		t.Fatal(err)
	}
	if ws.created == "" {
		t.Fatal("worktree was not created")
	}
	if ws.merged != ws.created {
		t.Errorf("merged %q, want created %q", ws.merged, ws.created)
	}
	if ws.discarded != "" {
		t.Errorf("should not discard on success (discarded %q)", ws.discarded)
	}
	if childRoot != "/wt/"+ws.created {
		t.Errorf("child tools root = %q, want the worktree dir", childRoot)
	}
	if !strings.Contains(out, "done") {
		t.Errorf("out = %q, want sub-agent output", out)
	}
}

func TestSubagentIsolationDiscardsOnError(t *testing.T) {
	ws := &recordWS{}
	// Empty script makes the sub-agent's Stream call fail, so the task errors.
	reg := agent.WithSubagent(
		agent.Config{Client: &fakeStreamer{}},
		agent.SubagentOptions{
			Tools:     func(string) []tools.Tool { return nil },
			Isolate:   true,
			Workspace: ws,
		},
	)

	if _, err := runTask(t, reg, "boom"); err == nil {
		t.Fatal("expected error from failed sub-agent")
	}
	if ws.discarded != ws.created || ws.created == "" {
		t.Errorf("should discard the worktree on error (created %q discarded %q)", ws.created, ws.discarded)
	}
	if ws.merged != "" {
		t.Errorf("should not merge on error (merged %q)", ws.merged)
	}
}

func TestSubagentNoIsolationSkipsWorkspace(t *testing.T) {
	ws := &recordWS{}
	reg := agent.WithSubagent(
		agent.Config{Client: &fakeStreamer{script: []*message.Result{endTurnResult("ok")}}},
		agent.SubagentOptions{
			Tools:     func(string) []tools.Tool { return nil },
			Isolate:   false,
			Workspace: ws,
		},
	)
	if _, err := runTask(t, reg, "plain"); err != nil {
		t.Fatal(err)
	}
	if ws.created != "" {
		t.Errorf("no worktree should be created when Isolate is false")
	}
}
