// Package anthropic is a minimal client for the Messages API, authenticated
// with a Claude Code subscription OAuth token (no API key).
//
// It speaks the content-block model (text, tool_use, tool_result) and streams
// responses over SSE.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	endpoint    = "https://api.anthropic.com/v1/messages"
	apiVersion  = "2023-06-01"
	oauthBeta   = "oauth-2025-04-20"
	cliIdentity = "You are Claude Code, Anthropic's official CLI for Claude."
)

// Client calls the Messages API with a bearer token.
type Client struct {
	token string
	http  *http.Client
}

// New returns a client bound to an OAuth access token.
func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 300 * time.Second}}
}

// Block is one content block. Fields are populated by type:
// text -> Text; tool_use -> ID, Name, Input; tool_result -> ToolUseID, Content.
type Block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// Message is one conversation turn.
type Message struct {
	Role    string  `json:"role"`
	Content []Block `json:"content"`
}

// UserText builds a plain user message.
func UserText(text string) Message {
	return Message{Role: "user", Content: []Block{{Type: "text", Text: text}}}
}

// Tool is a tool definition advertised to the model.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Request is a Messages API call. The required CLI identity is always sent as
// the first system block; System, if set, is appended as additional context.
type Request struct {
	Model     string
	MaxTokens int
	System    string
	Messages  []Message
	Tools     []Tool
}

// Usage reports token counts for a response.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Result is an accumulated (possibly streamed) response.
type Result struct {
	Content    []Block
	StopReason string
	Usage      Usage
}

// Text concatenates all text blocks.
func (r *Result) Text() string {
	var s string
	for _, b := range r.Content {
		if b.Type == "text" {
			s += b.Text
		}
	}
	return s
}

// ToolUses returns the tool_use blocks in order.
func (r *Result) ToolUses() []Block {
	var out []Block
	for _, b := range r.Content {
		if b.Type == "tool_use" {
			out = append(out, b)
		}
	}
	return out
}

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type wireRequest struct {
	Model     string      `json:"model"`
	MaxTokens int         `json:"max_tokens"`
	System    []textBlock `json:"system"`
	Messages  []Message   `json:"messages"`
	Tools     []Tool      `json:"tools,omitempty"`
	Stream    bool        `json:"stream"`
}

// Complete sends a request and returns the full response (streamed internally,
// no incremental callback).
func (c *Client) Complete(ctx context.Context, req Request) (*Result, error) {
	return c.Stream(ctx, req, nil)
}

// Stream sends a request and accumulates the SSE response. onText, if non-nil,
// receives text deltas as they arrive.
func (c *Client) Stream(ctx context.Context, req Request, onText func(string)) (*Result, error) {
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

func (c *Client) post(ctx context.Context, req Request) (*http.Response, error) {
	system := []textBlock{{Type: "text", Text: cliIdentity}}
	if req.System != "" {
		system = append(system, textBlock{Type: "text", Text: req.System})
	}
	body, err := json.Marshal(wireRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    system,
		Messages:  req.Messages,
		Tools:     req.Tools,
		Stream:    true,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("anthropic-version", apiVersion)
	httpReq.Header.Set("anthropic-beta", oauthBeta)
	httpReq.Header.Set("authorization", "Bearer "+c.token)
	return c.http.Do(httpReq)
}
