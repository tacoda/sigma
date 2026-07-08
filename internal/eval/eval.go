// Package eval is an A/B experimentation harness: it runs an agent under two
// variants (charters) over a suite of cases, scores each result, and reports
// the before/after comparison. Runs are fake-by-default (replay a recorded
// transcript); a live runner hits the real agent.
//
// Runner and Scorer are the ports; Experiment orchestrates; Report compares.
package eval

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tacoda/sigma/internal/hooks"
)

// Case is one task to run and how to score it.
type Case struct {
	Name   string       `yaml:"name"`
	Prompt string       `yaml:"prompt"`
	Setup  string       `yaml:"setup"`  // optional seed directory copied into the scratch workspace
	Checks []string     `yaml:"checks"` // programmatic checks (shell commands run in the workspace)
	Trace  *TraceAssert `yaml:"trace"`  // assertions over the event trace
	Judge  string       `yaml:"judge"`  // LLM-judge rubric
}

// TraceAssert is a set of assertions over a run's event stream.
type TraceAssert struct {
	Used     []string `yaml:"used"`     // tools that must appear
	NotUsed  []string `yaml:"notUsed"`  // tools that must not appear
	NoError  bool     `yaml:"noError"`  // no ToolError events
	MaxTurns int      `yaml:"maxTurns"` // model responses must be <= this
}

// Variant is a configuration under test. Charter names a .sigma/ bundle for the
// built-in charter runners; Params carries arbitrary key/values for a custom
// Runner supplied by a higher layer (e.g. a governance-policy A/B).
type Variant struct {
	Name    string            `yaml:"name"`
	Charter string            `yaml:"charter"`
	Params  map[string]string `yaml:"params"`
}

// Param returns a variant parameter. The key "charter" falls back to the
// Charter field.
func (v Variant) Param(key string) string {
	if val, ok := v.Params[key]; ok {
		return val
	}
	if key == "charter" {
		return v.Charter
	}
	return ""
}

// RunnerFunc adapts a function to the Runner port, so a higher layer can supply
// a runner inline without a struct.
type RunnerFunc func(ctx context.Context, v Variant, c Case) (Result, error)

func (f RunnerFunc) Run(ctx context.Context, v Variant, c Case) (Result, error) {
	return f(ctx, v, c)
}

// ScorerFunc adapts a function to the Scorer port.
type ScorerFunc func(ctx context.Context, c Case, r Result) []Score

func (f ScorerFunc) Score(ctx context.Context, c Case, r Result) []Score {
	return f(ctx, c, r)
}

// Result is one run's outcome.
type Result struct {
	Output string
	Trace  []hooks.Event
	Dir    string // scratch workspace (scored, then removed by the Experiment)
	Err    error
}

// Score is one scorer's judgment.
type Score struct {
	Name   string
	Value  float64 // 0..1
	Pass   bool
	Detail string
}

// Runner runs a variant against a case.
type Runner interface {
	Run(ctx context.Context, v Variant, c Case) (Result, error)
}

// Scorer judges a result.
type Scorer interface {
	Score(ctx context.Context, c Case, r Result) []Score
}

// Experiment compares variants over cases.
type Experiment struct {
	Name     string    `yaml:"name"`
	Level    string    `yaml:"level"` // selects the Reporter (e.g. "charter", "workflow", "policy")
	Variants []Variant `yaml:"variants"`
	Cases    []Case    `yaml:"cases"`
	Repeats  int       `yaml:"repeats"` // runs per (variant, case); <1 treated as 1
}

