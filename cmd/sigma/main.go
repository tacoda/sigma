// Command sigma is a coding-agent CLI authenticated with Claude Code
// subscription credentials.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/anthropic"
	"github.com/tacoda/sigma/internal/app"
	"github.com/tacoda/sigma/internal/auth"
	"github.com/tacoda/sigma/internal/eval"
	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/permission"
	_ "github.com/tacoda/sigma/internal/plugins/codehealth" // register built-in plugin
	_ "github.com/tacoda/sigma/internal/plugins/stylepack"  // register built-in plugin
	_ "github.com/tacoda/sigma/internal/plugins/telemetry"  // register built-in plugin
	"github.com/tacoda/sigma/internal/scaffold"
	"github.com/tacoda/sigma/internal/session"
	"github.com/tacoda/sigma/internal/tui"
	"github.com/tacoda/sigma/internal/workspace"
)

// consoleUI prints agent output to the terminal (used by `run`).
type consoleUI struct{}

func (consoleUI) Text(delta string) { fmt.Print(delta) }

func (consoleUI) ToolCall(name, input string) {
	fmt.Printf("\n  ⚙ %s %s\n", name, input)
}

func (consoleUI) ToolResult(name, output string, isErr bool) {
	status := "✓"
	if isErr {
		status = "✗"
	}
	fmt.Printf("  %s %s\n", status, name)
}

func (consoleUI) Usage(int, int) {}

const version = "0.0.1"

