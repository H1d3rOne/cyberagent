package daemon

import (
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

const (
	agentUIStateIdle        = "idle"
	agentUIStateRunning     = "running"
	agentUIStateFailed      = "failed"
	agentUIStateDone        = "done"
	agentUIStateApproval    = "approval"
	agentUIStateToolRunning = "tool_running"
)

type agentUIState struct {
	State          string `json:"state"`
	Phase          string `json:"phase,omitempty"`
	Tone           string `json:"tone"`
	Busy           bool   `json:"busy"`
	RequiresAction bool   `json:"requires_action,omitempty"`
	HasError       bool   `json:"has_error,omitempty"`
	Source         string `json:"source,omitempty"`
	Message        string `json:"message,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	ToolCallID     string `json:"tool_call_id,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	ApprovalID     string `json:"approval_id,omitempty"`
}

func deriveAgentUIState(session *agentSession) *agentUIState {
	if session == nil {
		return newAgentUIState(agentUIStateIdle, "idle", "neutral", false, "session")
	}

	if auth := session.PendingAuthorization; auth != nil && strings.EqualFold(strings.TrimSpace(auth.Status), "pending") {
		state := newAgentUIState(agentUIStateApproval, "approval", "warning", false, "pending_authorization")
		state.RequiresAction = true
		state.ApprovalID = auth.ID
		state.Message = firstNonEmpty(auth.Reason, auth.ActionName)
		if session.ActiveTask != nil {
			state.TaskID = session.ActiveTask.ID
		}
		return state
	}
	if action := session.NextAction; action != nil && agentUINextActionRequiresApproval(action.Type) {
		state := newAgentUIState(agentUIStateApproval, "approval", "warning", false, "next_action")
		state.RequiresAction = true
		state.Message = firstNonEmpty(action.Reason, action.SuggestedPrompt)
		if session.ActiveTask != nil {
			state.TaskID = session.ActiveTask.ID
		}
		return state
	}

	if tool := latestAgentUIToolCall(session.ToolCalls, agentUIToolStatusRunning); tool != nil {
		state := newAgentUIState(agentUIStateToolRunning, firstNonEmpty(agentUIToolPhase(*tool), "tool_call"), "info", true, "tool_call")
		state.ToolCallID = tool.ID
		state.ToolName = tool.Name
		state.Message = firstNonEmpty(tool.Detail, tool.Name)
		if session.ActiveTask != nil {
			state.TaskID = session.ActiveTask.ID
		}
		return state
	}

	if agentUISessionFailed(session) {
		state := newAgentUIState(agentUIStateFailed, firstNonEmpty(agentUIFailurePhase(session), "task"), "danger", false, agentUIFailureSource(session))
		state.HasError = true
		state.Message = agentUIFailureMessage(session)
		if tool := latestAgentUIToolCall(session.ToolCalls, agentUIToolStatusFailed); tool != nil {
			state.ToolCallID = tool.ID
			state.ToolName = tool.Name
		}
		if session.ActiveTask != nil {
			state.TaskID = session.ActiveTask.ID
		}
		return state
	}

	if agentUISessionDone(session) {
		state := newAgentUIState(agentUIStateDone, "final_answer", "success", false, "active_task")
		if session.ActiveTask != nil {
			state.TaskID = session.ActiveTask.ID
			state.Message = firstNonEmpty(session.ActiveTask.Summary, session.ActiveTask.Goal)
		}
		return state
	}

	if agentUISessionRunning(session) {
		state := newAgentUIState(agentUIStateRunning, "task", "info", true, "session")
		if session.ActiveTask != nil {
			state.TaskID = session.ActiveTask.ID
			state.Message = firstNonEmpty(session.ActiveTask.Summary, session.ActiveTask.Goal)
		}
		return state
	}

	state := newAgentUIState(agentUIStateIdle, "idle", "neutral", false, "session")
	if session.NextAction != nil {
		state.Message = firstNonEmpty(session.NextAction.Reason, session.NextAction.SuggestedPrompt)
	}
	return state
}

func newAgentUIState(state, phase, tone string, busy bool, source string) *agentUIState {
	return &agentUIState{State: state, Phase: phase, Tone: tone, Busy: busy, Source: source}
}

func agentUINextActionRequiresApproval(actionType string) bool {
	switch strings.ToLower(strings.TrimSpace(actionType)) {
	case "approval", "approval_required", "authorization_required", "waiting_authorization", "wait_authorization":
		return true
	default:
		return false
	}
}

func agentUISessionFailed(session *agentSession) bool {
	if session == nil {
		return false
	}
	if agentUIStatusFailed(session.Status) {
		return true
	}
	if session.ActiveTask != nil && agentUIStatusFailed(session.ActiveTask.Status) {
		return true
	}
	if session.NextAction != nil && agentUIStatusFailed(session.NextAction.Type) {
		return true
	}
	return latestAgentUIToolCall(session.ToolCalls, agentUIToolStatusFailed) != nil
}

func agentUISessionDone(session *agentSession) bool {
	if session == nil {
		return false
	}
	if session.ActiveTask != nil && agentUIStatusDone(session.ActiveTask.Status) {
		return true
	}
	if agentUIStatusDone(session.Status) {
		return true
	}
	return false
}

func agentUISessionRunning(session *agentSession) bool {
	if session == nil {
		return false
	}
	if agentUIStatusRunning(session.Status) {
		return true
	}
	if session.ActiveTask != nil && agentUIStatusRunning(session.ActiveTask.Status) {
		return true
	}
	for _, todo := range session.Todos {
		if agentUIStatusRunning(todo.Status) {
			return true
		}
	}
	return false
}

func agentUIStatusRunning(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "created", "queued", "pending", "planning", "ready", "running", "in_progress", "executing", "streaming":
		return true
	default:
		return false
	}
}

func agentUIStatusFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "failure", "error", "blocked", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func agentUIStatusDone(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "succeeded", "success", "completed", "complete", "finished":
		return true
	default:
		return false
	}
}

func latestAgentUIToolCall(calls []agentToolCall, match func(contracts.Status) bool) *agentToolCall {
	for i := len(calls) - 1; i >= 0; i-- {
		if match(calls[i].Status) {
			return &calls[i]
		}
	}
	return nil
}

func agentUIToolStatusRunning(status contracts.Status) bool {
	return agentUIStatusRunning(string(status)) || status == contracts.StatusWaitingApproval || status == contracts.StatusPaused
}

func agentUIToolStatusFailed(status contracts.Status) bool {
	return agentUIStatusFailed(string(status))
}

func agentUIToolPhase(tool agentToolCall) string {
	if tool.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(stringFromAny(tool.Metadata["phase"]))
}

func agentUIFailurePhase(session *agentSession) string {
	if tool := latestAgentUIToolCall(session.ToolCalls, agentUIToolStatusFailed); tool != nil {
		return firstNonEmpty(agentUIToolPhase(*tool), "tool_call")
	}
	if session.NextAction != nil && agentUIStatusFailed(session.NextAction.Type) {
		return "task"
	}
	if session.ActiveTask != nil && agentUIStatusFailed(session.ActiveTask.Status) {
		return "task"
	}
	return "session"
}

func agentUIFailureSource(session *agentSession) string {
	if latestAgentUIToolCall(session.ToolCalls, agentUIToolStatusFailed) != nil {
		return "tool_call"
	}
	if session.NextAction != nil && agentUIStatusFailed(session.NextAction.Type) {
		return "next_action"
	}
	if session.ActiveTask != nil && agentUIStatusFailed(session.ActiveTask.Status) {
		return "active_task"
	}
	return "session"
}

func agentUIFailureMessage(session *agentSession) string {
	if tool := latestAgentUIToolCall(session.ToolCalls, agentUIToolStatusFailed); tool != nil {
		return firstNonEmpty(agentToolError(*tool), tool.Detail, tool.Name)
	}
	if session.NextAction != nil {
		return firstNonEmpty(session.NextAction.Reason, session.NextAction.SuggestedPrompt)
	}
	if session.ActiveTask != nil {
		return firstNonEmpty(session.ActiveTask.Summary, session.ActiveTask.Goal)
	}
	return ""
}
