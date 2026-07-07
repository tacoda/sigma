package permission

import (
	"io"
	"strings"
	"testing"
)

func TestAllowOnce(t *testing.T) {
	g := New(strings.NewReader("y\n"), io.Discard)
	if !g.Allow("bash", "ls") {
		t.Error("y should allow")
	}
	if g.session["bash"] {
		t.Error("y should not persist for session")
	}
}

func TestAllowAlways(t *testing.T) {
	// First call reads "a"; second call must not need input.
	g := New(strings.NewReader("a\n"), io.Discard)
	if !g.Allow("bash", "ls") {
		t.Error("a should allow")
	}
	if !g.Allow("bash", "pwd") {
		t.Error("session allow should persist")
	}
}

func TestDeny(t *testing.T) {
	g := New(strings.NewReader("n\n"), io.Discard)
	if g.Allow("bash", "ls") {
		t.Error("n should deny")
	}
}
