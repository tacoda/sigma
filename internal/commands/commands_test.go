package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPlaceholder(t *testing.T) {
	got := Expand("review $ARGUMENTS now", "main.go")
	if got != "review main.go now" {
		t.Errorf("got %q", got)
	}
}

func TestExpandAppendsWhenNoPlaceholder(t *testing.T) {
	if got := Expand("summarize", "file.go"); got != "summarize\n\nfile.go" {
		t.Errorf("got %q", got)
	}
	if got := Expand("summarize", ""); got != "summarize" {
		t.Errorf("empty args should not append: %q", got)
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "review.md"), []byte("  do review  "), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644)

	s := Set{}
	s.loadDir(dir)

	if s["review"] != "do review" {
		t.Errorf("review = %q (should be trimmed)", s["review"])
	}
	if _, ok := s["notes"]; ok {
		t.Error("non-.md file should be ignored")
	}
	if names := s.Names(); len(names) != 1 || names[0] != "review" {
		t.Errorf("names = %v", names)
	}
}
