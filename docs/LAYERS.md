# Sigma — Layers Plan

**Idea:** lean into *layers* as sigma's central architecture. The agent loop is a thin core; every cross-cutting concern is a **layer** wrapping a hot path — a dedicated middleware stack for an agent (Rack / Express / gRPC interceptors, but for a coding agent).

A layer receives `next` and can **observe**, **transform** (rewrite in/out), **short-circuit** (block / deny / cache-hit / validation-fail), or **wrap** (retry / timeout / isolate). This generalizes the sensors/guides/gates vision into one mechanism: gates and sensors are layers; guides are context-path layers.

The codebase already drifts here — formalizing finishes a pattern sigma keeps reaching for (see "It already exists" below).

## Three spines (kept separate on purpose)

Different paths have different signatures. Do **not** collapse into one mega-interface — that loses type safety and clarity.

### 1. Model path — wraps the `LLM` port

```go
type LLMLayer func(agent.LLM) agent.LLM
```

Request in → response out. Layers: context-injection, prompt-cache, redaction/DLP, response-gate (retry on block), token-budget, retry/backoff, record, replay, model-router, telemetry.

### 2. Tool path — wraps tool invocation

```go
type ToolCall struct { Name string; Input json.RawMessage }
type Invoker interface { Invoke(ctx context.Context, c ToolCall) (string, error) }
type ToolLayer func(Invoker) Invoker
```

Call in → result out. This is the most scattered concern today (permission, canon guards, rooting, sandbox, hooks all fire inline in `runTools`). Canonical order (outer → inner): telemetry → permission(+modes) → canon-guards → path-rooting → sandbox → timeout → dry-run → result-cache → **exec** (the registry).

### 3. Turn path — wraps `Run(input)`

```go
type Turn func(ctx context.Context, input string) error
type TurnLayer func(Turn) Turn
```

Layers: session load/save, worktree setup/teardown, compaction trigger, stop-gate (validation) loop, session events.

Context / prompt assembly folds into a model-path layer (it transforms the `Request`). Sensors (`hooks.Bus`) remain the **observe** tap; **middleware is the act spine**. A layer may emit events; it does not replace the bus.

## It already exists (this is a refactor, not a rewrite)

| Today | Becomes |
|---|---|
| `recordingLLM`, `replayLLM` (eval) | model layers — proof the decorator works and is in use |
| prompt caching (`anthropic.wireOf`) | model layer |
| compaction, response/Stop gates | model / turn layers |
| `permission.ForMode` composed chain | tool layer (already composed) |
| canon guards (PreToolUse block) | tool layer |
| tool rooting, `exec.Sandbox` | tool layers |
| worktree isolation | turn layer |
| session persistence | turn layer |

## Composition & configuration

- `plugin.Host` grows `AddLLMLayer` / `AddToolLayer` / `AddTurnLayer`. Plugins contribute layers; `app.Build` composes them.
- The charter (`.sigma/settings.json`) declares order and per-layer config. A canonical default order ships; plugins insert; a charter may reorder or disable.
- **Ordering is the new lever** (canon guards before sandbox; redaction before logging; budget outermost). Wrong order is a subtle bug, so introspection is a first-class feature (`sigma layers`).

## Features this unlocks

1. **`sigma layers`** — print the active stack per spine, in order (like `iptables -L`). Agent behavior becomes readable as a pipeline.
2. **Per-charter stack composition** — A/B whole stacks via the eval harness (already demonstrated with canon on/off).
3. **Dry-run / plan preview** — a tool layer that reports mutations instead of executing → "what would this turn do?"
4. **Token/cost budget governance** — a model layer that short-circuits at a ceiling (ties to open-refinery oversight).
5. **Redaction / DLP** — scrub secrets/PII from requests + logs (open-refinery `scan_content` as a layer).
6. **Model router** — cheap model for simple turns, strong for hard, per request.
7. **Retry / self-heal** — backoff wrapping model + tool paths; flaky-tool tolerance.
8. **Result caching** — memoize `read_file`/`grep` within a turn; cache identical LLM requests.
9. **Provenance / audit** — wrap every action with an open-refinery-style record (who/what/why/output) for lights-out legibility.
10. **Shadow / canary** — run a second stack in shadow on the same turn and diff — eval bleeds into runtime.
11. **Oversight / approval routing** — L0–L4 as a tool layer (inline-approve / async-queue / auto).
12. **Time-travel debugger** — because model layers record, replay a turn layer-by-layer.

## Phasing

- **L0 — Tool spine.** ✅ Extracted `invoker` + `toolLayer` (internal/agent/toolstack.go): `runTools` now dispatches through a composed stack `ui → hooks → permission → exec` (UI split into call/result and hooks into pre/post so block/deny semantics are exact). `sigma layers` prints the spine. Canon guards/rooting/sandbox already ride the hooks/tool/exec seams. Behavior-preserving; golden green.
- **L1 — Model spine.** ✅ `LLMLayer` (internal/agent/modelstack.go): the client for every model call (turn loop + compaction) is wrapped `budget → retry → llm`, composed in `agent.New`. Config `tokenBudget` / `llmRetries` (default off → behavior-preserving); `sigma layers` shows the spine. Note: PreLLM/PostLLM emits + the response gate stay in the turn loop (the gate acts on the PostLLM outcome, which the LLM interface can't carry), and prompt caching stays in the anthropic adapter — those are turn-spine / adapter concerns, not model layers.
- **L2 — Turn spine.** ✅ `TurnLayer` (internal/agent/turnstack.go): `Run` dispatches through `compaction → prompt-gate → loop`, composed in `agent.New`. Compaction and the UserPromptSubmit gate moved out of the loop body into turn layers; the per-iteration response/stop gates stay in `loop` (iteration control, not turn wraps). `sigma layers` prints all three spines. Behavior-preserving; golden green.
- **L3 — Layers as first-class extension.** `plugin.Host.Add*Layer`; charter-declared order + config; a couple of net-new layers (dry-run, budget) to prove the seam; A/B a stack via eval.

Each phase: keep the golden agent-loop test green, unit-test each layer against its spine interface with a fake `next`, and classify each layer sensor/guide/gate.

## Risks / calls

- **Keep the three spines separate** — resist a single `Layer` interface.
- **Bus vs middleware** — bus = observation fan-out; middleware = transform/short-circuit spine. A layer emits; it doesn't fold the bus away.
- **Ordering footguns** — needs `sigma layers` + a declared canonical order + tests asserting order.
- **Migration churn** — `runTools` and `Run` get rewired; the golden test is the safety net; migrate one spine per phase.

## Recommended first slice

**L0, the Tool spine.** Most scattered today, biggest legibility win, and the golden test guards the refactor. Then model spine (record/replay already prove the decorator), then turn spine.
