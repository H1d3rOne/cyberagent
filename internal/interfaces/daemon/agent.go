package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

func (m *agentManager) loadSessions() error {
	if strings.TrimSpace(m.storeDir) == "" {
		return nil
	}
	entries, err := os.ReadDir(m.storeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(m.storeDir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var session agentSession
		if err := json.Unmarshal(data, &session); err != nil || session.SessionID == "" {
			continue
		}
		session.normalize()
		changed := normalizeCompletedOneShotAgentSession(&session)
		if normalizeLegacyDoctorMessages(&session) {
			changed = true
		}
		cloned := session
		m.sessions[session.SessionID] = &cloned
		if changed {
			_ = m.persistSessionLocked(&cloned)
		}
	}
	return nil
}

func (m *agentManager) loadSessionLocked(sessionID string) (*agentSession, error) {
	sessionID, err := normalizeAgentSessionID(sessionID)
	if err != nil {
		return nil, err
	}
	path := m.sessionPath(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, services.NewError(services.CodeNotFound, "agent session not found", map[string]any{"session_id": sessionID}, nil)
		}
		return nil, services.NewError(services.CodeInternal, "read agent session", map[string]any{"session_id": sessionID}, err)
	}
	var session agentSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, services.NewError(services.CodeInternal, "decode agent session", map[string]any{"session_id": sessionID}, err)
	}
	if session.SessionID == "" {
		session.SessionID = sessionID
	}
	session.normalize()
	changed := normalizeCompletedOneShotAgentSession(&session)
	if normalizeLegacyDoctorMessages(&session) {
		changed = true
	}
	m.sessions[session.SessionID] = &session
	if changed {
		_ = m.persistSessionLocked(&session)
	}
	return &session, nil
}

func (m *agentManager) syncTerminalActiveRunsLocked(ctx context.Context) {
	for _, session := range m.sessions {
		if m.syncTerminalActiveRunLocked(ctx, session) {
			_ = m.persistSessionLocked(session)
		}
	}
}

func (m *agentManager) syncTerminalActiveRunLocked(ctx context.Context, session *agentSession) bool {
	if session == nil || session.ActiveTask == nil {
		return false
	}
	runID := strings.TrimSpace(session.ActiveTask.ActiveRunID)
	if runID == "" {
		return false
	}
	detail, err := m.runs.GetRun(ctx, runID)
	if err != nil {
		return false
	}
	changed := m.refreshActiveRunContextSnapshotLocked(session, detail)
	if m.refreshApprovalGateForSessionLocked(ctx, session) {
		return true
	}
	status := detail.Summary.Status
	if status == "" && detail.Job != nil {
		status = detail.Job.Status
	}
	if !agentRunTerminalStatus(status) {
		return changed
	}
	if session.Status == "paused" || session.Status == "cancelled" {
		return changed
	}
	switch status {
	case contracts.StatusSucceeded, contracts.StatusPartiallySucceeded:
		return normalizeCompletedActiveRunSession(session, status, detail) || changed
	case contracts.StatusFailed, contracts.StatusCancelled:
		return normalizeStoppedActiveRunSession(session, status) || changed
	default:
		return changed
	}
}

func (m *agentManager) refreshActiveRunContextSnapshotLocked(session *agentSession, detail services.RunDetail) bool {
	if session == nil || session.ActiveTask == nil || strings.TrimSpace(detail.RunID) == "" {
		return false
	}
	runID := detail.RunID
	reportSnapshot := m.activeRunReportSnapshot(context.Background(), runID, detail)
	summary := agentRunContextSummary(session.Language, detail, reportSnapshot)
	changed := false
	changed = m.addContextRefLocked(session, agentContextRef{
		ID:      "run:" + runID,
		Kind:    "run",
		RefID:   runID,
		Label:   "run " + shortAgentID(runID, 18),
		To:      "/scan/" + runID,
		Summary: summary,
	}) || changed
	if detail.Summary.Report.JSON {
		changed = m.addContextRefLocked(session, agentContextRef{
			ID:      "report_json:" + runID,
			Kind:    "report_json",
			RefID:   runID,
			Label:   "report " + shortAgentID(runID, 18),
			To:      "/scan/" + runID,
			Summary: agentReportContextSummary(session.Language, detail, reportSnapshot),
		}) || changed
	}
	if detail.Summary.Counts.Artifacts > 0 {
		changed = m.addContextRefLocked(session, agentContextRef{
			ID:      "artifact_index:" + runID,
			Kind:    "artifact_index",
			RefID:   runID,
			Label:   "artifacts " + shortAgentID(runID, 18),
			To:      "/scan/" + runID,
			Summary: agentArtifactContextSummary(session.Language, detail, reportSnapshot),
		}) || changed
	}
	if detail.Summary.Counts.Findings > 0 {
		changed = m.addContextRefLocked(session, agentContextRef{
			ID:      "finding:" + runID,
			Kind:    "finding",
			RefID:   runID,
			Label:   agentText(session.Language, fmt.Sprintf("%d 个发现", detail.Summary.Counts.Findings), fmt.Sprintf("%d findings", detail.Summary.Counts.Findings)),
			To:      "/scan/" + runID,
			Summary: agentFindingContextSummary(session.Language, detail, reportSnapshot),
		}) || changed
	}
	if detail.Summary.Counts.Approvals > 0 {
		pending := detail.Summary.Counts.PendingApprovals
		changed = m.addContextRefLocked(session, agentContextRef{
			ID:      "approval:" + runID,
			Kind:    "approval",
			RefID:   runID,
			Label:   agentText(session.Language, fmt.Sprintf("%d/%d 待审批", pending, detail.Summary.Counts.Approvals), fmt.Sprintf("%d/%d pending approvals", pending, detail.Summary.Counts.Approvals)),
			To:      "/scan/" + runID,
			Summary: agentText(session.Language, "运行中的人工审批队列", "Human approval queue for this run"),
		}) || changed
	}
	if summary != "" && session.ActiveTask.Summary != summary && shouldRefreshActiveRunSummary(session.ActiveTask.Summary) {
		session.ActiveTask.Summary = summary
		session.ActiveTask.UpdatedAt = time.Now().UTC()
		session.UpdatedAt = session.ActiveTask.UpdatedAt
		changed = true
	}
	return changed
}

func (m *agentManager) activeRunReportSnapshot(ctx context.Context, runID string, detail services.RunDetail) agentRunReportSnapshot {
	snapshot := agentRunReportSnapshot{}
	if detail.Summary.Report.JSON {
		if report, err := m.runs.GetReport(ctx, runID, "json"); err == nil {
			snapshot = parseAgentRunReportSnapshot(report.Bytes)
		}
	}
	if len(snapshot.Artifacts) == 0 && detail.Summary.Counts.Artifacts > 0 {
		if index, err := m.runs.GetArtifactIndex(ctx, runID); err == nil {
			for _, artifact := range index.Artifacts {
				snapshot.Artifacts = append(snapshot.Artifacts, agentRunArtifactSnapshot{
					ID:       artifact.ID,
					Type:     string(artifact.Type),
					Path:     artifact.Path,
					MIMEType: artifact.MIMEType,
				})
				if len(snapshot.Artifacts) >= 3 {
					break
				}
			}
		}
	}
	return snapshot
}

func parseAgentRunReportSnapshot(data []byte) agentRunReportSnapshot {
	var raw struct {
		Findings  []agentRunFindingSnapshot  `json:"findings"`
		Evidence  []agentRunEvidenceSnapshot `json:"evidence"`
		Artifacts []agentRunArtifactSnapshot `json:"artifacts"`
	}
	if len(data) == 0 || json.Unmarshal(data, &raw) != nil {
		return agentRunReportSnapshot{}
	}
	snapshot := agentRunReportSnapshot{}
	for _, finding := range raw.Findings {
		finding.Title = trimSummary(finding.Title, 120)
		finding.Category = trimSummary(finding.Category, 80)
		finding.Impact = trimSummary(finding.Impact, 120)
		finding.Remediation = trimSummary(finding.Remediation, 140)
		if len(finding.EvidenceIDs) > 3 {
			finding.EvidenceIDs = append([]string(nil), finding.EvidenceIDs[:3]...)
		}
		snapshot.Findings = append(snapshot.Findings, finding)
		if len(snapshot.Findings) >= 3 {
			break
		}
	}
	for _, evidence := range raw.Evidence {
		evidence.Title = trimSummary(evidence.Title, 100)
		evidence.Summary = trimSummary(evidence.Summary, 160)
		snapshot.Evidence = append(snapshot.Evidence, evidence)
		if len(snapshot.Evidence) >= 3 {
			break
		}
	}
	for _, artifact := range raw.Artifacts {
		artifact.Path = trimSummary(artifact.Path, 140)
		artifact.MIMEType = trimSummary(artifact.MIMEType, 80)
		snapshot.Artifacts = append(snapshot.Artifacts, artifact)
		if len(snapshot.Artifacts) >= 3 {
			break
		}
	}
	return snapshot
}

func agentReportDraftPath(runID, outputPath string) string {
	outputPath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(outputPath)))
	if outputPath == "" || outputPath == "." || strings.HasPrefix(outputPath, "../") || filepath.IsAbs(outputPath) {
		return filepath.ToSlash(filepath.Join("reports", agentCleanToken(runID)+"-draft.md"))
	}
	if !strings.HasSuffix(strings.ToLower(outputPath), ".md") {
		outputPath += ".md"
	}
	return outputPath
}

func agentBuildReportDraftMarkdown(language string, detail services.RunDetail, snapshot agentRunReportSnapshot, generatedAt time.Time) string {
	target := firstNonEmpty(detail.Summary.Target, detail.Summary.Scope)
	status := string(detail.Summary.Status)
	if status == "" && detail.Job != nil {
		status = string(detail.Job.Status)
	}
	var b strings.Builder
	zh := normalizeAgentLanguage(language) == "zh"
	if zh {
		fmt.Fprintf(&b, "# CyberAgent 报告草稿\n\n")
		fmt.Fprintf(&b, "> 草稿由 Agent 基于运行时报告和上下文生成，需要人工复核后再对外输出。\n\n")
		fmt.Fprintf(&b, "## 概览\n\n")
	} else {
		fmt.Fprintf(&b, "# CyberAgent Report Draft\n\n")
		fmt.Fprintf(&b, "> Draft generated from runtime report/context. Review manually before external use.\n\n")
		fmt.Fprintf(&b, "## Overview\n\n")
	}
	fmt.Fprintf(&b, "- Run ID: `%s`\n", detail.RunID)
	fmt.Fprintf(&b, "- Target: `%s`\n", emptyDash(target))
	fmt.Fprintf(&b, "- Scope: `%s`\n", emptyDash(detail.Summary.Scope))
	fmt.Fprintf(&b, "- Profile: `%s`\n", emptyDash(detail.Summary.Profile))
	fmt.Fprintf(&b, "- Status: `%s`\n", emptyDash(status))
	fmt.Fprintf(&b, "- Findings: %d\n", len(snapshot.Findings))
	fmt.Fprintf(&b, "- Evidence: %d\n", len(snapshot.Evidence))
	fmt.Fprintf(&b, "- Artifacts: %d\n", len(snapshot.Artifacts))
	fmt.Fprintf(&b, "- Generated: %s\n\n", generatedAt.Format(time.RFC3339))
	if zh {
		fmt.Fprintf(&b, "## 执行摘要\n\n")
		if len(snapshot.Findings) == 0 {
			fmt.Fprintf(&b, "本次运行暂未在 report.json 中发现 finding。仍建议结合事件、artifact 和授权范围进行人工复核。\n\n")
		} else {
			fmt.Fprintf(&b, "本次运行发现 %d 个 finding。优先按严重度、可复现证据和业务暴露面进行确认，并在修复后对同一 scope 执行复测。\n\n", len(snapshot.Findings))
		}
	} else {
		fmt.Fprintf(&b, "## Executive Summary\n\n")
		if len(snapshot.Findings) == 0 {
			fmt.Fprintf(&b, "No findings were present in report.json. Review events, artifacts, and scope before closing the task.\n\n")
		} else {
			fmt.Fprintf(&b, "The run produced %d finding(s). Prioritize by severity, reproducible evidence, and exposed business surface; retest the same scope after remediation.\n\n", len(snapshot.Findings))
		}
	}
	fmt.Fprintf(&b, "## Findings\n\n")
	agentAppendReportDraftFindings(&b, snapshot, language)
	if zh {
		fmt.Fprintf(&b, "\n## Evidence 摘要\n\n")
	} else {
		fmt.Fprintf(&b, "\n## Evidence Summary\n\n")
	}
	agentAppendReportDraftEvidence(&b, snapshot, language)
	if zh {
		fmt.Fprintf(&b, "\n## Artifacts 摘要\n\n")
	} else {
		fmt.Fprintf(&b, "\n## Artifact Summary\n\n")
	}
	agentAppendReportDraftArtifacts(&b, snapshot, language)
	if zh {
		fmt.Fprintf(&b, "\n## 修复与复测建议\n\n")
		fmt.Fprintf(&b, "1. 逐条核对 finding 的 evidence_ids 与原始 artifact，确认不是代理、CDN、开发服务器或测试夹具导致的误报。\n")
		fmt.Fprintf(&b, "2. 按 finding 的 remediation 或业务配置修复。\n")
		fmt.Fprintf(&b, "3. 用最小复现命令验证修复，再让 Agent 对相同 scope 重跑 safe profile。\n")
		fmt.Fprintf(&b, "4. 将复测结果追加到本草稿的 Evidence / Retest Notes 中。\n")
	} else {
		fmt.Fprintf(&b, "\n## Remediation and Retest\n\n")
		fmt.Fprintf(&b, "1. Correlate each finding's evidence_ids with raw artifacts and rule out proxy/CDN/dev-server/test-fixture false positives.\n")
		fmt.Fprintf(&b, "2. Apply each finding's remediation or the equivalent business configuration change.\n")
		fmt.Fprintf(&b, "3. Verify with the minimal reproduction command, then rerun the safe profile for the same scope.\n")
		fmt.Fprintf(&b, "4. Append retest results to this draft's Evidence / Retest Notes.\n")
	}
	return b.String()
}

