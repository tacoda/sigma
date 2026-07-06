package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/workspace"
)

type fakeWS struct {
	created, merged, discarded string
	list                       []workspace.Handle
}

func (f *fakeWS) Create(_ context.Context, id string) (workspace.Handle, error) {
	f.created = id
	return workspace.Handle{ID: id, Dir: "/wt/" + id, Branch: "sigma/" + id}, nil
}
func (f *fakeWS) Merge(_ context.Context, id string) error   { f.merged = id; return nil }
func (f *fakeWS) Discard(_ context.Context, id string) error { f.discarded = id; return nil }
func (f *fakeWS) List(context.Context) ([]workspace.Handle, error) {
	return f.list, nil
}

func runWorktree(t *testing.T, w Worktree, in string) string {
	t.Helper()
	out, err := w.Run(context.Background(), json.RawMessage(in))
	if err != nil {
		t.Fatalf("Run(%s): %v", in, err)
	}
	return out
}

func TestWorktreeCreateSanitizes(t *testing.T) {
	f := &fakeWS{}
	w := Worktree{WS: f}
	runWorktree(t, w, `{"action":"create","name":"Feature/Risky Thing!"}`)
	if f.created != "Feature-Risky-Thing" {
		t.Errorf("created id = %q, want sanitized", f.created)
	}
}

func TestWorktreeMergeDiscard(t *testing.T) {
	f := &fakeWS{}
	w := Worktree{WS: f}
	runWorktree(t, w, `{"action":"merge","name":"one"}`)
	runWorktree(t, w, `{"action":"discard","name":"two"}`)
	if f.merged != "one" || f.discarded != "two" {
		t.Errorf("merged=%q discarded=%q", f.merged, f.discarded)
	}
}

func TestWorktreeCreateRequiresName(t *testing.T) {
	if _, err := (Worktree{WS: &fakeWS{}}).Run(context.Background(), json.RawMessage(`{"action":"create"}`)); err == nil {
		t.Error("create without name should error")
	}
}

func TestWorktreeList(t *testing.T) {
	f := &fakeWS{list: []workspace.Handle{{ID: "a", Branch: "sigma/a", Dir: "/wt/a"}}}
	out := runWorktree(t, Worktree{WS: f}, `{"action":"list"}`)
	if !strings.Contains(out, "sigma/a") {
		t.Errorf("list output = %q", out)
	}
}
