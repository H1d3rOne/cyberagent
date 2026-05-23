package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type openaiStreamRequest struct {
	openaiRequest
	Stream bool `json:"stream"`
}

func (p *OpenAIProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		req.Model = p.config.Model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}

	body := p.buildRequestBody(req)
	streamBody := openaiStreamRequest{openaiRequest: body, Stream: true}
	encoded, err := json.Marshal(streamBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("openai stream error %d", resp.StatusCode)
	}

	ch := make(chan StreamEvent, 16)
	go p.readSSE(ctx, resp, ch)
	return ch, nil
}

func (p *OpenAIProvider) readSSE(ctx context.Context, resp *http.Response, ch chan<- StreamEvent) {
	defer resp.Body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	type toolCallDelta struct {
		id      string
		name    string
		argsBuf strings.Builder
	}
	toolCalls := make(map[int]*toolCallDelta)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{Type: "error", Error: ctx.Err(), Done: true}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   *string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			ch <- StreamEvent{Type: "text_delta", TextDelta: *choice.Delta.Content}
		}

		for _, tc := range choice.Delta.ToolCalls {
			delta, ok := toolCalls[tc.Index]
			if !ok {
				delta = &toolCallDelta{}
				toolCalls[tc.Index] = delta
			}
			if tc.ID != "" {
				delta.id = tc.ID
			}
			if tc.Function.Name != "" {
				delta.name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				delta.argsBuf.WriteString(tc.Function.Arguments)
			}
		}

		if choice.FinishReason != nil {
			for _, delta := range toolCalls {
				if delta.name != "" {
					ch <- StreamEvent{
						Type: "tool_use",
						Block: ToolUseBlock{
							ID:    delta.id,
							Name:  delta.name,
							Input: json.RawMessage(delta.argsBuf.String()),
						},
					}
				}
			}

			stopReason := StopReasonEndTurn
			switch *choice.FinishReason {
			case "tool_calls":
				stopReason = StopReasonToolUse
			case "length":
				stopReason = StopReasonMaxTokens
			}

			var usage Usage
			if chunk.Usage != nil {
				usage = Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}
			}
			ch <- StreamEvent{Type: "done", Done: true, StopReason: stopReason, Usage: usage}
		}
	}
}