func agentAppendReportDraftFindings(b *strings.Builder, snapshot agentRunReportSnapshot, language string) {
	if len(snapshot.Findings) == 0 {
		fmt.Fprintf(b, "%s\n", agentText(language, "暂无 finding。", "No findings."))
		return
	}
	for idx, finding := range snapshot.Findings {
		title := firstNonEmpty(finding.Title, finding.ID, fmt.Sprintf("Finding %d", idx+1))
		fmt.Fprintf(b, "### %d. %s\n\n", idx+1, title)
		fmt.Fprintf(b, "- Severity: `%s`\n", emptyDash(string(finding.Severity)))
		fmt.Fprintf(b, "- Category: `%s`\n", emptyDash(finding.Category))
		if len(finding.EvidenceIDs) > 0 {
			fmt.Fprintf(b, "- Evidence IDs: `%s`\n", strings.Join(finding.EvidenceIDs, "`, `"))
		}
		if finding.Impact != "" {
			fmt.Fprintf(b, "- Impact: %s\n", finding.Impact)
		}
		if finding.Remediation != "" {
			fmt.Fprintf(b, "- Remediation: %s\n", finding.Remediation)
		}
		fmt.Fprintf(b, "\n")
	}
}

func agentAppendReportDraftEvidence(b *strings.Builder, snapshot agentRunReportSnapshot, language string) {
	if len(snapshot.Evidence) == 0 {
		fmt.Fprintf(b, "%s\n", agentText(language, "暂无 evidence 摘要。", "No evidence summary."))
		return
	}
	for _, evidence := range snapshot.Evidence {
		fmt.Fprintf(b, "- `%s` · %s · %s", emptyDash(evidence.ID), emptyDash(string(evidence.Type)), emptyDash(evidence.Title))
		if evidence.Summary != "" {
			fmt.Fprintf(b, " — %s", evidence.Summary)
		}
		if evidence.ArtifactID != "" {
			fmt.Fprintf(b, " (artifact `%s`)", evidence.ArtifactID)
		}
		fmt.Fprintf(b, "\n")
	}
}

func agentAppendReportDraftArtifacts(b *strings.Builder, snapshot agentRunReportSnapshot, language string) {
	if len(snapshot.Artifacts) == 0 {
		fmt.Fprintf(b, "%s\n", agentText(language, "暂无 artifact 摘要。", "No artifact summary."))
		return
	}
	for _, artifact := range snapshot.Artifacts {
		fmt.Fprintf(b, "- `%s` · %s · `%s`", emptyDash(artifact.ID), emptyDash(string(artifact.Type)), emptyDash(artifact.Path))
		if artifact.MIMEType != "" {
			fmt.Fprintf(b, " · %s", artifact.MIMEType)
		}
		fmt.Fprintf(b, "\n")
	}
}

func shouldRefreshActiveRunSummary(current string) bool {
	current = strings.TrimSpace(current)
	if current == "" {
		return true
	}
	lower := strings.ToLower(current)
	return strings.Contains(current, "正在跟踪运行") ||
		strings.Contains(lower, "tracking context for run") ||
		strings.Contains(current, "跟踪运行事件、证据和审批") ||
		strings.Contains(lower, "track run events")
}

func agentRunContextSummary(language string, detail services.RunDetail, snapshot agentRunReportSnapshot) string {
	status := detail.Summary.Status
	if status == "" && detail.Job != nil {
		status = detail.Job.Status
	}
	target := detail.Summary.Target
	if target == "" && detail.Job != nil {
		target = detail.Job.TargetID
	}
	if target == "" {
		target = "-"
	}
	counts := detail.Summary.Counts
	leadFinding := agentLeadFindingText(snapshot)
	leadEvidence := agentLeadEvidenceText(snapshot)
	extraZH := ""
	extraEN := ""
	if leadFinding != "" {
		extraZH += " 关键发现：" + leadFinding + "。"
		extraEN += " Key finding: " + leadFinding + "."
	}
	if leadEvidence != "" {
		extraZH += " 关键证据：" + leadEvidence + "。"
		extraEN += " Key evidence: " + leadEvidence + "."
	}
	return trimSummary(agentText(language,
		fmt.Sprintf("运行 %s · %s · target=%s · findings=%d · artifacts=%d · pending_approvals=%d。%s", shortAgentID(detail.RunID, 18), emptyDash(string(status)), target, counts.Findings, counts.Artifacts, counts.PendingApprovals, extraZH),
		fmt.Sprintf("Run %s · %s · target=%s · findings=%d · artifacts=%d · pending_approvals=%d.%s", shortAgentID(detail.RunID, 18), emptyDash(string(status)), target, counts.Findings, counts.Artifacts, counts.PendingApprovals, extraEN),
	), 360)
}

func agentReportContextSummary(language string, detail services.RunDetail, snapshot agentRunReportSnapshot) string {
	leadFinding := agentLeadFindingText(snapshot)
	leadEvidence := agentLeadEvidenceText(snapshot)
	if leadFinding != "" || leadEvidence != "" {
		return trimSummary(agentText(language,
			fmt.Sprintf("%d 个发现，%d 条证据。%s%s", detail.Summary.Counts.Findings, len(snapshot.Evidence), prefixIfNotEmpty("关键发现：", leadFinding), prefixIfNotEmpty(" 关键证据：", leadEvidence)),
			fmt.Sprintf("%d findings, %d evidence items. %s%s", detail.Summary.Counts.Findings, len(snapshot.Evidence), prefixIfNotEmpty("Key finding: ", leadFinding), prefixIfNotEmpty(" Key evidence: ", leadEvidence)),
		), 220)
	}
	return agentText(language, fmt.Sprintf("%d findings · %d evidence artifacts", detail.Summary.Counts.Findings, detail.Summary.Counts.Artifacts), fmt.Sprintf("%d findings · %d evidence artifacts", detail.Summary.Counts.Findings, detail.Summary.Counts.Artifacts))
}

func agentFindingContextSummary(language string, detail services.RunDetail, snapshot agentRunReportSnapshot) string {
	leadFinding := agentLeadFindingText(snapshot)
	if leadFinding != "" {
		return trimSummary(agentText(language,
			"关键发现："+leadFinding+"。建议按影响、证据、误报、修复、复测处理。",
			"Key finding: "+leadFinding+". Triage as impact, evidence, false positive, remediation, retest.",
		), 220)
	}
	_ = detail
	return agentText(language, "来自当前运行报告，建议按影响、证据、误报、修复、复测处理。", "From the active run report; triage as impact, evidence, false positive, remediation, retest.")
}

func agentArtifactContextSummary(language string, detail services.RunDetail, snapshot agentRunReportSnapshot) string {
	leadArtifact := agentLeadArtifactText(snapshot)
	leadEvidence := agentLeadEvidenceText(snapshot)
	if leadArtifact != "" || leadEvidence != "" {
		return trimSummary(agentText(language,
			fmt.Sprintf("%d 个 artifact。%s%s", detail.Summary.Counts.Artifacts, prefixIfNotEmpty("关键 artifact：", leadArtifact), prefixIfNotEmpty(" 关联证据：", leadEvidence)),
			fmt.Sprintf("%d artifacts. %s%s", detail.Summary.Counts.Artifacts, prefixIfNotEmpty("Key artifact: ", leadArtifact), prefixIfNotEmpty(" Related evidence: ", leadEvidence)),
		), 220)
	}
	return agentText(language, fmt.Sprintf("%d 个证据 artifact", detail.Summary.Counts.Artifacts), fmt.Sprintf("%d artifact(s)", detail.Summary.Counts.Artifacts))
}

func agentLeadFindingText(snapshot agentRunReportSnapshot) string {
	if len(snapshot.Findings) == 0 {
		return ""
	}
	finding := snapshot.Findings[0]
	parts := []string{}
	if finding.Severity != "" {
		parts = append(parts, strings.ToUpper(finding.Severity))
	}
	if finding.Title != "" {
		parts = append(parts, finding.Title)
	}
	if finding.Category != "" {
		parts = append(parts, "category="+finding.Category)
	}
	if finding.Remediation != "" {
		parts = append(parts, "fix="+finding.Remediation)
	}
	return trimSummary(strings.Join(parts, " · "), 180)
}

func agentLeadEvidenceText(snapshot agentRunReportSnapshot) string {
	if len(snapshot.Evidence) == 0 {
		return ""
	}
	evidence := snapshot.Evidence[0]
	parts := []string{}
	if evidence.ID != "" {
		parts = append(parts, evidence.ID)
	}
	if evidence.Type != "" {
		parts = append(parts, string(evidence.Type))
	}
	if evidence.Title != "" {
		parts = append(parts, evidence.Title)
	}
	if evidence.Summary != "" {
		parts = append(parts, evidence.Summary)
	}
	return trimSummary(strings.Join(parts, " · "), 180)
}

func agentLeadArtifactText(snapshot agentRunReportSnapshot) string {
	if len(snapshot.Artifacts) == 0 {
		return ""
	}
	artifact := snapshot.Artifacts[0]
	parts := []string{}
	if artifact.ID != "" {
		parts = append(parts, artifact.ID)
	}
	if artifact.Type != "" {
		parts = append(parts, artifact.Type)
	}
	if artifact.Path != "" {
		parts = append(parts, artifact.Path)
	}
	return trimSummary(strings.Join(parts, " · "), 160)
}

func prefixIfNotEmpty(prefix, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + value
}

func agentRunTerminalStatus(status contracts.Status) bool {
	switch status {
	case contracts.StatusSucceeded, contracts.StatusPartiallySucceeded, contracts.StatusFailed, contracts.StatusCancelled:
		return true
	default:
		return false
	}
}

func normalizeCompletedActiveRunSession(session *agentSession, status contracts.Status, detail services.RunDetail) bool {
	if session == nil || session.ActiveTask == nil {
		return false
	}
	runID := strings.TrimSpace(session.ActiveTask.ActiveRunID)
	if sessionHasRunEvidenceActionTodos(session) {
		now := time.Now().UTC()
		session.Status = "running"
		session.ActiveTask.Status = "running"
		session.ActiveTask.Summary = trimSummary(agentText(session.Language,
			"运行 "+session.ActiveTask.ActiveRunID+" 已结束（"+string(status)+"），报告发现已转入误报检查、修复和复测行动计划。",
			"Run "+session.ActiveTask.ActiveRunID+" has finished ("+string(status)+"); findings are now in false-positive check, remediation and retest action planning.",
		), 360)
		session.ActiveTask.UpdatedAt = now
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          "active run reached terminal status and follow-up evidence action todos remain",
			SuggestedPrompt: agentSuggestedEvidenceActionPrompt(session.Language, session.ActiveTask.ActiveRunID),
			AutoContinue:    true,
		}
		session.UpdatedAt = now
		return true
	}
	needsFix := session.Status == "running" || session.ActiveTask.Status == "running"
	if session.NextAction != nil && session.NextAction.Type == "continue" {
		needsFix = true
	}
	for _, todo := range session.Todos {
		if todo.Status == "running" || todo.Status == "pending" {
			needsFix = true
			break
		}
	}
	if !needsFix {
		return false
	}
	now := time.Now().UTC()
	for idx := range session.Todos {
		if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
			markAgentTodoByIndex(session.Todos, idx, "done", now, "", "")
		}
	}
	session.Status = "idle"
	session.ActiveTask.Status = "idle"
	session.ActiveTask.Summary = trimSummary(agentText(session.Language,
		"运行 "+session.ActiveTask.ActiveRunID+" 已结束（"+string(status)+"），等待用户要求总结证据或继续分析。",
		"Run "+session.ActiveTask.ActiveRunID+" has finished ("+string(status)+"); waiting for the operator to request evidence summary or follow-up analysis.",
	), 360)
	session.ActiveTask.UpdatedAt = now
	session.NextAction = &agentNextAction{
		Type:            "wait_user",
		Reason:          "active run reached terminal status: " + string(status),
		SuggestedPrompt: agentSuggestedRunSummaryPrompt(session.Language, session.ActiveTask.ActiveRunID),
	}
	if runID != "" && !sessionHasTerminalRunSummaryMessage(session, runID) {
		session.Messages = append(session.Messages, agentSyntheticRunSummaryMessage(session.Language, runID, status, detail))
	}
	return true
}

func sessionHasTerminalRunSummaryMessage(session *agentSession, runID string) bool {
	if session == nil || strings.TrimSpace(runID) == "" {
		return false
	}
	for _, message := range session.Messages {
		if message.Role == "agent" && message.Phase == "final_answer" && strings.Contains(message.Content, runID) {
			return true
		}
	}
	return false
}

func agentSyntheticRunSummaryMessage(language, runID string, status contracts.Status, detail services.RunDetail) agentMessage {
	now := time.Now().UTC()
	counts := detail.Summary.Counts
	target := firstNonEmpty(detail.Summary.Target, detail.Summary.Scope)
	if target == "" && detail.Job != nil {
		target = detail.Job.TargetID
	}
	reportText := agentText(language, "报告尚未生成", "report pending")
	if detail.Summary.Report.JSON {
		reportText = agentText(language, "报告已生成", "report ready")
	}
	content := agentText(language,
		fmt.Sprintf("任务已完成：运行 `%s` 状态为 `%s`。\n\n- 目标：%s\n- 事件：%d\n- 证据/Artifact：%d\n- 发现：%d\n- 审批：%d（待处理 %d）\n- 报告：%s\n\n下一步建议：发送 `%s`，我会读取报告/事件并给出证据链、风险摘要和复测建议。", runID, status, emptyDash(target), counts.Events, counts.Artifacts, counts.Findings, counts.Approvals, counts.PendingApprovals, reportText, agentSuggestedRunSummaryPrompt(language, runID)),
		fmt.Sprintf("Task completed: run `%s` status is `%s`.\n\n- Target: %s\n- Events: %d\n- Artifacts: %d\n- Findings: %d\n- Approvals: %d (pending %d)\n- Report: %s\n\nSuggested next step: send `%s`; I will read the report/events and summarize evidence chains, risk, and retest guidance.", runID, status, emptyDash(target), counts.Events, counts.Artifacts, counts.Findings, counts.Approvals, counts.PendingApprovals, reportText, agentSuggestedRunSummaryPrompt(language, runID)),
	)
	return agentMessage{
		ID:        strings.Replace(runs.NewJobID(now), "job_", "msg_", 1),
		Role:      "agent",
		Content:   content,
		Timestamp: now,
		Phase:     "final_answer",
		Links:     []agentLink{{To: "/scan/" + runID, Label: agentText(language, "打开运行详情", "Open run detail"), Kind: "run"}},
	}
}

