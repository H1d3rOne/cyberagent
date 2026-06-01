package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/agent/search"
	"github.com/H1d3rOne/cyberagent/internal/command"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
	doctorruntime "github.com/H1d3rOne/cyberagent/internal/runtime/doctor"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
	"github.com/H1d3rOne/cyberagent/internal/workspace"
)

const agentAPIVersion = "v0.1.8"

const agentSessionMaxMessages = 80
const agentSessionMemoryCompactionTurns = 40

type agentManager struct {
	mu        sync.Mutex
	sessions  map[string]*agentSession
	storeDir  string
	runs      services.RunService
	approvals services.ApprovalService
	workspace *workspace.Manager
	command   *command.Executor
	providers web.ProviderStore
	// engine is the optional runtime.Engine used to drive streaming event
	// output for sessions. nil-safe; set via AttachEngine.
	engine *runtime.Engine
}

// AttachEngine wires a runtime.Engine into the agent manager. Once attached,
// the manager exposes Subscribe(ctx, sessionID) so router-side SSE handlers
// can stream model_token / tool_* / approval_required events instead of
// polling GetSession.
type agentSession struct {
	SessionID            string                     `json:"session_id"`
	CreatedAt            time.Time                  `json:"created_at"`
	UpdatedAt            time.Time                  `json:"updated_at"`
	Operator             string                     `json:"operator"`
	Language             string                     `json:"language"`
	OperationLevel       string                     `json:"operation_level,omitempty"`
	Status               string                     `json:"status"`
	Messages             []agentMessage             `json:"messages"`
	ActiveTask           *agentTask                 `json:"active_task,omitempty"`
	Todos                []agentTodo                `json:"todos,omitempty"`
	ToolCalls            []agentToolCall            `json:"tool_calls,omitempty"`
	ContextRefs          []agentContextRef          `json:"context_refs,omitempty"`
	PendingChanges       []agentWorkspaceChange     `json:"pending_changes,omitempty"`
	PendingAuthorization *agentAuthorizationRequest `json:"pending_authorization,omitempty"`
	MemorySummary        *agentMemorySummary        `json:"memory_summary,omitempty"`
	NextAction           *agentNextAction           `json:"next_action,omitempty"`
	RunSummary           *agentSessionRunSummary    `json:"run_summary,omitempty"`
}

type agentMessage struct {
	ID                    string                   `json:"id"`
	Role                  string                   `json:"role"`
	Content               string                   `json:"content"`
	Timestamp             time.Time                `json:"timestamp"`
	Phase                 string                   `json:"phase,omitempty"`
	ToolCalls             []agentToolCall          `json:"tool_calls,omitempty"`
	Links                 []agentLink              `json:"links,omitempty"`
	Reasoning             string                   `json:"reasoning,omitempty"`
	SuggestedPrompts      []string                 `json:"suggested_prompts,omitempty"`
	RecommendationOptions []agentRecommendationOpt `json:"recommendation_options,omitempty"`
}

type agentToolCall struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Status     contracts.Status       `json:"status"`
	Detail     string                 `json:"detail,omitempty"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt *time.Time             `json:"finished_at,omitempty"`
	Result     map[string]any         `json:"result,omitempty"`
	Error      *daemonJobError        `json:"error,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type agentLink struct {
	To    string `json:"to"`
	Label string `json:"label"`
	Kind  string `json:"kind,omitempty"`
}

type agentTask struct {
	ID          string    `json:"id"`
	Goal        string    `json:"goal"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	ActiveRunID string    `json:"active_run_id,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type agentTodo struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Detail        string    `json:"detail,omitempty"`
	Status        string    `json:"status"`
	Priority      int       `json:"priority"`
	ToolCallID    string    `json:"tool_call_id,omitempty"`
	ContextRefIDs []string  `json:"context_ref_ids,omitempty"`
	BlockedReason string    `json:"blocked_reason,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type agentMemorySummary struct {
	Summary       string     `json:"summary"`
	MessageCount  int        `json:"message_count"`
	LastMessageID string     `json:"last_message_id,omitempty"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type agentRunReportSnapshot struct {
	Findings  []agentRunFindingSnapshot  `json:"findings,omitempty"`
	Evidence  []agentRunEvidenceSnapshot `json:"evidence,omitempty"`
	Artifacts []agentRunArtifactSnapshot `json:"artifacts,omitempty"`
}

type agentRunFindingSnapshot struct {
	ID          string   `json:"id,omitempty"`
	Title       string   `json:"title,omitempty"`
	Severity    string   `json:"severity,omitempty"`
	Category    string   `json:"category,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Impact      string   `json:"impact,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
}

type agentRunEvidenceSnapshot struct {
	ID         string `json:"id,omitempty"`
	Type       string `json:"type,omitempty"`
	Title      string `json:"title,omitempty"`
	Summary    string `json:"summary,omitempty"`
	ArtifactID string `json:"artifact_id,omitempty"`
}

type agentRunArtifactSnapshot struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Path     string `json:"path,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
}

type agentWorkspaceChange struct {
	ID             string     `json:"id"`
	GroupID        string     `json:"group_id,omitempty"`
	Path           string     `json:"path"`
	OldText        string     `json:"old_text,omitempty"`
	NewText        string     `json:"new_text,omitempty"`
	SizeBefore     int        `json:"size_before"`
	SizeAfter      int        `json:"size_after"`
	Replacements   int        `json:"replacements"`
	DiffPreview    string     `json:"diff_preview"`
	ContentPreview string     `json:"content_preview,omitempty"`
	Status         string     `json:"status"`
	Error          string     `json:"error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	AppliedAt      *time.Time `json:"applied_at,omitempty"`
	ToolCallID     string     `json:"tool_call_id,omitempty"`
}

type agentWorkspaceChangeInput struct {
	Path string `json:"path"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

type agentAuthorizationRequest struct {
	ID             string         `json:"id"`
	ActionName     string         `json:"action_name"`
	Arguments      map[string]any `json:"arguments,omitempty"`
	RequiredLevel  string         `json:"required_level,omitempty"`
	Policy         string         `json:"policy,omitempty"`
	Reason         string         `json:"reason,omitempty"`
	OriginalPrompt string         `json:"original_prompt,omitempty"`
	Status         string         `json:"status"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type preparedWorkspaceApply struct {
	Change agentWorkspaceChange
	Before string
	After  string
}

type createAgentSessionRequest struct {
	Operator       string `json:"operator"`
	Language       string `json:"language"`
	OperationLevel string `json:"operation_level,omitempty"`
}

type createAgentSessionResponse struct {
	Session agentSession `json:"session"`
}

type agentSessionSummary struct {
	SessionID            string                     `json:"session_id"`
	CreatedAt            time.Time                  `json:"created_at"`
	UpdatedAt            time.Time                  `json:"updated_at"`
	Operator             string                     `json:"operator"`
	Language             string                     `json:"language"`
	OperationLevel       string                     `json:"operation_level,omitempty"`
	Status               string                     `json:"status"`
	ActiveTask           *agentTask                 `json:"active_task,omitempty"`
	NextAction           *agentNextAction           `json:"next_action,omitempty"`
	PendingAuthorization *agentAuthorizationRequest `json:"pending_authorization,omitempty"`
	RunSummary           *agentSessionRunSummary    `json:"run_summary,omitempty"`
	MemorySummary        *agentMemorySummary        `json:"memory_summary,omitempty"`
	LastMessage          string                     `json:"last_message,omitempty"`
	MessageCount         int                        `json:"message_count"`
	ToolCallCount        int                        `json:"tool_call_count"`
	ContextCount         int                        `json:"context_count"`
}

type agentTraceResponse struct {
	SessionID   string                  `json:"session_id"`
	GeneratedAt time.Time               `json:"generated_at"`
	Summary     agentTraceSummary       `json:"summary"`
	Session     agentSession            `json:"session"`
	Events      []agentTraceEvent       `json:"events"`
	ActiveTask  *agentTask              `json:"active_task,omitempty"`
	Todos       []agentTodo             `json:"todos,omitempty"`
	NextAction  *agentNextAction        `json:"next_action,omitempty"`
	ContextRefs []agentContextRef       `json:"context_refs,omitempty"`
	RunSummary  *agentSessionRunSummary `json:"run_summary,omitempty"`
}

type agentTraceSummary struct {
	Messages       int `json:"messages"`
	ToolCalls      int `json:"tool_calls"`
	Todos          int `json:"todos"`
	ContextRefs    int `json:"context_refs"`
	PendingChanges int `json:"pending_changes"`
}

type agentTraceEvent struct {
	Index      int                    `json:"index"`
	Timestamp  time.Time              `json:"timestamp"`
	Kind       string                 `json:"kind"`
	ID         string                 `json:"id"`
	Phase      string                 `json:"phase,omitempty"`
	Role       string                 `json:"role,omitempty"`
	Title      string                 `json:"title"`
	Detail     string                 `json:"detail,omitempty"`
	Status     string                 `json:"status,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type agentSessionRunSummary struct {
	RunID            string           `json:"run_id"`
	Status           contracts.Status `json:"status,omitempty"`
	Target           string           `json:"target,omitempty"`
	Findings         int              `json:"findings"`
	Artifacts        int              `json:"artifacts"`
	Events           int              `json:"events"`
	Approvals        int              `json:"approvals"`
	PendingApprovals int              `json:"pending_approvals"`
	ReportJSON       bool             `json:"report_json,omitempty"`
	Summary          string           `json:"summary,omitempty"`
	LeadFinding      string           `json:"lead_finding,omitempty"`
	LeadEvidence     string           `json:"lead_evidence,omitempty"`
	LeadArtifact     string           `json:"lead_artifact,omitempty"`
}

type listAgentSessionsResponse struct {
	Items []agentSessionSummary `json:"items"`
	Count int                   `json:"count"`
	Limit int                   `json:"limit"`
}

type deleteAgentSessionResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

type sendAgentMessageRequest struct {
	Message              string                 `json:"message"`
	Operator             string                 `json:"operator"`
	Language             string                 `json:"language"`
	OperationLevel       string                 `json:"operation_level,omitempty"`
	Origin               string                 `json:"-"`
	Provider             string                 `json:"provider,omitempty"`
	Model                string                 `json:"model,omitempty"`
	APIKey               string                 `json:"api_key,omitempty"`
	BaseURL              string                 `json:"base_url,omitempty"`
	Instructions         string                 `json:"instructions,omitempty"`
	AuthProfiles         []agentHTTPAuthProfile `json:"auth_profiles,omitempty"`
	DefaultAuthProfileID string                 `json:"default_auth_profile_id,omitempty"`
}

type agentHTTPAuthProfile struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	Token       string `json:"token,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	HeaderName  string `json:"header_name,omitempty"`
	HeaderValue string `json:"header_value,omitempty"`
	Cookie      string `json:"cookie,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

type agentHTTPAuthProfileUsage struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	Host    string `json:"host,omitempty"`
	Applied bool   `json:"applied"`
}

type agentControlRequest struct {
	Action         string `json:"action"`
	Operator       string `json:"operator"`
	Language       string `json:"language"`
	OperationLevel string `json:"operation_level,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	JobID          string `json:"job_id,omitempty"`
	ClearInput     bool   `json:"clear_input,omitempty"`
}

type agentControlResponse struct {
	Session agentSession `json:"session"`
	Message agentMessage `json:"message"`
}

type agentAuthorizationDecisionRequest struct {
	Action               string                 `json:"action"`
	AuthorizationID      string                 `json:"authorization_id,omitempty"`
	Operator             string                 `json:"operator"`
	Language             string                 `json:"language"`
	OperationLevel       string                 `json:"operation_level,omitempty"`
	Reason               string                 `json:"reason,omitempty"`
	Provider             string                 `json:"provider,omitempty"`
	Model                string                 `json:"model,omitempty"`
	APIKey               string                 `json:"api_key,omitempty"`
	BaseURL              string                 `json:"base_url,omitempty"`
	Instructions         string                 `json:"instructions,omitempty"`
	AuthProfiles         []agentHTTPAuthProfile `json:"auth_profiles,omitempty"`
	DefaultAuthProfileID string                 `json:"default_auth_profile_id,omitempty"`
	SkipSynthesis        bool                   `json:"skip_synthesis,omitempty"`
}

type sendAgentMessageResponse struct {
	SessionID            string                     `json:"session_id"`
	Messages             []agentMessage             `json:"messages"`
	ToolCalls            []agentToolCall            `json:"tool_calls,omitempty"`
	ActiveTask           *agentTask                 `json:"active_task,omitempty"`
	Todos                []agentTodo                `json:"todos,omitempty"`
	ContextRefs          []agentContextRef          `json:"context_refs,omitempty"`
	PendingChanges       []agentWorkspaceChange     `json:"pending_changes,omitempty"`
	PendingAuthorization *agentAuthorizationRequest `json:"pending_authorization,omitempty"`
	MemorySummary        *agentMemorySummary        `json:"memory_summary,omitempty"`
	NextAction           *agentNextAction           `json:"next_action,omitempty"`
}

func newAgentManager(runsDir string) *agentManager {
	if strings.TrimSpace(runsDir) == "" {
		runsDir = DefaultRunsDir
	}
	wsRoot := filepath.Join(runsDir, ".agent", "workspaces")
	manager := &agentManager{
		sessions:  map[string]*agentSession{},
		storeDir:  filepath.Join(runsDir, ".agent", "sessions"),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		workspace: workspace.NewManager(wsRoot, 0),
		command:   command.NewExecutor(wsRoot, 0, 0),
	}
	_ = manager.loadSessions()
	return manager
}

func (m *agentManager) CreateSession(req createAgentSessionRequest) (createAgentSessionResponse, error) {
	now := time.Now().UTC()
	session := &agentSession{
		SessionID:      strings.Replace(runs.NewJobID(now), "job_", "agent_", 1),
		CreatedAt:      now,
		UpdatedAt:      now,
		Operator:       normalizeAgentOperator(req.Operator),
		Language:       normalizeAgentLanguage(req.Language),
		OperationLevel: normalizeAgentOperationLevel(req.OperationLevel),
		Status:         "idle",
		NextAction: &agentNextAction{
			Type:   "wait_user",
			Reason: agentText(req.Language, "等待用户给出授权目标、scope 或下一步指令。", "Waiting for an authorized target, scope, or next instruction."),
		},
	}
	session.Messages = append(session.Messages, m.newMessageLocked(session.Language, "agent", agentWelcome(session.Language), nil, nil))

	m.mu.Lock()
	defer m.mu.Unlock()
	for {
		if _, exists := m.sessions[session.SessionID]; !exists {
			break
		}
		session.SessionID = strings.Replace(runs.NewJobID(time.Now().UTC()), "job_", "agent_", 1)
	}
	m.sessions[session.SessionID] = session
	if err := m.persistSessionLocked(session); err != nil {
		return createAgentSessionResponse{}, err
	}
	return createAgentSessionResponse{Session: cloneAgentSession(session)}, nil
}

func (m *agentManager) ListSessions(ctx context.Context, limit int) (listAgentSessionsResponse, error) {
	if err := contextError(ctx); err != nil {
		return listAgentSessionsResponse{}, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if err := m.loadSessions(); err != nil {
		return listAgentSessionsResponse{}, services.NewError(services.CodeInternal, "load agent sessions", nil, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncTerminalActiveRunsLocked(ctx)

	items := make([]agentSessionSummary, 0, len(m.sessions))
	for _, session := range m.sessions {
		item := agentSessionSummaryFrom(session)
		item.RunSummary = m.agentSessionRunSummary(ctx, session)
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].SessionID > items[j].SessionID
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return listAgentSessionsResponse{Items: items, Count: len(items), Limit: limit}, nil
}

func (m *agentManager) DeleteSession(ctx context.Context, sessionID string) (deleteAgentSessionResponse, error) {
	if err := contextError(ctx); err != nil {
		return deleteAgentSessionResponse{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return deleteAgentSessionResponse{}, services.NewError(services.CodeBadRequest, "session id is required", map[string]any{"field": "session_id"}, nil)
	}
	if err := m.loadSessions(); err != nil {
		return deleteAgentSessionResponse{}, services.NewError(services.CodeInternal, "load agent sessions", nil, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[sessionID]; !ok {
		return deleteAgentSessionResponse{}, services.NewError(services.CodeNotFound, "agent session not found", map[string]any{"session_id": sessionID}, nil)
	}
	delete(m.sessions, sessionID)

	if path := m.sessionPath(sessionID); path != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return deleteAgentSessionResponse{}, services.NewError(services.CodeInternal, "delete agent session", map[string]any{"session_id": sessionID}, err)
		}
	}
	return deleteAgentSessionResponse{SessionID: sessionID, Status: "deleted"}, nil
}

func agentSessionSummaryFrom(session *agentSession) agentSessionSummary {
	if session == nil {
		return agentSessionSummary{}
	}
	lastMessage := ""
	for i := len(session.Messages) - 1; i >= 0; i-- {
		content := strings.TrimSpace(session.Messages[i].Content)
		if content != "" {
			lastMessage = trimSummary(content, 180)
			break
		}
	}
	return agentSessionSummary{
		SessionID:            session.SessionID,
		CreatedAt:            session.CreatedAt,
		UpdatedAt:            session.UpdatedAt,
		Operator:             session.Operator,
		Language:             session.Language,
		OperationLevel:       session.OperationLevel,
		Status:               session.Status,
		ActiveTask:           cloneAgentTask(session.ActiveTask),
		NextAction:           cloneAgentNextAction(session.NextAction),
		PendingAuthorization: cloneAgentAuthorizationRequest(session.PendingAuthorization),
		MemorySummary:        cloneAgentMemorySummary(session.MemorySummary),
		LastMessage:          lastMessage,
		MessageCount:         len(session.Messages),
		ToolCallCount:        len(session.ToolCalls),
		ContextCount:         len(session.ContextRefs),
	}
}

func (m *agentManager) agentSessionRunSummary(ctx context.Context, session *agentSession) *agentSessionRunSummary {
	if session == nil || session.ActiveTask == nil {
		return nil
	}
	runID := strings.TrimSpace(session.ActiveTask.ActiveRunID)
	if runID == "" {
		return nil
	}
	detail, err := m.runs.GetRun(ctx, runID)
	if err != nil {
		return &agentSessionRunSummary{
			RunID:   runID,
			Status:  contracts.Status(session.ActiveTask.Status),
			Summary: agentContextSummaryFromRefs(session.ContextRefs, "run:"+runID, "finding:"+runID, "report_json:"+runID, "artifact_index:"+runID),
		}
	}
	snapshot := m.activeRunReportSnapshot(ctx, runID, detail)
	status := detail.Summary.Status
	if status == "" && detail.Job != nil {
		status = detail.Job.Status
	}
	target := detail.Summary.Target
	if target == "" && detail.Job != nil {
		target = detail.Job.TargetID
	}
	counts := detail.Summary.Counts
	summary := agentRunContextSummary(session.Language, detail, snapshot)
	if summary == "" {
		summary = agentContextSummaryFromRefs(session.ContextRefs, "run:"+runID, "finding:"+runID, "report_json:"+runID, "artifact_index:"+runID)
	}
	return &agentSessionRunSummary{
		RunID:            runID,
		Status:           status,
		Target:           target,
		Findings:         counts.Findings,
		Artifacts:        counts.Artifacts,
		Events:           counts.Events,
		Approvals:        counts.Approvals,
		PendingApprovals: counts.PendingApprovals,
		ReportJSON:       detail.Summary.Report.JSON,
		Summary:          trimSummary(summary, 240),
		LeadFinding:      agentLeadFindingText(snapshot),
		LeadEvidence:     agentLeadEvidenceText(snapshot),
		LeadArtifact:     agentLeadArtifactText(snapshot),
	}
}

func agentContextSummaryFromRefs(refs []agentContextRef, ids ...string) string {
	for _, id := range ids {
		for _, ref := range refs {
			if ref.ID == id && strings.TrimSpace(ref.Summary) != "" {
				return trimSummary(ref.Summary, 240)
			}
		}
	}
	return ""
}

func (m *agentManager) GetSession(ctx context.Context, sessionID string) (agentSession, error) {
	if err := contextError(ctx); err != nil {
		return agentSession{}, err
	}
	sessionID, err := normalizeAgentSessionID(sessionID)
	if err != nil {
		return agentSession{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			return agentSession{}, loadErr
		}
		session = loaded
	}
	if m.syncTerminalActiveRunLocked(ctx, session) {
		_ = m.persistSessionLocked(session)
	}
	cloned := cloneAgentSession(session)
	cloned.RunSummary = m.agentSessionRunSummary(ctx, session)
	return cloned, nil
}

func (m *agentManager) ControlSession(ctx context.Context, sessionID string, req agentControlRequest, svc routerServices) (agentControlResponse, error) {
	if err := contextError(ctx); err != nil {
		return agentControlResponse{}, err
	}
	sessionID, err := normalizeAgentSessionID(sessionID)
	if err != nil {
		return agentControlResponse{}, err
	}
	action := normalizeAgentControlAction(req.Action)
	if action == "" {
		return agentControlResponse{}, services.NewError(services.CodeBadRequest, "unsupported agent control action", map[string]any{"action": req.Action}, nil)
	}
	language := normalizeAgentLanguage(req.Language)
	operator := normalizeAgentOperator(req.Operator)
	operationLevel := normalizeAgentOperationLevel(req.OperationLevel)
	now := time.Now().UTC()

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return agentControlResponse{}, loadErr
		}
		session = loaded
	}
	session.Operator = operator
	session.Language = language
	session.OperationLevel = operationLevel
	previousNextAction := cloneAgentNextAction(session.NextAction)

	if session.PendingAuthorization != nil &&
		strings.EqualFold(session.PendingAuthorization.Status, "pending") &&
		(action == "continue" || action == "revise") {
		session.Status = "waiting_authorization"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		session.NextAction = &agentNextAction{
			Type:            "authorization_required",
			Reason:          session.PendingAuthorization.Reason,
			SuggestedPrompt: agentAuthorizationSuggestedPrompt(language, session.PendingAuthorization),
			AutoContinue:    false,
		}
		message := agentMessageWithPhase(m.newMessageLocked(language, "agent", agentText(language, "当前有挂起的工具授权，请先点击允许或拒绝；在此之前不会继续执行任务。", "A tool authorization is pending. Approve or deny it first; the task will not continue before that."), nil, nil), "step")
		session.Messages = append(session.Messages, message)
		session.UpdatedAt = now
		compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
		if err := m.persistSessionLocked(session); err != nil {
			m.mu.Unlock()
			return agentControlResponse{}, err
		}
		sessionState := cloneAgentSession(session)
		m.mu.Unlock()
		m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionState, agentRuntimeTurnRef{}, compaction)
		return agentControlResponse{Session: sessionState, Message: message}, nil
	}

	var message agentMessage
	var cancelJobID string
	switch action {
	case "continue":
		session.Status = "running"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
		m.ensureRunningTodoLocked(session, now)
		previousAutoContinue := previousNextAction != nil &&
			previousNextAction.Type == "continue" &&
			previousNextAction.AutoContinue &&
			strings.TrimSpace(req.Prompt) == "" &&
			strings.TrimSpace(previousNextAction.SuggestedPrompt) != ""
		previousSuggestedPrompt := ""
		if previousAutoContinue {
			previousSuggestedPrompt = previousNextAction.SuggestedPrompt
		}
		suggestedPrompt := firstNonEmpty(strings.TrimSpace(req.Prompt), strings.TrimSpace(previousSuggestedPrompt), agentSuggestedContinuePrompt(language, session))
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(language, "用户请求继续当前任务。", "The operator requested continuing the current task."),
			SuggestedPrompt: suggestedPrompt,
			AutoContinue:    previousAutoContinue,
		}
		if agentEvidenceActionHasApprovalGate(suggestedPrompt) {
			session.NextAction = &agentNextAction{
				Type:            "approval_required",
				Reason:          agentText(language, "当前继续动作仍包含待审批 gate，需先处理审批后再继续。", "The continue action still contains a pending approval gate; resolve approvals before continuing."),
				SuggestedPrompt: suggestedPrompt,
			}
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "收到：继续当前任务。我会基于当前计划、工具调用和工作区上下文推进下一步。", "Received: continue the current task. I will proceed based on the current plan, tool calls and workspace context."), nil, nil)
	case "revise":
		revision := strings.TrimSpace(req.Prompt)
		if revision == "" {
			revision = strings.TrimSpace(req.Reason)
		}
		if revision == "" {
			revision = agentText(language, "修订当前任务计划。", "Revise the current task plan.")
		}
		session.Status = "running"
		if session.ActiveTask == nil {
			session.ActiveTask = &agentTask{
				ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
				Goal:      agentText(language, "修订安全分析任务", "Revise security analysis task"),
				Kind:      "planning",
				Status:    "running",
				Summary:   revision,
				CreatedAt: now,
				UpdatedAt: now,
			}
		} else {
			session.ActiveTask.Status = "running"
			session.ActiveTask.Summary = trimSummary(revision, 360)
			session.ActiveTask.UpdatedAt = now
		}
		isEvidenceAction := agentIsEvidenceActionPrompt(revision)
		evidenceActionRequiresApproval := false
		if isEvidenceAction {
			evidenceActionRequiresApproval = agentEvidenceActionHasApprovalGate(revision)
			session.Todos = agentEvidenceActionTodos(language, revision, now)
		} else {
			m.ensureRunningTodoLocked(session, now)
			if len(session.Todos) == 0 {
				session.Todos = agentInitialTodos(language, "chat", now)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(language, "用户要求修订当前任务计划。", "The operator requested revising the current task plan."),
			SuggestedPrompt: revision,
		}
		if evidenceActionRequiresApproval {
			session.NextAction = &agentNextAction{
				Type:            "approval_required",
				Reason:          agentText(language, "当前证据行动计划存在待审批 gate，需先处理审批后再继续。", "The evidence action plan has a pending approval gate; resolve approvals before continuing."),
				SuggestedPrompt: revision,
			}
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "收到：我会按你的修订要求更新当前任务计划，并在下一轮继续推进。", "Received: I will update the current task plan according to your revision and continue in the next turn."), nil, nil)
	case "pause":
		session.Status = "paused"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = agentText(language, "用户请求暂停当前 Agent 会话。", "The operator requested pausing the current Agent session.")
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" {
				markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", reason)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          reason,
			SuggestedPrompt: firstNonEmpty(strings.TrimSpace(req.Prompt), agentSuggestedContinuePrompt(language, session)),
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已暂停当前 Agent 会话；不会清空上下文。发送继续/Resume 可从当前计划恢复。", "Paused the current Agent session without clearing context. Send continue/resume to proceed from the current plan."), nil, nil)
	case "cancel":
		session.PendingAuthorization = nil
		session.Status = "cancelled"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		cancelJobID = strings.TrimSpace(req.JobID)
		if cancelJobID == "" {
			cancelJobID = m.findActiveJobIDLocked(session, strings.TrimSpace(req.RunID))
		}
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = agentText(language, "用户请求取消当前 Agent 任务。", "The operator requested cancelling the current Agent task.")
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "skipped", now, "", reason)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: firstNonEmpty(strings.TrimSpace(req.Prompt), agentText(language, "开启新任务并说明授权目标与 scope", "start a new task and provide the authorized target and scope")),
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已取消当前 Agent 任务；如存在运行中的 daemon job，会尝试取消。", "Cancelled the current Agent task; if a daemon job is running, I will try to cancel it."), nil, nil)
	case "stop":
		session.PendingAuthorization = nil
		session.Status = "blocked"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		cancelJobID = strings.TrimSpace(req.JobID)
		if cancelJobID == "" {
			cancelJobID = m.findActiveJobIDLocked(session, strings.TrimSpace(req.RunID))
		}
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = agentText(language, "用户请求停止当前任务。", "The operator requested stopping the current task.")
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", reason)
			}
		}
		session.NextAction = &agentNextAction{
			Type:            "blocked",
			Reason:          reason,
			SuggestedPrompt: firstNonEmpty(strings.TrimSpace(req.Prompt), agentSuggestedStopPrompt(language, session)),
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已暂停当前 Agent 任务状态；如存在运行中的 daemon job，会尝试取消。", "The current Agent task state is paused; if a daemon job is running, I will try to cancel it."), nil, nil)
	case "new_task":
		session.Status = "idle"
		session.ActiveTask = nil
		session.Todos = []agentTodo{}
		session.ToolCalls = []agentToolCall{}
		session.ContextRefs = []agentContextRef{}
		session.PendingChanges = []agentWorkspaceChange{}
		session.PendingAuthorization = nil
		session.MemorySummary = nil
		session.NextAction = &agentNextAction{
			Type:   "wait_user",
			Reason: agentText(language, "已开启新任务，等待用户输入目标、scope 或下一步指令。", "New task started; waiting for target, scope, or next instruction."),
		}
		if req.ClearInput {
			session.Messages = []agentMessage{}
		}
		message = m.newMessageLocked(language, "agent", agentText(language, "已开启新的 Agent 任务上下文。", "Started a new Agent task context."), nil, nil)
	default:
		m.mu.Unlock()
		return agentControlResponse{}, services.NewError(services.CodeBadRequest, "unsupported agent control action", map[string]any{"action": req.Action}, nil)
	}
	session.Messages = append(session.Messages, message)
	session.UpdatedAt = now
	compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return agentControlResponse{}, err
	}
	sessionState := cloneAgentSession(session)
	m.mu.Unlock()
	m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionState, agentRuntimeTurnRef{}, compaction)
	m.emitAgentRuntimeEvents(ctx, m.runtimeEngine(), sessionID, sessionState, message)

	if (action == "stop" || action == "cancel") && cancelJobID != "" && svc.jobs != nil {
		if job, cancelErr := svc.jobs.CancelJob(ctx, cancelJobID); cancelErr == nil {
			tool := runningAgentToolCall("POST /api/v1/jobs/{job_id}/cancel", shortAgentID(cancelJobID, 22))
			tool = finishAgentToolCall(tool, nil)
			tool.Result = map[string]any{"job_id": job.JobID, "run_id": job.RunID, "status": string(job.Status)}
			m.appendMessages(sessionID, m.message(language, "tool", agentText(language, "daemon job 已收到取消请求。", "daemon job received the cancellation request."), []agentToolCall{tool}, nil))
		} else if services.ErrorCodeOf(cancelErr) != services.CodeConflict {
			tool := failedAgentToolCall("POST /api/v1/jobs/{job_id}/cancel", cancelErr)
			m.appendMessages(sessionID, m.message(language, "tool", cancelErr.Error(), []agentToolCall{tool}, nil))
		}
	}

	return agentControlResponse{Session: sessionState, Message: message}, nil
}