// Run executes the experiment and returns the comparison report. scorers
// defaults to a single Programmatic scorer.
func (e Experiment) Run(ctx context.Context, runner Runner, scorers []Scorer) (*Report, error) {
	if len(scorers) == 0 {
		scorers = []Scorer{Programmatic{}, Trace{}}
	}
	reps := e.Repeats
	if reps < 1 {
		reps = 1
	}

	rep := &Report{Experiment: e.Name, Level: e.Level}
	for _, v := range e.Variants {
		vr := VariantReport{Name: v.Name}
		var sum float64
		for _, c := range e.Cases {
			var scoreSum float64
			var last []Score
			for i := 0; i < reps; i++ {
				r, err := runner.Run(ctx, v, c)
				if err != nil {
					last = []Score{{Name: "run", Value: 0, Pass: false, Detail: err.Error()}}
					continue
				}
				last = scoreAll(ctx, scorers, c, r)
				scoreSum += caseScore(last)
				if r.Dir != "" {
					os.RemoveAll(r.Dir)
				}
			}
			cs := scoreSum / float64(reps)
			vr.Cases = append(vr.Cases, CaseReport{Name: c.Name, Score: cs, Scores: last})
			sum += cs
		}
		if len(e.Cases) > 0 {
			vr.Mean = sum / float64(len(e.Cases))
		}
		rep.Variants = append(rep.Variants, vr)
	}
	return rep, nil
}

func scoreAll(ctx context.Context, scorers []Scorer, c Case, r Result) []Score {
	var out []Score
	for _, s := range scorers {
		out = append(out, s.Score(ctx, c, r)...)
	}
	return out
}

// caseScore is the mean of a case's score values (0 if none).
func caseScore(scores []Score) float64 {
	if len(scores) == 0 {
		return 0
	}
	var sum float64
	for _, s := range scores {
		sum += s.Value
	}
	return sum / float64(len(scores))
}

// --- report ---

type Report struct {
	Experiment string
	Level      string
	Variants   []VariantReport
}

type VariantReport struct {
	Name  string
	Mean  float64
	Cases []CaseReport
}

type CaseReport struct {
	Name   string
	Score  float64
	Scores []Score
}

// Delta is one case's A→B score change.
type Delta struct {
	Case string
	A, B float64
	Diff float64
}

// Comparison is the A/B result: per-case deltas, overall delta, and a sign-test
// significance verdict.
type Comparison struct {
	A, B               string
	Deltas             []Delta
	Overall            float64
	Wins, Losses, Ties int
	P                  float64 // two-sided sign-test p-value
}

// Compare returns the A/B comparison; ok is false unless there are exactly two
// variants.
func (r *Report) Compare() (Comparison, bool) {
	if len(r.Variants) != 2 {
		return Comparison{}, false
	}
	a, b := r.Variants[0], r.Variants[1]
	cmp := Comparison{A: a.Name, B: b.Name, Overall: b.Mean - a.Mean}
	diffs := make([]float64, len(a.Cases))
	for i := range a.Cases {
		d := b.Cases[i].Score - a.Cases[i].Score
		cmp.Deltas = append(cmp.Deltas, Delta{Case: a.Cases[i].Name, A: a.Cases[i].Score, B: b.Cases[i].Score, Diff: d})
		diffs[i] = d
	}
	cmp.Wins, cmp.Losses, cmp.Ties, cmp.P = signTest(diffs)
	return cmp, true
}

// String renders a terse text summary.
func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "experiment: %s\n", r.Experiment)
	for _, v := range r.Variants {
		fmt.Fprintf(&b, "\n[%s] mean %.2f\n", v.Name, v.Mean)
		for _, c := range v.Cases {
			fmt.Fprintf(&b, "  %-24s %.2f\n", c.Name, c.Score)
		}
	}
	if cmp, ok := r.Compare(); ok {
		fmt.Fprintf(&b, "\nA/B: %s vs %s\n", cmp.A, cmp.B)
		for _, d := range cmp.Deltas {
			fmt.Fprintf(&b, "  %-24s %+.2f\n", d.Case, d.Diff)
		}
		fmt.Fprintf(&b, "  %-24s %+.2f  (overall)\n", "", cmp.Overall)
		fmt.Fprintf(&b, "significance: %d win / %d loss / %d tie, sign-test p=%.3f\n",
			cmp.Wins, cmp.Losses, cmp.Ties, cmp.P)
	}
	return b.String()
}
