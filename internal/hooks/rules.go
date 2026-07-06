package hooks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// A declarative hook rule from .sigma/hooks.yaml:
//
//	hooks:
//	  - on: PreToolUse
//	    match: { tool: "write_file|edit_file" }
//	    deny: "writes disabled"
//	  - on: PostToolUse
//	    match: { tool: bash }
//	    run: python3 .sigma/hooks/audit.py   # any interpreter; JSON on stdin
//	  - on: Stop
//	    notify: "turn complete"
//
// Actions run in order log -> notify -> run -> deny, so side effects happen
// before a deny blocks. run executes via `bash -c` and blocks on non-zero exit.
type ruleSpec struct {
	On    string `yaml:"on"`
	Match struct {
		Tool string `yaml:"tool"`
	} `yaml:"match"`
	Run    string `yaml:"run"`
	Deny   string `yaml:"deny"`
	Log    string `yaml:"log"`
	Notify string `yaml:"notify"`
}

type rulesFile struct {
	Hooks []ruleSpec `yaml:"hooks"`
}

type rule struct {
	kind Kind
	tool *regexp.Regexp // nil matches any tool
	act  Func
}

// Rules is a declarative bus compiled from YAML.
type Rules struct {
	rules []rule
}

func (r *Rules) Emit(ctx context.Context, ev Event) Outcome {
	for _, ru := range r.rules {
		if ru.kind != ev.Kind {
			continue
		}
		if ru.tool != nil && !ru.tool.MatchString(ev.Tool) {
			continue
		}
		if o := ru.act(ctx, ev); o.Block {
			return o
		}
	}
	return Outcome{}
}

// RulePaths returns the user and project rule files, project last.
func RulePaths() []string {
	var ps []string
	if home, err := os.UserHomeDir(); err == nil {
		ps = append(ps, filepath.Join(home, ".sigma", "hooks.yaml"))
	}
	return append(ps, filepath.Join(".sigma", "hooks.yaml"))
}

// LoadRules parses the given YAML files (missing ones are skipped) into a Rules
// bus.
func LoadRules(paths ...string) (*Rules, error) {
	var out []rule
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var f rulesFile
		if err := yaml.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		for i, spec := range f.Hooks {
			ru, err := compileRule(spec)
			if err != nil {
				return nil, fmt.Errorf("%s: rule %d: %w", p, i, err)
			}
			out = append(out, ru)
		}
	}
	return &Rules{rules: out}, nil
}

func compileRule(spec ruleSpec) (rule, error) {
	kind := Kind(spec.On)
	if !validKind(kind) {
		return rule{}, fmt.Errorf("unknown event %q", spec.On)
	}
	var re *regexp.Regexp
	if spec.Match.Tool != "" {
		r, err := regexp.Compile(spec.Match.Tool)
		if err != nil {
			return rule{}, fmt.Errorf("match.tool: %w", err)
		}
		re = r
	}
	act, err := compileAction(spec)
	if err != nil {
		return rule{}, err
	}
	return rule{kind: kind, tool: re, act: act}, nil
}

// compileAction builds the action chain. At least one action is required.
func compileAction(spec ruleSpec) (Func, error) {
	var acts []Func
	if spec.Log != "" {
		msg := spec.Log
		acts = append(acts, func(_ context.Context, ev Event) Outcome {
			fmt.Fprintln(os.Stderr, "[hook] "+expand(msg, ev))
			return Outcome{}
		})
	}
	if spec.Notify != "" {
		msg := spec.Notify
		acts = append(acts, func(_ context.Context, ev Event) Outcome {
			fmt.Fprintln(os.Stderr, "🔔 "+expand(msg, ev))
			return Outcome{}
		})
	}
	if spec.Run != "" {
		cmd := spec.Run
		acts = append(acts, func(ctx context.Context, ev Event) Outcome {
			out, err := run(ctx, cmd, ev)
			if err != nil {
				reason := strings.TrimSpace(out)
				if reason == "" {
					reason = err.Error()
				}
				return Outcome{Block: true, Reason: reason}
			}
			return Outcome{}
		})
	}
	if spec.Deny != "" {
		reason := spec.Deny
		acts = append(acts, func(_ context.Context, ev Event) Outcome {
			return Outcome{Block: true, Reason: expand(reason, ev)}
		})
	}
	if len(acts) == 0 {
		return nil, errors.New("no action (need run, deny, log, or notify)")
	}
	return func(ctx context.Context, ev Event) Outcome {
		for _, a := range acts {
			if o := a(ctx, ev); o.Block {
				return o
			}
		}
		return Outcome{}
	}, nil
}

func validKind(k Kind) bool {
	for _, v := range AllKinds {
		if v == k {
			return true
		}
	}
	return false
}

// expand substitutes {event} {tool} {output} {prompt} {message} in templates.
func expand(s string, ev Event) string {
	return strings.NewReplacer(
		"{event}", string(ev.Kind),
		"{tool}", ev.Tool,
		"{output}", ev.Output,
		"{prompt}", ev.Prompt,
		"{message}", ev.Message,
	).Replace(s)
}
