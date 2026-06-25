package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Tool loads a skill's full instructions on demand. It structurally satisfies
// the agent's tool interface, so no import of the tools package is needed.
type Tool struct {
	set Set
}

// NewTool returns the skill-loading tool for the given skill set.
func NewTool(s Set) Tool { return Tool{set: s} }

func (Tool) Name() string { return "skill" }

func (Tool) ReadOnly() bool { return true }

func (Tool) Description() string {
	return "Load the full instructions for a skill listed in the system prompt. Call this before acting on a skill."
}

func (Tool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "The skill name to load"}
		},
		"required": ["name"]
	}`)
}

func (t Tool) Run(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	sk, ok := t.set[args.Name]
	if !ok {
		return "", fmt.Errorf("unknown skill %q; available: %s", args.Name, strings.Join(t.set.names(), ", "))
	}
	return sk.Body, nil
}
