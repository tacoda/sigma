# Sigma

A coding-agent CLI in Go. It reuses your Claude Code subscription credentials,
so no API key is needed.

Sigma is a learning project: a minimal but complete agent harness with the
Claude-standard feature set — tools, a permission gate, rules, slash commands,
skills, MCP servers, hooks, sub-agents, and resumable sessions.

## Requirements

- Go 1.25+
- macOS with Claude Code already authenticated (Sigma reads its OAuth token from
  the login Keychain). The Linux/file fallback reads `~/.claude/.credentials.json`.

## Build

```sh
go build -o sigma ./cmd/sigma
```

## Usage

```sh
sigma version                  # print version
sigma auth status              # show credential status (no API call)
sigma auth test                # verify credentials with a live call
sigma auth refresh             # force an OAuth token refresh
sigma run [--yes] <prompt>     # one-shot agent run (--yes auto-approves tools)
sigma chat [--resume]          # interactive TUI session
```

Example:

```sh
sigma run "Find the TODOs in this repo and summarize them."
sigma chat            # then type; /help lists commands, ctrl+c quits
```

## Authentication

Sigma calls the Anthropic Messages API with the OAuth access token Claude Code
stores (Keychain service `Claude Code-credentials`). When the token expires it
refreshes and writes the new credential back to that shared entry. No API key is
involved.

## Configuration

Settings merge from `~/.sigma/settings.json` (user) then `./.sigma/settings.json`
(project, wins on conflict):

```json
{
  "model": "claude-sonnet-4-6",
  "allowedTools": ["bash", "grep"],
  "mcpServers": {
    "fs": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "."] },
    "remote": { "url": "https://example.com/mcp" }
  },
  "hooks": {
    "PreToolUse": ["./guard.sh"],
    "PostToolUse": ["./log.sh"]
  }
}
```

- **model** — overrides the default agent model.
- **allowedTools** — auto-approved; the permission gate won't prompt for these.
- **mcpServers** — external MCP servers (stdio `command`/`args`/`env` or http `url`).
  Their tools appear namespaced as `server__tool`.
- **hooks** — shell commands run around tool calls. They get a JSON payload on
  stdin and `SIGMA_TOOL` in the environment. A `PreToolUse` command that exits
  non-zero blocks the tool; its output is the reason.

## Features

| Feature | Where |
| --- | --- |
| Tools | `read_file`, `write_file`, `edit_file`, `bash`, `glob`, `grep` |
| Permission gate | read-only tools auto-run; mutating tools prompt y/always/no |
| Rules | `~/.claude/CLAUDE.md` + `./CLAUDE.md` injected into the system prompt |
| Slash commands | `~/.sigma/commands/*.md` + `./.sigma/commands/*.md`; `$ARGUMENTS` expansion |
| Skills | `./.sigma/skills/<name>/SKILL.md`; descriptions in context, body loaded on demand |
| MCP | stdio + streamable-http servers via the official Go SDK |
| Hooks | `PreToolUse` / `PostToolUse` shell commands |
| Sub-agents | the `task` tool delegates a subtask to a fresh agent |
| Sessions | `chat` autosaves to `./.sigma/session.json`; `--resume` continues it |

## Layout

```
cmd/sigma        entrypoint, subcommand router
internal/
  auth           Claude Code OAuth credentials + refresh
  anthropic      Messages API client (SSE streaming, tool use)
  agent          conversation loop, tool dispatch, sub-agents
  tools          built-in tools + registry
  permission     allow / ask / deny gate
  config         layered settings
  rules          CLAUDE.md discovery
  commands       slash-command templates
  skills         progressive-disclosure skills
  mcp            MCP client
  hooks          tool-lifecycle shell hooks
  session        conversation persistence
  tui            Bubble Tea chat UI
```

See `PLAN.md` for the phased build and the verified auth recipe.
