package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeProjectOverridesModel(t *testing.T) {
	dir := t.TempDir()
	user := filepath.Join(dir, "user.json")
	proj := filepath.Join(dir, "proj.json")
	os.WriteFile(user, []byte(`{"model":"u","allowedTools":["bash"]}`), 0o644)
	os.WriteFile(proj, []byte(`{"model":"p","allowedTools":["grep"]}`), 0o644)

	var s Settings
	merge(&s, user)
	merge(&s, proj)

	if s.Model != "p" {
		t.Errorf("model = %q, want project value p", s.Model)
	}
	if len(s.AllowedTools) != 2 {
		t.Errorf("allowedTools = %v, want union", s.AllowedTools)
	}
}

func TestMergeMissingFileIgnored(t *testing.T) {
	var s Settings
	merge(&s, filepath.Join(t.TempDir(), "nope.json"))
	if s.Model != "" || s.AllowedTools != nil {
		t.Errorf("missing file mutated settings: %+v", s)
	}
}