func (m *agentManager) DecideAgentAuthorization(ctx context.Context, sessionID string, req agentAuthorizationDecisionRequest, svc routerServices) (agentControlResponse, error) {
	if err := contextError(ctx); err != nil {
		return agentControlResponse{}, err
	}
	sessionID, err := normalizeAgentSessionID(sessionID)
	if err != nil {
		return agentControlResponse{}, err
	}
	decision := normalizeAgentAuthorizationDecision(req.Action)
	if decision == "" {
		return agentControlResponse{}, services.NewError(services.CodeBadRequest, "unsupported authorization decision", map[string]any{"action": req.Action}, nil)
	}
	language := normalizeAgentLanguage(req.Language)
	operator := normalizeAgentOperator(req.Operator)
	operationLevel := normalizeAgentOperationLevel(req.OperationLevel)
	now := time.Now().UTC()

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return agentControlResponse{}, loadErr
		}
		session = loaded
	}
	pending := cloneAgentAuthorizationRequest(session.PendingAuthorization)
	if pending == nil || !strings.EqualFold(pending.Status, "pending") {
		m.mu.Unlock()
		return agentControlResponse{}, services.NewError(services.CodeConflict, "no pending agent authorization", map[string]any{"session_id": sessionID}, nil)
	}
	if requestedID := strings.TrimSpace(req.AuthorizationID); requestedID != "" && requestedID != pending.ID {
		m.mu.Unlock()
		return agentControlResponse{}, services.NewError(services.CodeConflict, "authorization id does not match pending request", map[string]any{"authorization_id": requestedID, "pending_authorization_id": pending.ID}, nil)
	}
	session.Operator = operator
	session.Language = language
	if strings.TrimSpace(req.OperationLevel) == "" {
		operationLevel = normalizeAgentOperationLevel(session.OperationLevel)
	}
	if decision == "approve" && !agentOperationLevelAtLeast(operationLevel, pending.RequiredLevel) {
		operationLevel = normalizeAgentOperationLevel(pending.RequiredLevel)
	}
	session.OperationLevel = operationLevel

	var message agentMessage
	switch decision {
	case "deny":
		pending.Status = "denied"
		pending.UpdatedAt = now
		session.PendingAuthorization = nil
		session.Status = "blocked"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", agentText(language, "操作者拒绝授权该工具动作。", "Operator denied authorization for this tool action."))
			}
		}
		reason := firstNonEmpty(strings.TrimSpace(req.Reason), agentText(language, "操作者拒绝授权，挂起动作未执行。", "Operator denied authorization; the pending action was not executed."))
		session.NextAction = &agentNextAction{
			Type:            "blocked",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "换一个低风险方案或重新说明授权边界。", "Choose a lower-risk alternative or restate the authorization boundary."),
		}
		message = agentMessageWithPhase(m.newMessageLocked(language, "agent", agentText(language, fmt.Sprintf("已拒绝授权：`%s` 未执行。", pending.ActionName), fmt.Sprintf("Authorization denied: `%s` was not executed.", pending.ActionName)), nil, nil), "step")
		session.Messages = append(session.Messages, message)
		session.UpdatedAt = now
		compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
		if err := m.persistSessionLocked(session); err != nil {
			m.mu.Unlock()
			return agentControlResponse{}, err
		}
		sessionState := cloneAgentSession(session)
		m.mu.Unlock()
		m.emitRuntimeApprovalResolved(ctx, sessionID, sessionState, *pending, "denied")
		m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionState, agentRuntimeTurnRef{}, compaction)
		m.emitAgentRuntimeEvents(ctx, m.runtimeEngine(), sessionID, sessionState, message)
		return agentControlResponse{Session: sessionState, Message: message}, nil
	case "approve":
		pending.Status = "approved"
		pending.UpdatedAt = now
		session.PendingAuthorization = nil
		session.Status = "running"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(language, "授权已通过，继续执行挂起工具动作。", "Authorization approved; continuing the pending tool action."),
			SuggestedPrompt: pending.OriginalPrompt,
			AutoContinue:    true,
		}
		message = agentMessageWithPhase(m.newMessageLocked(language, "agent", agentText(language, fmt.Sprintf("已授权，继续执行挂起动作：`%s`。", pending.ActionName), fmt.Sprintf("Authorized. Continuing pending action: `%s`.", pending.ActionName)), nil, nil), "step")
		session.Messages = append(session.Messages, message)
		session.UpdatedAt = now
		compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
		if err := m.persistSessionLocked(session); err != nil {
			m.mu.Unlock()
			return agentControlResponse{}, err
		}
		approvedSessionState := cloneAgentSession(session)
		m.mu.Unlock()
		m.emitRuntimeApprovalResolved(ctx, sessionID, approvedSessionState, *pending, "approved")
		m.emitRuntimeMemoryCompacted(ctx, sessionID, approvedSessionState, agentRuntimeTurnRef{}, compaction)
		m.emitAgentRuntimeEvents(ctx, m.runtimeEngine(), sessionID, approvedSessionState, message)
	default:
		m.mu.Unlock()
		return agentControlResponse{}, services.NewError(services.CodeBadRequest, "unsupported authorization decision", map[string]any{"action": req.Action}, nil)
	}

	action := agentPlannerAction{Name: pending.ActionName, Arguments: cloneMapStringAny(pending.Arguments)}
	actionMessages, actionToolCalls, execErr := m.executeAgentPlannerAction(ctx, sessionID, language, operator, operationLevel, svc, action, req.AuthProfiles, req.DefaultAuthProfileID)
	if execErr != nil {
		tool := failedAgentToolCall(pending.ActionName, execErr)
		tool.Result = map[string]any{"action": pending.ActionName, "authorization_id": pending.ID}
		actionMessages = []agentMessage{m.message(language, "tool", execErr.Error(), []agentToolCall{tool}, nil)}
		actionToolCalls = []agentToolCall{tool}
	}
	actionMessages = agentMessagesWithPhase(actionMessages, "step")
	actionToolCalls = agentToolCallsWithPhase(actionToolCalls, "step")
	authorizationMetadata := map[string]interface{}{
		"authorization_id":     pending.ID,
		"authorization_status": "approved",
		"action_name":          pending.ActionName,
	}
	for i := range actionToolCalls {
		actionToolCalls[i] = withAgentToolMetadata(actionToolCalls[i], authorizationMetadata)
	}
	for i := range actionMessages {
		for j := range actionMessages[i].ToolCalls {
			actionMessages[i].ToolCalls[j] = withAgentToolMetadata(actionMessages[i].ToolCalls[j], authorizationMetadata)
		}
	}
	if len(actionMessages) > 0 {
		m.appendMessages(sessionID, actionMessages...)
	}
	responseMessages := append([]agentMessage{message}, actionMessages...)
	responseToolCalls := append([]agentToolCall{}, actionToolCalls...)

	llmReq := sendAgentMessageRequest{
		Operator:             operator,
		Language:             language,
		OperationLevel:       operationLevel,
		Provider:             req.Provider,
		Model:                req.Model,
		APIKey:               req.APIKey,
		BaseURL:              req.BaseURL,
		Instructions:         req.Instructions,
		AuthProfiles:         req.AuthProfiles,
		DefaultAuthProfileID: req.DefaultAuthProfileID,
	}
	if execErr == nil && !req.SkipSynthesis && agentLLMConfigured(llmReq) && len(actionMessages) > 0 {
		sessionState, getErr := m.GetSession(ctx, sessionID)
		if getErr == nil {
			history := cloneAgentMessages(sessionState.Messages)
			memorySummary := cloneAgentMemorySummary(sessionState.MemorySummary)
			synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.synthesizeAgentToolResults(ctx, sessionID, agentRuntimeTurnRef{}, language, llmReq, history, memorySummary, actionMessages)
			if synthesisErr != nil {
				warningTool := failedAgentToolCall("LLM synthesis", synthesisErr)
				warningTool = agentToolCallWithPhase(warningTool, "step_replan")
				warningMessage := agentMessageWithPhase(m.message(
					language,
					"tool",
					agentText(language, "授权动作已执行，但 StepReplan 总结失败："+synthesisErr.Error(), "Authorized action executed, but StepReplan synthesis failed: "+synthesisErr.Error()),
					[]agentToolCall{warningTool},
					nil,
				), "step_replan")
				m.appendMessages(sessionID, warningMessage)
				responseMessages = append(responseMessages, warningMessage)
				responseToolCalls = append(responseToolCalls, warningTool)
			} else if len(synthesisMessages) > 0 {
				m.appendMessagesWithPlanner(sessionID, synthesisPlan, synthesisMessages...)
				responseMessages = append(responseMessages, synthesisMessages...)
				responseToolCalls = append(responseToolCalls, synthesisToolCalls...)
				if synthesisPlan != nil && synthesisPlan.NextAction != nil && synthesisPlan.NextAction.AutoContinue {
					nextState, _ := m.GetSession(ctx, sessionID)
					loopHistory := cloneAgentMessages(nextState.Messages)
					followupMessages, followupToolCalls := m.runAgentReplanActionContinuation(ctx, sessionID, agentRuntimeTurnRef{}, pending.OriginalPrompt, language, operator, operationLevel, llmReq, loopHistory, memorySummary, svc, synthesisPlan, reqMaxAgentReplans(llmReq)-1)
					responseMessages = append(responseMessages, followupMessages...)
					responseToolCalls = append(responseToolCalls, followupToolCalls...)
				}
			}
		}
	}

	sessionState, getErr := m.GetSession(ctx, sessionID)
	if getErr != nil {
		return agentControlResponse{}, getErr
	}
	_ = responseToolCalls
	return agentControlResponse{Session: sessionState, Message: message}, nil
}

func (m *agentManager) SendMessage(ctx context.Context, sessionID string, req sendAgentMessageRequest, svc routerServices) (resp sendAgentMessageResponse, err error) {
	if err := contextError(ctx); err != nil {
		return sendAgentMessageResponse{}, err
	}
	sessionID, err = normalizeAgentSessionID(sessionID)
	if err != nil {
		return sendAgentMessageResponse{}, err
	}
	prompt := strings.TrimSpace(req.Message)
	if prompt == "" {
		return sendAgentMessageResponse{}, services.NewError(services.CodeBadRequest, "message is required", map[string]any{"field": "message"}, nil)
	}
	operator := normalizeAgentOperator(req.Operator)
	language := normalizeAgentLanguage(req.Language)
	operationLevel := normalizeAgentOperationLevel(req.OperationLevel)
	var turnRef agentRuntimeTurnRef
	defer func() {
		m.finishRuntimeTurn(ctx, sessionID, turnRef, resp, err)
	}()

	var history []agentMessage
	var memorySummary *agentMemorySummary
	userMessage := agentMessage{}
	sessionSnapshot := agentSession{}
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return sendAgentMessageResponse{}, loadErr
		}
		session = loaded
	}
	session.Operator = operator
	session.Language = language
	session.OperationLevel = operationLevel
	userMessage = m.newMessageLocked(language, "user", prompt, nil, nil)
	session.Messages = append(session.Messages, userMessage)
	session.UpdatedAt = userMessage.Timestamp
	if m.shouldStartNewAgentTaskLocked(session, prompt, language) {
		m.startAgentTaskLocked(session, prompt, language)
	} else {
		m.touchAgentTaskForMessageLocked(session, prompt, language)
	}
	compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	history = cloneAgentMessages(session.Messages)
	memorySummary = cloneAgentMemorySummary(session.MemorySummary)
	_ = m.persistSessionLocked(session)
	sessionSnapshot = cloneAgentSession(session)
	m.mu.Unlock()
	turnRef = m.beginRuntimeTurn(ctx, sessionID, prompt, firstNonEmpty(req.Origin, "user"), operationLevel, sessionSnapshot, userMessage)
	m.emitRuntimeMemoryCompacted(ctx, sessionID, sessionSnapshot, turnRef, compaction)

	if controlReq, ok := agentControlRequestFromPrompt(prompt, operator, language); ok {
		controlReq.OperationLevel = operationLevel
		controlResp, controlErr := m.ControlSession(ctx, sessionID, controlReq, svc)
		if controlErr != nil {
			errMessage := m.message(language, "agent", controlErr.Error(), []agentToolCall{failedAgentToolCall("agent.control", controlErr)}, nil)
			m.appendMessages(sessionID, errMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, errMessage}, errMessage.ToolCalls, sessionState), nil
		}
		if controlReq.Action == "continue" && !agentLLMConfigured(req) {
			if contextMessages, contextToolCalls, ok := m.handleAgentActiveRunContinue(ctx, sessionID, language, svc); ok {
				m.appendMessages(sessionID, contextMessages...)
				sessionState, getErr := m.GetSession(ctx, sessionID)
				if getErr != nil {
					sessionState = controlResp.Session
				}
				responseMessages := append([]agentMessage{userMessage, controlResp.Message}, contextMessages...)
				return buildAgentSendResponse(sessionID, responseMessages, contextToolCalls, sessionState), nil
			}
			if autoPrompt := agentExecutableAutoContinuePrompt(controlResp.Session.NextAction); autoPrompt != "" && agentCanAutoRouteContinuePrompt(autoPrompt) {
				autoMessages, autoToolCalls, routeErr := m.routePrompt(ctx, sessionID, autoPrompt, language, operator, operationLevel, svc)
				if routeErr != nil {
					errMessage := m.message(language, "agent", routeErr.Error(), []agentToolCall{failedAgentToolCall("agent.auto_continue", routeErr)}, nil)
					m.appendMessages(sessionID, errMessage)
					sessionState, getErr := m.GetSession(ctx, sessionID)
					if getErr != nil {
						sessionState = controlResp.Session
					}
					return buildAgentSendResponse(sessionID, []agentMessage{userMessage, controlResp.Message, errMessage}, errMessage.ToolCalls, sessionState), nil
				}
				autoMessages, autoToolCalls = tagAgentAutoContinueMessages(autoMessages, autoToolCalls, autoPrompt)
				m.appendMessages(sessionID, autoMessages...)
				sessionState, getErr := m.GetSession(ctx, sessionID)
				if getErr != nil {
					sessionState = controlResp.Session
				}
				responseMessages := append([]agentMessage{userMessage, controlResp.Message}, autoMessages...)
				return buildAgentSendResponse(sessionID, responseMessages, autoToolCalls, sessionState), nil
			}
		}
		if agentLLMConfigured(req) && (controlReq.Action == "continue" || controlReq.Action == "revise") {
			sessionState, getErr := m.GetSession(ctx, sessionID)
			if getErr == nil {
				history = cloneAgentMessages(sessionState.Messages)
				memorySummary = cloneAgentMemorySummary(sessionState.MemorySummary)
			}
			messages, toolCalls, err := m.runAgentLLMToolLoop(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc)
			if err == nil {
				responseMessages := append([]agentMessage{userMessage, controlResp.Message}, messages...)
				sessionState, _ = m.GetSession(ctx, sessionID)
				return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
			}
			warningTool := failedAgentToolCall("LLM chat", err)
			if len(toolCalls) > 0 {
				warningTool = toolCalls[0]
			}
			warningMessage := m.message(
				language,
				"tool",
				agentText(language, "AI provider 调用失败，已保留控制状态更新："+err.Error(), "AI provider call failed; the control-state update was preserved: "+err.Error()),
				[]agentToolCall{warningTool},
				nil,
			)
			m.appendMessages(sessionID, warningMessage)
			sessionState, _ = m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, controlResp.Message, warningMessage}, []agentToolCall{warningTool}, sessionState), nil
		}
		sessionState, getErr := m.GetSession(ctx, sessionID)
		if getErr != nil {
			sessionState = controlResp.Session
		}
		return buildAgentSendResponse(sessionID, []agentMessage{userMessage, controlResp.Message}, nil, sessionState), nil
	}

	if sessionState, getErr := m.GetSession(ctx, sessionID); getErr == nil &&
		sessionState.PendingAuthorization != nil &&
		strings.EqualFold(sessionState.PendingAuthorization.Status, "pending") {
		blockMessage := agentMessageWithPhase(m.message(language, "agent", agentText(language, "当前任务已暂停在工具授权门。请先在授权卡片中点击“允许并继续”或“拒绝”；我不会在授权前执行新的步骤。", "The current task is paused at a tool authorization gate. Approve or deny the authorization card first; I will not execute new steps before authorization."), nil, nil), "step")
		m.appendMessages(sessionID, blockMessage)
		latestState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, []agentMessage{userMessage, blockMessage}, nil, latestState), nil
	}

	if agentLLMConfigured(req) && agentShouldRouteBeforeLLMSynthesis(prompt) {
		messages, toolCalls, routeErr := m.routePrompt(ctx, sessionID, prompt, language, operator, operationLevel, svc)
		if routeErr != nil {
			errMessage := m.message(language, "agent", routeErr.Error(), []agentToolCall{failedAgentToolCall("agent.route", routeErr)}, nil)
			m.appendMessages(sessionID, errMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, errMessage}, errMessage.ToolCalls, sessionState), nil
		}
		messages = agentMessagesWithPhase(messages, "step")
		toolCalls = agentToolCallsWithPhase(toolCalls, "step")
		m.appendMessages(sessionID, messages...)
		responseMessages := append([]agentMessage{userMessage}, messages...)
		if len(toolCalls) > 0 {
			synthesisHistory := append(cloneAgentMessages(history), messages...)
			synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.synthesizeAgentToolResults(ctx, sessionID, turnRef, language, req, synthesisHistory, memorySummary, messages)
			if synthesisErr != nil {
				warningTool := failedAgentToolCall("LLM synthesis", synthesisErr)
				warningTool = withAgentToolMetadata(warningTool, map[string]interface{}{"phase": "tool_synthesis"})
				warningTool = agentToolCallWithPhase(warningTool, "step_replan")
				warningMessage := agentMessageWithPhase(m.message(
					language,
					"tool",
					agentText(language, "工具已执行，但 AI 结果总结失败："+synthesisErr.Error(), "Tools executed, but AI synthesis failed: "+synthesisErr.Error()),
					[]agentToolCall{warningTool},
					nil,
				), "step_replan")
				m.appendMessages(sessionID, warningMessage)
				responseMessages = append(responseMessages, warningMessage)
				toolCalls = append(toolCalls, warningTool)
			} else if len(synthesisMessages) > 0 {
				m.appendMessagesWithPlanner(sessionID, synthesisPlan, synthesisMessages...)
				responseMessages = append(responseMessages, synthesisMessages...)
				toolCalls = append(toolCalls, synthesisToolCalls...)
				if synthesisPlan != nil && synthesisPlan.NextAction != nil && synthesisPlan.NextAction.AutoContinue {
					loopHistory := append(synthesisHistory, synthesisMessages...)
					followupMessages, followupToolCalls := m.runAgentReplanActionContinuation(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, loopHistory, memorySummary, svc, synthesisPlan, reqMaxAgentReplans(req)-1)
					responseMessages = append(responseMessages, followupMessages...)
					toolCalls = append(toolCalls, followupToolCalls...)
				}
			}
		}
		sessionState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
	}

	if agentLLMConfigured(req) {
		messages, toolCalls, err := m.runAgentLLMToolLoop(ctx, sessionID, turnRef, prompt, language, operator, operationLevel, req, history, memorySummary, svc)
		if err == nil {
			responseMessages := append([]agentMessage{userMessage}, messages...)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
		}
		warningTool := failedAgentToolCall("LLM chat", err)
		if len(toolCalls) > 0 {
			warningTool = toolCalls[0]
		}
		warningMessage := m.message(
			language,
			"tool",
			agentText(language, "AI provider 调用失败，已回退到本地规则 Agent："+err.Error(), "AI provider call failed; falling back to the local rule Agent: "+err.Error()),
			[]agentToolCall{warningTool},
			nil,
		)
		if action, ok := agentManualAuthorizationActionFromPrompt(prompt); ok {
			actionName := normalizeAgentPlannerActionName(action.Name)
			requiredLevel := agentOperationLevelRequiredForPlannerAction(actionName, action.Arguments)
			authMessage := m.requestAgentAuthorization(
				sessionID,
				language,
				prompt,
				action,
				actionName,
				requiredLevel,
				"manual_tool_authorization_required",
				agentText(
					language,
					fmt.Sprintf("AI provider 失败后检测到 %s 需要人工确认；已暂停执行，等待授权。", actionName),
					fmt.Sprintf("After the AI provider failed, %s was detected as requiring human confirmation; execution is paused until authorization.", actionName),
				),
			)
			m.appendMessages(sessionID, warningMessage, authMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, warningMessage, authMessage}, []agentToolCall{warningTool}, sessionState), nil
		}
		messages, toolCalls, routeErr := m.routePrompt(ctx, sessionID, prompt, language, operator, operationLevel, svc)
		if routeErr != nil {
			errMessage := m.message(language, "agent", routeErr.Error(), []agentToolCall{failedAgentToolCall("agent.route", routeErr)}, nil)
			m.appendMessages(sessionID, warningMessage, errMessage)
			sessionState, _ := m.GetSession(ctx, sessionID)
			return buildAgentSendResponse(sessionID, []agentMessage{userMessage, warningMessage, errMessage}, append([]agentToolCall{warningTool}, errMessage.ToolCalls...), sessionState), nil
		}
		messages = append([]agentMessage{warningMessage}, messages...)
		toolCalls = append([]agentToolCall{warningTool}, toolCalls...)
		m.appendMessages(sessionID, messages...)
		responseMessages := append([]agentMessage{userMessage}, messages...)
		sessionState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
	}

	messages, toolCalls, err := m.routePrompt(ctx, sessionID, prompt, language, operator, operationLevel, svc)
	if err != nil {
		errMessage := m.message(language, "agent", err.Error(), []agentToolCall{failedAgentToolCall("agent.route", err)}, nil)
		m.appendMessages(sessionID, errMessage)
		sessionState, _ := m.GetSession(ctx, sessionID)
		return buildAgentSendResponse(sessionID, []agentMessage{userMessage, errMessage}, errMessage.ToolCalls, sessionState), nil
	}

	m.appendMessages(sessionID, messages...)
	responseMessages := append([]agentMessage{userMessage}, messages...)
	if agentLLMConfigured(req) && len(toolCalls) > 0 {
		synthesisHistory := append(cloneAgentMessages(history), messages...)
		synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.synthesizeAgentToolResults(ctx, sessionID, turnRef, language, req, synthesisHistory, memorySummary, messages)
		if synthesisErr != nil {
			warningTool := failedAgentToolCall("LLM synthesis", synthesisErr)
			warningTool = withAgentToolMetadata(warningTool, map[string]interface{}{"phase": "tool_synthesis"})
			warningMessage := m.message(
				language,
				"tool",
				agentText(language, "工具已执行，但 AI 结果总结失败："+synthesisErr.Error(), "Tools executed, but AI synthesis failed: "+synthesisErr.Error()),
				[]agentToolCall{warningTool},
				nil,
			)
			m.appendMessages(sessionID, warningMessage)
			responseMessages = append(responseMessages, warningMessage)
			toolCalls = append(toolCalls, warningTool)
		} else if len(synthesisMessages) > 0 {
			m.appendMessagesWithPlanner(sessionID, synthesisPlan, synthesisMessages...)
			responseMessages = append(responseMessages, synthesisMessages...)
			toolCalls = append(toolCalls, synthesisToolCalls...)
		}
	}
	sessionState, _ := m.GetSession(ctx, sessionID)
	return buildAgentSendResponse(sessionID, responseMessages, toolCalls, sessionState), nil
}

func (m *agentManager) handleAgentActiveRunContinue(ctx context.Context, sessionID, language string, svc routerServices) ([]agentMessage, []agentToolCall, bool) {
	sessionState, err := m.GetSession(ctx, sessionID)
	if err != nil || sessionState.ActiveTask == nil {
		return nil, nil, false
	}
	runID := strings.TrimSpace(sessionState.ActiveTask.ActiveRunID)
	if runID == "" {
		return nil, nil, false
	}
	evidenceStage := agentEvidenceActionStageFromTodos(sessionState.Todos)
	contextKind := "events"
	if detail, detailErr := svc.runs.GetRun(ctx, runID); detailErr == nil {
		switch {
		case detail.Summary.Counts.PendingApprovals > 0:
			contextKind = "approvals"
		case detail.Summary.Report.JSON:
			contextKind = "report_json"
		case detail.Summary.Counts.Findings > 0:
			contextKind = "findings"
		case detail.Summary.Counts.Artifacts > 0:
			contextKind = "artifact_index"
		default:
			contextKind = "events"
		}
	}
	prompt := fmt.Sprintf("读取运行 %s 的 %s 上下文", runID, contextKind)
	messages, toolCalls, routeErr := m.handleAgentRunContextAnalysis(ctx, prompt, language, svc)
	if routeErr != nil || len(messages) == 0 {
		return nil, nil, false
	}
	if len(toolCalls) > 0 && agentRunContextHasFindings(toolCalls[0].Result) {
		conclusion := agentEvidenceActionStageConclusion(language, runID, evidenceStage, toolCalls[0].Result)
		if conclusion != "" {
			messages = append(messages, m.message(language, "agent", conclusion, nil, []agentLink{{To: "/scan/" + runID, Label: agentText(language, "打开运行详情", "Open run detail"), Kind: "run"}}))
		}
	}
	return messages, toolCalls, true
}

