// Command sigma is a coding-agent CLI authenticated with Claude Code
// subscription credentials.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/anthropic"
	"github.com/tacoda/sigma/internal/auth"
	"github.com/tacoda/sigma/internal/config"
	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/mcp"
	"github.com/tacoda/sigma/internal/message"
	"github.com/tacoda/sigma/internal/permission"
	"github.com/tacoda/sigma/internal/prompt"
	"github.com/tacoda/sigma/internal/rules"
	"github.com/tacoda/sigma/internal/session"
	"github.com/tacoda/sigma/internal/skills"
	"github.com/tacoda/sigma/internal/tools"
	"github.com/tacoda/sigma/internal/tui"
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
	case "auth":
		runAuth(os.Args[2:])
	case "run":
		runAgent(os.Args[2:])
	case "chat":
		runChat(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: sigma <command>

commands:
  version            print version
  auth test          verify Claude Code credentials with a live API call
  auth status        show credential status without calling the API
  auth refresh       force an OAuth token refresh
  run [--yes] <prompt...>  run the coding agent on a one-shot prompt
                           (--yes auto-approves all tool calls)
  chat [--resume]          interactive multi-turn TUI session
                           (--resume continues the saved session)`)
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

// deps are the shared building blocks for an agent session. childTools are the
// tools a sub-agent may use; the parent registry adds a `task` tool on top.
type deps struct {
	client     *anthropic.Client
	childTools []tools.Tool
	hooks      agent.Hooks
	model      string
	system     string
	allowed    []string
	cleanup    func()
}

// buildDeps assembles config, client, tools, skills, MCP servers, hooks, and
// the system prompt. The caller must invoke deps.cleanup when the session ends.
func buildDeps() deps {
	cfg := config.Load()
	model := agentModel
	if cfg.Model != "" {
		model = cfg.Model
	}

	childTools := []tools.Tool{
		tools.ReadFile{}, tools.WriteFile{}, tools.EditFile{},
		tools.Bash{}, tools.Glob{}, tools.Grep{},
	}
	sources := []prompt.Source{rules.Source{}}
	if sk := skills.Load(); len(sk) > 0 {
		childTools = append(childTools, skills.NewTool(sk))
		sources = append(sources, sk)
	}
	system, err := prompt.Assemble(sources...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "assemble system prompt:", err)
		os.Exit(1)
	}

	cleanup := func() {}
	if len(cfg.MCPServers) > 0 {
		client, mcpTools := mcp.Connect(context.Background(), cfg.MCPServers)
		childTools = append(childTools, mcpTools...)
		cleanup = client.Close
	}

	return deps{
		client:     loadClient(),
		childTools: childTools,
		hooks:      hooks.New(cfg.Hooks),
		model:      model,
		system:     system,
		allowed:    cfg.AllowedTools,
		cleanup:    cleanup,
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
	gate := permission.New(os.Stdin, os.Stderr)
	if autoYes {
		gate = permission.NewAuto()
	}
	gate.PreApprove(d.allowed...)

	base := agent.Config{
		Client:     d.client,
		Permission: gate,
		Hooks:      d.hooks,
		Model:      d.model,
		System:     d.system,
	}
	base.Tools = agent.WithSubagent(base, d.childTools)
	base.UI = consoleUI{}
	a := agent.New(base)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := a.Run(ctx, strings.Join(args, " "))
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
		Client:     d.client,
		ChildTools: d.childTools,
		Hooks:      d.hooks,
		Allowed:    d.allowed,
		Model:      d.model,
		System:     d.system,
		Store:      session.Store{},
		Resume:     resume,
	}
	if err := tui.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "chat failed:", err)
		os.Exit(1)
	}
}
