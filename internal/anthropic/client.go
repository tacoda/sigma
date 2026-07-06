// Package anthropic is a minimal client for the Messages API, authenticated
// with a Claude Code subscription OAuth token (no API key).
//
// It is an adapter: it converts sigma's provider-neutral message model (see
// internal/message) to and from the Anthropic wire format and streams responses
// over SSE. It satisfies the LLM port consumed by the agent.
package anthropic

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

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type wireRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	System    []textBlock       `json:"system"`
	Messages  []message.Message `json:"messages"`
	Tools     []message.Tool    `json:"tools,omitempty"`
	Stream    bool              `json:"stream"`
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
