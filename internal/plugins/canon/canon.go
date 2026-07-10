// Package canon is a default-enabled plugin that shapes how the agent works. It
// contributes the engineering & platform canon to the system prompt (a guide)
// and installs deterministic guards (hooks) that block violations at the tool
// boundary. Rule-in-context + guard-on-boundary for each practice that has a
// clean check; context-only for the inferential ones.
package canon

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/plugin"
)

func init() { plugin.RegisterDefault(plug{}) }

type plug struct{}

func (plug) Name() string { return "canon" }

func (plug) Register(h *plugin.Host, _ plugin.Config) error {
	h.AddSource(guide{})
	h.AddHook(guards{})
	return nil
}

// guide contributes the canon to the system prompt.
type guide struct{}

func (guide) Contribute() (string, error) { return canonText, nil }

// guards blocks canon violations at PreToolUse.
type guards struct{}

func (guards) Emit(_ context.Context, ev hooks.Event) hooks.Outcome {
	if ev.Kind != hooks.PreTool {
		return hooks.Outcome{}
	}
	switch ev.Tool {
	case "bash":
		cmd := field(ev.Input, "command")
		if reason := dangerousCommand(cmd); reason != "" {
			return block(reason)
		}
		if reason := commitPolicy(cmd); reason != "" {
			return block(reason)
		}
	case "read_file":
		if p := field(ev.Input, "path"); sensitivePath(p) {
			return block("refusing to read sensitive file " + p + " — handle secrets out of band")
		}
	case "write_file", "edit_file":
		if p := field(ev.Input, "path"); sensitivePath(p) {
			return block("refusing to touch sensitive file " + p + " — handle secrets out of band")
		}
		content := field(ev.Input, "content") + "\n" + field(ev.Input, "new_string")
		if reason := scanContent(content); reason != "" {
			return block(reason)
		}
	}
	return hooks.Outcome{}
}

func block(reason string) hooks.Outcome { return hooks.Outcome{Block: true, Reason: reason} }

// field extracts a string field from a tool's JSON input.
func field(input, key string) string {
	var m map[string]any
	if json.Unmarshal([]byte(input), &m) != nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

var (
	sensitiveRe   = regexp.MustCompile(`(?i)(^|/)(\.env($|\.)|.*\.(pem|key)$|.*credentials.*|.*/secrets?/|id_rsa)`)
	forcePushRe   = regexp.MustCompile(`git\s+push\b.*(--force\b|-f\b)`)
	resetHardRe   = regexp.MustCompile(`git\s+reset\s+--hard`)
	noVerifyRe    = regexp.MustCompile(`git\s+commit\b.*--no-verify|\s-n\b`)
	pipeShellRe   = regexp.MustCompile(`(curl|wget)\b.*\|\s*(sudo\s+)?(ba)?sh`)
	rmRfRe        = regexp.MustCompile(`\brm\s+-[a-zA-Z]*[rf][a-zA-Z]*\s`)
	attributionRe = regexp.MustCompile(`(?i)co-authored-by|generated with|ai-generated|\bclaude\b`)
	gitAddAllRe   = regexp.MustCompile(`git\s+add\s+(-A|--all|\.)(\s|$)`)
	commitMsgRe   = regexp.MustCompile(`git\s+commit\b[^|;&]*?-m\s+(?:"([^"]*)"|'([^']*)')`)
	convCommitRe  = regexp.MustCompile(`^(feat|fix|refactor|test|docs|chore|perf|build|ci|style|revert)(\([^)]+\))?!?: .+`)
	mergeMsgRe    = regexp.MustCompile(`^(Merge |Revert |fixup!|squash!)`)

	secretRes = []*regexp.Regexp{
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
		regexp.MustCompile(`gh[pousr]_[0-9A-Za-z]{20,}`),
		regexp.MustCompile(`xox[baprs]-[0-9A-Za-z-]{10,}`),
		regexp.MustCompile(`(?i)(password|secret|api[_-]?key|token)\s*[:=]\s*["'][^"']{8,}["']`),
	}
	debugRes = map[string]*regexp.Regexp{
		"debugger statement": regexp.MustCompile(`(^|\W)debugger\s*;`),
		"console.log":        regexp.MustCompile(`console\.log\(`),
		"binding.pry":        regexp.MustCompile(`binding\.pry`),
		"var_dump/dd":        regexp.MustCompile(`(^|\W)(var_dump|dd)\(`),
	}
)

func sensitivePath(p string) bool { return p != "" && sensitiveRe.MatchString(p) }

func dangerousCommand(cmd string) string {
	switch {
	case cmd == "":
		return ""
	case forcePushRe.MatchString(cmd):
		return "force-push is destructive — rebase and push normally, or do it manually"
	case resetHardRe.MatchString(cmd):
		return "git reset --hard discards work — do it manually if truly intended"
	case noVerifyRe.MatchString(cmd):
		return "never bypass checks with --no-verify; fix the failure instead"
	case pipeShellRe.MatchString(cmd):
		return "refusing to pipe a download into a shell — inspect and run deliberately"
	case rmRfRe.MatchString(cmd):
		return "recursive force-delete is destructive — do it manually if truly intended"
	case strings.Contains(cmd, "git commit") && attributionRe.MatchString(cmd):
		return "no AI attribution in commits — the author is the human running the harness"
	}
	return ""
}

// commitPolicy enforces the git workflow canon: stage specific files, and use
// Conventional Commits. It only inspects an inline -m message (editor/heredoc
// commits are left alone).
func commitPolicy(cmd string) string {
	if gitAddAllRe.MatchString(cmd) {
		return "stage specific files, not everything (avoid git add . / -A / --all)"
	}
	m := commitMsgRe.FindStringSubmatch(cmd)
	if m == nil {
		return ""
	}
	subject := m[1]
	if subject == "" {
		subject = m[2]
	}
	subject = strings.SplitN(strings.TrimSpace(subject), "\n", 2)[0] // first line only
	if subject == "" || convCommitRe.MatchString(subject) || mergeMsgRe.MatchString(subject) {
		return ""
	}
	return "use Conventional Commits: type(scope): subject (e.g. feat(x): add y)"
}

func scanContent(content string) string {
	for _, re := range secretRes {
		if re.MatchString(content) {
			return "possible secret in the content — never commit credentials; handle out of band"
		}
	}
	for name, re := range debugRes {
		if re.MatchString(content) {
			return "debugging artifact (" + name + ") — remove it before writing committed code"
		}
	}
	return ""
}
