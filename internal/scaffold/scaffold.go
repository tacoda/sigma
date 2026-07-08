// Package scaffold writes a starter .sigma/ layout so a new project has working
// config and examples to edit. It never overwrites existing files.
package scaffold

import (
	"os"
	"path/filepath"
	"sort"
)

// files maps a path (relative to the target root) to its starter content.
var files = map[string]string{
	".sigma/settings.json":             settingsJSON,
	".sigma/agents/reviewer.md":        reviewerAgent,
	".sigma/workflows/review-fix.yaml": reviewWorkflow,
	".sigma/hooks.yaml":                hooksYAML,
}

// Init writes the starter files under root, skipping any that already exist. It
// returns the relative paths it created.
func Init(root string) ([]string, error) {
	paths := make([]string, 0, len(files))
	for rel := range files {
		paths = append(paths, rel)
	}
	sort.Strings(paths)

	var created []string
	for _, rel := range paths {
		full := filepath.Join(root, rel)
		if _, err := os.Stat(full); err == nil {
			continue // exists; never overwrite
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return created, err
		}
		if err := os.WriteFile(full, []byte(files[rel]), 0o644); err != nil {
			return created, err
		}
		created = append(created, rel)
	}
	return created, nil
}

const settingsJSON = `{
  "permissionMode": "default",
  "plugins": ["telemetry"]
}
`

const reviewerAgent = `---
name: reviewer
description: Reviews a diff or file for bugs, read-only
tools: read_file, grep, glob
---
You are a meticulous code reviewer. Find correctness bugs, missing error
handling, and security issues. You have read-only tools; do not modify files.
Return a concise, prioritized list of findings with file:line references.
`

const reviewWorkflow = `name: review-fix
steps:
  - name: review
    type: reviewer
    prompt: "Review {input} for correctness bugs. List findings with file:line."
  - name: fix
    prompt: |
      Apply fixes for the findings below, then run the tests.
      Findings:
      {review}
`

const hooksYAML = `hooks:
  - on: Stop
    notify: "turn complete"

# Examples (uncomment and adapt):
#  - on: PreToolUse
#    match: { tool: bash }
#    run: ./scripts/guard.sh          # non-zero exit blocks the command
#  - on: PostToolUse
#    match: { tool: "write_file|edit_file" }
#    run: gofmt -w .
`
