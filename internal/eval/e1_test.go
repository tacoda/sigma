package eval

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/message"
)

func TestTraceScorer(t *testing.T) {
	r := Result{Trace: []hooks.Event{
		{Kind: hooks.PreTool, Tool: "read_file"},
		{Kind: hooks.PostLLM},
		{Kind: hooks.ToolError, Tool: "bash"},
	}}
	c := Case{Trace: &TraceAssert{
		Used: []string{"read_file"}, NotUsed: []string{"worktree"},
		NoError: true, MaxTurns: 1,
	}}
	got := map[string]bool{}
	for _, s := range (Trace{}).Score(context.Background(), c, r) {
		got[s.Name] = s.Pass
	}
	if !got["used: read_file"] {
		t.Error("read_file should be marked used")
	}
	if !got["notUsed: worktree"] {
		t.Error("worktree should be marked not-used")
	}
	if got["noError"] {
		t.Error("noError should fail (a ToolError occurred)")
	}
	if !got["maxTurns"] {
		t.Error("maxTurns should pass (1 turn <= 1)")
	}
}

type judgeClient struct{ reply string }

func (j judgeClient) Stream(_ context.Context, _ message.Request, _ func(string)) (*message.Result, error) {
	return &message.Result{Content: []message.Block{{Type: "text", Text: j.reply}}, StopReason: "end_turn"}, nil
}

func TestJudgeScorer(t *testing.T) {
	j := Judge{Client: judgeClient{reply: "sure!\n{\"pass\":true,\"score\":0.8,\"reason\":\"looks good\"}\ndone"}, Model: "m"}
	got := j.Score(context.Background(), Case{Judge: "is it good?"}, Result{Output: "the work"})
	if len(got) != 1 || !got[0].Pass || got[0].Value != 0.8 || got[0].Detail != "looks good" {
		t.Errorf("judge score = %+v", got)
	}

	// No rubric or no client -> no score.
	if len(j.Score(context.Background(), Case{}, Result{})) != 0 {
		t.Error("empty rubric should yield no judge score")
	}
	if len((Judge{}).Score(context.Background(), Case{Judge: "x"}, Result{})) != 0 {
		t.Error("nil client should yield no judge score")
	}
}

func TestSignTest(t *testing.T) {
	w, l, ti, p := signTest([]float64{1, 1, 1, -1})
	if w != 3 || l != 1 || ti != 0 {
		t.Errorf("counts = %d/%d/%d, want 3/1/0", w, l, ti)
	}
	if math.Abs(p-0.625) > 1e-9 {
		t.Errorf("p = %v, want 0.625", p)
	}
	if _, _, _, p0 := signTest([]float64{0, 0}); p0 != 1 {
		t.Errorf("all ties p = %v, want 1", p0)
	}
}

func TestHTMLReportAndRegistry(t *testing.T) {
	rep := &Report{
		Experiment: "exp1", Level: "charter",
		Variants: []VariantReport{
			{Name: "before", Mean: 0.5, Cases: []CaseReport{{Name: "c1", Score: 0.5}}},
			{Name: "after", Mean: 1.0, Cases: []CaseReport{{Name: "c1", Score: 1.0}}},
		},
	}
	html, err := HTMLReporter{}.Render(rep)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<!doctype html>", "exp1", "charter", "0.50", `class="num up"`, "sign-test"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
	}

	// Registry: a level-specific reporter is selected; unknown falls back.
	RegisterReporter("custom", stubReporter{})
	if _, ok := ReporterFor("custom").(stubReporter); !ok {
		t.Error("ReporterFor(custom) should return the registered reporter")
	}
	if _, ok := ReporterFor("nope").(HTMLReporter); !ok {
		t.Error("ReporterFor(unknown) should fall back to HTMLReporter")
	}
}

type stubReporter struct{}

func (stubReporter) Render(*Report) (string, error) { return "", nil }
