// Package agents loads named sub-agent types: a system prompt plus an allowed
// tool subset the `task`/`fanout` tools can select by name.
//
// A type is a markdown file with YAML frontmatter (name, description, tools)
// and a body used as the sub-agent's system prompt:
//
//	---
//	name: reviewer
//	description: Reviews a diff for bugs
//	tools: read_file, grep, glob
//	---
//	You are a meticulous code reviewer. ...
//
// Discovery (project overrides user on name conflict):
//
//	~/.sigma/agents/<name>.md
//	./.sigma/agents/<name>.md
package agents

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Type is a named sub-agent configuration.
type Type struct {
	Name        string
	Description string
	System      string   // sub-agent system prompt (the file body)
	Tools       []string // allowed tool names; empty means all
}

// Set is the loaded types, keyed by name.
type Set map[string]Type

// Load reads agent types from the user then project directories.
func Load() Set {
	s := Set{}
	if home, err := os.UserHomeDir(); err == nil {
		s.loadDir(filepath.Join(home, ".sigma", "agents"))
	}
	s.loadDir(filepath.Join(".sigma", "agents"))
	return s
}

// Names returns the type names, sorted.
func (s Set) Names() []string {
	names := make([]string, 0, len(s))
	for n := range s {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (s Set) loadDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		t := parse(string(data))
		if t.Name == "" {
			t.Name = strings.TrimSuffix(e.Name(), ".md")
		}
		s[t.Name] = t
	}
}

// parse splits YAML frontmatter (name, description, tools) from the body.
func parse(text string) Type {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "---") {
		return Type{System: strings.TrimSpace(text)}
	}
	rest := text[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Type{System: strings.TrimSpace(text)}
	}
	t := parseFrontmatter(rest[:end])
	after := rest[end+len("\n---"):]
	if nl := strings.IndexByte(after, '\n'); nl >= 0 {
		after = after[nl+1:]
	}
	t.System = strings.TrimSpace(after)
	return t
}

func parseFrontmatter(fm string) Type {
	var t Type
	for _, line := range strings.Split(fm, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "name":
			t.Name = val
		case "description":
			t.Description = val
		case "tools":
			for _, name := range strings.Split(val, ",") {
				if n := strings.TrimSpace(name); n != "" {
					t.Tools = append(t.Tools, n)
				}
			}
		}
	}
	return t
}
