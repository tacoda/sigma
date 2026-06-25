// Package skills implements progressive disclosure of agent skills.
//
// A skill is a directory containing SKILL.md with YAML frontmatter (name,
// description) and a markdown body of instructions. Only the name and
// description are placed in the system prompt; the model loads the full body on
// demand by calling the `skill` tool. This keeps idle skills out of context.
//
// Discovery (project overrides user on name conflict):
//
//	~/.sigma/skills/<name>/SKILL.md
//	./.sigma/skills/<name>/SKILL.md
package skills

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill is a single loaded skill.
type Skill struct {
	Name        string
	Description string
	Body        string
}

// Set is the loaded skills, keyed by name.
type Set map[string]Skill

// Load reads skills from the user then project directories.
func Load() Set {
	s := Set{}
	if home, err := os.UserHomeDir(); err == nil {
		s.loadDir(filepath.Join(home, ".sigma", "skills"))
	}
	s.loadDir(filepath.Join(".sigma", "skills"))
	return s
}

func (s Set) loadDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name(), "SKILL.md"))
		if err != nil {
			continue
		}
		sk := parse(string(data))
		if sk.Name == "" {
			sk.Name = e.Name()
		}
		s[sk.Name] = sk
	}
}

// Index renders the skills list for the system prompt, or "" if none.
func (s Set) Index() string {
	if len(s) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Available skills\n\n")
	b.WriteString("Each entry is a capability with detailed instructions you can load. When a task matches a skill, call the `skill` tool with its name to load the full instructions before acting.\n\n")
	for _, name := range s.names() {
		b.WriteString("- " + name + ": " + s[name].Description + "\n")
	}
	return b.String()
}

func (s Set) names() []string {
	names := make([]string, 0, len(s))
	for n := range s {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// parse splits YAML frontmatter (name, description) from the markdown body.
func parse(text string) Skill {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "---") {
		return Skill{Body: strings.TrimSpace(text)}
	}
	rest := text[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Skill{Body: strings.TrimSpace(text)}
	}
	name, desc := parseFrontmatter(rest[:end])
	// Skip the closing fence line ("---\n...") to the first newline after it.
	after := rest[end+len("\n---"):]
	if nl := strings.IndexByte(after, '\n'); nl >= 0 {
		after = after[nl+1:]
	}
	return Skill{Name: name, Description: desc, Body: strings.TrimSpace(after)}
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
