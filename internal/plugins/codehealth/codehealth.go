// Package codehealth is a built-in plugin: an output-validation gate. It runs
// configured check commands (tests, vet, lint, a CodeScene call, ...) when a
// turn tries to finish, and blocks completion if any fail — the agent gets the
// failure output and must fix it before the turn can end. It also exposes a
// code_health tool to run the checks on demand.
//
// Config (pluginConfig.codehealth):
//
//	{ "checks": ["go test ./...", "go vet ./..."] }
//
// Defaults to `go test ./...` and `go vet ./...` when unset.
package codehealth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tacoda/sigma/internal/exec"
	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/plugin"
)

func init() { plugin.Register(plug{}) }

type plug struct{}

func (plug) Name() string { return "codehealth" }

func (plug) Register(h *plugin.Host, raw plugin.Config) error {
	var cfg struct {
		Checks []string `json:"checks"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	if len(cfg.Checks) == 0 {
		cfg.Checks = []string{"go test ./...", "go vet ./..."}
	}
	h.AddHook(gate{checks: cfg.Checks})
	h.AddTool(checkTool{checks: cfg.Checks})
	return nil
}

// runChecks runs each command; ok is false if any fail, and the report contains
// the failing commands' output.
func runChecks(ctx context.Context, checks []string) (report string, ok bool) {
	var b strings.Builder
	ok = true
	for _, c := range checks {
		out, err := exec.Local{}.Run(ctx, exec.Spec{Command: c})
		if err != nil {
			ok = false
			fmt.Fprintf(&b, "✗ %s\n%s\n", c, strings.TrimSpace(out))
		} else {
			fmt.Fprintf(&b, "✓ %s\n", c)
		}
	}
	return b.String(), ok
}

// gate blocks Stop until every check passes.
type gate struct{ checks []string }

func (g gate) Emit(ctx context.Context, ev hooks.Event) hooks.Outcome {
	if ev.Kind != hooks.Stop {
		return hooks.Outcome{}
	}
	if report, ok := runChecks(ctx, g.checks); !ok {
		return hooks.Outcome{Block: true, Reason: "code-health checks failed:\n" + report}
	}
	return hooks.Outcome{}
}

// checkTool runs the checks on demand.
type checkTool struct{ checks []string }

func (checkTool) Name() string { return "code_health" }
func (checkTool) Description() string {
	return "Run the project's code-health checks (tests, vet, lint) and report pass/fail."
}
func (checkTool) ReadOnly() bool          { return true }
func (checkTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t checkTool) Run(ctx context.Context, _ json.RawMessage) (string, error) {
	report, _ := runChecks(ctx, t.checks)
	return report, nil
}