func normalizeStoppedActiveRunSession(session *agentSession, status contracts.Status) bool {
	if session == nil || session.ActiveTask == nil {
		return false
	}
	if session.Status == "blocked" && session.ActiveTask.Status == "blocked" && session.NextAction != nil && session.NextAction.Type == "blocked" {
		return false
	}
	now := time.Now().UTC()
	reason := "active run stopped with terminal status: " + string(status)
	for idx := range session.Todos {
		if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
			markAgentTodoByIndex(session.Todos, idx, "blocked", now, "", reason)
		}
	}
	session.Status = "blocked"
	session.ActiveTask.Status = "blocked"
	session.ActiveTask.Summary = trimSummary(agentText(session.Language,
		"运行 "+session.ActiveTask.ActiveRunID+" 已停止（"+string(status)+"），需要分析失败/取消原因后再继续。",
		"Run "+session.ActiveTask.ActiveRunID+" stopped ("+string(status)+"); analyze the failure/cancellation before continuing.",
	), 360)
	session.ActiveTask.UpdatedAt = now
	session.NextAction = &agentNextAction{
		Type:            "blocked",
		Reason:          reason,
		SuggestedPrompt: agentSuggestedStoppedRunPrompt(session.Language, session.ActiveTask.ActiveRunID),
	}
	return true
}

func (m *agentManager) persistSessionLocked(session *agentSession) error {
	if session == nil || strings.TrimSpace(session.SessionID) == "" {
		return nil
	}
	session.normalize()
	if err := os.MkdirAll(m.storeDir, 0o755); err != nil {
		return services.NewError(services.CodeInternal, "create agent session store", map[string]any{"path": m.storeDir}, err)
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return services.NewError(services.CodeInternal, "encode agent session", map[string]any{"session_id": session.SessionID}, err)
	}
	path := m.sessionPath(session.SessionID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return services.NewError(services.CodeInternal, "write agent session", map[string]any{"session_id": session.SessionID}, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return services.NewError(services.CodeInternal, "commit agent session", map[string]any{"session_id": session.SessionID}, err)
	}
	return nil
}

func (m *agentManager) sessionPath(sessionID string) string {
	return filepath.Join(m.storeDir, sessionID+".json")
}

func (s *agentSession) normalize() {
	if s.Status == "" {
		s.Status = "idle"
	}
	s.OperationLevel = normalizeAgentOperationLevel(s.OperationLevel)
	if s.Messages == nil {
		s.Messages = []agentMessage{}
	}
	if s.Todos == nil {
		s.Todos = []agentTodo{}
	}
	if s.ToolCalls == nil {
		s.ToolCalls = []agentToolCall{}
	}
	if s.ContextRefs == nil {
		s.ContextRefs = []agentContextRef{}
	}
	if s.PendingChanges == nil {
		s.PendingChanges = []agentWorkspaceChange{}
	}
	if s.NextAction == nil {
		s.NextAction = &agentNextAction{Type: "wait_user"}
	}
}

func normalizeCompletedOneShotAgentSession(session *agentSession) bool {
	if session == nil || len(session.Todos) == 0 || len(session.ToolCalls) == 0 {
		return false
	}
	if session.PendingAuthorization != nil && strings.EqualFold(session.PendingAuthorization.Status, "pending") {
		return false
	}
	if session.ActiveTask != nil && strings.TrimSpace(session.ActiveTask.ActiveRunID) != "" {
		return false
	}
	if sessionHasScanToolCall(session) {
		return false
	}
	last := lastSuccessfulAgentToolCall(session)
	if last == nil || !agentToolCompletesLegacyOneShotTask(strings.ToLower(last.Name)) {
		return false
	}
	needsFix := session.Status == "running" || (session.ActiveTask != nil && session.ActiveTask.Status == "running")
	if session.NextAction != nil && session.NextAction.Type == "continue" {
		needsFix = true
	}
	for _, todo := range session.Todos {
		if todo.Status == "running" || todo.Status == "pending" {
			needsFix = true
			break
		}
	}
	if !needsFix {
		return false
	}
	now := time.Now().UTC()
	for idx := range session.Todos {
		if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
			markAgentTodoByIndex(session.Todos, idx, "done", now, last.ID, "")
		}
	}
	session.Status = "idle"
	if session.ActiveTask != nil {
		session.ActiveTask.Status = "done"
		session.ActiveTask.UpdatedAt = now
	}
	session.NextAction = &agentNextAction{Type: "finished", Reason: "task steps completed"}
	return true
}

func normalizeLegacyDoctorMessages(session *agentSession) bool {
	if session == nil {
		return false
	}
	changed := false
	for idx := range session.Messages {
		if !agentMessageHasSucceededDoctorTool(session.Messages[idx]) {
			continue
		}
		normalized, ok := normalizeLegacyDoctorMessageContent(session.Messages[idx].Content, session.Language)
		if !ok {
			continue
		}
		session.Messages[idx].Content = normalized
		changed = true
	}
	return changed
}

func agentMessageHasSucceededDoctorTool(message agentMessage) bool {
	for _, call := range message.ToolCalls {
		if call.Status == contracts.StatusSucceeded && strings.Contains(strings.ToLower(call.Name), "doctor") {
			return true
		}
	}
	return false
}

func normalizeLegacyDoctorMessageContent(content, language string) (string, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" || strings.Contains(trimmed, "必需检查") || strings.Contains(trimmed, "Required checks") {
		return "", false
	}
	if !strings.Contains(trimmed, "体检") || !strings.Contains(trimmed, "通过") || !strings.Contains(trimmed, "异常") {
		return "", false
	}
	version := "v0.1.0"
	if match := regexp.MustCompile(`版本\s*([^。,\s]+)`).FindStringSubmatch(trimmed); len(match) == 2 {
		version = match[1]
	}
	checks := 0
	if match := regexp.MustCompile(`检查项\s*(\d+)`).FindStringSubmatch(trimmed); len(match) == 2 {
		fmt.Sscanf(match[1], "%d", &checks)
	}
	optionalMissing := 0
	if match := regexp.MustCompile(`异常\s*(\d+)`).FindStringSubmatch(trimmed); len(match) == 2 {
		fmt.Sscanf(match[1], "%d", &optionalMissing)
	}
	if optionalMissing <= 0 {
		return "", false
	}
	if checks <= 0 {
		checks = optionalMissing
	}
	return agentText(language,
		fmt.Sprintf("Daemon 在线，版本 %s。必需检查通过，检查项 %d；历史体检中的 %d 个可选工具缺失已归一化为可选项（不影响内置 safe 能力）。", version, checks, optionalMissing),
		fmt.Sprintf("Daemon online, version %s. Required checks passed, %d checks total; %d optional missing tool(s) from a legacy doctor summary are now normalized as optional and non-blocking.", version, checks, optionalMissing),
	), true
}

func sessionHasScanToolCall(session *agentSession) bool {
	if session == nil {
		return false
	}
	for _, call := range session.ToolCalls {
		if strings.Contains(strings.ToLower(call.Name), "scans/web") {
			return true
		}
	}
	return false
}

func lastSuccessfulAgentToolCall(session *agentSession) *agentToolCall {
	if session == nil {
		return nil
	}
	for i := len(session.ToolCalls) - 1; i >= 0; i-- {
		if session.ToolCalls[i].Status == contracts.StatusSucceeded {
			return &session.ToolCalls[i]
		}
	}
	for i := len(session.Messages) - 1; i >= 0; i-- {
		for j := len(session.Messages[i].ToolCalls) - 1; j >= 0; j-- {
			if session.Messages[i].ToolCalls[j].Status == contracts.StatusSucceeded {
				return &session.Messages[i].ToolCalls[j]
			}
		}
	}
	return nil
}

func agentToolCompletesLegacyOneShotTask(name string) bool {
	switch {
	case strings.Contains(name, "doctor"),
		strings.Contains(name, "plugins"),
		strings.Contains(name, "runs"),
		strings.Contains(name, "approvals"),
		strings.Contains(name, "context"),
		strings.Contains(name, "workspace.command"),
		strings.Contains(name, "workspace.prepare"),
		strings.Contains(name, "workspace.files.list"),
		strings.Contains(name, "workspace.files.tree"),
		strings.Contains(name, "workspace.files.read"),
		strings.Contains(name, "workspace.search"),
		strings.Contains(name, "workspace.files.write"),
		strings.Contains(name, "workspace.files.patch"),
		strings.Contains(name, "workspace.files.diff"),
		strings.Contains(name, "workspace.files.rebase"),
		strings.Contains(name, "workspace.files.apply"),
		strings.Contains(name, "workspace.files.discard"),
		strings.Contains(name, "workspace.files.delete"),
		strings.Contains(name, "workspace.files.move"),
		strings.Contains(name, "workspace.files.copy"),
		strings.Contains(name, "workspace.files.mkdir"),
		strings.Contains(name, "http.request"):
		return true
	default:
		return false
	}
}

func (m *agentManager) shouldStartNewAgentTaskLocked(session *agentSession, prompt, language string) bool {
	if session == nil {
		return true
	}
	if session.ActiveTask == nil {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(session.ActiveTask.Status))
	if status == "done" || status == "blocked" {
		return true
	}
	if agentIsApprovalIntent(prompt) || agentIsHealthIntent(prompt) || agentIsPluginIntent(prompt) || agentIsRunIntent(prompt) {
		return false
	}
	if agentIsScanIntent(prompt) {
		return !m.hasActiveRunForSameScanLocked(session, prompt)
	}
	return false
}

func (m *agentManager) hasActiveRunForSameScanLocked(session *agentSession, prompt string) bool {
	if session == nil || session.ActiveTask == nil {
		return false
	}
	currentTarget := strings.TrimSpace(agentExtractURL(prompt))
	if currentTarget == "" {
		return false
	}
	for _, tool := range session.ToolCalls {
		if tool.Name != "POST /api/v1/scans/web" {
			continue
		}
		var previousRunID string
		if tool.Result != nil {
			previousRunID = stringFromAny(tool.Result["run_id"])
		}
		if previousRunID == "" {
			continue
		}
		if session.ActiveTask.ActiveRunID == previousRunID {
			return true
		}
	}
	return false
}

func (m *agentManager) startAgentTaskLocked(session *agentSession, prompt, language string) {
	now := time.Now().UTC()
	kind := agentTaskKind(prompt)
	goal := agentTaskGoal(language, kind, prompt)
	session.Status = "running"
	session.ActiveTask = &agentTask{
		ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
		Goal:      goal,
		Kind:      kind,
		Status:    "running",
		Summary:   agentTaskSummary(language, kind),
		CreatedAt: now,
		UpdatedAt: now,
	}
	session.Todos = agentInitialTodos(language, kind, now)
	session.NextAction = &agentNextAction{
		Type:   "continue",
		Reason: agentText(language, "正在规划并选择工具。", "Planning and selecting tools."),
	}
	session.UpdatedAt = now
	m.applyAgentRunLinkFromMessagesLocked(session, session.Messages)
}

func (m *agentManager) applyAgentRunLinkFromMessagesLocked(session *agentSession, messages []agentMessage) {
	if session == nil || len(messages) == 0 {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		for _, tool := range message.ToolCalls {
			if tool.Result == nil {
				continue
			}
			runID := stringFromAny(tool.Result["run_id"])
			if runID == "" {
				continue
			}
			if session.ActiveTask == nil {
				now := time.Now().UTC()
				session.ActiveTask = &agentTask{
					ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
					Goal:      agentText(session.Language, "跟踪运行上下文", "Track run context"),
					Kind:      "ops",
					Status:    "running",
					CreatedAt: now,
					UpdatedAt: now,
				}
			}
			if session.ActiveTask.ActiveRunID == "" {
				session.ActiveTask.ActiveRunID = runID
			}
			if session.ActiveTask.Summary == "" {
				session.ActiveTask.Summary = agentText(session.Language, "正在跟踪运行 "+shortAgentID(runID, 18)+" 的上下文。", "Tracking context for run "+shortAgentID(runID, 18)+".")
			}
			return
		}
	}
}

