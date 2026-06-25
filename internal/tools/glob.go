package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Glob lists files matching a shell pattern.
type Glob struct{}

func (Glob) Name() string { return "glob" }

func (Glob) ReadOnly() bool { return true }

func (Glob) Description() string {
	return "List files matching a glob pattern (e.g. internal/*.go). Does not support ** recursion; use grep or bash find for recursive search."
}

func (Glob) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Glob pattern, e.g. cmd/*/*.go"}
		},
		"required": ["pattern"]
	}`)
}

func (Glob) Run(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	// ponytail: stdlib filepath.Glob, no ** support. Upgrade to doublestar if
	// recursive globbing is needed.
	matches, err := filepath.Glob(args.Pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "no matches", nil
	}
	return strings.Join(matches, "\n"), nil
}
