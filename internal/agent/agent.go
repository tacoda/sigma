// Package agent runs the conversation loop: send messages, execute requested
// tools, feed results back, repeat until the model stops asking for tools.
package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/tools"
)

const maxTokens = 4096

// maxIterations bounds the tool-use loop so a model stuck calling tools cannot
// run unbounded API requests.
const maxIterations = 50

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
}

// Agent holds conversation state across turns.
type Agent struct {
	cfg      Config
	messages []message.Message
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
	a.messages = append(a.messages, message.UserText(input))
	a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.UserPrompt, Prompt: input})

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
		a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.PostLLM,
			InTokens: result.Usage.InputTokens, OutTokens: result.Usage.OutputTokens})
		a.cfg.UI.Usage(result.Usage.InputTokens, result.Usage.OutputTokens)

		if result.StopReason != "tool_use" {
			a.cfg.Hooks.Emit(ctx, hooks.Event{Kind: hooks.Stop})
			return nil
		}
		a.messages = append(a.messages, message.Message{
			Role:    "user",
			Content: a.runTools(ctx, result.ToolUses()),
		})
	}
	return fmt.Errorf("exceeded max tool iterations (%d)", maxIterations)
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
