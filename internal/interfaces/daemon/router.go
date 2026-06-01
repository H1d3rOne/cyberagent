package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/command"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
)

type routerServices struct {
	version     services.VersionService
	plugins     services.PluginService
	runs        services.RunService
	approvals   services.ApprovalService
	jobs        *jobManager
	agent       *agentManager
	scanManager *web.ScanManager
	schedules   *scheduleManager
	providers   web.ProviderStore
}

func newRouter(svc routerServices, token string) http.Handler {
	if svc.jobs == nil {
		svc.jobs = newJobManager(jobManagerConfig{})
	}
	if svc.agent == nil {
		svc.agent = newAgentManager(svc.runs.RunsDir)
	}
	providerStoreProvided := svc.providers != nil
	if svc.providers == nil {
		svc.providers = newProviderStore("")
	}
	if providerStoreProvided {
		svc.agent.AttachProviderStore(svc.providers)
	}
	mux := http.NewServeMux()

	// Register /api/scans endpoints from the web API server
	if svc.scanManager != nil {
		scanAPI := web.NewAPIServer(svc.scanManager, svc.providers)
		mux.Handle("/api/scans", scanAPI.Handler())
		mux.Handle("/api/scans/", scanAPI.Handler())
		mux.Handle("/api/providers", scanAPI.Handler())
		mux.Handle("/api/provider-profiles", scanAPI.Handler())
		mux.Handle("/api/provider-profiles/", scanAPI.Handler())
		mux.Handle("/api/tools", scanAPI.Handler())
	}

	mux.HandleFunc("/api/v1/healthz", requireMethod(http.MethodGet, handleHealthz))
	mux.HandleFunc("/api/v1/version", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, http.StatusOK, svc.version.GetVersion(r.Context()))
	}))
	mux.HandleFunc("/api/v1/doctor", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, http.StatusOK, svc.version.Doctor(r.Context()))
	}))
	mux.HandleFunc("/api/v1/plugins", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		plugins, err := svc.plugins.ListPlugins(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{"plugins": plugins})
	}))
	mux.HandleFunc("/api/v1/plugins/doctor", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		report, err := svc.plugins.Doctor(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, report)
	}))
	mux.HandleFunc("/api/v1/agent/capabilities", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, http.StatusOK, agentCapabilities(r.URL.Query().Get("language")))
	}))
	mux.HandleFunc("/api/v1/agent/skills", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, http.StatusOK, agentSkillRegistry(r.URL.Query().Get("language")))
	}))
	mux.HandleFunc("/api/v1/agent/mcp", func(w http.ResponseWriter, r *http.Request) {
		handleAgentMCPRoute(w, r, svc)
	})
	mux.HandleFunc("/api/v1/agent/mcp/", func(w http.ResponseWriter, r *http.Request) {
		handleAgentMCPRoute(w, r, svc)
	})
	mux.HandleFunc("/api/v1/agent/sessions", func(w http.ResponseWriter, r *http.Request) {
		handleAgentSessionsRoute(w, r, svc)
	})
	mux.HandleFunc("/api/v1/agent/sessions/", func(w http.ResponseWriter, r *http.Request) {
		handleAgentSessionsRoute(w, r, svc)
	})
	mux.HandleFunc("/api/v1/runs", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		limit, err := optionalInt(r.URL.Query().Get("limit"))
		if err != nil {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "invalid limit", map[string]any{"field": "limit"}, err))
			return
		}
		page, err := svc.runs.ListRuns(r.Context(), services.RunListRequest{
			Cursor: r.URL.Query().Get("cursor"),
			Limit:  limit,
			Status: contracts.Status(strings.TrimSpace(r.URL.Query().Get("status"))),
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, page)
	}))
	mux.HandleFunc("/api/v1/runs/", func(w http.ResponseWriter, r *http.Request) {
		handleRunRoute(w, r, svc)
	})
	mux.HandleFunc("/api/v1/scans/web", func(w http.ResponseWriter, r *http.Request) {
		handleScanWebRoute(w, r, svc)
	})
	mux.HandleFunc("/api/v1/jobs/", func(w http.ResponseWriter, r *http.Request) {
		handleJobRoute(w, r, svc)
	})
	mux.HandleFunc("/api/v1/schedules", func(w http.ResponseWriter, r *http.Request) {
		handleScheduleRoute(w, r, svc)
	})
	mux.HandleFunc("/api/v1/schedules/", func(w http.ResponseWriter, r *http.Request) {
		handleScheduleRoute(w, r, svc)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Serve frontend for non-API routes
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			web.FrontendHandler().ServeHTTP(w, r)
			return
		}
		writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
	})
	// Frontend routes don't need auth; API routes do
	mainMux := http.NewServeMux()
	mainMux.Handle("/api/", corsMiddleware(bearerAuth(token, mux)))
	mainMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		web.FrontendHandler().ServeHTTP(w, r)
	})
	return mainMux
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "cyberagent-daemon",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func requireMethod(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		next(w, r)
	}
}

