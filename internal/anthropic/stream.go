package anthropic

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// sseEvent is the union of the SSE event payloads we care about.
type sseEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock *Block `json:"content_block"`
	Delta        *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Message *struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// streamState accumulates blocks across SSE events.
type streamState struct {
	blocks   []Block
	inputBuf map[int]*strings.Builder
	result   Result
	onText   func(string)
}

func parseStream(body io.Reader, onText func(string)) (*Result, error) {
	s := &streamState{inputBuf: map[int]*strings.Builder{}, onText: onText}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var ev sseEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return nil, err
		}
		if ev.Type == "message_stop" {
			break
		}
		s.apply(ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	s.result.Content = s.blocks
	return &s.result, nil
}

func (s *streamState) apply(ev sseEvent) {
	switch ev.Type {
	case "message_start":
		s.applyStart(ev)
	case "content_block_start":
		s.startBlock(ev.Index, ev.ContentBlock)
	case "content_block_delta":
		s.applyDelta(ev.Index, ev.Delta)
	case "content_block_stop":
		s.stopBlock(ev.Index)
	case "message_delta":
		s.applyFinish(ev)
	}
}

func (s *streamState) applyStart(ev sseEvent) {
	if ev.Message != nil {
		s.result.Usage.InputTokens = ev.Message.Usage.InputTokens
	}
}

func (s *streamState) applyFinish(ev sseEvent) {
	if ev.Delta != nil {
		s.result.StopReason = ev.Delta.StopReason
	}
	if ev.Usage != nil {
		s.result.Usage.OutputTokens = ev.Usage.OutputTokens
	}
}

func (s *streamState) startBlock(index int, block *Block) {
	for len(s.blocks) <= index {
		s.blocks = append(s.blocks, Block{})
	}
	if block != nil {
		s.blocks[index] = *block
	}
}

func (s *streamState) applyDelta(index int, delta *struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	PartialJSON string `json:"partial_json"`
	StopReason  string `json:"stop_reason"`
}) {
	if delta == nil || index >= len(s.blocks) {
		return
	}
	switch delta.Type {
	case "text_delta":
		s.blocks[index].Text += delta.Text
		if s.onText != nil {
			s.onText(delta.Text)
		}
	case "input_json_delta":
		buf := s.inputBuf[index]
		if buf == nil {
			buf = &strings.Builder{}
			s.inputBuf[index] = buf
		}
		buf.WriteString(delta.PartialJSON)
	}
}

func (s *streamState) stopBlock(index int) {
	if index >= len(s.blocks) {
		return
	}
	if buf := s.inputBuf[index]; buf != nil {
		s.blocks[index].Input = json.RawMessage(buf.String())
	}
}