// authModel is cheap; used for the auth smoke test and the eval judge.
const authModel = "claude-haiku-4-5-20251001"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("sigma", version)
	case "init":
		runInit()
	case "auth":
		runAuth(os.Args[2:])
	case "run":
		runAgent(os.Args[2:])
	case "chat":
		runChat(os.Args[2:])
	case "eval":
		runEval(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: sigma <command>

commands:
  version            print version
  init               scaffold a starter .sigma/ config and examples
  auth test          verify Claude Code credentials with a live API call
  auth status        show credential status without calling the API
  auth refresh       force an OAuth token refresh
  run [--yes] <prompt...>  run the coding agent on a one-shot prompt
                           (--yes auto-approves all tool calls)
  chat [--resume]          interactive multi-turn TUI session
                           (--resume continues the saved session)
  eval <experiment.yaml>   run an A/B eval experiment (replay by default)`)
}

func runInit() {
	created, err := scaffold.Init(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		os.Exit(1)
	}
	if len(created) == 0 {
		fmt.Println("nothing to do: .sigma/ already set up")
		return
	}
	for _, p := range created {
		fmt.Println("created", p)
	}
}

func runEval(args []string) {
	live := false
	if len(args) > 0 && args[0] == "--live" {
		live = true
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "eval: experiment file required")
		os.Exit(2)
	}
	exp, err := eval.Load(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
	// Transcripts are resolved relative to the experiment file's directory.
	base := filepath.Dir(args[0])

	var runner eval.Runner = eval.ReplayRunner{Base: base}
	if live {
		client, err := tryLoadClient()
		if err != nil {
			fmt.Fprintln(os.Stderr, "eval --live:", err)
			os.Exit(1)
		}
		runner = eval.LiveRunner{Client: client, RecordDir: base}
	}

	scorers := []eval.Scorer{eval.Programmatic{}, eval.Trace{}}
	if usesJudge(exp) {
		if c, err := tryLoadClient(); err == nil {
			scorers = append(scorers, eval.Judge{Client: c, Model: authModel})
		} else {
			fmt.Fprintln(os.Stderr, "eval: judge scorer skipped (no credentials):", err)
		}
	}

	rep, err := exp.Run(context.Background(), runner, scorers)
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}

	html, err := eval.ReporterFor(exp.Level).Render(rep)
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval: render:", err)
		os.Exit(1)
	}
	out := filepath.Join(base, "report.html")
	if err := os.WriteFile(out, []byte(html), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "eval: write report:", err)
		os.Exit(1)
	}
	fmt.Print(rep.String())
	fmt.Println("\nreport:", out)
}

func usesJudge(exp *eval.Experiment) bool {
	for _, c := range exp.Cases {
		if c.Judge != "" {
			return true
		}
	}
	return false
}

func loadClient() *anthropic.Client {
	c, err := tryLoadClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return c
}

// tryLoadClient loads credentials without exiting, for callers that can degrade
// (e.g. the eval judge scorer).
func tryLoadClient() (*anthropic.Client, error) {
	creds, err := auth.Load()
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	if err := auth.EnsureValid(creds); err != nil {
		return nil, fmt.Errorf("credentials expired and refresh failed: %w", err)
	}
	return anthropic.New(creds.AccessToken), nil
}

func runAuth(args []string) {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "status":
		creds, err := auth.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "load credentials:", err)
			os.Exit(1)
		}
		fmt.Printf("ok: subscription=%s scopes=%v expired=%v\n",
			creds.SubscriptionType, creds.Scopes, creds.Expired())
	case "test":
		authTest()
	case "refresh":
		refreshCreds()
	default:
		usage()
		os.Exit(2)
	}
}

func refreshCreds() {
	creds, err := auth.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load credentials:", err)
		os.Exit(1)
	}
	if err := creds.Refresh(); err != nil {
		fmt.Fprintln(os.Stderr, "refresh failed:", err)
		os.Exit(1)
	}
	fmt.Println("ok: token refreshed and saved")
}

func authTest() {
	client := loadClient()
	resp, err := client.Complete(context.Background(), message.Request{
		Model:     authModel,
		MaxTokens: 64,
		Messages:  []message.Message{message.UserText("Reply with exactly: SIGMA_AUTH_OK")},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "auth test failed:", err)
		os.Exit(1)
	}
	fmt.Printf("reply: %s\n", resp.Text())
	fmt.Printf("tokens: in=%d out=%d\n", resp.Usage.InputTokens, resp.Usage.OutputTokens)
}

// build assembles the agent's collaborators from the local charter via the app
// composition root, exiting on failure.
func build() *app.Deps {
	d, err := app.Build(app.Options{Client: loadClient()})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return d
}

func runAgent(args []string) {
	autoYes := false
	if len(args) > 0 && args[0] == "--yes" {
		autoYes = true
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "run: prompt required")
		os.Exit(2)
	}
	d := build()
	defer d.Cleanup()
	mode := d.PermMode
	if autoYes {
		mode = permission.Bypass
	}
	gate := permission.New(os.Stdin, os.Stderr)
	gate.PreApprove(d.Allowed...)

	base := agent.Config{
		Client:     d.Client,
		Permission: permission.ForMode(mode, gate),
		Hooks:      d.Bus,
		Model:      d.Model,
		System:     d.System,
		CompactAt:  d.CompactAt,
	}
	base.Tools = agent.WithSubagent(base, agent.SubagentOptions{
		Tools:     d.NewTools,
		Types:     d.Types,
		Workflows: d.Workflows,
		Isolate:   d.Isolate,
		Workspace: workspace.Git{},
	})
	base.UI = consoleUI{}
	a := agent.New(base)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	d.Bus.Emit(ctx, hooks.Event{Kind: hooks.SessionStart})
	err := a.Run(ctx, strings.Join(args, " "))
	d.Bus.Emit(ctx, hooks.Event{Kind: hooks.SessionEnd})
	fmt.Println()
	if err != nil {
		fmt.Fprintln(os.Stderr, "run failed:", err)
		os.Exit(1)
	}
}

func runChat(args []string) {
	resume := len(args) > 0 && args[0] == "--resume"
	d := build()
	defer d.Cleanup()
	cfg := tui.Config{
		Client:    d.Client,
		NewTools:  d.NewTools,
		Types:     d.Types,
		Workflows: d.Workflows,
		Isolate:   d.Isolate,
		Hooks:     d.Bus,
		Allowed:   d.Allowed,
		Model:     d.Model,
		System:    d.System,
		Store:     session.Store{},
		Mode:      d.PermMode,
		CompactAt: d.CompactAt,
		Resume:    resume,
	}
	if err := tui.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "chat failed:", err)
		os.Exit(1)
	}
}
