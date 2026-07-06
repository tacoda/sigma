// Package tools defines the tool interface and registry the agent dispatches to.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tacoda/sigma/internal/exec"
	"github.com/tacoda/sigma/internal/message"
)

// Tool is one capability the agent can invoke.
type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	// ReadOnly reports whether the tool mutates state. Read-only tools bypass
	// the permission gate.
	ReadOnly() bool
	Run(ctx context.Context, input json.RawMessage) (string, error)
}

// FS returns the standard tool set. A non-empty root confines the file tools
// (read/write/edit/glob/grep) under it — for worktree isolation. ex is the
// executor bash runs through; nil defaults to an unsandboxed exec.Local rooted
// at root.
func FS(root string, ex exec.Executor) []Tool {
	if ex == nil {
		ex = exec.Local{Dir: root}
	}
	return []Tool{
		ReadFile{Root: root}, WriteFile{Root: root}, EditFile{Root: root},
		Bash{Exec: ex}, Glob{Root: root}, Grep{Root: root}, Worktree{},
	}
}

// Registry holds the available tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry builds a registry from the given tools.
func NewRegistry(ts ...Tool) *Registry {
	m := make(map[string]Tool, len(ts))
	for _, t := range ts {
		m[t.Name()] = t
	}
	return &Registry{tools: m}
}

// Defs returns the API tool definitions.
func (r *Registry) Defs() []message.Tool {
	defs := make([]message.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, message.Tool{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.Schema(),
		})
	}
	return defs
}

// Run dispatches to a tool by name.
func (r *Registry) Run(ctx context.Context, name string, input json.RawMessage) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Run(ctx, input)
}

// ReadOnly reports whether the named tool is read-only (unknown tools are
// treated as mutating, i.e. not read-only).
func (r *Registry) ReadOnly(name string) bool {
	t, ok := r.tools[name]
	return ok && t.ReadOnly()
}
