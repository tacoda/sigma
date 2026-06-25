package mcp

import (
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToSchemaFallback(t *testing.T) {
	if string(toSchema(nil)) != `{"type":"object"}` {
		t.Error("nil schema should fall back to empty object schema")
	}
	got := string(toSchema(map[string]any{"type": "object", "x": 1}))
	if got == "" || got == "null" {
		t.Errorf("valid schema marshaled to %q", got)
	}
}

func TestAdapterName(t *testing.T) {
	a := adapter{server: "github", tool: "search"}
	if a.Name() != "github__search" {
		t.Errorf("name = %q", a.Name())
	}
}

func TestTextOf(t *testing.T) {
	res := &sdk.CallToolResult{Content: []sdk.Content{
		&sdk.TextContent{Text: "one"},
		&sdk.TextContent{Text: "two"},
	}}
	if textOf(res) != "one\ntwo" {
		t.Errorf("textOf = %q", textOf(res))
	}
}
