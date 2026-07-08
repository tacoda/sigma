# Sigma — Claude Code Parity Plan

**Goal:** Claude Code in *capability*, hexagonal (ports & adapters) in *philosophy*, extras shipped as *plugins*.

---

## Status — P0–P3 complete (2026-07)

| Phase | Status | What shipped |
|---|---|---|
| **P0** Foundation | ✅ | Makefile; fake LLM; golden agent-loop test |
| **P1** Ports & adapters | ✅ | `message` domain split from `anthropic`; ports: LLM, Executor, ContextSource (`prompt.Source`), SessionStore, PermissionPolicy, HookBus, Workspace, UI, Tool; core imports adapters only at the composition root |
| **P2** Capability parity | ✅ | Expanded `hooks.Bus` (12 events; Go callbacks + declarative YAML + shell adapters); worktrees; tool rooting; subagent isolation; sandbox (`exec.Sandbox`); permission modes; `Block` honored at prompt/response/tool/stop; compaction; prompt caching; subagent types + parallel fanout; output styles; memory hierarchy + `@imports`; MCP resources/prompts |
| **P3** Extras as plugins | ✅ | Plugin host + per-plugin config; `telemetry`, `codehealth` (Stop gate), `stylepack`; declarative workflow engine; `sigma init` scaffolding |

**Design realized (sensors / guides / gates):** sensors = event bus + JSONL sink; guides = rules/skills/styles/agent-prompts via `prompt.Source`; gates = permission+modes, 4 hook block points, rooting, worktree isolation, sandbox, code-health Stop gate. Every layer is a typed adapter behind a narrow port or a hook subscriber; all default-safe; golden loop test green throughout.

**Deferred:** TUI runtime `/style` + plan-mode exit flow; isolated-parallel fanout; conversation-prefix caching; workflow loops/conditionals.

**Next project:** eval harness (see `docs/EVAL_PLAN.md`).

---

**Two load-bearing decisions (locked):**

1. **Ports & adapters is the spine.** The agent core depends only on narrow, specific port interfaces. Every existing extension point (tools, hooks, rules, skills, commands, MCP, permission, session, LLM, UI) is migrated to sit *behind a port as an adapter*. There is **no** universal `Plugin` contract in the core — ports stay specific on purpose.
2. **A "plugin" is a separate, higher-level concept.** A plugin is an optional *bundle* that packages one or more adapters + assets (skills/commands/hooks) + metadata, enabled/disabled via config. Plugins compose *over* ports; they are not how the core extends. Reserved for Phase 3 extras.
3. **Sequencing: refactor the spine first.** Phase 1 extracts ports and migrates today's behavior onto them with no feature change. Everything after is built against ports.

---

## 1. Current state (baseline)

Sigma already has, wired ad-hoc through `cmd/sigma/main.go:buildDeps()`:

| Concern | Location | Shape today |
|---|---|---|
| Tools | `internal/tools/` | Clean `Tool` interface + `Registry` — **already a good port** |
| Agent loop | `internal/agent/agent.go:69` | Stream → tool_use → runTools, 50-iter cap |
| LLM client | `internal/anthropic/` | `Stream()` over SSE, OAuth bearer |
| Permission | `internal/permission/` | `Gate.Allow(name, detail) bool`, session memo |
| Hooks | `internal/hooks/` | Shell only, **PreToolUse / PostToolUse only** |
| Rules | `internal/rules/` | `CLAUDE.md` user+project → system prompt |
| Skills | `internal/skills/` | `SKILL.md` frontmatter index + lazy body tool |
| Commands | `internal/commands/` | `*.md` slash commands, `$ARGUMENTS` |
| MCP | `internal/mcp/` | stdio/HTTP client, tools namespaced `srv__tool` |
| Subagent | `internal/agent/subagent.go` | `task` tool, one level deep |
| Session | `internal/session/` | JSON snapshot of messages |
| UI | `internal/tui/` | Bubble Tea, `bridge` implements `UI`+`Approver` |

**Gaps vs Claude Code:** worktrees, sandboxing, expanded hook events, permission modes (plan/acceptEdits/bypass), context compaction, prompt caching, TodoWrite, Web tools, glob `**`, subagent types + parallel fan-out, statusline, output styles, memory hierarchy/imports, MCP resources/prompts.

**Architectural debt for the refactor:** each extension point wires in a *different* way. `Tool` is the only real port. Ports & adapters unifies the wiring discipline (narrow interface + composition root) without forcing a one-size contract.

---

## 2. Port catalog (the spine)

Core defines each port as a **narrow** interface in a new `internal/core/ports` package. Adapters live in their own packages and are constructed in a composition root. The agent depends on ports only.

