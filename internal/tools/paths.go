package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// rooted resolves p against root and confines it there: the result can never
// escape root via ".." or an absolute path. An empty root returns p unchanged,
// so paths resolve from the process cwd (the default, unconfined behavior).
func rooted(root, p string) (string, error) {
	if root == "" {
		return p, nil
	}
	joined := filepath.Join(root, p)
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace root", p)
	}
	return joined, nil
}

// unrooted expresses a full path (under root) relative to root, so tool output
// paths round-trip as inputs. An empty root returns full unchanged.
func unrooted(root, full string) string {
	if root == "" {
		return full
	}
	if rel, err := filepath.Rel(root, full); err == nil {
		return rel
	}
	return full
}
