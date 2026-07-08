# Sigma — Eval Harness Plan

**Goal:** a general **experimentation primitive**. Any higher layer — a charter (a `.sigma/` config bundle), a workflow, or an external product's unit (e.g. open-refinery governance policies) — is a *variant*. An experiment compares variants (before/after, A/B) over a case suite, scores each with a mix of scorers, runs **fake-by-default**, and reports a statistically-grounded verdict. *The experiment is the eval.*

## Locked decisions

1. **Scoring is a mix** — programmatic checks, trace assertions, and LLM-as-judge, composed per case.
2. **Fake target by default** — runs replay a recorded transcript (deterministic, free, CI-safe). A `--live` override runs the real agent (and records transcripts for future replay).
3. **Compare two variants** — the primary output is an A/B (before/after) comparison, not a single pass/fail.
4. **Variant = a `.sigma/` charter** for sigma; but the `Variant`/`Runner` abstraction is generic so higher layers plug their own unit in.
5. **Reusable by all higher layers** — a library + a trigger (`sigma eval`), callable programmatically. open-refinery compares two governance policies with the same engine.

## Philosophy (consistent with sigma)

Ports & adapters again. The two ports:

- **Runner** — `Run(ctx, Variant, Case) (Result, error)`. Adapters: `replay` (fake LLM from a recorded transcript — default), `live` (real agent). The LLM port already makes this a swap.
- **Scorer** — `Score(ctx, Case, Result) []Score`. Adapters: `programmatic`, `trace`, `judge`. Composed per case.

`Variant` and `Case` are data; `Experiment` orchestrates; `Report` compares. The harness knows nothing about charters vs policies — a charter Runner is just the first concrete adapter.

## Core types (`internal/eval`)

```go
type Case struct {
    Name    string
    Prompt  string        // task given to the agent
    Setup   string        // optional: seed workspace (a dir to copy) 
    Scorers []ScorerSpec  // how to score this case
}

type Variant struct {
    Name    string
    Charter string        // path to a .sigma/ bundle (the config under test)
}

type Result struct {
    Output string
    Trace  []hooks.Event  // captured event stream
    Err    error
}

type Score struct {
    Scorer string
    Value  float64        // 0..1
    Pass   bool
    Detail string
}

type Runner interface { Run(ctx, v Variant, c Case) (Result, error) }
type Scorer interface { Score(ctx, c Case, r Result) []Score }

type Experiment struct {
    Name     string
    Variants []Variant    // exactly 2 for A/B
    Cases    []Case
    Repeats  int          // runs per (variant, case); >1 only meaningful live
}
```

## Scorers (the mix)

- **programmatic** — shell/file assertions: `run: go test ./...` (exit 0), `file X contains Y`, `test ! -e forbidden`. Deterministic; reuses the `codehealth` check pattern.
- **trace** — assertions over the event stream (from the hooks sink): tools used / not used, ordering, `no: ToolError`, `max_turns`, ended on a passing Stop gate.
- **judge** — an LLM scores the result/trace against a rubric or the charter's criteria, returning value + rationale. Handles fuzzy goals. (Uses the LLM port; judge model configurable.)

A case's total score = weighted/mean of its scorers' values (config'able; default mean, all must-pass gates respected).

## Statistical comparison

Cases are the sample. Each case is scored under variant A and B (paired). The report gives:

- per-variant mean score + per-case table,
- **paired delta** (B − A) per case,
- a **significance verdict** on the deltas — start with the sign test / Wilcoxon signed-rank (nonparametric, small-N friendly); paired bootstrap CI as an upgrade.

With `--live` + `Repeats>1`, average a case's repeats first (captures within-case nondeterminism), then compare across cases. Replay is deterministic, so significance comes from variance *across cases*, not repeats.

## Trigger

- CLI: `sigma eval <experiment.yaml>` → runs, prints the A/B report; `--live` to hit the real model; `--json` for machine output.
- Library: `eval.Run(ctx, exp, opts)` for higher layers (open-refinery, workflow tuning) to embed.

## Experiment file

```yaml
name: tighten-review-charter
variants:
  - { name: before, charter: .sigma }
  - { name: after,  charter: experiments/charter-v2 }
cases:
  - name: fix-failing-test
    prompt: "Fix the failing test in internal/foo."
    setup: experiments/seeds/foo-broken
    scorers:
      - programmatic: { checks: ["go test ./internal/foo/"] }
      - trace:        { notUsed: [worktree], noError: true, maxTurns: 8 }
      - judge:        { rubric: "Fixed the bug without touching unrelated files?" }
repeats: 1
```

Replay transcripts live at `experiments/<name>/transcripts/<variant>-<case>.jsonl`, recorded from a prior `--live` run.

## Phasing

- **E0 — core + replay + programmatic + trigger.** ✅ Types (Case/Variant/Result/Score, Runner/Scorer ports), replay Runner (fake LLM replays a transcript fixture), programmatic Scorer, Experiment runner, A/B Report (means + per-case table), `sigma eval`. Deterministic, no creds, fully testable.
- **E1 — scorers + stats + reporting.** ✅ trace Scorer; judge Scorer (LLM port, degrades when no creds); paired sign-test significance; **pluggable `Reporter` port + registry (level → reporter)** so each higher level renders its own report; default HTML reporter; `sigma eval` writes `report.html`.
- **E2 — live + charter parameterization.** ✅ Extracted the composition root into `internal/app.Build` (charter loaded via a config-root chdir), used by both `main` and eval. LiveRunner builds the real agent under a variant's charter in a scratch workspace and records the transcript (feeds replay). `sigma eval --live`. (Eval runs are non-isolated for determinism.)
- **E3 — generic higher-layer reuse.** ✅ Generalized `Variant` with `Params` (beyond `Charter`); added `RunnerFunc`/`ScorerFunc` adapters so any layer supplies runner/scorers inline. Embedding demonstrated by a non-charter policy A/B test; see `docs/eval-embedding.md`.

## Risks / calls

- **Charter parameterization needs a config-root seam.** Loaders read fixed paths today; E2 threads a root. Until then (E0/E1) replay sidesteps it — the charter difference lives in the recorded transcript.
- **Judge nondeterminism.** The judge is itself an LLM; pin its model + low temp, and treat judge scores as one signal among the mix, not sole truth.
- **Significance with tiny suites.** Report the test but also the raw per-case deltas; don't over-claim significance on 3 cases.

First step after this doc: **E0** — `internal/eval` core, replay runner, programmatic scorer, `sigma eval`, A/B report.
