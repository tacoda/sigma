# Sigma

A reliable coding-agent CLI in Go: **Claude Code in capability, hexagonal (ports & adapters) in philosophy, extras shipped as plugins** — with a built-in A/B **eval harness** for testing changes before and after with real data.

Sigma's premise: an agent becomes reliable when its output is *constrained* by layers you compose — **sensors** (observe), **guides** (steer), and **gates** (permit / deny / validate). Every layer is a small typed adapter behind a narrow port.

> Status: early and evolving. Sigma reuses your Claude Code subscription credentials (OAuth) — no API key.

## Install

Requires Go 1.25+ and Claude Code already authenticated on this machine.

```sh
git clone https://github.com/tacoda/sigma
cd sigma
make build          # -> bin/sigma
```

## Quickstart

```sh
sigma auth status              # verify Claude Code credentials
sigma init                     # scaffold a starter .sigma/ (config + examples)
sigma run "fix the failing test in internal/foo"   # one-shot
sigma chat                     # interactive TUI session (--resume to continue)
```

`run` gates every mutating tool call for approval (`--yes` auto-approves). Read-only tools run without prompting.

## Authentication

Sigma calls the Anthropic Messages API with the OAuth access token Claude Code stores (macOS Keychain service `Claude Code-credentials`; Linux/file fallback `~/.claude/.credentials.json`). It refreshes and writes back the credential when it expires. No API key is involved. `sigma auth status|test|refresh` inspects and manages it.

## The charter — your `.sigma/` config

A **charter** is the configuration bundle the agent loads from `.sigma/` (and `~/.sigma/` for user-global). It shapes behavior without touching code:

| File / dir | Purpose |
|---|---|
| `.sigma/settings.json` | model, permission mode, sandbox, plugins, compaction, event log |
| `CLAUDE.md` | instructions (hierarchy from repo root down; `@path` imports) |
| `.sigma/skills/<name>/SKILL.md` | on-demand skills (progressive disclosure) |
| `.sigma/agents/<name>.md` | named sub-agent types (own prompt + tool subset) |
| `.sigma/styles/<name>.md` | output styles (voice / behavior) |
| `.sigma/workflows/<name>.yaml` | declarative multi-step / parallel orchestrations |
| `.sigma/commands/<name>.md` | slash commands (`$ARGUMENTS` expansion) |
| `.sigma/hooks.yaml` | declarative lifecycle hooks (match + run / deny / log / notify) |

Copy-paste starters live in `docs/*.example.*` (`settings`, `hooks`, `agent-type`, `output-style`, `workflow`), or run `sigma init`.

## Layers: sensors, guides, gates

- **Sensors** — a typed event bus (`SessionStart`, `PreToolUse`, `PostLLMResponse`, `Stop`, …) with a JSONL audit sink.
- **Guides** — the system prompt assembled from rules, skills, output styles, and agent-type prompts.
- **Gates** — permission modes (`default` / `acceptEdits` / `plan` / `bypass`), hook blocks honored at prompt / response / tool / stop, filesystem path rooting, git-worktree isolation, an OS sandbox (seatbelt / bwrap), and a code-health Stop gate that won't let a turn finish until tests/lint pass.

Hooks can be **declarative YAML**, **shell commands**, or **in-process Go callbacks** — all on one bus. A `Stop` hook that exits non-zero makes the agent keep working until it's fixed.

## Extras (plugins)

**canon** is enabled by default: it contributes the engineering & platform canon (design, testing, safety, security, commits, agentic discipline) to the system prompt as a guide, and installs deterministic guards that block canon violations at the tool boundary — sensitive-file access, secrets or debug artifacts in written content, dangerous shell (`rm -rf`, force-push, `reset --hard`, `--no-verify`, piping downloads to a shell), and AI attribution in commits.

Optional bundles enabled in `settings.json` (`"plugins": ["telemetry", "codehealth", "styles"]`):

- **telemetry** — event-count sensor + a `telemetry_stats` tool.
- **codehealth** — a Stop gate running your checks (tests / vet / lint or a CodeScene call) + a `code_health` tool.
- **styles** — built-in output styles (concise, explanatory, caveman).

Plugins compose *over* the ports; they never touch the agent core.

## Eval harness

A general A/B experimentation primitive: compare two **variants** (e.g. two charters, before/after) over a case suite, scored with a mix of programmatic checks, event-trace assertions, and an LLM judge, with a statistically-grounded HTML report.

```sh
sigma eval experiments/tighten-review/experiment.yaml         # replay (fake, free, deterministic)
sigma eval --live experiments/tighten-review/experiment.yaml  # real agent; records transcripts
```

Runs are **fake-by-default** (replay a recorded transcript); `--live` runs the real agent and records transcripts for future replay. The engine is generic — higher layers embed it with their own runner/scorers (see `docs/eval-embedding.md`).

## Architecture

Ports & adapters. The agent core depends only on narrow interfaces; adapters are wired at a single composition root (`internal/app`).

```
internal/
  agent/      the conversation loop; ports: LLM, UI, PermissionPolicy, hooks.Bus
  message/    provider-neutral content-block model (domain)
  anthropic/  LLM adapter (Messages API, OAuth, prompt caching)
  tools/      Tool port + built-ins (read/write/edit/bash/glob/grep/worktree)
  exec/       Executor port: Local + Sandbox (seatbelt/bwrap)
  hooks/      HookBus: typed events; Callbacks + YAML rules + Shell + Sink
  permission/ PermissionPolicy + modes
  prompt rules skills styles agents commands/  guides + charter loaders
  workflows/  declarative workflow definitions
  workspace/  git-worktree isolation
  mcp/        Model Context Protocol client (tools/resources/prompts)
  plugin/ plugins/  the extras layer
  app/        composition root (builds the agent from a charter)
  eval/       A/B experimentation harness
  tui/        interactive front-end
cmd/sigma/    CLI
docs/         plans (PARITY_PLAN, EVAL_PLAN) + config examples
```

## Development

```sh
make build   # compile
make test    # run tests
make lint    # gofmt check + go vet
make golden  # regenerate golden fixtures
```

See [CONTRIBUTING.md](CONTRIBUTING.md). The phased build is documented in [`docs/PARITY_PLAN.md`](docs/PARITY_PLAN.md) and [`docs/EVAL_PLAN.md`](docs/EVAL_PLAN.md).

## License

[MIT](LICENSE) © Ian Johnson
