package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/message"
)

// writeFileResult is a tool_use turn that calls write_file, then the transcript
// needs an end_turn turn after it.
func writeFileResult(path, content string) *message.Result {
	input := fmt.Sprintf(`{"path":%q,"content":%q}`, path, content)
	return &message.Result{
		Content:    []message.Block{{Type: "tool_use", ID: "1", Name: "write_file", Input: json.RawMessage(input)}},
		StopReason: "tool_use",
	}
}

func endTurnResult(text string) *message.Result {
	return &message.Result{
		Content:    []message.Block{{Type: "text", Text: text}},
		StopReason: "end_turn",
	}
}

func writeTranscript(t *testing.T, base, variant, caseName string, rs []*message.Result) {
	t.Helper()
	dir := filepath.Join(base, "transcripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, variant+"-"+caseName+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExperimentABReplay(t *testing.T) {
	base := t.TempDir()
	// "before" writes the wrong content (check fails); "after" writes OK (passes).
	writeTranscript(t, base, "before", "make-file",
		[]*message.Result{writeFileResult("out.txt", "NOPE"), endTurnResult("done")})
	writeTranscript(t, base, "after", "make-file",
		[]*message.Result{writeFileResult("out.txt", "OK"), endTurnResult("done")})

	exp := Experiment{
		Name:     "toy",
		Variants: []Variant{{Name: "before"}, {Name: "after"}},
		Cases: []Case{{
			Name:   "make-file",
			Prompt: "create out.txt",
			Checks: []string{"grep -q OK out.txt"},
		}},
	}

	rep, err := exp.Run(context.Background(), ReplayRunner{Base: base}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Variants[0].Mean != 0 {
		t.Errorf("before mean = %.2f, want 0 (check should fail)", rep.Variants[0].Mean)
	}
	if rep.Variants[1].Mean != 1 {
		t.Errorf("after mean = %.2f, want 1 (check should pass)", rep.Variants[1].Mean)
	}
	if !strings.Contains(rep.String(), "+1.00  (overall)") {
		t.Errorf("report should show +1.00 overall delta:\n%s", rep.String())
	}
}

func TestReplayCapturesTrace(t *testing.T) {
	base := t.TempDir()
	writeTranscript(t, base, "v", "c",
		[]*message.Result{writeFileResult("x.txt", "hi"), endTurnResult("ok")})

	r, err := ReplayRunner{Base: base}.Run(context.Background(), Variant{Name: "v"}, Case{Name: "c", Prompt: "go"})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(r.Dir)

	if r.Output != "ok" {
		t.Errorf("output = %q, want %q", r.Output, "ok")
	}
	// The trace should include a PreToolUse for write_file.
	var sawTool bool
	for _, ev := range r.Trace {
		if ev.Kind == "PreToolUse" && ev.Tool == "write_file" {
			sawTool = true
		}
	}
	if !sawTool {
		t.Error("trace missing PreToolUse write_file event")
	}
}