func handleAgentMCPRoute(w http.ResponseWriter, r *http.Request, svc routerServices) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/agent/mcp")
	path = strings.Trim(path, "/")
	switch {
	case path == "":
		if r.Method != http.MethodGet {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		view, err := svc.agent.ListMCPRegistry(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, view)
		return
	case path == "servers":
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body agentMCPServerConfig
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		for idx := range body.Tools {
			if !body.Tools[idx].Enabled {
				body.Tools[idx].Enabled = true
			}
		}
		server, err := svc.agent.AddMCPServer(r.Context(), body)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusCreated, map[string]any{"server": server})
		return
	case path == "discover":
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		response, err := svc.agent.DiscoverMCPTools(r.Context(), r.URL.Query().Get("server_id"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, response)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) == 3 && parts[0] == "servers" {
		serverID := parts[1]
		action := parts[2]
		switch {
		case action == "enable" && r.Method == http.MethodPost:
			server, err := svc.agent.SetMCPServerEnabled(r.Context(), serverID, true)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, map[string]any{"server": server})
			return
		case action == "disable" && r.Method == http.MethodPost:
			server, err := svc.agent.SetMCPServerEnabled(r.Context(), serverID, false)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, map[string]any{"server": server})
			return
		case action == "discover" && r.Method == http.MethodPost:
			response, err := svc.agent.DiscoverMCPTools(r.Context(), serverID)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, response)
			return
		}
	}
	if len(parts) == 2 && parts[0] == "servers" && r.Method == http.MethodDelete {
		server, err := svc.agent.DeleteMCPServer(r.Context(), parts[1])
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{"server": server, "status": "deleted"})
		return
	}
	writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
}

func handleAgentSessionsRoute(w http.ResponseWriter, r *http.Request, svc routerServices) {
	if r.URL.Path == "/api/v1/agent/sessions" {
		switch r.Method {
		case http.MethodGet:
			limit, err := optionalInt(r.URL.Query().Get("limit"))
			if err != nil {
				writeServiceError(w, services.NewError(services.CodeBadRequest, "invalid limit", map[string]any{"field": "limit"}, err))
				return
			}
			response, err := svc.agent.ListSessions(r.Context(), limit)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, response)
			return
		case http.MethodPost:
			var body createAgentSessionRequest
			if err := decodeJSONBody(r, &body); err != nil {
				writeServiceError(w, err)
				return
			}
			response, err := svc.agent.CreateSession(body)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusCreated, response)
			return
		default:
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
	}

	sessionID, resource, ok := parseAgentSessionPath(r.URL.Path)
	if !ok {
		writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
		return
	}

	switch r.Method {
	case http.MethodGet:
		if resource == "stream" {
			handleAgentSessionStream(w, r, svc, sessionID)
			return
		}
		session, err := svc.agent.GetSession(r.Context(), sessionID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		switch resource {
		case "":
			writeOK(w, http.StatusOK, map[string]any{"session": session})
		case "state":
			writeOK(w, http.StatusOK, agentSessionStatePayload(r, svc, session))
		case "context":
			writeOK(w, http.StatusOK, agentSessionContextPayload(r, svc, session))
		case "trace":
			writeOK(w, http.StatusOK, agentSessionTracePayload(r, svc, session))
		default:
			writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
		}
	case http.MethodDelete:
		if resource != "" {
			writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
			return
		}
		response, err := svc.agent.DeleteSession(r.Context(), sessionID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, response)
	case http.MethodPost:
		switch resource {
		case "messages":
			var body sendAgentMessageRequest
			if err := decodeJSONBody(r, &body); err != nil {
				writeServiceError(w, err)
				return
			}
			response, err := svc.agent.SendMessage(r.Context(), sessionID, body, svc)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, response)
		case "control":
			var body agentControlRequest
			if err := decodeJSONBody(r, &body); err != nil {
				writeServiceError(w, err)
				return
			}
			response, err := svc.agent.ControlSession(r.Context(), sessionID, body, svc)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, response)
		case "continue", "resume", "pause", "stop", "cancel", "new-task", "new_task":
			var body agentControlRequest
			if err := decodeJSONBody(r, &body); err != nil {
				writeServiceError(w, err)
				return
			}
			body.Action = resource
			response, err := svc.agent.ControlSession(r.Context(), sessionID, body, svc)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, response)
		case "authorization", "authorization/approve", "authorization/deny":
			var body agentAuthorizationDecisionRequest
			if err := decodeJSONBody(r, &body); err != nil {
				writeServiceError(w, err)
				return
			}
			if resource == "authorization/approve" {
				body.Action = "approve"
			} else if resource == "authorization/deny" {
				body.Action = "deny"
			}
			response, err := svc.agent.DecideAgentAuthorization(r.Context(), sessionID, body, svc)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, response)
		case "workspace":
			handleWorkspaceRoute(w, r, svc, sessionID)
		default:
			writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
		}
	default:
		writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
	}
}

