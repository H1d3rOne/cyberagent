package runtime

import "time"

// AgentLoopMemorySummary is the runtime-neutral compacted session memory used
// by bounded LLM, tool execution, and session mutation stages.
type AgentLoopMemorySummary struct {
	Summary       string
	MessageCount  int
	LastMessageID string
	LastMessageAt *time.Time
	UpdatedAt     time.Time
}

// Clone returns a copy with pointer fields isolated.
func (summary AgentLoopMemorySummary) Clone() AgentLoopMemorySummary {
	cloned := summary
	if summary.LastMessageAt != nil {
		last := *summary.LastMessageAt
		cloned.LastMessageAt = &last
	}
	return cloned
}

// CloneAgentLoopMemorySummary clones an optional memory summary.
func CloneAgentLoopMemorySummary(summary *AgentLoopMemorySummary) *AgentLoopMemorySummary {
	if summary == nil {
		return nil
	}
	cloned := summary.Clone()
	return &cloned
}
