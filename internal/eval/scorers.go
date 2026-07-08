package eval

import (
	"context"

	"github.com/tacoda/sigma/internal/exec"
)

// Programmatic scores a case by running its Checks (shell commands) in the
// result's workspace; each check passes if it exits zero.
type Programmatic struct{}

func (Programmatic) Score(ctx context.Context, c Case, r Result) []Score {
	out := make([]Score, 0, len(c.Checks))
	for _, check := range c.Checks {
		cmdOut, err := exec.Local{Dir: r.Dir}.Run(ctx, exec.Spec{Command: check})
		s := Score{Name: "check: " + check, Pass: err == nil}
		if err == nil {
			s.Value = 1
		} else {
			s.Detail = firstLine(cmdOut)
		}
		out = append(out, s)
	}
	return out
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
