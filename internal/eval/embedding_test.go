package eval

import (
	"context"
	"strings"
	"testing"
)

// A higher layer (e.g. open-refinery comparing two governance policies) embeds
// the harness with its own Runner and Scorer — no charter, no agent. This proves
// the engine is generic: Experiment/Report/stats work over any Runner/Scorer.
func TestEmbedNonCharterExperiment(t *testing.T) {
	exp := Experiment{
		Name:  "governance-policy-ab",
		Level: "policy",
		Variants: []Variant{
			{Name: "strict", Params: map[string]string{"policy": "strict"}},
			{Name: "lax", Params: map[string]string{"policy": "lax"}},
		},
		Cases: []Case{{Name: "request-1"}, {Name: "request-2"}},
	}

	// Runner applies the variant's policy param to produce a result.
	runner := RunnerFunc(func(_ context.Context, v Variant, c Case) (Result, error) {
		return Result{Output: v.Param("policy") + ":" + c.Name}, nil
	})
	// Scorer: the strict policy scores higher.
	scorer := ScorerFunc(func(_ context.Context, _ Case, r Result) []Score {
		v := 0.5
		if strings.HasPrefix(r.Output, "strict") {
			v = 1
		}
		return []Score{{Name: "compliance", Value: v, Pass: v == 1}}
	})

	rep, err := exp.Run(context.Background(), runner, []Scorer{scorer})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Variants[0].Mean != 1.0 {
		t.Errorf("strict mean = %.2f, want 1.0", rep.Variants[0].Mean)
	}
	if rep.Variants[1].Mean != 0.5 {
		t.Errorf("lax mean = %.2f, want 0.5", rep.Variants[1].Mean)
	}
	cmp, ok := rep.Compare()
	if !ok || cmp.Overall != -0.5 {
		t.Errorf("overall delta = %.2f, want -0.50 (lax worse than strict)", cmp.Overall)
	}

	// The same report machinery renders a level-appropriate report.
	if _, err := ReporterFor(exp.Level).Render(rep); err != nil {
		t.Fatal(err)
	}
}