func (m *agentManager) executeAgentPlannerAction(ctx context.Context, sessionID, language, operator, operationLevel string, svc routerServices, action agentPlannerAction, authProfiles []agentHTTPAuthProfile, defaultAuthProfileID string) ([]agentMessage, []agentToolCall, error) {
	name := normalizeAgentPlannerActionName(action.Name)
	args := action.Arguments
	if args == nil {
		args = map[string]any{}
	}

	if spec, ok := toolRegistry().Lookup(name); ok && spec.Handler != nil {
		return m.executeAgentRegistryHandler(ctx, sessionID, language, operator, operationLevel, svc, spec, args, authProfiles, defaultAuthProfileID)
	}

	// Deprecated: legacy per-tool dispatch. Migrate cases into ToolSpec.Handler
	// in small slices until the registry is the only execution source.
	switch name {
	case "scan.web.safe":
		target := firstNonEmpty(
			stringFromAny(args["target"]),
			stringFromAny(args["url"]),
			stringFromAny(args["input"]),
		)
		scope := firstNonEmpty(
			stringFromAny(args["scope"]),
			stringFromAny(args["scope_url"]),
			target,
		)
		prompt := "扫描 " + target
		if scope != "" {
			prompt += " scope=" + scope
		}
		return m.handleAgentScan(ctx, prompt, language, svc)
	case "ports.scan.safe":
		target := firstNonEmpty(
			stringFromAny(args["target"]),
			stringFromAny(args["host"]),
			stringFromAny(args["url"]),
			stringFromAny(args["input"]),
		)
		if target == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "ports.scan.safe requires target or host", map[string]any{"action": action.Name}, nil)
		}
		prompt := "扫描 " + target + " 开放端口"
		if portsSpec := agentPortSpecFromPlannerArgument(args["ports"]); portsSpec != "" {
			prompt += " ports=" + portsSpec
		}
		return m.handleAgentPortScan(ctx, prompt, language)
	case "workspace.command":
		cmd := firstNonEmpty(
			stringFromAny(args["command"]),
			stringFromAny(args["cmd"]),
			stringFromAny(args["input"]),
		)
		if cmd == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.command requires command", map[string]any{"action": action.Name}, nil)
		}
		if !agentPlannerCommandAllowed(cmd) {
			return nil, nil, services.NewError(services.CodePolicyDenied, "planner command is not in the low-risk allowlist; ask the operator to run it explicitly", map[string]any{"command": cmd}, nil)
		}
		return m.handleAgentCommand(ctx, sessionID, "command: "+cmd, language)
	case "workspace.files.list":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["directory"]),
			stringFromAny(args["dir"]),
			stringFromAny(args["input"]),
			".",
		)
		return m.handleAgentWorkspaceList(sessionID, "path="+relPath, language)
	case "workspace.files.tree":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["directory"]),
			stringFromAny(args["dir"]),
			stringFromAny(args["input"]),
			".",
		)
		depth := intFromAny(args["depth"])
		if depth <= 0 {
			depth = 3
		}
		limit := intFromAny(args["limit"])
		if limit <= 0 {
			limit = 120
		}
		includeHidden := boolFromAny(args["include_hidden"])
		return m.handleAgentWorkspaceTree(sessionID, relPath, depth, limit, includeHidden, language)
	case "workspace.files.read":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
			stringFromAny(args["input"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.read requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceRead(sessionID, "file="+relPath, language)
	case "workspace.search":
		query := firstNonEmpty(
			stringFromAny(args["query"]),
			stringFromAny(args["pattern"]),
			stringFromAny(args["text"]),
			stringFromAny(args["input"]),
		)
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["directory"]),
			stringFromAny(args["dir"]),
			".",
		)
		if query == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.search requires query", map[string]any{"action": action.Name}, nil)
		}
		limit := intFromAny(args["limit"])
		if limit <= 0 {
			limit = 80
		}
		return m.handleAgentWorkspaceSearch(sessionID, query, relPath, boolFromAny(args["regex"]), boolFromAny(args["case_sensitive"]), limit, language)
	case "workspace.files.write":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)
		content := firstNonEmpty(
			stringFromAny(args["content"]),
			stringFromAny(args["body"]),
			stringFromAny(args["text"]),
			stringFromAny(args["input"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.write requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceWrite(sessionID, relPath, content, language)
	case "workspace.files.patch":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)
		oldText := firstNonEmpty(
			stringFromAny(args["old"]),
			stringFromAny(args["old_text"]),
			stringFromAny(args["before"]),
			stringFromAny(args["search"]),
		)
		newText := firstNonEmpty(
			stringFromAny(args["new"]),
			stringFromAny(args["new_text"]),
			stringFromAny(args["after"]),
			stringFromAny(args["replace"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.patch requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspacePatch(sessionID, relPath, oldText, newText, language)
	case "workspace.files.diff":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)
		oldText := firstNonEmpty(
			stringFromAny(args["old"]),
			stringFromAny(args["old_text"]),
			stringFromAny(args["before"]),
			stringFromAny(args["search"]),
		)
		newText := firstNonEmpty(
			stringFromAny(args["new"]),
			stringFromAny(args["new_text"]),
			stringFromAny(args["after"]),
			stringFromAny(args["replace"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.diff requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceDiff(sessionID, relPath, oldText, newText, language)
	case "workspace.files.diff_many":
		changes := workspaceChangeInputsFromAny(args["changes"])
		if len(changes) == 0 {
			changes = workspaceChangeInputsFromAny(args["input"])
		}
		return m.handleAgentWorkspaceDiffMany(sessionID, changes, language)
	case "workspace.files.rebase":
		changeID := firstNonEmpty(
			stringFromAny(args["change_id"]),
			stringFromAny(args["id"]),
		)
		oldText := firstNonEmpty(
			stringFromAny(args["old"]),
			stringFromAny(args["old_text"]),
			stringFromAny(args["before"]),
			stringFromAny(args["search"]),
		)
		newText := firstNonEmpty(
			stringFromAny(args["new"]),
			stringFromAny(args["new_text"]),
			stringFromAny(args["after"]),
			stringFromAny(args["replace"]),
		)
		return m.handleAgentWorkspaceRebase(sessionID, changeID, oldText, newText, language)
	case "workspace.files.apply":
		changeID := firstNonEmpty(
			stringFromAny(args["change_id"]),
			stringFromAny(args["id"]),
			stringFromAny(args["input"]),
		)
		groupID := firstNonEmpty(stringFromAny(args["group_id"]), stringFromAny(args["group"]))
		if groupID != "" {
			return m.handleAgentWorkspaceApplyGroup(sessionID, groupID, language)
		}
		if boolFromAny(args["all"]) || strings.EqualFold(changeID, "all") || strings.EqualFold(stringFromAny(args["scope"]), "all") {
			return m.handleAgentWorkspaceApplyAll(sessionID, language)
		}
		if changeID == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.apply requires change_id", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceApply(sessionID, changeID, language)
	case "workspace.files.discard":
		changeID := firstNonEmpty(
			stringFromAny(args["change_id"]),
			stringFromAny(args["id"]),
			stringFromAny(args["input"]),
		)
		groupID := firstNonEmpty(stringFromAny(args["group_id"]), stringFromAny(args["group"]))
		if groupID != "" {
			return m.handleAgentWorkspaceDiscardGroup(sessionID, groupID, language)
		}
		if boolFromAny(args["all"]) || strings.EqualFold(changeID, "all") || strings.EqualFold(stringFromAny(args["scope"]), "all") {
			return m.handleAgentWorkspaceDiscardAll(sessionID, language)
		}
		if changeID == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.discard requires change_id", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceDiscard(sessionID, changeID, language)
	case "workspace.files.delete":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
			stringFromAny(args["input"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.delete requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceDelete(sessionID, relPath, language)
	case "workspace.files.move":
		fromPath := firstNonEmpty(
			stringFromAny(args["from"]),
			stringFromAny(args["source"]),
			stringFromAny(args["old"]),
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)
		toPath := firstNonEmpty(
			stringFromAny(args["to"]),
			stringFromAny(args["target"]),
			stringFromAny(args["new"]),
			stringFromAny(args["dest"]),
			stringFromAny(args["destination"]),
		)
		if fromPath == "" || toPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.move requires from and to", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceMove(sessionID, fromPath, toPath, language)
	case "workspace.files.copy":
		fromPath := firstNonEmpty(
			stringFromAny(args["from"]),
			stringFromAny(args["source"]),
			stringFromAny(args["old"]),
			stringFromAny(args["path"]),
			stringFromAny(args["file"]),
			stringFromAny(args["filename"]),
		)
		toPath := firstNonEmpty(
			stringFromAny(args["to"]),
			stringFromAny(args["target"]),
			stringFromAny(args["new"]),
			stringFromAny(args["dest"]),
			stringFromAny(args["destination"]),
		)
		if fromPath == "" || toPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.copy requires from and to", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceCopy(sessionID, fromPath, toPath, language)
	case "workspace.files.mkdir":
		relPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["directory"]),
			stringFromAny(args["dir"]),
			stringFromAny(args["input"]),
		)
		if relPath == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "workspace.files.mkdir requires path", map[string]any{"action": action.Name}, nil)
		}
		return m.handleAgentWorkspaceMkdir(sessionID, relPath, language)
	case "http.request":
		targetURL := firstNonEmpty(
			stringFromAny(args["url"]),
			stringFromAny(args["target"]),
			stringFromAny(args["input"]),
		)
		method := firstNonEmpty(stringFromAny(args["method"]), "GET")
		if targetURL == "" {
			return nil, nil, services.NewError(services.CodeBadRequest, "http.request requires url", map[string]any{"action": action.Name}, nil)
		}
		targetURL = agentNormalizeURLInput(targetURL)
		headers := stringMapFromAny(args["headers"])
		if contentType := stringFromAny(args["content_type"]); contentType != "" {
			headers["Content-Type"] = contentType
		}
		authProfileID := firstNonEmpty(
			stringFromAny(args["auth_profile_id"]),
			stringFromAny(args["auth_profile"]),
			stringFromAny(args["profile_id"]),
			stringFromAny(args["profile"]),
		)
		profileHeaders, profileUsage, profileErr := agentResolveHTTPAuthProfileHeaders(authProfiles, authProfileID, defaultAuthProfileID, targetURL)
		if profileErr != nil {
			return nil, nil, profileErr
		}
		for key, value := range profileHeaders {
			if _, exists := headers[key]; !exists {
				headers[key] = value
			}
		}
		body := firstNonEmpty(
			stringFromAny(args["body"]),
			stringFromAny(args["content"]),
			stringFromAny(args["data"]),
			stringFromAny(args["input_body"]),
		)
		return m.handleAgentHTTPRequest(ctx, method, targetURL, headers, body, language, profileUsage)
	case "daemon.health":
		return m.handleAgentHealth(ctx, language, svc)
	case "plugins.list":
		return m.handleAgentPlugins(ctx, language, svc)
	case "runs.list":
		return m.handleAgentRuns(ctx, language, svc)
	case "runs.context":
		runID := firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
			stringFromAny(args["id"]),
		)
		contextKind := firstNonEmpty(
			stringFromAny(args["context"]),
			stringFromAny(args["kind"]),
			stringFromAny(args["section"]),
			stringFromAny(args["type"]),
		)
		return m.handleAgentRunContextTool(ctx, sessionID, runID, contextKind, language, svc)
	case "reports.draft":
		runID := firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
			stringFromAny(args["id"]),
		)
		outputPath := firstNonEmpty(
			stringFromAny(args["path"]),
			stringFromAny(args["output_path"]),
			stringFromAny(args["file"]),
		)
		return m.handleAgentReportDraft(ctx, sessionID, runID, language, svc, outputPath)
	case "runs.approvals.list":
		runID := firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
			stringFromAny(args["id"]),
		)
		if runID == "" {
			return m.handleAgentApprovals(ctx, language, svc)
		}
		return m.handleAgentRunContextAnalysis(ctx, "读取运行 "+runID+" 的 approvals 审批队列", language, svc)
	case "runs.approvals.approve", "runs.approvals.deny":
		runID := firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
		)
		approvalID := firstNonEmpty(
			stringFromAny(args["approval_id"]),
			stringFromAny(args["approval"]),
			stringFromAny(args["step_id"]),
			stringFromAny(args["step"]),
			stringFromAny(args["id"]),
		)
		comment := stringFromAny(args["comment"])
		verb := "批准"
		if name == "runs.approvals.deny" {
			verb = "拒绝"
		}
		prompt := verb + "当前审批"
		if runID != "" && approvalID != "" {
			prompt = fmt.Sprintf("%s运行 %s 的 %s", verb, runID, approvalID)
		} else if runID != "" {
			prompt = fmt.Sprintf("%s运行 %s 的当前审批", verb, runID)
		} else if approvalID != "" {
			prompt = fmt.Sprintf("%s %s", verb, approvalID)
		}
		if strings.TrimSpace(comment) != "" {
			prompt += " comment=" + comment
		}
		return m.handleAgentApprovalDecision(ctx, sessionID, prompt, language, operator, svc)
	case "runs.resume_approved":
		runID := firstNonEmpty(
			stringFromAny(args["run_id"]),
			stringFromAny(args["run"]),
			stringFromAny(args["id"]),
		)
		prompt := "恢复已批准当前运行"
		if runID != "" {
			prompt = "恢复已批准运行 " + runID
		}
		return m.handleAgentResumeApproved(ctx, sessionID, prompt, language, operator, svc)
	case "schedule.create":
		return m.handleAgentScheduleCreate(sessionID, language, operator, svc, args)
	case "schedule.list":
		return m.handleAgentScheduleList(language, svc)
	case "schedule.delete":
		return m.handleAgentScheduleDelete(language, svc, args)
	case "web.search":
		return m.handleAgentWebSearch(ctx, language, svc, args)
	case "web.fetch":
		return m.handleAgentWebFetch(ctx, language, args)
	default:
		return nil, nil, services.NewError(services.CodeUnsupported, "unsupported planner action", map[string]any{"action": action.Name}, nil)
	}
}

func (m *agentManager) routePrompt(ctx context.Context, sessionID, prompt, language, operator, operationLevel string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	requiredLevel := agentOperationLevelRequiredForPrompt(prompt)
	if !agentOperationLevelAtLeast(operationLevel, requiredLevel) {
		return []agentMessage{m.message(
			language,
			"agent",
			agentOperationLevelDeniedError(language, operationLevel, requiredLevel, "natural_language_route").Error(),
			nil,
			nil,
		)}, nil, nil
	}
	switch {
	case agentIsScheduleCreateIntent(prompt):
		return m.handleAgentScheduleCreate(sessionID, language, operator, svc, agentScheduleArgsFromPrompt(prompt, language))
	case agentIsScheduleListIntent(prompt):
		return m.handleAgentScheduleList(language, svc)
	case agentIsScheduleDeleteIntent(prompt):
		return m.handleAgentScheduleDelete(language, svc, map[string]any{"id": agentExtractScheduleID(prompt)})
	case agentIsResumeApprovedIntent(prompt):
		return m.handleAgentResumeApproved(ctx, sessionID, prompt, language, operator, svc)
	case agentIsApprovalDecisionIntent(prompt):
		return m.handleAgentApprovalDecision(ctx, sessionID, prompt, language, operator, svc)
	case agentIsReportDraftIntent(prompt):
		return m.handleAgentReportDraft(ctx, sessionID, agentExtractRunID(prompt), language, svc, "")
	case agentIsRunContextAnalysisIntent(prompt):
		return m.handleAgentRunContextTool(ctx, sessionID, agentExtractRunID(prompt), agentRunContextKind(prompt), language, svc)
	case agentIsPortScanIntent(prompt):
		return m.handleAgentPortScan(ctx, prompt, language)
	case agentHasCapabilityGapIntent(prompt):
		plan := agentCapabilityGapPlannerFromPrompt(prompt, "", language)
		if plan != nil {
			message := agentMessageWithPhase(m.message(language, "agent", plan.Answer, nil, nil), "plan")
			message.Reasoning = trimSummary(plan.Reasoning, 800)
			m.appendMessagesWithPlanner(sessionID, plan)
			return []agentMessage{message}, nil, nil
		}
		return []agentMessage{m.message(language, "agent", agentExplain(language), nil, nil)}, nil, nil
	case agentIsScanIntent(prompt):
		return m.handleAgentScan(ctx, prompt, language, svc)
	case agentIsCommandIntent(prompt):
		return m.handleAgentCommand(ctx, sessionID, prompt, language)
	case agentIsApprovalIntent(prompt):
		return m.handleAgentApprovals(ctx, language, svc)
	case agentIsHealthIntent(prompt):
		return m.handleAgentHealth(ctx, language, svc)
	case agentIsPluginIntent(prompt):
		return m.handleAgentPlugins(ctx, language, svc)
	case agentIsRunIntent(prompt):
		return m.handleAgentRuns(ctx, language, svc)
	case agentIsHTTPRequestIntent(prompt) || agentIsAPIFunctionalTestIntent(prompt):
		return m.handleAgentHTTPRequest(ctx, agentExtractHTTPMethod(prompt), agentExtractURL(prompt), nil, "", language, nil)
	case agentIsWorkspaceSearchIntent(prompt):
		query := agentExtractWorkspaceSearchQuery(prompt)
		return m.handleAgentWorkspaceSearch(sessionID, query, agentExtractWorkspaceSearchPath(prompt, query), false, false, 80, language)
	case agentIsWorkspaceTreeIntent(prompt):
		return m.handleAgentWorkspaceTree(sessionID, agentExtractPath(prompt), 3, 120, false, language)
	case agentIsWorkspaceIntent(prompt):
		return m.handleAgentWorkspace(ctx, sessionID, prompt, language)
	default:
		_ = operator
		return []agentMessage{m.message(language, "agent", agentExplain(language), nil, nil)}, nil, nil
	}
}

func agentExecutableAutoContinuePrompt(next *agentNextAction) string {
	if next == nil || next.Type != "continue" || !next.AutoContinue {
		return ""
	}
	prompt := strings.TrimSpace(next.SuggestedPrompt)
	if prompt == "" {
		return ""
	}
	if agentControlActionFromPrompt(prompt) == "continue" {
		remainder := strings.TrimSpace(agentControlPromptFromPrompt(prompt, "continue"))
		if remainder == "" {
			return ""
		}
		prompt = remainder
	}
	return prompt
}

func agentCanAutoRouteContinuePrompt(prompt string) bool {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return false
	}
	if agentIsControlIntent(prompt) || agentIsScanIntent(prompt) || agentIsPortScanIntent(prompt) || agentIsResumeApprovedIntent(prompt) || agentIsApprovalDecisionIntent(prompt) {
		return false
	}
	if agentIsCommandIntent(prompt) {
		commands := agentExtractCommands(prompt)
		if len(commands) == 0 {
			if cmd := agentExtractCommand(prompt); cmd != "" {
				commands = []string{cmd}
			}
		}
		if len(commands) == 0 {
			return false
		}
		for _, cmd := range commands {
			if !agentPlannerCommandAllowed(cmd) {
				return false
			}
		}
		return true
	}
	if agentIsWorkspaceIntent(prompt) {
		return (agentIsWorkspaceListIntent(prompt) || agentIsWorkspaceTreeIntent(prompt) || agentIsWorkspaceSearchIntent(prompt) || agentIsWorkspaceReadIntent(prompt) || agentIsWorkspacePrepareOnlyIntent(prompt)) &&
			!agentIsWorkspaceWriteIntent(prompt) &&
			!agentIsWorkspacePatchIntent(prompt) &&
			!agentIsWorkspaceDiffIntent(prompt) &&
			!agentIsWorkspaceApplyIntent(prompt) &&
			!agentIsWorkspaceApplyAllIntent(prompt) &&
			!agentIsWorkspaceDiscardIntent(prompt) &&
			!agentIsWorkspaceDiscardAllIntent(prompt) &&
			!agentIsWorkspaceRebaseIntent(prompt)
	}
	return agentIsRunContextAnalysisIntent(prompt) ||
		agentIsHealthIntent(prompt) ||
		agentIsPluginIntent(prompt) ||
		agentIsRunIntent(prompt) ||
		agentIsApprovalIntent(prompt)
}

func agentIsWorkspacePrepareOnlyIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(workspace|工作空间)`).MatchString(prompt) &&
		!agentIsWorkspaceListIntent(prompt) &&
		!agentIsWorkspaceTreeIntent(prompt) &&
		!agentIsWorkspaceSearchIntent(prompt) &&
		!agentIsWorkspaceReadIntent(prompt)
}

func tagAgentAutoContinueMessages(messages []agentMessage, calls []agentToolCall, prompt string) ([]agentMessage, []agentToolCall) {
	metadata := map[string]interface{}{
		"phase":                "auto_continue",
		"auto_continue":        true,
		"auto_continue_prompt": trimSummary(prompt, 220),
	}
	for i := range calls {
		calls[i] = withAgentToolMetadata(calls[i], metadata)
	}
	for i := range messages {
		for j := range messages[i].ToolCalls {
			messages[i].ToolCalls[j] = withAgentToolMetadata(messages[i].ToolCalls[j], metadata)
		}
	}
	return messages, calls
}

func (m *agentManager) handleAgentRunContextAnalysis(ctx context.Context, prompt, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	runID := agentExtractRunID(prompt)
	if runID == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我已识别为上下文分析，但缺少 run id。请从运行详情页的 finding / artifact 按钮交给 Agent。", "I recognized a context-analysis request, but no run id was found. Send it from a finding / artifact button on the run detail page."), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("GET /api/v1/runs/{run_id}/context", shortAgentID(runID, 22))
	detail, err := svc.runs.GetRun(ctx, runID)
	contextKind := agentRunContextKind(prompt)
	if err == nil {
		base := map[string]any{
			"run_id":    runID,
			"context":   contextKind,
			"target":    detail.Summary.Target,
			"status":    string(detail.Summary.Status),
			"events":    detail.Summary.Counts.Events,
			"artifacts": detail.Summary.Counts.Artifacts,
			"findings":  detail.Summary.Counts.Findings,
			"approvals": detail.Summary.Counts.Approvals,
		}
		switch contextKind {
		case "artifact_index":
			var index any
			index, err = svc.runs.GetArtifactIndex(ctx, runID)
			if err == nil {
				base["artifact_count"] = agentArtifactIndexCount(index)
				base["sample_artifacts"] = agentArtifactIndexSamples(index, 5)
			}
		case "events":
			var page services.EventPage
			page, err = svc.runs.GetEvents(ctx, runID, "", 12)
			if err == nil {
				base["event_count"] = len(page.Items)
				base["eof"] = page.EOF
				base["next_cursor"] = page.NextCursor
				base["sample_events"] = agentEventPageSamples(page, 6)
			}
		case "approvals":
			var approvals []contracts.HITLRequest
			approvals, err = svc.approvals.ListApprovals(ctx, runID)
			if err == nil {
				base["approval_count"] = len(approvals)
				base["pending"] = agentPendingApprovalCount(approvals)
				base["sample_approvals"] = agentApprovalSamples(approvals, 5)
			}
		case "findings":
			var report services.RunReport
			report, err = svc.runs.GetReport(ctx, runID, "json")
			if err == nil {
				findings, evidence, artifacts := agentReportJSONCounts(report.Bytes)
				base["finding_count"] = findings
				base["evidence_count"] = evidence
				base["artifact_count"] = artifacts
				base["sample_findings"] = agentReportJSONFindingSamples(report.Bytes, 5)
			} else if services.ErrorCodeOf(err) == services.CodeNotFound {
				err = nil
				base["report_available"] = false
				base["report_status"] = "pending"
				base["finding_count"] = 0
				base["evidence_count"] = 0
				base["artifact_count"] = detail.Summary.Counts.Artifacts
				if page, pageErr := svc.runs.GetEvents(ctx, runID, "", 8); pageErr == nil {
					base["event_count"] = len(page.Items)
					base["sample_events"] = agentEventPageSamples(page, 5)
				}
			}
		default:
			contextKind = "report_json"
			base["context"] = contextKind
			var report services.RunReport
			report, err = svc.runs.GetReport(ctx, runID, "json")
			if err == nil {
				findings, evidence, artifacts := agentReportJSONCounts(report.Bytes)
				base["format"] = report.Format
				base["content_type"] = report.ContentType
				base["bytes"] = len(report.Bytes)
				base["finding_count"] = findings
				base["evidence_count"] = evidence
				base["artifact_count"] = artifacts
				base["sample_findings"] = agentReportJSONFindingSamples(report.Bytes, 5)
			} else if services.ErrorCodeOf(err) == services.CodeNotFound {
				err = nil
				base["report_available"] = false
				base["report_status"] = "pending"
				base["finding_count"] = detail.Summary.Counts.Findings
				base["evidence_count"] = 0
				base["artifact_count"] = detail.Summary.Counts.Artifacts
				if page, pageErr := svc.runs.GetEvents(ctx, runID, "", 8); pageErr == nil {
					base["event_count"] = len(page.Items)
					base["sample_events"] = agentEventPageSamples(page, 5)
				}
			}
		}
		tool.Result = base
	}
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	content := agentBuildRunContextObservation(language, detail, contextKind, tool.Result)
	message := m.message(language, "tool", content, []agentToolCall{tool}, []agentLink{{To: "/scan/" + runID, Label: agentText(language, "打开运行详情", "Open run detail"), Kind: "run"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentRunContextTool(ctx context.Context, sessionID, runID, contextKind, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		if session, err := m.GetSession(ctx, sessionID); err == nil && session.ActiveTask != nil {
			runID = strings.TrimSpace(session.ActiveTask.ActiveRunID)
		}
	}
	if runID == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我需要 run id 或当前活动运行，才能读取运行上下文。", "I need a run ID or an active run to read run context."), nil, nil)}, nil, nil
	}
	contextKind = agentNormalizeRunContextKind(contextKind)
	return m.handleAgentRunContextAnalysis(ctx, fmt.Sprintf("读取运行 %s 的 %s 上下文", runID, contextKind), language, svc)
}

func (m *agentManager) handleAgentReportDraft(ctx context.Context, sessionID, runID, language string, svc routerServices, outputPath string) ([]agentMessage, []agentToolCall, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		if session, err := m.GetSession(ctx, sessionID); err == nil && session.ActiveTask != nil {
			runID = strings.TrimSpace(session.ActiveTask.ActiveRunID)
		}
	}
	if runID == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我需要 run id 或当前活动运行，才能生成报告草稿。", "I need a run ID or an active run to generate a report draft."), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("reports.draft", shortAgentID(runID, 22))
	detail, err := svc.runs.GetRun(ctx, runID)
	var (
		ws       *workspace.Workspace
		snapshot agentRunReportSnapshot
		content  string
	)
	if err == nil {
		snapshot = m.activeRunReportSnapshot(ctx, runID, detail)
		content = agentBuildReportDraftMarkdown(language, detail, snapshot, time.Now().UTC())
		outputPath = agentReportDraftPath(runID, outputPath)
		ws, err = m.workspace.Create(sessionID)
	}
	if err == nil {
		err = m.workspace.WriteFile(sessionID, outputPath, []byte(content))
	}
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"run_id":          runID,
		"path":            outputPath,
		"bytes":           len(content),
		"workspace_path":  ws.Path,
		"target":          detail.Summary.Target,
		"status":          string(detail.Summary.Status),
		"finding_count":   len(snapshot.Findings),
		"evidence_count":  len(snapshot.Evidence),
		"artifact_count":  len(snapshot.Artifacts),
		"context":         "report_draft",
		"workspace_scope": "agent_workspace",
		"draft_format":    "markdown",
		"review_required": true,
	}
	message := m.message(language, "tool", agentText(
		language,
		fmt.Sprintf("已生成报告草稿：%s（run=%s，findings=%d）。请在工作区预览并人工复核后再对外输出。", outputPath, shortAgentID(runID, 22), len(snapshot.Findings)),
		fmt.Sprintf("Generated report draft: %s (run=%s, findings=%d). Preview it in the workspace and review before external use.", outputPath, shortAgentID(runID, 22), len(snapshot.Findings)),
	), []agentToolCall{tool}, []agentLink{{To: "/scan/" + runID, Label: agentText(language, "打开运行详情", "Open run detail"), Kind: "run"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentScan(ctx context.Context, prompt, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	target := agentExtractURL(prompt)
	if target == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我需要一个 http/https 授权目标 URL 才能开始扫描。", "I need an authorized http/https target URL to start a scan."), nil, nil)}, nil, nil
	}
	scope := agentExtractScope(prompt, target)
	allowExternalTools := agentWantsExternalTools(prompt)
	plan := m.message(language, "agent", agentText(
		language,
		fmt.Sprintf("已生成执行计划：校验 scope=%s → 使用 safe profile → %s → 收集证据与报告。", emptyDash(scope), agentExternalToolPhrase(language, allowExternalTools)),
		fmt.Sprintf("Plan generated: validate scope=%s → use safe profile → %s → collect evidence and report.", emptyDash(scope), agentExternalToolPhrase(language, allowExternalTools)),
	), nil, nil)

	tool := runningAgentToolCall("POST /api/v1/scans/web", agentHostLabel(target))
	response, err := svc.jobs.StartWebScan(ctx, startWebScanJobRequest{
		Target:          target,
		Scope:           scope,
		Profile:         "safe",
		NoExternalTools: !allowExternalTools,
		EnabledPlugins:  nil,
	})
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{plan, m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"job_id":            response.JobID,
		"run_id":            response.RunID,
		"run_url":           response.RunURL,
		"status_url":        response.StatusURL,
		"target":            target,
		"scope":             scope,
		"profile":           "safe",
		"no_external_tools": !allowExternalTools,
		"next_step":         "track_run_context",
	}
	done := m.message(language, "tool", agentText(
		language,
		fmt.Sprintf("已把任务交给 daemon：job=%s，run=%s。运行详情会持续同步事件、证据、审批和报告。", shortAgentID(response.JobID, 22), shortAgentID(response.RunID, 22)),
		fmt.Sprintf("Task handed to daemon: job=%s, run=%s. The run detail will keep syncing events, evidence, approvals and reports.", shortAgentID(response.JobID, 22), shortAgentID(response.RunID, 22)),
	), []agentToolCall{tool}, []agentLink{{To: "/scan/" + response.RunID, Label: agentText(language, "打开运行", "Open run"), Kind: "run"}})
	return []agentMessage{plan, done}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentPortScan(ctx context.Context, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	target := agentExtractURL(prompt)
	host := agentExtractPortScanHostFromPrompt(prompt)
	if host == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我需要一个授权目标主机，才能执行端口探测。", "I need an authorized target host to run a port probe."), nil, nil)}, nil, nil
	}
	if target == "" {
		target = host
	}
	ports, portLabel, err := agentExtractSafePortScanPorts(prompt)
	if err != nil {
		tool := failedAgentToolCall("ports.scan.safe", err)
		return []agentMessage{m.message(language, "agent", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	plan := m.message(language, "agent", agentText(
		language,
		fmt.Sprintf("已生成端口探测计划：目标=%s → 端口集=%s → TCP connect 安全探测 → 汇总开放端口与限制。", host, portLabel),
		fmt.Sprintf("Port probe plan generated: target=%s → port set=%s → safe TCP connect probe → summarize open ports and limitations.", host, portLabel),
	), nil, nil)

	timeout := 750 * time.Millisecond
	tool := runningAgentToolCall("ports.scan.safe", host)
	results := agentScanTCPPorts(ctx, host, ports, timeout)
	openPorts := make([]int, 0)
	closedCount := 0
	errorSamples := make([]string, 0, 3)
	for _, result := range results {
		if result.Open {
			openPorts = append(openPorts, result.Port)
			continue
		}
		closedCount++
		if result.Error != "" && len(errorSamples) < 3 {
			errorSamples = append(errorSamples, fmt.Sprintf("%d/tcp: %s", result.Port, result.Error))
		}
	}
	sort.Ints(openPorts)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"target":        target,
		"host":          host,
		"profile":       "safe_tcp_connect",
		"port_profile":  portLabel,
		"scanned_ports": ports,
		"scanned_count": len(ports),
		"open_ports":    openPorts,
		"open_count":    len(openPorts),
		"closed_count":  closedCount,
		"timeout_ms":    timeout.Milliseconds(),
		"error_samples": errorSamples,
		"limitations":   "TCP connect probe only; filtered UDP services and deep service fingerprints are out of scope for this built-in.",
	}
	openText := agentText(language, "未发现开放端口", "No open ports detected")
	if len(openPorts) > 0 {
		parts := make([]string, 0, len(openPorts))
		for _, port := range openPorts {
			parts = append(parts, fmt.Sprintf("%d/tcp", port))
		}
		openText = strings.Join(parts, ", ")
	}
	message := m.message(language, "tool", agentText(
		language,
		fmt.Sprintf("端口探测已完成。\n\n- target: `%s`\n- profile: `safe_tcp_connect`\n- scanned: `%d` ports (%s)\n- open: %s\n\n限制：这是 TCP connect 安全探测，不等同于 nmap 全量服务识别；UDP、被过滤端口和深度指纹需要专用插件。", host, len(ports), portLabel, openText),
		fmt.Sprintf("Port probe completed.\n\n- target: `%s`\n- profile: `safe_tcp_connect`\n- scanned: `%d` ports (%s)\n- open: %s\n\nLimitations: this is a safe TCP connect probe, not full nmap service fingerprinting; UDP, filtered ports, and deep fingerprints need a dedicated plugin.", host, len(ports), portLabel, openText),
	), []agentToolCall{tool}, nil)
	return []agentMessage{plan, message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentHealth(ctx context.Context, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("GET /api/v1/doctor", "daemon")
	version := svc.version.GetVersion(ctx)
	doctor := svc.version.Doctor(ctx)
	summary := summarizeDoctorChecks(doctor.Checks)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"version":                version.Version,
		"ok":                     doctor.Healthy,
		"checks":                 len(doctor.Checks),
		"required_issues":        summary.RequiredIssues,
		"optional_missing":       summary.OptionalMissing,
		"warnings":               summary.Warnings,
		"required_issue_names":   summary.RequiredIssueNames,
		"optional_missing_names": summary.OptionalMissingNames,
		"warning_names":          summary.WarningNames,
	}
	message := m.message(language, "tool", agentText(
		language,
		agentDoctorSummaryText("zh", version.Version, len(doctor.Checks), summary),
		agentDoctorSummaryText("en", version.Version, len(doctor.Checks), summary),
	), []agentToolCall{tool}, []agentLink{{To: "/settings", Label: agentText(language, "连接设置", "Connection"), Kind: "settings"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentPlugins(ctx context.Context, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("GET /api/v1/plugins", "registry")
	plugins, err := svc.plugins.ListPlugins(ctx)
	if err == nil {
		var doctor services.PluginDoctorReport
		doctor, err = svc.plugins.Doctor(ctx)
		tool.Result = map[string]any{"plugins": len(plugins), "ok": doctor.OK, "issues": countUnhealthyPlugins(doctor)}
	}
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	sample := make([]string, 0, 5)
	for _, plugin := range plugins {
		if len(sample) >= 5 {
			break
		}
		sample = append(sample, plugin.ID)
	}
	message := m.message(language, "tool", agentText(
		language,
		fmt.Sprintf("发现 %d 个插件/工具。Agent 会优先使用内置轻量能力，再按策略申请高风险插件。%s", len(plugins), agentSamplePhrase(language, sample)),
		fmt.Sprintf("Discovered %d plugins/tools. The Agent prefers built-in lightweight tools, then requests policy approval for higher-risk plugins. %s", len(plugins), agentSamplePhrase(language, sample)),
	), []agentToolCall{tool}, []agentLink{{To: "/ops", Label: agentText(language, "工具面板", "Tool panel"), Kind: "tools"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentRuns(ctx context.Context, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("GET /api/v1/runs", "runs-dir")
	page, err := svc.runs.ListRuns(ctx, services.RunListRequest{Limit: 10})
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{"runs": len(page.Items), "eof": page.EOF}
	lines := make([]string, 0, 4)
	for i, run := range page.Items {
		if i >= 4 {
			break
		}
		lines = append(lines, fmt.Sprintf("%s · %s · %s", run.Status, agentHostLabel(run.Target), shortAgentID(run.RunID, 18)))
	}
	content := agentText(language, "还没有运行记录。你可以直接让我扫描一个授权目标。", "No runs yet. You can ask me to scan an authorized target.")
	if len(lines) > 0 {
		content = agentText(language, fmt.Sprintf("最近 %d 个运行：\n%s", len(page.Items), strings.Join(lines, "\n")), fmt.Sprintf("Recent %d runs:\n%s", len(page.Items), strings.Join(lines, "\n")))
	}
	message := m.message(language, "tool", content, []agentToolCall{tool}, []agentLink{{To: "/history", Label: agentText(language, "运行历史", "Run history"), Kind: "runs"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentApprovals(ctx context.Context, language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("GET /api/v1/runs/*/approvals", "policy")
	page, err := svc.runs.ListRuns(ctx, services.RunListRequest{Limit: 12})
	pending := 0
	firstRun := ""
	checkedRuns := 0
	if err == nil {
		for _, run := range page.Items {
			checkedRuns++
			approvals, listErr := svc.approvals.ListApprovals(ctx, run.RunID)
			if listErr != nil {
				continue
			}
			for _, approval := range approvals {
				if approval.Status == contracts.HITLPending {
					pending++
					if firstRun == "" {
						firstRun = run.RunID
					}
				}
			}
		}
	}
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"pending":      pending,
		"checked_runs": checkedRuns,
		"context":      "approval_queue",
	}
	if firstRun != "" {
		tool.Result["run_id"] = firstRun
	}
	content := agentText(language, "没有待审批动作。高风险或外部工具步骤会在这里等待人工决策。", "No pending approvals. Higher-risk or external-tool steps will wait here for human decisions.")
	links := []agentLink{{To: "/history", Label: agentText(language, "运行历史", "Runs"), Kind: "runs"}}
	if pending > 0 {
		content = agentText(language, fmt.Sprintf("发现 %d 个待审批动作。请进入运行详情查看风险摘要并 approve/deny。", pending), fmt.Sprintf("Found %d pending approval actions. Open the run detail to review risk summary and approve/deny.", pending))
		links = []agentLink{{To: "/scan/" + firstRun, Label: agentText(language, "处理审批", "Review"), Kind: "approval"}}
	}
	message := m.message(language, "tool", content, []agentToolCall{tool}, links)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentResumeApproved(ctx context.Context, sessionID, prompt, language, operator string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	runID := agentExtractRunID(prompt)
	if runID == "" {
		if session, err := m.GetSession(ctx, sessionID); err == nil && session.ActiveTask != nil {
			runID = strings.TrimSpace(session.ActiveTask.ActiveRunID)
		}
	}
	if runID == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我需要 run_id 或当前 active run 才能恢复已批准步骤。", "I need a run_id or current active run to resume approved steps."), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("POST /api/v1/runs/{run_id}/resume-approved", shortAgentID(runID, 22))
	result, err := svc.approvals.ResumeApproved(ctx, services.ResumeApprovedRequest{
		RunIDOrPath: runID,
		Operator:    operator,
	})
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{
			"run_id":  runID,
			"context": "resume_approved",
		}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"run_id":        result.RunID,
		"run_dir":       result.RunDir,
		"resumed":       result.Resumed,
		"skipped":       result.Skipped,
		"resumed_count": len(result.Resumed),
		"skipped_count": len(result.Skipped),
		"context":       "resume_approved",
	}

	content := agentText(language,
		fmt.Sprintf("已执行恢复已批准步骤：run=%s，resumed=%d，skipped=%d。Agent 将继续跟踪运行事件、证据和报告。", result.RunID, len(result.Resumed), len(result.Skipped)),
		fmt.Sprintf("Resume-approved completed: run=%s, resumed=%d, skipped=%d. The Agent will continue tracking events, evidence and report.", result.RunID, len(result.Resumed), len(result.Skipped)),
	)
	message := m.message(language, "tool", content, []agentToolCall{tool}, []agentLink{{To: "/scan/" + result.RunID, Label: agentText(language, "打开运行", "Open run"), Kind: "run"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentApprovalDecision(ctx context.Context, sessionID, prompt, language, operator string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	action := agentApprovalDecisionAction(prompt)
	if action == "" {
		return []agentMessage{m.message(language, "agent", agentText(language, "我识别到审批相关请求，但没有明确是批准还是拒绝。请说明“批准 approval_xxx”或“拒绝 approval_xxx”。", "I detected an approval request, but it is not clear whether to approve or deny. Say `approve approval_xxx` or `deny approval_xxx`."), nil, nil)}, nil, nil
	}
	target, targetErr := m.resolveAgentApprovalDecisionTarget(ctx, sessionID, prompt, svc)
	if targetErr != nil {
		return []agentMessage{m.message(language, "agent", targetErr.Error(), nil, nil)}, nil, nil
	}

	comment := agentExtractApprovalComment(prompt)
	toolName := fmt.Sprintf("POST /api/v1/runs/{run_id}/approvals/{approval_id}/%s", action)
	tool := runningAgentToolCall(toolName, shortAgentID(target.ApprovalID, 22))
	req := services.ApprovalDecisionRequest{
		RunIDOrPath:        target.RunID,
		ApprovalIDOrStepID: target.ApprovalID,
		Operator:           operator,
		Comment:            comment,
	}
	var (
		resolved contracts.HITLRequest
		err      error
	)
	switch action {
	case "approve":
		resolved, err = svc.approvals.Approve(ctx, req)
	case "deny":
		resolved, err = svc.approvals.Deny(ctx, req)
	}
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{
			"run_id":      target.RunID,
			"approval_id": target.ApprovalID,
			"action":      action,
			"context":     "approval_decision",
		}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	pending := 0
	if approvals, listErr := svc.approvals.ListApprovals(ctx, target.RunID); listErr == nil {
		pending = agentPendingApprovalCount(approvals)
	}
	tool.Result = map[string]any{
		"run_id":           target.RunID,
		"approval_id":      resolved.ID,
		"step_id":          resolved.StepID,
		"requested_action": resolved.RequestedAction,
		"status":           string(resolved.Status),
		"action":           action,
		"pending":          pending,
		"context":          "approval_decision",
	}
	if comment != "" {
		tool.Result["comment"] = comment
	}

	decisionText := agentText(language, "已批准", "Approved")
	if action == "deny" {
		decisionText = agentText(language, "已拒绝", "Denied")
	}
	content := agentText(language,
		fmt.Sprintf("%s审批 %s（step=%s，run=%s）。剩余待审批：%d。", decisionText, resolved.ID, emptyDash(resolved.StepID), target.RunID, pending),
		fmt.Sprintf("%s approval %s (step=%s, run=%s). Pending approvals left: %d.", decisionText, resolved.ID, emptyDash(resolved.StepID), target.RunID, pending),
	)
	if pending == 0 {
		content += agentText(language, " Agent 已刷新审批 gate，可继续跟踪运行或恢复已批准步骤。", " The Agent refreshed the approval gate; continue tracking the run or resume approved steps.")
	}
	message := m.message(language, "tool", content, []agentToolCall{tool}, []agentLink{{To: "/scan/" + target.RunID, Label: agentText(language, "打开运行", "Open run"), Kind: "run"}})
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

type agentApprovalDecisionTarget struct {
	RunID      string
	ApprovalID string
}

func (m *agentManager) resolveAgentApprovalDecisionTarget(ctx context.Context, sessionID, prompt string, svc routerServices) (agentApprovalDecisionTarget, error) {
	runID := agentExtractRunID(prompt)
	approvalID := agentExtractApprovalID(prompt)
	if runID == "" {
		if session, err := m.GetSession(ctx, sessionID); err == nil && session.ActiveTask != nil {
			runID = strings.TrimSpace(session.ActiveTask.ActiveRunID)
		}
	}
	if runID != "" {
		approvals, err := svc.approvals.ListApprovals(ctx, runID)
		if err != nil {
			return agentApprovalDecisionTarget{}, err
		}
		if approvalID == "" {
			pending := agentPendingApprovals(approvals)
			if len(pending) == 1 {
				approvalID = pending[0].ID
			} else if len(pending) > 1 {
				return agentApprovalDecisionTarget{}, services.NewError(services.CodeBadRequest, "multiple pending approvals; specify approval_id or step_id", map[string]any{"run_id": runID, "pending": agentApprovalIDs(pending)}, nil)
			}
		}
		if approvalID == "" {
			return agentApprovalDecisionTarget{}, services.NewError(services.CodeBadRequest, "approval id or step id is required", map[string]any{"run_id": runID}, nil)
		}
		return agentApprovalDecisionTarget{RunID: runID, ApprovalID: approvalID}, nil
	}

	page, err := svc.runs.ListRuns(ctx, services.RunListRequest{Limit: 30})
	if err != nil {
		return agentApprovalDecisionTarget{}, err
	}
	var matches []agentApprovalDecisionTarget
	for _, run := range page.Items {
		approvals, listErr := svc.approvals.ListApprovals(ctx, run.RunID)
		if listErr != nil {
			continue
		}
		if approvalID != "" {
			if _, ok := agentFindApproval(approvals, approvalID); ok {
				matches = append(matches, agentApprovalDecisionTarget{RunID: run.RunID, ApprovalID: approvalID})
			}
			continue
		}
		for _, approval := range approvals {
			if approval.Status == contracts.HITLPending {
				matches = append(matches, agentApprovalDecisionTarget{RunID: run.RunID, ApprovalID: approval.ID})
			}
		}
	}
	switch len(matches) {
	case 0:
		return agentApprovalDecisionTarget{}, services.NewError(services.CodeBadRequest, "no matching pending approval found; include run_id and approval_id", nil, nil)
	case 1:
		return matches[0], nil
	default:
		return agentApprovalDecisionTarget{}, services.NewError(services.CodeBadRequest, "multiple matching approvals; include run_id and approval_id", nil, nil)
	}
}

func (m *agentManager) refreshApprovalGateForSessionLocked(ctx context.Context, session *agentSession) bool {
	if session == nil || session.ActiveTask == nil || strings.TrimSpace(session.ActiveTask.ActiveRunID) == "" {
		return false
	}
	runID := strings.TrimSpace(session.ActiveTask.ActiveRunID)
	approvals, err := m.approvals.ListApprovals(ctx, runID)
	if err != nil {
		return false
	}
	pending := 0
	for _, approval := range approvals {
		if approval.Status == contracts.HITLPending {
			pending++
		}
	}
	if pending == 0 {
		now := time.Now().UTC()
		todosChanged := resolveAgentApprovalGateTodos(session, now)
		if session.NextAction != nil && session.NextAction.Type == "approval_required" || todosChanged {
			session.NextAction = &agentNextAction{
				Type:            "continue",
				Reason:          agentText(session.Language, "审批已处理，可继续跟踪运行。", "Approvals have been handled; continue tracking the run."),
				SuggestedPrompt: agentSuggestedContinuePrompt(session.Language, session),
				AutoContinue:    true,
			}
			if session.Status == "idle" || session.Status == "blocked" {
				session.Status = "running"
			}
			if session.ActiveTask.Status == "idle" || session.ActiveTask.Status == "blocked" || session.ActiveTask.Status == "" {
				session.ActiveTask.Status = "running"
			}
			session.UpdatedAt = now
			session.ActiveTask.UpdatedAt = session.UpdatedAt
			return true
		}
		return false
	}
	now := time.Now().UTC()
	for idx := range session.Todos {
		if agentTodoBlocksOnApprovalGate(session.Todos[idx]) {
			markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", agentText(session.Language, "等待人工审批", "Waiting for human approval"))
		}
	}
	session.Status = "running"
	session.ActiveTask.Status = "running"
	session.ActiveTask.UpdatedAt = now
	session.NextAction = &agentNextAction{
		Type:            "approval_required",
		Reason:          agentText(session.Language, fmt.Sprintf("运行 %s 有 %d 个待审批动作。", runID, pending), fmt.Sprintf("Run %s has %d pending approval action(s).", runID, pending)),
		SuggestedPrompt: agentText(session.Language, "先处理审批队列，然后恢复已批准运行", "Review the approval queue, then resume the approved run"),
		AutoContinue:    false,
	}
	session.UpdatedAt = now
	return true
}

func resolveAgentApprovalGateTodos(session *agentSession, now time.Time) bool {
	if session == nil {
		return false
	}
	changed := false
	running := false
	for idx := range session.Todos {
		todo := session.Todos[idx]
		if todo.Status == "done" {
			continue
		}
		switch {
		case agentTodoWasBlockedByApprovalGate(todo) || agentTodoIsExplicitApprovalGate(todo):
			nextStatus := "done"
			if agentApprovalTodoShouldResume(todo) && todo.Status == "blocked" {
				nextStatus = "running"
			}
			if todo.Status != nextStatus || todo.BlockedReason != "" {
				markAgentTodoByIndex(session.Todos, idx, nextStatus, now, "", "")
				changed = true
			}
		}
		if session.Todos[idx].Status == "running" {
			running = true
		}
	}
	if running {
		return changed
	}
	for idx := range session.Todos {
		if session.Todos[idx].Status == "pending" {
			markAgentTodoByIndex(session.Todos, idx, "running", now, "", "")
			return true
		}
	}
	return changed
}

func agentTodoIsExplicitApprovalGate(todo agentTodo) bool {
	title := strings.ToLower(strings.TrimSpace(todo.Title))
	if title == "" {
		return false
	}
	hasApproval := strings.Contains(title, "审批") ||
		strings.Contains(title, "approval") ||
		strings.Contains(title, "approvals")
	if !hasApproval {
		return false
	}
	return strings.Contains(title, "待审批") ||
		strings.Contains(title, "pending approval") ||
		strings.Contains(title, "approval gate") ||
		strings.Contains(title, "审批 gate") ||
		strings.Contains(title, "审批队列") ||
		strings.Contains(title, "approval queue") ||
		strings.Contains(title, "处理") ||
		strings.Contains(title, "resolve")
}

func agentTodoBlocksOnApprovalGate(todo agentTodo) bool {
	title := strings.ToLower(strings.TrimSpace(todo.Title))
	if title == "" {
		return false
	}
	hasApproval := strings.Contains(title, "审批") ||
		strings.Contains(title, "approval") ||
		strings.Contains(title, "approvals")
	if !hasApproval {
		return false
	}
	return strings.Contains(title, "待审批") ||
		strings.Contains(title, "approval gate") ||
		strings.Contains(title, "pending approval") ||
		strings.Contains(title, "审批 gate") ||
		strings.Contains(title, "审批队列") ||
		strings.Contains(title, "approval queue") ||
		strings.Contains(title, "跟踪") ||
		strings.Contains(title, "track") ||
		strings.Contains(title, "monitor") ||
		strings.Contains(title, "处理") ||
		strings.Contains(title, "resolve") ||
		strings.Contains(title, "检查") ||
		strings.Contains(title, "check") ||
		strings.Contains(title, "确认") ||
		strings.Contains(title, "confirm")
}

func agentApprovalTodoShouldResume(todo agentTodo) bool {
	title := strings.ToLower(strings.TrimSpace(todo.Title))
	return (strings.Contains(title, "跟踪") ||
		strings.Contains(title, "track") ||
		strings.Contains(title, "monitor")) &&
		!strings.Contains(title, "待审批") &&
		!strings.Contains(title, "pending approval") &&
		!strings.Contains(title, "approval gate") &&
		!strings.Contains(title, "审批 gate")
}

func agentTodoWasBlockedByApprovalGate(todo agentTodo) bool {
	if todo.Status != "blocked" {
		return false
	}
	reason := strings.ToLower(strings.TrimSpace(todo.BlockedReason))
	return strings.Contains(reason, "审批") ||
		strings.Contains(reason, "approval") ||
		strings.Contains(reason, "human")
}

func (m *agentManager) appendMessages(sessionID string, messages ...agentMessage) {
	m.appendMessagesWithPlanner(sessionID, nil, messages...)
}

func (m *agentManager) appendMessagesWithPlanner(sessionID string, plan *agentPlannerResponse, messages ...agentMessage) {
	eng := m.runtimeEngine()
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return
	}
	session.Messages = append(session.Messages, messages...)
	m.applyAgentMessagesLocked(session, messages...)
	if plan != nil {
		m.applyAgentPlannerLocked(session, plan, session.Language)
	}
	if len(messages) > 0 {
		session.UpdatedAt = messages[len(messages)-1].Timestamp
	}
	compaction := compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	_ = m.persistSessionLocked(session)
	sessionState := cloneAgentSession(session)
	m.mu.Unlock()
	m.emitRuntimeMemoryCompacted(context.Background(), sessionID, sessionState, agentRuntimeTurnRef{}, compaction)
	m.emitAgentRuntimeEvents(context.Background(), eng, sessionID, sessionState, messages...)
}

func (m *agentManager) message(language, role, content string, toolCalls []agentToolCall, links []agentLink) agentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.newMessageLocked(language, role, content, toolCalls, links)
}

func (m *agentManager) newMessageLocked(_ string, role, content string, toolCalls []agentToolCall, links []agentLink) agentMessage {
	now := time.Now().UTC()
	return agentMessage{
		ID:        strings.Replace(runs.NewJobID(now), "job_", "msg_", 1),
		Role:      role,
		Content:   content,
		Timestamp: now,
		ToolCalls: cloneAgentToolCalls(toolCalls),
		Links:     cloneAgentLinks(links),
	}
}

// applyAgentPlannerToMessage promotes planner-declared suggestions and
// recommendation options onto the agentMessage so the frontend can render them
// from authoritative fields instead of re-parsing the message content.
func applyAgentPlannerToMessage(message *agentMessage, plan *agentPlannerResponse) {
	if message == nil || plan == nil {
		return
	}
	prompts := collectAgentPlannerSuggestedPrompts(plan)
	if len(prompts) > 0 {
		message.SuggestedPrompts = prompts
	}
	options := dedupeAgentRecommendationOptions(plan.RecommendationOptions)
	if len(options) > 0 {
		message.RecommendationOptions = options
	}
}

func collectAgentPlannerSuggestedPrompts(plan *agentPlannerResponse) []string {
	if plan == nil {
		return nil
	}
	seen := make(map[string]struct{}, 4)
	out := make([]string, 0, 4)
	add := func(value string) {
		v := strings.TrimSpace(value)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	add(plan.SuggestedPrompt)
	add(plan.SuggestPrompt)
	for _, prompt := range plan.SuggestedPrompts {
		add(prompt)
	}
	if plan.NextAction != nil {
		add(plan.NextAction.SuggestedPrompt)
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func dedupeAgentRecommendationOptions(options []agentRecommendationOpt) []agentRecommendationOpt {
	if len(options) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(options))
	out := make([]agentRecommendationOpt, 0, len(options))
	for i, opt := range options {
		title := strings.TrimSpace(opt.Title)
		detail := strings.TrimSpace(opt.Detail)
		prompt := strings.TrimSpace(opt.Prompt)
		if title == "" && detail == "" && prompt == "" {
			continue
		}
		key := strings.ToLower(detail)
		if key == "" {
			key = strings.ToLower(title)
		}
		if key == "" {
			key = strings.ToLower(prompt)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		id := strings.TrimSpace(opt.ID)
		if id == "" {
			id = fmt.Sprintf("recommendation_%d", i+1)
		}
		out = append(out, agentRecommendationOpt{
			ID:      id,
			Title:   title,
			Detail:  detail,
			Prompt:  prompt,
			Command: strings.TrimSpace(opt.Command),
		})
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

func agentNormalizePhase(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "plan", "planning":
		return "plan"
	case "step", "execute", "executing", "tool", "tool_execution":
		return "step"
	case "step_replan", "stepreplan", "replan", "replanning", "step-replan":
		return "step_replan"
	case "final_answer", "finalanswer", "final", "answer", "done", "complete":
		return "final_answer"
	default:
		return ""
	}
}

func agentMessageWithPhase(message agentMessage, phase string) agentMessage {
	message.Phase = agentNormalizePhase(phase)
	for i := range message.ToolCalls {
		message.ToolCalls[i] = agentToolCallWithPhase(message.ToolCalls[i], message.Phase)
	}
	return message
}

func agentMessagesWithPhase(messages []agentMessage, phase string) []agentMessage {
	normalized := agentNormalizePhase(phase)
	if normalized == "" {
		return messages
	}
	out := append([]agentMessage(nil), messages...)
	for i := range out {
		out[i] = agentMessageWithPhase(out[i], normalized)
	}
	return out
}

func agentToolCallWithPhase(call agentToolCall, phase string) agentToolCall {
	normalized := agentNormalizePhase(phase)
	if normalized == "" {
		return call
	}
	if call.Metadata == nil {
		call.Metadata = map[string]interface{}{}
	}
	if _, exists := call.Metadata["phase"]; exists {
		call.Metadata["agent_phase"] = normalized
	} else {
		call.Metadata["phase"] = normalized
	}
	return call
}

func agentToolCallsWithPhase(calls []agentToolCall, phase string) []agentToolCall {
	normalized := agentNormalizePhase(phase)
	if normalized == "" {
		return calls
	}
	out := append([]agentToolCall(nil), calls...)
	for i := range out {
		out[i] = agentToolCallWithPhase(out[i], normalized)
	}
	return out
}

func normalizeAgentSessionID(raw string) (string, error) {
	sessionID := strings.TrimSpace(raw)
	if sessionID == "" {
		return "", services.NewError(services.CodeBadRequest, "session id is required", map[string]any{"field": "session_id"}, nil)
	}
	if strings.Contains(sessionID, "\x00") || strings.ContainsAny(sessionID, `/\`) || strings.Contains(sessionID, "..") {
		return "", services.NewError(services.CodeBadRequest, "invalid session id", map[string]any{"field": "session_id"}, nil)
	}
	return sessionID, nil
}

func normalizeAgentOperator(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "local-user"
	}
	return raw
}

func normalizeAgentLanguage(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "en", "en-us":
		return "en"
	default:
		return "zh"
	}
}

func normalizeAgentOperationLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "read_only", "readonly", "read-only", "ro", "只读":
		return "read_only"
	case "workspace_write", "workspace-write", "workspace", "write", "工作区写入":
		return "workspace_write"
	case "safe_scan", "safe-scan", "scan", "扫描", "安全扫描":
		return "safe_scan"
	case "approval_ops", "approval-ops", "approval", "approvals", "approve", "审批":
		return "approval_ops"
	case "bypass", "bypass_permissions", "bypass-permissions", "danger_full_access", "danger-full-access", "绕过权限", "跳过权限":
		return "bypass"
	case "", "advanced", "full", "高权限", "高级":
		return "advanced"
	default:
		return "advanced"
	}
}

func agentOperationLevelLabel(level, language string) string {
	switch normalizeAgentOperationLevel(level) {
	case "read_only":
		return agentText(language, "只读", "Read-only")
	case "workspace_write":
		return agentText(language, "工作区写入", "Workspace write")
	case "safe_scan":
		return agentText(language, "安全扫描", "Safe scan")
	case "approval_ops":
		return agentText(language, "审批操作", "Approval ops")
	case "advanced":
		return agentText(language, "高级", "Advanced")
	case "bypass":
		return agentText(language, "Bypass 权限", "Bypass permissions")
	default:
		return agentText(language, "只读", "Read-only")
	}
}

func agentOperationLevelDescription(level, language string) string {
	switch normalizeAgentOperationLevel(level) {
	case "read_only":
		return agentText(language,
			"只能执行 daemon 健康检查、插件/运行/审批读取、工作区列文件/文件树/搜索/读文件和白名单只读命令。",
			"May only run daemon health checks, plugin/run/approval reads, workspace file listing/tree/search/reading, and allowlisted read-only commands.",
		)
	case "workspace_write":
		return agentText(language,
			"包含只读能力，并允许写入、生成 diff、patch、应用、丢弃、删除、重命名、复制文件或创建当前 Agent 工作区目录。",
			"Includes read-only capabilities and permits writing, staging diffs, patching, applying, discarding, deleting, renaming/copying files, or creating directories in the Agent workspace.",
		)
	case "safe_scan":
		return agentText(language,
			"包含工作区能力，并允许对明确授权目标发起低风险 HTTP 请求或启动 safe profile Web/API 扫描；外部工具默认仍由扫描策略控制。",
			"Includes workspace capabilities and permits low-risk HTTP requests or safe-profile Web/API scans for explicitly authorized targets; external tools remain controlled by scan policy.",
		)
	case "approval_ops":
		return agentText(language,
			"包含安全扫描能力，并允许批准/拒绝审批与恢复已批准运行。",
			"Includes safe-scan capabilities and permits approving/denying approvals and resuming approved runs.",
		)
	case "advanced":
		return agentText(language,
			"最高内置权限：允许当前 Agent 已实现的工具动作；仍受 scope、白名单命令和显式用户意图约束。",
			"Highest built-in level: permits implemented Agent tool actions while still enforcing scope, command allowlists, and explicit operator intent.",
		)
	default:
		return agentOperationLevelDescription("read_only", language)
	}
}

func agentOperationLevelRank(level string) int {
	switch normalizeAgentOperationLevel(level) {
	case "read_only":
		return 0
	case "workspace_write":
		return 1
	case "safe_scan":
		return 2
	case "approval_ops":
		return 3
	case "advanced":
		return 4
	case "bypass":
		return 5
	default:
		return 0
	}
}

func agentOperationBypassPermissions(level string) bool {
	return normalizeAgentOperationLevel(level) == "bypass"
}

func agentOperationLevelAtLeast(level, required string) bool {
	return agentOperationLevelRank(level) >= agentOperationLevelRank(required)
}

func agentOperationLevelRequiredForAction(actionName string) string {
	// Delegate to the canonical tool registry. Unknown actions (plugin / MCP
	// names) fall through to read_only to preserve legacy behaviour during the
	// migration window — the registry-side default is "advanced", but flipping
	// that here would gate previously-allowed plugin invocations.
	if spec, ok := toolRegistry().Lookup(actionName); ok {
		return spec.RequiredLevel
	}
	return "read_only"
}

func agentOperationLevelRequiredForPlannerAction(actionName string, args map[string]any) string {
	normalized := normalizeAgentPlannerActionName(actionName)
	if normalized == "http.request" {
		method := firstNonEmpty(stringFromAny(args["method"]), "GET")
		if agentHTTPMethodIsSafe(method) {
			return "safe_scan"
		}
		return "advanced"
	}
	return agentOperationLevelRequiredForAction(normalized)
}

func agentPlannerActionRequiresManualAuthorization(actionName string) bool {
	switch normalizeAgentPlannerActionName(actionName) {
	case "scan.web.safe",
		"ports.scan.safe",
		"http.request",
		"workspace.files.write",
		"workspace.files.patch",
		"workspace.files.diff_many",
		"workspace.files.rebase",
		"workspace.files.apply",
		"workspace.files.discard",
		"workspace.files.delete",
		"workspace.files.move",
		"workspace.files.copy",
		"workspace.files.mkdir",
		"reports.draft",
		"runs.approvals.approve",
		"runs.approvals.deny",
		"runs.resume_approved":
		return true
	default:
		return false
	}
}

func agentOperationLevelRequiredForPrompt(prompt string) string {
	switch {
	case agentIsApprovalDecisionIntent(prompt) || agentIsResumeApprovedIntent(prompt):
		return "approval_ops"
	case agentIsScheduleCreateIntent(prompt) || agentIsScheduleDeleteIntent(prompt):
		return "advanced"
	case agentIsReportDraftIntent(prompt):
		return "workspace_write"
	case (agentIsHTTPRequestIntent(prompt) || agentIsAPIFunctionalTestIntent(prompt)) && !agentHTTPMethodIsSafe(agentExtractHTTPMethod(prompt)):
		return "advanced"
	case agentIsScanIntent(prompt) || agentIsPortScanIntent(prompt) || agentIsHTTPRequestIntent(prompt) || agentIsAPIFunctionalTestIntent(prompt):
		return "safe_scan"
	case agentIsWorkspaceWriteIntent(prompt) ||
		agentIsWorkspacePatchIntent(prompt) ||
		agentIsWorkspaceDiffIntent(prompt) ||
		agentIsWorkspaceRebaseIntent(prompt) ||
		agentIsWorkspaceApplyIntent(prompt) ||
		agentIsWorkspaceApplyAllIntent(prompt) ||
		agentIsWorkspaceDiscardIntent(prompt) ||
		agentIsWorkspaceDiscardAllIntent(prompt) ||
		agentIsWorkspaceDeleteIntent(prompt) ||
		agentIsWorkspaceMoveIntent(prompt) ||
		agentIsWorkspaceCopyIntent(prompt) ||
		agentIsWorkspaceMkdirIntent(prompt):
		return "workspace_write"
	default:
		return "read_only"
	}
}

func agentOperationLevelDeniedError(language, level, required, action string) error {
	level = normalizeAgentOperationLevel(level)
	required = normalizeAgentOperationLevel(required)
	return services.NewError(
		services.CodePolicyDenied,
		agentText(
			language,
			fmt.Sprintf("当前 Agent 操作权限为「%s」，不足以执行「%s」。请切换到「%s」后重试。", agentOperationLevelLabel(level, language), action, agentOperationLevelLabel(required, language)),
			fmt.Sprintf("Current Agent operation level is %s, which is not enough to execute %s. Switch to %s and retry.", agentOperationLevelLabel(level, language), action, agentOperationLevelLabel(required, language)),
		),
		map[string]any{
			"operation_level":          level,
			"required_operation_level": required,
			"action":                   action,
		},
		nil,
	)
}

func normalizeAgentControlAction(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "continue", "resume", "next", "next_step", "next-step", "proceed", "继续", "下一步", "继续执行":
		return "continue"
	case "revise", "revision", "adjust", "update_plan", "update-plan", "修订", "修改计划", "调整计划", "更新计划":
		return "revise"
	case "pause", "暂停", "暂停任务":
		return "pause"
	case "cancel", "取消", "取消任务":
		return "cancel"
	case "stop", "停止", "停止任务":
		return "stop"
	case "new_task", "new-task", "new", "reset", "新任务", "重置", "清空任务":
		return "new_task"
	default:
		return ""
	}
}

func normalizeAgentAuthorizationDecision(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "approve", "allow", "yes", "grant", "授权", "允许", "同意", "确认":
		return "approve"
	case "deny", "reject", "no", "block", "拒绝", "不允许", "阻止":
		return "deny"
	default:
		return ""
	}
}

func agentControlRequestFromPrompt(prompt, operator, language string) (agentControlRequest, bool) {
	action := agentControlActionFromPrompt(prompt)
	if action == "" {
		return agentControlRequest{}, false
	}
	return agentControlRequest{
		Action:     action,
		Operator:   operator,
		Language:   language,
		Prompt:     agentControlPromptFromPrompt(prompt, action),
		Reason:     agentControlReasonFromPrompt(prompt, action, language),
		RunID:      agentExtractRunID(prompt),
		JobID:      agentExtractJobID(prompt),
		ClearInput: agentControlClearInputFromPrompt(prompt),
	}, true
}

func agentIsControlIntent(prompt string) bool {
	return agentControlActionFromPrompt(prompt) != ""
}

func agentControlActionFromPrompt(prompt string) string {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	if normalized == "" {
		return ""
	}
	switch normalizeAgentControlAction(normalized) {
	case "continue", "revise", "pause", "stop", "cancel", "new_task":
		return normalizeAgentControlAction(normalized)
	}
	if regexp.MustCompile(`(?i)^\s*/?(continue|resume|next|next[-_ ]step|proceed)\b`).MatchString(prompt) ||
		regexp.MustCompile(`^\s*/?(?:请)?(?:继续(?:监控|跟踪|当前|任务|执行|推进)|继续(?:\s|[:：-]|$))`).MatchString(prompt) ||
		regexp.MustCompile(`^\s*/?(?:请)?(继续|下一步|继续执行|继续监控|继续跟踪|接着来|继续推进)(?:\s|[:：-]|$)`).MatchString(prompt) {
		return "continue"
	}
	if regexp.MustCompile(`(?i)^\s*/?(revise|revision|adjust|update[-_ ]?plan)\b`).MatchString(prompt) ||
		regexp.MustCompile(`^\s*/?(修订计划|修订当前计划|修订|修改计划|调整计划|更新计划)(?:\s|[:：-]|$)`).MatchString(prompt) {
		return "revise"
	}
	if regexp.MustCompile(`(?i)^\s*/?pause\b`).MatchString(prompt) ||
		regexp.MustCompile(`^\s*/?(暂停任务|暂停)(?:\s|[:：-]|$)`).MatchString(prompt) {
		return "pause"
	}
	if regexp.MustCompile(`(?i)^\s*/?cancel\b`).MatchString(prompt) ||
		regexp.MustCompile(`^\s*/?(取消任务|取消)(?:\s|[:：-]|$)`).MatchString(prompt) {
		return "cancel"
	}
	if regexp.MustCompile(`(?i)^\s*/?stop\b`).MatchString(prompt) ||
		regexp.MustCompile(`^\s*/?(停止任务|停止当前任务|停止)(?:\s|[:：-]|$)`).MatchString(prompt) {
		return "stop"
	}
	if regexp.MustCompile(`(?i)^\s*/?(new[-_ ]?task|new|reset|clear[-_ ]?task)\b`).MatchString(prompt) ||
		regexp.MustCompile(`^\s*/?(新任务|开启新任务|重置任务|清空任务|重新开始)(?:\s|[:：-]|$)`).MatchString(prompt) {
		return "new_task"
	}
	return ""
}

func agentControlPromptFromPrompt(prompt, action string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)^\s*/?(continue|resume|next|next[-_ ]step|proceed)\s*[:：-]?\s*(.*)$`),
		regexp.MustCompile(`^\s*/?(?:请)?继续(?:监控|跟踪|当前|任务|执行|推进)?\s*[:：-]?\s*(.*)$`),
		regexp.MustCompile(`^\s*/?(?:请)?(继续执行|继续推进|继续监控|继续跟踪|继续|下一步|接着来)\s*[:：-]?\s*(.*)$`),
		regexp.MustCompile(`(?i)^\s*/?(revise|revision|adjust|update[-_ ]?plan)\s*[:：-]?\s*(.*)$`),
		regexp.MustCompile(`^\s*/?(修订计划|修订当前计划|修订|修改计划|调整计划|更新计划)\s*[:：-]?\s*(.*)$`),
		regexp.MustCompile(`(?i)^\s*/?(stop|pause|cancel)\s*[:：-]?\s*(.*)$`),
		regexp.MustCompile(`^\s*/?(停止任务|停止当前任务|停止|暂停任务|暂停|取消任务|取消)\s*[:：-]?\s*(.*)$`),
		regexp.MustCompile(`(?i)^\s*/?(new[-_ ]?task|new|reset|clear[-_ ]?task)\s*[:：-]?\s*(.*)$`),
		regexp.MustCompile(`^\s*/?(新任务|开启新任务|重置任务|清空任务|重新开始)\s*[:：-]?\s*(.*)$`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 2 {
			return strings.TrimSpace(match[2])
		}
	}
	if strings.TrimSpace(prompt) == action {
		return ""
	}
	return prompt
}

