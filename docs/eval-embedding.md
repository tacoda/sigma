# Embedding the eval harness in a higher layer

`internal/eval` is a generic A/B experimentation engine. `Runner` and `Scorer`
are interfaces, `Variant` carries arbitrary `Params`, and `Report`/stats/reporters
work over any of them — so a higher layer (a workflow tuner, or an external
product like open-refinery comparing two governance policies) embeds it with its
own runner and scorers. No charter or agent required.

```go
import "github.com/tacoda/sigma/internal/eval"

exp := eval.Experiment{
    Name:  "governance-policy-ab",
    Level: "policy", // selects a registered Reporter, else the default HTML one
    Variants: []eval.Variant{
        {Name: "before", Params: map[string]string{"policy": "v1"}},
        {Name: "after",  Params: map[string]string{"policy": "v2"}},
    },
    Cases: []eval.Case{{Name: "request-1"}, {Name: "request-2"}},
}

// Supply your own runner: apply the variant's policy, produce a Result.
runner := eval.RunnerFunc(func(ctx context.Context, v eval.Variant, c eval.Case) (eval.Result, error) {
    out := applyPolicy(v.Param("policy"), c.Name) // your logic
    return eval.Result{Output: out}, nil
})

// Supply your own scorer(s).
scorer := eval.ScorerFunc(func(ctx context.Context, c eval.Case, r eval.Result) []eval.Score {
    return []eval.Score{{Name: "compliance", Value: complianceOf(r.Output)}}
})

rep, _ := exp.Run(ctx, runner, []eval.Scorer{scorer})
cmp, _ := rep.Compare()          // per-case deltas + sign-test significance
html, _ := eval.ReporterFor(exp.Level).Render(rep)  // your level's report
```

Register a level-specific reporter so the output matches your domain:

```go
func init() { eval.RegisterReporter("policy", policyReporter{}) }
```

The built-in charter runners (`ReplayRunner`, `LiveRunner`) are just the first
adapters of `Runner`; "the experiment is the eval" applies at every layer.
