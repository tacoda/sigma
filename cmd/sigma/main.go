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
	"github.com/tacoda/sigma/internal/agents"
	"github.com/tacoda/sigma/internal/anthropic"
	"github.com/tacoda/sigma/internal/auth"
	"github.com/tacoda/sigma/internal/config"
	"github.com/tacoda/sigma/internal/eval"
	"github.com/tacoda/sigma/internal/exec"
	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/mcp"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/permission"
	"github.com/tacoda/sigma/internal/plugin"
	_ "github.com/tacoda/sigma/internal/plugins/codehealth" // register built-in plugin
	_ "github.com/tacoda/sigma/internal/plugins/stylepack"  // register built-in plugin
	_ "github.com/tacoda/sigma/internal/plugins/telemetry"  // register built-in plugin
	"github.com/tacoda/sigma/internal/prompt"
	"github.com/tacoda/sigma/internal/rules"
	"github.com/tacoda/sigma/internal/scaffold"
	"github.com/tacoda/sigma/internal/session"
	"github.com/tacoda/sigma/internal/skills"
	"github.com/tacoda/sigma/internal/styles"
	"github.com/tacoda/sigma/internal/tools"
	"github.com/tacoda/sigma/internal/tui"
	"github.com/tacoda/sigma/internal/workflows"
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

// authModel is cheap; used for the auth smoke test.
const authModel = "claude-haiku-4-5-20251001"

// agentModel drives the coding agent.
const agentModel = "claude-sonnet-4-6"

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
	if live {
		fmt.Fprintln(os.Stderr, "eval: --live not yet supported (replay only)")
		os.Exit(2)
	}
	exp, err := eval.Load(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
	// Transcripts are resolved relative to the experiment file's directory.
	runner := eval.ReplayRunner{Base: filepath.Dir(args[0])}
	rep, err := exp.Run(context.Background(), runner, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
	fmt.Print(rep.String())
}

func loadClient() *anthropic.Client {
	creds, err := auth.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load credentials:", err)
		os.Exit(1)
	}
	if err := auth.EnsureValid(creds); err != nil {
		fmt.Fprintln(os.Stderr, "credentials expired and refresh failed:", err)
		os.Exit(1)
	}
	return anthropic.New(creds.AccessToken)
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

// deps are the shared building blocks for an agent session. newTools builds the
// tool set for a workspace root (cwd by default; a worktree when isolating).
type deps struct {
	client    *anthropic.Client
	newTools  func(root string) []tools.Tool
	types     agents.Set
	workflows workflows.Set
	isolate   bool
	permMode  permission.Mode
	compactAt int
	bus       hooks.Bus
	model     string
	system    string
	allowed   []string
	cleanup   func()
}

// hookDebug logs every event to stderr when SIGMA_HOOK_DEBUG is set.
func hookDebug(_ context.Context, ev hooks.Event) hooks.Outcome {
	fmt.Fprintf(os.Stderr, "[hook] %s %s\n", ev.Kind, ev.Tool)
	return hooks.Outcome{}
}

// eventLogPath resolves the event-log path: env override wins over config.
func eventLogPath(cfg config.Settings) string {
	if p := os.Getenv("SIGMA_EVENT_LOG"); p != "" {
		return p
	}
	return cfg.EventLog
}

// openEventLog opens the JSONL event log for appending, creating its directory.
// Returns nil on failure (logging is best-effort).
func openEventLog(path string) *os.File {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "event log:", err)
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "event log:", err)
		return nil
	}
	return f
}

// buildBus composes the hook buses: in-process callbacks, declarative YAML
// rules, then legacy shell commands from settings.json.
func buildBus(shellHooks map[string][]string) (hooks.Bus, error) {
	rules, err := hooks.LoadRules(hooks.RulePaths()...)
	if err != nil {
		return nil, err
	}
	cb := hooks.NewCallbacks()
	if os.Getenv("SIGMA_HOOK_DEBUG") != "" {
		cb.OnAll(hookDebug)
	}
	return hooks.Multi{cb, rules, hooks.NewShell(shellHooks)}, nil
}

