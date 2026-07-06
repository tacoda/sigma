package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/tacoda/sigma/internal/workspace"
)

// Worktree manages isolated git worktrees for risky or parallel work.
type Worktree struct{ WS workspace.Workspace }

func (Worktree) Name() string { return "worktree" }

func (Worktree) ReadOnly() bool { return false }

func (Worktree) Description() string {
	return "Manage isolated git worktrees. action=create makes a worktree on a new branch and returns its directory; run commands there with the bash tool's dir argument (or absolute paths). action=merge merges the worktree branch back and removes it; action=discard throws it away; action=list shows active worktrees."
}

func (Worktree) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["create", "merge", "discard", "list"]},
			"name": {"type": "string", "description": "Worktree name (required for create/merge/discard)"}
		},
		"required": ["action"]
	}`)
}

func (w Worktree) Run(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	ws := w.WS
	if ws == nil {
		ws = workspace.Git{}
	}

	if args.Action == "list" {
		hs, err := ws.List(ctx)
		if err != nil {
			return "", err
		}
		if len(hs) == 0 {
			return "no active worktrees", nil
		}
		var b strings.Builder
		for _, h := range hs {
			fmt.Fprintf(&b, "%s\t%s\t%s\n", h.ID, h.Branch, h.Dir)
		}
		return b.String(), nil
	}

	id := sanitizeID(args.Name)
	if id == "" {
		return "", fmt.Errorf("name is required for %s", args.Action)
	}
	switch args.Action {
	case "create":
		h, err := ws.Create(ctx, id)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("created worktree %q on branch %s at %s\nwork there via bash dir=%s or absolute paths, then merge or discard", h.ID, h.Branch, h.Dir, h.Dir), nil
	case "merge":
		if err := ws.Merge(ctx, id); err != nil {
			return "", err
		}
		return fmt.Sprintf("merged worktree %q and removed it", id), nil
	case "discard":
		if err := ws.Discard(ctx, id); err != nil {
			return "", err
		}
		return fmt.Sprintf("discarded worktree %q", id), nil
	default:
		return "", fmt.Errorf("unknown action %q", args.Action)
	}
}

var idUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// sanitizeID makes a name safe for a branch and directory.
func sanitizeID(name string) string {
	return strings.Trim(idUnsafe.ReplaceAllString(strings.TrimSpace(name), "-"), "-")
}
