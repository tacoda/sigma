package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EditFile replaces an exact string in a file. old_string must be unique unless
// replace_all is set. Root, if set, confines edits under it.
type EditFile struct{ Root string }

func (EditFile) Name() string { return "edit_file" }

func (EditFile) ReadOnly() bool { return false }

func (EditFile) Description() string {
	return "Replace an exact occurrence of old_string with new_string in a file. old_string must match uniquely unless replace_all is true."
}

func (EditFile) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "File to edit"},
			"old_string": {"type": "string", "description": "Exact text to replace"},
			"new_string": {"type": "string", "description": "Replacement text"},
			"replace_all": {"type": "boolean", "description": "Replace all occurrences (default false)"}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}

func (e EditFile) Run(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.Path == "" || args.OldString == "" {
		return "", fmt.Errorf("path and old_string are required")
	}
	path, err := rooted(e.Root, args.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)

	n, err := matchCount(content, args.OldString, args.ReplaceAll, args.Path)
	if err != nil {
		return "", err
	}

	updated := strings.ReplaceAll(content, args.OldString, args.NewString)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("replaced %d occurrence(s) in %s", n, args.Path), nil
}

// matchCount validates that old_string occurs the expected number of times.
func matchCount(content, old string, replaceAll bool, path string) (int, error) {
	n := strings.Count(content, old)
	switch {
	case n == 0:
		return 0, fmt.Errorf("old_string not found in %s", path)
	case n > 1 && !replaceAll:
		return 0, fmt.Errorf("old_string is not unique in %s (%d matches); set replace_all or add context", path, n)
	}
	return n, nil
}
