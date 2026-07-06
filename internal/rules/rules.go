// Package rules discovers CLAUDE.md instruction files and assembles them into a
// system-prompt fragment.
//
// Discovery order (user first, project last so project rules win on conflict):
//
//	~/.claude/CLAUDE.md
//	./CLAUDE.md
package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxBytes caps each rules file so a large CLAUDE.md can't dominate context.
const maxBytes = 32 * 1024

// Source contributes CLAUDE.md instructions to the system prompt
// (satisfies prompt.Source).
type Source struct{}

func (Source) Contribute() (string, error) { return Load(), nil }

// Load returns the concatenated rules, or "" if none are found.
func Load() string {
	var parts []string
	for _, p := range paths() {
		data, err := os.ReadFile(p)
		if err != nil || len(data) == 0 {
			continue
		}
		if len(data) > maxBytes {
			data = append(data[:maxBytes], []byte("\n...[truncated]")...)
		}
		parts = append(parts, fmt.Sprintf("# Instructions from %s\n\n%s", p, data))
	}
	return strings.Join(parts, "\n\n")
}

func paths() []string {
	var ps []string
	if home, err := os.UserHomeDir(); err == nil {
		ps = append(ps, filepath.Join(home, ".claude", "CLAUDE.md"))
	}
	ps = append(ps, "CLAUDE.md")
	return ps
}
