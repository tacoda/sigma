package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHierarchyOrderAndImports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	proj := filepath.Join(home, "proj")
	sub := filepath.Join(proj, "sub")

	write(t, filepath.Join(proj, "CLAUDE.md"), "PROJECT_RULES\n@shared.md")
	write(t, filepath.Join(proj, "shared.md"), "SHARED_FRAGMENT")
	write(t, filepath.Join(sub, "CLAUDE.md"), "SUBDIR_RULES")
	t.Chdir(sub)

	out := Load()

	for _, want := range []string{"PROJECT_RULES", "SHARED_FRAGMENT", "SUBDIR_RULES"} {
		if !strings.Contains(out, want) {
			t.Errorf("Load missing %q:\n%s", want, out)
		}
	}
	// Import expanded in place of the @shared.md line.
	if strings.Contains(out, "@shared.md") {
		t.Error("import line should be replaced, not kept")
	}
	// More-specific (subdir) rules come after the project rules.
	if strings.Index(out, "PROJECT_RULES") > strings.Index(out, "SUBDIR_RULES") {
		t.Error("hierarchy order wrong: project should precede subdir")
	}
}

func TestImportCycleTerminates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "p")
	write(t, filepath.Join(dir, "CLAUDE.md"), "ROOT\n@a.md")
	write(t, filepath.Join(dir, "a.md"), "A\n@b.md")
	write(t, filepath.Join(dir, "b.md"), "B\n@a.md") // cycle back to a
	t.Chdir(dir)

	out := Load() // must not loop forever
	for _, want := range []string{"ROOT", "A", "B"} {
		if !strings.Contains(out, want) {
			t.Errorf("Load missing %q", want)
		}
	}
	// a.md is included once; the cyclic re-import contributes nothing.
	if strings.Count(out, "\nA\n") > 1 || strings.Count(out, "A\nB") > 1 {
		t.Errorf("cycle should not duplicate content:\n%s", out)
	}
}
