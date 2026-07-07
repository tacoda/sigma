package telemetry

import (
	"context"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/plugin"
)

func TestTelemetryPlugin(t *testing.T) {
	h, err := plugin.Mount([]string{"telemetry"}, nil) // init() registered it on import
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Tools) != 1 || len(h.Hooks) != 1 {
		t.Fatalf("host has %d tools / %d hooks, want 1 / 1", len(h.Tools), len(h.Hooks))
	}

	ctx := context.Background()
	h.Hooks[0].Emit(ctx, hooks.Event{Kind: hooks.PreTool})
	h.Hooks[0].Emit(ctx, hooks.Event{Kind: hooks.PreTool})
	h.Hooks[0].Emit(ctx, hooks.Event{Kind: hooks.Stop})

	out, err := h.Tools[0].Run(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "PreToolUse: 2") || !strings.Contains(out, "Stop: 1") {
		t.Errorf("stats = %q", out)
	}
}