func agentSessionStatePayload(r *http.Request, svc routerServices, session agentSession) map[string]any {
	return map[string]any{
		"session":               session,
		"session_id":            session.SessionID,
		"status":                session.Status,
		"active_task":           cloneAgentTask(session.ActiveTask),
		"todos":                 cloneAgentTodos(session.Todos),
		"tool_calls":            cloneAgentToolCalls(session.ToolCalls),
		"context_refs":          cloneAgentContextRefs(session.ContextRefs),
		"pending_changes":       cloneAgentWorkspaceChanges(session.PendingChanges),
		"pending_authorization": cloneAgentAuthorizationRequest(session.PendingAuthorization),
		"memory_summary":        cloneAgentMemorySummary(session.MemorySummary),
		"next_action":           cloneAgentNextAction(session.NextAction),
		"run_summary":           svc.agent.agentSessionRunSummary(r.Context(), &session),
	}
}

func agentSessionContextPayload(r *http.Request, svc routerServices, session agentSession) map[string]any {
	return map[string]any{
		"session_id":            session.SessionID,
		"active_task":           cloneAgentTask(session.ActiveTask),
		"context_refs":          cloneAgentContextRefs(session.ContextRefs),
		"pending_changes":       cloneAgentWorkspaceChanges(session.PendingChanges),
		"pending_authorization": cloneAgentAuthorizationRequest(session.PendingAuthorization),
		"memory_summary":        cloneAgentMemorySummary(session.MemorySummary),
		"run_summary":           svc.agent.agentSessionRunSummary(r.Context(), &session),
	}
}

func agentSessionTracePayload(r *http.Request, svc routerServices, session agentSession) map[string]any {
	timeline := agentSessionTraceTimeline(session)
	phaseCounts := map[string]int{}
	for _, item := range timeline {
		if phase, ok := item["phase"].(string); ok && strings.TrimSpace(phase) != "" {
			phaseCounts[phase]++
		}
	}
	return map[string]any{
		"trace_version":         "agent.trace.v1",
		"generated_at":          time.Now().UTC(),
		"session_id":            session.SessionID,
		"operator":              session.Operator,
		"language":              session.Language,
		"operation_level":       session.OperationLevel,
		"status":                session.Status,
		"created_at":            session.CreatedAt,
		"updated_at":            session.UpdatedAt,
		"active_task":           cloneAgentTask(session.ActiveTask),
		"todos":                 cloneAgentTodos(session.Todos),
		"context_refs":          cloneAgentContextRefs(session.ContextRefs),
		"pending_changes":       cloneAgentWorkspaceChanges(session.PendingChanges),
		"pending_authorization": cloneAgentAuthorizationRequest(session.PendingAuthorization),
		"memory_summary":        cloneAgentMemorySummary(session.MemorySummary),
		"next_action":           cloneAgentNextAction(session.NextAction),
		"run_summary":           svc.agent.agentSessionRunSummary(r.Context(), &session),
		"messages":              cloneAgentMessages(session.Messages),
		"tool_calls":            cloneAgentToolCalls(session.ToolCalls),
		"timeline":              timeline,
		"summary": map[string]any{
			"message_count":        len(session.Messages),
			"tool_call_count":      len(session.ToolCalls),
			"context_ref_count":    len(session.ContextRefs),
			"pending_change_count": len(session.PendingChanges),
			"phase_counts":         phaseCounts,
		},
		"replay_hints": []string{
			"Replay timeline items in timestamp order.",
			"Message items contain the user/agent/tool chat stream.",
			"Tool items contain tool name, status, metadata, result, and errors.",
			"Use active_task, todos, context_refs, pending_changes, and next_action to restore cockpit state.",
		},
	}
}

func agentSessionTraceTimeline(session agentSession) []map[string]any {
	items := make([]map[string]any, 0, len(session.Messages)+len(session.ToolCalls))
	for _, message := range session.Messages {
		item := map[string]any{
			"kind":       "message",
			"id":         message.ID,
			"role":       message.Role,
			"phase":      message.Phase,
			"content":    message.Content,
			"timestamp":  message.Timestamp,
			"tool_calls": cloneAgentToolCalls(message.ToolCalls),
			"links":      cloneAgentLinks(message.Links),
		}
		if strings.TrimSpace(message.Reasoning) != "" {
			item["reasoning"] = message.Reasoning
		}
		items = append(items, item)
	}
	for _, call := range session.ToolCalls {
		phase := ""
		if call.Metadata != nil {
			phase = firstNonEmpty(stringFromAny(call.Metadata["agent_phase"]), stringFromAny(call.Metadata["phase"]))
		}
		item := map[string]any{
			"kind":      "tool_call",
			"id":        call.ID,
			"name":      call.Name,
			"status":    call.Status,
			"detail":    call.Detail,
			"phase":     phase,
			"timestamp": call.StartedAt,
			"tool_call": cloneAgentToolCalls([]agentToolCall{call})[0],
		}
		if call.FinishedAt != nil {
			item["finished_at"] = *call.FinishedAt
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return agentTraceTimestamp(items[i]).Before(agentTraceTimestamp(items[j]))
	})
	return items
}