func (m *agentManager) ensureAgentActiveTaskSummaryLocked(session *agentSession, answer string) {
	if session == nil || strings.TrimSpace(answer) == "" {
		return
	}
	trimmed := strings.TrimSpace(answer)
	if len(trimmed) > 320 {
		trimmed = trimmed[:320]
	}
	if session.ActiveTask == nil {
		now := time.Now().UTC()
		session.ActiveTask = &agentTask{
			ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
			Goal:      agentText(session.Language, "总结 Agent 回复", "Summarize agent reply"),
			Kind:      "planning",
			Status:    "running",
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	if session.ActiveTask.Summary == "" {
		session.ActiveTask.Summary = trimmed
	}
	session.ActiveTask.UpdatedAt = time.Now().UTC()
}

func (m *agentManager) touchAgentTaskForMessageLocked(session *agentSession, prompt, language string) {
	if session == nil {
		return
	}
	now := time.Now().UTC()
	session.Status = "running"
	session.UpdatedAt = now
	if session.ActiveTask != nil {
		if session.ActiveTask.Status != "blocked" && session.ActiveTask.Status != "done" {
			session.ActiveTask.Status = "running"
		}
		session.ActiveTask.UpdatedAt = now
	}
	if session.NextAction == nil {
		session.NextAction = &agentNextAction{
			Type:   "continue",
			Reason: agentText(language, "用户继续当前任务上下文。", "Operator continues the current task context."),
		}
	}
}

func (m *agentManager) applyAgentMessagesLocked(session *agentSession, messages ...agentMessage) {
	for _, message := range messages {
		if len(message.ToolCalls) > 0 {
			m.applyAgentToolCallsLocked(session, message.ToolCalls...)
		}
		for _, link := range message.Links {
			m.addContextRefLocked(session, contextRefFromLink(link, message.Content))
		}
	}
	m.applyAgentRunLinkFromMessagesLocked(session, messages)
	_ = m.refreshApprovalGateForSessionLocked(context.Background(), session)
	m.updateNextActionLocked(session)
}

func (m *agentManager) applyAgentPlannerLocked(session *agentSession, plan *agentPlannerResponse, language string) {
	if session == nil || plan == nil {
		return
	}
	now := time.Now().UTC()
	language = normalizeAgentLanguage(language)
	session.Language = language

	if plan.Answer != "" {
		m.ensureAgentActiveTaskSummaryLocked(session, plan.Answer)
	}

	if plan.Task != nil {
		if session.ActiveTask == nil {
			session.ActiveTask = &agentTask{
				ID:        strings.Replace(runs.NewJobID(now), "job_", "task_", 1),
				Kind:      "planning",
				Status:    "running",
				CreatedAt: now,
				UpdatedAt: now,
			}
		}
		if goal := strings.TrimSpace(plan.Task.Goal); goal != "" {
			session.ActiveTask.Goal = trimSummary(goal, 260)
		}
		if kind := normalizeAgentPlannerTaskKind(plan.Task.Kind); kind != "" {
			session.ActiveTask.Kind = kind
		}
		if status := normalizeAgentPlannerTaskStatus(plan.Task.Status); status != "" {
			session.ActiveTask.Status = status
			session.Status = agentSessionStatusFromTask(status)
		}
		if activeRunID := strings.TrimSpace(plan.Task.ActiveRunID); activeRunID != "" {
			session.ActiveTask.ActiveRunID = agentCleanToken(activeRunID)
		}
		if summary := strings.TrimSpace(plan.Task.Summary); summary != "" {
			session.ActiveTask.Summary = trimSummary(summary, 360)
		}
		if strings.TrimSpace(session.ActiveTask.Goal) == "" {
			session.ActiveTask.Goal = agentText(language, "规划安全分析任务", "Plan security analysis task")
		}
		if strings.TrimSpace(session.ActiveTask.Kind) == "" {
			session.ActiveTask.Kind = "planning"
		}
		if strings.TrimSpace(session.ActiveTask.Status) == "" {
			session.ActiveTask.Status = "running"
		}
		session.ActiveTask.UpdatedAt = now
	}

	if len(plan.ContextRefs) > 0 {
		for _, ref := range plan.ContextRefs {
			m.addContextRefLocked(session, sanitizeAgentPlannerContextRef(ref))
		}
	}

	if len(plan.Todos) > 0 {
		todos := make([]agentTodo, 0, len(plan.Todos))
		for _, todo := range plan.Todos {
			title := strings.TrimSpace(todo.Title)
			if title == "" {
				continue
			}
			priority := todo.Priority
			if priority <= 0 {
				priority = len(todos) + 1
			}
			todos = append(todos, agentTodo{
				ID:            fmt.Sprintf("todo_%02d", len(todos)+1),
				Title:         trimSummary(title, 180),
				Detail:        trimSummary(todo.Detail, 360),
				Status:        normalizeAgentTodoStatus(todo.Status),
				Priority:      priority,
				ContextRefIDs: append([]string(nil), todo.ContextRefIDs...),
				BlockedReason: trimSummary(todo.BlockedReason, 260),
				UpdatedAt:     now,
			})
		}
		if len(todos) > 0 {
			todos = mergeAgentRecoveryTodos(todos, session.Todos)
			session.Todos = todos
			m.updateSessionStatusFromTodosLocked(session)
		}
	}

	if plan.NextAction != nil {
		next := sanitizeAgentNextAction(plan.NextAction)
		if next != nil {
			session.NextAction = next
			switch next.Type {
			case "blocked", "approval_required":
				if next.Type == "blocked" {
					session.Status = "blocked"
					if session.ActiveTask != nil {
						session.ActiveTask.Status = "blocked"
					}
				}
			case "finished":
				session.Status = "idle"
				if session.ActiveTask != nil {
					session.ActiveTask.Status = "done"
				}
			case "wait_user":
				if session.Status == "" || session.Status == "running" {
					session.Status = "idle"
				}
			case "continue":
				session.Status = "running"
				if session.ActiveTask != nil && session.ActiveTask.Status != "blocked" {
					session.ActiveTask.Status = "running"
				}
			}
		}
	}

	// Parse reasoning from Planner response
	if reasoning := strings.TrimSpace(plan.Reasoning); reasoning != "" {
		// Store reasoning in the last agent message
		for i := len(session.Messages) - 1; i >= 0; i-- {
			if session.Messages[i].Role == "agent" {
				session.Messages[i].Reasoning = trimSummary(reasoning, 800)
				break
			}
		}
	}

	session.UpdatedAt = now
}

func normalizeAgentPlannerTaskKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "chat", "triage", "planning", "ops", "scan", "context_analysis", "approval", "tools", "runs", "health":
		return strings.ToLower(strings.TrimSpace(raw))
	case "context-analysis", "context analysis":
		return "context_analysis"
	default:
		return ""
	}
}

func normalizeAgentPlannerTaskStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "running", "blocked", "done", "idle":
		return strings.ToLower(strings.TrimSpace(raw))
	case "complete", "completed", "finished":
		return "done"
	default:
		return ""
	}
}

func agentSessionStatusFromTask(taskStatus string) string {
	switch normalizeAgentPlannerTaskStatus(taskStatus) {
	case "blocked":
		return "blocked"
	case "done", "idle":
		return "idle"
	case "running":
		return "running"
	default:
		return "running"
	}
}

func normalizeAgentTodoStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending", "running", "done", "blocked", "skipped":
		return strings.ToLower(strings.TrimSpace(raw))
	case "todo", "open":
		return "pending"
	case "in_progress", "in-progress", "progress":
		return "running"
	case "complete", "completed", "finished":
		return "done"
	default:
		return "pending"
	}
}

func mergeAgentRecoveryTodos(planned, existing []agentTodo) []agentTodo {
	if len(existing) == 0 {
		return planned
	}
	seen := map[string]struct{}{}
	for _, todo := range planned {
		if todo.ID != "" {
			seen[todo.ID] = struct{}{}
		}
		if todo.ToolCallID != "" {
			seen["tool:"+todo.ToolCallID] = struct{}{}
		}
	}
	out := append([]agentTodo(nil), planned...)
	for _, todo := range existing {
		if !strings.HasPrefix(todo.ID, "todo_recover_") {
			continue
		}
		if _, ok := seen[todo.ID]; ok {
			continue
		}
		if todo.ToolCallID != "" {
			if _, ok := seen["tool:"+todo.ToolCallID]; ok {
				continue
			}
			seen["tool:"+todo.ToolCallID] = struct{}{}
		}
		todo.Priority = len(out) + 1
		out = append(out, todo)
		seen[todo.ID] = struct{}{}
	}
	return out
}

func sanitizeAgentPlannerContextRef(ref agentContextRef) agentContextRef {
	ref.ID = agentCleanToken(ref.ID)
	ref.Kind = strings.ToLower(agentCleanToken(ref.Kind))
	ref.RefID = agentCleanToken(ref.RefID)
	ref.Label = trimSummary(ref.Label, 120)
	ref.To = strings.TrimSpace(ref.To)
	ref.Summary = trimSummary(ref.Summary, 180)
	if ref.Kind == "" {
		ref.Kind = "context"
	}
	if ref.RefID == "" {
		ref.RefID = ref.ID
	}
	if ref.ID == "" && ref.Kind != "" && ref.RefID != "" {
		ref.ID = ref.Kind + ":" + ref.RefID
	}
	if ref.Label == "" {
		ref.Label = ref.Kind + " " + shortAgentID(ref.RefID, 18)
	}
	return ref
}

func sanitizeAgentNextAction(action *agentNextAction) *agentNextAction {
	if action == nil {
		return nil
	}
	nextType := strings.ToLower(strings.TrimSpace(action.Type))
	switch nextType {
	case "continue", "wait_user", "approval_required", "finished", "blocked":
	case "wait-user", "wait":
		nextType = "wait_user"
	case "approval", "approve":
		nextType = "approval_required"
	case "done", "complete", "completed":
		nextType = "finished"
	default:
		nextType = "continue"
	}
	return &agentNextAction{
		Type:            nextType,
		Reason:          trimSummary(action.Reason, 260),
		SuggestedPrompt: trimSummary(action.SuggestedPrompt, 260),
		AutoContinue:    action.AutoContinue && nextType == "continue",
	}
}

func (m *agentManager) updateSessionStatusFromTodosLocked(session *agentSession) {
	if session == nil {
		return
	}
	blocked := false
	runningOrPending := false
	for _, todo := range session.Todos {
		switch todo.Status {
		case "blocked":
			blocked = true
		case "running", "pending":
			runningOrPending = true
		}
	}
	switch {
	case blocked:
		session.Status = "blocked"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
		}
	case runningOrPending:
		session.Status = "running"
		if session.ActiveTask != nil && session.ActiveTask.Status != "blocked" {
			session.ActiveTask.Status = "running"
		}
	default:
		session.Status = "idle"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "done"
		}
	}
}

func (m *agentManager) applyAgentToolCallsLocked(session *agentSession, calls ...agentToolCall) {
	for _, call := range calls {
		session.ToolCalls = upsertAgentToolCall(session.ToolCalls, call)
		m.updateTodosForToolLocked(session, call)
		for _, ref := range contextRefsFromToolCall(call) {
			m.addContextRefLocked(session, ref)
		}
	}
	if len(session.ToolCalls) > 40 {
		session.ToolCalls = append([]agentToolCall(nil), session.ToolCalls[len(session.ToolCalls)-40:]...)
	}
}

func (m *agentManager) updateTodosForToolLocked(session *agentSession, call agentToolCall) {
	now := time.Now().UTC()
	if len(session.Todos) == 0 {
		return
	}
	if call.Status == contracts.StatusFailed {
		recovery := agentRecoveryForFailedTool(session.Language, call)
		reason := firstNonEmpty(recovery.Reason, agentToolError(call))
		markAgentTodoByIndex(session.Todos, len(session.Todos)-1, "blocked", now, call.ID, reason)
		upsertAgentRecoveryTodo(session, call, recovery, now)
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.Summary = trimSummary(reason, 360)
			session.ActiveTask.UpdatedAt = now
		}
		session.Status = "blocked"
		session.NextAction = &agentNextAction{
			Type:            firstNonEmpty(recovery.Type, "blocked"),
			Reason:          reason,
			SuggestedPrompt: recovery.SuggestedPrompt,
			AutoContinue:    recovery.AutoContinue && recovery.Type == "continue",
		}
		return
	}

	name := strings.ToLower(call.Name)
	switch {
	case strings.Contains(name, "scans/web"):
		markAgentTodoByIndex(session.Todos, 0, "done", now, "", "")
		markAgentTodoByIndex(session.Todos, 1, "done", now, "", "")
		markAgentTodoByIndex(session.Todos, 2, "done", now, call.ID, "")
		markAgentTodoByIndex(session.Todos, 3, "running", now, call.ID, "")
		if len(session.Todos) > 4 {
			session.Todos[4].ContextRefIDs = appendUniqueString(session.Todos[4].ContextRefIDs, contextRefIDsFromToolCall(call)...)
		}
		if session.ActiveTask != nil {
			if runID := stringFromAny(call.Result["run_id"]); runID != "" {
				session.ActiveTask.ActiveRunID = runID
			}
			if target := stringFromAny(call.Result["target"]); target != "" {
				session.ActiveTask.Summary = trimSummary(agentText(session.Language,
					fmt.Sprintf("已启动 safe profile 扫描：target=%s，scope=%s，run=%s。正在跟踪事件、证据、审批和报告。", target, emptyDash(stringFromAny(call.Result["scope"])), emptyDash(stringFromAny(call.Result["run_id"]))),
					fmt.Sprintf("Started safe-profile scan: target=%s, scope=%s, run=%s. Tracking events, evidence, approvals and report.", target, emptyDash(stringFromAny(call.Result["scope"])), emptyDash(stringFromAny(call.Result["run_id"]))),
				), 360)
			}
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
	case strings.Contains(name, "resume-approved"):
		if session.ActiveTask != nil {
			if runID := stringFromAny(call.Result["run_id"]); runID != "" {
				session.ActiveTask.ActiveRunID = runID
			}
			session.ActiveTask.Status = "running"
			session.ActiveTask.Summary = trimSummary(agentText(session.Language,
				"已恢复已批准步骤，继续跟踪运行事件、证据和报告。",
				"Resumed approved steps; continue tracking run events, evidence and report.",
			), 360)
			session.ActiveTask.UpdatedAt = now
		}
		hasRunning := false
		firstPending := -1
		for idx := range session.Todos {
			if session.Todos[idx].Status == "done" {
				continue
			}
			title := strings.ToLower(session.Todos[idx].Title)
			if strings.Contains(title, "恢复") ||
				strings.Contains(title, "resume") ||
				agentTodoWasBlockedByApprovalGate(session.Todos[idx]) ||
				agentTodoIsExplicitApprovalGate(session.Todos[idx]) {
				markAgentTodoByIndex(session.Todos, idx, "done", now, call.ID, "")
				continue
			}
			if session.Todos[idx].Status == "running" {
				hasRunning = true
			}
			if session.Todos[idx].Status == "pending" && firstPending < 0 {
				firstPending = idx
			}
		}
		if !hasRunning && firstPending >= 0 {
			markAgentTodoByIndex(session.Todos, firstPending, "running", now, call.ID, "")
			hasRunning = true
		}
		if !hasRunning {
			session.Todos = append(session.Todos, agentTodo{
				ID:            fmt.Sprintf("todo_resume_track_%d", len(session.Todos)+1),
				Title:         agentText(session.Language, "跟踪恢复后的运行事件、证据和报告", "Track events, evidence and report after resume"),
				Status:        "running",
				Priority:      len(session.Todos) + 1,
				ToolCallID:    call.ID,
				ContextRefIDs: contextRefIDsFromToolCall(call),
				UpdatedAt:     now,
			})
		}
	case strings.Contains(name, "runs/{run_id}/context"):
		contextKind := stringFromAny(call.Result["context"])
		if session.ActiveTask != nil && session.ActiveTask.Kind == "scan" && len(session.Todos) >= 5 {
			markAgentTodoByIndex(session.Todos, 3, "done", now, call.ID, "")
			switch contextKind {
			case "report_json", "findings":
				markAgentTodoByIndex(session.Todos, 4, "done", now, call.ID, "")
				if agentRunContextHasFindings(call.Result) {
					if !m.ensureRunEvidenceActionTodosLocked(session, call, now) {
						m.advanceRunEvidenceActionTodoLocked(session, call, now)
					}
				}
			default:
				if session.Todos[4].Status == "pending" {
					markAgentTodoByIndex(session.Todos, 4, "running", now, call.ID, "")
				}
			}
			if session.ActiveTask.ActiveRunID == "" {
				if runID := stringFromAny(call.Result["run_id"]); runID != "" {
					session.ActiveTask.ActiveRunID = runID
				}
			}
			session.ActiveTask.UpdatedAt = now
		} else {
			for idx := range session.Todos {
				if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
					markAgentTodoByIndex(session.Todos, idx, "done", now, call.ID, "")
				}
			}
		}
	case strings.Contains(name, "/approvals/") && (strings.Contains(name, "/approve") || strings.Contains(name, "/deny")):
		if session.ActiveTask != nil {
			if runID := stringFromAny(call.Result["run_id"]); runID != "" {
				session.ActiveTask.ActiveRunID = runID
			}
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
		resolveAgentApprovalGateTodos(session, now)
	case agentToolCompletesLocalTask(name):
		for idx := range session.Todos {
			if session.Todos[idx].Status == "running" || session.Todos[idx].Status == "pending" {
				markAgentTodoByIndex(session.Todos, idx, "done", now, call.ID, "")
			}
		}
	}
}

