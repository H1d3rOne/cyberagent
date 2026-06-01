package daemon

import (
	"context"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func (m *agentManager) AttachEngine(e *runtime.Engine) {
	m.mu.Lock()
	m.engine = e
	m.mu.Unlock()
}

func (m *agentManager) runtimeEngine() *runtime.Engine {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.engine
}

// Subscribe fans all engine events out as a per-session channel. Caller
// cancels the ctx to unsubscribe. Filters by the legacy session_id carried
// in runtime.Event.Data while SendMessage/tool execution is incrementally
// migrated into Engine.runTurn. When the manager has no engine, the channel
// is closed immediately.
func (m *agentManager) Subscribe(ctx context.Context, sessionID string) <-chan runtime.Event {
	return m.SubscribeSince(ctx, sessionID, 0)
}

func (m *agentManager) SubscribeSince(ctx context.Context, sessionID string, fromSeq int64) <-chan runtime.Event {
	eng := m.runtimeEngine()
	if eng == nil {
		out := make(chan runtime.Event)
		close(out)
		return out
	}
	return eng.Subscribe(ctx, runtime.EventFilter{SessionID: sessionID, FromSeq: fromSeq})
}

type agentRuntimeTurnRef struct {
	TaskID string
	RunID  string
	TurnID string
}

type agentMemoryCompaction struct {
	Summary          *agentMemorySummary
	CompactedCount   int
	RetainedCount    int
	PreviousCount    int
	LastCompactedID  string
	LastCompactedAt  *time.Time
	Trigger          string
	TriggerThreshold int
	OccurredAt       time.Time
}

func (m *agentManager) beginRuntimeTurn(ctx context.Context, sessionID, prompt, origin, operationLevel string, session agentSession, userMessage agentMessage) agentRuntimeTurnRef {
	eng := m.runtimeEngine()
	if eng == nil {
		return agentRuntimeTurnRef{}
	}
	taskID := agentRuntimeTaskID(sessionID, session)
	goal := prompt
	if session.ActiveTask != nil && strings.TrimSpace(session.ActiveTask.Goal) != "" {
		goal = session.ActiveTask.Goal
	}
	runID := strings.Replace(userMessage.ID, "msg_", "run_", 1)
	if runID == userMessage.ID {
		runID = "run_" + userMessage.ID
	}
	turnID := strings.Replace(userMessage.ID, "msg_", "turn_", 1)
	if turnID == userMessage.ID {
		turnID = "turn_" + userMessage.ID
	}
	now := time.Now().UTC()
	_, _ = eng.Submit(ctx, runtime.Task{
		ID:             taskID,
		SessionID:      sessionID,
		Origin:         firstNonEmpty(strings.TrimSpace(origin), "user"),
		Goal:           trimSummary(goal, 360),
		Prompt:         trimSummary(prompt, 1000),
		OperationLevel: operationLevel,
		Created:        now,
	})
	_, _ = eng.SubmitRun(ctx, runtime.Run{
		ID:        runID,
		TaskID:    taskID,
		State:     string(runtime.RunRunning),
		Turn:      1,
		CreatedAt: now,
	})
	eng.Publish(ctx, runtime.Event{
		Type:   "turn_started",
		TaskID: taskID,
		RunID:  runID,
		TurnID: turnID,
		Data: map[string]any{
			"session_id": sessionID,
			"message_id": userMessage.ID,
			"origin":     firstNonEmpty(strings.TrimSpace(origin), "user"),
			"prompt":     prompt,
		},
		Timestamp: now,
	})
	return agentRuntimeTurnRef{TaskID: taskID, RunID: runID, TurnID: turnID}
}

func (m *agentManager) emitAgentRuntimeEvents(ctx context.Context, eng *runtime.Engine, sessionID string, session agentSession, messages ...agentMessage) {
	if eng == nil {
		return
	}
	taskID := agentRuntimeTaskID(sessionID, session)
	for _, message := range messages {
		turnID := firstNonEmpty(strings.Replace(message.ID, "msg_", "turn_", 1), message.ID)
		runtimeStreamed := agentMessageRuntimeStreamed(message)
		if streamedTurnID := agentMessageRuntimeTurnID(message); streamedTurnID != "" {
			turnID = streamedTurnID
		}
		base := map[string]any{
			"session_id": sessionID,
			"message_id": message.ID,
			"role":       message.Role,
			"phase":      message.Phase,
		}
		if strings.TrimSpace(message.Reasoning) != "" {
			data := cloneMapStringAny(base)
			data["text"] = message.Reasoning
			eng.Publish(ctx, runtime.Event{Type: "model_reasoning", TaskID: taskID, TurnID: turnID, Data: data, Timestamp: message.Timestamp})
		}
		if message.Role == "agent" && strings.TrimSpace(message.Content) != "" && !runtimeStreamed {
			data := cloneMapStringAny(base)
			data["text"] = message.Content
			eng.Publish(ctx, runtime.Event{Type: "model_token", TaskID: taskID, TurnID: turnID, Data: data, Timestamp: message.Timestamp})
		}
		for _, call := range message.ToolCalls {
			toolTurnID := firstNonEmpty(agentToolCallRuntimeTurnID(call), turnID)
			toolData := cloneMapStringAny(base)
			toolData["tool_call"] = call
			toolData["tool"] = call.Name
			toolData["status"] = string(call.Status)
			eng.Publish(ctx, runtime.Event{Type: "tool_started", TaskID: taskID, TurnID: toolTurnID, Data: toolData, Timestamp: call.StartedAt})
			if call.FinishedAt != nil || call.Status == contracts.StatusSucceeded || call.Status == contracts.StatusFailed || call.Status == contracts.StatusCancelled {
				finishedData := cloneMapStringAny(toolData)
				finishedData["result"] = call.Result
				if call.Error != nil {
					finishedData["error"] = call.Error
				}
				ts := time.Now().UTC()
				if call.FinishedAt != nil {
					ts = *call.FinishedAt
				}
				eng.Publish(ctx, runtime.Event{Type: "tool_finished", TaskID: taskID, TurnID: toolTurnID, Data: finishedData, Timestamp: ts})
			}
		}
	}
	if len(session.Todos) > 0 {
		eng.Publish(ctx, runtime.Event{
			Type:   "todo_updated",
			TaskID: taskID,
			Data: map[string]any{
				"session_id": sessionID,
				"todos":      cloneAgentTodos(session.Todos),
			},
			Timestamp: time.Now().UTC(),
		})
	}
	if session.PendingAuthorization != nil && strings.EqualFold(session.PendingAuthorization.Status, "pending") {
		eng.Publish(ctx, runtime.Event{
			Type:   "approval_required",
			TaskID: taskID,
			Data: map[string]any{
				"session_id":     sessionID,
				"approval_kind":  "agent_authorization",
				"authorization":  cloneAgentAuthorizationRequest(session.PendingAuthorization),
				"approval_id":    session.PendingAuthorization.ID,
				"action_name":    session.PendingAuthorization.ActionName,
				"required_level": session.PendingAuthorization.RequiredLevel,
				"reason":         session.PendingAuthorization.Reason,
			},
			Timestamp: session.PendingAuthorization.CreatedAt,
		})
	}
}

func (m *agentManager) emitRuntimeApprovalResolved(ctx context.Context, sessionID string, session agentSession, auth agentAuthorizationRequest, decision string) {
	eng := m.runtimeEngine()
	if eng == nil {
		return
	}
	eng.Publish(ctx, runtime.Event{
		Type:   "approval_resolved",
		TaskID: agentRuntimeTaskID(sessionID, session),
		Data: map[string]any{
			"session_id":     sessionID,
			"approval_kind":  "agent_authorization",
			"approval_id":    auth.ID,
			"action_name":    auth.ActionName,
			"decision":       decision,
			"required_level": auth.RequiredLevel,
		},
		Timestamp: time.Now().UTC(),
	})
}

func (m *agentManager) emitRuntimeMemoryCompacted(ctx context.Context, sessionID string, session agentSession, turnRef agentRuntimeTurnRef, compaction *agentMemoryCompaction) {
	eng := m.runtimeEngine()
	if eng == nil || compaction == nil {
		return
	}
	taskID := firstNonEmpty(turnRef.TaskID, agentRuntimeTaskID(sessionID, session))
	eng.Publish(ctx, runtime.Event{
		Type:   "memory_compacted",
		TaskID: taskID,
		RunID:  turnRef.RunID,
		TurnID: turnRef.TurnID,
		Data: map[string]any{
			"session_id":        sessionID,
			"compacted_count":   compaction.CompactedCount,
			"retained_count":    compaction.RetainedCount,
			"previous_count":    compaction.PreviousCount,
			"memory_count":      compaction.Summary.MessageCount,
			"last_compacted_id": compaction.LastCompactedID,
			"trigger":           compaction.Trigger,
			"trigger_threshold": compaction.TriggerThreshold,
			"memory_summary":    cloneAgentMemorySummary(compaction.Summary),
		},
		Timestamp: compaction.OccurredAt,
	})
}

func (m *agentManager) finishRuntimeTurn(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, resp sendAgentMessageResponse, err error) {
	eng := m.runtimeEngine()
	if eng == nil || turnRef.RunID == "" || turnRef.TurnID == "" {
		return
	}
	status, reason := agentRuntimeTurnCompletion(resp, err)
	data := map[string]any{
		"session_id": sessionID,
		"status":     status,
	}
	if reason != "" {
		data["reason"] = reason
	}
	if err != nil {
		data["error"] = err.Error()
		eng.Publish(ctx, runtime.Event{
			Type:      "turn_failed",
			TaskID:    turnRef.TaskID,
			RunID:     turnRef.RunID,
			TurnID:    turnRef.TurnID,
			Data:      data,
			Timestamp: time.Now().UTC(),
		})
		_ = eng.FinishRun(ctx, turnRef.RunID, false)
		return
	}

	eng.Publish(ctx, runtime.Event{
		Type:      "turn_finished",
		TaskID:    turnRef.TaskID,
		RunID:     turnRef.RunID,
		TurnID:    turnRef.TurnID,
		Data:      data,
		Timestamp: time.Now().UTC(),
	})
	switch status {
	case "approval_required":
		_ = eng.RequestApproval(turnRef.RunID, reason)
	case "finished":
		_ = eng.FinishRun(ctx, turnRef.RunID, true)
	case "failed":
		_ = eng.FinishRun(ctx, turnRef.RunID, false)
	}
}

func agentRuntimeTurnCompletion(resp sendAgentMessageResponse, err error) (string, string) {
	if err != nil {
		return "failed", err.Error()
	}
	if resp.PendingAuthorization != nil && strings.EqualFold(resp.PendingAuthorization.Status, "pending") {
		return "approval_required", firstNonEmpty(resp.PendingAuthorization.Reason, resp.PendingAuthorization.ActionName)
	}
	if resp.NextAction != nil {
		switch strings.ToLower(strings.TrimSpace(resp.NextAction.Type)) {
		case "approval_required", "authorization_required", "waiting_authorization":
			return "approval_required", firstNonEmpty(resp.NextAction.Reason, resp.NextAction.SuggestedPrompt)
		case "finished", "done", "complete", "completed":
			return "finished", resp.NextAction.Reason
		case "blocked", "failed", "error", "cancelled", "canceled":
			return "failed", resp.NextAction.Reason
		}
	}
	if resp.ActiveTask != nil {
		switch strings.ToLower(strings.TrimSpace(resp.ActiveTask.Status)) {
		case "done", "finished", "succeeded", "completed":
			return "finished", resp.ActiveTask.Summary
		case "failed", "error", "cancelled", "canceled":
			return "failed", resp.ActiveTask.Summary
		case "blocked":
			return "blocked", resp.ActiveTask.Summary
		}
	}
	for i := len(resp.Messages) - 1; i >= 0; i-- {
		if resp.Messages[i].Role == "agent" && agentNormalizePhase(resp.Messages[i].Phase) == "final_answer" {
			return "finished", ""
		}
	}
	return "running", ""
}

func agentMessageRuntimeStreamed(message agentMessage) bool {
	for _, call := range message.ToolCalls {
		if agentToolCallRuntimeStreamed(call) {
			return true
		}
	}
	return false
}

func agentToolCallRuntimeStreamed(call agentToolCall) bool {
	if call.Metadata == nil {
		return false
	}
	streamed, _ := call.Metadata["runtime_streamed"].(bool)
	return streamed
}

func agentMessageRuntimeTurnID(message agentMessage) string {
	for _, call := range message.ToolCalls {
		if turnID := agentToolCallRuntimeTurnID(call); turnID != "" {
			return turnID
		}
	}
	return ""
}

func agentToolCallRuntimeTurnID(call agentToolCall) string {
	if call.Metadata == nil {
		return ""
	}
	turnID, _ := call.Metadata["runtime_turn_id"].(string)
	return strings.TrimSpace(turnID)
}

func agentRuntimeTaskID(sessionID string, session agentSession) string {
	if session.ActiveTask != nil && strings.TrimSpace(session.ActiveTask.ID) != "" {
		return session.ActiveTask.ID
	}
	return "task_" + sessionID
}
