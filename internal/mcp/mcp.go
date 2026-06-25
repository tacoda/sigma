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
		if tc, ok := c.(*sdk.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}