func agentRunContextHasFindings(result map[string]any) bool {
	if result == nil {
		return false
	}
	return intFromAny(result["finding_count"]) > 0 || intFromAny(result["findings"]) > 0
}

func (m *agentManager) ensureRunEvidenceActionTodosLocked(session *agentSession, call agentToolCall, now time.Time) bool {
	if session == nil {
		return false
	}
	runID := stringFromAny(call.Result["run_id"])
	if runID == "" {
		if session.ActiveTask != nil {
			runID = session.ActiveTask.ActiveRunID
		}
	}
	for _, todo := range session.Todos {
		if strings.HasPrefix(todo.ID, "todo_run_action_") {
			return false
		}
	}
	refs := contextRefIDsFromToolCall(call)
	if runID != "" {
		refs = appendUniqueString(refs, "run:"+runID, stringFromAny(call.Result["context"])+":"+runID)
	}
	start := len(session.Todos) + 1
	titles := []string{
		agentText(session.Language, "关联 finding、evidence 与 artifact 证据链", "Correlate findings, evidence and artifact chain"),
		agentText(session.Language, "执行误报检查并明确影响边界", "Perform false-positive checks and clarify impact boundary"),
		agentText(session.Language, "生成修复建议与最小复测计划", "Produce remediation guidance and minimal retest plan"),
	}
	for idx, title := range titles {
		status := "pending"
		if idx == 0 {
			status = "running"
		}
		session.Todos = append(session.Todos, agentTodo{
			ID:            fmt.Sprintf("todo_run_action_%02d", idx+1),
			Title:         title,
			Status:        status,
			Priority:      start + idx,
			ToolCallID:    call.ID,
			ContextRefIDs: refs,
			UpdatedAt:     now,
		})
	}
	if session.ActiveTask != nil {
		session.ActiveTask.Status = "running"
		session.ActiveTask.Summary = trimSummary(agentText(session.Language,
			"已读取运行 "+emptyDash(runID)+" 的报告发现，正在把证据转成误报检查、修复和复测行动计划。",
			"Read findings for run "+emptyDash(runID)+"; converting evidence into false-positive checks, remediation and retest plan.",
		), 360)
		session.ActiveTask.UpdatedAt = now
	}
	session.Status = "running"
	session.NextAction = &agentNextAction{
		Type:            "continue",
		Reason:          agentText(session.Language, "报告中存在发现，已自动生成后续安全行动计划。", "Findings exist in the report; generated the follow-up security action plan."),
		SuggestedPrompt: agentSuggestedEvidenceActionPrompt(session.Language, runID),
		AutoContinue:    true,
	}
	return true
}

func (m *agentManager) advanceRunEvidenceActionTodoLocked(session *agentSession, call agentToolCall, now time.Time) {
	if session == nil {
		return
	}
	advanced := false
	for idx := range session.Todos {
		if !strings.HasPrefix(session.Todos[idx].ID, "todo_run_action_") || session.Todos[idx].Status != "running" {
			continue
		}
		markAgentTodoByIndex(session.Todos, idx, "done", now, call.ID, "")
		for next := idx + 1; next < len(session.Todos); next++ {
			if strings.HasPrefix(session.Todos[next].ID, "todo_run_action_") && session.Todos[next].Status == "pending" {
				markAgentTodoByIndex(session.Todos, next, "running", now, call.ID, "")
				advanced = true
				break
			}
		}
		if !advanced {
			advanced = true
		}
		break
	}
	if !advanced {
		return
	}
	runID := stringFromAny(call.Result["run_id"])
	if runID == "" && session.ActiveTask != nil {
		runID = session.ActiveTask.ActiveRunID
	}
	if sessionHasRunEvidenceActionTodos(session) {
		session.Status = "running"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "running"
			session.ActiveTask.UpdatedAt = now
		}
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          agentText(session.Language, "已推进一个证据行动 TODO，可继续完成误报检查、修复或复测计划。", "Advanced one evidence-action TODO; continue to complete false-positive check, remediation, or retest plan."),
			SuggestedPrompt: agentSuggestedEvidenceActionPrompt(session.Language, runID),
			AutoContinue:    true,
		}
		return
	}
	session.Status = "idle"
	if session.ActiveTask != nil {
		session.ActiveTask.Status = "done"
		session.ActiveTask.Summary = trimSummary(agentText(session.Language,
			"运行 "+emptyDash(runID)+" 的发现已完成证据链关联、误报检查、修复建议与复测计划整理。",
			"Completed evidence correlation, false-positive checks, remediation guidance and retest planning for run "+emptyDash(runID)+".",
		), 360)
		session.ActiveTask.UpdatedAt = now
	}
	session.NextAction = &agentNextAction{
		Type:            "wait_user",
		Reason:          agentText(session.Language, "证据行动计划已完成，等待用户要求导出报告、复测或开始新任务。", "Evidence action plan completed; waiting for export, retest, or a new task."),
		SuggestedPrompt: agentSuggestedRunSummaryPrompt(session.Language, runID),
	}
}

func agentToolCompletesLocalTask(name string) bool {
	switch {
	case strings.Contains(name, "doctor"),
		strings.Contains(name, "plugins"),
		strings.Contains(name, "runs"),
		strings.Contains(name, "approvals"),
		strings.Contains(name, "context"),
		strings.Contains(name, "workspace.command"),
		strings.Contains(name, "workspace.prepare"),
		strings.Contains(name, "workspace.files.list"),
		strings.Contains(name, "workspace.files.tree"),
		strings.Contains(name, "workspace.files.read"),
		strings.Contains(name, "workspace.search"),
		strings.Contains(name, "workspace.files.write"),
		strings.Contains(name, "workspace.files.patch"),
		strings.Contains(name, "workspace.files.diff"),
		strings.Contains(name, "workspace.files.rebase"),
		strings.Contains(name, "workspace.files.apply"),
		strings.Contains(name, "workspace.files.discard"),
		strings.Contains(name, "workspace.files.delete"),
		strings.Contains(name, "workspace.files.move"),
		strings.Contains(name, "workspace.files.copy"),
		strings.Contains(name, "workspace.files.mkdir"),
		strings.Contains(name, "http.request"),
		strings.Contains(name, "llm"):
		return true
	default:
		return false
	}
}

func (m *agentManager) updateNextActionLocked(session *agentSession) {
	now := time.Now().UTC()
	if session.ActiveTask != nil {
		session.ActiveTask.UpdatedAt = now
	}
	if session.PendingAuthorization != nil && strings.EqualFold(session.PendingAuthorization.Status, "pending") {
		session.Status = "waiting_authorization"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
		}
		if session.NextAction == nil || session.NextAction.Type != "authorization_required" {
			session.NextAction = &agentNextAction{
				Type:            "authorization_required",
				Reason:          session.PendingAuthorization.Reason,
				SuggestedPrompt: agentAuthorizationSuggestedPrompt(session.Language, session.PendingAuthorization),
				AutoContinue:    false,
			}
		}
		session.UpdatedAt = now
		return
	}
	if session.NextAction != nil && session.NextAction.Type == "approval_required" {
		session.Status = "running"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "running"
		}
		session.UpdatedAt = now
		return
	}
	if session.NextAction != nil && session.NextAction.Type == "wait_user" && agentNextActionIsEvidenceActionComplete(session.NextAction) {
		session.Status = "idle"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "done"
		}
		session.UpdatedAt = now
		return
	}
	pending := 0
	running := 0
	blocked := 0
	for _, todo := range session.Todos {
		switch todo.Status {
		case "pending":
			pending++
		case "running":
			running++
		case "blocked":
			blocked++
		}
	}
	switch {
	case blocked > 0:
		session.Status = "blocked"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
		}
		if session.NextAction == nil || session.NextAction.Type != "blocked" {
			session.NextAction = &agentNextAction{Type: "blocked", Reason: "tool or policy step is blocked"}
		}
	case running > 0 || pending > 0:
		session.Status = "running"
		session.NextAction = &agentNextAction{
			Type:            "continue",
			Reason:          "task has pending or running steps",
			SuggestedPrompt: agentSuggestedContinuePrompt(session.Language, session),
			AutoContinue:    session.ActiveTask != nil && strings.TrimSpace(session.ActiveTask.ActiveRunID) != "",
		}
	default:
		session.Status = "idle"
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "done"
		}
		session.NextAction = &agentNextAction{Type: "finished", Reason: "task steps completed"}
	}
	session.UpdatedAt = now
}

func agentNextActionIsEvidenceActionComplete(next *agentNextAction) bool {
	if next == nil {
		return false
	}
	reason := strings.ToLower(strings.TrimSpace(next.Reason))
	return strings.Contains(reason, "证据行动计划已完成") ||
		strings.Contains(reason, "evidence action plan completed")
}

func agentTaskKind(prompt string) string {
	if decision := agentClassifyCapabilityIntent(prompt); decision.ID != "" && decision.ID != agentIntentGeneralChat {
		return decision.Kind
	}
	switch {
	case agentIsPortScanIntent(prompt):
		return "port_scan"
	case agentIsScanIntent(prompt):
		return "scan"
	case agentIsResumeApprovedIntent(prompt):
		return "approval"
	case agentIsRunContextAnalysisIntent(prompt):
		return "context_analysis"
	case agentIsApprovalIntent(prompt):
		return "approval"
	case agentIsHealthIntent(prompt):
		return "health"
	case agentIsPluginIntent(prompt):
		return "tools"
	case agentIsRunIntent(prompt):
		return "runs"
	default:
		return "chat"
	}
}

func agentTaskGoal(language, kind, prompt string) string {
	if kind == "scan" {
		target := agentExtractURL(prompt)
		scope := agentExtractScope(prompt, target)
		return agentText(language, "扫描授权目标 "+emptyDash(target)+" scope="+emptyDash(scope), "Scan authorized target "+emptyDash(target)+" scope="+emptyDash(scope))
	}
	if kind == "http_probe" {
		target := agentExtractURL(prompt)
		return agentText(language, "探测 HTTP 响应 "+emptyDash(target), "Probe HTTP response for "+emptyDash(target))
	}
	if kind == "port_scan" {
		target := firstNonEmpty(agentExtractURL(prompt), agentExtractPortScanHostFromPrompt(prompt))
		return agentText(language, "枚举开放端口 "+emptyDash(target), "Enumerate open ports for "+emptyDash(target))
	}
	if kind == "content_discovery" || kind == "js_route_analysis" || kind == "api_auth_test" {
		return agentCapabilityGoal(language, agentClassifyCapabilityIntent(prompt), prompt)
	}
	if kind == "context_analysis" {
		return agentText(language, "分析运行上下文与证据", "Analyze run context and evidence")
	}
	if kind == "approval" {
		if agentIsResumeApprovedIntent(prompt) {
			return agentText(language, "恢复已批准运行步骤", "Resume approved run steps")
		}
		return agentText(language, "检查待审批动作", "Check pending approvals")
	}
	if kind == "health" {
		return agentText(language, "检查 daemon 健康状态", "Check daemon health")
	}
	if kind == "tools" {
		return agentText(language, "列出 Agent 工具和插件能力", "List Agent tools and plugin capabilities")
	}
	if kind == "runs" {
		return agentText(language, "查看运行历史和报告上下文", "Review run history and report context")
	}
	return agentText(language, "回答并澄清安全任务", "Answer and clarify the security task")
}

