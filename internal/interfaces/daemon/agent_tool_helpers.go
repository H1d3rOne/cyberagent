package daemon

import (
	"net/url"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

func runningAgentToolCall(name, detail string) agentToolCall {
	now := time.Now().UTC()
	return withAgentToolMetadata(agentToolCall{
		ID:        strings.Replace(runs.NewJobID(now), "job_", "tool_", 1),
		Name:      name,
		Status:    contracts.StatusRunning,
		Detail:    detail,
		StartedAt: now,
	}, nil)
}

func withAgentToolMetadata(tool agentToolCall, metadata map[string]interface{}) agentToolCall {
	merged := agentToolDefaultMetadata(tool.Name)
	for key, value := range tool.Metadata {
		merged[key] = value
	}
	for key, value := range metadata {
		merged[key] = value
	}
	tool.Metadata = merged
	return tool
}

func agentToolDefaultMetadata(name string) map[string]interface{} {
	normalized := strings.ToLower(strings.TrimSpace(name))
	metadata := map[string]interface{}{
		"phase":             "tool_call",
		"risk":              "low",
		"auto_run":          true,
		"approval_required": false,
		"approval_policy":   "auto_low_risk_builtin",
	}
	switch {
	case strings.Contains(normalized, "llm chat"):
		metadata["phase"] = "planner"
		metadata["approval_policy"] = "planner_provider_call"
	case strings.Contains(normalized, "llm synthesis"):
		metadata["phase"] = "tool_synthesis"
		metadata["approval_policy"] = "synthesize_observations"
	case strings.Contains(normalized, "scans/web"):
		metadata["phase"] = "daemon_job"
		metadata["risk"] = "medium"
		metadata["auto_run"] = false
		metadata["approval_policy"] = "explicit_user_instruction"
	case strings.Contains(normalized, "http.request"):
		metadata["phase"] = "http_probe"
		metadata["risk"] = "medium"
		metadata["auto_run"] = false
		metadata["approval_policy"] = "explicit_authorized_target"
	case strings.Contains(normalized, "jobs/{job_id}/cancel"):
		metadata["phase"] = "control"
		metadata["risk"] = "medium"
		metadata["auto_run"] = false
		metadata["approval_policy"] = "explicit_user_control"
	case strings.Contains(normalized, "resume-approved"):
		metadata["phase"] = "approval_resume"
		metadata["risk"] = "medium"
		metadata["auto_run"] = false
		metadata["approval_policy"] = "explicit_user_instruction"
	case strings.Contains(normalized, "/approvals/") && (strings.Contains(normalized, "/approve") || strings.Contains(normalized, "/deny")):
		metadata["phase"] = "approval_decision"
		metadata["risk"] = "medium"
		metadata["auto_run"] = false
		metadata["approval_policy"] = "explicit_user_instruction"
	case strings.Contains(normalized, "approvals"):
		metadata["phase"] = "approval_check"
		metadata["approval_policy"] = "read_approval_queue"
	case strings.Contains(normalized, "plugins"):
		metadata["phase"] = "inventory"
	case strings.Contains(normalized, "runs"):
		metadata["phase"] = "history"
	case strings.Contains(normalized, "context"):
		metadata["phase"] = "context"
	case strings.Contains(normalized, "workspace"):
		metadata["phase"] = "workspace"
	}
	return metadata
}

func finishAgentToolCall(tool agentToolCall, err error) agentToolCall {
	now := time.Now().UTC()
	tool.FinishedAt = &now
	if err != nil {
		tool.Status = contracts.StatusFailed
		return tool
	}
	tool.Status = contracts.StatusSucceeded
	return tool
}

func failedAgentToolCall(name string, err error) agentToolCall {
	tool := runningAgentToolCall(name, "")
	tool = finishAgentToolCall(tool, err)
	tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
	return tool
}

func buildAgentSendResponse(sessionID string, messages []agentMessage, toolCalls []agentToolCall, session agentSession) sendAgentMessageResponse {
	return sendAgentMessageResponse{
		SessionID:            sessionID,
		Messages:             cloneAgentMessages(messages),
		ToolCalls:            cloneAgentToolCalls(toolCalls),
		ActiveTask:           cloneAgentTask(session.ActiveTask),
		Todos:                cloneAgentTodos(session.Todos),
		ContextRefs:          cloneAgentContextRefs(session.ContextRefs),
		PendingChanges:       cloneAgentWorkspaceChanges(session.PendingChanges),
		PendingAuthorization: cloneAgentAuthorizationRequest(session.PendingAuthorization),
		MemorySummary:        cloneAgentMemorySummary(session.MemorySummary),
		NextAction:           cloneAgentNextAction(session.NextAction),
		UIState:              deriveAgentUIState(&session),
	}
}

func boolFromAny(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y", "all":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func workspaceChangeInputsFromAny(value any) []agentWorkspaceChangeInput {
	switch v := value.(type) {
	case []agentWorkspaceChangeInput:
		return append([]agentWorkspaceChangeInput(nil), v...)
	case []map[string]any:
		out := make([]agentWorkspaceChangeInput, 0, len(v))
		for _, item := range v {
			out = append(out, agentWorkspaceChangeInput{
				Path: firstNonEmpty(stringFromAny(item["path"]), stringFromAny(item["file"]), stringFromAny(item["filename"])),
				Old:  firstNonEmpty(stringFromAny(item["old"]), stringFromAny(item["old_text"]), stringFromAny(item["before"]), stringFromAny(item["search"])),
				New:  firstNonEmpty(stringFromAny(item["new"]), stringFromAny(item["new_text"]), stringFromAny(item["after"]), stringFromAny(item["replace"])),
			})
		}
		return out
	case []any:
		out := make([]agentWorkspaceChangeInput, 0, len(v))
		for _, raw := range v {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, agentWorkspaceChangeInput{
				Path: firstNonEmpty(stringFromAny(item["path"]), stringFromAny(item["file"]), stringFromAny(item["filename"])),
				Old:  firstNonEmpty(stringFromAny(item["old"]), stringFromAny(item["old_text"]), stringFromAny(item["before"]), stringFromAny(item["search"])),
				New:  firstNonEmpty(stringFromAny(item["new"]), stringFromAny(item["new_text"]), stringFromAny(item["after"]), stringFromAny(item["replace"])),
			})
		}
		return out
	default:
		return nil
	}
}

