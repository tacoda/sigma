package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tacoda/sigma/internal/exec"
)

const (
	bashDefaultTimeout = 120 * time.Second
	maxOutputBytes     = 64 * 1024
)

// Bash runs a shell command through an Executor. A nil Exec uses exec.Local
// (host, no isolation); a sandbox or worktree adapter can be substituted.
type Bash struct{ Exec exec.Executor }

func (Bash) Name() string { return "bash" }

func (Bash) ReadOnly() bool { return false }

func (Bash) Description() string {
	return "Run a shell command with `bash -c` and return its combined stdout and stderr. Output is truncated if large."
}

func (Bash) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "The shell command to run"},
			"timeout_seconds": {"type": "integer", "description": "Optional timeout in seconds (default 120)"}
		},
		"required": ["command"]
	}`)
}

func (b Bash) Run(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Command        string `json:"command"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.Command == "" {
		return "", fmt.Errorf("command is required")
	}
	timeout := bashDefaultTimeout
	if args.TimeoutSeconds > 0 {
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
	}

	ex := b.Exec
	if ex == nil {
		ex = exec.Local{}
	}
	out, err := ex.Run(ctx, exec.Spec{Command: args.Command, Timeout: timeout})
	text := truncate(out)
	if err != nil {
		return text, fmt.Errorf("command failed: %w", err)
	}
	return text, nil
}

func truncate(s string) string {
	if len(s) > maxOutputBytes {
		return s[:maxOutputBytes] + "\n...[truncated]"
	}
	return s
}
