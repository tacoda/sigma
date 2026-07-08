package eval

import (
	"context"

	"github.com/tacoda/sigma/internal/exec"
	"github.com/tacoda/sigma/internal/hooks"
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

// Trace scores a case's TraceAssert against the run's event stream.
type Trace struct{}

func (Trace) Score(_ context.Context, c Case, r Result) []Score {
	if c.Trace == nil {
		return nil
	}
	used := map[string]bool{}
	turns, toolErrs := 0, 0
	for _, ev := range r.Trace {
		switch ev.Kind {
		case hooks.PreTool:
			used[ev.Tool] = true
		case hooks.PostLLM:
			turns++
		case hooks.ToolError:
			toolErrs++
		}
	}

	var out []Score
	for _, t := range c.Trace.Used {
		out = append(out, boolScore("used: "+t, used[t]))
	}
	for _, t := range c.Trace.NotUsed {
		out = append(out, boolScore("notUsed: "+t, !used[t]))
	}
	if c.Trace.NoError {
		out = append(out, boolScore("noError", toolErrs == 0))
	}
	if c.Trace.MaxTurns > 0 {
		out = append(out, boolScore("maxTurns", turns <= c.Trace.MaxTurns))
	}
	return out
}

func boolScore(name string, ok bool) Score {
	s := Score{Name: name, Pass: ok}
	if ok {
		s.Value = 1
	}
	return s
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