func emptyString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func trimSummary(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}

func cloneAgentSession(session *agentSession) agentSession {
	if session == nil {
		return agentSession{}
	}
	cloned := *session
	cloned.Messages = cloneAgentMessages(session.Messages)
	cloned.ActiveTask = cloneAgentTask(session.ActiveTask)
	cloned.Todos = cloneAgentTodos(session.Todos)
	cloned.ToolCalls = cloneAgentToolCalls(session.ToolCalls)
	cloned.ContextRefs = cloneAgentContextRefs(session.ContextRefs)
	cloned.PendingChanges = cloneAgentWorkspaceChanges(session.PendingChanges)
	cloned.PendingAuthorization = cloneAgentAuthorizationRequest(session.PendingAuthorization)
	cloned.MemorySummary = cloneAgentMemorySummary(session.MemorySummary)
	cloned.NextAction = cloneAgentNextAction(session.NextAction)
	cloned.RunSummary = cloneAgentRunSummary(session.RunSummary)
	cloned.UIState = deriveAgentUIState(&cloned)
	return cloned
}

func cloneAgentAuthorizationRequest(request *agentAuthorizationRequest) *agentAuthorizationRequest {
	if request == nil {
		return nil
	}
	cloned := *request
	cloned.Arguments = cloneMapStringAny(request.Arguments)
	return &cloned
}

