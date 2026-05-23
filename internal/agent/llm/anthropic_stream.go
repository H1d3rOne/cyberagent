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

type anthropicStreamRequest struct {
	anthropicRequest
	Stream bool `json:"stream"`
}

func (p *AnthropicProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if req.Model == "" {
		req.Model = p.config.Model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}

	body := p.buildRequestBody(req)
	streamBody := anthropicStreamRequest{anthropicRequest: body, Stream: true}
	encoded, err := json.Marshal(streamBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic stream error %d", resp.StatusCode)
	}

	ch := make(chan StreamEvent, 16)
	go p.readSSE(ctx, resp, ch)
	return ch, nil
}

func (p *AnthropicProvider) readSSE(ctx context.Context, resp *http.Response, ch chan<- StreamEvent) {
	defer resp.Body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentToolID, currentToolName string
	var toolInputBuf strings.Builder
	var usage Usage

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

		var event struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Message struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "message_start":
			usage.InputTokens = event.Message.Usage.InputTokens

		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				currentToolID = event.ContentBlock.ID
				currentToolName = event.ContentBlock.Name
				toolInputBuf.Reset()
			}

		case "content_block_delta":
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				ch <- StreamEvent{Type: "text_delta", TextDelta: event.Delta.Text}
			} else if event.Delta.Type == "input_json_delta" && event.Delta.PartialJSON != "" {
				toolInputBuf.WriteString(event.Delta.PartialJSON)
			}

		case "content_block_stop":
			if currentToolName != "" {
				ch <- StreamEvent{
					Type: "tool_use",
					Block: ToolUseBlock{
						ID:    currentToolID,
						Name:  currentToolName,
						Input: json.RawMessage(toolInputBuf.String()),
					},
				}
				currentToolID = ""
				currentToolName = ""
				toolInputBuf.Reset()
			}

		case "message_delta":
			usage.OutputTokens = event.Usage.OutputTokens
			if event.Delta.StopReason != "" {
				ch <- StreamEvent{
					Type:       "done",
					Done:       true,
					StopReason: StopReason(event.Delta.StopReason),
					Usage:      usage,
				}
			}
		}
	}
}
