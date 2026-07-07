// Package agent runs the conversation loop: send messages, execute requested
// tools, feed results back, repeat until the model stops asking for tools.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/tools"
)

const maxTokens = 4096

// maxIterations bounds the tool-use loop so a model stuck calling tools cannot
// run unbounded API requests.
const maxIterations = 50

// maxGateRetries bounds how many times a gate rejection (a failing Stop
// validation hook, or a rejected LLM response) can force the turn to continue
// before giving up.
const maxGateRetries = 3

// Feedback prefixes fed back to the model when a gate rejects.
const (
	stopRetryPrefix = "A validation gate rejected this result. Fix the following, then finish:\n\n"
	respRetryPrefix = "A response gate rejected that reply. Address this, then continue:\n\n"
	promptRejected  = "⛔ prompt rejected: "
)

var (
	errDenied  = errors.New("denied by user")
	errBlocked = errors.New("blocked by hook")
)

// UI receives agent output. Implementations drive a console or a TUI.
type UI interface {
	// Text receives streamed assistant text deltas.
	Text(delta string)
	// ToolCall is called before a tool runs.
	ToolCall(name, input string)
	// ToolResult is called after a tool runs, with its output and whether it
	// failed.
	ToolResult(name, output string, isErr bool)
	// Usage reports token counts for each model response.
	Usage(inTokens, outTokens int)
}

// PermissionPolicy decides whether a mutating tool may run. Adapters include
// interactive prompting, auto-approve, and a config allowlist.
type PermissionPolicy interface {
	Allow(name, detail string) bool
}

// LLM is the model port: send a request, stream the response. *anthropic.Client
// satisfies it; tests supply a fake.
type LLM interface {
	Stream(ctx context.Context, req message.Request, onText func(string)) (*message.Result, error)
}

// Config holds an agent's collaborators.
type Config struct {
	Client     LLM
	Tools      *tools.Registry
	Permission PermissionPolicy
	UI         UI
	Hooks      hooks.Bus
	Model      string
	System     string
	// CompactAt summarizes the history once a request's input tokens reach this
	// count. 0 disables compaction.
	CompactAt int
}

// Agent holds conversation state across turns.
type Agent struct {
	cfg       Config
	messages  []message.Message
	lastInput int // input tokens of the most recent request
}

// New creates an agent. A nil Hooks is replaced with a no-op.
func New(cfg Config) *Agent {
	if cfg.Hooks == nil {
		cfg.Hooks = hooks.Nop{}
	}
	return &Agent{cfg: cfg}
}

// Snapshot returns the conversation history (for persistence).
func (a *Agent) Snapshot() []message.Message { return a.messages }

// Restore replaces the conversation history (for resume).
func (a *Agent) Restore(m []message.Message) { a.messages = m }

// Reset clears the conversation history, starting a fresh session.
func (a *Agent) Reset() { a.messages = nil }

