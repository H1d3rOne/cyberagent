package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/command"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
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
	ApprovalState        *approvalCanonicalState    `json:"approval_state,omitempty"`
	MemorySummary        *agentMemorySummary        `json:"memory_summary,omitempty"`
	NextAction           *agentNextAction           `json:"next_action,omitempty"`
	RunSummary           *agentSessionRunSummary    `json:"run_summary,omitempty"`
	UIState              *agentUIState              `json:"ui_state,omitempty"`
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
	ApprovalState        *approvalCanonicalState    `json:"approval_state,omitempty"`
	RunSummary           *agentSessionRunSummary    `json:"run_summary,omitempty"`
	MemorySummary        *agentMemorySummary        `json:"memory_summary,omitempty"`
	UIState              *agentUIState              `json:"ui_state,omitempty"`
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
	Session       agentSession            `json:"session"`
	Message       agentMessage            `json:"message"`
	ApprovalState *approvalCanonicalState `json:"approval_state,omitempty"`
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
	ApprovalState        *approvalCanonicalState    `json:"approval_state,omitempty"`
	MemorySummary        *agentMemorySummary        `json:"memory_summary,omitempty"`
	NextAction           *agentNextAction           `json:"next_action,omitempty"`
	UIState              *agentUIState              `json:"ui_state,omitempty"`
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
		ApprovalState:        deriveAgentApprovalState(session),
		MemorySummary:        cloneAgentMemorySummary(session.MemorySummary),
		UIState:              deriveAgentUIState(session),
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
	cloned.UIState = deriveAgentUIState(&cloned)
	return cloned, nil
}
