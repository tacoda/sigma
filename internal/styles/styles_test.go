package styles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegisteredStyleLoadsAndFileWins(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	Register(Style{Name: "built", Description: "builtin", Body: "BUILTIN_BODY"})

	if got := Load()["built"]; got.Body != "BUILTIN_BODY" {
		t.Errorf("registered style not loaded: %+v", got)
	}

	// A file style of the same name overrides the built-in.
	dir := filepath.Join(".sigma", "styles")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "built.md"), []byte("FILE_BODY"), 0o644)
	if got := Load()["built"]; got.Body != "FILE_BODY" {
		t.Errorf("file style should override built-in, got %q", got.Body)
	}
}

func TestLoadAndContribute(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join(".sigma", "styles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
name: concise
description: Terse
---
Answer in the fewest correct words. No preamble.`
	if err := os.WriteFile(filepath.Join(dir, "concise.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	st, ok := Load()["concise"]
	if !ok {
		t.Fatal("concise style not loaded")
	}
	if st.Description != "Terse" {
		t.Errorf("description = %q", st.Description)
	}
	got, err := st.Contribute()
	if err != nil {
		t.Fatal(err)
	}
	if got != "Answer in the fewest correct words. No preamble." {
		t.Errorf("Contribute = %q", got)
	}
}
