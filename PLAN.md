# Sigma — Go Coding Agent

A coding-agent CLI built to learn how this tooling works. Reuses Claude Code
subscription auth (OAuth from the macOS Keychain), no API key required.

## Decisions

- Module: `github.com/tacoda/sigma`
- TUI: bubbletea (charmbracelet), introduced in Phase 5
- HTTP: stdlib `net/http` (no SDK)
- Auth: reuse Claude Code OAuth creds; API key path deferred

## Verified auth recipe (Phase 1, proven live)

- Token source: macOS Keychain service `Claude Code-credentials`,
  JSON `claudeAiOauth.accessToken` (fallback `~/.claude/.credentials.json`).
- Endpoint: `POST https://api.anthropic.com/v1/messages`
- Headers: `authorization: Bearer <token>`, `anthropic-version: 2023-06-01`,
  `anthropic-beta: oauth-2025-04-20`. No `x-api-key`.
- System prompt first block must be
  `"You are Claude Code, Anthropic's official CLI for Claude."`
- Token TTL ~8h; refresh grant deferred (see Phase 1 notes).

## Architecture (target)

```
cmd/sigma/main.go        entrypoint, subcommand router
internal/
  auth/        Claude Code OAuth creds load + expiry (refresh later)
  anthropic/   Messages API client (non-stream now, SSE in Phase 2)
  agent/       conversation loop, tool dispatch
  tools/       bash, read, write, edit, glob, grep
  permission/  allow / ask / deny gate
  config/      settings.json load + merge
  rules/       CLAUDE.md discovery + system-prompt assembly
  commands/    slash commands
  skills/      progressive-disclosure skill loader
  mcp/         MCP client (stdio + http)
  session/     history + persistence
  tui/         bubbletea chat UI
```

## Phases

- **0 — Scaffold:** go.mod, structure, `sigma version`. ✅ build runs.
- **1 — Auth + bare client:** keychain read, non-streaming Messages call,
  `sigma auth test` round-trip. ✅ proven gate.
- **2 — Streaming + agent loop:** SSE parse, one tool (`read_file`),
  tool-use loop until stop.
- **3 — Full tools + permissions:** bash, write, edit, glob, grep + gate.
- **4 — Config + rules:** settings.json merge, CLAUDE.md injection.
- **5 — TUI:** bubbletea chat loop, streaming render.
- **6 — Slash commands:** markdown command files + built-ins.
- **7 — Skills:** progressive disclosure (index names, load on trigger).
- **8 — MCP client:** stdio + http transports, tool namespacing.
- **9 — Extras:** subagents, hooks, session resume, token refresh.

Each phase: small commits, tests where logic is non-trivial, Code Health gate
before "done".

## Phase 1 notes / open risks

- Token **refresh** is deferred. When expired, Sigma errors and asks the user
  to re-auth via Claude Code, rather than writing the shared Keychain entry
  (avoids clobbering Claude Code's credential). Real refresh: Phase 9.
- The required-system-prompt assumption is unverified by negative test; kept
  because it is known-good.