// Run processes one user input, looping through any tool calls until the model
// produces a final answer.
func (a *Agent) Run(ctx context.Context, input string) error {
	// Compact prior history at the turn boundary (clean, no split tool pairs).
	if a.shouldCompact() {
		a.compact(ctx)
	}
	// UserPromptSubmit gate: a block rejects the prompt before any model call.
	if o := a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.UserPrompt, Prompt: input}); o.Block {
		a.cfg.UI.Text(promptRejected + o.Reason)
		return nil
	}
	a.messages = append(a.messages, message.UserText(input))

	stopBlocks, respBlocks := 0, 0
	for i := 0; i < maxIterations; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.PreLLM})
		result, err := a.cfg.Client.Stream(ctx, message.Request{
			Model:     a.cfg.Model,
			MaxTokens: maxTokens,
			System:    a.cfg.System,
			Messages:  a.messages,
			Tools:     a.cfg.Tools.Defs(),
		}, a.cfg.UI.Text)
		if err != nil {
			return err
		}
		a.messages = append(a.messages, message.Message{Role: "assistant", Content: result.Content})
		a.lastInput = result.Usage.InputTokens
		o := a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.PostLLM,
			InTokens: result.Usage.InputTokens, OutTokens: result.Usage.OutputTokens})
		a.cfg.UI.Usage(result.Usage.InputTokens, result.Usage.OutputTokens)

		// PostLLMResponse gate: a block rejects this reply before tools run;
		// feed the reason back and re-request.
		if o.Block {
			respBlocks++
			if respBlocks > maxGateRetries {
				return fmt.Errorf("response gate still failing after %d attempts: %s", maxGateRetries, o.Reason)
			}
			a.messages = append(a.messages, message.UserText(respRetryPrefix+o.Reason))
			continue
		}

		if result.StopReason != "tool_use" {
			o = a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.Stop})
			if !o.Block {
				return nil
			}
			// A validation gate rejected the result. Feed the reason back and
			// let the model fix it, bounded by maxStopRetries.
			stopBlocks++
			if stopBlocks > maxGateRetries {
				return fmt.Errorf("validation gate still failing after %d attempts: %s", maxGateRetries, o.Reason)
			}
			a.messages = append(a.messages, message.UserText(stopRetryPrefix+o.Reason))
			continue
		}
		a.messages = append(a.messages, message.Message{
			Role:    "user",
			Content: a.runTools(ctx, result.ToolUses()),
		})
	}
	return fmt.Errorf("exceeded max tool iterations (%d)", maxIterations)
}

const (
	compactMaxTokens   = 2048
	compactSystem      = "You compact a coding session into a concise summary for continuation."
	compactInstruction = "Summarize the conversation so far. Preserve the user's goals, decisions made, files changed and how, commands run and their results, and any open tasks. Be concise but complete enough to keep working."
	compactedPrefix    = "Summary of the conversation so far:\n\n"
)

// shouldCompact reports whether the history has grown past the configured
// threshold and is worth summarizing.
func (a *Agent) shouldCompact() bool {
	return a.cfg.CompactAt > 0 && a.lastInput >= a.cfg.CompactAt && len(a.messages) >= 2
}

// compact summarizes the current history into a single message. On any failure
// the history is left untouched, so compaction never breaks a turn.
func (a *Agent) compact(ctx context.Context) {
	a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.PreCompact})
	msgs := append(append([]message.Message{}, a.messages...), message.UserText(compactInstruction))
	res, err := a.cfg.Client.Stream(ctx, message.Request{
		Model:     a.cfg.Model,
		MaxTokens: compactMaxTokens,
		System:    compactSystem,
		Messages:  msgs,
	}, nil)
	if err != nil || strings.TrimSpace(res.Text()) == "" {
		return
	}
	a.messages = []message.Message{message.UserText(compactedPrefix + res.Text())}
	a.lastInput = 0
	a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.PostCompact})
}

// runTools executes each requested tool and returns the tool_result blocks.
func (a *Agent) runTools(ctx context.Context, uses []message.Block) []message.Block {
	results := make([]message.Block, 0, len(uses))
	for _, use := range uses {
		input := string(use.Input)
		a.cfg.UI.ToolCall(use.Name, input)

		if o := a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.PreTool, Tool: use.Name, Input: input}); o.Block {
			results = append(results, toolResult(use.ID, o.Reason, errBlocked))
			continue
		}
		if !a.cfg.Tools.ReadOnly(use.Name) && !a.cfg.Permission.Allow(use.Name, input) {
			results = append(results, toolResult(use.ID, "", errDenied))
			continue
		}
		out, err := a.cfg.Tools.Run(ctx, use.Name, use.Input)
		a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.PostTool, Tool: use.Name, Output: out})
		if err != nil {
			a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.ToolError, Tool: use.Name, Output: out})
		}
		a.cfg.UI.ToolResult(use.Name, out, err != nil)
		results = append(results, toolResult(use.ID, out, err))
	}
	return results
}

func toolResult(id, out string, err error) message.Block {
	b := message.Block{Type: "tool_result", ToolUseID: id}
	if err != nil {
		b.Content = err.Error()
		if out != "" {
			b.Content += ": " + out
		}
		b.IsError = true
		return b
	}
	b.Content = out
	return b
}
