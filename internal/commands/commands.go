// Package commands loads slash-command templates from markdown files.
//
// A command named "foo" lives in foo.md; its body becomes a prompt, with
// $ARGUMENTS replaced by whatever the user typed after the command.
//
// Discovery (project overrides user on name conflict):
//
//	~/.sigma/commands/*.md
//	./.sigma/commands/*.md
package commands

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Set is the loaded command templates, keyed by name.
type Set map[string]string

// Load reads command files from the user then project directories.
func Load() Set {
	s := Set{}
	if home, err := os.UserHomeDir(); err == nil {
		s.loadDir(filepath.Join(home, ".sigma", "commands"))
	}
	s.loadDir(filepath.Join(".sigma", "commands"))
	return s
}

func (s Set) loadDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if !ok || e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		s[name] = strings.TrimSpace(string(data))
	}
}

// Names returns the command names, sorted for stable display.
func (s Set) Names() []string {
	names := make([]string, 0, len(s))
	for n := range s {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Expand substitutes $ARGUMENTS in a command body. If the body has no
// $ARGUMENTS placeholder, args are appended on a new line.
func Expand(body, args string) string {
	if strings.Contains(body, "$ARGUMENTS") {
		return strings.ReplaceAll(body, "$ARGUMENTS", args)
	}
	if args == "" {
		return body
	}
	return body + "\n\n" + args
}