func agentControlReasonFromPrompt(prompt, action, language string) string {
	remainder := agentControlPromptFromPrompt(prompt, action)
	if remainder != "" && (action == "stop" || action == "pause" || action == "cancel") {
		return trimSummary(remainder, 260)
	}
	switch action {
	case "continue":
		return agentText(language, "用户通过聊天请求继续当前任务。", "The operator requested continuing the current task from chat.")
	case "revise":
		return agentText(language, "用户通过聊天请求修订当前任务计划。", "The operator requested revising the current task plan from chat.")
	case "pause":
		return agentText(language, "用户通过聊天请求暂停当前会话。", "The operator requested pausing the current session from chat.")
	case "stop":
		return agentText(language, "用户通过聊天请求停止当前任务。", "The operator requested stopping the current task from chat.")
	case "cancel":
		return agentText(language, "用户通过聊天请求取消当前任务。", "The operator requested cancelling the current task from chat.")
	case "new_task":
		return agentText(language, "用户通过聊天请求开启新的任务上下文。", "The operator requested a new task context from chat.")
	default:
		return ""
	}
}

func agentControlClearInputFromPrompt(prompt string) bool {
	return regexp.MustCompile(`(?i)(clear|reset|empty|清空|重置|重新开始)`).MatchString(prompt)
}

func agentWelcome(language string) string {
	return agentText(language,
		"我是 CyberAgent。给我授权目标与 scope，我会规划步骤、选择工具、执行 safe profile、收集证据并生成可复现报告。",
		"I am CyberAgent. Give me an authorized target and scope; I will plan steps, select tools, run the safe profile, collect evidence and produce a reproducible report.",
	)
}

func agentExplain(language string) string {
	return agentText(language,
		"我不是普通扫描后台：你用自然语言给目标、范围、策略和意图；我负责理解任务、生成计划、选择工具、执行 daemon job、跟踪事件、沉淀证据、触发人工审批、汇总 finding 和报告。",
		"I am not a regular scan dashboard: describe target, scope, policy and intent in natural language; I understand the task, plan steps, select tools, run daemon jobs, track events, collect evidence, trigger human approvals, and summarize findings/reports.",
	)
}

func agentText(language, zh, en string) string {
	if normalizeAgentLanguage(language) == "en" {
		return en
	}
	return zh
}

func agentArtifactIndexCount(index any) int {
	var decoded struct {
		Count     int                  `json:"count"`
		Artifacts []contracts.Artifact `json:"artifacts"`
	}
	if err := decodeAgentAny(index, &decoded); err != nil {
		return 0
	}
	if decoded.Count > 0 {
		return decoded.Count
	}
	return len(decoded.Artifacts)
}

func agentArtifactIndexSamples(index any, limit int) []map[string]any {
	var decoded struct {
		Artifacts []contracts.Artifact `json:"artifacts"`
	}
	if err := decodeAgentAny(index, &decoded); err != nil {
		return nil
	}
	if limit <= 0 || limit > len(decoded.Artifacts) {
		limit = len(decoded.Artifacts)
	}
	samples := make([]map[string]any, 0, limit)
	for _, artifact := range decoded.Artifacts[:limit] {
		samples = append(samples, map[string]any{
			"id":        artifact.ID,
			"step_id":   artifact.StepID,
			"type":      artifact.Type,
			"path":      artifact.Path,
			"mime_type": artifact.MIMEType,
			"size":      artifact.SizeBytes,
		})
	}
	return samples
}

func agentEventPageSamples(page services.EventPage, limit int) []map[string]any {
	if limit <= 0 || limit > len(page.Items) {
		limit = len(page.Items)
	}
	samples := make([]map[string]any, 0, limit)
	for _, item := range page.Items[:limit] {
		samples = append(samples, map[string]any{
			"sequence": item.Sequence,
			"cursor":   item.Cursor,
			"id":       item.Event.ID,
			"type":     item.Event.Type,
			"level":    item.Event.Level,
			"step_id":  item.Event.StepID,
			"message":  trimSummary(item.Event.Message, 180),
		})
	}
	return samples
}

func agentPendingApprovalCount(approvals []contracts.HITLRequest) int {
	pending := 0
	for _, approval := range approvals {
		if approval.Status == contracts.HITLPending {
			pending++
		}
	}
	return pending
}

func agentPendingApprovals(approvals []contracts.HITLRequest) []contracts.HITLRequest {
	pending := make([]contracts.HITLRequest, 0, len(approvals))
	for _, approval := range approvals {
		if approval.Status == contracts.HITLPending {
			pending = append(pending, approval)
		}
	}
	return pending
}

func agentApprovalIDs(approvals []contracts.HITLRequest) []string {
	ids := make([]string, 0, len(approvals))
	for _, approval := range approvals {
		if strings.TrimSpace(approval.ID) != "" {
			ids = append(ids, approval.ID)
			continue
		}
		if strings.TrimSpace(approval.StepID) != "" {
			ids = append(ids, approval.StepID)
		}
	}
	return ids
}

func agentFindApproval(approvals []contracts.HITLRequest, idOrStepID string) (contracts.HITLRequest, bool) {
	idOrStepID = strings.TrimSpace(idOrStepID)
	if idOrStepID == "" {
		return contracts.HITLRequest{}, false
	}
	for _, approval := range approvals {
		if approval.ID == idOrStepID || approval.StepID == idOrStepID {
			return approval, true
		}
	}
	return contracts.HITLRequest{}, false
}

func agentApprovalSamples(approvals []contracts.HITLRequest, limit int) []map[string]any {
	if limit <= 0 || limit > len(approvals) {
		limit = len(approvals)
	}
	samples := make([]map[string]any, 0, limit)
	for _, approval := range approvals[:limit] {
		samples = append(samples, map[string]any{
			"id":               approval.ID,
			"step_id":          approval.StepID,
			"status":           approval.Status,
			"requested_action": approval.RequestedAction,
			"reason":           trimSummary(approval.Reason, 180),
		})
	}
	return samples
}

func agentReportJSONCounts(data []byte) (int, int, int) {
	var decoded struct {
		Findings  []contracts.Finding  `json:"findings"`
		Evidence  []contracts.Evidence `json:"evidence"`
		Artifacts []contracts.Artifact `json:"artifacts"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return 0, 0, 0
	}
	return len(decoded.Findings), len(decoded.Evidence), len(decoded.Artifacts)
}

func agentReportJSONFindingSamples(data []byte, limit int) []map[string]any {
	var decoded struct {
		Findings []contracts.Finding `json:"findings"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil
	}
	if limit <= 0 || limit > len(decoded.Findings) {
		limit = len(decoded.Findings)
	}
	samples := make([]map[string]any, 0, limit)
	for _, finding := range decoded.Findings[:limit] {
		samples = append(samples, map[string]any{
			"id":           finding.ID,
			"title":        finding.Title,
			"severity":     finding.Severity,
			"confidence":   finding.Confidence,
			"category":     finding.Category,
			"evidence_ids": finding.EvidenceIDs,
			"step_id":      finding.Source.StepID,
		})
	}
	return samples
}

func agentBuildRunContextObservation(language string, detail services.RunDetail, contextKind string, result map[string]any) string {
	runID := shortAgentID(detail.RunID, 22)
	target := emptyDash(detail.Summary.Target)
	switch contextKind {
	case "artifact_index":
		return agentText(language,
			fmt.Sprintf("已读取运行 %s 的 artifact index：artifacts=%d，target=%s。可基于 sample_artifacts 继续定位原始证据、响应头、JS 路由或工具输出。", runID, intFromAny(result["artifact_count"]), target),
			fmt.Sprintf("Read artifact index for run %s: artifacts=%d, target=%s. Use sample_artifacts to locate raw evidence, response headers, JS routes, or tool output.", runID, intFromAny(result["artifact_count"]), target),
		)
	case "events":
		return agentText(language,
			fmt.Sprintf("已读取运行 %s 的事件流片段：events=%d，sample_events=%d。可按 step_id 串联执行链、错误、审批和发现。", runID, intFromAny(result["events"]), intFromAny(result["event_count"])),
			fmt.Sprintf("Read event stream slice for run %s: events=%d, sample_events=%d. Correlate by step_id to reconstruct execution chain, errors, approvals, and findings.", runID, intFromAny(result["events"]), intFromAny(result["event_count"])),
		)
	case "approvals":
		return agentText(language,
			fmt.Sprintf("已读取运行 %s 的审批队列：approvals=%d，pending=%d。若 pending > 0，需要先处理审批 gate 再继续执行。", runID, intFromAny(result["approval_count"]), intFromAny(result["pending"])),
			fmt.Sprintf("Read approval queue for run %s: approvals=%d, pending=%d. If pending > 0, resolve the approval gate before continuing execution.", runID, intFromAny(result["approval_count"]), intFromAny(result["pending"])),
		)
	case "findings":
		if available, ok := result["report_available"].(bool); ok && !available {
			return agentText(language,
				fmt.Sprintf("运行 %s 的 report.json 尚未生成，当前仅能读取运行状态和事件片段：status=%s，events=%d，artifacts=%d。任务仍在执行或报告步骤尚未完成；可继续等待事件流或打开运行详情查看实时进度。", runID, detail.Summary.Status, intFromAny(result["event_count"]), intFromAny(result["artifact_count"])),
				fmt.Sprintf("report.json for run %s is not generated yet; only run status and event samples are available: status=%s, events=%d, artifacts=%d. The task may still be running or the report step has not completed; continue watching events or open the run detail for live progress.", runID, detail.Summary.Status, intFromAny(result["event_count"]), intFromAny(result["artifact_count"])),
			)
		}
		return agentText(language,
			fmt.Sprintf("已读取运行 %s 的 findings：findings=%d，evidence=%d，artifacts=%d。下一步可按 sample_findings 的 evidence_ids 追溯证据，并按“影响 → 证据 → 误报检查 → 修复 → 复测”生成行动计划。", runID, intFromAny(result["finding_count"]), intFromAny(result["evidence_count"]), intFromAny(result["artifact_count"])),
			fmt.Sprintf("Read findings for run %s: findings=%d, evidence=%d, artifacts=%d. Next, trace evidence_ids from sample_findings and produce an impact → evidence → false-positive check → remediation → retest action plan.", runID, intFromAny(result["finding_count"]), intFromAny(result["evidence_count"]), intFromAny(result["artifact_count"])),
		)
	default:
		if available, ok := result["report_available"].(bool); ok && !available {
			return agentText(language,
				fmt.Sprintf("运行 %s 的 report.json 尚未生成，当前状态=%s，events=%d，artifacts=%d，target=%s。任务完成前不会再把这类情况显示成错误；可继续监控事件流，报告生成后再读取总结。", runID, detail.Summary.Status, intFromAny(result["event_count"]), intFromAny(result["artifact_count"]), target),
				fmt.Sprintf("report.json for run %s is not generated yet; current status=%s, events=%d, artifacts=%d, target=%s. This is treated as pending instead of an error; keep monitoring events and read the summary after report generation.", runID, detail.Summary.Status, intFromAny(result["event_count"]), intFromAny(result["artifact_count"]), target),
			)
		}
		return agentText(language,
			fmt.Sprintf("已读取运行 %s 的 report.json：findings=%d，evidence=%d，artifacts=%d，target=%s。可继续要求我解释 finding、证据链或生成复测计划。", runID, intFromAny(result["finding_count"]), intFromAny(result["evidence_count"]), intFromAny(result["artifact_count"]), target),
			fmt.Sprintf("Read report.json for run %s: findings=%d, evidence=%d, artifacts=%d, target=%s. You can ask me to explain findings, evidence chains, or produce a retest plan.", runID, intFromAny(result["finding_count"]), intFromAny(result["evidence_count"]), intFromAny(result["artifact_count"]), target),
		)
	}
}

func decodeAgentAny(value any, out any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, out)
}

func agentBuildContextAnalysis(language, prompt string, detail services.RunDetail) string {
	lower := strings.ToLower(prompt)
	target := detail.Summary.Target
	if target == "" {
		target = agentHostLabel(agentExtractURL(prompt))
	}
	title := agentExtractQuoted(prompt)
	if title == "" {
		title = agentText(language, "当前条目", "the selected item")
	}
	if strings.Contains(lower, "artifact") || strings.Contains(prompt, "证据 artifact") {
		return agentText(
			language,
			fmt.Sprintf("我已读取运行 %s 的证据索引。这个 artifact（%s）主要用于支撑扫描链路中的可复现证据：确认目标 %s 的响应、资源、头信息或发现依据。建议下一步：1）打开运行详情核对原始 artifact；2）关联报告中的 finding/evidence id；3）如果它支撑安全头缺失类发现，优先用一次手工 `curl -I` 复核；4）修复后重新让 Agent 对同一 scope 执行 safe profile 验证。", shortAgentID(detail.RunID, 22), title, emptyDash(target)),
			fmt.Sprintf("I read the artifact index for run %s. This artifact (%s) primarily supports reproducible evidence for the scan chain: target %s response, resources, headers, or finding rationale. Recommended next steps: 1) open the run detail and inspect the raw artifact; 2) correlate it with report finding/evidence ids; 3) if it supports missing-security-header findings, manually verify with `curl -I`; 4) after remediation, ask the Agent to rerun the safe profile for the same scope.", shortAgentID(detail.RunID, 22), title, emptyDash(target)),
		)
	}
	return agentText(
		language,
		fmt.Sprintf("我已读取运行 %s 的报告上下文。这个 finding（%s）应按“影响 → 证据 → 误报 → 修复 → 复测”处理：影响通常取决于目标 %s 的业务场景；关键证据以报告中的 evidence id、响应头和复现命令为准；误报检查先确认是否由反向代理/CDN/开发服务器导致；修复后用最小命令复核，再让 Agent 对同一 scope 重跑 safe profile。", shortAgentID(detail.RunID, 22), title, emptyDash(target)),
		fmt.Sprintf("I read the report context for run %s. Treat this finding (%s) as impact → evidence → false-positive check → remediation → retest: impact depends on target %s context; key evidence should come from report evidence ids, response headers, and reproduction commands; false-positive checks should consider reverse proxy/CDN/dev-server behavior; after remediation, verify with a minimal command and ask the Agent to rerun the safe profile for the same scope.", shortAgentID(detail.RunID, 22), title, emptyDash(target)),
	)
}

func agentExtractQuoted(prompt string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`title="([^"]+)"`),
		regexp.MustCompile(`artifact[：:]\s*[“"]([^”"]+)`),
		regexp.MustCompile(`"([^"]+)"`),
		regexp.MustCompile(`“([^”]+)”`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return match[1]
		}
	}
	return ""
}

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

type agentDoctorCheckSummary struct {
	RequiredIssues       int
	OptionalMissing      int
	Warnings             int
	RequiredIssueNames   []string
	OptionalMissingNames []string
	WarningNames         []string
}

func summarizeDoctorChecks(checks []doctorruntime.CheckResult) agentDoctorCheckSummary {
	summary := agentDoctorCheckSummary{}
	for _, check := range checks {
		if check.Status == doctorruntime.StatusOK {
			continue
		}
		name := strings.TrimSpace(check.Name)
		if name == "" {
			name = string(check.Status)
		}
		if check.Required {
			summary.RequiredIssues++
			summary.RequiredIssueNames = append(summary.RequiredIssueNames, name)
			continue
		}
		if check.Status == doctorruntime.StatusMissing {
			summary.OptionalMissing++
			summary.OptionalMissingNames = append(summary.OptionalMissingNames, name)
			continue
		}
		summary.Warnings++
		summary.WarningNames = append(summary.WarningNames, name)
	}
	return summary
}

