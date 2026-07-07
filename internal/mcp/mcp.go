// Package mcp connects to external Model Context Protocol servers and adapts
// their tools into the agent's tool registry.
//
// Each remote tool is namespaced as "<server>__<tool>". MCP tools are treated
// as mutating (gated), since the protocol does not guarantee they are safe.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tacoda/sigma/internal/config"
	"github.com/tacoda/sigma/internal/tools"
)

// Client holds open sessions; Close shuts them all down.
type Client struct {
	sessions []*sdk.ClientSession
}

// Close ends all MCP sessions.
func (c *Client) Close() {
	for _, s := range c.sessions {
		_ = s.Close()
	}
}

// Connect dials every configured server and returns the discovered tools plus a
// Client to close when done. Servers that fail to connect are logged and
// skipped, never fatal.
func Connect(ctx context.Context, servers map[string]config.MCPServer) (*Client, []tools.Tool) {
	c := &Client{}
	var discovered []tools.Tool
	for name, spec := range servers {
		session, err := dial(ctx, spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mcp: skip %q: %v\n", name, err)
			continue
		}
		c.sessions = append(c.sessions, session)
		discovered = append(discovered, listTools(ctx, name, session)...)
		discovered = append(discovered, listResources(ctx, name, session)...)
		discovered = append(discovered, listPrompts(ctx, name, session)...)
	}
	return c, discovered
}

func dial(ctx context.Context, spec config.MCPServer) (*sdk.ClientSession, error) {
	client := sdk.NewClient(&sdk.Implementation{Name: "sigma", Version: "0.0.1"}, nil)

	var transport sdk.Transport
	switch {
	case spec.URL != "":
		transport = &sdk.StreamableClientTransport{Endpoint: spec.URL}
	case spec.Command != "":
		cmd := exec.Command(spec.Command, spec.Args...)
		cmd.Env = append(os.Environ(), envSlice(spec.Env)...)
		transport = &sdk.CommandTransport{Command: cmd}
	default:
		return nil, errors.New("server has neither command nor url")
	}
	return client.Connect(ctx, transport, nil)
}

func listTools(ctx context.Context, server string, session *sdk.ClientSession) []tools.Tool {
	res, err := session.ListTools(ctx, &sdk.ListToolsParams{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp: list tools for %q: %v\n", server, err)
		return nil
	}
	out := make([]tools.Tool, 0, len(res.Tools))
	for _, t := range res.Tools {
		out = append(out, adapter{
			session: session,
			server:  server,
			tool:    t.Name,
			desc:    t.Description,
			schema:  toSchema(t.InputSchema),
		})
	}
	return out
}

func envSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// toSchema marshals the remote JSON Schema, falling back to an empty object
// schema (Anthropic requires an object schema).
func toSchema(s any) json.RawMessage {
	if s == nil {
		return json.RawMessage(`{"type":"object"}`)
	}
	data, err := json.Marshal(s)
	if err != nil || isEmptyJSON(data) {
		return json.RawMessage(`{"type":"object"}`)
	}
	return data
}

func isEmptyJSON(data []byte) bool {
	return len(data) == 0 || string(data) == "null"
}

// adapter exposes one remote MCP tool through the agent's tool interface.
type adapter struct {
	session *sdk.ClientSession
	server  string
	tool    string
	desc    string
	schema  json.RawMessage
}

func (a adapter) Name() string            { return a.server + "__" + a.tool }
func (a adapter) Description() string     { return a.desc }
func (a adapter) Schema() json.RawMessage { return a.schema }
func (a adapter) ReadOnly() bool          { return false }

func (a adapter) Run(ctx context.Context, input json.RawMessage) (string, error) {
	var args map[string]any
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return "", err
		}
	}
	res, err := a.session.CallTool(ctx, &sdk.CallToolParams{Name: a.tool, Arguments: args})
	if err != nil {
		return "", err
	}
	text := textOf(res)
	if res.IsError {
		return text, fmt.Errorf("mcp tool %q reported an error", a.tool)
	}
	return text, nil
}

func textOf(res *sdk.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		parts = append(parts, contentText(c))
	}
	return strings.Join(nonEmpty(parts), "\n")
}

func contentText(c sdk.Content) string {
	if tc, ok := c.(*sdk.TextContent); ok {
		return tc.Text
	}
	return ""
}

func nonEmpty(ss []string) []string {
	out := ss[:0]
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// --- resources ---

// listResources exposes a read_resource tool if the server advertises any
// resources; the tool's description enumerates the available URIs.
func listResources(ctx context.Context, server string, session *sdk.ClientSession) []tools.Tool {
	res, err := session.ListResources(ctx, &sdk.ListResourcesParams{})
	if err != nil || len(res.Resources) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Read a resource from the %q MCP server by uri. Available resources:\n", server)
	for _, r := range res.Resources {
		fmt.Fprintf(&b, "- %s: %s\n", r.URI, r.Description)
	}
	return []tools.Tool{resourceTool{session: session, server: server, desc: b.String()}}
}

type resourceTool struct {
	session *sdk.ClientSession
	server  string
	desc    string
}

func (r resourceTool) Name() string        { return r.server + "__read_resource" }
func (r resourceTool) Description() string { return r.desc }
func (r resourceTool) ReadOnly() bool      { return true }
func (resourceTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"uri":{"type":"string"}},"required":["uri"]}`)
}

func (r resourceTool) Run(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.URI == "" {
		return "", fmt.Errorf("uri is required")
	}
	res, err := r.session.ReadResource(ctx, &sdk.ReadResourceParams{URI: args.URI})
	if err != nil {
		return "", err
	}
	return renderResource(res.Contents), nil
}

func renderResource(contents []*sdk.ResourceContents) string {
	var parts []string
	for _, c := range contents {
		switch {
		case c.Text != "":
			parts = append(parts, c.Text)
		case len(c.Blob) > 0:
			parts = append(parts, fmt.Sprintf("[binary resource %s, %d bytes]", c.URI, len(c.Blob)))
		}
	}
	return strings.Join(parts, "\n")
}

// --- prompts ---

// listPrompts exposes a prompt tool if the server advertises any prompts.
func listPrompts(ctx context.Context, server string, session *sdk.ClientSession) []tools.Tool {
	res, err := session.ListPrompts(ctx, &sdk.ListPromptsParams{})
	if err != nil || len(res.Prompts) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Render a prompt template from the %q MCP server by name. Available prompts:\n", server)
	for _, p := range res.Prompts {
		fmt.Fprintf(&b, "- %s: %s\n", p.Name, p.Description)
	}
	return []tools.Tool{promptTool{session: session, server: server, desc: b.String()}}
}

type promptTool struct {
	session *sdk.ClientSession
	server  string
	desc    string
}

func (p promptTool) Name() string        { return p.server + "__prompt" }
func (p promptTool) Description() string { return p.desc }
func (p promptTool) ReadOnly() bool      { return true }
func (promptTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"arguments":{"type":"object"}},"required":["name"]}`)
}

func (p promptTool) Run(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	res, err := p.session.GetPrompt(ctx, &sdk.GetPromptParams{Name: args.Name, Arguments: args.Arguments})
	if err != nil {
		return "", err
	}
	return renderPrompt(res.Messages), nil
}

func renderPrompt(msgs []*sdk.PromptMessage) string {
	var parts []string
	for _, m := range msgs {
		if text := contentText(m.Content); text != "" {
			parts = append(parts, string(m.Role)+": "+text)
		}
	}
	return strings.Join(parts, "\n\n")
}
