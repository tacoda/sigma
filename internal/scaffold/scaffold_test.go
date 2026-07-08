package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesThenSkips(t *testing.T) {
	root := t.TempDir()

	created, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != len(files) {
		t.Errorf("created %d files, want %d", len(created), len(files))
	}
	// Every advertised file exists and is non-empty.
	for rel := range files {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil || len(data) == 0 {
			t.Errorf("%s: not written (%v)", rel, err)
		}
	}

	// A second run is a no-op (non-destructive).
	again, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("second Init created %v, want nothing", again)
	}
}

func TestInitDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	settings := filepath.Join(root, ".sigma", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("KEEP_ME"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(settings)
	if string(data) != "KEEP_ME" {
		t.Error("Init overwrote an existing file")
	}
}