func agentTraceTimestamp(item map[string]any) time.Time {
	switch value := item["timestamp"].(type) {
	case time.Time:
		return value
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func handleAgentSessionStream(w http.ResponseWriter, r *http.Request, svc routerServices, sessionID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeServiceError(w, services.NewError(services.CodeUnsupported, "streaming is not supported by this server", nil, nil))
		return
	}

	session, err := svc.agent.GetSession(r.Context(), sessionID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	lastSignature := ""
	writeJSONEvent := func(event string, payload any) bool {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encoded)
		flusher.Flush()
		return true
	}
	writeRuntimeEvent := func(ev agentruntime.Event) bool {
		if strings.TrimSpace(ev.Type) == "" {
			ev.Type = "runtime_event"
		}
		encoded, err := json.Marshal(ev)
		if err != nil {
			return false
		}
		if ev.Seq > 0 {
			_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, encoded)
		} else {
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, encoded)
		}
		flusher.Flush()
		return true
	}
	writeSessionEvent := func(session agentSession) bool {
		signature := agentSessionStreamSignature(session)
		if signature == lastSignature {
			return false
		}
		lastSignature = signature
		return writeJSONEvent("session", map[string]any{"session": session})
	}

	writeSessionEvent(session)
	previousSession := session
	var runtimeEvents <-chan agentruntime.Event
	if svc.agent.runtimeEngine() != nil {
		runtimeEvents = svc.agent.SubscribeSince(r.Context(), sessionID, agentRuntimeStreamFromSeq(r))
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			session, err := svc.agent.GetSession(r.Context(), sessionID)
			if err != nil {
				writeJSONEvent("error", map[string]any{"error": serviceErrorMap(err)})
				return
			}
			for _, activity := range agentSessionStreamActivities(previousSession, session) {
				writeJSONEvent("activity", map[string]any{"activity": activity})
			}
			writeSessionEvent(session)
			previousSession = session
		case ev, ok := <-runtimeEvents:
			if !ok {
				runtimeEvents = nil
				continue
			}
			writeRuntimeEvent(ev)
		case <-pingTicker.C:
			_, _ = fmt.Fprintf(w, "event: ping\ndata: {\"time\":%q}\n\n", time.Now().UTC().Format(time.RFC3339))
			flusher.Flush()
		}
	}
}

func agentRuntimeStreamFromSeq(r *http.Request) int64 {
	if r == nil {
		return 0
	}
	for _, raw := range []string{
		r.Header.Get("Last-Event-ID"),
		r.URL.Query().Get("from_seq"),
		r.URL.Query().Get("last_event_id"),
	} {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		seq, err := strconv.ParseInt(value, 10, 64)
		if err == nil && seq > 0 {
			return seq
		}
	}
	return 0
}

