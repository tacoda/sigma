// Package message defines the content-block conversation model sigma's agent
// speaks: messages composed of text, tool_use, and tool_result blocks, plus the
// request/result values the LLM port exchanges.
//
// It is domain data — no transport, HTTP, or provider concern lives here. The
// anthropic adapter (and any future provider) converts between these types and
// the wire.
package message

import "encoding/json"

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

// Usage reports token counts for a response.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Request is one LLM call. System, if set, is additional context an adapter may
// combine with its own required preamble.
type Request struct {
	Model     string
	MaxTokens int
	System    string
	Messages  []Message
	Tools     []Tool
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
