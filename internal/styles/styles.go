// Package styles loads output styles: named system-prompt fragments that shape
// the agent's voice and behavior (e.g. concise, explanatory, teaching).
//
// A style is a markdown file with YAML frontmatter (name, description) and a
// body appended to the system prompt:
//
//	---
//	name: concise
//	description: Terse, no preamble
//	---
//	Answer in the fewest words that are correct. No preamble or filler.
//
// Discovery (project overrides user on name conflict):
//
//	~/.sigma/styles/<name>.md
//	./.sigma/styles/<name>.md
package styles

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Style is a named output style.
type Style struct {
	Name        string
	Description string
	Body        string
}

// Contribute returns the style body for the system prompt (satisfies
// prompt.Source).
func (s Style) Contribute() (string, error) { return s.Body, nil }

// Set is the loaded styles, keyed by name.
type Set map[string]Style

// registered holds built-in styles contributed in-process (e.g. by a plugin).
var registered = map[string]Style{}

// Register adds a built-in style, available to Load unless a file style of the
// same name overrides it.
func Register(s Style) { registered[s.Name] = s }

// Load returns registered built-in styles overlaid with user then project file
// styles (files win on name conflict).
func Load() Set {
	s := Set{}
	for name, st := range registered {
		s[name] = st
	}
	if home, err := os.UserHomeDir(); err == nil {
		s.loadDir(filepath.Join(home, ".sigma", "styles"))
	}
	s.loadDir(filepath.Join(".sigma", "styles"))
	return s
}

// Names returns the style names, sorted.
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
		st := parse(string(data))
		if st.Name == "" {
			st.Name = strings.TrimSuffix(e.Name(), ".md")
		}
		s[st.Name] = st
	}
}

func parse(text string) Style {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "---") {
		return Style{Body: strings.TrimSpace(text)}
	}
	rest := text[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Style{Body: strings.TrimSpace(text)}
	}
	name, desc := parseFrontmatter(rest[:end])
	after := rest[end+len("\n---"):]
	if nl := strings.IndexByte(after, '\n'); nl >= 0 {
		after = after[nl+1:]
	}
	return Style{Name: name, Description: desc, Body: strings.TrimSpace(after)}
}

func parseFrontmatter(fm string) (name, desc string) {
	for _, line := range strings.Split(fm, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "name":
			name = val
		case "description":
			desc = val
		}
	}
	return name, desc
}
