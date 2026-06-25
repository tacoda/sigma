// Package hooks runs user-configured shell commands around tool execution.
//
// Configured in settings.json under "hooks", keyed by event:
//
//	"hooks": { "PreToolUse": ["./guard.sh"], "PostToolUse": ["./log.sh"] }
//
// Each command receives a JSON payload on stdin and SIGMA_TOOL in the
// environment. A PreToolUse command that exits non-zero blocks the tool; its
// output becomes the block reason.
package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

const (
	preToolUse  = "PreToolUse"
	postToolUse = "PostToolUse"
)

// Runner executes hooks for events.
type Runner struct {
	events map[string][]string
}

// New builds a runner from the configured event→commands map.
func New(events map[string][]string) *Runner {
	return &Runner{events: events}
}

// PreTool runs PreToolUse hooks. It blocks (returns true) if any command exits
// non-zero, with that command's output as the reason.
func (r *Runner) PreTool(name, input string) (bool, string) {
	for _, cmd := range r.events[preToolUse] {
		out, err := run(cmd, name, payload{Tool: name, Input: input})
		if err != nil {
			reason := strings.TrimSpace(out)
			if reason == "" {
				reason = err.Error()
			}
			return true, reason
		}
	}
	return false, ""
}

// PostTool runs PostToolUse hooks; their exit status is ignored.
func (r *Runner) PostTool(name, output string) {
	for _, cmd := range r.events[postToolUse] {
		_, _ = run(cmd, name, payload{Tool: name, Output: output})
	}
}

type payload struct {
	Tool   string `json:"tool"`
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
}

func run(command, tool string, p payload) (string, error) {
	data, _ := json.Marshal(p)
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Env = append(os.Environ(), "SIGMA_TOOL="+tool)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
