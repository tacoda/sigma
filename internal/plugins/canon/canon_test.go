package canon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/plugin"
)

func in(m map[string]string) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func emit(tool, input string) hooks.Outcome {
	return guards{}.Emit(context.Background(), hooks.Event{Kind: hooks.PreTool, Tool: tool, Input: input})
}

func TestMountsByDefault(t *testing.T) {
	h, err := plugin.Mount(nil, nil, nil) // canon is a default; init() registered it
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Sources) == 0 || len(h.Hooks) == 0 {
		t.Fatalf("canon should contribute a source and a hook: %d sources, %d hooks", len(h.Sources), len(h.Hooks))
	}
}

func TestGuideNonEmpty(t *testing.T) {
	s, err := guide{}.Contribute()
	if err != nil || len(s) < 500 {
		t.Errorf("canon guide too short (%d bytes)", len(s))
	}
}

func TestGuardsBlockViolations(t *testing.T) {
	blocked := []struct {
		name, tool, input string
	}{
		{"read secret", "read_file", in(map[string]string{"path": ".env"})},
		{"write to key", "write_file", in(map[string]string{"path": "server.key", "content": "x"})},
		{"secret in content", "write_file", in(map[string]string{"path": "a.go", "content": "k := \"AKIAABCDEFGHIJKLMNOP\""})},
		{"debug artifact", "write_file", in(map[string]string{"path": "a.js", "content": "console.log(x)"})},
		{"force push", "bash", in(map[string]string{"command": "git push --force origin main"})},
		{"reset hard", "bash", in(map[string]string{"command": "git reset --hard HEAD~1"})},
		{"no-verify", "bash", in(map[string]string{"command": "git commit --no-verify -m x"})},
		{"ai attribution", "bash", in(map[string]string{"command": "git commit -m 'feat: x\n\nCo-Authored-By: bot'"})},
		{"rm -rf", "bash", in(map[string]string{"command": "rm -rf build/"})},
		{"git add .", "bash", in(map[string]string{"command": "git add ."})},
		{"git add -A", "bash", in(map[string]string{"command": "git add -A"})},
		{"non-conventional commit", "bash", in(map[string]string{"command": `git commit -m "fixed the thing"`})},
	}
	for _, tc := range blocked {
		if o := emit(tc.tool, tc.input); !o.Block {
			t.Errorf("%s: expected block", tc.name)
		}
	}
}

func TestGuardsAllowNormal(t *testing.T) {
	allowed := []struct {
		name, tool, input string
	}{
		{"normal write", "write_file", in(map[string]string{"path": "internal/x.go", "content": "package x\n\nfunc F() {}"})},
		{"normal read", "read_file", in(map[string]string{"path": "internal/x.go"})},
		{"normal bash", "bash", in(map[string]string{"command": "go test ./..."})},
		{"clean commit", "bash", in(map[string]string{"command": "git commit -m 'feat(x): add F'"})},
		{"stage specific", "bash", in(map[string]string{"command": "git add internal/x.go"})},
		{"merge commit", "bash", in(map[string]string{"command": `git commit -m "Merge branch main"`})},
		{"editor commit", "bash", in(map[string]string{"command": "git commit"})},
	}
	for _, tc := range allowed {
		if o := emit(tc.tool, tc.input); o.Block {
			t.Errorf("%s: should be allowed, blocked with %q", tc.name, o.Reason)
		}
	}
	// Non-PreToolUse events are ignored.
	if (guards{}).Emit(context.Background(), hooks.Event{Kind: hooks.PostTool, Tool: "bash"}).Block {
		t.Error("guards should only act on PreToolUse")
	}
}
