// Package plugin is the extras layer: optional bundles that register
// contributions (tools, prompt sources, hook buses) into the agent's ports.
//
// A plugin is NOT a port — ports stay specific. A plugin composes over them: it
// bundles one or more contributions and is toggled by name in config. Built-in
// plugins self-register via init(); Mount assembles a Host from the enabled
// names, which the composition root merges into the core wiring.
package plugin

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/prompt"
	"github.com/tacoda/sigma/internal/tools"
)

// Config is a plugin's raw JSON configuration (nil if none was provided).
type Config = json.RawMessage

// Host collects what mounted plugins contribute.
type Host struct {
	Tools   []tools.Tool
	Sources []prompt.Source
	Hooks   []hooks.Bus
}

func (h *Host) AddTool(t tools.Tool)      { h.Tools = append(h.Tools, t) }
func (h *Host) AddSource(s prompt.Source) { h.Sources = append(h.Sources, s) }
func (h *Host) AddHook(b hooks.Bus)       { h.Hooks = append(h.Hooks, b) }

// Plugin is an optional bundle of contributions. Register receives the plugin's
// config (may be nil).
type Plugin interface {
	Name() string
	Register(*Host, Config) error
}

var registry = map[string]Plugin{}
var defaults []string

// Register adds a built-in plugin. Call from an init() function.
func Register(p Plugin) { registry[p.Name()] = p }

// RegisterDefault adds a plugin that mounts by default (without being listed in
// config). Call from an init() function.
func RegisterDefault(p Plugin) {
	Register(p)
	defaults = append(defaults, p.Name())
}

// Available lists the registered plugin names, sorted.
func Available() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Mount registers the named plugins into a fresh Host. An unknown name is an
// error so a typo fails loudly rather than silently disabling a layer.
func Mount(enabled, disabled []string, cfgs map[string]Config) (*Host, error) {
	off := make(map[string]bool, len(disabled))
	for _, n := range disabled {
		off[n] = true
	}
	h := &Host{}
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, defaults...), enabled...) {
		if seen[name] || off[name] {
			seen[name] = true
			continue
		}
		seen[name] = true
		p, ok := registry[name]
		if !ok {
			return nil, fmt.Errorf("unknown plugin %q (available: %v)", name, Available())
		}
		if err := p.Register(h, cfgs[name]); err != nil {
			return nil, fmt.Errorf("plugin %q: %w", name, err)
		}
	}
	return h, nil
}
