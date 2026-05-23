package plugins

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// SubprocessRequest is the MVP JSON request sent to subprocess_jsonl plugins on stdin.
type SubprocessRequest struct {
	ID        string               `json:"id"`
	JobID     string               `json:"job_id"`
	StepID    string               `json:"step_id"`
	Target    string               `json:"target,omitempty"`
	Scope     contracts.Scope      `json:"scope,omitempty"`
	Input     map[string]any       `json:"input,omitempty"`
	Artifacts []contracts.Artifact `json:"artifacts,omitempty"`
	Evidence  []contracts.Evidence `json:"evidence,omitempty"`
}

// SubprocessMessage is one JSON or JSONL response message emitted by a subprocess plugin.
type SubprocessMessage struct {
	Type      string          `json:"type,omitempty"`
	RequestID string          `json:"request,omitempty"`
	Routes    []PluginRoute   `json:"routes,omitempty"`
	Findings  []PluginFinding `json:"findings,omitempty"`
	Data      map[string]any  `json:"data,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// PluginRoute is a normalized route emitted by analyzer plugins.
type PluginRoute struct {
	Raw      string         `json:"raw,omitempty"`
	Resolved string         `json:"resolved,omitempty"`
	Source   string         `json:"source,omitempty"`
	Method   string         `json:"method,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// EncodeSubprocessRequest marshals a plugin request as stdin JSON.
func EncodeSubprocessRequest(request SubprocessRequest) ([]byte, error) {
	if strings.TrimSpace(request.ID) == "" {
		return nil, fmt.Errorf("subprocess request id is required")
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal subprocess request: %w", err)
	}
	return append(encoded, '\n'), nil
}

// DecodeSubprocessMessages decodes stdout containing either one JSON object or JSONL objects.
func DecodeSubprocessMessages(stdout []byte) ([]SubprocessMessage, error) {
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed == "" {
		return nil, nil
	}
	var messages []SubprocessMessage
	for lineNo, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var message SubprocessMessage
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			return nil, fmt.Errorf("decode subprocess message line %d: %w", lineNo+1, err)
		}
		if message.Data == nil {
			var raw map[string]any
			if err := json.Unmarshal([]byte(line), &raw); err == nil {
				message.Data = raw
			}
		}
		messages = append(messages, message)
	}
	return messages, nil
}