func agentTaskSummary(language, kind string) string {
	switch kind {
	case "scan":
		return agentText(language, "Agent 将校验 scope、启动 safe profile、跟踪运行并收集报告上下文。", "The Agent will validate scope, start the safe profile, track the run and collect report context.")
	case "http_probe":
		return agentText(language, "Agent 将执行最小 HTTP 请求并根据响应状态、响应头和正文摘要给出结论。", "The Agent will run the minimal HTTP request and summarize status, headers, and body snippet.")
	case "port_scan":
		return agentText(language, "Agent 会识别这是端口枚举任务；若缺少 nmap/masscan 类工具，会先提示可选替代方案。", "The Agent will classify this as port enumeration; if nmap/masscan-style tools are missing, it will offer alternatives first.")
	case "content_discovery":
		return agentText(language, "Agent 会识别这是内容发现/目录枚举任务；缺少等价工具时不会误用漏洞扫描代替。", "The Agent will classify this as content discovery/directory enumeration; without an equivalent tool it will not substitute vulnerability scanning.")
	case "js_route_analysis":
		return agentText(language, "Agent 会识别这是 JS 路由/前端接口提取任务；缺少分析器时等待插件或 JS 文件。", "The Agent will classify this as JS route/front-end endpoint extraction; without an analyzer it waits for a plugin or JS files.")
	case "api_auth_test":
		return agentText(language, "Agent 会识别这是认证/鉴权测试；需要具体 endpoint、凭据/Auth Profile 或专用流程。", "The Agent will classify this as auth/authorization testing; it needs exact endpoints, credentials/Auth Profile, or a dedicated workflow.")
	case "web_search":
		return agentText(language, "Agent 将使用配置的 Web Search provider 检索公开信息并返回来源摘要。", "The Agent will use the configured Web Search provider to look up public information and summarize sourced results.")
	case "web_fetch":
		return agentText(language, "Agent 将读取指定公开 URL，并在 SSRF 防护下返回可读内容摘要。", "The Agent will read the specified public URL and return readable content under SSRF protections.")
	case "context_analysis":
		return agentText(language, "Agent 将读取 run / artifact / finding 上下文并给出下一步建议。", "The Agent will read run / artifact / finding context and suggest next actions.")
	default:
		return agentText(language, "Agent 将选择最小必要工具并返回可执行结果。", "The Agent will select the minimal required tools and return actionable results.")
	}
}

func agentInitialTodos(language, kind string, now time.Time) []agentTodo {
	titles := []string{}
	switch kind {
	case "scan":
		titles = []string{
			agentText(language, "确认目标与 scope", "Confirm target and scope"),
			agentText(language, "生成 safe profile 执行计划", "Create safe-profile plan"),
			agentText(language, "启动 daemon Web 扫描", "Start daemon web scan"),
			agentText(language, "跟踪运行事件、证据和审批", "Track run events, evidence and approvals"),
			agentText(language, "读取报告并总结下一步", "Read report and summarize next actions"),
		}
	case "port_scan":
		titles = []string{
			agentText(language, "确认目标与端口枚举范围", "Confirm target and port-enumeration scope"),
			agentText(language, "检查是否有端口扫描专用工具", "Check for a dedicated port-scanning tool"),
			agentText(language, "选择 nmap/masscan 插件或 HTTP 探测替代方案", "Choose nmap/masscan plugin or HTTP-probe fallback"),
			agentText(language, "汇总开放端口证据与限制", "Summarize open-port evidence and limitations"),
		}
	case "http_probe":
		titles = []string{
			agentText(language, "确认 URL 与 HTTP 方法", "Confirm URL and HTTP method"),
			agentText(language, "执行最小 HTTP 请求", "Run the minimal HTTP request"),
			agentText(language, "总结状态码、响应头和正文摘要", "Summarize status, headers, and body snippet"),
		}
	case "content_discovery":
		titles = []string{
			agentText(language, "确认这是内容发现/目录枚举", "Confirm this is content discovery/directory enumeration"),
			agentText(language, "检查是否有 ffuf/gobuster/dirsearch 等价工具", "Check for equivalent ffuf/gobuster/dirsearch tool"),
			agentText(language, "等待插件或具体路径验证方案", "Wait for plugin or exact-path validation fallback"),
		}
	case "js_route_analysis":
		titles = []string{
			agentText(language, "确认这是 JS 路由/前端接口提取", "Confirm this is JS route/front-end endpoint extraction"),
			agentText(language, "检查是否有 JS 路由分析器", "Check for a JS route analyzer"),
			agentText(language, "等待插件、JS 文件或入口页验证方案", "Wait for plugin, JS files, or entry-page fallback"),
		}
	case "api_auth_test":
		titles = []string{
			agentText(language, "确认认证/鉴权测试目标", "Confirm auth/authorization test target"),
			agentText(language, "收集 endpoint、method、Auth Profile/凭据上下文", "Collect endpoint, method, Auth Profile/credential context"),
			agentText(language, "选择 http.request 验证或专用插件流程", "Choose http.request validation or dedicated plugin workflow"),
		}
	case "web_search":
		titles = []string{
			agentText(language, "确认检索问题", "Confirm search query"),
			agentText(language, "使用 Web Search provider 检索", "Search with the Web Search provider"),
			agentText(language, "汇总来源和关键信息", "Summarize sources and key information"),
		}
	case "web_fetch":
		titles = []string{
			agentText(language, "确认要读取的 URL", "Confirm URL to read"),
			agentText(language, "获取并提取页面内容", "Fetch and extract page content"),
			agentText(language, "总结页面要点", "Summarize page takeaways"),
		}
	case "context_analysis":
		titles = []string{
			agentText(language, "定位 run / finding / artifact", "Locate run / finding / artifact"),
			agentText(language, "读取上下文和报告", "Read context and report"),
			agentText(language, "总结影响、证据和复测建议", "Summarize impact, evidence and retest advice"),
		}
	default:
		titles = []string{
			agentText(language, "理解用户意图", "Understand user intent"),
			agentText(language, "选择必要工具或本地回答", "Select required tool or local answer"),
			agentText(language, "返回结果和下一步", "Return result and next action"),
		}
	}
	todos := make([]agentTodo, 0, len(titles))
	for i, title := range titles {
		status := "pending"
		if i == 0 {
			status = "running"
		}
		todos = append(todos, agentTodo{
			ID:        fmt.Sprintf("todo_%02d", i+1),
			Title:     title,
			Status:    status,
			Priority:  i + 1,
			UpdatedAt: now,
		})
	}
	return todos
}

func agentIsEvidenceActionPrompt(prompt string) bool {
	normalized := strings.ToLower(strings.TrimSpace(prompt))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "evidence") && (strings.Contains(normalized, "next action") || strings.Contains(normalized, "action plan")) {
		return true
	}
	return strings.Contains(prompt, "证据") && (strings.Contains(prompt, "下一步行动计划") || strings.Contains(prompt, "行动计划"))
}

func agentEvidenceActionHasApprovalGate(prompt string) bool {
	normalized := strings.ToLower(prompt)
	return strings.Contains(normalized, "pending_approvals=1") ||
		strings.Contains(normalized, "pending approvals:") ||
		strings.Contains(normalized, "approval gate") ||
		strings.Contains(prompt, "待审批") ||
		strings.Contains(prompt, "审批 gate")
}

func agentEvidenceActionTodos(language, prompt string, now time.Time) []agentTodo {
	hasApproval := agentEvidenceActionHasApprovalGate(prompt)
	titles := []string{}
	if hasApproval {
		titles = append(titles, agentText(language, "处理待审批 gate 并确认是否可继续", "Resolve pending approval gate and confirm whether execution can continue"))
	}
	titles = append(titles,
		agentText(language, "读取 run/report/artifact/approval 证据上下文", "Read run/report/artifact/approval evidence context"),
		agentText(language, "关联关键 finding、artifact 与 step", "Correlate key findings, artifacts and steps"),
		agentText(language, "形成下一步安全动作与复测计划", "Produce next safe action and retest plan"),
	)
	if !hasApproval {
		titles = append(titles, agentText(language, "执行低风险只读验证或等待人工确认", "Run low-risk read-only validation or wait for operator confirmation"))
	}
	todos := make([]agentTodo, 0, len(titles))
	for i, title := range titles {
		status := "pending"
		if i == 0 {
			status = "running"
		}
		todos = append(todos, agentTodo{
			ID:        fmt.Sprintf("todo_evidence_%02d", i+1),
			Title:     title,
			Status:    status,
			Priority:  i + 1,
			UpdatedAt: now,
		})
	}
	return todos
}

func markAgentTodoByIndex(todos []agentTodo, idx int, status string, now time.Time, toolCallID, blockedReason string) {
	if idx < 0 || idx >= len(todos) {
		return
	}
	todos[idx].Status = status
	todos[idx].UpdatedAt = now
	if toolCallID != "" {
		todos[idx].ToolCallID = toolCallID
	}
	todos[idx].BlockedReason = blockedReason
}

func (m *agentManager) ensureRunningTodoLocked(session *agentSession, now time.Time) {
	if len(session.Todos) == 0 {
		return
	}
	for idx := range session.Todos {
		if session.Todos[idx].Status == "running" {
			return
		}
	}
	for idx := range session.Todos {
		if session.Todos[idx].Status == "pending" || session.Todos[idx].Status == "blocked" {
			markAgentTodoByIndex(session.Todos, idx, "running", now, "", "")
			return
		}
	}
}

func (m *agentManager) findActiveJobIDLocked(session *agentSession, runID string) string {
	for i := len(session.ToolCalls) - 1; i >= 0; i-- {
		call := session.ToolCalls[i]
		if call.Result == nil {
			continue
		}
		callRunID := stringFromAny(call.Result["run_id"])
		if runID != "" && callRunID != "" && callRunID != runID {
			continue
		}
		if jobID := stringFromAny(call.Result["job_id"]); jobID != "" {
			return jobID
		}
	}
	return ""
}

func agentSuggestedContinuePrompt(language string, session *agentSession) string {
	if session != nil && session.ActiveTask != nil && session.ActiveTask.ActiveRunID != "" {
		if sessionHasRunEvidenceActionTodos(session) {
			return agentSuggestedEvidenceActionPrompt(language, session.ActiveTask.ActiveRunID)
		}
		return agentText(language, "继续跟踪运行 "+session.ActiveTask.ActiveRunID+" 的上下文、证据并总结下一步", "continue tracking run "+session.ActiveTask.ActiveRunID+" context and evidence, then summarize next steps")
	}
	return agentText(language, "继续当前任务", "continue the current task")
}

func sessionHasRunEvidenceActionTodos(session *agentSession) bool {
	if session == nil {
		return false
	}
	for _, todo := range session.Todos {
		if strings.HasPrefix(todo.ID, "todo_run_action_") && (todo.Status == "running" || todo.Status == "pending") {
			return true
		}
	}
	return false
}

func agentEvidenceActionStageFromTodos(todos []agentTodo) int {
	for _, todo := range todos {
		if !strings.HasPrefix(todo.ID, "todo_run_action_") || todo.Status != "running" {
			continue
		}
		switch todo.ID {
		case "todo_run_action_01":
			return 1
		case "todo_run_action_02":
			return 2
		case "todo_run_action_03":
			return 3
		}
	}
	for _, todo := range todos {
		if strings.HasPrefix(todo.ID, "todo_run_action_") {
			return 1
		}
	}
	return 0
}

func agentEvidenceActionStageConclusion(language, runID string, stage int, result map[string]any) string {
	findings := intFromAny(result["finding_count"])
	if findings == 0 {
		findings = intFromAny(result["findings"])
	}
	evidence := intFromAny(result["evidence_count"])
	artifacts := intFromAny(result["artifact_count"])
	switch stage {
	case 0:
		return agentText(language,
			fmt.Sprintf("已基于运行 %s 的报告生成后续行动计划：先关联 %d 个 finding 与 %d 条 evidence / %d 个 artifact，再做误报检查、影响边界、修复建议和复测计划。", emptyDash(runID), findings, evidence, artifacts),
			fmt.Sprintf("Generated a follow-up action plan from run %s: correlate %d finding(s) with %d evidence item(s) / %d artifact(s), then perform false-positive checks, impact scoping, remediation and retest planning.", emptyDash(runID), findings, evidence, artifacts),
		)
	case 1:
		return agentText(language,
			fmt.Sprintf("证据链关联结论：运行 %s 当前至少包含 %d 个 finding、%d 条 evidence、%d 个 artifact；下一步应按 evidence_ids/step_id 把每个发现映射到原始响应、头信息或工具输出，避免凭标题直接下结论。", emptyDash(runID), findings, evidence, artifacts),
			fmt.Sprintf("Evidence-chain conclusion: run %s has at least %d finding(s), %d evidence item(s), and %d artifact(s). Next, map each finding via evidence_ids/step_id to raw responses, headers, or tool output before drawing conclusions.", emptyDash(runID), findings, evidence, artifacts),
		)
	case 2:
		return agentText(language,
			fmt.Sprintf("误报检查与影响边界：对运行 %s 的发现，应优先确认是否由反向代理、CDN、开发服务器或测试夹具导致；再结合授权目标的真实暴露面判断业务影响。", emptyDash(runID)),
			fmt.Sprintf("False-positive and impact boundary: for run %s, first check whether reverse proxies, CDNs, dev servers, or test fixtures explain the findings; then scope business impact against the authorized target exposure.", emptyDash(runID)),
		)
	case 3:
		return agentText(language,
			fmt.Sprintf("修复与复测计划：对运行 %s，建议按证据优先级修复配置或代码，再用最小复现命令验证，最后让 Agent 对同一 scope 重跑 safe profile 确认 finding 是否消失。", emptyDash(runID)),
			fmt.Sprintf("Remediation and retest plan: for run %s, fix configuration or code by evidence priority, verify with a minimal reproduction command, then ask the Agent to rerun the safe profile on the same scope.", emptyDash(runID)),
		)
	default:
		return ""
	}
}

