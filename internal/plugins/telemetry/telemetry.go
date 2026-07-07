// Package telemetry is a built-in plugin: a sensor that counts lifecycle events
// this session, plus a tool to read the counts back. It demonstrates a plugin
// bundling more than one contribution (a hook and a tool).
package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/plugin"
)

func init() { plugin.Register(plug{}) }

type plug struct{}

func (plug) Name() string { return "telemetry" }

func (plug) Register(h *plugin.Host, _ plugin.Config) error {
	t := &counter{counts: map[hooks.Kind]int{}}
	h.AddHook(t)            // sensor: observe every event
	h.AddTool(statsTool{t}) // tool: read the counts
	return nil
}

// counter tallies events. It never blocks (a sensor).
type counter struct {
	mu     sync.Mutex
	counts map[hooks.Kind]int
}

func (c *counter) Emit(_ context.Context, ev hooks.Event) hooks.Outcome {
	c.mu.Lock()
	c.counts[ev.Kind]++
	c.mu.Unlock()
	return hooks.Outcome{}
}

func (c *counter) render() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	kinds := make([]string, 0, len(c.counts))
	for k := range c.counts {
		kinds = append(kinds, string(k))
	}
	sort.Strings(kinds)
	var b strings.Builder
	for _, k := range kinds {
		fmt.Fprintf(&b, "%s: %d\n", k, c.counts[hooks.Kind(k)])
	}
	if b.Len() == 0 {
		return "no events yet"
	}
	return b.String()
}

type statsTool struct{ c *counter }

func (statsTool) Name() string            { return "telemetry_stats" }
func (statsTool) Description() string     { return "Report lifecycle event counts for this session." }
func (statsTool) ReadOnly() bool          { return true }
func (statsTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (s statsTool) Run(context.Context, json.RawMessage) (string, error) {
	return s.c.render(), nil
}
