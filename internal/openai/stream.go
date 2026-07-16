package openai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tacoda/sigma/internal/message"
)

// chunk is the union of the streamed chat.completion.chunk fields we care about.
type chunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// toolAcc accumulates one streamed tool call across chunks.
type toolAcc struct {
	id   string
	name string
	args strings.Builder
}

func parseStream(body io.Reader, onText func(string)) (*message.Result, error) {
	var text strings.Builder
	tools := map[int]*toolAcc{}
	var order []int
	var res message.Result

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var c chunk
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			return nil, err
		}
		if c.Error != nil {
			return nil, fmt.Errorf("stream error (%s): %s", c.Error.Type, c.Error.Message)
		}
		if c.Usage != nil {
			res.Usage.InputTokens = c.Usage.PromptTokens
			res.Usage.OutputTokens = c.Usage.CompletionTokens
		}
		for _, ch := range c.Choices {
			if ch.FinishReason != "" {
				res.StopReason = stopReason(ch.FinishReason)
			}
			if ch.Delta.Content != "" {
				text.WriteString(ch.Delta.Content)
				if onText != nil {
					onText(ch.Delta.Content)
				}
			}
			for _, tc := range ch.Delta.ToolCalls {
				acc := tools[tc.Index]
				if acc == nil {
					acc = &toolAcc{}
					tools[tc.Index] = acc
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				acc.args.WriteString(tc.Function.Arguments)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	res.Content = blocks(text.String(), tools, order)
	return &res, nil
}

// stopReason maps an OpenAI finish_reason to the agent's expected vocabulary;
// only "tool_use" is significant to the agent loop.
func stopReason(finish string) string {
	if finish == "tool_calls" {
		return "tool_use"
	}
	return "end_turn"
}

func blocks(text string, tools map[int]*toolAcc, order []int) []message.Block {
	var out []message.Block
	if text != "" {
		out = append(out, message.Block{Type: "text", Text: text})
	}
	for _, idx := range order {
		acc := tools[idx]
		args := acc.args.String()
		if args == "" {
			args = "{}"
		}
		out = append(out, message.Block{
			Type:  "tool_use",
			ID:    acc.id,
			Name:  acc.name,
			Input: json.RawMessage(args),
		})
	}
	return out
}
