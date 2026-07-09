package dryrun

import (
	"context"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/agent"
	"github.com/tacoda/sigma/internal/plugin"
)

func TestDryRunLayerSuppressesMutations(t *testing.T) {
	h, err := plugin.Mount([]string{"dryrun"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.ToolLayers) != 1 || h.ToolLayers[0].Name != "dry-run" {
		t.Fatalf("expected one dry-run tool layer, got %+v", h.ToolLayers)
	}

	ran := false
	core := agent.InvokerFunc(func(context.Context, agent.ToolCall) (string, error) {
		ran = true
		return "real", nil
	})
	inv := h.ToolLayers[0].Layer(core)

	// Mutating tool is suppressed (core not reached).
	out, _ := inv.Invoke(context.Background(), agent.ToolCall{Name: "write_file"})
	if ran || !strings.Contains(out, "[dry-run]") {
		t.Errorf("write_file should be dry-run skipped: ran=%v out=%q", ran, out)
	}
	// Read-only tool passes through to core.
	ran = false
	if out, _ := inv.Invoke(context.Background(), agent.ToolCall{Name: "read_file"}); !ran || out != "real" {
		t.Errorf("read_file should reach core: ran=%v out=%q", ran, out)
	}
}
