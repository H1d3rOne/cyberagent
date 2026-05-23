package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

type SessionState struct {
	Target   string              `json:"target"`
	Scope    []string            `json:"scope"`
	History  []sessionMessage    `json:"history"`
	Findings []contracts.Finding `json:"findings"`
	Turns    int                 `json:"turns"`
	SavedAt  time.Time           `json:"saved_at"`
}

type sessionMessage struct {
	Role   string         `json:"role"`
	Blocks []sessionBlock `json:"blocks"`
}

type sessionBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

func (a *Agent) SaveSession(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create session dir: %w", err)
	}

	state := SessionState{
		Target:   a.config.Target,
		Scope:    a.config.Scope,
		History:  marshalHistory(a.history),
		Findings: a.findings,
		Turns:    len(a.history) / 2,
		SavedAt:  time.Now().UTC(),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal session: %w", err)
	}

	path := filepath.Join(dir, "session.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write session: %w", err)
	}
	return path, nil
}

func (a *Agent) LoadSession(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read session: %w", err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("unmarshal session: %w", err)
	}

	a.history = unmarshalHistory(state.History)
	a.findings = state.Findings
	return nil
}

func marshalHistory(msgs []llm.Message) []sessionMessage {
	out := make([]sessionMessage, 0, len(msgs))
	for _, msg := range msgs {
		sm := sessionMessage{Role: string(msg.Role)}
		for _, b := range msg.Content {
			switch block := b.(type) {
			case llm.TextBlock:
				sm.Blocks = append(sm.Blocks, sessionBlock{Type: "text", Text: block.Text})
			case llm.ToolUseBlock:
				sm.Blocks = append(sm.Blocks, sessionBlock{Type: "tool_use", ID: block.ID, Name: block.Name, Input: block.Input})
			case llm.ToolResultBlock:
				sm.Blocks = append(sm.Blocks, sessionBlock{Type: "tool_result", ToolUseID: block.ToolUseID, Content: block.Content, IsError: block.IsError})
			}
		}
		out = append(out, sm)
	}
	return out
}

func unmarshalHistory(msgs []sessionMessage) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, sm := range msgs {
		msg := llm.Message{Role: llm.Role(sm.Role)}
		for _, sb := range sm.Blocks {
			switch sb.Type {
			case "text":
				msg.Content = append(msg.Content, llm.TextBlock{Text: sb.Text})
			case "tool_use":
				msg.Content = append(msg.Content, llm.ToolUseBlock{ID: sb.ID, Name: sb.Name, Input: sb.Input})
			case "tool_result":
				msg.Content = append(msg.Content, llm.ToolResultBlock{ToolUseID: sb.ToolUseID, Content: sb.Content, IsError: sb.IsError})
			}
		}
		out = append(out, msg)
	}
	return out
}