func agentDoctorSummaryText(language, version string, total int, summary agentDoctorCheckSummary) string {
	if normalizeAgentLanguage(language) == "en" {
		required := "Required checks passed"
		if summary.RequiredIssues > 0 {
			required = fmt.Sprintf("Required checks have %d issue(s)%s", summary.RequiredIssues, agentNameListSuffix("en", summary.RequiredIssueNames))
		}
		parts := []string{}
		if summary.OptionalMissing > 0 {
			parts = append(parts, fmt.Sprintf("%d optional tool(s) missing%s; this does not block built-in safe capabilities", summary.OptionalMissing, agentNameListSuffix("en", summary.OptionalMissingNames)))
		}
		if summary.Warnings > 0 {
			parts = append(parts, fmt.Sprintf("%d warning(s)%s", summary.Warnings, agentNameListSuffix("en", summary.WarningNames)))
		}
		if len(parts) == 0 {
			return fmt.Sprintf("Daemon online, version %s. %s, %d checks total.", version, required, total)
		}
		return fmt.Sprintf("Daemon online, version %s. %s, %d checks total; %s.", version, required, total, strings.Join(parts, "; "))
	}

	required := "必需检查通过"
	if summary.RequiredIssues > 0 {
		required = fmt.Sprintf("必需检查异常 %d 项%s", summary.RequiredIssues, agentNameListSuffix("zh", summary.RequiredIssueNames))
	}
	parts := []string{}
	if summary.OptionalMissing > 0 {
		parts = append(parts, fmt.Sprintf("可选工具缺失 %d 个%s（不影响内置 safe 能力）", summary.OptionalMissing, agentNameListSuffix("zh", summary.OptionalMissingNames)))
	}
	if summary.Warnings > 0 {
		parts = append(parts, fmt.Sprintf("告警 %d 个%s", summary.Warnings, agentNameListSuffix("zh", summary.WarningNames)))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("Daemon 在线，版本 %s。%s，检查项 %d。", version, required, total)
	}
	return fmt.Sprintf("Daemon 在线，版本 %s。%s，检查项 %d；%s。", version, required, total, strings.Join(parts, "；"))
}

func agentNameListSuffix(language string, names []string) string {
	if len(names) == 0 {
		return ""
	}
	limit := len(names)
	if limit > 4 {
		limit = 4
	}
	suffix := strings.Join(names[:limit], ", ")
	if len(names) > limit {
		if normalizeAgentLanguage(language) == "en" {
			suffix += fmt.Sprintf(", +%d more", len(names)-limit)
		} else {
			suffix += fmt.Sprintf(" 等 %d 项", len(names))
		}
	}
	if normalizeAgentLanguage(language) == "en" {
		return " (" + suffix + ")"
	}
	return "（" + suffix + "）"
}

func countUnhealthyPlugins(report services.PluginDoctorReport) int {
	count := 0
	for _, plugin := range report.Plugins {
		if !plugin.OK {
			count++
		}
	}
	return count
}

func agentSamplePhrase(language string, values []string) string {
	if len(values) == 0 {
		return ""
	}
	return agentText(language, "示例："+strings.Join(values, ", "), "Examples: "+strings.Join(values, ", "))
}

func agentIsWorkspaceIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(workspace|工作空间|文件|file|目录|directory|folder)`).MatchString(prompt) ||
		agentIsWorkspaceTreeIntent(prompt) ||
		agentIsWorkspaceSearchIntent(prompt) ||
		agentIsWorkspaceWriteIntent(prompt) ||
		agentIsWorkspaceDiffIntent(prompt) ||
		agentIsWorkspaceRebaseIntent(prompt) ||
		agentIsWorkspaceApplyAllIntent(prompt) ||
		agentIsWorkspaceApplyIntent(prompt) ||
		agentIsWorkspaceDiscardAllIntent(prompt) ||
		agentIsWorkspaceDiscardIntent(prompt) ||
		agentIsWorkspaceDeleteIntent(prompt) ||
		agentIsWorkspaceMoveIntent(prompt) ||
		agentIsWorkspaceCopyIntent(prompt) ||
		agentIsWorkspaceMkdirIntent(prompt)
}

func agentIsWorkspaceTreeIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(tree|file\s+tree|directory\s+tree|文件树|目录树|结构树|递归列出)`).MatchString(prompt)
}

func agentIsWorkspaceSearchIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(search|grep|find\s+text|find\s+string|查找|搜索|检索|全文搜索)`).MatchString(prompt) &&
		regexp.MustCompile(`(?i)(workspace|工作空间|file|files|文件|目录|path|路径|内容|text|string|pattern|query)`).MatchString(prompt)
}

func agentIsWorkspaceListIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(list|ls|tree|show files|列出|查看|文件列表|目录列表)`).MatchString(prompt)
}

func agentIsWorkspaceReadIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(read|cat|open|show file|读取|查看文件|打开文件)`).MatchString(prompt)
}

func agentIsWorkspaceWriteIntent(prompt string) bool {
	verb := regexp.MustCompile(`(?i)(write|create|save|update|replace|写入|创建|保存|修改|更新|覆盖)`).MatchString(prompt)
	if !verb {
		return false
	}
	return regexp.MustCompile(`(?i)(workspace|工作空间|file|path\s*[:=]|file\s*[:=]|content\s*[:=]|body\s*[:=]|text\s*[:=]|input\s*[:=]|文件|路径|内容|正文|[A-Za-z0-9._/\-]+\.[A-Za-z0-9][A-Za-z0-9._\-]*)`).MatchString(prompt)
}

func agentFormatCommandResult(language string, result *command.Result) string {
	if result == nil {
		return agentText(language, "命令没有返回结果。", "The command returned no result.")
	}
	stdout := strings.TrimRight(result.Stdout, "\n")
	stderr := strings.TrimRight(result.Stderr, "\n")
	if stdout == "" {
		stdout = agentText(language, "（无标准输出）", "(no stdout)")
	}
	if stderr == "" {
		stderr = agentText(language, "（无标准错误）", "(no stderr)")
	}
	return agentText(language,
		fmt.Sprintf("命令已执行。\n\n- command: `%s`\n- exit_code: %d\n- workdir: `%s`\n- duration: %dms\n\nstdout:\n```text\n%s\n```\n\nstderr:\n```text\n%s\n```", result.Command, result.ExitCode, result.WorkDir, result.DurationMs, agentLimitBlock(stdout, 6000), agentLimitBlock(stderr, 3000)),
		fmt.Sprintf("Command executed.\n\n- command: `%s`\n- exit_code: %d\n- workdir: `%s`\n- duration: %dms\n\nstdout:\n```text\n%s\n```\n\nstderr:\n```text\n%s\n```", result.Command, result.ExitCode, result.WorkDir, result.DurationMs, agentLimitBlock(stdout, 6000), agentLimitBlock(stderr, 3000)),
	)
}

func agentFormatCommandBatchResult(language string, calls []agentToolCall) string {
	if len(calls) == 0 {
		return agentText(language, "没有执行任何命令。", "No commands were executed.")
	}
	lines := make([]string, 0, len(calls))
	for _, call := range calls {
		commandText := stringFromAny(call.Result["command"])
		if commandText == "" {
			commandText = call.Detail
		}
		exitCode := intFromAny(call.Result["exit_code"])
		stdout := strings.TrimSpace(stringFromAny(call.Result["stdout"]))
		stderr := strings.TrimSpace(stringFromAny(call.Result["stderr"]))
		if call.Error != nil && strings.TrimSpace(call.Error.Message) != "" {
			stderr = call.Error.Message
		}
		summary := stdout
		if summary == "" {
			summary = stderr
		}
		if summary == "" {
			summary = agentText(language, "无输出", "no output")
		}
		lines = append(lines, fmt.Sprintf(
			"- `%s` → %s exit=%d\n  ```text\n  %s\n  ```",
			commandText,
			call.Status,
			exitCode,
			strings.ReplaceAll(agentLimitBlock(summary, 1200), "\n", "\n  "),
		))
	}
	return agentText(language,
		fmt.Sprintf("已按顺序执行 %d 个低风险工作区命令：\n\n%s", len(calls), strings.Join(lines, "\n")),
		fmt.Sprintf("Executed %d low-risk workspace command(s) in sequence:\n\n%s", len(calls), strings.Join(lines, "\n")),
	)
}

func agentFormatWorkspaceList(language, relPath string, files []workspace.FileInfo) string {
	if len(files) == 0 {
		return agentText(language,
			fmt.Sprintf("工作空间目录 `%s` 为空。", relPath),
			fmt.Sprintf("Workspace directory `%s` is empty.", relPath),
		)
	}
	sorted := append([]workspace.FileInfo(nil), files...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].IsDir != sorted[j].IsDir {
			return sorted[i].IsDir
		}
		return sorted[i].Name < sorted[j].Name
	})
	lines := make([]string, 0, len(sorted))
	for _, file := range sorted {
		kind := "file"
		if file.IsDir {
			kind = "dir "
		}
		lines = append(lines, fmt.Sprintf("- `%s` %s %dB %s", file.Path, kind, file.Size, file.Mode))
	}
	body := strings.Join(lines, "\n")
	return agentText(language,
		fmt.Sprintf("工作空间目录 `%s` 文件列表：\n\n%s", relPath, body),
		fmt.Sprintf("Workspace directory `%s` files:\n\n%s", relPath, body),
	)
}

func agentFormatWorkspaceRead(language, relPath, content string) string {
	return agentText(language,
		fmt.Sprintf("已读取工作空间文件 `%s`：\n\n```text\n%s\n```", relPath, agentLimitBlock(content, 8000)),
		fmt.Sprintf("Read workspace file `%s`:\n\n```text\n%s\n```", relPath, agentLimitBlock(content, 8000)),
	)
}

func agentFormatWorkspaceTree(language, relPath string, entries []agentWorkspaceTreeEntry, truncated bool) string {
	if len(entries) == 0 {
		return agentText(language,
			fmt.Sprintf("工作空间目录 `%s` 没有可展示的文件树条目。", relPath),
			fmt.Sprintf("Workspace directory `%s` has no tree entries to show.", relPath),
		)
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		prefix := strings.Repeat("  ", maxInt(entry.Depth-1, 0))
		icon := "📄"
		if entry.IsDir {
			icon = "📁"
		}
		lines = append(lines, fmt.Sprintf("%s- %s `%s`", prefix, icon, entry.Path))
	}
	if truncated {
		lines = append(lines, agentText(language, "- … 已达到展示上限", "- … display limit reached"))
	}
	return agentText(language,
		fmt.Sprintf("工作空间目录 `%s` 文件树：\n\n%s", relPath, strings.Join(lines, "\n")),
		fmt.Sprintf("Workspace directory `%s` tree:\n\n%s", relPath, strings.Join(lines, "\n")),
	)
}

func agentFormatWorkspaceSearch(language, query string, matches []agentWorkspaceSearchMatch, filesScanned int, truncated bool) string {
	if len(matches) == 0 {
		return agentText(language,
			fmt.Sprintf("工作空间搜索 `%s` 没有命中（扫描文件 %d 个）。", query, filesScanned),
			fmt.Sprintf("Workspace search `%s` found no matches (%d file(s) scanned).", query, filesScanned),
		)
	}
	limit := minInt(len(matches), 40)
	lines := make([]string, 0, limit+1)
	for _, match := range matches[:limit] {
		lines = append(lines, fmt.Sprintf("- `%s:%d` %s", match.Path, match.Line, match.Text))
	}
	if truncated || len(matches) > limit {
		lines = append(lines, agentText(language, "- … 已达到搜索结果上限", "- … search result limit reached"))
	}
	return agentText(language,
		fmt.Sprintf("工作空间搜索 `%s` 命中 %d 条（扫描文件 %d 个）：\n\n%s", query, len(matches), filesScanned, strings.Join(lines, "\n")),
		fmt.Sprintf("Workspace search `%s` found %d match(es) (%d file(s) scanned):\n\n%s", query, len(matches), filesScanned, strings.Join(lines, "\n")),
	)
}

func agentFormatHTTPRequest(language string, result map[string]any) string {
	method := stringFromAny(result["method"])
	targetURL := stringFromAny(result["url"])
	status := stringFromAny(result["status"])
	body := strings.TrimSpace(stringFromAny(result["body"]))
	truncated := boolFromAny(result["body_truncated"])
	if body != "" {
		body = agentLimitBlock(body, 900)
		if truncated {
			body += agentText(language, "\n…（响应体已截断）", "\n…(response body truncated)")
		}
	}
	if body == "" {
		return agentText(language,
			fmt.Sprintf("HTTP 响应：`%s %s`，status: `%s`。", method, targetURL, status),
			fmt.Sprintf("HTTP response: `%s %s`, status: `%s`.", method, targetURL, status),
		)
	}
	return agentText(language,
		fmt.Sprintf("HTTP 响应：`%s %s`，status: `%s`。\n\nbody 预览：\n```text\n%s\n```", method, targetURL, status, body),
		fmt.Sprintf("HTTP response: `%s %s`, status: `%s`.\n\nBody preview:\n```text\n%s\n```", method, targetURL, status, body),
	)
}

func agentFormatWorkspaceWrite(language, relPath, content string, size int) string {
	return agentText(language,
		fmt.Sprintf("已写入工作空间文件 `%s`（%d bytes）。\n\n内容预览：\n```text\n%s\n```", relPath, size, agentLimitBlock(content, 1200)),
		fmt.Sprintf("Wrote workspace file `%s` (%d bytes).\n\nPreview:\n```text\n%s\n```", relPath, size, agentLimitBlock(content, 1200)),
	)
}

func agentFormatWorkspaceDelete(language, relPath string) string {
	return agentText(language,
		fmt.Sprintf("已删除工作空间文件 `%s`。", relPath),
		fmt.Sprintf("Deleted workspace file `%s`.", relPath),
	)
}

func agentFormatWorkspaceMove(language, fromPath, toPath string) string {
	return agentText(language,
		fmt.Sprintf("已移动工作空间文件 `%s` → `%s`。", fromPath, toPath),
		fmt.Sprintf("Moved workspace file `%s` → `%s`.", fromPath, toPath),
	)
}

func agentFormatWorkspaceCopy(language, fromPath, toPath string) string {
	return agentText(language,
		fmt.Sprintf("已复制工作空间文件 `%s` → `%s`。", fromPath, toPath),
		fmt.Sprintf("Copied workspace file `%s` → `%s`.", fromPath, toPath),
	)
}

func agentFormatWorkspaceMkdir(language, relPath string) string {
	return agentText(language,
		fmt.Sprintf("已创建工作空间目录 `%s`。", relPath),
		fmt.Sprintf("Created workspace directory `%s`.", relPath),
	)
}

func agentFormatWorkspacePatch(language, relPath string, result map[string]any) string {
	diffPreview := stringFromAny(result["diff_preview"])
	contentPreview := stringFromAny(result["content_preview"])
	replacements := 1
	switch value := result["replacements"].(type) {
	case int:
		replacements = value
	case float64:
		replacements = int(value)
	}
	return agentText(language,
		fmt.Sprintf("已修改工作空间文件 `%s`（%d 处替换）。\n\nDiff 预览：\n```diff\n%s\n```\n\n内容预览：\n```text\n%s\n```", relPath, replacements, agentLimitBlock(diffPreview, 1600), agentLimitBlock(contentPreview, 1200)),
		fmt.Sprintf("Patched workspace file `%s` (%d replacement(s)).\n\nDiff preview:\n```diff\n%s\n```\n\nContent preview:\n```text\n%s\n```", relPath, replacements, agentLimitBlock(diffPreview, 1600), agentLimitBlock(contentPreview, 1200)),
	)
}

func agentFormatWorkspaceDiff(language, relPath string, result map[string]any) string {
	changeID := stringFromAny(result["change_id"])
	diffPreview := stringFromAny(result["diff_preview"])
	contentPreview := stringFromAny(result["content_preview"])
	return agentText(language,
		fmt.Sprintf("已生成待应用修改 `%s`，尚未写入文件。\n\n- change_id: `%s`\n\nDiff 预览：\n```diff\n%s\n```\n\n应用后内容预览：\n```text\n%s\n```", relPath, changeID, agentLimitBlock(diffPreview, 1600), agentLimitBlock(contentPreview, 1200)),
		fmt.Sprintf("Created staged change for `%s`; the file is not modified yet.\n\n- change_id: `%s`\n\nDiff preview:\n```diff\n%s\n```\n\nContent preview after apply:\n```text\n%s\n```", relPath, changeID, agentLimitBlock(diffPreview, 1600), agentLimitBlock(contentPreview, 1200)),
	)
}

func agentFormatWorkspaceDiffMany(language string, result map[string]any) string {
	groupID := stringFromAny(result["group_id"])
	count := intFromAny(result["count"])
	return agentText(language,
		fmt.Sprintf("已生成多文件待应用修改集。\n\n- group_id: `%s`\n- changes: %d\n\n可执行：`应用 group_id: %s` 或 `丢弃 group_id: %s`。", groupID, count, groupID, groupID),
		fmt.Sprintf("Created a multi-file staged change set.\n\n- group_id: `%s`\n- changes: %d\n\nYou can run: `apply group_id: %s` or `discard group_id: %s`.", groupID, count, groupID, groupID),
	)
}

func agentFormatWorkspaceRebase(language string, result map[string]any) string {
	oldID := stringFromAny(result["rebased_from"])
	newID := stringFromAny(result["change_id"])
	relPath := stringFromAny(result["path"])
	diffPreview := stringFromAny(result["diff_preview"])
	return agentText(language,
		fmt.Sprintf("已重新生成 staged change。\n\n- old: `%s`\n- new: `%s`\n- file: `%s`\n\nDiff 预览：\n```diff\n%s\n```", oldID, newID, relPath, agentLimitBlock(diffPreview, 1600)),
		fmt.Sprintf("Regenerated staged change.\n\n- old: `%s`\n- new: `%s`\n- file: `%s`\n\nDiff preview:\n```diff\n%s\n```", oldID, newID, relPath, agentLimitBlock(diffPreview, 1600)),
	)
}

func agentFormatWorkspaceApply(language string, result map[string]any) string {
	relPath := stringFromAny(result["path"])
	changeID := stringFromAny(result["change_id"])
	diffPreview := stringFromAny(result["diff_preview"])
	contentPreview := stringFromAny(result["content_preview"])
	return agentText(language,
		fmt.Sprintf("已应用待修改 `%s` 到 `%s`。\n\nDiff：\n```diff\n%s\n```\n\n内容预览：\n```text\n%s\n```", changeID, relPath, agentLimitBlock(diffPreview, 1600), agentLimitBlock(contentPreview, 1200)),
		fmt.Sprintf("Applied staged change `%s` to `%s`.\n\nDiff:\n```diff\n%s\n```\n\nContent preview:\n```text\n%s\n```", changeID, relPath, agentLimitBlock(diffPreview, 1600), agentLimitBlock(contentPreview, 1200)),
	)
}

func agentFormatWorkspaceDiscard(language string, result map[string]any) string {
	relPath := stringFromAny(result["path"])
	changeID := stringFromAny(result["change_id"])
	return agentText(language,
		fmt.Sprintf("已丢弃待修改 `%s`（文件 `%s` 未被修改）。", changeID, relPath),
		fmt.Sprintf("Discarded staged change `%s`; file `%s` was not modified.", changeID, relPath),
	)
}

func agentFormatWorkspaceBatch(language, action string, result map[string]any) string {
	status := stringFromAny(result["status"])
	requested := intFromAny(result["requested_count"])
	succeeded := intFromAny(result["succeeded_count"])
	failed := intFromAny(result["failed_count"])
	failures := asAgentFailureLines(result["failed"])
	if action == "apply_all" || action == "apply_group" {
		if requested == 0 {
			return agentText(language, "当前没有待应用的 staged change。", "There are no pending staged changes to apply.")
		}
		suffix := ""
		if failed > 0 {
			suffix = "\n\n失败项：\n" + failures
		}
		return agentText(language,
			fmt.Sprintf("批量应用完成：%d/%d 成功，%d 失败（status: %s）。%s", succeeded, requested, failed, status, suffix),
			fmt.Sprintf("Batch apply finished: %d/%d succeeded, %d failed (status: %s).%s", succeeded, requested, failed, status, suffix),
		)
	}
	if requested == 0 {
		return agentText(language, "当前没有待丢弃的 staged change。", "There are no pending staged changes to discard.")
	}
	suffix := ""
	if failed > 0 {
		suffix = "\n\n失败项：\n" + failures
	}
	return agentText(language,
		fmt.Sprintf("批量丢弃完成：%d/%d 成功，%d 失败（status: %s）。%s", succeeded, requested, failed, status, suffix),
		fmt.Sprintf("Batch discard finished: %d/%d succeeded, %d failed (status: %s).%s", succeeded, requested, failed, status, suffix),
	)
}

func asAgentFailureLines(value any) string {
	items, ok := value.([]map[string]any)
	if !ok || len(items) == 0 {
		return "- none"
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		changeID := stringFromAny(item["change_id"])
		message := stringFromAny(item["message"])
		if changeID == "" {
			changeID = "unknown"
		}
		if message == "" {
			message = "failed"
		}
		lines = append(lines, fmt.Sprintf("- `%s`: %s", changeID, message))
	}
	return strings.Join(lines, "\n")
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

func agentLimitBlock(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "\n... (truncated)"
}

func (m *agentManager) handleAgentCommand(ctx context.Context, sessionID, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	commands := agentExtractCommands(prompt)
	if len(commands) == 0 {
		if cmd := agentExtractCommand(prompt); cmd != "" {
			commands = []string{cmd}
		}
	}
	if len(commands) == 0 {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"我识别到你要执行命令，但没有提取到具体命令。请使用：命令: ls -la 或 command: pwd。",
			"I detected a command intent, but no command was extracted. Use: command: ls -la or command: pwd.",
		), nil, nil)}, nil, nil
	}

	ws, err := m.workspace.Create(sessionID)
	if err != nil {
		tool := runningAgentToolCall("workspace.command", strings.Join(commands, " && "))
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	executor := command.NewExecutor(ws.Path, 0, 0)
	toolCalls := make([]agentToolCall, 0, len(commands))
	var firstResult *command.Result
	for idx, cmd := range commands {
		tool := runningAgentToolCall("workspace.command", cmd)
		result, execErr := executor.Execute(ctx, cmd, "")
		if execErr != nil {
			execErr = agentCommandServiceError(execErr, cmd, "")
		}
		tool = finishAgentToolCall(tool, execErr)
		if execErr != nil {
			tool.Error = &daemonJobError{Code: services.ErrorCodeOf(execErr), Message: execErr.Error()}
			tool.Result = map[string]any{
				"command":       cmd,
				"command_index": idx + 1,
				"command_count": len(commands),
				"workdir":       ws.Path,
			}
			toolCalls = append(toolCalls, tool)
			break
		}
		if firstResult == nil {
			firstResult = result
		}
		tool.Result = map[string]any{
			"command":       result.Command,
			"command_index": idx + 1,
			"command_count": len(commands),
			"exit_code":     result.ExitCode,
			"stdout":        result.Stdout,
			"stderr":        result.Stderr,
			"duration_ms":   result.DurationMs,
			"workdir":       result.WorkDir,
		}
		toolCalls = append(toolCalls, tool)
		if result.ExitCode != 0 {
			break
		}
	}

	if len(toolCalls) == 1 && len(commands) == 1 && toolCalls[0].Status == contracts.StatusSucceeded && firstResult != nil {
		content := agentFormatCommandResult(language, firstResult)
		message := m.message(language, "tool", content, toolCalls, nil)
		return []agentMessage{message}, toolCalls, nil
	}

	content := agentFormatCommandBatchResult(language, toolCalls)
	message := m.message(language, "tool", content, toolCalls, nil)
	return []agentMessage{message}, toolCalls, nil
}

func agentCommandServiceError(err error, cmd, workdir string) error {
	if err == nil {
		return nil
	}
	if code := services.ErrorCodeOf(err); code != services.CodeInternal {
		return err
	}
	message := err.Error()
	code := services.CodeInternal
	switch {
	case strings.Contains(message, "command not allowed"):
		code = services.CodePolicyDenied
	case strings.Contains(message, "empty command"),
		strings.Contains(message, "workdir"),
		strings.Contains(message, "argument escapes workspace"):
		code = services.CodeBadRequest
	}
	return services.NewError(code, message, map[string]any{
		"command": cmd,
		"workdir": workdir,
	}, nil)
}

func (m *agentManager) handleAgentWorkspace(ctx context.Context, sessionID, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	_ = ctx

	switch {
	case agentIsWorkspaceRebaseIntent(prompt):
		oldText, newText := agentExtractWorkspaceRebaseTexts(prompt)
		return m.handleAgentWorkspaceRebase(sessionID, agentExtractWorkspaceChangeID(prompt), oldText, newText, language)
	case agentIsWorkspaceDiscardAllIntent(prompt):
		return m.handleAgentWorkspaceDiscardAll(sessionID, language)
	case agentIsWorkspaceDiscardIntent(prompt):
		if groupID := agentExtractWorkspaceGroupID(prompt); groupID != "" {
			return m.handleAgentWorkspaceDiscardGroup(sessionID, groupID, language)
		}
		return m.handleAgentWorkspaceDiscard(sessionID, agentExtractWorkspaceChangeID(prompt), language)
	case agentIsWorkspaceApplyAllIntent(prompt):
		return m.handleAgentWorkspaceApplyAll(sessionID, language)
	case agentIsWorkspaceApplyIntent(prompt):
		if groupID := agentExtractWorkspaceGroupID(prompt); groupID != "" {
			return m.handleAgentWorkspaceApplyGroup(sessionID, groupID, language)
		}
		return m.handleAgentWorkspaceApply(sessionID, agentExtractWorkspaceChangeID(prompt), language)
	case agentIsWorkspaceDeleteIntent(prompt):
		return m.handleAgentWorkspaceDelete(sessionID, agentExtractPath(prompt), language)
	case agentIsWorkspaceMoveIntent(prompt):
		fromPath, toPath := agentExtractWorkspaceMovePaths(prompt)
		return m.handleAgentWorkspaceMove(sessionID, fromPath, toPath, language)
	case agentIsWorkspaceCopyIntent(prompt):
		fromPath, toPath := agentExtractWorkspaceMovePaths(prompt)
		return m.handleAgentWorkspaceCopy(sessionID, fromPath, toPath, language)
	case agentIsWorkspaceMkdirIntent(prompt):
		return m.handleAgentWorkspaceMkdir(sessionID, agentExtractWorkspaceMkdirPath(prompt), language)
	case agentIsWorkspaceDiffIntent(prompt):
		relPath, oldText, newText := agentExtractWorkspacePatch(prompt)
		return m.handleAgentWorkspaceDiff(sessionID, relPath, oldText, newText, language)
	case agentIsWorkspacePatchIntent(prompt):
		relPath, oldText, newText := agentExtractWorkspacePatch(prompt)
		return m.handleAgentWorkspacePatch(sessionID, relPath, oldText, newText, language)
	case agentIsWorkspaceWriteIntent(prompt):
		relPath, content := agentExtractWorkspaceWrite(prompt)
		return m.handleAgentWorkspaceWrite(sessionID, relPath, content, language)
	case agentIsWorkspaceSearchIntent(prompt):
		query := agentExtractWorkspaceSearchQuery(prompt)
		return m.handleAgentWorkspaceSearch(sessionID, query, agentExtractWorkspaceSearchPath(prompt, query), false, false, 80, language)
	case agentIsWorkspaceTreeIntent(prompt):
		return m.handleAgentWorkspaceTree(sessionID, agentExtractPath(prompt), 3, 120, false, language)
	case agentIsWorkspaceReadIntent(prompt):
		return m.handleAgentWorkspaceRead(sessionID, prompt, language)
	case agentIsWorkspaceListIntent(prompt):
		return m.handleAgentWorkspaceList(sessionID, prompt, language)
	default:
		return m.handleAgentWorkspaceReady(sessionID, language)
	}
}

func (m *agentManager) handleAgentWorkspaceReady(sessionID, language string) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("workspace.prepare", sessionID)
	ws, err := m.workspace.Create(sessionID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	tool.Result = map[string]any{"session_id": sessionID, "path": ws.Path}
	content := agentText(language,
		fmt.Sprintf("工作空间已准备就绪。\n\n- session: %s\n- path: %s\n\n可直接让我：列出文件、读取文件、执行安全允许列表内的命令。", sessionID, ws.Path),
		fmt.Sprintf("Workspace is ready.\n\n- session: %s\n- path: %s\n\nYou can ask me to list files, read files, or execute allowlisted commands.", sessionID, ws.Path),
	)
	message := m.message(language, "tool", content, []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceList(sessionID, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	relPath := agentExtractPath(prompt)
	tool := runningAgentToolCall("workspace.files.list", relPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	files, err := m.workspace.ListFiles(sessionID, relPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{"session_id": sessionID, "path": relPath, "count": len(files), "files": files}

	content := agentFormatWorkspaceList(language, relPath, files)
	message := m.message(language, "tool", content, []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceRead(sessionID, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	relPath := agentExtractPath(prompt)
	if relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要读取的工作空间文件路径，例如：读取文件 notes.md。",
			"Please provide the workspace file path to read, for example: read file notes.md.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.read", relPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	data, err := m.workspace.ReadFile(sessionID, relPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	content := string(data)
	tool.Result = map[string]any{"session_id": sessionID, "path": relPath, "size": len(data), "content": content}

	message := m.message(language, "tool", agentFormatWorkspaceRead(language, relPath, content), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

type agentWorkspaceTreeEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
	Depth int    `json:"depth"`
}

type agentWorkspaceSearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func (m *agentManager) handleAgentWorkspaceTree(sessionID, relPath string, depth, limit int, includeHidden bool, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = firstNonEmpty(relPath, ".")
	if depth <= 0 {
		depth = 3
	}
	if depth > 8 {
		depth = 8
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 500 {
		limit = 500
	}

	tool := runningAgentToolCall("workspace.files.tree", relPath)
	ws, err := m.workspace.Create(sessionID)
	if err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	root, cleanRel, err := agentWorkspaceResolvePath(ws.Path, relPath)
	var entries []agentWorkspaceTreeEntry
	truncated := false
	if err == nil {
		stopWalk := errors.New("agent workspace tree limit reached")
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			name := d.Name()
			if !includeHidden && strings.HasPrefix(name, ".") && path != root {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			entryDepth := 0
			if rel != "." {
				entryDepth = len(strings.Split(rel, "/"))
			}
			if entryDepth > depth {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if rel != "." {
				info, _ := d.Info()
				size := int64(0)
				if info != nil && !d.IsDir() {
					size = info.Size()
				}
				p := rel
				if cleanRel != "." {
					p = filepath.ToSlash(filepath.Join(cleanRel, rel))
				}
				entries = append(entries, agentWorkspaceTreeEntry{Path: p, IsDir: d.IsDir(), Size: size, Depth: entryDepth})
				if len(entries) >= limit {
					truncated = true
					return stopWalk
				}
			}
			if d.IsDir() && entryDepth >= depth {
				return filepath.SkipDir
			}
			return nil
		})
		if errors.Is(err, stopWalk) {
			err = nil
		}
	}

	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id":      sessionID,
		"path":            cleanRel,
		"depth":           depth,
		"limit":           limit,
		"include_hidden":  includeHidden,
		"count":           len(entries),
		"entries":         entries,
		"truncated":       truncated,
		"workspace_path":  ws.Path,
		"workspace_scope": "agent_workspace",
	}
	message := m.message(language, "tool", agentFormatWorkspaceTree(language, cleanRel, entries, truncated), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceSearch(sessionID, query, relPath string, regexMode, caseSensitive bool, limit int, language string) ([]agentMessage, []agentToolCall, error) {
	query = strings.TrimSpace(query)
	relPath = firstNonEmpty(relPath, ".")
	if limit <= 0 {
		limit = 80
	}
	if limit > 300 {
		limit = 300
	}

	tool := runningAgentToolCall("workspace.search", query)
	ws, err := m.workspace.Create(sessionID)
	if err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	root, cleanRel, err := agentWorkspaceResolvePath(ws.Path, relPath)
	var matcher *regexp.Regexp
	if err == nil && regexMode {
		pattern := query
		if !caseSensitive {
			pattern = "(?i)" + pattern
		}
		matcher, err = regexp.Compile(pattern)
	}

	matches := make([]agentWorkspaceSearchMatch, 0, minInt(limit, 64))
	filesScanned := 0
	truncated := false
	if err == nil {
		stopWalk := errors.New("agent workspace search limit reached")
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				if d != nil && d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			info, infoErr := d.Info()
			if infoErr != nil || info.Size() > 1024*1024 {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || agentLooksBinary(data) {
				return nil
			}
			filesScanned++
			lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
			for idx, line := range lines {
				if agentLineMatches(line, query, matcher, regexMode, caseSensitive) {
					rel, relErr := filepath.Rel(root, path)
					if relErr != nil {
						continue
					}
					rel = filepath.ToSlash(rel)
					if cleanRel != "." {
						rel = filepath.ToSlash(filepath.Join(cleanRel, rel))
					}
					matches = append(matches, agentWorkspaceSearchMatch{
						Path: rel,
						Line: idx + 1,
						Text: agentLimitBlock(strings.TrimSpace(line), 500),
					})
					if len(matches) >= limit {
						truncated = true
						return stopWalk
					}
				}
			}
			return nil
		})
		if errors.Is(err, stopWalk) {
			err = nil
		}
	}

	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id":     sessionID,
		"path":           cleanRel,
		"query":          query,
		"regex":          regexMode,
		"case_sensitive": caseSensitive,
		"limit":          limit,
		"files_scanned":  filesScanned,
		"count":          len(matches),
		"matches":        matches,
		"truncated":      truncated,
	}
	message := m.message(language, "tool", agentFormatWorkspaceSearch(language, query, matches, filesScanned, truncated), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentHTTPRequest(ctx context.Context, method, targetURL string, requestHeaders map[string]string, requestBodyText string, language string, authProfile *agentHTTPAuthProfileUsage) ([]agentMessage, []agentToolCall, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return nil, nil, services.NewError(
			services.CodePolicyDenied,
			"http.request allows only GET, HEAD, OPTIONS, POST, PUT, PATCH, or DELETE",
			map[string]any{"method": method},
			nil,
		)
	}
	if agentHTTPMethodIsSafe(method) && strings.TrimSpace(requestBodyText) != "" {
		return nil, nil, services.NewError(
			services.CodePolicyDenied,
			"http.request body is only allowed with POST, PUT, PATCH, or DELETE",
			map[string]any{"method": method},
			nil,
		)
	}
	if len(requestBodyText) > 1024*1024 {
		return nil, nil, services.NewError(services.CodeBadRequest, "http.request body is too large", map[string]any{"max_bytes": 1024 * 1024}, nil)
	}
	targetURL = agentNormalizeURLInput(targetURL)
	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, nil, services.NewError(services.CodeBadRequest, "http.request requires an http/https url", map[string]any{"url": targetURL}, err)
	}
	if err := agentURLOutboundAllowed(parsed); err != nil {
		return nil, nil, err
	}

	tool := runningAgentToolCall("http.request", method+" "+parsed.String())
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var requestBody io.Reader
	if strings.TrimSpace(requestBodyText) != "" {
		requestBody = strings.NewReader(requestBodyText)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, parsed.String(), requestBody)
	if err == nil {
		req.Header.Set("User-Agent", "CyberAgent/agent-http-tool")
		req.Header.Set("Accept", "*/*")
		for key, value := range requestHeaders {
			req.Header.Set(key, value)
		}
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	var result map[string]any
	if err == nil {
		var resp *http.Response
		resp, err = client.Do(req)
		if resp != nil {
			defer resp.Body.Close()
			limited := io.LimitReader(resp.Body, 64*1024+1)
			body, readErr := io.ReadAll(limited)
			if readErr != nil && err == nil {
				err = readErr
			}
			truncated := len(body) > 64*1024
			if truncated {
				body = body[:64*1024]
			}
			headers := map[string]string{}
			for key, values := range resp.Header {
				headers[key] = strings.Join(values, ", ")
			}
			result = map[string]any{
				"method":             method,
				"url":                parsed.String(),
				"status":             resp.Status,
				"status_code":        resp.StatusCode,
				"headers":            headers,
				"request_headers":    agentRedactHTTPHeaders(requestHeaders),
				"request_body_bytes": len(requestBodyText),
				"body":               string(body),
				"body_bytes":         len(body),
				"body_truncated":     truncated,
			}
			if authProfile != nil && authProfile.Applied {
				result["auth_profile"] = authProfile
			}
		}
	}
	tool = finishAgentToolCall(tool, err)
	if result != nil {
		tool.Result = result
	}
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	message := m.message(language, "tool", agentFormatHTTPRequest(language, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func agentWorkspaceResolvePath(wsPath, relPath string) (string, string, error) {
	cleanRel := filepath.Clean(strings.TrimSpace(relPath))
	if cleanRel == "" || cleanRel == string(filepath.Separator) {
		cleanRel = "."
	}
	if filepath.IsAbs(cleanRel) {
		return "", cleanRel, fmt.Errorf("absolute workspace paths are not allowed")
	}
	wsClean := filepath.Clean(wsPath)
	absPath := filepath.Clean(filepath.Join(wsClean, cleanRel))
	if absPath != wsClean && !strings.HasPrefix(absPath, wsClean+string(os.PathSeparator)) {
		return "", cleanRel, fmt.Errorf("path traversal not allowed")
	}
	return absPath, filepath.ToSlash(cleanRel), nil
}

func agentLooksBinary(data []byte) bool {
	limit := minInt(len(data), 8000)
	for i := 0; i < limit; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func agentLineMatches(line, query string, matcher *regexp.Regexp, regexMode, caseSensitive bool) bool {
	if regexMode {
		return matcher != nil && matcher.MatchString(line)
	}
	if caseSensitive {
		return strings.Contains(line, query)
	}
	return strings.Contains(strings.ToLower(line), strings.ToLower(query))
}

func (m *agentManager) handleAgentWorkspaceWrite(sessionID, relPath, content, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	content = agentCleanWorkspaceWriteContent(content)
	if relPath == "" || relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要写入的工作空间文件路径，例如：写入文件 notes.md 内容: hello。",
			"Please provide the workspace file path to write, for example: write file notes.md content: hello.",
		), nil, nil)}, nil, nil
	}
	if strings.TrimSpace(content) == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要写入文件的内容，例如：写入文件 notes.md 内容: hello。",
			"Please provide the file content, for example: write file notes.md content: hello.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.write", relPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	data := []byte(content)
	err := m.workspace.WriteFile(sessionID, relPath, data)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id":      sessionID,
		"path":            relPath,
		"size":            len(data),
		"content_preview": agentLimitBlock(content, 500),
	}

	message := m.message(language, "tool", agentFormatWorkspaceWrite(language, relPath, content, len(data)), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceDelete(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	if relPath == "" || relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要删除的工作空间文件路径，例如：删除文件 notes.md。",
			"Please provide the workspace file path to delete, for example: delete file notes.md.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.delete", relPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	err := m.workspace.DeleteFile(sessionID, relPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id": sessionID,
		"path":       relPath,
		"status":     "deleted",
	}

	message := m.message(language, "tool", agentFormatWorkspaceDelete(language, relPath), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceMove(sessionID, fromPath, toPath, language string) ([]agentMessage, []agentToolCall, error) {
	fromPath = agentCleanWorkspaceWritePath(fromPath)
	toPath = agentCleanWorkspaceWritePath(toPath)
	if fromPath == "" || fromPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要移动的源文件路径，例如：重命名文件 notes.md 为 archive/notes.md。",
			"Please provide the source file path to move, for example: rename file notes.md to archive/notes.md.",
		), nil, nil)}, nil, nil
	}
	if toPath == "" || toPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要移动到的目标文件路径，例如：重命名文件 notes.md 为 archive/notes.md。",
			"Please provide the destination file path to move to, for example: rename file notes.md to archive/notes.md.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.move", fromPath+" -> "+toPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	err := m.workspace.MoveFile(sessionID, fromPath, toPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id": sessionID,
		"from":       fromPath,
		"path":       toPath,
		"status":     "moved",
	}

	message := m.message(language, "tool", agentFormatWorkspaceMove(language, fromPath, toPath), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceCopy(sessionID, fromPath, toPath, language string) ([]agentMessage, []agentToolCall, error) {
	fromPath = agentCleanWorkspaceWritePath(fromPath)
	toPath = agentCleanWorkspaceWritePath(toPath)
	if fromPath == "" || fromPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要复制的源文件路径，例如：复制文件 notes.md 到 archive/notes-copy.md。",
			"Please provide the source file path to copy, for example: copy file notes.md to archive/notes-copy.md.",
		), nil, nil)}, nil, nil
	}
	if toPath == "" || toPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要复制到的目标文件路径，例如：复制文件 notes.md 到 archive/notes-copy.md。",
			"Please provide the destination file path to copy to, for example: copy file notes.md to archive/notes-copy.md.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.copy", fromPath+" -> "+toPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	err := m.workspace.CopyFile(sessionID, fromPath, toPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id": sessionID,
		"from":       fromPath,
		"path":       toPath,
		"status":     "copied",
	}

	message := m.message(language, "tool", agentFormatWorkspaceCopy(language, fromPath, toPath), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceMkdir(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	if relPath == "" || relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要创建的工作空间目录路径，例如：创建目录 reports。",
			"Please provide the workspace directory path to create, for example: mkdir reports.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.mkdir", relPath)
	if _, err := m.workspace.Create(sessionID); err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	err := m.workspace.MakeDir(sessionID, relPath)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"session_id": sessionID,
		"path":       relPath,
		"status":     "created",
	}

	message := m.message(language, "tool", agentFormatWorkspaceMkdir(language, relPath), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) patchWorkspaceFile(sessionID, relPath, oldText, newText string) (map[string]any, error) {
	change, _, err := m.buildWorkspaceChange(sessionID, relPath, oldText, newText, "workspace.files.patch")
	if err != nil {
		return nil, err
	}
	if err := m.applyWorkspaceChangeToFile(sessionID, change); err != nil {
		return nil, err
	}
	return map[string]any{
		"session_id":      sessionID,
		"path":            change.Path,
		"size_before":     change.SizeBefore,
		"size_after":      change.SizeAfter,
		"replacements":    change.Replacements,
		"diff_preview":    change.DiffPreview,
		"content_preview": change.ContentPreview,
	}, nil
}

func (m *agentManager) buildWorkspaceChange(sessionID, relPath, oldText, newText, toolName string) (agentWorkspaceChange, string, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	oldText = agentCleanWorkspacePatchText(oldText)
	newText = agentCleanWorkspacePatchText(newText)
	if relPath == "" || relPath == "." {
		return agentWorkspaceChange{}, "", services.NewError(services.CodeBadRequest, toolName+" requires path", map[string]any{"field": "path"}, nil)
	}
	if oldText == "" {
		return agentWorkspaceChange{}, "", services.NewError(services.CodeBadRequest, toolName+" requires old text", map[string]any{"field": "old"}, nil)
	}
	if _, err := m.workspace.Create(sessionID); err != nil {
		return agentWorkspaceChange{}, "", err
	}
	data, err := m.workspace.ReadFile(sessionID, relPath)
	if err != nil {
		return agentWorkspaceChange{}, "", err
	}
	before := string(data)
	replacements := strings.Count(before, oldText)
	switch {
	case replacements == 0:
		return agentWorkspaceChange{}, "", services.NewError(services.CodeConflict, "old text was not found in workspace file", map[string]any{"path": relPath}, nil)
	case replacements > 1:
		return agentWorkspaceChange{}, "", services.NewError(services.CodeConflict, "old text is ambiguous; provide a longer exact block", map[string]any{"path": relPath, "matches": replacements}, nil)
	}
	after := strings.Replace(before, oldText, newText, 1)
	now := time.Now().UTC()
	change := agentWorkspaceChange{
		ID:             agentWorkspaceChangeID(now, relPath),
		Path:           relPath,
		OldText:        oldText,
		NewText:        newText,
		SizeBefore:     len(before),
		SizeAfter:      len(after),
		Replacements:   1,
		DiffPreview:    agentDiffPreview(relPath, oldText, newText),
		ContentPreview: agentLimitBlock(after, 500),
		Status:         "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return change, after, nil
}

func (m *agentManager) prepareWorkspaceChangeApply(sessionID string, change agentWorkspaceChange) (preparedWorkspaceApply, error) {
	if change.Path == "" || change.OldText == "" {
		return preparedWorkspaceApply{}, services.NewError(services.CodeBadRequest, "workspace change is incomplete", map[string]any{"change_id": change.ID}, nil)
	}
	if _, err := m.workspace.Create(sessionID); err != nil {
		return preparedWorkspaceApply{}, err
	}
	data, err := m.workspace.ReadFile(sessionID, change.Path)
	if err != nil {
		return preparedWorkspaceApply{}, err
	}
	before := string(data)
	replacements := strings.Count(before, change.OldText)
	switch {
	case replacements == 0:
		return preparedWorkspaceApply{}, services.NewError(services.CodeConflict, "staged old text is no longer present in workspace file", map[string]any{"change_id": change.ID, "path": change.Path}, nil)
	case replacements > 1:
		return preparedWorkspaceApply{}, services.NewError(services.CodeConflict, "staged old text is now ambiguous; regenerate the diff", map[string]any{"change_id": change.ID, "path": change.Path, "matches": replacements}, nil)
	}
	after := strings.Replace(before, change.OldText, change.NewText, 1)
	return preparedWorkspaceApply{Change: change, Before: before, After: after}, nil
}

func (m *agentManager) applyWorkspaceChangeToFile(sessionID string, change agentWorkspaceChange) error {
	prepared, err := m.prepareWorkspaceChangeApply(sessionID, change)
	if err != nil {
		return err
	}
	return m.workspace.WriteFile(sessionID, prepared.Change.Path, []byte(prepared.After))
}

func agentWorkspaceChangeID(now time.Time, relPath string) string {
	clean := regexp.MustCompile(`[^A-Za-z0-9]+`).ReplaceAllString(relPath, "_")
	clean = strings.Trim(clean, "_")
	if clean == "" {
		clean = "file"
	}
	if len(clean) > 32 {
		clean = clean[:32]
	}
	return fmt.Sprintf("change_%s_%s", now.Format("20060102_150405_000000000"), clean)
}

func agentWorkspaceGroupID(now time.Time) string {
	return strings.Replace(runs.NewJobID(now), "job_", "changeset_", 1)
}

func (m *agentManager) stageWorkspaceFileChange(sessionID, relPath, oldText, newText, toolCallID string) (map[string]any, error) {
	change, _, err := m.buildWorkspaceChange(sessionID, relPath, oldText, newText, "workspace.files.diff")
	if err != nil {
		return nil, err
	}
	change.ToolCallID = toolCallID
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return nil, loadErr
		}
		session = loaded
	}
	now := time.Now().UTC()
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].Status == "pending" && session.PendingChanges[idx].Path == change.Path {
			session.PendingChanges[idx].Status = "superseded"
			session.PendingChanges[idx].UpdatedAt = now
		}
	}
	session.PendingChanges = append([]agentWorkspaceChange{change}, session.PendingChanges...)
	if len(session.PendingChanges) > 20 {
		session.PendingChanges = session.PendingChanges[:20]
	}
	session.UpdatedAt = now
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()
	return workspaceChangeResult(sessionID, change), nil
}

func (m *agentManager) stageWorkspaceFileChanges(sessionID string, inputs []agentWorkspaceChangeInput, toolCallID string) (map[string]any, error) {
	if len(inputs) == 0 {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.diff_many requires changes", map[string]any{"field": "changes"}, nil)
	}
	if len(inputs) > 20 {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.diff_many supports at most 20 changes", map[string]any{"field": "changes", "max": 20}, nil)
	}
	now := time.Now().UTC()
	groupID := agentWorkspaceGroupID(now)
	seenPaths := map[string]struct{}{}
	changes := make([]agentWorkspaceChange, 0, len(inputs))
	results := make([]map[string]any, 0, len(inputs))
	for idx, input := range inputs {
		change, _, err := m.buildWorkspaceChange(sessionID, input.Path, input.Old, input.New, "workspace.files.diff_many")
		if err != nil {
			return nil, err
		}
		if _, ok := seenPaths[change.Path]; ok {
			return nil, services.NewError(services.CodeConflict, "workspace.files.diff_many cannot include the same path twice", map[string]any{"path": change.Path, "index": idx}, nil)
		}
		seenPaths[change.Path] = struct{}{}
		change.ID = fmt.Sprintf("%s_%02d", change.ID, idx+1)
		change.GroupID = groupID
		change.ToolCallID = toolCallID
		changes = append(changes, change)
	}

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return nil, loadErr
		}
		session = loaded
	}
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].Status != "pending" {
			continue
		}
		if _, ok := seenPaths[session.PendingChanges[idx].Path]; ok {
			session.PendingChanges[idx].Status = "superseded"
			session.PendingChanges[idx].UpdatedAt = now
		}
	}
	for _, change := range changes {
		results = append(results, workspaceChangeResult(sessionID, change))
	}
	session.PendingChanges = append(changes, session.PendingChanges...)
	if len(session.PendingChanges) > 50 {
		session.PendingChanges = session.PendingChanges[:50]
	}
	session.UpdatedAt = now
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()

	return map[string]any{
		"session_id": sessionID,
		"group_id":   groupID,
		"count":      len(changes),
		"changes":    results,
		"status":     "pending",
	}, nil
}

func (m *agentManager) rebaseWorkspaceFileChange(sessionID, changeID, oldText, newText, toolCallID string) (map[string]any, error) {
	changeID = agentCleanToken(changeID)
	oldText = agentCleanWorkspacePatchText(oldText)
	newText = agentCleanWorkspacePatchText(newText)
	if changeID == "" {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.rebase requires change_id", map[string]any{"field": "change_id"}, nil)
	}
	if oldText == "" {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.rebase requires current old text", map[string]any{"field": "old"}, nil)
	}

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return nil, loadErr
		}
		session = loaded
	}
	var original agentWorkspaceChange
	found := false
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID == changeID {
			original = session.PendingChanges[idx]
			found = true
			break
		}
	}
	m.mu.Unlock()

	if !found {
		return nil, services.NewError(services.CodeNotFound, "workspace staged change not found", map[string]any{"change_id": changeID}, nil)
	}
	if original.Status != "pending" && original.Status != "conflict" {
		return nil, services.NewError(services.CodeConflict, "workspace staged change cannot be rebased", map[string]any{"change_id": changeID, "status": original.Status}, nil)
	}
	if newText == "" {
		newText = original.NewText
	}
	change, _, err := m.buildWorkspaceChange(sessionID, original.Path, oldText, newText, "workspace.files.rebase")
	if err != nil {
		return nil, err
	}
	change.GroupID = original.GroupID
	change.ToolCallID = toolCallID

	now := time.Now().UTC()
	m.mu.Lock()
	session, ok = m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return nil, loadErr
		}
		session = loaded
	}
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID == changeID ||
			(session.PendingChanges[idx].Status == "pending" && session.PendingChanges[idx].Path == change.Path) {
			session.PendingChanges[idx].Status = "superseded"
			session.PendingChanges[idx].Error = ""
			session.PendingChanges[idx].UpdatedAt = now
		}
	}
	session.PendingChanges = append([]agentWorkspaceChange{change}, session.PendingChanges...)
	if len(session.PendingChanges) > 50 {
		session.PendingChanges = session.PendingChanges[:50]
	}
	session.UpdatedAt = now
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()

	result := workspaceChangeResult(sessionID, change)
	result["rebased_from"] = changeID
	return result, nil
}

func (m *agentManager) applyWorkspaceFileChange(sessionID, changeID string) (map[string]any, error) {
	changeID = agentCleanToken(changeID)
	if changeID == "" {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.apply requires change_id", map[string]any{"field": "change_id"}, nil)
	}

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return nil, loadErr
		}
		session = loaded
	}
	changeIndex := -1
	var change agentWorkspaceChange
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID == changeID {
			changeIndex = idx
			change = session.PendingChanges[idx]
			break
		}
	}
	m.mu.Unlock()

	if changeIndex < 0 {
		return nil, services.NewError(services.CodeNotFound, "workspace staged change not found", map[string]any{"change_id": changeID}, nil)
	}
	if change.Status != "pending" {
		return nil, services.NewError(services.CodeConflict, "workspace staged change is not pending", map[string]any{"change_id": changeID, "status": change.Status}, nil)
	}
	if err := m.applyWorkspaceChangeToFile(sessionID, change); err != nil {
		if services.ErrorCodeOf(err) == services.CodeConflict {
			m.markWorkspaceChangeConflict(sessionID, changeID, err)
		}
		return nil, err
	}

	now := time.Now().UTC()
	change.Status = "applied"
	change.UpdatedAt = now
	change.AppliedAt = &now
	m.mu.Lock()
	session, ok = m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return nil, loadErr
		}
		session = loaded
	}
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID == changeID {
			session.PendingChanges[idx] = change
			break
		}
	}
	session.UpdatedAt = now
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()

	result := workspaceChangeResult(sessionID, change)
	result["status"] = "applied"
	return result, nil
}

func (m *agentManager) markWorkspaceChangeConflict(sessionID, changeID string, cause error) {
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			return
		}
		session = loaded
	}
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID != changeID {
			continue
		}
		if session.PendingChanges[idx].Status == "pending" {
			session.PendingChanges[idx].Status = "conflict"
			session.PendingChanges[idx].Error = cause.Error()
			session.PendingChanges[idx].UpdatedAt = now
			session.UpdatedAt = now
			_ = m.persistSessionLocked(session)
		}
		return
	}
}

func (m *agentManager) markWorkspaceChangesApplied(sessionID string, applied []agentWorkspaceChange) error {
	now := time.Now().UTC()
	byID := map[string]agentWorkspaceChange{}
	for _, change := range applied {
		change.Status = "applied"
		change.Error = ""
		change.UpdatedAt = now
		change.AppliedAt = &now
		byID[change.ID] = change
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			return loadErr
		}
		session = loaded
	}
	for idx := range session.PendingChanges {
		if change, ok := byID[session.PendingChanges[idx].ID]; ok {
			session.PendingChanges[idx] = change
		}
	}
	session.UpdatedAt = now
	return m.persistSessionLocked(session)
}

func (m *agentManager) discardWorkspaceFileChange(sessionID, changeID string) (map[string]any, error) {
	changeID = agentCleanToken(changeID)
	if changeID == "" {
		return nil, services.NewError(services.CodeBadRequest, "workspace.files.discard requires change_id", map[string]any{"field": "change_id"}, nil)
	}

	now := time.Now().UTC()
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			m.mu.Unlock()
			return nil, loadErr
		}
		session = loaded
	}
	changeIndex := -1
	var change agentWorkspaceChange
	for idx := range session.PendingChanges {
		if session.PendingChanges[idx].ID == changeID {
			changeIndex = idx
			change = session.PendingChanges[idx]
			break
		}
	}
	if changeIndex < 0 {
		m.mu.Unlock()
		return nil, services.NewError(services.CodeNotFound, "workspace staged change not found", map[string]any{"change_id": changeID}, nil)
	}
	if change.Status != "pending" && change.Status != "conflict" {
		m.mu.Unlock()
		return nil, services.NewError(services.CodeConflict, "workspace staged change is not pending", map[string]any{"change_id": changeID, "status": change.Status}, nil)
	}
	change.Status = "discarded"
	change.Error = ""
	change.UpdatedAt = now
	session.PendingChanges[changeIndex] = change
	session.UpdatedAt = now
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()

	result := workspaceChangeResult(sessionID, change)
	result["status"] = "discarded"
	return result, nil
}

func (m *agentManager) pendingWorkspaceChangeIDs(sessionID string) ([]string, error) {
	return m.workspaceChangeIDsByStatus(sessionID, "pending")
}

func (m *agentManager) discardableWorkspaceChangeIDs(sessionID string) ([]string, error) {
	return m.workspaceChangeIDsByStatus(sessionID, "pending", "conflict")
}

func (m *agentManager) workspaceChangeIDsByGroup(sessionID, groupID string, statuses ...string) ([]string, error) {
	changes, err := m.workspaceChangesByGroup(sessionID, groupID, statuses...)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(changes))
	for _, change := range changes {
		ids = append(ids, change.ID)
	}
	return ids, nil
}

func (m *agentManager) workspaceChangesByGroup(sessionID, groupID string, statuses ...string) ([]agentWorkspaceChange, error) {
	groupID = agentCleanToken(groupID)
	if groupID == "" {
		return nil, services.NewError(services.CodeBadRequest, "workspace change group requires group_id", map[string]any{"field": "group_id"}, nil)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			return nil, loadErr
		}
		session = loaded
	}
	allowed := map[string]struct{}{}
	for _, status := range statuses {
		allowed[status] = struct{}{}
	}
	changes := make([]agentWorkspaceChange, 0, len(session.PendingChanges))
	for idx := range session.PendingChanges {
		change := session.PendingChanges[idx]
		if change.GroupID != groupID || change.ID == "" {
			continue
		}
		if _, ok := allowed[change.Status]; ok {
			changes = append(changes, change)
		}
	}
	if len(changes) == 0 {
		return nil, services.NewError(services.CodeNotFound, "workspace change group has no matching changes", map[string]any{"group_id": groupID}, nil)
	}
	return changes, nil
}

func (m *agentManager) workspaceChangeIDsByStatus(sessionID string, statuses ...string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		loaded, loadErr := m.loadSessionLocked(sessionID)
		if loadErr != nil {
			return nil, loadErr
		}
		session = loaded
	}
	allowed := map[string]struct{}{}
	for _, status := range statuses {
		allowed[status] = struct{}{}
	}
	ids := make([]string, 0, len(session.PendingChanges))
	for idx := range session.PendingChanges {
		change := session.PendingChanges[idx]
		if _, ok := allowed[change.Status]; ok && change.ID != "" {
			ids = append(ids, change.ID)
		}
	}
	return ids, nil
}

func workspaceBatchResult(sessionID, action string, requested int, succeeded []map[string]any, failed []map[string]any) map[string]any {
	status := "succeeded"
	switch {
	case requested == 0:
		status = "empty"
	case len(failed) == requested:
		status = "failed"
	case len(failed) > 0:
		status = "partial"
	}
	return map[string]any{
		"session_id":      sessionID,
		"action":          action,
		"status":          status,
		"requested_count": requested,
		"succeeded_count": len(succeeded),
		"failed_count":    len(failed),
		"succeeded":       succeeded,
		"failed":          failed,
	}
}

func workspaceChangeResult(sessionID string, change agentWorkspaceChange) map[string]any {
	return map[string]any{
		"session_id":      sessionID,
		"change_id":       change.ID,
		"group_id":        change.GroupID,
		"path":            change.Path,
		"size_before":     change.SizeBefore,
		"size_after":      change.SizeAfter,
		"replacements":    change.Replacements,
		"diff_preview":    change.DiffPreview,
		"content_preview": change.ContentPreview,
		"status":          change.Status,
		"error":           change.Error,
		"created_at":      change.CreatedAt,
		"updated_at":      change.UpdatedAt,
	}
}

func (m *agentManager) handleAgentWorkspacePatch(sessionID, relPath, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	oldText = agentCleanWorkspacePatchText(oldText)
	newText = agentCleanWorkspacePatchText(newText)
	if relPath == "" || relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要修改的工作空间文件路径，例如：修改文件 notes.md old: hello new: world。",
			"Please provide the workspace file path to patch, for example: patch file notes.md old: hello new: world.",
		), nil, nil)}, nil, nil
	}
	if oldText == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要精确替换的 old 文本，例如：修改文件 notes.md old: hello new: world。",
			"Please provide the exact old text to replace, for example: patch file notes.md old: hello new: world.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.patch", relPath)
	result, err := m.patchWorkspaceFile(sessionID, relPath, oldText, newText)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "path": relPath}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result

	message := m.message(language, "tool", agentFormatWorkspacePatch(language, relPath, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceDiff(sessionID, relPath, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	relPath = agentCleanWorkspaceWritePath(relPath)
	oldText = agentCleanWorkspacePatchText(oldText)
	newText = agentCleanWorkspacePatchText(newText)
	if relPath == "" || relPath == "." {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请告诉我要生成 diff 的工作空间文件路径，例如：预览修改文件 notes.md old: hello new: world。",
			"Please provide the workspace file path to diff, for example: preview patch file notes.md old: hello new: world.",
		), nil, nil)}, nil, nil
	}
	if oldText == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要预览替换的 old 文本，例如：预览修改文件 notes.md old: hello new: world。",
			"Please provide the exact old text to preview, for example: preview patch file notes.md old: hello new: world.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.diff", relPath)
	result, err := m.stageWorkspaceFileChange(sessionID, relPath, oldText, newText, tool.ID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "path": relPath}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result

	message := m.message(language, "tool", agentFormatWorkspaceDiff(language, relPath, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceDiffMany(sessionID string, changes []agentWorkspaceChangeInput, language string) ([]agentMessage, []agentToolCall, error) {
	if len(changes) == 0 {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供 changes 数组，例如：[{\"path\":\"a.md\",\"old\":\"before\",\"new\":\"after\"}]。",
			"Please provide a changes array, for example: [{\"path\":\"a.md\",\"old\":\"before\",\"new\":\"after\"}].",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.diff_many", fmt.Sprintf("%d changes", len(changes)))
	result, err := m.stageWorkspaceFileChanges(sessionID, changes, tool.ID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "count": len(changes)}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceDiffMany(language, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceRebase(sessionID, changeID, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	changeID = agentCleanToken(changeID)
	oldText = agentCleanWorkspacePatchText(oldText)
	newText = agentCleanWorkspacePatchText(newText)
	if changeID == "" || oldText == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供 change_id 和当前文件中的精确 old 文本，例如：重新生成 change_id: change_xxx old: 当前文本。",
			"Please provide change_id and the current exact old text, for example: rebase change_id: change_xxx old: current text.",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.rebase", changeID)
	result, err := m.rebaseWorkspaceFileChange(sessionID, changeID, oldText, newText, tool.ID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "change_id": changeID}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceRebase(language, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceApply(sessionID, changeID, language string) ([]agentMessage, []agentToolCall, error) {
	changeID = agentCleanToken(changeID)
	if changeID == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要应用的 change_id，例如：应用 change_id: change_20260527_...。",
			"Please provide the change_id to apply, for example: apply change_id: change_20260527_...",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.apply", changeID)
	result, err := m.applyWorkspaceFileChange(sessionID, changeID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "change_id": changeID}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result

	message := m.message(language, "tool", agentFormatWorkspaceApply(language, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceApplyAll(sessionID, language string) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("workspace.files.apply", "all")
	changeIDs, err := m.pendingWorkspaceChangeIDs(sessionID)
	if err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "action": "apply_all"}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	applied := make([]map[string]any, 0, len(changeIDs))
	failed := make([]map[string]any, 0)
	for _, changeID := range changeIDs {
		result, applyErr := m.applyWorkspaceFileChange(sessionID, changeID)
		if applyErr != nil {
			failed = append(failed, map[string]any{
				"change_id": changeID,
				"code":      services.ErrorCodeOf(applyErr),
				"message":   applyErr.Error(),
			})
			continue
		}
		applied = append(applied, result)
	}

	result := workspaceBatchResult(sessionID, "apply_all", len(changeIDs), applied, failed)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceBatch(language, "apply_all", result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) applyWorkspaceChangeGroup(sessionID, groupID string) (map[string]any, error) {
	changes, err := m.workspaceChangesByGroup(sessionID, groupID, "pending")
	if err != nil {
		return nil, err
	}
	prepared := make([]preparedWorkspaceApply, 0, len(changes))
	failed := make([]map[string]any, 0)
	for _, change := range changes {
		item, prepareErr := m.prepareWorkspaceChangeApply(sessionID, change)
		if prepareErr != nil {
			if services.ErrorCodeOf(prepareErr) == services.CodeConflict {
				m.markWorkspaceChangeConflict(sessionID, change.ID, prepareErr)
			}
			failed = append(failed, map[string]any{
				"change_id": change.ID,
				"path":      change.Path,
				"code":      services.ErrorCodeOf(prepareErr),
				"message":   prepareErr.Error(),
			})
			continue
		}
		prepared = append(prepared, item)
	}
	if len(failed) > 0 {
		return nil, services.NewError(services.CodeConflict, "workspace change group preflight failed; no files were modified", map[string]any{
			"group_id": groupID,
			"failed":   failed,
		}, nil)
	}

	written := make([]preparedWorkspaceApply, 0, len(prepared))
	for _, item := range prepared {
		if writeErr := m.workspace.WriteFile(sessionID, item.Change.Path, []byte(item.After)); writeErr != nil {
			for i := len(written) - 1; i >= 0; i-- {
				_ = m.workspace.WriteFile(sessionID, written[i].Change.Path, []byte(written[i].Before))
			}
			return nil, services.NewError(services.CodeInternal, "workspace change group apply failed; attempted rollback", map[string]any{
				"group_id": groupID,
				"path":     item.Change.Path,
			}, writeErr)
		}
		written = append(written, item)
	}

	appliedChanges := make([]agentWorkspaceChange, 0, len(prepared))
	applied := make([]map[string]any, 0, len(prepared))
	now := time.Now().UTC()
	for _, item := range prepared {
		change := item.Change
		change.Status = "applied"
		change.Error = ""
		change.UpdatedAt = now
		change.AppliedAt = &now
		appliedChanges = append(appliedChanges, change)
		applied = append(applied, workspaceChangeResult(sessionID, change))
	}
	if err := m.markWorkspaceChangesApplied(sessionID, appliedChanges); err != nil {
		return nil, err
	}
	result := workspaceBatchResult(sessionID, "apply_group", len(changes), applied, failed)
	result["group_id"] = groupID
	result["atomic"] = true
	return result, nil
}

func (m *agentManager) handleAgentWorkspaceApplyGroup(sessionID, groupID, language string) ([]agentMessage, []agentToolCall, error) {
	groupID = agentCleanToken(groupID)
	if groupID == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要应用的 group_id，例如：应用 group_id: changeset_20260527_...。",
			"Please provide the group_id to apply, for example: apply group_id: changeset_20260527_...",
		), nil, nil)}, nil, nil
	}
	tool := runningAgentToolCall("workspace.files.apply", groupID)
	result, err := m.applyWorkspaceChangeGroup(sessionID, groupID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "group_id": groupID}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceBatch(language, "apply_group", result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceDiscard(sessionID, changeID, language string) ([]agentMessage, []agentToolCall, error) {
	changeID = agentCleanToken(changeID)
	if changeID == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要丢弃的 change_id，例如：丢弃 change_id: change_20260527_...。",
			"Please provide the change_id to discard, for example: discard change_id: change_20260527_...",
		), nil, nil)}, nil, nil
	}

	tool := runningAgentToolCall("workspace.files.discard", changeID)
	result, err := m.discardWorkspaceFileChange(sessionID, changeID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "change_id": changeID}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result

	message := m.message(language, "tool", agentFormatWorkspaceDiscard(language, result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) discardWorkspaceChangeGroup(sessionID, groupID string) (map[string]any, error) {
	changeIDs, err := m.workspaceChangeIDsByGroup(sessionID, groupID, "pending", "conflict")
	if err != nil {
		return nil, err
	}
	discarded := make([]map[string]any, 0, len(changeIDs))
	failed := make([]map[string]any, 0)
	for _, changeID := range changeIDs {
		result, discardErr := m.discardWorkspaceFileChange(sessionID, changeID)
		if discardErr != nil {
			failed = append(failed, map[string]any{
				"change_id": changeID,
				"code":      services.ErrorCodeOf(discardErr),
				"message":   discardErr.Error(),
			})
			continue
		}
		discarded = append(discarded, result)
	}
	result := workspaceBatchResult(sessionID, "discard_group", len(changeIDs), discarded, failed)
	result["group_id"] = groupID
	return result, nil
}

func (m *agentManager) handleAgentWorkspaceDiscardGroup(sessionID, groupID, language string) ([]agentMessage, []agentToolCall, error) {
	groupID = agentCleanToken(groupID)
	if groupID == "" {
		return []agentMessage{m.message(language, "agent", agentText(
			language,
			"请提供要丢弃的 group_id，例如：丢弃 group_id: changeset_20260527_...。",
			"Please provide the group_id to discard, for example: discard group_id: changeset_20260527_...",
		), nil, nil)}, nil, nil
	}
	tool := runningAgentToolCall("workspace.files.discard", groupID)
	result, err := m.discardWorkspaceChangeGroup(sessionID, groupID)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "group_id": groupID}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceBatch(language, "discard_group", result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentWorkspaceDiscardAll(sessionID, language string) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("workspace.files.discard", "all")
	changeIDs, err := m.discardableWorkspaceChangeIDs(sessionID)
	if err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		tool.Result = map[string]any{"session_id": sessionID, "action": "discard_all"}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	discarded := make([]map[string]any, 0, len(changeIDs))
	failed := make([]map[string]any, 0)
	for _, changeID := range changeIDs {
		result, discardErr := m.discardWorkspaceFileChange(sessionID, changeID)
		if discardErr != nil {
			failed = append(failed, map[string]any{
				"change_id": changeID,
				"code":      services.ErrorCodeOf(discardErr),
				"message":   discardErr.Error(),
			})
			continue
		}
		discarded = append(discarded, result)
	}

	result := workspaceBatchResult(sessionID, "discard_all", len(changeIDs), discarded, failed)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = result
	message := m.message(language, "tool", agentFormatWorkspaceBatch(language, "discard_all", result), []agentToolCall{tool}, nil)
	return []agentMessage{message}, []agentToolCall{tool}, nil
}

func agentDiffPreview(path, oldText, newText string) string {
	oldLines := strings.Split(strings.TrimRight(oldText, "\n"), "\n")
	newLines := strings.Split(strings.TrimRight(newText, "\n"), "\n")
	lines := []string{"--- " + path, "+++ " + path}
	for _, line := range oldLines {
		if line == "" && len(oldLines) == 1 {
			continue
		}
		lines = append(lines, "-"+line)
	}
	for _, line := range newLines {
		if line == "" && len(newLines) == 1 {
			continue
		}
		lines = append(lines, "+"+line)
	}
	return agentLimitBlock(strings.Join(lines, "\n"), 1600)
}

func (m *agentManager) handleAgentScheduleCreate(sessionID, language, operator string, svc routerServices, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	if svc.schedules == nil {
		return nil, nil, services.NewError(services.CodeUnsupported, "scheduler not available", nil, nil)
	}
	tool := runningAgentToolCall("POST /api/v1/schedules", "from_agent")

	rawAction := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["action"]),
		stringFromAny(args["kind"]),
		stringFromAny(args["type"]),
	)))
	cron := strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["cron"]),
		stringFromAny(args["expression"]),
		stringFromAny(args["schedule"]),
	))
	if cron == "" {
		err := services.NewError(services.CodeBadRequest, "schedule.create requires cron expression", map[string]any{"action": "schedule.create"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	spec := scheduleSpec{
		Cron:      cron,
		Name:      strings.TrimSpace(stringFromAny(args["name"])),
		Operator:  firstNonEmpty(operator, "agent"),
		Language:  firstNonEmpty(stringFromAny(args["language"]), language),
		SessionID: strings.TrimSpace(firstNonEmpty(stringFromAny(args["session_id"]), sessionID)),
		Prompt:    strings.TrimSpace(stringFromAny(args["prompt"])),
		OpsLevel:  strings.TrimSpace(firstNonEmpty(stringFromAny(args["ops_level"]), stringFromAny(args["operation_level"]))),
		Target:    strings.TrimSpace(firstNonEmpty(stringFromAny(args["target"]), stringFromAny(args["url"]))),
		Scope:     strings.TrimSpace(stringFromAny(args["scope"])),
		Profile:   strings.TrimSpace(stringFromAny(args["profile"])),
	}
	if v, ok := args["enabled"].(bool); ok {
		spec.Enabled = v
	} else {
		spec.Enabled = true
	}

	switch rawAction {
	case "scan_web", "scan.web", "scan", "web_scan", "web.scan", "scanweb":
		spec.Action = scheduleActionAgentPrompt
		if spec.Prompt == "" {
			spec.Prompt = schedulePromptFromLegacyScanWeb(spec)
		}
	case "agent_prompt", "agent.prompt", "prompt", "chat", "ask", "agent", "":
		spec.Action = scheduleActionAgentPrompt
	case "ports.scan.safe", "port.scan", "port_scan", "ports_scan", "ports.scan":
		spec.Action = scheduleActionAgentPrompt
		if spec.Prompt == "" {
			target := strings.TrimSpace(firstNonEmpty(spec.Target, stringFromAny(args["host"]), stringFromAny(args["input"])))
			if target != "" {
				spec.Prompt = "扫描 " + target + " 开放端口"
				if portsSpec := agentPortSpecFromPlannerArgument(args["ports"]); portsSpec != "" {
					spec.Prompt += " ports=" + portsSpec
				}
			}
		}
	case "http.request", "web.search", "web.fetch", "workspace.command", "workspace.files.list", "workspace.files.tree", "workspace.files.read", "workspace.search", "reports.draft":
		spec.Action = scheduleActionAgentPrompt
	default:
		if spec.Prompt != "" {
			spec.Action = scheduleActionAgentPrompt
		} else {
			spec.Action = scheduleAction(rawAction)
		}
	}

	if spec.Action == scheduleActionAgentPrompt && spec.Prompt == "" {
		err := services.NewError(services.CodeBadRequest, "schedule.create requires prompt", map[string]any{"action": "schedule.create"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	if spec.Name == "" {
		spec.Name = agentText(language, "定时提示词", "scheduled prompt")
	}

	created, err := svc.schedules.Create(spec)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	next := ""
	if created.NextRunAt != nil {
		next = created.NextRunAt.Local().Format(time.RFC3339)
	}
	tool.Result = map[string]any{
		"id":          created.ID,
		"action":      string(created.Action),
		"cron":        created.Cron,
		"enabled":     created.Enabled,
		"ops_level":   created.OpsLevel,
		"next_run_at": next,
	}
	content := agentText(
		language,
		fmt.Sprintf("已创建定时任务 %s（%s，cron=%s）。下次触发：%s", created.Name, created.Action, created.Cron, firstNonEmpty(next, agentText(language, "未排定", "not scheduled"))),
		fmt.Sprintf("Created schedule %s (%s, cron=%s). Next fire: %s", created.Name, created.Action, created.Cron, firstNonEmpty(next, "not scheduled")),
	)
	links := []agentLink{{To: "/tasks", Label: agentText(language, "查看定时任务", "View schedules"), Kind: "schedules"}}
	return []agentMessage{m.message(language, "tool", content, []agentToolCall{tool}, links)}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentScheduleList(language string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	if svc.schedules == nil {
		return nil, nil, services.NewError(services.CodeUnsupported, "scheduler not available", nil, nil)
	}
	tool := runningAgentToolCall("GET /api/v1/schedules", "")
	items := svc.schedules.List()
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{"count": len(items)}

	if len(items) == 0 {
		content := agentText(language, "当前没有定时任务。", "No schedules configured.")
		return []agentMessage{m.message(language, "tool", content, []agentToolCall{tool}, []agentLink{{To: "/tasks", Label: agentText(language, "任务页", "Tasks"), Kind: "schedules"}})}, []agentToolCall{tool}, nil
	}
	limit := len(items)
	if limit > 5 {
		limit = 5
	}
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		s := items[i]
		next := "-"
		if s.NextRunAt != nil {
			next = s.NextRunAt.Local().Format(time.RFC3339)
		}
		flag := agentText(language, "启用", "on")
		if !s.Enabled {
			flag = agentText(language, "停用", "off")
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %s cron=%s next=%s (%s)", s.ID, s.Action, s.Name, s.Cron, next, flag))
	}
	more := ""
	if len(items) > limit {
		more = agentText(language, fmt.Sprintf(" 等共 %d 条。", len(items)), fmt.Sprintf(" of %d total.", len(items)))
	}
	content := agentText(language, "定时任务："+strings.Join(lines, "; ")+more, "Schedules: "+strings.Join(lines, "; ")+more)
	links := []agentLink{{To: "/tasks", Label: agentText(language, "管理定时任务", "Manage schedules"), Kind: "schedules"}}
	return []agentMessage{m.message(language, "tool", content, []agentToolCall{tool}, links)}, []agentToolCall{tool}, nil
}

func (m *agentManager) handleAgentScheduleDelete(language string, svc routerServices, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	if svc.schedules == nil {
		return nil, nil, services.NewError(services.CodeUnsupported, "scheduler not available", nil, nil)
	}
	id := strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["id"]),
		stringFromAny(args["schedule_id"]),
		stringFromAny(args["name"]),
	))
	tool := runningAgentToolCall("DELETE /api/v1/schedules/"+shortAgentID(id, 22), "")
	if id == "" {
		err := services.NewError(services.CodeBadRequest, "schedule.delete requires id", map[string]any{"action": "schedule.delete"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	err := svc.schedules.Delete(id)
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{"id": id, "deleted": true}
	content := agentText(language, "已删除定时任务 "+id+"。", "Deleted schedule "+id+".")
	links := []agentLink{{To: "/tasks", Label: agentText(language, "任务页", "Tasks"), Kind: "schedules"}}
	return []agentMessage{m.message(language, "tool", content, []agentToolCall{tool}, links)}, []agentToolCall{tool}, nil
}

func agentLoadWebSearchConfig(svc routerServices) (search.Config, bool, error) {
	if svc.providers == nil {
		return search.Config{}, false, nil
	}
	cfg, err := svc.providers.Load()
	if err != nil {
		return search.Config{}, false, err
	}
	if cfg.WebSearch == nil {
		return search.Config{}, false, nil
	}
	return search.Config{
		Provider:   cfg.WebSearch.Provider,
		APIKey:     cfg.WebSearch.APIKey,
		BaseURL:    cfg.WebSearch.BaseURL,
		MaxResults: cfg.WebSearch.MaxResults,
	}, true, nil
}

func (m *agentManager) handleAgentWebSearch(ctx context.Context, language string, svc routerServices, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	query := strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["query"]),
		stringFromAny(args["q"]),
		stringFromAny(args["input"]),
	))
	tool := runningAgentToolCall("web.search", query)
	if query == "" {
		err := services.NewError(services.CodeBadRequest, "web.search requires query", map[string]any{"action": "web.search"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	cfg, ok, loadErr := agentLoadWebSearchConfig(svc)
	if loadErr != nil {
		tool = finishAgentToolCall(tool, loadErr)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(loadErr), Message: loadErr.Error()}
		return []agentMessage{m.message(language, "tool", loadErr.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	if !ok {
		err := services.NewError(services.CodeBadRequest, "web.search not configured; open Settings → Web Search and set a provider + api key", map[string]any{"action": "web.search"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	provider, err := search.NewFromConfig(cfg)
	if err != nil {
		if errors.Is(err, search.ErrNotConfigured) {
			err = services.NewError(services.CodeBadRequest, "web.search not configured; open Settings → Web Search and set a provider + api key", map[string]any{"action": "web.search"}, err)
		}
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	if cfg.BaseURL != "" {
		if parsed, parseErr := url.Parse(cfg.BaseURL); parseErr == nil && parsed != nil {
			if guardErr := agentURLOutboundAllowed(parsed); guardErr != nil {
				tool = finishAgentToolCall(tool, guardErr)
				tool.Error = &daemonJobError{Code: services.ErrorCodeOf(guardErr), Message: guardErr.Error()}
				return []agentMessage{m.message(language, "tool", guardErr.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
			}
		}
	}
	max := intFromAny(args["max_results"])
	if max <= 0 {
		max = cfg.MaxResults
	}
	if max <= 0 {
		max = 5
	}
	if max > 10 {
		max = 10
	}
	reqCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	result, err := provider.Search(reqCtx, search.Query{
		Query:      query,
		MaxResults: max,
		Locale:     stringFromAny(args["locale"]),
	})
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	tool.Result = map[string]any{
		"provider": result.Provider,
		"query":    result.Query,
		"items":    result.Items,
	}
	return []agentMessage{m.message(language, "tool", agentFormatWebSearch(language, result), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
}

func agentFormatWebSearch(language string, result search.Result) string {
	if len(result.Items) == 0 {
		return agentText(language,
			"web.search 没有结果。",
			"web.search returned no results.",
		)
	}
	var b strings.Builder
	header := agentText(language,
		fmt.Sprintf("web.search via %s — %d 条结果：", result.Provider, len(result.Items)),
		fmt.Sprintf("web.search via %s — %d results:", result.Provider, len(result.Items)),
	)
	b.WriteString(header)
	b.WriteString("\n")
	for i, item := range result.Items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = item.URL
		}
		snippet := strings.TrimSpace(item.Snippet)
		if len(snippet) > 320 {
			snippet = snippet[:320] + "…"
		}
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, title, item.URL)
		if snippet != "" {
			fmt.Fprintf(&b, "   %s\n", snippet)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *agentManager) handleAgentWebFetch(ctx context.Context, language string, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	rawURL := strings.TrimSpace(firstNonEmpty(
		stringFromAny(args["url"]),
		stringFromAny(args["target"]),
		stringFromAny(args["input"]),
	))
	mode := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringFromAny(args["mode"]), "readable")))
	if mode != "readable" && mode != "raw" {
		mode = "readable"
	}
	maxChars := intFromAny(args["max_chars"])
	if maxChars <= 0 {
		maxChars = 8000
	}
	if maxChars > 20000 {
		maxChars = 20000
	}
	tool := runningAgentToolCall("web.fetch", rawURL)
	if rawURL == "" {
		err := services.NewError(services.CodeBadRequest, "web.fetch requires url", map[string]any{"action": "web.fetch"}, nil)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	rawURL = agentNormalizeURLInput(rawURL)
	parsed, perr := url.Parse(rawURL)
	if perr != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		err := services.NewError(services.CodeBadRequest, "web.fetch requires an http/https url", map[string]any{"url": rawURL}, perr)
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	if guardErr := agentURLOutboundAllowed(parsed); guardErr != nil {
		tool = finishAgentToolCall(tool, guardErr)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(guardErr), Message: guardErr.Error()}
		return []agentMessage{m.message(language, "tool", guardErr.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, parsed.String(), nil)
	if err == nil {
		req.Header.Set("User-Agent", "CyberAgent/agent-web-fetch")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain;q=0.9,*/*;q=0.8")
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			if req.URL != nil {
				if guardErr := agentURLOutboundAllowed(req.URL); guardErr != nil {
					return guardErr
				}
			}
			return nil
		},
	}
	var resp *http.Response
	if err == nil {
		resp, err = client.Do(req)
	}
	if err != nil {
		tool = finishAgentToolCall(tool, err)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(err), Message: err.Error()}
		return []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 1<<20+1)
	bodyBytes, readErr := io.ReadAll(limited)
	if readErr != nil {
		tool = finishAgentToolCall(tool, readErr)
		tool.Error = &daemonJobError{Code: services.ErrorCodeOf(readErr), Message: readErr.Error()}
		return []agentMessage{m.message(language, "tool", readErr.Error(), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
	}
	contentType := resp.Header.Get("Content-Type")
	body := string(bodyBytes)
	title := ""
	content := body
	if mode == "readable" && strings.Contains(strings.ToLower(contentType), "html") {
		title, content = htmlToReadable(body)
	}
	truncated := false
	if len(content) > maxChars {
		content = content[:maxChars]
		truncated = true
	}
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"url":           parsed.String(),
		"mode":          mode,
		"status":        resp.Status,
		"status_code":   resp.StatusCode,
		"content_type":  contentType,
		"title":         title,
		"content":       content,
		"content_chars": len(content),
		"truncated":     truncated,
	}
	return []agentMessage{m.message(language, "tool", agentFormatWebFetch(language, parsed.String(), title, content, contentType, truncated), []agentToolCall{tool}, nil)}, []agentToolCall{tool}, nil
}

func agentFormatWebFetch(language, target, title, content, contentType string, truncated bool) string {
	var b strings.Builder
	header := agentText(language,
		fmt.Sprintf("web.fetch %s (%s)", target, strings.TrimSpace(contentType)),
		fmt.Sprintf("web.fetch %s (%s)", target, strings.TrimSpace(contentType)),
	)
	b.WriteString(header)
	b.WriteString("\n")
	if title != "" {
		fmt.Fprintf(&b, "title: %s\n", title)
	}
	b.WriteString(content)
	if truncated {
		b.WriteString(agentText(language, "\n... (已截断)", "\n... (truncated)"))
	}
	return strings.TrimRight(b.String(), "\n")
}

var (
	htmlScriptRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	htmlStyleRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlCommentRe  = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlTitleRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlTagRe      = regexp.MustCompile(`(?s)<[^>]+>`)
	htmlBlockRe    = regexp.MustCompile(`(?i)</?(p|div|section|article|li|tr|h[1-6]|br|hr|blockquote|pre)[^>]*>`)
	htmlEntitySpRe = regexp.MustCompile(`&nbsp;`)
	whitespaceRe   = regexp.MustCompile(`[\t \x00-\x08\x0b\x0c]+`)
	multiNewlineRe = regexp.MustCompile(`\n{3,}`)
)

// htmlToReadable strips scripts/styles/comments and collapses tags into plain
// text, preserving paragraph boundaries. Returns (title, body).
func htmlToReadable(html string) (string, string) {
	cleaned := htmlScriptRe.ReplaceAllString(html, "")
	cleaned = htmlStyleRe.ReplaceAllString(cleaned, "")
	cleaned = htmlCommentRe.ReplaceAllString(cleaned, "")
	title := ""
	if m := htmlTitleRe.FindStringSubmatch(cleaned); len(m) == 2 {
		title = strings.TrimSpace(htmlDecodeBasic(htmlTagRe.ReplaceAllString(m[1], "")))
	}
	cleaned = htmlBlockRe.ReplaceAllString(cleaned, "\n")
	cleaned = htmlTagRe.ReplaceAllString(cleaned, "")
	cleaned = htmlEntitySpRe.ReplaceAllString(cleaned, " ")
	cleaned = htmlDecodeBasic(cleaned)
	cleaned = whitespaceRe.ReplaceAllString(cleaned, " ")
	lines := strings.Split(cleaned, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimSpace(line))
	}
	body := strings.Join(out, "\n")
	body = multiNewlineRe.ReplaceAllString(body, "\n\n")
	return title, strings.TrimSpace(body)
}

func htmlDecodeBasic(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
		"&apos;", "'",
	)
	return r.Replace(s)
}
