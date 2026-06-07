package runtime

import (
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func TestAgentLoopToolCallCloneIsolatesPointerAndMapFields(t *testing.T) {
	startedAt := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 7, 10, 1, 0, 0, time.UTC)
	originalFinishedAt := finishedAt
	call := AgentLoopToolCall{
		ID:         "tool-runtime-cycle",
		Name:       "daemon.health",
		Status:     contracts.StatusSucceeded,
		Detail:     "health ok",
		StartedAt:  startedAt,
		FinishedAt: &finishedAt,
		Result:     map[string]any{"source": "runtime"},
		Error:      &AgentLoopToolCallError{Code: services.CodeBadRequest, Message: "warning"},
		Metadata:   map[string]any{"phase": "tool"},
	}

	cloned := call.Clone()
	mutatedFinishedAt := time.Date(2026, 6, 7, 10, 2, 0, 0, time.UTC)
	*call.FinishedAt = mutatedFinishedAt
	call.Result["source"] = "mutated"
	call.Error.Message = "mutated warning"
	call.Metadata["phase"] = "mutated"

	if cloned.ID != "tool-runtime-cycle" || cloned.Name != "daemon.health" || cloned.Status != contracts.StatusSucceeded || cloned.Detail != "health ok" || !cloned.StartedAt.Equal(startedAt) {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
	if cloned.FinishedAt == nil || !cloned.FinishedAt.Equal(originalFinishedAt) {
		t.Fatalf("clone shares or lost FinishedAt: %#v", cloned.FinishedAt)
	}
	if cloned.FinishedAt == call.FinishedAt {
		t.Fatalf("clone reused FinishedAt pointer")
	}
	if cloned.Result["source"] != "runtime" || cloned.Metadata["phase"] != "tool" {
		t.Fatalf("clone shares map fields: result=%#v metadata=%#v", cloned.Result, cloned.Metadata)
	}
	if cloned.Error == nil || cloned.Error.Code != services.CodeBadRequest || cloned.Error.Message != "warning" {
		t.Fatalf("clone shares or lost error: %#v", cloned.Error)
	}
	if cloned.Error == call.Error {
		t.Fatalf("clone reused Error pointer")
	}
}