func cloneMapStringAny(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneAgentMemorySummary(summary *agentMemorySummary) *agentMemorySummary {
	if summary == nil {
		return nil
	}
	cloned := *summary
	if summary.LastMessageAt != nil {
		last := *summary.LastMessageAt
		cloned.LastMessageAt = &last
	}
	return &cloned
}

func cloneAgentRunSummary(summary *agentSessionRunSummary) *agentSessionRunSummary {
	if summary == nil {
		return nil
	}
	cloned := *summary
	return &cloned
}

func cloneAgentTask(task *agentTask) *agentTask {
	if task == nil {
		return nil
	}
	cloned := *task
	return &cloned
}

func cloneAgentTodos(todos []agentTodo) []agentTodo {
	if todos == nil {
		return nil
	}
	cloned := make([]agentTodo, len(todos))
	copy(cloned, todos)
	for i := range todos {
		if todos[i].ContextRefIDs != nil {
			cloned[i].ContextRefIDs = append([]string(nil), todos[i].ContextRefIDs...)
		}
	}
	return cloned
}

func cloneAgentContextRefs(refs []agentContextRef) []agentContextRef {
	if refs == nil {
		return nil
	}
	cloned := make([]agentContextRef, len(refs))
	copy(cloned, refs)
	return cloned
}

func cloneAgentWorkspaceChanges(changes []agentWorkspaceChange) []agentWorkspaceChange {
	if changes == nil {
		return nil
	}
	cloned := make([]agentWorkspaceChange, len(changes))
	copy(cloned, changes)
	for i := range changes {
		if changes[i].AppliedAt != nil {
			applied := *changes[i].AppliedAt
			cloned[i].AppliedAt = &applied
		}
	}
	return cloned
}

func cloneAgentNextAction(action *agentNextAction) *agentNextAction {
	if action == nil {
		return nil
	}
	cloned := *action
	return &cloned
}

func cloneAgentMessages(messages []agentMessage) []agentMessage {
	if messages == nil {
		return nil
	}
	cloned := make([]agentMessage, len(messages))
	for i, message := range messages {
		cloned[i] = message
		cloned[i].ToolCalls = cloneAgentToolCalls(message.ToolCalls)
		cloned[i].Links = cloneAgentLinks(message.Links)
	}
	return cloned
}

func cloneAgentToolCalls(calls []agentToolCall) []agentToolCall {
	if calls == nil {
		return nil
	}
	cloned := make([]agentToolCall, len(calls))
	copy(cloned, calls)
	for i := range calls {
		if calls[i].FinishedAt != nil {
			finished := *calls[i].FinishedAt
			cloned[i].FinishedAt = &finished
		}
		if calls[i].Result != nil {
			cloned[i].Result = map[string]any{}
			for key, value := range calls[i].Result {
				cloned[i].Result[key] = value
			}
		}
		if calls[i].Metadata != nil {
			cloned[i].Metadata = map[string]interface{}{}
			for key, value := range calls[i].Metadata {
				cloned[i].Metadata[key] = value
			}
		}
		if calls[i].Error != nil {
			cloned[i].Error = &daemonJobError{Code: calls[i].Error.Code, Message: calls[i].Error.Message}
		}
	}
	return cloned
}

func cloneAgentLinks(links []agentLink) []agentLink {
	if links == nil {
		return nil
	}
	cloned := make([]agentLink, len(links))
	copy(cloned, links)
	return cloned
}

func agentHostLabel(target string) string {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" {
		return target
	}
	return parsed.Host
}

func shortAgentID(value string, size int) string {
	if len(value) <= size+4 {
		return value
	}
	return value[:size] + "…"
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func agentExternalToolPhrase(language string, allow bool) string {
	if allow {
		return agentText(language, "按策略启用插件", "enable plugins by policy")
	}
	return agentText(language, "默认禁用外部工具", "disable external tools by default")
}
