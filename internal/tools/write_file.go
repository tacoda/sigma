package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile creates or overwrites a file, making parent directories. Root, if
// set, confines writes under it.
type WriteFile struct{ Root string }

func (WriteFile) Name() string { return "write_file" }

func (WriteFile) ReadOnly() bool { return false }

func (WriteFile) Description() string {
	return "Write content to a file, creating parent directories and overwriting any existing file."
}

func (WriteFile) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Path to write"},
			"content": {"type": "string", "description": "File content"}
		},
		"required": ["path", "content"]
	}`)
}

func (w WriteFile) Run(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	path, err := rooted(w.Root, args.Path)
	if err != nil {
		return "", err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), nil
}