// buildDeps assembles config, client, tools, skills, MCP servers, hooks, and
// the system prompt. The caller must invoke deps.cleanup when the session ends.
func buildDeps() deps {
	cfg := config.Load()
	model := agentModel
	if cfg.Model != "" {
		model = cfg.Model
	}

	// Mount enabled plugin bundles; they contribute tools, sources, and hooks.
	host, err := plugin.Mount(cfg.Plugins, cfg.PluginConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mount plugins:", err)
		os.Exit(1)
	}

	// extra tools are workspace-root-independent (skills, MCP, plugins); the file
	// tools are rebuilt per root by newTools.
	var extra []tools.Tool
	sources := []prompt.Source{rules.Source{}}
	if cfg.OutputStyle != "" {
		if st, ok := styles.Load()[cfg.OutputStyle]; ok {
			sources = append(sources, st)
		} else {
			fmt.Fprintf(os.Stderr, "output style %q not found; ignoring\n", cfg.OutputStyle)
		}
	}
	if sk := skills.Load(); len(sk) > 0 {
		extra = append(extra, skills.NewTool(sk))
		sources = append(sources, sk)
	}
	sources = append(sources, host.Sources...)
	system, err := prompt.Assemble(sources...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "assemble system prompt:", err)
		os.Exit(1)
	}

	bus, err := buildBus(cfg.Hooks)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load hooks:", err)
		os.Exit(1)
	}
	if len(host.Hooks) > 0 {
		bus = append(hooks.Multi{bus}, host.Hooks...)
	}

	cleanup := func() {}
	if len(cfg.MCPServers) > 0 {
		client, mcpTools := mcp.Connect(context.Background(), cfg.MCPServers)
		extra = append(extra, mcpTools...)
		cleanup = client.Close
	}
	extra = append(extra, host.Tools...)

	// Event-log sensor: record the event stream as JSONL if configured.
	if path := eventLogPath(cfg); path != "" {
		if f := openEventLog(path); f != nil {
			bus = hooks.Multi{hooks.NewSink(f), bus}
			prev := cleanup
			cleanup = func() { f.Close(); prev() }
		}
	}

	sb := cfg.Sandbox
	newTools := func(root string) []tools.Tool {
		var ex exec.Executor = exec.Local{Dir: root}
		if sb.Enabled {
			ex = exec.Sandbox{Dir: root, Policy: exec.Policy{AllowNetwork: sb.Network, Writable: sb.Writable}}
		}
		return append(tools.FS(root, ex), extra...)
	}

	return deps{
		client:    loadClient(),
		newTools:  newTools,
		types:     agents.Load(),
		workflows: workflows.Load(),
		isolate:   cfg.Isolate,
		permMode:  permission.ParseMode(cfg.PermissionMode),
		compactAt: cfg.CompactAt,
		bus:       bus,
		model:     model,
		system:    system,
		allowed:   cfg.AllowedTools,
		cleanup:   cleanup,
	}
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
	d := buildDeps()
	defer d.cleanup()
	mode := d.permMode
	if autoYes {
		mode = permission.Bypass
	}
	gate := permission.New(os.Stdin, os.Stderr)
	gate.PreApprove(d.allowed...)

	base := agent.Config{
		Client:     d.client,
		Permission: permission.ForMode(mode, gate),
		Hooks:      d.bus,
		Model:      d.model,
		System:     d.system,
		CompactAt:  d.compactAt,
	}
	base.Tools = agent.WithSubagent(base, agent.SubagentOptions{
		Tools:     d.newTools,
		Types:     d.types,
		Workflows: d.workflows,
		Isolate:   d.isolate,
		Workspace: workspace.Git{},
	})
	base.UI = consoleUI{}
	a := agent.New(base)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	d.bus.Emit(ctx, hooks.Event{Kind: hooks.SessionStart})
	err := a.Run(ctx, strings.Join(args, " "))
	d.bus.Emit(ctx, hooks.Event{Kind: hooks.SessionEnd})
	fmt.Println()
	if err != nil {
		fmt.Fprintln(os.Stderr, "run failed:", err)
		os.Exit(1)
	}
}

func runChat(args []string) {
	resume := len(args) > 0 && args[0] == "--resume"
	d := buildDeps()
	defer d.cleanup()
	cfg := tui.Config{
		Client:    d.client,
		NewTools:  d.newTools,
		Types:     d.types,
		Workflows: d.workflows,
		Isolate:   d.isolate,
		Hooks:     d.bus,
		Allowed:   d.allowed,
		Model:     d.model,
		System:    d.system,
		Store:     session.Store{},
		Mode:      d.permMode,
		CompactAt: d.compactAt,
		Resume:    resume,
	}
	if err := tui.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "chat failed:", err)
		os.Exit(1)
	}
}
