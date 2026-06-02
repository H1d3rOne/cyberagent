package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
