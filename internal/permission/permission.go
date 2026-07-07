// Package permission gates mutating tool calls behind user approval.
//
// Read-only tools bypass the gate (the agent decides that). Mutating tools ask
// the user [y]es once, [a]lways for this session, or anything else to deny.
package permission

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Gate decides whether a tool call may proceed.
type Gate struct {
	in      *bufio.Reader
	out     io.Writer
	session map[string]bool
}

// New returns an interactive gate reading approvals from in, prompting on out.
func New(in io.Reader, out io.Writer) *Gate {
	return &Gate{in: bufio.NewReader(in), out: out, session: map[string]bool{}}
}

// PreApprove marks tools as approved up front (e.g. from settings) so they
// never prompt.
func (g *Gate) PreApprove(names ...string) {
	for _, n := range names {
		g.session[n] = true
	}
}

// Allow reports whether the named tool may run. detail describes the call.
func (g *Gate) Allow(name, detail string) bool {
	if g.session[name] {
		return true
	}
	fmt.Fprintf(g.out, "\n  ⚠ allow %s? %s\n  [y]es / [a]lways / [N]o: ", name, detail)
	line, err := g.in.ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "a", "always":
		g.session[name] = true
		return true
	default:
		return false
	}
}