```go
// internal/core/ports

// --- already effectively a port ---
type Tool interface {
    Name() string
    Description() string
    Schema() json.RawMessage
    ReadOnly() bool
    Run(ctx context.Context, input json.RawMessage) (string, error)
}

// LLM backend. Adapter: anthropic. (Lets you swap providers / fakes in tests.)
type LLM interface {
    Stream(ctx context.Context, req Request, onText func(string)) (Result, error)
}

// Permission decision. Adapters: interactive, auto, allowlist, mode-based.
// Composable as a chain (first decisive wins).
type PermissionPolicy interface {
    Decide(ctx context.Context, req PermissionRequest) Decision // Allow | Deny | Ask
}

// Lifecycle events. Adapters: in-proc handlers, shell-hook runner.
// THE place hooks live — many more events than today (see §4).
type HookBus interface {
    Emit(ctx context.Context, ev Event) (HookOutcome, error) // may block/modify/inject
}

// Contributes segments to the system prompt / context. Adapters: rules, skills-index, memory.
type ContextSource interface {
    Contribute(ctx context.Context) ([]ContextBlock, error)
}

// Slash commands. Adapter: commands (*.md).
type CommandSource interface {
    Commands() []Command
}

// Skill lookup + lazy body. Adapter: skills.
type SkillStore interface {
    Index() []SkillMeta
    Body(name string) (string, error)
}

// Conversation persistence. Adapter: json-file session.
type SessionStore interface {
    Load() ([]Message, bool, error)
    Save(msgs []Message) error
}

// Command execution environment. Adapters: local exec, sandboxed exec.
// bash tool + worktree ops route through this — never os/exec directly.
type Executor interface {
    Run(ctx context.Context, spec ExecSpec) (ExecResult, error)
}

// Isolated workspace lifecycle. Adapter: git-worktree.
type Workspace interface {
    Create(ctx context.Context, base string) (Handle, error)
    Merge(ctx context.Context, h Handle) error
    Discard(ctx context.Context, h Handle) error
}

// Output surface. Adapters: TUI bridge, plain stdout, statusline.
type UI interface {
    Text(delta string)
    ToolCall(name string, input json.RawMessage)
    Status(s Status) // for statusline / progress
}
```

**Composition root** replaces `buildDeps()`: reads config, constructs concrete adapters, injects ports into the agent. One file, explicit, testable. No magic registry for core ports — specific wiring beats reflection.

**Rule the whole refactor enforces:** the agent core imports `ports` only. Adapters import `ports` + their deps. Nothing in core imports an adapter package.

---

## 3. Migration map (Phase 1 — behavior-preserving)

Each existing package becomes an adapter behind a specific port. No feature change; golden tests (see §7) prove parity.

| Today | Becomes | Port |
|---|---|---|
| `tools.Registry` | keep, minor cleanup | `Tool` (already) |
| `anthropic.Client` | `anthropic` adapter | `LLM` |
| `permission.Gate` | `perm/interactive`, `perm/auto`, `perm/allowlist` adapters, chained | `PermissionPolicy` |
| `hooks.Runner` (shell) | `hooks/shell` adapter subscribing to events | `HookBus` |
| `rules.Load` | `context/rules` adapter | `ContextSource` |
| `skills` index | `context/skills` adapter (+ existing skill tool) | `ContextSource` + `SkillStore` |
| `commands` | `commands` adapter | `CommandSource` |
| `mcp.Connect` | `mcp` adapter emitting `Tool`s | supplies `Tool`s |
| `session` | `session/jsonfile` adapter | `SessionStore` |
| `tui.bridge` | `ui/tui` adapter | `UI` (+ drives `PermissionPolicy` interactive) |
| local `os/exec` in `bash.go` | `exec/local` adapter | `Executor` |

Exit criteria: `sigma run` / `sigma chat` behave identically; golden transcript tests pass; core imports only `ports`.

---

## 4. Hook events — the explicit ask ("more places for hooks to exist")

Today: `PreToolUse`, `PostToolUse`, shell-only, exit-code contract. Expand to a typed event set on the `HookBus`, with **two adapter kinds**:

- **In-proc handlers** — rich: can block, rewrite tool input, inject context blocks, add system messages, cancel the turn.
- **Shell-hook runner** — Claude-Code-compatible contract (JSON on stdin, exit code + stdout), so existing shell hooks keep working. This is the migration of today's `hooks.Runner`.

**Event set (superset of Claude Code):**

| Event | When | Handler can |
|---|---|---|
| `SessionStart` / `SessionEnd` | chat/run open & close | inject context, seed memory |
| `UserPromptSubmit` | user submits input | rewrite/reject prompt, inject context |
| `PreLLMRequest` | **new** — before each model call | mutate system/messages, add cache breakpoints |
| `PostLLMResponse` | **new** — after each model call | inspect usage, redact |
| `PreToolUse` | before a tool runs | block, modify input |
| `PostToolUse` | after a tool returns | inspect/annotate output |
| `ToolError` | **new** — tool returned error | retry hint, escalate |
| `FileChange` | **new** — after write/edit | format, lint, git add |
| `PermissionRequest` | before asking user | auto-decide, log |
| `Stop` / `TurnEnd` | assistant ends turn | continue-loop, checkpoint |
| `SubagentStart` / `SubagentStop` | subagent lifecycle | scope tools, collect output |
| `PreCompact` / `PostCompact` | **new** — around compaction | preserve pinned context |
| `Notification` | out-of-band notice | route to Slack/desktop |

