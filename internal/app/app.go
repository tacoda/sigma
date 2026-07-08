// Package app is the composition root: it assembles the agent's collaborators
// (tools, skills, MCP, hooks, plugins, system prompt, sub-agent options) from a
// charter — the .sigma/ configuration bundle. main and the eval harness both
// build through it so they run the same agent.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/agents"
	"github.com/tacoda/sigma/internal/config"
	"github.com/tacoda/sigma/internal/exec"
	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/mcp"
	"github.com/tacoda/sigma/internal/permission"
	"github.com/tacoda/sigma/internal/plugin"
	"github.com/tacoda/sigma/internal/prompt"
	"github.com/tacoda/sigma/internal/rules"
	"github.com/tacoda/sigma/internal/skills"
	"github.com/tacoda/sigma/internal/styles"
	"github.com/tacoda/sigma/internal/tools"
	"github.com/tacoda/sigma/internal/workflows"
)

// DefaultModel drives the coding agent when the charter sets none.
const DefaultModel = "claude-sonnet-4-6"

// Options parameterize a build.
type Options struct {
	Client     agent.LLM // model backend (required)
	ConfigRoot string    // directory to load the charter from; "" = process cwd
}

// Deps are the assembled building blocks. The caller supplies UI and permission
// and constructs the agent (console, TUI, and eval differ only there).
type Deps struct {
	Client    agent.LLM
	NewTools  func(root string) []tools.Tool
	Types     agents.Set
	Workflows workflows.Set
	Isolate   bool
	PermMode  permission.Mode
	CompactAt int
	Bus       hooks.Bus
	Model     string
	System    string
	Allowed   []string
	Cleanup   func()
}

// Build assembles Deps from the charter at ConfigRoot. It loads config while
// chdir'd to ConfigRoot so all loaders (which read cwd-relative .sigma) pick up
// that charter; cwd is restored before returning.
func Build(o Options) (*Deps, error) {
	if o.ConfigRoot != "" {
		orig, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		if err := os.Chdir(o.ConfigRoot); err != nil {
			return nil, err
		}
		defer os.Chdir(orig)
	}

	cfg := config.Load()
	model := DefaultModel
	if cfg.Model != "" {
		model = cfg.Model
	}

	host, err := plugin.Mount(cfg.Plugins, cfg.DisablePlugins, cfg.PluginConfig)
	if err != nil {
		return nil, fmt.Errorf("mount plugins: %w", err)
	}

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
		return nil, fmt.Errorf("assemble system prompt: %w", err)
	}

	bus, err := buildBus(cfg.Hooks)
	if err != nil {
		return nil, fmt.Errorf("load hooks: %w", err)
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

	if path := eventLogPath(cfg); path != "" {
		if f := openEventLog(path); f != nil {
			bus = append(hooks.Multi{hooks.NewSink(f)}, bus)
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

	return &Deps{
		Client:    o.Client,
		NewTools:  newTools,
		Types:     agents.Load(),
		Workflows: workflows.Load(),
		Isolate:   cfg.Isolate,
		PermMode:  permission.ParseMode(cfg.PermissionMode),
		CompactAt: cfg.CompactAt,
		Bus:       bus,
		Model:     model,
		System:    system,
		Allowed:   cfg.AllowedTools,
		Cleanup:   cleanup,
	}, nil
}

// hookDebug logs every event to stderr when SIGMA_HOOK_DEBUG is set.
func hookDebug(_ context.Context, ev hooks.Event) hooks.Outcome {
	fmt.Fprintf(os.Stderr, "[hook] %s %s\n", ev.Kind, ev.Tool)
	return hooks.Outcome{}
}

func buildBus(shellHooks map[string][]string) (hooks.Bus, error) {
	rls, err := hooks.LoadRules(hooks.RulePaths()...)
	if err != nil {
		return nil, err
	}
	cb := hooks.NewCallbacks()
	if os.Getenv("SIGMA_HOOK_DEBUG") != "" {
		cb.OnAll(hookDebug)
	}
	return hooks.Multi{cb, rls, hooks.NewShell(shellHooks)}, nil
}

func eventLogPath(cfg config.Settings) string {
	if p := os.Getenv("SIGMA_EVENT_LOG"); p != "" {
		return p
	}
	return cfg.EventLog
}

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