func agentSuggestedEvidenceActionPrompt(language, runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return agentText(language,
			"基于当前运行 findings 生成影响、证据、误报检查、修复与复测计划",
			"build an impact, evidence, false-positive check, remediation and retest plan from the current run findings",
		)
	}
	return agentText(language,
		"基于运行 "+runID+" 的 findings 生成影响、证据、误报检查、修复与复测计划",
		"build an impact, evidence, false-positive check, remediation and retest plan from findings in run "+runID,
	)
}

func agentSuggestedRunSummaryPrompt(language, runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return agentText(language, "总结最近一次运行的证据、发现和下一步建议", "summarize the latest run evidence, findings, and next steps")
	}
	return agentText(language, "总结运行 "+runID+" 的证据、发现和下一步建议", "summarize evidence, findings, and next steps for run "+runID)
}

func agentSuggestedStoppedRunPrompt(language, runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return agentText(language, "分析最近一次停止运行的错误、证据和下一步恢复建议", "analyze the latest stopped run error, evidence, and recovery suggestions")
	}
	return agentText(language, "分析运行 "+runID+" 的停止原因、已有证据和下一步恢复建议", "analyze why run "+runID+" stopped, existing evidence, and recovery suggestions")
}

func agentSuggestedStopPrompt(language string, session *agentSession) string {
	if session != nil && session.ActiveTask != nil && session.ActiveTask.ActiveRunID != "" {
		return agentText(language, "分析已停止运行 "+session.ActiveTask.ActiveRunID+" 的当前证据", "analyze the current evidence for stopped run "+session.ActiveTask.ActiveRunID)
	}
	return agentText(language, "解释当前任务为何停止，并给出下一步", "explain why the current task stopped and suggest next steps")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func appendAgentInstructionBlock(base, block string) string {
	base = strings.TrimSpace(base)
	block = strings.TrimSpace(block)
	if block == "" {
		return base
	}
	if base == "" {
		return block
	}
	return base + "\n\n" + block
}

func agentPlannerActions(plan *agentPlannerResponse) []agentPlannerAction {
	if plan == nil {
		return nil
	}
	rawActions := append([]agentPlannerAction{}, plan.Actions...)
	rawActions = append(rawActions, plan.ToolCalls...)

	actions := make([]agentPlannerAction, 0, len(rawActions))
	seen := map[string]struct{}{}
	for _, action := range rawActions {
		name := normalizeAgentPlannerActionName(action.Name)
		if name == "" {
			name = normalizeAgentPlannerActionName(stringFromAny(action.Arguments["name"]))
		}
		if name == "" {
			continue
		}
		action.Name = name
		if action.Arguments == nil {
			action.Arguments = map[string]any{}
		}
		keyParts := []string{
			name,
			stringFromAny(action.Arguments["host"]),
			agentPortSpecFromPlannerArgument(action.Arguments["ports"]),
			stringFromAny(action.Arguments["url"]),
			stringFromAny(action.Arguments["target"]),
			stringFromAny(action.Arguments["method"]),
			stringFromAny(action.Arguments["query"]),
			stringFromAny(action.Arguments["pattern"]),
			stringFromAny(action.Arguments["command"]),
			stringFromAny(action.Arguments["path"]),
			stringFromAny(action.Arguments["file"]),
			stringFromAny(action.Arguments["content"]),
			stringFromAny(action.Arguments["body"]),
			stringFromAny(action.Arguments["text"]),
			stringFromAny(action.Arguments["old"]),
			stringFromAny(action.Arguments["old_text"]),
			stringFromAny(action.Arguments["before"]),
			stringFromAny(action.Arguments["search"]),
			stringFromAny(action.Arguments["new"]),
			stringFromAny(action.Arguments["new_text"]),
			stringFromAny(action.Arguments["after"]),
			stringFromAny(action.Arguments["replace"]),
			stringFromAny(action.Arguments["change_id"]),
			stringFromAny(action.Arguments["id"]),
		}
		key := strings.Join(keyParts, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		actions = append(actions, action)
	}
	return actions
}

// normalizeAgentPlannerActionName collapses planner-emitted tool names
// (canonical, alias, casing variant, dash/underscore variants) into the
// canonical name held by the daemon tool registry. Unknown names pass through
// folded so plugin/MCP names still flow downstream.
//
// The implementation is a thin wrapper over toolregistry.Registry.Normalize
// to keep a single source of truth; a parity test in
// agent_toolregistry_parity_test.go guards against drift while remaining
// callers migrate to the registry directly.
func normalizeAgentPlannerActionName(raw string) string {
	if mcpName := normalizeAgentMCPToolActionName(raw); mcpName != "" {
		return mcpName
	}
	return toolRegistry().Normalize(raw)
}

func agentPlannerCommandAllowlist() []string {
	return []string{
		"pwd", "ls", "cat", "head", "tail", "wc", "grep", "find",
		"sort", "uniq", "cut", "printf", "echo",
		"date", "whoami", "which", "file", "stat", "uname",
		"hostname", "id", "groups",
		"git status", "git log", "git show", "git diff",
		"git branch", "git rev-parse", "git ls-files",
	}
}

func agentPlannerCommandAllowed(cmd string) bool {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "pwd", "ls", "cat", "head", "tail", "wc", "grep", "find", "sort", "uniq", "cut", "printf", "echo", "date", "whoami", "which", "file", "stat", "uname", "hostname", "id", "groups":
		return true
	case "git":
		if len(fields) < 2 {
			return false
		}
		switch fields[1] {
		case "status", "log", "show", "diff", "branch", "rev-parse", "ls-files":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func upsertAgentToolCall(calls []agentToolCall, call agentToolCall) []agentToolCall {
	for i := range calls {
		if calls[i].ID == call.ID {
			calls[i] = call
			return calls
		}
	}
	return append(calls, call)
}

func (m *agentManager) addContextRefLocked(session *agentSession, ref agentContextRef) bool {
	if ref.ID == "" || ref.Kind == "" || ref.RefID == "" {
		return false
	}
	for i := range session.ContextRefs {
		if session.ContextRefs[i].ID == ref.ID {
			if session.ContextRefs[i] == ref {
				return false
			}
			session.ContextRefs[i] = ref
			return true
		}
	}
	session.ContextRefs = append([]agentContextRef{ref}, session.ContextRefs...)
	if len(session.ContextRefs) > 24 {
		session.ContextRefs = session.ContextRefs[:24]
	}
	return true
}

func contextRefsFromToolCall(call agentToolCall) []agentContextRef {
	refs := []agentContextRef{}
	if call.Result == nil {
		return refs
	}
	if runID := stringFromAny(call.Result["run_id"]); runID != "" {
		refs = append(refs, agentContextRef{
			ID:      "run:" + runID,
			Kind:    "run",
			RefID:   runID,
			Label:   "run " + shortAgentID(runID, 18),
			To:      "/scan/" + runID,
			Summary: "Agent runtime run context",
		})
	}
	if jobID := stringFromAny(call.Result["job_id"]); jobID != "" {
		refs = append(refs, agentContextRef{
			ID:      "job:" + jobID,
			Kind:    "job",
			RefID:   jobID,
			Label:   "job " + shortAgentID(jobID, 18),
			To:      "/history",
			Summary: call.Name,
		})
	}
	if contextKind := stringFromAny(call.Result["context"]); contextKind != "" {
		if runID := stringFromAny(call.Result["run_id"]); runID != "" {
			refs = append(refs, agentContextRef{
				ID:      contextKind + ":" + runID,
				Kind:    contextKind,
				RefID:   runID,
				Label:   contextKind + " " + shortAgentID(runID, 18),
				To:      "/scan/" + runID,
				Summary: call.Name,
			})
		}
	}
	if approvalID := stringFromAny(call.Result["approval_id"]); approvalID != "" {
		runID := stringFromAny(call.Result["run_id"])
		refID := approvalID
		to := "/history"
		id := "approval:" + approvalID
		if runID != "" {
			refID = runID
			to = "/scan/" + runID
			id = "approval:" + runID
		}
		refs = append(refs, agentContextRef{
			ID:      id,
			Kind:    "approval",
			RefID:   refID,
			Label:   "approval " + shortAgentID(approvalID, 18),
			To:      to,
			Summary: call.Name,
		})
	}
	if pending := intFromAny(call.Result["pending"]); pending > 0 {
		runID := stringFromAny(call.Result["run_id"])
		refID := "approval_queue"
		to := "/history"
		if runID != "" {
			refID = runID
			to = "/scan/" + runID
		}
		refs = append(refs, agentContextRef{
			ID:      "approval:" + refID,
			Kind:    "approval",
			RefID:   refID,
			Label:   fmt.Sprintf("approval queue (%d)", pending),
			To:      to,
			Summary: call.Name,
		})
	}
	if path := stringFromAny(call.Result["path"]); path != "" && path != "." {
		name := strings.ToLower(call.Name)
		if strings.Contains(name, "workspace.files.read") || strings.Contains(name, "workspace.files.write") || strings.Contains(name, "workspace.files.patch") || strings.Contains(name, "workspace.files.diff") || strings.Contains(name, "workspace.files.apply") || strings.Contains(name, "workspace.files.discard") || strings.Contains(name, "workspace.files.delete") || strings.Contains(name, "workspace.files.move") || strings.Contains(name, "workspace.files.copy") || strings.Contains(name, "workspace.files.mkdir") || strings.Contains(name, "reports.draft") {
			kind := "workspace_file"
			label := "file " + path
			if strings.Contains(name, "workspace.files.mkdir") {
				kind = "workspace_dir"
				label = "dir " + path
			}
			refs = append(refs, agentContextRef{
				ID:      "workspace_file:" + path,
				Kind:    kind,
				RefID:   path,
				Label:   label,
				Summary: call.Name,
			})
		}
	}
	return refs
}

func contextRefIDsFromToolCall(call agentToolCall) []string {
	refs := contextRefsFromToolCall(call)
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.ID) != "" {
			ids = append(ids, ref.ID)
		}
	}
	return ids
}

func appendUniqueString(items []string, values ...string) []string {
	seen := make(map[string]struct{}, len(items)+len(values))
	out := make([]string, 0, len(items)+len(values))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func contextRefFromLink(link agentLink, summary string) agentContextRef {
	refID := strings.Trim(strings.TrimSpace(link.To), "/")
	if refID == "" {
		refID = link.Label
	}
	return agentContextRef{
		ID:      link.Kind + ":" + refID,
		Kind:    emptyString(link.Kind, "link"),
		RefID:   refID,
		Label:   link.Label,
		To:      link.To,
		Summary: trimSummary(summary, 160),
	}
}

func agentToolError(call agentToolCall) string {
	if call.Error != nil && call.Error.Message != "" {
		return call.Error.Message
	}
	return "tool failed"
}

type agentToolFailureRecovery struct {
	Type            string
	Reason          string
	SuggestedPrompt string
	TodoTitle       string
	TodoDetail      string
	AutoContinue    bool
}

func agentRecoveryForFailedTool(language string, call agentToolCall) agentToolFailureRecovery {
	action := firstNonEmpty(stringFromAny(call.Result["action"]), stringFromAny(call.Result["action_name"]), call.Name)
	code := ""
	if call.Error != nil {
		code = string(call.Error.Code)
	}
	details := mapFromAny(call.Result["details"])
	requiredLevel := firstNonEmpty(stringFromAny(call.Result["required_operation_level"]), stringFromAny(details["required_operation_level"]))
	policy := firstNonEmpty(stringFromAny(call.Result["policy"]), stringFromAny(details["policy"]))
	reason := agentToolError(call)
	displayAction := agentRecoveryActionLabel(action, language)

	switch {
	case requiredLevel != "":
		return agentToolFailureRecovery{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "切换 Agent 操作权限到 "+requiredLevel+" 后重试："+displayAction, "switch Agent operation level to "+requiredLevel+" and retry: "+displayAction),
			TodoTitle:       agentText(language, "调整 Agent 操作权限后重试", "Adjust Agent operation level and retry"),
			TodoDetail:      reason,
		}
	case policy == "explicit_operator_intent_required":
		return agentToolFailureRecovery{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "请明确确认要执行 "+displayAction+"，并重新说明目标、范围和参数", "explicitly confirm "+displayAction+" and restate the target, scope, and parameters"),
			TodoTitle:       agentText(language, "等待明确的执行确认", "Wait for explicit execution confirmation"),
			TodoDetail:      reason,
		}
	case action == "http.request" && (strings.Contains(reason, "auth profile") || strings.Contains(reason, "Authorization") || strings.Contains(reason, "header")):
		return agentToolFailureRecovery{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "到设置页修正 API Auth Profile / Base URL 后，重新请求同一授权 URL", "fix the API auth profile / Base URL in Settings, then retry the same authorized URL"),
			TodoTitle:       agentText(language, "修正 API Auth Profile", "Fix API auth profile"),
			TodoDetail:      reason,
		}
	case code == string(services.CodeConflict) || strings.Contains(strings.ToLower(reason), "conflict"):
		return agentToolFailureRecovery{
			Type:            "continue",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "重新读取相关文件，使用当前内容 rebase 待应用变更，再预览 diff", "reread the related file, rebase the pending change with current content, then preview the diff"),
			TodoTitle:       agentText(language, "重新基于当前状态生成变更", "Regenerate change from current state"),
			TodoDetail:      reason,
			AutoContinue:    true,
		}
	case code == string(services.CodeNotFound):
		return agentToolFailureRecovery{
			Type:            "continue",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "先列出当前可用资源或工作区文件，再选择正确 ID / 路径重试", "list current resources or workspace files first, then retry with the correct ID/path"),
			TodoTitle:       agentText(language, "重新定位正确资源", "Relocate the correct resource"),
			TodoDetail:      reason,
			AutoContinue:    true,
		}
	case code == string(services.CodeToolDependencyMissing):
		return agentToolFailureRecovery{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "安装或启用缺失工具后重试，或让我改用已有内置工具", "install/enable the missing tool and retry, or ask me to use available built-in tools"),
			TodoTitle:       agentText(language, "补齐缺失工具依赖", "Install missing tool dependency"),
			TodoDetail:      reason,
		}
	case code == string(services.CodeUnsupported):
		return agentToolFailureRecovery{
			Type:            "continue",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "列出可用内置工具，并选择一个支持的替代动作", "list available built-in tools and choose a supported alternative"),
			TodoTitle:       agentText(language, "选择支持的替代工具", "Choose a supported alternative tool"),
			TodoDetail:      reason,
			AutoContinue:    true,
		}
	default:
		return agentToolFailureRecovery{
			Type:            "blocked",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "检查失败原因，调整参数后重试："+displayAction, "inspect the failure, adjust arguments, then retry: "+displayAction),
			TodoTitle:       agentText(language, "分析失败并准备重试", "Analyze failure and prepare retry"),
			TodoDetail:      reason,
		}
	}
}

