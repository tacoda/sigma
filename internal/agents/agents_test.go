package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadParsesTypes(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join(".sigma", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
name: reviewer
description: Reviews a diff
tools: read_file, grep , glob
---
You are a meticulous reviewer.`
	if err := os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	set := Load()
	rv, ok := set["reviewer"]
	if !ok {
		t.Fatal("reviewer not loaded")
	}
	if rv.Description != "Reviews a diff" {
		t.Errorf("description = %q", rv.Description)
	}
	if len(rv.Tools) != 3 || rv.Tools[0] != "read_file" || rv.Tools[1] != "grep" || rv.Tools[2] != "glob" {
		t.Errorf("tools = %v, want [read_file grep glob] trimmed", rv.Tools)
	}
	if rv.System != "You are a meticulous reviewer." {
		t.Errorf("system = %q", rv.System)
	}
}

func TestLoadNameDefaultsToFilename(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join(".sigma", "agents")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "planner.md"), []byte("Just a body, no frontmatter."), 0o644)

	set := Load()
	if _, ok := set["planner"]; !ok {
		t.Errorf("name should default to filename, got %v", set.Names())
	}
}
