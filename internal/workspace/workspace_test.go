package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v: %s", args, err, out)
	}
}

// setupRepo creates a temp git repo with one commit and returns its path.
func setupRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q")
	run(t, dir, "git", "config", "user.email", "t@example.com")
	run(t, dir, "git", "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-q", "-m", "init")
	return dir
}

func TestCreateMerge(t *testing.T) {
	dir := setupRepo(t)
	g := Git{Root: dir}
	ctx := context.Background()

	h, err := g.Create(ctx, "feature-x")
	if err != nil {
		t.Fatal(err)
	}
	if h.Branch != "sigma/feature-x" {
		t.Errorf("branch = %q", h.Branch)
	}
	if _, err := os.Stat(h.Dir); err != nil {
		t.Fatalf("worktree dir missing: %v", err)
	}

	// Commit a file inside the worktree, then merge it back.
	if err := os.WriteFile(filepath.Join(h.Dir, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, h.Dir, "git", "add", ".")
	run(t, h.Dir, "git", "commit", "-q", "-m", "add new")

	if err := g.Merge(ctx, "feature-x"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err != nil {
		t.Errorf("merged file missing from main tree: %v", err)
	}
	if _, err := os.Stat(h.Dir); !os.IsNotExist(err) {
		t.Errorf("worktree should be removed after merge (err=%v)", err)
	}
}

func TestDiscard(t *testing.T) {
	dir := setupRepo(t)
	g := Git{Root: dir}
	ctx := context.Background()

	h, err := g.Create(ctx, "tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Discard(ctx, "tmp"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(h.Dir); !os.IsNotExist(err) {
		t.Errorf("worktree should be gone after discard (err=%v)", err)
	}
}

func TestList(t *testing.T) {
	dir := setupRepo(t)
	g := Git{Root: dir}
	ctx := context.Background()

	if _, err := g.Create(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	hs, err := g.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 1 || hs[0].ID != "one" {
		t.Errorf("List = %+v, want one sigma worktree", hs)
	}
}
