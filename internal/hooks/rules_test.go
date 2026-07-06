package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeRules(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hooks.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRulesDenyMatchesTool(t *testing.T) {
	p := writeRules(t, `hooks:
  - on: PreToolUse
    match: { tool: "write_file|edit_file" }
    deny: "no writes in {event}"
`)
	r, err := LoadRules(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if o := r.Emit(ctx, Event{Kind: PreTool, Tool: "write_file"}); !o.Block || o.Reason != "no writes in PreToolUse" {
		t.Errorf("matching tool: got %+v, want block with expanded reason", o)
	}
	if r.Emit(ctx, Event{Kind: PreTool, Tool: "bash"}).Block {
		t.Error("non-matching tool should not block")
	}
	if r.Emit(ctx, Event{Kind: PostTool, Tool: "write_file"}).Block {
		t.Error("wrong event kind should not match")
	}
}

func TestRulesUnknownEventErrors(t *testing.T) {
	p := writeRules(t, "hooks:\n  - on: Nope\n    deny: x\n")
	if _, err := LoadRules(p); err == nil {
		t.Error("want error for unknown event")
	}
}

func TestRulesNoActionErrors(t *testing.T) {
	p := writeRules(t, "hooks:\n  - on: Stop\n")
	if _, err := LoadRules(p); err == nil {
		t.Error("want error for rule with no action")
	}
}

func TestLoadRulesMissingFileIsEmpty(t *testing.T) {
	r, err := LoadRules(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("missing file should be skipped: %v", err)
	}
	if r.Emit(context.Background(), Event{Kind: Stop}).Block {
		t.Error("empty rules should never block")
	}
}
