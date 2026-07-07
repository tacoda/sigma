// Package rules discovers CLAUDE.md instruction files and assembles them into a
// system-prompt fragment.
//
// Hierarchy (least to most specific; more specific wins on conflict):
//
//	~/.claude/CLAUDE.md          user global
//	<ancestors>/CLAUDE.md        each directory from home (or fs root) down to cwd
//	./CLAUDE.md                  the working directory
//
// Within any file, a line that is just `@path` inlines that file's contents
// (relative to the importing file, or ~/ for home). Imports are expanded
// recursively, guarding against cycles and runaway depth.
package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxBytes       = 32 * 1024
	maxImportDepth = 5
)

// Source contributes CLAUDE.md instructions to the system prompt
// (satisfies prompt.Source).
type Source struct{}

func (Source) Contribute() (string, error) { return Load(), nil }

// Load returns the concatenated hierarchy with imports expanded, or "" if none.
func Load() string {
	seen := map[string]bool{}
	var parts []string
	for _, p := range paths() {
		if content := read(p, seen, 0); content != "" {
			parts = append(parts, fmt.Sprintf("# Instructions from %s\n\n%s", p, content))
		}
	}
	return strings.Join(parts, "\n\n")
}

// paths lists the CLAUDE.md files to load, least specific first.
func paths() []string {
	var ps []string
	home, _ := os.UserHomeDir()
	if home != "" {
		ps = append(ps, filepath.Join(home, ".claude", "CLAUDE.md"))
	}
	if wd, err := os.Getwd(); err == nil {
		ps = append(ps, hierarchy(wd, home)...)
	}
	return ps
}

// hierarchy returns CLAUDE.md paths from the outermost ancestor down to wd,
// stopping at home (inclusive) or the filesystem root.
func hierarchy(wd, home string) []string {
	var dirs []string
	for d := wd; ; {
		dirs = append(dirs, d)
		parent := filepath.Dir(d)
		if parent == d || d == home {
			break
		}
		d = parent
	}
	ps := make([]string, 0, len(dirs))
	for i := len(dirs) - 1; i >= 0; i-- {
		ps = append(ps, filepath.Join(dirs[i], "CLAUDE.md"))
	}
	return ps
}

// read loads a file (capped) and expands its @imports. seen guards against
// re-including a file (cycles or duplicates across the hierarchy).
func read(path string, seen map[string]bool, depth int) string {
	abs, err := filepath.Abs(path)
	if err != nil || seen[abs] || depth > maxImportDepth {
		return ""
	}
	seen[abs] = true
	data, err := os.ReadFile(abs)
	if err != nil || len(data) == 0 {
		return ""
	}
	if len(data) > maxBytes {
		data = append(data[:maxBytes], []byte("\n...[truncated]")...)
	}
	return expandImports(string(data), filepath.Dir(abs), seen, depth)
}

// expandImports replaces bare `@path` lines with the imported file's content.
func expandImports(content, baseDir string, seen map[string]bool, depth int) string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		imp, ok := importPath(line)
		if !ok {
			out = append(out, line)
			continue
		}
		if inc := read(resolveImport(imp, baseDir), seen, depth+1); inc != "" {
			out = append(out, inc)
		}
		// An unresolved or already-seen import contributes nothing.
	}
	return strings.Join(out, "\n")
}

// importPath returns the target of a line that is a single `@path` token.
func importPath(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if len(t) > 1 && strings.HasPrefix(t, "@") && !strings.ContainsAny(t, " \t") {
		return t[1:], true
	}
	return "", false
}

func resolveImport(imp, baseDir string) string {
	switch {
	case strings.HasPrefix(imp, "~/"):
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, imp[2:])
		}
		return imp
	case filepath.IsAbs(imp):
		return imp
	default:
		return filepath.Join(baseDir, imp)
	}
}
