package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxGrepMatches = 100

// Grep searches file contents with a regular expression. Root, if set, confines
// the search under it and returns paths relative to it.
type Grep struct{ Root string }

func (Grep) Name() string { return "grep" }

func (Grep) ReadOnly() bool { return true }

func (Grep) Description() string {
	return "Search file contents recursively with a regular expression. Returns path:line:text matches (capped). Skips hidden dirs, .git, and node_modules."
}

func (Grep) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Regular expression to search for"},
			"path": {"type": "string", "description": "Directory to search (default current directory)"}
		},
		"required": ["pattern"]
	}`)
}

func (g Grep) Run(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}
	sub := args.Path
	if sub == "" {
		sub = "."
	}
	dir, err := rooted(g.Root, sub)
	if err != nil {
		return "", err
	}

	matches, err := search(dir, re, g.Root)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "no matches", nil
	}
	return strings.Join(matches, "\n"), nil
}

// search walks dir, collecting matches up to maxGrepMatches, with paths shown
// relative to root.
func search(dir string, re *regexp.Regexp, root string) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil // skip unreadable entries
		case d.IsDir():
			return skipUnwanted(d.Name())
		}
		matches = append(matches, grepFile(path, re, maxGrepMatches-len(matches), root)...)
		if len(matches) >= maxGrepMatches {
			return filepath.SkipAll
		}
		return nil
	})
	return matches, err
}

func skipUnwanted(dir string) error {
	if skipDir(dir) {
		return filepath.SkipDir
	}
	return nil
}

func skipDir(name string) bool {
	return name == ".git" || name == "node_modules" || (strings.HasPrefix(name, ".") && name != ".")
}

func grepFile(path string, re *regexp.Regexp, limit int, root string) []string {
	if limit <= 0 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	display := unrooted(root, path)
	var out []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if re.MatchString(scanner.Text()) {
			out = append(out, fmt.Sprintf("%s:%d:%s", display, line, scanner.Text()))
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
