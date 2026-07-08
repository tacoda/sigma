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
	Name   string   `yaml:"name"`
	Prompt string   `yaml:"prompt"`
	Setup  string   `yaml:"setup"`  // optional seed directory copied into the scratch workspace
	Checks []string `yaml:"checks"` // programmatic checks (shell commands run in the workspace)
}

// Variant is a configuration under test — a .sigma/ charter.
type Variant struct {
	Name    string `yaml:"name"`
	Charter string `yaml:"charter"`
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
	Variants []Variant `yaml:"variants"`
	Cases    []Case    `yaml:"cases"`
	Repeats  int       `yaml:"repeats"` // runs per (variant, case); <1 treated as 1
}

// Run executes the experiment and returns the comparison report. scorers
// defaults to a single Programmatic scorer.
func (e Experiment) Run(ctx context.Context, runner Runner, scorers []Scorer) (*Report, error) {
	if len(scorers) == 0 {
		scorers = []Scorer{Programmatic{}}
	}
	reps := e.Repeats
	if reps < 1 {
		reps = 1
	}

	rep := &Report{Experiment: e.Name}
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

// String renders the report. With exactly two variants it adds a per-case A/B
// delta (variant[1] − variant[0]) and the overall delta.
func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "experiment: %s\n", r.Experiment)
	for _, v := range r.Variants {
		fmt.Fprintf(&b, "\n[%s] mean %.2f\n", v.Name, v.Mean)
		for _, c := range v.Cases {
			fmt.Fprintf(&b, "  %-24s %.2f\n", c.Name, c.Score)
		}
	}
	if len(r.Variants) == 2 {
		a, c := r.Variants[0], r.Variants[1]
		fmt.Fprintf(&b, "\nA/B: %s vs %s\n", a.Name, c.Name)
		for i := range a.Cases {
			d := c.Cases[i].Score - a.Cases[i].Score
			fmt.Fprintf(&b, "  %-24s %+.2f\n", a.Cases[i].Name, d)
		}
		fmt.Fprintf(&b, "  %-24s %+.2f  (overall)\n", "", c.Mean-a.Mean)
	}
	return b.String()
}
