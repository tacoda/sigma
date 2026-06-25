package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// maxReadBytes caps a single read so a huge file can't blow up context.
// ponytail: flat byte cap; add offset/limit windowing in Phase 3 if needed.
const maxReadBytes = 256 * 1024

// ReadFile reads a file's contents.
type ReadFile struct{}

func (ReadFile) Name() string { return "read_file" }

func (ReadFile) ReadOnly() bool { return true }

func (ReadFile) Description() string {
	return "Read the contents of a file at the given path. Returns the file text, truncated if very large."
}

func (ReadFile) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Path to the file to read"}
		},
		"required": ["path"]
	}`)
}

func (ReadFile) Run(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	data, err := os.ReadFile(args.Path)
	if err != nil {
		return "", err
	}
	if len(data) > maxReadBytes {
		return string(data[:maxReadBytes]) + "\n...[truncated]", nil
	}
	return string(data), nil
}
