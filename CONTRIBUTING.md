# Contributing to Sigma

Thanks for your interest. Sigma aims to be a *reliable* coding agent, and the
codebase is opinionated about how it stays that way. A few conventions keep
contributions coherent.

## Getting started

Requires Go 1.25+.

```sh
make build   # compile to bin/sigma
make test    # run the test suite
make lint    # gofmt check + go vet
make golden  # regenerate golden fixtures (only when you intend to)
```

Please run `make test` and `make lint` before opening a PR. Both must be green.

## Architecture rules

Sigma is hexagonal — **ports & adapters**. The agent core depends only on narrow
interfaces; concrete adapters are wired at a single composition root
(`internal/app`). When adding something:

1. **Classify it: sensor, guide, or gate.** Sensors observe (hook subscribers),
   guides steer (system-prompt `prompt.Source`), gates permit/deny/validate
   (permission policy, hook `Block`, executor, workspace). Say which in the PR.
2. **Extend through a port, not the core.** A new capability is an adapter behind
   an existing port, a new hook subscriber, or a plugin bundle — not a change to
   the agent loop. Add a *new* port only when a second adapter or a test fake
   justifies it; keep ports narrow and specific.
3. **The core imports adapters only at the composition root.** `internal/agent`
   must not import a concrete adapter.
4. **Keep changes small and reviewable.** Prefer several focused commits over one
   large one. Match the surrounding style.

The phased design lives in [`docs/PARITY_PLAN.md`](docs/PARITY_PLAN.md) and
[`docs/EVAL_PLAN.md`](docs/EVAL_PLAN.md) — worth skimming before a large change.

## Testing

- Table-driven tests alongside each adapter; use a port fake to isolate the
  agent loop.
- The **golden agent-loop test** (`internal/agent`) pins behavior — keep it green;
  regenerate deliberately with `make golden` only when behavior intentionally
  changes, and explain why in the PR.
- Run concurrency-touching code under `go test -race`.
- Non-trivial logic ships with at least one runnable check.

## Config & docs

If you add a config field or an extension point, update the relevant
`docs/*.example.*` file so it's discoverable.

## Commits & PRs

- Conventional-commit style subjects (`feat(agent): …`, `fix(tools): …`,
  `docs: …`, `refactor: …`).
- Describe *what changed and why*; note the sensor/guide/gate classification for
  behavior changes.
- Ensure `make test` and `make lint` pass; update docs/examples as needed.

## Security

Sigma executes tools (including shell) and handles credentials. Don't weaken the
default gates (permission prompts, path rooting, sandbox, fail-closed backends)
without discussion. Report security issues privately via a GitHub security
advisory rather than a public issue.

By contributing you agree your contributions are licensed under the project's
[MIT License](LICENSE).