type agentStreamActivity struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Detail    string    `json:"detail,omitempty"`
	Status    string    `json:"status,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

func agentSessionStreamActivities(previous, current agentSession) []agentStreamActivity {
	activities := make([]agentStreamActivity, 0, 8)
	now := current.UpdatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if previous.Status != current.Status {
		activities = append(activities, agentStreamActivity{
			Kind:      "state",
			ID:        "state:status",
			Title:     agentText(current.Language, "会话状态变化", "Session status changed"),
			Detail:    previous.Status + " → " + current.Status,
			Status:    current.Status,
			Timestamp: now,
		})
	}

	if previousNext, currentNext := agentNextActionStreamSignature(previous.NextAction), agentNextActionStreamSignature(current.NextAction); previousNext != currentNext {
		status := ""
		detail := ""
		if current.NextAction != nil {
			status = current.NextAction.Type
			detail = current.NextAction.Reason
			if detail == "" {
				detail = current.NextAction.SuggestedPrompt
			}
		}
		activities = append(activities, agentStreamActivity{
			Kind:      "state",
			ID:        "state:next_action",
			Title:     agentText(current.Language, "下一步动作更新", "Next action updated"),
			Detail:    trimSummary(detail, 220),
			Status:    status,
			Timestamp: now,
		})
	}

	if previousTask, currentTask := agentTaskStreamSignature(previous.ActiveTask), agentTaskStreamSignature(current.ActiveTask); previousTask != currentTask && current.ActiveTask != nil {
		activities = append(activities, agentStreamActivity{
			Kind:      "state",
			ID:        "state:active_task",
			Title:     agentText(current.Language, "当前任务更新", "Active task updated"),
			Detail:    trimSummary(current.ActiveTask.Goal, 240),
			Status:    current.ActiveTask.Status,
			Timestamp: current.ActiveTask.UpdatedAt,
		})
	}

	previousMessages := make(map[string]struct{}, len(previous.Messages))
	for _, message := range previous.Messages {
		if message.ID != "" {
			previousMessages[message.ID] = struct{}{}
		}
	}
	for _, message := range current.Messages {
		if message.ID == "" {
			continue
		}
		if _, ok := previousMessages[message.ID]; ok {
			continue
		}
		title := agentText(current.Language, "Agent 消息", "Agent message")
		switch message.Role {
		case "user":
			title = agentText(current.Language, "操作者消息", "Operator message")
		case "tool":
			title = agentText(current.Language, "工具观察", "Tool observation")
		case "system":
			title = agentText(current.Language, "系统消息", "System message")
		}
		activities = append(activities, agentStreamActivity{
			Kind:      "message",
			ID:        "message:" + message.ID,
			Title:     title,
			Detail:    trimSummary(message.Content, 240),
			Status:    message.Role,
			Timestamp: message.Timestamp,
		})
	}

	previousTools := make(map[string]string, len(previous.ToolCalls))
	for _, tool := range previous.ToolCalls {
		previousTools[agentToolCallStreamID(tool)] = agentToolCallStreamSignature(tool)
	}
	for _, tool := range current.ToolCalls {
		id := agentToolCallStreamID(tool)
		signature := agentToolCallStreamSignature(tool)
		if previousTools[id] == signature {
			continue
		}
		detail := tool.Detail
		if detail == "" && tool.Error != nil {
			detail = tool.Error.Message
		}
		timestamp := tool.StartedAt
		if tool.FinishedAt != nil {
			timestamp = *tool.FinishedAt
		}
		activities = append(activities, agentStreamActivity{
			Kind:      "tool",
			ID:        "tool:" + id,
			Title:     tool.Name,
			Detail:    trimSummary(detail, 240),
			Status:    string(tool.Status),
			Timestamp: timestamp,
		})
	}

	previousTodos := make(map[string]string, len(previous.Todos))
	for _, todo := range previous.Todos {
		previousTodos[todo.ID] = agentTodoStreamSignature(todo)
	}
	for _, todo := range current.Todos {
		if todo.ID == "" {
			continue
		}
		if previousTodos[todo.ID] == agentTodoStreamSignature(todo) {
			continue
		}
		detail := todo.Detail
		if detail == "" {
			detail = todo.BlockedReason
		}
		activities = append(activities, agentStreamActivity{
			Kind:      "todo",
			ID:        "todo:" + todo.ID,
			Title:     trimSummary(todo.Title, 160),
			Detail:    trimSummary(detail, 220),
			Status:    todo.Status,
			Timestamp: todo.UpdatedAt,
		})
	}

	return activities
}

func agentSessionStreamSignature(session agentSession) string {
	nextAction := ""
	if session.NextAction != nil {
		nextAction = agentNextActionStreamSignature(session.NextAction)
	}
	activeRun := ""
	if session.ActiveTask != nil {
		activeRun = session.ActiveTask.ID + "|" + session.ActiveTask.Status + "|" + session.ActiveTask.ActiveRunID + "|" + session.ActiveTask.UpdatedAt.Format(time.RFC3339Nano)
	}
	memory := ""
	if session.MemorySummary != nil {
		memory = strconv.Itoa(session.MemorySummary.MessageCount) + "|" + session.MemorySummary.LastMessageID + "|" + session.MemorySummary.UpdatedAt.Format(time.RFC3339Nano)
	}
	return fmt.Sprintf(
		"%s|%s|%d|%d|%d|%s|%s|%s",
		session.UpdatedAt.Format(time.RFC3339Nano),
		session.Status,
		len(session.Messages),
		len(session.ToolCalls),
		len(session.Todos),
		nextAction,
		activeRun,
		memory,
	)
}

func agentNextActionStreamSignature(next *agentNextAction) string {
	if next == nil {
		return ""
	}
	return next.Type + "|" + next.Reason + "|" + next.SuggestedPrompt + "|" + strconv.FormatBool(next.AutoContinue)
}

func agentTaskStreamSignature(task *agentTask) string {
	if task == nil {
		return ""
	}
	return task.ID + "|" + task.Goal + "|" + task.Status + "|" + task.ActiveRunID + "|" + task.Summary + "|" + task.UpdatedAt.Format(time.RFC3339Nano)
}

func agentToolCallStreamID(tool agentToolCall) string {
	if strings.TrimSpace(tool.ID) != "" {
		return tool.ID
	}
	return tool.Name + ":" + tool.StartedAt.Format(time.RFC3339Nano)
}

func agentToolCallStreamSignature(tool agentToolCall) string {
	finished := ""
	if tool.FinishedAt != nil {
		finished = tool.FinishedAt.Format(time.RFC3339Nano)
	}
	errMessage := ""
	if tool.Error != nil {
		errMessage = string(tool.Error.Code) + ":" + tool.Error.Message
	}
	return tool.Name + "|" + string(tool.Status) + "|" + tool.Detail + "|" + finished + "|" + errMessage
}

func agentTodoStreamSignature(todo agentTodo) string {
	return todo.Title + "|" + todo.Detail + "|" + todo.Status + "|" + todo.BlockedReason + "|" + todo.UpdatedAt.Format(time.RFC3339Nano)
}

func serviceErrorMap(err error) map[string]any {
	_, body := serviceErrorBody(err)
	return map[string]any{
		"code":    body.Code,
		"message": body.Message,
		"details": body.Details,
	}
}

func handleRunRoute(w http.ResponseWriter, r *http.Request, svc routerServices) {
	switch r.Method {
	case http.MethodGet:
		handleRunRead(w, r, svc)
	case http.MethodPost:
		handleRunWrite(w, r, svc)
	default:
		writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
	}
}

func handleRunRead(w http.ResponseWriter, r *http.Request, svc routerServices) {
	runID, resource, ok := parseRunReadPath(r.URL.Path)
	if !ok {
		writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
		return
	}
	switch resource {
	case "":
		detail, err := svc.runs.GetRun(r.Context(), runID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, detail)
	case "events":
		limit, err := optionalInt(r.URL.Query().Get("limit"))
		if err != nil {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "invalid limit", map[string]any{"field": "limit"}, err))
			return
		}
		page, err := svc.runs.GetEvents(r.Context(), runID, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, page)
	case "report":
		report, err := svc.runs.GetReport(r.Context(), runID, r.URL.Query().Get("format"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, reportPayload(report))
	case "artifacts":
		index, err := svc.runs.GetArtifactIndex(r.Context(), runID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, index)
	case "approvals":
		approvals, err := svc.approvals.ListApprovals(r.Context(), runID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{"approvals": approvals})
	default:
		writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
	}
}

type approvalDecisionHTTPBody struct {
	Operator string `json:"operator"`
	Comment  string `json:"comment"`
}

type resumeApprovedHTTPBody struct {
	Operator string `json:"operator"`
}

func handleRunWrite(w http.ResponseWriter, r *http.Request, svc routerServices) {
	if runID, approvalID, action, ok := parseApprovalDecisionPath(r.URL.Path); ok {
		var body approvalDecisionHTTPBody
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		req := services.ApprovalDecisionRequest{
			RunIDOrPath:        runID,
			ApprovalIDOrStepID: approvalID,
			Operator:           body.Operator,
			Comment:            body.Comment,
		}
		var (
			resolved any
			err      error
		)
		switch action {
		case "approve":
			resolved, err = svc.approvals.Approve(r.Context(), req)
		case "deny":
			resolved, err = svc.approvals.Deny(r.Context(), req)
		default:
			writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
			return
		}
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, resolved)
		return
	}

	if runID, ok := parseResumeApprovedPath(r.URL.Path); ok {
		var body resumeApprovedHTTPBody
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		result, err := svc.approvals.ResumeApproved(r.Context(), services.ResumeApprovedRequest{
			RunIDOrPath: runID,
			Operator:    body.Operator,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, result)
		return
	}

	writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
}

type startWebScanHTTPBody struct {
	Target          string   `json:"target"`
	Scope           string   `json:"scope"`
	Profile         string   `json:"profile,omitempty"`
	EnabledPlugins  []string `json:"enabled_plugins,omitempty"`
	NoExternalTools *bool    `json:"no_external_tools,omitempty"`
}

func handleScanWebRoute(w http.ResponseWriter, r *http.Request, svc routerServices) {
	if r.Method != http.MethodPost {
		writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
		return
	}
	if r.URL.Path != "/api/v1/scans/web" {
		writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
		return
	}
	var body startWebScanHTTPBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeServiceError(w, err)
		return
	}
	noExternalTools := true
	if body.NoExternalTools != nil {
		noExternalTools = *body.NoExternalTools
	}
	result, err := svc.jobs.StartWebScan(r.Context(), startWebScanJobRequest{
		Target:          body.Target,
		Scope:           body.Scope,
		Profile:         body.Profile,
		EnabledPlugins:  body.EnabledPlugins,
		NoExternalTools: noExternalTools,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeOK(w, http.StatusOK, result)
}

func handleJobRoute(w http.ResponseWriter, r *http.Request, svc routerServices) {
	jobID, action, ok := parseJobPath(r.URL.Path)
	if !ok {
		writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
		return
	}
	switch r.Method {
	case http.MethodGet:
		if action != "" {
			writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
			return
		}
		job, err := svc.jobs.GetJob(r.Context(), jobID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, job)
	case http.MethodPost:
		if action != "cancel" {
			writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
			return
		}
		job, err := svc.jobs.CancelJob(r.Context(), jobID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{"job_id": job.JobID, "status": job.Status})
	default:
		writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
	}
}

func parseRunReadPath(path string) (runID string, resource string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/runs/")
	if rest == "" || rest == path {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", "", false
	}
	if len(parts) == 1 {
		return parts[0], "", true
	}
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func parseApprovalDecisionPath(path string) (runID string, approvalID string, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/runs/")
	if rest == "" || rest == path {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 4 {
		return "", "", "", false
	}
	if strings.TrimSpace(parts[0]) == "" ||
		parts[1] != "approvals" ||
		strings.TrimSpace(parts[2]) == "" ||
		strings.TrimSpace(parts[3]) == "" {
		return "", "", "", false
	}
	return parts[0], parts[2], parts[3], true
}

func parseResumeApprovedPath(path string) (runID string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/runs/")
	if rest == "" || rest == path {
		return "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || parts[1] != "resume-approved" {
		return "", false
	}
	return parts[0], true
}

func parseJobPath(path string) (jobID string, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/jobs/")
	if rest == "" || rest == path {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", "", false
	}
	if len(parts) == 1 {
		return parts[0], "", true
	}
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func parseAgentSessionPath(path string) (sessionID string, resource string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/agent/sessions/")
	if rest == "" || rest == path {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", "", false
	}
	if len(parts) == 1 {
		return parts[0], "", true
	}
	if len(parts) >= 2 && strings.TrimSpace(parts[1]) == "workspace" {
		return parts[0], "workspace", true
	}
	if len(parts) == 3 && strings.TrimSpace(parts[1]) == "authorization" && strings.TrimSpace(parts[2]) != "" {
		return parts[0], "authorization/" + strings.TrimSpace(parts[2]), true
	}
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func decodeJSONBody(r *http.Request, out any) error {
	if r.Body == nil {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return services.NewError(services.CodeBadRequest, "invalid JSON body", nil, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return services.NewError(services.CodeBadRequest, "invalid JSON body", nil, err)
	} else if err == nil {
		return services.NewError(services.CodeBadRequest, "invalid JSON body", map[string]any{"reason": "multiple JSON values"}, nil)
	}
	return nil
}

func optionalInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value, nil
}

type reportHTTPPayload struct {
	Format      string `json:"format"`
	ContentType string `json:"content_type"`
	Report      any    `json:"report,omitempty"`
	Content     string `json:"content,omitempty"`
}

func reportPayload(report services.RunReport) reportHTTPPayload {
	payload := reportHTTPPayload{Format: report.Format, ContentType: report.ContentType}
	if report.Format == "json" {
		var decoded any
		if err := json.Unmarshal(report.Bytes, &decoded); err == nil {
			payload.Report = decoded
			return payload
		}
	}
	payload.Content = string(report.Bytes)
	return payload
}

func handleWorkspaceRoute(w http.ResponseWriter, r *http.Request, svc routerServices, sessionID string) {
	// Parse workspace action from URL
	// Expected: /api/v1/agent/sessions/{id}/workspace/{action}
	path := r.URL.Path
	prefix := "/api/v1/agent/sessions/" + sessionID + "/workspace/"
	action := strings.TrimPrefix(path, prefix)
	action = strings.TrimSuffix(action, "/")
	operationLevel := normalizeAgentOperationLevel(r.URL.Query().Get("operation_level"))
	if headerLevel := strings.TrimSpace(r.Header.Get("X-Agent-Operation-Level")); headerLevel != "" {
		operationLevel = normalizeAgentOperationLevel(headerLevel)
	}

	switch action {
	case "files/read":
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		if _, err := svc.agent.workspace.Create(sessionID); err != nil {
			writeServiceError(w, err)
			return
		}
		data, err := svc.agent.workspace.ReadFile(sessionID, body.Path)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{
			"path":    body.Path,
			"content": string(data),
			"size":    len(data),
		})

	case "files/write":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.write"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		if _, err := svc.agent.workspace.Create(sessionID); err != nil {
			writeServiceError(w, err)
			return
		}
		if err := svc.agent.workspace.WriteFile(sessionID, body.Path, []byte(body.Content)); err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{
			"path":   body.Path,
			"size":   len(body.Content),
			"status": "written",
		})

	case "files/patch":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.patch"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			Path string `json:"path"`
			Old  string `json:"old"`
			New  string `json:"new"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		result, err := svc.agent.patchWorkspaceFile(sessionID, body.Path, body.Old, body.New)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, result)

	case "files/diff":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.diff"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			Path string `json:"path"`
			Old  string `json:"old"`
			New  string `json:"new"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		result, err := svc.agent.stageWorkspaceFileChange(sessionID, body.Path, body.Old, body.New, "")
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, result)

	case "files/diff-many":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.diff_many"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			Changes []agentWorkspaceChangeInput `json:"changes"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		result, err := svc.agent.stageWorkspaceFileChanges(sessionID, body.Changes, "")
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, result)

	case "files/rebase":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.rebase"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			ChangeID string `json:"change_id"`
			Old      string `json:"old"`
			New      string `json:"new"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		result, err := svc.agent.rebaseWorkspaceFileChange(sessionID, body.ChangeID, body.Old, body.New, "")
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, result)

	case "files/apply":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.apply"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			ChangeID string `json:"change_id"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		result, err := svc.agent.applyWorkspaceFileChange(sessionID, body.ChangeID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, result)

	case "files/apply-group":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.apply"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			GroupID string `json:"group_id"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		result, err := svc.agent.applyWorkspaceChangeGroup(sessionID, body.GroupID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, result)

	case "files/discard":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.discard"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			ChangeID string `json:"change_id"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		result, err := svc.agent.discardWorkspaceFileChange(sessionID, body.ChangeID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, result)

	case "files/discard-group":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.discard"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			GroupID string `json:"group_id"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		result, err := svc.agent.discardWorkspaceChangeGroup(sessionID, body.GroupID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, result)

	case "files/list":
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		if _, err := svc.agent.workspace.Create(sessionID); err != nil {
			writeServiceError(w, err)
			return
		}
		files, err := svc.agent.workspace.ListFiles(sessionID, body.Path)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{
			"path":  body.Path,
			"files": files,
			"count": len(files),
		})

	case "files/delete":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.delete"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		if _, err := svc.agent.workspace.Create(sessionID); err != nil {
			writeServiceError(w, err)
			return
		}
		if err := svc.agent.workspace.DeleteFile(sessionID, body.Path); err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{
			"path":   body.Path,
			"status": "deleted",
		})

	case "files/move":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.move"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		if _, err := svc.agent.workspace.Create(sessionID); err != nil {
			writeServiceError(w, err)
			return
		}
		if err := svc.agent.workspace.MoveFile(sessionID, body.From, body.To); err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{
			"from":   body.From,
			"path":   body.To,
			"status": "moved",
		})

	case "files/copy":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.copy"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		if _, err := svc.agent.workspace.Create(sessionID); err != nil {
			writeServiceError(w, err)
			return
		}
		if err := svc.agent.workspace.CopyFile(sessionID, body.From, body.To); err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{
			"from":   body.From,
			"path":   body.To,
			"status": "copied",
		})

	case "files/mkdir":
		if !agentOperationLevelAtLeast(operationLevel, "workspace_write") {
			writeServiceError(w, agentOperationLevelDeniedError(normalizeAgentLanguage(r.URL.Query().Get("language")), operationLevel, "workspace_write", "workspace.files.mkdir"))
			return
		}
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		if _, err := svc.agent.workspace.Create(sessionID); err != nil {
			writeServiceError(w, err)
			return
		}
		if err := svc.agent.workspace.MakeDir(sessionID, body.Path); err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{
			"path":   body.Path,
			"status": "created",
		})

	case "command":
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		var body struct {
			Command string `json:"command"`
			WorkDir string `json:"workdir"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		ws, err := svc.agent.workspace.Create(sessionID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		executor := command.NewExecutor(ws.Path, 0, 0)
		result, err := executor.Execute(r.Context(), body.Command, body.WorkDir)
		if err != nil {
			writeServiceError(w, agentCommandServiceError(err, body.Command, body.WorkDir))
			return
		}
		writeOK(w, http.StatusOK, result)

	default:
		writeServiceError(w, services.NewError(services.CodeNotFound, "workspace action not found", map[string]any{"action": action}, nil))
	}
}

func handleScheduleRoute(w http.ResponseWriter, r *http.Request, svc routerServices) {
	if svc.schedules == nil {
		writeServiceError(w, services.NewError(services.CodeInternal, "scheduler not initialized", nil, nil))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/schedules")
	path = strings.Trim(path, "/")

	if path == "" {
		switch r.Method {
		case http.MethodGet:
			writeOK(w, http.StatusOK, map[string]any{"schedules": svc.schedules.List()})
		case http.MethodPost:
			var body scheduleSpec
			if err := decodeJSONBody(r, &body); err != nil {
				writeServiceError(w, err)
				return
			}
			created, err := svc.schedules.Create(body)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusCreated, map[string]any{"schedule": created})
		default:
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
		}
		return
	}

	parts := strings.Split(path, "/")
	id := parts[0]
	if strings.TrimSpace(id) == "" {
		writeServiceError(w, services.NewError(services.CodeBadRequest, "schedule id required", nil, nil))
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			spec, err := svc.schedules.Get(id)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, map[string]any{"schedule": spec})
		case http.MethodPut:
			var body scheduleSpec
			if err := decodeJSONBody(r, &body); err != nil {
				writeServiceError(w, err)
				return
			}
			updated, err := svc.schedules.Update(id, body)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, map[string]any{"schedule": updated})
		case http.MethodDelete:
			if err := svc.schedules.Delete(id); err != nil {
				writeServiceError(w, err)
				return
			}
			writeOK(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
		default:
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
		}
		return
	}

	if len(parts) != 2 {
		writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
		return
	}

	action := parts[1]
	if r.Method != http.MethodPost {
		writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
		return
	}
	switch action {
	case "enable":
		spec, err := svc.schedules.SetEnabled(id, true)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{"schedule": spec})
	case "disable":
		spec, err := svc.schedules.SetEnabled(id, false)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{"schedule": spec})
	case "run":
		spec, err := svc.schedules.RunNow(r.Context(), id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{"schedule": spec})
	default:
		writeServiceError(w, services.NewError(services.CodeNotFound, "schedule action not found", map[string]any{"action": action}, nil))
	}
}
