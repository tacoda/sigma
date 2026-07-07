package permission

// Mode selects how mutating tool calls are gated. Modes compose over an
// interactive gate: acceptEdits and default delegate prompts to it, while
// bypass and plan decide outright.
type Mode string

const (
	Default     Mode = "default"     // prompt for every mutation
	AcceptEdits Mode = "acceptEdits" // auto-approve file edits, prompt the rest
	Plan        Mode = "plan"        // deny all mutations (read-only exploration)
	Bypass      Mode = "bypass"      // approve everything
)

// ParseMode maps a config string to a Mode, defaulting to Default.
func ParseMode(s string) Mode {
	switch Mode(s) {
	case AcceptEdits, Plan, Bypass:
		return Mode(s)
	default:
		return Default
	}
}

// Allower decides whether a mutating tool may run.
type Allower interface {
	Allow(name, detail string) bool
}

// ForMode wraps the inner (interactive) gate according to mode.
func ForMode(m Mode, inner Allower) Allower {
	switch m {
	case Bypass:
		return autoAllow{}
	case Plan:
		return denyAll{}
	case AcceptEdits:
		return acceptTools{names: map[string]bool{"write_file": true, "edit_file": true}, inner: inner}
	default:
		return inner
	}
}

type autoAllow struct{}

func (autoAllow) Allow(string, string) bool { return true }

type denyAll struct{}

func (denyAll) Allow(string, string) bool { return false }

// acceptTools approves the named tools outright and delegates the rest.
type acceptTools struct {
	names map[string]bool
	inner Allower
}

func (a acceptTools) Allow(name, detail string) bool {
	if a.names[name] {
		return true
	}
	return a.inner.Allow(name, detail)
}
