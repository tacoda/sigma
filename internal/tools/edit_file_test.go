package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func runEdit(t *testing.T, path, old, new string, all bool) (string, error) {
	t.Helper()
	in, _ := json.Marshal(map[string]any{
		"path": path, "old_string": old, "new_string": new, "replace_all": all,
	})
	return EditFile{}.Run(context.Background(), in)
}

func TestEditUnique(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(p, []byte("alpha beta gamma"), 0o644)
	if _, err := runEdit(t, p, "beta", "BETA", false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "alpha BETA gamma" {
		t.Errorf("got %q", got)
	}
}

func TestEditNotFound(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(p, []byte("abc"), 0o644)
	if _, err := runEdit(t, p, "xyz", "zzz", false); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestEditNotUnique(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(p, []byte("x x x"), 0o644)
	if _, err := runEdit(t, p, "x", "y", false); err == nil {
		t.Fatal("expected non-unique error")
	}
	if _, err := runEdit(t, p, "x", "y", true); err != nil {
		t.Fatalf("replace_all should succeed: %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "y y y" {
		t.Errorf("got %q", got)
	}
}
