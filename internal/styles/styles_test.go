package styles

import (
	"os"
	"path/filepath"
	"testing"
)

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
