package codehealth

import (
	"context"
	"strings"
	"testing"

	"github.com/tacoda/sigma/internal/hooks"
	"github.com/tacoda/sigma/internal/plugin"
)

func TestMountDefaults(t *testing.T) {
	h, err := plugin.Mount([]string{"codehealth"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Hooks) != 1 || len(h.Tools) != 1 {
		t.Fatalf("host has %d hooks / %d tools, want 1 / 1", len(h.Hooks), len(h.Tools))
	}
}

func TestGateBlocksOnFailingCheck(t *testing.T) {
	g := gate{checks: []string{"true", "exit 1"}}
	ctx := context.Background()

	// Non-Stop events pass through untouched.
	if g.Emit(ctx, hooks.Event{Kind: hooks.PreTool}).Block {
		t.Error("gate should only act on Stop")
	}

	o := g.Emit(ctx, hooks.Event{Kind: hooks.Stop})
	if !o.Block {
		t.Error("a failing check should block Stop")
	}
	if !strings.Contains(o.Reason, "✗ exit 1") {
		t.Errorf("reason should name the failing check: %q", o.Reason)
	}
}

func TestGateAllowsWhenChecksPass(t *testing.T) {
	g := gate{checks: []string{"true", "echo ok"}}
	if g.Emit(context.Background(), hooks.Event{Kind: hooks.Stop}).Block {
		t.Error("passing checks should not block")
	}
}

func TestCheckToolReports(t *testing.T) {
	out, err := checkTool{checks: []string{"true"}}.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "✓ true") {
		t.Errorf("report = %q", out)
	}
}
