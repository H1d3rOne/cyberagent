package runtime

import (
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

// AgentLoopToolCall is the runtime-neutral representation of a tool call
// carried by agent loop messages and top-level cycle state. Daemon-specific
// session structs convert to and from this DTO at legacy boundaries.
type AgentLoopToolCall struct {
	ID         string
	Name       string
	Status     contracts.Status
	Detail     string
	StartedAt  time.Time
	FinishedAt *time.Time
	Result     map[string]any
	Error      *AgentLoopToolCallError
	Metadata   map[string]any
}

// AgentLoopToolCallError is the runtime-neutral tool call error envelope.
type AgentLoopToolCallError struct {
	Code    services.ErrorCode
	Message string
}

// Clone returns a deep-enough copy for runtime loop DTO boundaries. Maps are
// cloned at the top level only, matching the daemon legacy tool call semantics.
func (call AgentLoopToolCall) Clone() AgentLoopToolCall {
	cloned := call
	cloned.FinishedAt = CloneAgentLoopTimePtr(call.FinishedAt)
	cloned.Result = CloneAgentLoopMapStringAny(call.Result)
	cloned.Error = CloneAgentLoopToolCallError(call.Error)
	cloned.Metadata = CloneAgentLoopMapStringAny(call.Metadata)
	return cloned
}

// CloneAgentLoopToolCalls clones tool call slices and their pointer/map fields.
func CloneAgentLoopToolCalls(calls []AgentLoopToolCall) []AgentLoopToolCall {
	if calls == nil {
		return nil
	}
	out := make([]AgentLoopToolCall, len(calls))
	for i, call := range calls {
		out[i] = call.Clone()
	}
	return out
}

// CloneAgentLoopToolCallError clones an optional tool call error.
func CloneAgentLoopToolCallError(err *AgentLoopToolCallError) *AgentLoopToolCallError {
	if err == nil {
		return nil
	}
	cloned := *err
	return &cloned
}

// CloneAgentLoopTimePtr clones an optional time pointer used by runtime loop DTOs.
func CloneAgentLoopTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// CloneAgentLoopMapStringAny clones a top-level string-keyed map.
func CloneAgentLoopMapStringAny(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
