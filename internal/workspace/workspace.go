// Package workspace manages isolated git worktrees so risky or parallel work
// happens off the main tree, then merges back or is discarded.
//
// Everything is deterministic from an id: the branch is sigma/<id> and the
// worktree lives at <root>/.sigma/worktrees/<id>.
package workspace

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Handle describes one worktree.
type Handle struct {
	ID     string
	Dir    string
	Branch string
}

// Workspace creates and reconciles isolated worktrees.
type Workspace interface {
	Create(ctx context.Context, id string) (Handle, error)
	Merge(ctx context.Context, id string) error
	Discard(ctx context.Context, id string) error
	List(ctx context.Context) ([]Handle, error)
}

// Git is a git-worktree-backed Workspace rooted at a repository.
type Git struct {
	Root string // repository root; empty means the process cwd
}

func (g Git) handle(id string) Handle {
	return Handle{
		ID:     id,
		Dir:    filepath.Join(g.Root, ".sigma", "worktrees", id),
		Branch: "sigma/" + id,
	}
}

// Create adds a worktree on a new branch cut from the current HEAD.
func (g Git) Create(ctx context.Context, id string) (Handle, error) {
	h := g.handle(id)
	if _, err := g.git(ctx, "worktree", "add", "-b", h.Branch, h.Dir); err != nil {
		return Handle{}, err
	}
	return h, nil
}

// Merge merges the worktree's branch into the repo's current branch, then
// removes the worktree and its branch.
func (g Git) Merge(ctx context.Context, id string) error {
	h := g.handle(id)
	if _, err := g.git(ctx, "merge", "--no-edit", h.Branch); err != nil {
		return err
	}
	return g.Discard(ctx, id)
}

// Discard removes the worktree and deletes its branch without merging.
func (g Git) Discard(ctx context.Context, id string) error {
	h := g.handle(id)
	if _, err := g.git(ctx, "worktree", "remove", "--force", h.Dir); err != nil {
		return err
	}
	_, err := g.git(ctx, "branch", "-D", h.Branch)
	return err
}

// List returns the sigma-managed worktrees.
func (g Git) List(ctx context.Context) ([]Handle, error) {
	out, err := g.git(ctx, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var hs []Handle
	var dir string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			dir = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			if b := strings.TrimPrefix(ref, "refs/heads/"); strings.HasPrefix(b, "sigma/") {
				hs = append(hs, Handle{ID: strings.TrimPrefix(b, "sigma/"), Dir: dir, Branch: b})
			}
		}
	}
	return hs, nil
}

func (g Git) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.Root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
