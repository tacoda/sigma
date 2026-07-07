// Package config loads layered settings: user-level then project-level, with
// project values taking precedence.
//
//	user:    ~/.sigma/settings.json
//	project: ./.sigma/settings.json
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings is the merged configuration.
type Settings struct {
	// Model overrides the default agent model.
	Model string `json:"model,omitempty"`
	// AllowedTools are auto-approved (the permission gate won't prompt).
	AllowedTools []string `json:"allowedTools,omitempty"`
	// MCPServers are external Model Context Protocol servers to connect.
	MCPServers map[string]MCPServer `json:"mcpServers,omitempty"`
	// Hooks are shell commands run around tool execution, keyed by event.
	Hooks map[string][]string `json:"hooks,omitempty"`
	// Isolate runs each sub-agent task in a fresh git worktree.
	Isolate bool `json:"isolate,omitempty"`
	// Sandbox confines bash commands with the OS sandbox.
	Sandbox Sandbox `json:"sandbox,omitempty"`
	// EventLog, if set, is a path where the event stream is recorded as JSONL.
	EventLog string `json:"eventLog,omitempty"`
	// PermissionMode gates mutating tools: default, acceptEdits, plan, bypass.
	PermissionMode string `json:"permissionMode,omitempty"`
	// CompactAt summarizes history once a request's input tokens reach this
	// count. 0 disables compaction.
	CompactAt int `json:"compactAt,omitempty"`
}

// Sandbox configures command confinement.
type Sandbox struct {
	Enabled  bool     `json:"enabled,omitempty"`
	Network  bool     `json:"network,omitempty"`  // allow network access
	Writable []string `json:"writable,omitempty"` // extra writable directories
}

// MCPServer describes one MCP server: a stdio command, or an HTTP URL.
type MCPServer struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// Load reads and merges user then project settings. Missing files are ignored.
func Load() Settings {
	var s Settings
	if home, err := os.UserHomeDir(); err == nil {
		merge(&s, filepath.Join(home, ".sigma", "settings.json"))
	}
	merge(&s, filepath.Join(".sigma", "settings.json"))
	return s
}

func merge(s *Settings, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var f Settings
	if json.Unmarshal(data, &f) != nil {
		return
	}
	if f.Model != "" {
		s.Model = f.Model
	}
	if f.Isolate {
		s.Isolate = true
	}
	if f.Sandbox.Enabled {
		s.Sandbox = f.Sandbox
	}
	if f.EventLog != "" {
		s.EventLog = f.EventLog
	}
	if f.PermissionMode != "" {
		s.PermissionMode = f.PermissionMode
	}
	if f.CompactAt != 0 {
		s.CompactAt = f.CompactAt
	}
	s.AllowedTools = append(s.AllowedTools, f.AllowedTools...)
	for name, srv := range f.MCPServers {
		if s.MCPServers == nil {
			s.MCPServers = map[string]MCPServer{}
		}
		s.MCPServers[name] = srv
	}
	for event, cmds := range f.Hooks {
		if s.Hooks == nil {
			s.Hooks = map[string][]string{}
		}
		s.Hooks[event] = append(s.Hooks[event], cmds...)
	}
}