Config keeps the familiar map shape; in-proc handlers register via the composition root or (Phase 3) a plugin bundle.

---

## 5. Phase 2 — capability parity (all built against ports)

Each item is an adapter or a `Tool`, landing on the Phase 1 spine.

- **5.1 Expanded hook events** (§4) — the priority ask. In-proc + shell adapters on `HookBus`.
- **5.2 Worktrees** — `Workspace` adapter (`git worktree`). Subagents/tasks run in an isolated worktree; `Merge`/`Discard` on completion. Auto-discard if untouched.
- **5.3 Sandboxing** — `Executor` sandbox adapter. macOS: `sandbox-exec`/seatbelt profile; Linux: `bwrap` + landlock/seccomp. FS + network policy from config. `bash` tool routes through `Executor`, so sandbox is a swap, not a rewrite.
- **5.4 Permission modes** — `default` / `acceptEdits` / `plan` / `bypassPermissions` as composed `PermissionPolicy` adapters. `plan` mode = read-only tools only until approval.
- **5.5 Missing tools** — `TodoWrite` (task tracking), `WebFetch`/`WebSearch`, `MultiEdit`, glob `**` fix.
- **5.6 Context management** — auto-compaction when near context limit + `PreCompact`/`PostCompact` hooks; prompt caching (`cache_control` on system/tools) via `PreLLMRequest`.
- **5.7 Subagents** — named agent *types* (own tools + system prompt), parallel fan-out for `Task`, worktree isolation per subagent (ties 5.2).
- **5.8 Surface** — statusline (`UI.Status`), output styles, memory hierarchy + `@import` in `CLAUDE.md`, MCP resources/prompts (not just tools).

Order within Phase 2: `5.1 → 5.4` (policy/hooks foundation) → `5.2/5.3` (isolation) → `5.5` (cheap tool wins) → `5.6/5.7` (harder) → `5.8` (polish).

---

## 6. Phase 3 — sigma extras (as plugins)

Now the *plugin* concept lands: a bundle that registers adapters + ships assets, toggled in config. Each extra is a plugin, none touch the core.

- **6.1 Plugin host** — a thin `Plugin` bundle type: `Register(host)` attaches its adapters to ports; manifest for name/enable/config; discovery under `~/.sigma/plugins/` and `./.sigma/plugins/`. (Distinct from ports — packaging only.)
- **6.2 Workflow engine** — higher-level multi-agent orchestration (pipeline / parallel / fan-out / verify), the harness-style layer above single-agent. Ships as a plugin exposing a `workflow` tool + commands.
- **6.3 Code-health gate** — CodeScene pre-commit/PR gate as a `PostToolUse`/`FileChange` hook plugin.
- **6.4 Telemetry / insight** — session metrics via hook subscriptions.
- **6.5 Output-style plugins** — caveman / ponytail etc. as `UI` decorators + `SessionStart` context injectors.
- **6.6 Scaffolding** — `sigma plugin new` command to stamp a plugin skeleton.

---

## 7. Testing & safety strategy

Refactoring the spine with **no CI and no tests today** is the top risk. Mitigate before Phase 1 touches code:

- **7.0 Foundation** — `Makefile` (build/test/lint), fake `LLM` adapter that replays canned SSE, **golden transcript tests** around `agent.Run` (fixed input → fixed tool-call sequence). These pin current behavior so the port migration is provably behavior-preserving.
- **Per phase** — every new adapter ships with a table test against its port; ports get a fake for isolating the agent loop.
- **Sandbox/worktree** — integration tests behind a build tag (need git + OS sandbox).
- **Code health** — run the CodeScene pre-commit safeguard before each commit (per CLAUDE.md); target 10.0 on new adapter code.

---

## 8. Risks & calls

| Risk | Call |
|---|---|
| Big-bang refactor breaks working CLI | Golden tests first (§7.0); migrate one port per commit |
| Over-abstraction (ports nobody needs) | Only extract a port when a 2nd adapter or a test fake justifies it; `Tool`/`LLM`/`PermissionPolicy`/`Executor` clear the bar today |
| Sandbox portability (mac vs linux) | `Executor` port isolates it; ship local adapter first, sandbox adapter as opt-in |
| "Plugin" scope creep back into core | Hard rule: plugins only compose adapters over existing ports; if a plugin needs a new core hook, add the *port/event* first |

---

## 9. Milestone summary

- **P0** Foundation: Makefile, fake LLM, golden tests.
- **P1** Ports & adapters spine + migrate all existing extension points (behavior-preserving).
- **P2** Capability parity: hook events → permission modes → worktrees/sandbox → tools → compaction/caching → subagent types → surface.
- **P3** Extras as plugins: plugin host → workflows → code-health → telemetry → output styles → scaffolding.

First concrete step after approval: **P0 + define `internal/core/ports`**, then migrate the `LLM` port (smallest, unblocks the fake + golden tests).
