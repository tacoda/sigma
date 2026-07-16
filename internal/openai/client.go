// Package openai is a minimal client for the Chat Completions API,
// authenticated with an API key from the environment.
//
// It is an adapter: it converts sigma's provider-neutral message model (see
// internal/message) to and from the OpenAI wire format and streams responses
// over SSE. It satisfies the LLM port consumed by the agent.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tacoda/sigma/internal/message"
)

const endpoint = "https://api.openai.com/v1/chat/completions"

// Client calls the Chat Completions API with an API key.
type Client struct {
	key  string
	http *http.Client
}

// New returns a client bound to an OpenAI API key.
func New(key string) *Client {
	return &Client{key: key, http: &http.Client{Timeout: 300 * time.Second}}
}

type wireFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFuncCall `json:"function"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireRequest struct {
	Model         string            `json:"model"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
	Messages      []wireMessage     `json:"messages"`
	Tools         []wireTool        `json:"tools,omitempty"`
	Stream        bool              `json:"stream"`
	StreamOptions wireStreamOptions `json:"stream_options"`
}

// wireOf converts a request to the wire form. The optional System string
// becomes a leading system message; content blocks are flattened into OpenAI's
// role model (tool_result blocks become their own role:"tool" messages).
func wireOf(req message.Request) wireRequest {
	msgs := make([]wireMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, wireMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, convert(m)...)
	}

	tools := make([]wireTool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = wireTool{Type: "function", Function: wireFunction{
			Name: t.Name, Description: t.Description, Parameters: t.InputSchema,
		}}
	}

	return wireRequest{
		Model:         req.Model,
		MaxTokens:     req.MaxTokens,
		Messages:      msgs,
		Tools:         tools,
		Stream:        true,
		StreamOptions: wireStreamOptions{IncludeUsage: true},
	}
}

// convert splits one provider-neutral message into OpenAI wire messages. Text
// and tool_use blocks fold into a single message with the original role;
// tool_result blocks each become a separate role:"tool" message.
func convert(m message.Message) []wireMessage {
	var text string
	var calls []wireToolCall
	var results []wireMessage
	for _, b := range m.Content {
		switch b.Type {
		case "text":
			text += b.Text
		case "tool_use":
			args := string(b.Input)
			if args == "" {
				args = "{}"
			}
			calls = append(calls, wireToolCall{ID: b.ID, Type: "function",
				Function: wireFuncCall{Name: b.Name, Arguments: args}})
		case "tool_result":
			results = append(results, wireMessage{Role: "tool",
				ToolCallID: b.ToolUseID, Content: b.Content})
		}
	}
	var out []wireMessage
	if text != "" || len(calls) > 0 {
		out = append(out, wireMessage{Role: m.Role, Content: text, ToolCalls: calls})
	}
	return append(out, results...)
}

// Complete sends a request and returns the full response (streamed internally,
// no incremental callback).
func (c *Client) Complete(ctx context.Context, req message.Request) (*message.Result, error) {
	return c.Stream(ctx, req, nil)
}

// Stream sends a request and accumulates the SSE response. onText, if non-nil,
// receives text deltas as they arrive.
func (c *Client) Stream(ctx context.Context, req message.Request, onText func(string)) (*message.Result, error) {
	resp, err := c.post(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, data)
	}
	return parseStream(resp.Body, onText)
}

func (c *Client) post(ctx context.Context, req message.Request) (*http.Response, error) {
	body, err := json.Marshal(wireOf(req))
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("authorization", "Bearer "+c.key)
	return c.http.Do(httpReq)
}
