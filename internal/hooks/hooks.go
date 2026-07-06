// Package hooks dispatches lifecycle events to hooks. Two adapters implement
// the Bus port: Callbacks (in-process Go functions, for tapping into events
// programmatically) and Shell (user-configured commands from settings.json).
// Multi fans an event out to several buses.
//
// Shell config is keyed by event name:
//
//	"hooks": { "PreToolUse": ["./guard.sh"], "Stop": ["./done.sh"] }
//
// Each command receives a JSON payload on stdin plus SIGMA_EVENT and SIGMA_TOOL
// in the environment. A command that exits non-zero yields a blocking outcome;
// the agent honors that for PreToolUse (the tool is skipped, output = reason).
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

// Kind identifies a lifecycle event.
type Kind string

const (
	SessionStart Kind = "SessionStart"
	SessionEnd   Kind = "SessionEnd"
	UserPrompt   Kind = "UserPromptSubmit"
	PreLLM       Kind = "PreLLMRequest"
	PostLLM      Kind = "PostLLMResponse"
	PreTool      Kind = "PreToolUse"
	PostTool     Kind = "PostToolUse"
	ToolError    Kind = "ToolError"
	Stop         Kind = "Stop"
	Notification Kind = "Notification"
)

// AllKinds lists every event kind, for handlers that tap all events.
var AllKinds = []Kind{
	SessionStart, SessionEnd, UserPrompt, PreLLM, PostLLM,
	PreTool, PostTool, ToolError, Stop, Notification,
}

// Event is one lifecycle event. Fields are populated by kind.
type Event struct {
	Kind      Kind
	Tool      string // PreTool/PostTool/ToolError
	Input     string // PreTool
	Output    string // PostTool/ToolError
	Prompt    string // UserPrompt
	Message   string // Notification/Stop
	InTokens  int    // PostLLM
	OutTokens int    // PostLLM
}

// Outcome is a hook's response. Block stops a pending action (honored for
// PreToolUse); Reason explains why.
type Outcome struct {
	Block  bool
	Reason string
}

// Bus dispatches an event to hooks and aggregates the result. The first
// blocking outcome wins.
type Bus interface {
	Emit(ctx context.Context, ev Event) Outcome
}

// Nop is a bus that does nothing.
type Nop struct{}

func (Nop) Emit(context.Context, Event) Outcome { return Outcome{} }

// Func is an in-process hook. Returning Outcome{Block:true} stops a Pre* action.
type Func func(ctx context.Context, ev Event) Outcome

// Callbacks is an in-process bus: register Go functions per event kind.
type Callbacks struct {
	handlers map[Kind][]Func
}

func NewCallbacks() *Callbacks { return &Callbacks{handlers: map[Kind][]Func{}} }

// On registers fn for one event kind. It returns the receiver for chaining.
func (c *Callbacks) On(kind Kind, fn Func) *Callbacks {
	c.handlers[kind] = append(c.handlers[kind], fn)
	return c
}

// OnAll registers fn for every event kind.
func (c *Callbacks) OnAll(fn Func) *Callbacks {
	for _, k := range AllKinds {
		c.On(k, fn)
	}
	return c
}

// Len reports the number of registered handlers.
func (c *Callbacks) Len() int {
	n := 0
	for _, hs := range c.handlers {
		n += len(hs)
	}
	return n
}

func (c *Callbacks) Emit(ctx context.Context, ev Event) Outcome {
	for _, fn := range c.handlers[ev.Kind] {
		if o := fn(ctx, ev); o.Block {
			return o
		}
	}
	return Outcome{}
}

// Multi fans an event out to several buses; the first blocking outcome wins.
type Multi []Bus

func (m Multi) Emit(ctx context.Context, ev Event) Outcome {
	for _, b := range m {
		if o := b.Emit(ctx, ev); o.Block {
			return o
		}
	}
	return Outcome{}
}

// Shell runs user-configured commands for an event.
type Shell struct {
	events map[string][]string
}

// NewShell builds a shell bus from the configured event→commands map.
func NewShell(events map[string][]string) *Shell { return &Shell{events: events} }

func (s *Shell) Emit(ctx context.Context, ev Event) Outcome {
	for _, cmd := range s.events[string(ev.Kind)] {
		out, err := run(ctx, cmd, ev)
		if err != nil {
			reason := strings.TrimSpace(out)
			if reason == "" {
				reason = err.Error()
			}
			return Outcome{Block: true, Reason: reason}
		}
	}
	return Outcome{}
}

type payload struct {
	Event   string `json:"event"`
	Tool    string `json:"tool,omitempty"`
	Input   string `json:"input,omitempty"`
	Output  string `json:"output,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	Message string `json:"message,omitempty"`
}

func run(ctx context.Context, command string, ev Event) (string, error) {
	data, _ := json.Marshal(payload{
		Event: string(ev.Kind), Tool: ev.Tool, Input: ev.Input,
		Output: ev.Output, Prompt: ev.Prompt, Message: ev.Message,
	})
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Env = append(os.Environ(), "SIGMA_EVENT="+string(ev.Kind), "SIGMA_TOOL="+ev.Tool)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
