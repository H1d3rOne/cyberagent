package runtime

import (
	"testing"
	"time"
)

func TestAgentLoopMemorySummaryCloneIsolatesPointerFields(t *testing.T) {
	lastMessageAt := time.Date(2026, 6, 7, 10, 30, 0, 0, time.UTC)
	originalLastMessageAt := lastMessageAt
	updatedAt := time.Date(2026, 6, 7, 10, 35, 0, 0, time.UTC)
	summary := AgentLoopMemorySummary{
		Summary:       "memory",
		MessageCount:  4,
		LastMessageID: "msg-4",
		LastMessageAt: &lastMessageAt,
		UpdatedAt:     updatedAt,
	}

	cloned := summary.Clone()
	*summary.LastMessageAt = lastMessageAt.Add(time.Hour)

	if cloned.Summary != "memory" || cloned.MessageCount != 4 || cloned.LastMessageID != "msg-4" || !cloned.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
	if cloned.LastMessageAt == nil || !cloned.LastMessageAt.Equal(originalLastMessageAt) {
		t.Fatalf("clone shares or lost LastMessageAt: %#v", cloned.LastMessageAt)
	}
	if cloned.LastMessageAt == summary.LastMessageAt {
		t.Fatalf("clone reused LastMessageAt pointer")
	}
}
