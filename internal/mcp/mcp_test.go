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

func TestResourceAndPromptToolNames(t *testing.T) {
	if (resourceTool{server: "docs"}).Name() != "docs__read_resource" {
		t.Error("resource tool name")
	}
	if (promptTool{server: "docs"}).Name() != "docs__prompt" {
		t.Error("prompt tool name")
	}
	// Both are reads and should bypass the permission gate.
	if !(resourceTool{}).ReadOnly() || !(promptTool{}).ReadOnly() {
		t.Error("resource/prompt tools should be read-only")
	}
}

func TestRenderResource(t *testing.T) {
	got := renderResource([]*sdk.ResourceContents{
		{URI: "file://a", Text: "hello"},
		{URI: "file://b", Blob: []byte{1, 2, 3}},
	})
	if got != "hello\n[binary resource file://b, 3 bytes]" {
		t.Errorf("renderResource = %q", got)
	}
}

func TestRenderPrompt(t *testing.T) {
	got := renderPrompt([]*sdk.PromptMessage{
		{Role: "user", Content: &sdk.TextContent{Text: "do X"}},
		{Role: "assistant", Content: &sdk.TextContent{Text: "ok"}},
	})
	if got != "user: do X\n\nassistant: ok" {
		t.Errorf("renderPrompt = %q", got)
	}
}
