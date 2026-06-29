// Package agent runs the conversation loop: send messages, execute requested
// tools, feed results back, repeat until the model stops asking for tools.
package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/tacoda/sigma/internal/anthropic"
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
}

// Approver decides whether a mutating tool may run.
type Approver interface {
	Allow(name, detail string) bool
}

// Streamer sends a Messages request and streams the response. *anthropic.Client
// satisfies it; tests supply a fake.
type Streamer interface {
	Stream(ctx context.Context, req anthropic.Request, onText func(string)) (*anthropic.Result, error)
}

// Hooks fire around tool execution. A blocking PreTool stops the tool.
type Hooks interface {
	PreTool(name, input string) (block bool, reason string)
	PostTool(name, output string)
}

// Config holds an agent's collaborators.
type Config struct {
	Client   Streamer
	Tools    *tools.Registry
	Approver Approver
	UI       UI
	Hooks    Hooks
	Model    string
	System   string
}

// Agent holds conversation state across turns.
type Agent struct {
	cfg      Config
	messages []anthropic.Message
}

// New creates an agent. A nil Hooks is replaced with a no-op.
func New(cfg Config) *Agent {
	if cfg.Hooks == nil {
		cfg.Hooks = noopHooks{}
	}
	return &Agent{cfg: cfg}
}

// Snapshot returns the conversation history (for persistence).
func (a *Agent) Snapshot() []anthropic.Message { return a.messages }

// Restore replaces the conversation history (for resume).
func (a *Agent) Restore(m []anthropic.Message) { a.messages = m }

// Reset clears the conversation history, starting a fresh session.
func (a *Agent) Reset() { a.messages = nil }

// Run processes one user input, looping through any tool calls until the model
// produces a final answer.
func (a *Agent) Run(ctx context.Context, input string) error {
	a.messages = append(a.messages, anthropic.UserText(input))

	for i := 0; i < maxIterations; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := a.cfg.Client.Stream(ctx, anthropic.Request{
			Model:     a.cfg.Model,
			MaxTokens: maxTokens,
			System:    a.cfg.System,
			Messages:  a.messages,
			Tools:     a.cfg.Tools.Defs(),
		}, a.cfg.UI.Text)
		if err != nil {
			return err
		}
		a.messages = append(a.messages, anthropic.Message{Role: "assistant", Content: result.Content})

		if result.StopReason != "tool_use" {
			return nil
		}
		a.messages = append(a.messages, anthropic.Message{
			Role:    "user",
			Content: a.runTools(ctx, result.ToolUses()),
		})
	}
	return fmt.Errorf("exceeded max tool iterations (%d)", maxIterations)
}

// runTools executes each requested tool and returns the tool_result blocks.
func (a *Agent) runTools(ctx context.Context, uses []anthropic.Block) []anthropic.Block {
	results := make([]anthropic.Block, 0, len(uses))
	for _, use := range uses {
		input := string(use.Input)
		a.cfg.UI.ToolCall(use.Name, input)

		if block, reason := a.cfg.Hooks.PreTool(use.Name, input); block {
			results = append(results, toolResult(use.ID, reason, errBlocked))
			continue
		}
		if !a.cfg.Tools.ReadOnly(use.Name) && !a.cfg.Approver.Allow(use.Name, input) {
			results = append(results, toolResult(use.ID, "", errDenied))
			continue
		}
		out, err := a.cfg.Tools.Run(ctx, use.Name, use.Input)
		a.cfg.Hooks.PostTool(use.Name, out)
		results = append(results, toolResult(use.ID, out, err))
	}
	return results
}

func toolResult(id, out string, err error) anthropic.Block {
	b := anthropic.Block{Type: "tool_result", ToolUseID: id}
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

// noopHooks is the default when no hooks are configured.
type noopHooks struct{}

func (noopHooks) PreTool(string, string) (bool, string) { return false, "" }
func (noopHooks) PostTool(string, string)               {}