func agentRecoveryActionLabel(action, language string) string {
	normalized := normalizeAgentPlannerActionName(action)
	switch normalized {
	case "workspace.files.write":
		return agentText(language, "写入文件（workspace.files.write）", "write file (workspace.files.write)")
	case "workspace.files.patch":
		return agentText(language, "直接修改文件（workspace.files.patch）", "patch file (workspace.files.patch)")
	case "workspace.files.diff", "workspace.files.diff_many":
		return agentText(language, "预览工作区变更（"+normalized+"）", "preview workspace change ("+normalized+")")
	case "workspace.files.apply":
		return agentText(language, "应用待处理变更（workspace.files.apply）", "apply pending change (workspace.files.apply)")
	case "workspace.files.discard":
		return agentText(language, "丢弃待处理变更（workspace.files.discard）", "discard pending change (workspace.files.discard)")
	case "workspace.files.delete":
		return agentText(language, "删除工作区文件（workspace.files.delete）", "delete workspace file (workspace.files.delete)")
	case "workspace.files.move":
		return agentText(language, "移动/重命名工作区文件（workspace.files.move）", "move/rename workspace file (workspace.files.move)")
	case "workspace.files.copy":
		return agentText(language, "复制工作区文件（workspace.files.copy）", "copy workspace file (workspace.files.copy)")
	case "workspace.files.mkdir":
		return agentText(language, "创建工作区目录（workspace.files.mkdir）", "create workspace directory (workspace.files.mkdir)")
	case "http.request":
		return agentText(language, "HTTP 请求（http.request）", "HTTP request (http.request)")
	case "scan.web.safe":
		return agentText(language, "safe Web/API 扫描（scan.web.safe）", "safe Web/API scan (scan.web.safe)")
	case "runs.context":
		return agentText(language, "读取运行上下文（runs.context）", "read run context (runs.context)")
	default:
		if strings.TrimSpace(action) == "" {
			return agentText(language, "该动作", "the action")
		}
		return action
	}
}

func mapFromAny(value any) map[string]any {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case map[string]any:
		return v
	default:
		return nil
	}
}

func upsertAgentRecoveryTodo(session *agentSession, call agentToolCall, recovery agentToolFailureRecovery, now time.Time) {
	title := strings.TrimSpace(recovery.TodoTitle)
	if session == nil || title == "" {
		return
	}
	id := "todo_recover_" + agentCleanToken(call.ID)
	todo := agentTodo{
		ID:            id,
		Title:         trimSummary(title, 120),
		Detail:        trimSummary(recovery.TodoDetail, 220),
		Status:        "blocked",
		Priority:      len(session.Todos) + 1,
		ToolCallID:    call.ID,
		ContextRefIDs: contextRefIDsFromToolCall(call),
		BlockedReason: trimSummary(recovery.Reason, 220),
		UpdatedAt:     now,
	}
	if recovery.AutoContinue {
		todo.Status = "pending"
		todo.BlockedReason = ""
	}
	for idx := range session.Todos {
		if session.Todos[idx].ID == id {
			session.Todos[idx] = todo
			return
		}
	}
	session.Todos = append(session.Todos, todo)
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func stringMapFromAny(value any) map[string]string {
	out := map[string]string{}
	switch v := value.(type) {
	case map[string]string:
		for key, val := range v {
			key = strings.TrimSpace(key)
			if key != "" && !strings.ContainsAny(key, "\r\n:") && !strings.ContainsAny(val, "\r\n") {
				out[key] = val
			}
		}
	case map[string]any:
		for key, val := range v {
			key = strings.TrimSpace(key)
			value := stringFromAny(val)
			if key != "" && value != "" && !strings.ContainsAny(key, "\r\n:") && !strings.ContainsAny(value, "\r\n") {
				out[key] = value
			}
		}
	}
	if len(out) > 20 {
		keys := make([]string, 0, len(out))
		for key := range out {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		trimmed := map[string]string{}
		for _, key := range keys[:20] {
			trimmed[key] = out[key]
		}
		return trimmed
	}
	return out
}

func agentRedactHTTPHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "token") || strings.Contains(lower, "key") {
			out[key] = "[redacted]"
			continue
		}
		out[key] = value
	}
	return out
}

// agentURLOutboundAllowed rejects URLs whose host resolves to non-public IP
// space (loopback, RFC1918, link-local, CGNAT, multicast, unspecified) or to
// the well-known cloud metadata endpoints. Used by web.fetch / web.search /
// http.request to keep the agent from probing the host's internal network.
func agentURLOutboundAllowed(parsed *url.URL) error {
	if parsed == nil {
		return services.NewError(services.CodeBadRequest, "missing url", nil, nil)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return services.NewError(services.CodeBadRequest, "url has no host", map[string]any{"url": parsed.String()}, nil)
	}
	switch host {
	case "localhost", "metadata", "metadata.google.internal", "metadata.goog", "instance-data":
		return services.NewError(services.CodePolicyDenied, "outbound to internal host blocked", map[string]any{"host": host}, nil)
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := agentIPOutboundAllowed(ip, host); err != nil {
			return err
		}
		return nil
	}
	resolveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil {
		return services.NewError(services.CodeBadRequest, "dns resolution failed: "+err.Error(), map[string]any{"host": host}, err)
	}
	if len(ips) == 0 {
		return services.NewError(services.CodeBadRequest, "host did not resolve", map[string]any{"host": host}, nil)
	}
	for _, addr := range ips {
		if err := agentIPOutboundAllowed(addr.IP, host); err != nil {
			return err
		}
	}
	return nil
}

func agentIPOutboundAllowed(ip net.IP, host string) error {
	if ip == nil {
		return services.NewError(services.CodeBadRequest, "invalid ip", map[string]any{"host": host}, nil)
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() || ip.IsInterfaceLocalMulticast() {
		return services.NewError(services.CodePolicyDenied, "outbound to non-public ip blocked", map[string]any{"host": host, "ip": ip.String()}, nil)
	}
	// CGNAT 100.64.0.0/10
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return services.NewError(services.CodePolicyDenied, "outbound to CGNAT ip blocked", map[string]any{"host": host, "ip": ip.String()}, nil)
	}
	// 169.254.169.254 covered by IsLinkLocalUnicast above.
	return nil
}

func agentHTTPAuthProfileEnabled(profile agentHTTPAuthProfile) bool {
	return profile.Enabled == nil || *profile.Enabled
}

func normalizeAgentHTTPAuthProfileType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "basic":
		return "basic"
	case "api_key", "apikey", "api-key":
		return "api_key"
	case "cookie":
		return "cookie"
	case "custom", "header":
		return "custom"
	default:
		return "bearer"
	}
}

func agentResolveHTTPAuthProfileHeaders(profiles []agentHTTPAuthProfile, requestedID, defaultID, targetURL string) (map[string]string, *agentHTTPAuthProfileUsage, error) {
	requestedID = strings.TrimSpace(requestedID)
	defaultID = strings.TrimSpace(defaultID)
	explicit := requestedID != ""
	profileID := requestedID
	if profileID == "" {
		profileID = defaultID
	}
	if profileID == "" || len(profiles) == 0 {
		return nil, nil, nil
	}

	profile, ok := agentFindHTTPAuthProfile(profiles, profileID)
	if !ok {
		if explicit {
			return nil, nil, services.NewError(services.CodeBadRequest, "http.request auth profile was not found", map[string]any{"auth_profile_id": profileID}, nil)
		}
		return nil, nil, nil
	}
	if !agentHTTPAuthProfileEnabled(profile) {
		if explicit {
			return nil, nil, services.NewError(services.CodePolicyDenied, "http.request auth profile is disabled", map[string]any{"auth_profile_id": profileID}, nil)
		}
		return nil, nil, nil
	}
	if !explicit && strings.TrimSpace(profile.BaseURL) == "" {
		// Default auth profiles must be host-scoped to avoid accidentally
		// attaching credentials to an unrelated URL selected by the model.
		return nil, nil, nil
	}
	matches, host := agentHTTPAuthProfileMatchesTarget(profile, targetURL)
	if !matches {
		if explicit {
			return nil, nil, services.NewError(
				services.CodePolicyDenied,
				"http.request auth profile does not match the request host",
				map[string]any{"auth_profile_id": firstNonEmpty(profile.ID, profile.Name), "base_url": profile.BaseURL, "url": targetURL},
				nil,
			)
		}
		return nil, nil, nil
	}
	headers, err := agentHTTPAuthProfileHeaders(profile)
	if err != nil {
		return nil, nil, err
	}
	if len(headers) == 0 {
		return nil, nil, nil
	}
	usage := &agentHTTPAuthProfileUsage{
		ID:      strings.TrimSpace(profile.ID),
		Name:    strings.TrimSpace(profile.Name),
		Type:    normalizeAgentHTTPAuthProfileType(profile.Type),
		BaseURL: strings.TrimSpace(profile.BaseURL),
		Host:    host,
		Applied: true,
	}
	return headers, usage, nil
}

func agentFindHTTPAuthProfile(profiles []agentHTTPAuthProfile, id string) (agentHTTPAuthProfile, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, profile := range profiles {
		if strings.ToLower(strings.TrimSpace(profile.ID)) == id || strings.ToLower(strings.TrimSpace(profile.Name)) == id {
			return profile, true
		}
	}
	return agentHTTPAuthProfile{}, false
}

func agentHTTPAuthProfileMatchesTarget(profile agentHTTPAuthProfile, targetURL string) (bool, string) {
	parsedTarget, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || parsedTarget == nil || parsedTarget.Host == "" {
		return false, ""
	}
	targetHost := strings.ToLower(parsedTarget.Hostname())
	if strings.TrimSpace(profile.BaseURL) == "" {
		return true, targetHost
	}
	base := strings.TrimSpace(profile.BaseURL)
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	parsedBase, err := url.Parse(base)
	if err != nil || parsedBase == nil || parsedBase.Host == "" {
		return false, targetHost
	}
	baseHost := strings.ToLower(parsedBase.Hostname())
	if baseHost == "" || baseHost != targetHost {
		return false, targetHost
	}
	basePort := parsedBase.Port()
	if basePort != "" && basePort != parsedTarget.Port() {
		return false, targetHost
	}
	return true, targetHost
}

func agentHTTPAuthProfileHeaders(profile agentHTTPAuthProfile) (map[string]string, error) {
	headers := map[string]string{}
	profileID := firstNonEmpty(profile.ID, profile.Name)
	switch normalizeAgentHTTPAuthProfileType(profile.Type) {
	case "bearer":
		token := strings.TrimSpace(profile.Token)
		if token == "" {
			token = strings.TrimSpace(profile.HeaderValue)
		}
		if token == "" {
			return nil, services.NewError(services.CodeBadRequest, "bearer auth profile requires token", map[string]any{"auth_profile_id": profileID}, nil)
		}
		headers["Authorization"] = "Bearer " + token
	case "basic":
		username := strings.TrimSpace(profile.Username)
		if username == "" {
			return nil, services.NewError(services.CodeBadRequest, "basic auth profile requires username", map[string]any{"auth_profile_id": profileID}, nil)
		}
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+profile.Password))
	case "api_key":
		headerName := firstNonEmpty(profile.HeaderName, "X-API-Key")
		value := firstNonEmpty(profile.HeaderValue, profile.Token)
		if value == "" {
			return nil, services.NewError(services.CodeBadRequest, "api_key auth profile requires header_value or token", map[string]any{"auth_profile_id": profileID}, nil)
		}
		headers[headerName] = value
	case "cookie":
		cookie := strings.TrimSpace(profile.Cookie)
		if cookie == "" {
			cookie = strings.TrimSpace(profile.HeaderValue)
		}
		if cookie == "" {
			return nil, services.NewError(services.CodeBadRequest, "cookie auth profile requires cookie", map[string]any{"auth_profile_id": profileID}, nil)
		}
		headers["Cookie"] = cookie
	case "custom":
		headerName := strings.TrimSpace(profile.HeaderName)
		value := strings.TrimSpace(profile.HeaderValue)
		if headerName == "" || value == "" {
			return nil, services.NewError(services.CodeBadRequest, "custom auth profile requires header_name and header_value", map[string]any{"auth_profile_id": profileID}, nil)
		}
		headers[headerName] = value
	}
	for key, value := range headers {
		if strings.ContainsAny(key, "\r\n:") || strings.ContainsAny(value, "\r\n") {
			return nil, services.NewError(services.CodeBadRequest, "auth profile contains an invalid header", map[string]any{"auth_profile_id": profileID, "header": key}, nil)
		}
	}
	return headers, nil
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
