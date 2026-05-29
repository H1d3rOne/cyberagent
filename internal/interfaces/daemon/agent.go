package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/command"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	doctorruntime "github.com/H1d3rOne/cyberagent/internal/runtime/doctor"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
	"github.com/H1d3rOne/cyberagent/internal/workspace"
)

const agentAPIVersion = "v0.1.8"

const agentSessionMaxMessages = 80

type agentManager struct {
	mu        sync.Mutex
	sessions  map[string]*agentSession
	storeDir  string
	runs      services.RunService
	approvals services.ApprovalService
	workspace *workspace.Manager
	command   *command.Executor
}

type agentSession struct {
	SessionID      string                 `json:"session_id"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Operator       string                 `json:"operator"`
	Language       string                 `json:"language"`
	Status         string                 `json:"status"`
	Messages       []agentMessage         `json:"messages"`
	ActiveTask     *agentTask             `json:"active_task,omitempty"`
	Todos          []agentTodo            `json:"todos,omitempty"`
	ToolCalls      []agentToolCall        `json:"tool_calls,omitempty"`
	ContextRefs    []agentContextRef      `json:"context_refs,omitempty"`
	PendingChanges []agentWorkspaceChange `json:"pending_changes,omitempty"`
	MemorySummary  *agentMemorySummary    `json:"memory_summary,omitempty"`
	NextAction     *agentNextAction       `json:"next_action,omitempty"`
}

type agentMessage struct {
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	Timestamp time.Time       `json:"timestamp"`
	ToolCalls []agentToolCall `json:"tool_calls,omitempty"`
	Links     []agentLink     `json:"links,omitempty"`
	Reasoning string          `json:"reasoning,omitempty"`
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

type agentContextRef struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	RefID   string `json:"ref_id"`
	Label   string `json:"label"`
	To      string `json:"to,omitempty"`
	Summary string `json:"summary,omitempty"`
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

type agentNextAction struct {
	Type            string `json:"type"`
	Reason          string `json:"reason,omitempty"`
	SuggestedPrompt string `json:"suggested_prompt,omitempty"`
	AutoContinue    bool   `json:"auto_continue,omitempty"`
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

type preparedWorkspaceApply struct {
	Change agentWorkspaceChange
	Before string
	After  string
}

type createAgentSessionRequest struct {
	Operator string `json:"operator"`
	Language string `json:"language"`
}

type createAgentSessionResponse struct {
	Session agentSession `json:"session"`
}

type agentSessionSummary struct {
	SessionID     string                  `json:"session_id"`
	CreatedAt     time.Time               `json:"created_at"`
	UpdatedAt     time.Time               `json:"updated_at"`
	Operator      string                  `json:"operator"`
	Language      string                  `json:"language"`
	Status        string                  `json:"status"`
	ActiveTask    *agentTask              `json:"active_task,omitempty"`
	NextAction    *agentNextAction        `json:"next_action,omitempty"`
	RunSummary    *agentSessionRunSummary `json:"run_summary,omitempty"`
	MemorySummary *agentMemorySummary     `json:"memory_summary,omitempty"`
	LastMessage   string                  `json:"last_message,omitempty"`
	MessageCount  int                     `json:"message_count"`
	ToolCallCount int                     `json:"tool_call_count"`
	ContextCount  int                     `json:"context_count"`
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

type sendAgentMessageRequest struct {
	Message      string `json:"message"`
	Operator     string `json:"operator"`
	Language     string `json:"language"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

type agentControlRequest struct {
	Action     string `json:"action"`
	Operator   string `json:"operator"`
	Language   string `json:"language"`
	Reason     string `json:"reason,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	JobID      string `json:"job_id,omitempty"`
	ClearInput bool   `json:"clear_input,omitempty"`
}

type agentControlResponse struct {
	Session agentSession `json:"session"`
	Message agentMessage `json:"message"`
}

type sendAgentMessageResponse struct {
	SessionID      string                 `json:"session_id"`
	Messages       []agentMessage         `json:"messages"`
	ToolCalls      []agentToolCall        `json:"tool_calls,omitempty"`
	ActiveTask     *agentTask             `json:"active_task,omitempty"`
	Todos          []agentTodo            `json:"todos,omitempty"`
	ContextRefs    []agentContextRef      `json:"context_refs,omitempty"`
	PendingChanges []agentWorkspaceChange `json:"pending_changes,omitempty"`
	MemorySummary  *agentMemorySummary    `json:"memory_summary,omitempty"`
	NextAction     *agentNextAction       `json:"next_action,omitempty"`
}

type agentCapabilitiesResponse struct {
	APIVersion   string                    `json:"api_version"`
	Name         string                    `json:"name"`
	Description  string                    `json:"description"`
	EntryPoint   string                    `json:"entry_point"`
	Endpoints    []agentCapabilityEndpoint `json:"endpoints"`
	Capabilities []agentCapability         `json:"capabilities"`
	Stages       []agentCapabilityStage    `json:"stages"`
	BuiltinTools []agentCapabilityTool     `json:"builtin_tools"`
	Safety       agentCapabilitySafety     `json:"safety"`
	Integrations []agentCapabilityItem     `json:"integrations"`
}

type agentCapability struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}

type agentCapabilityEndpoint struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

type agentCapabilityStage struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type agentCapabilityTool struct {
	Name        string          `json:"name"`
	Risk        string          `json:"risk"`
	AutoRun     bool            `json:"auto_run"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type agentCapabilitySafety struct {
	ScopePolicy       string   `json:"scope_policy"`
	AutoExecutePolicy string   `json:"auto_execute_policy"`
	ApprovalRequired  []string `json:"approval_required"`
	CommandAllowlist  []string `json:"command_allowlist"`
}

type agentCapabilityItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type agentPlannerResponse struct {
	Answer      string               `json:"answer"`
	Task        *agentPlannerTask    `json:"task,omitempty"`
	Todos       []agentPlannerTodo   `json:"todos,omitempty"`
	ContextRefs []agentContextRef    `json:"context_refs,omitempty"`
	NextAction  *agentNextAction     `json:"next_action,omitempty"`
	Reasoning   string               `json:"reasoning,omitempty"`
	Actions     []agentPlannerAction `json:"actions,omitempty"`
	ToolCalls   []agentPlannerAction `json:"tool_calls,omitempty"`
}

type agentPlannerAction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

func (a *agentPlannerAction) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name       string         `json:"name"`
		Arguments  any            `json:"arguments"`
		Args       any            `json:"args"`
		Input      any            `json:"input"`
		Parameters any            `json:"parameters"`
		Function   map[string]any `json:"function"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Name = strings.TrimSpace(raw.Name)
	args := raw.Arguments
	if args == nil {
		args = raw.Args
	}
	if args == nil {
		args = raw.Input
	}
	if args == nil {
		args = raw.Parameters
	}
	if len(raw.Function) > 0 {
		if a.Name == "" {
			a.Name = stringFromAny(raw.Function["name"])
		}
		if args == nil {
			args = raw.Function["arguments"]
		}
	}
	a.Arguments = normalizeAgentPlannerActionArguments(args)
	return nil
}

type agentPlannerTask struct {
	Goal        string `json:"goal,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Status      string `json:"status,omitempty"`
	ActiveRunID string `json:"active_run_id,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

type agentPlannerTodo struct {
	Title         string   `json:"title"`
	Detail        string   `json:"detail,omitempty"`
	Status        string   `json:"status,omitempty"`
	Priority      int      `json:"priority,omitempty"`
	ContextRefIDs []string `json:"context_ref_ids,omitempty"`
	BlockedReason string   `json:"blocked_reason,omitempty"`
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
		SessionID: strings.Replace(runs.NewJobID(now), "job_", "agent_", 1),
		CreatedAt: now,
		UpdatedAt: now,
		Operator:  normalizeAgentOperator(req.Operator),
		Language:  normalizeAgentLanguage(req.Language),
		Status:    "idle",
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

func agentCapabilities(language string) agentCapabilitiesResponse {
	language = normalizeAgentLanguage(language)
	tools := make([]agentCapabilityTool, 0, len(agentLLMToolDefinitions()))
	for _, tool := range agentLLMToolDefinitions() {
		risk, autoRun := agentCapabilityToolPolicy(tool.Name)
		tools = append(tools, agentCapabilityTool{
			Name:        tool.Name,
			Risk:        risk,
			AutoRun:     autoRun,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	return agentCapabilitiesResponse{
		APIVersion: agentAPIVersion,
		Name:       "CyberAgent",
		Description: agentText(
			language,
			"面向授权目标的 Web/API 安全分析 Agent：通过自然语言维护任务、规划步骤、调用工具、沉淀上下文并生成可复现结论。",
			"Authorized-target Web/API security analysis Agent: maintains tasks, plans steps, calls tools, preserves context, and produces reproducible conclusions through natural language.",
		),
		EntryPoint: "/api/v1/agent/sessions/{session_id}/messages",
		Endpoints: []agentCapabilityEndpoint{
			{Method: "POST", Path: "/api/v1/agent/sessions", Description: agentText(language, "创建 Agent 会话。", "Create an Agent session.")},
			{Method: "GET", Path: "/api/v1/agent/sessions", Description: agentText(language, "列出 Agent 会话摘要。", "List Agent session summaries.")},
			{Method: "GET", Path: "/api/v1/agent/sessions/{session_id}", Description: agentText(language, "读取完整 Agent 会话。", "Read the full Agent session.")},
			{Method: "GET", Path: "/api/v1/agent/sessions/{session_id}/state", Description: agentText(language, "读取可恢复 Agent state：task、todos、tool calls、context refs、next action。", "Read recoverable Agent state: task, todos, tool calls, context refs, and next action.")},
			{Method: "GET", Path: "/api/v1/agent/sessions/{session_id}/context", Description: agentText(language, "读取轻量工作上下文：context refs、pending changes 和 run summary。", "Read lightweight working context: context refs, pending changes, and run summary.")},
			{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/messages", Description: agentText(language, "发送自然语言消息并触发 Agent 执行循环。", "Send a natural-language message and trigger the Agent execution loop.")},
			{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/control", Description: agentText(language, "通用控制入口：continue、revise、stop、new_task。", "Generic control entrypoint: continue, revise, stop, new_task.")},
			{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/continue", Description: agentText(language, "继续当前任务。", "Continue the current task.")},
			{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/stop", Description: agentText(language, "停止或暂停当前任务。", "Stop or pause the current task.")},
			{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/new-task", Description: agentText(language, "开启新的任务上下文。", "Start a new task context.")},
			{Method: "GET", Path: "/api/v1/agent/sessions/{session_id}/stream", Description: agentText(language, "订阅 Agent session/activity 事件流。", "Subscribe to Agent session/activity events.")},
		},
		Capabilities: []agentCapability{
			{
				ID:          "natural_language_ops",
				Label:       agentText(language, "自然语言运维/测试", "Natural-language ops/testing"),
				Description: agentText(language, "将操作者意图转为任务、待办、工具调用和下一步建议。", "Turns operator intent into tasks, todos, tool calls, and next-step suggestions."),
				Examples: []string{
					agentText(language, "检查 daemon 状态", "check daemon status"),
					agentText(language, "列出最近运行并总结失败项", "list recent runs and summarize failures"),
				},
			},
			{
				ID:          "agent_memory",
				Label:       agentText(language, "会话与上下文保持", "Session/context memory"),
				Description: agentText(language, "保持聊天会话、任务状态、工具轨迹、上下文引用和下一步动作。", "Keeps chat sessions, task state, tool traces, context references, and next actions."),
			},
			{
				ID:          "planner_tool_loop",
				Label:       agentText(language, "规划→工具→总结闭环", "Planner→tools→synthesis loop"),
				Description: agentText(language, "支持 JSON actions 与模型原生 tool_calls，执行低风险工具后回灌 observation 并生成最终总结。", "Supports JSON actions and native LLM tool_calls; after low-risk tool execution, observations are fed back for final synthesis."),
			},
			{
				ID:          "auto_continue_stepper",
				Label:       agentText(language, "自动续行步进器", "Auto-continue stepper"),
				Description: agentText(language, "当 next_action 标记为 auto_continue 且下一步是低风险只读动作时，“继续”会直接执行下一步工具并写回 observation。", "When next_action is marked auto_continue and the next step is a low-risk read-only action, Continue directly executes the next tool and persists the observation."),
			},
			{
				ID:          "web_api_security_runtime",
				Label:       agentText(language, "Web/API 安全运行时", "Web/API security runtime"),
				Description: agentText(language, "可按明确授权指令启动 safe profile 扫描、跟踪运行、收集证据、处理审批并复测。", "Can start safe-profile scans on explicit authorized instructions, track runs, collect evidence, handle approvals, and retest."),
			},
			{
				ID:          "workspace_tools",
				Label:       agentText(language, "Agent 工作区", "Agent workspace"),
				Description: agentText(language, "为每个会话提供隔离工作区，支持文件列表、读取和低风险白名单命令。", "Provides an isolated workspace per session with file listing, reading, and low-risk allowlisted commands."),
			},
		},
		Stages: []agentCapabilityStage{
			{ID: "understand", Label: agentText(language, "理解", "Understand"), Description: agentText(language, "解析目标、scope、授权边界和操作者意图。", "Parse target, scope, authorization boundary, and operator intent.")},
			{ID: "plan", Label: agentText(language, "规划", "Plan"), Description: agentText(language, "生成 task/todos/next_action，并判断是否需要工具或人工确认。", "Generate task/todos/next_action and decide whether tools or human confirmation are needed.")},
			{ID: "tool_call", Label: agentText(language, "工具调用", "Tool call"), Description: agentText(language, "执行低风险内置工具；扫描和高风险动作需明确指令或审批。", "Execute low-risk built-ins; scans and higher-risk actions require explicit instruction or approval.")},
			{ID: "observation", Label: agentText(language, "观测", "Observation"), Description: agentText(language, "把工具结果沉淀为消息、工具轨迹和上下文引用。", "Persist tool results as messages, tool traces, and context references.")},
			{ID: "synthesis", Label: agentText(language, "总结", "Synthesis"), Description: agentText(language, "把 observation 回灌给 LLM，生成最终结论和下一步。", "Feed observations back to the LLM to produce final conclusions and next steps.")},
		},
		BuiltinTools: tools,
		Safety: agentCapabilitySafety{
			ScopePolicy: agentText(
				language,
				"默认只处理授权目标；目标、scope 或授权边界不清晰时等待用户确认。",
				"Only authorized targets are handled by default; if target, scope, or authorization boundary is unclear, wait for user confirmation.",
			),
			AutoExecutePolicy: agentText(
				language,
				"LLM 自动执行仅限低风险内置工具；扫描需明确自然语言指令；外部插件/高风险动作进入审批 gate。",
				"LLM auto-execution is limited to low-risk built-ins; scans need explicit natural-language instructions; external plugins/high-risk actions go through an approval gate.",
			),
			ApprovalRequired: []string{
				"external_plugins",
				"high_risk_commands",
				"out_of_scope_targets",
				"destructive_or_state_changing_actions",
			},
			CommandAllowlist: agentPlannerCommandAllowlist(),
		},
		Integrations: []agentCapabilityItem{
			{ID: "llm_providers", Label: "LLM providers", Description: "OpenAI-compatible, Anthropic-compatible, and Ollama provider adapters."},
			{ID: "daemon_runtime", Label: "Daemon runtime", Description: "Jobs, runs, events, reports, approvals, and safe-profile Web/API scan runtime."},
			{ID: "plugins", Label: "Plugin registry", Description: "Built-in and external tool/plugin registry with policy gates."},
			{ID: "workspace", Label: "Workspace", Description: "Per-session files and allowlisted command execution sandbox."},
		},
	}
}

func agentCapabilityToolPolicy(name string) (string, bool) {
	switch normalizeAgentPlannerActionName(name) {
	case "runs.approvals.approve", "runs.approvals.deny", "runs.resume_approved":
		return "medium", false
	default:
		return "low", true
	}
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
		SessionID:     session.SessionID,
		CreatedAt:     session.CreatedAt,
		UpdatedAt:     session.UpdatedAt,
		Operator:      session.Operator,
		Language:      session.Language,
		Status:        session.Status,
		ActiveTask:    cloneAgentTask(session.ActiveTask),
		NextAction:    cloneAgentNextAction(session.NextAction),
		MemorySummary: cloneAgentMemorySummary(session.MemorySummary),
		LastMessage:   lastMessage,
		MessageCount:  len(session.Messages),
		ToolCallCount: len(session.ToolCalls),
		ContextCount:  len(session.ContextRefs),
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
	return cloneAgentSession(session), nil
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
	previousNextAction := cloneAgentNextAction(session.NextAction)

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
	case "stop":
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
	compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	if err := m.persistSessionLocked(session); err != nil {
		m.mu.Unlock()
		return agentControlResponse{}, err
	}
	sessionState := cloneAgentSession(session)
	m.mu.Unlock()

	if action == "stop" && cancelJobID != "" && svc.jobs != nil {
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

func (m *agentManager) SendMessage(ctx context.Context, sessionID string, req sendAgentMessageRequest, svc routerServices) (sendAgentMessageResponse, error) {
	if err := contextError(ctx); err != nil {
		return sendAgentMessageResponse{}, err
	}
	sessionID, err := normalizeAgentSessionID(sessionID)
	if err != nil {
		return sendAgentMessageResponse{}, err
	}
	prompt := strings.TrimSpace(req.Message)
	if prompt == "" {
		return sendAgentMessageResponse{}, services.NewError(services.CodeBadRequest, "message is required", map[string]any{"field": "message"}, nil)
	}
	operator := normalizeAgentOperator(req.Operator)
	language := normalizeAgentLanguage(req.Language)

	var history []agentMessage
	var memorySummary *agentMemorySummary
	userMessage := agentMessage{}
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
	userMessage = m.newMessageLocked(language, "user", prompt, nil, nil)
	session.Messages = append(session.Messages, userMessage)
	session.UpdatedAt = userMessage.Timestamp
	if m.shouldStartNewAgentTaskLocked(session, prompt, language) {
		m.startAgentTaskLocked(session, prompt, language)
	} else {
		m.touchAgentTaskForMessageLocked(session, prompt, language)
	}
	compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	history = cloneAgentMessages(session.Messages)
	memorySummary = cloneAgentMemorySummary(session.MemorySummary)
	_ = m.persistSessionLocked(session)
	m.mu.Unlock()

	if controlReq, ok := agentControlRequestFromPrompt(prompt, operator, language); ok {
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
				autoMessages, autoToolCalls, routeErr := m.routePrompt(ctx, sessionID, autoPrompt, language, operator, svc)
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
			messages, toolCalls, plan, err := m.handleAgentLLMChat(ctx, language, req, history, memorySummary)
			if err == nil {
				m.appendMessagesWithPlanner(sessionID, plan, messages...)
				actionMessages, actionToolCalls := m.executeAgentPlannerActions(ctx, sessionID, prompt, language, operator, svc, plan)
				if len(actionMessages) > 0 {
					m.appendMessages(sessionID, actionMessages...)
				}
				responseMessages := append([]agentMessage{userMessage, controlResp.Message}, messages...)
				responseMessages = append(responseMessages, actionMessages...)
				toolCalls = append(toolCalls, actionToolCalls...)

				if len(actionMessages) > 0 {
					synthesisHistory := append(cloneAgentMessages(history), messages...)
					synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.synthesizeAgentToolResults(ctx, language, req, synthesisHistory, memorySummary, actionMessages)
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

	if agentLLMConfigured(req) && !agentNeedsDaemonTool(prompt) {
		messages, toolCalls, plan, err := m.handleAgentLLMChat(ctx, language, req, history, memorySummary)
		if err == nil {
			m.appendMessagesWithPlanner(sessionID, plan, messages...)
			actionMessages, actionToolCalls := m.executeAgentPlannerActions(ctx, sessionID, prompt, language, operator, svc, plan)
			if len(actionMessages) > 0 {
				m.appendMessages(sessionID, actionMessages...)
			}
			responseMessages := append([]agentMessage{userMessage}, messages...)
			responseMessages = append(responseMessages, actionMessages...)
			toolCalls = append(toolCalls, actionToolCalls...)

			if len(actionMessages) > 0 {
				synthesisHistory := append(cloneAgentMessages(history), messages...)
				synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.synthesizeAgentToolResults(ctx, language, req, synthesisHistory, memorySummary, actionMessages)
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
		messages, toolCalls, routeErr := m.routePrompt(ctx, sessionID, prompt, language, operator, svc)
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

	messages, toolCalls, err := m.routePrompt(ctx, sessionID, prompt, language, operator, svc)
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
		synthesisMessages, synthesisToolCalls, synthesisPlan, synthesisErr := m.synthesizeAgentToolResults(ctx, language, req, synthesisHistory, memorySummary, messages)
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

func (m *agentManager) handleAgentLLMChat(ctx context.Context, language string, req sendAgentMessageRequest, history []agentMessage, memory *agentMemorySummary) ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
	providerName := normalizeAgentProvider(req.Provider)
	provider := buildAgentLLMProvider(providerName, llm.ProviderConfig{
		APIKey:  strings.TrimSpace(req.APIKey),
		BaseURL: strings.TrimSpace(req.BaseURL),
		Model:   strings.TrimSpace(req.Model),
	})
	tool := runningAgentToolCall("LLM chat", provider.Name())
	chatCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	response, err := provider.Chat(chatCtx, llm.ChatRequest{
		Model:       strings.TrimSpace(req.Model),
		System:      agentPlannerSystemPrompt(language, req.Instructions),
		Messages:    agentLLMHistory(history, memory, language),
		Tools:       agentLLMToolDefinitions(),
		MaxTokens:   1600,
		Temperature: 0.2,
	})
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.CodeBadRequest, Message: err.Error()}
		return nil, []agentToolCall{tool}, nil, err
	}
	tool.Result = map[string]any{
		"provider":      provider.Name(),
		"model":         strings.TrimSpace(req.Model),
		"input_tokens":  response.Usage.InputTokens,
		"output_tokens": response.Usage.OutputTokens,
	}
	content := agentLLMText(response)
	plan, parseErr := parseAgentPlannerResponse(content)
	if parseErr == nil && plan != nil {
		tool.Result["planner"] = true
		tool.Result["planner_todos"] = len(plan.Todos)
		tool.Result["planner_actions"] = len(agentPlannerActions(plan))
		if strings.TrimSpace(plan.Answer) != "" {
			content = strings.TrimSpace(plan.Answer)
		} else if content != "" {
			plan.Answer = content
		}
	} else {
		tool.Result["planner"] = false
		if parseErr != nil {
			tool.Result["planner_parse_error"] = parseErr.Error()
		}
	}
	nativeActions := agentPlannerActionsFromLLMToolUse(response)
	if len(nativeActions) > 0 {
		if plan == nil {
			plan = &agentPlannerResponse{}
		}
		plan.ToolCalls = append(plan.ToolCalls, nativeActions...)
		tool.Result["planner_native_tool_calls"] = len(nativeActions)
		tool.Result["planner_actions"] = len(agentPlannerActions(plan))
	}
	if strings.TrimSpace(content) == "" {
		if len(nativeActions) > 0 {
			content = agentText(language, "我将先调用低风险内置工具检查当前上下文。", "I will first call low-risk built-in tools to inspect the current context.")
			plan.Answer = content
		} else {
			content = agentText(language, "AI provider 返回了空内容。", "The AI provider returned empty content.")
		}
	}
	message := m.message(language, "agent", content, []agentToolCall{tool}, nil)
	if plan != nil {
		message.Reasoning = trimSummary(plan.Reasoning, 800)
	}
	return []agentMessage{message}, []agentToolCall{tool}, plan, nil
}

func (m *agentManager) synthesizeAgentToolResults(ctx context.Context, language string, req sendAgentMessageRequest, history []agentMessage, memory *agentMemorySummary, toolMessages []agentMessage) ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
	if len(toolMessages) == 0 {
		return nil, nil, nil, nil
	}

	providerName := normalizeAgentProvider(req.Provider)
	provider := buildAgentLLMProvider(providerName, llm.ProviderConfig{
		APIKey:  strings.TrimSpace(req.APIKey),
		BaseURL: strings.TrimSpace(req.BaseURL),
		Model:   strings.TrimSpace(req.Model),
	})
	tool := runningAgentToolCall("LLM synthesis", provider.Name())
	tool = withAgentToolMetadata(tool, map[string]interface{}{"phase": "tool_synthesis"})

	chatCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	response, err := provider.Chat(chatCtx, llm.ChatRequest{
		Model:       strings.TrimSpace(req.Model),
		System:      agentToolSynthesisSystemPrompt(language, req.Instructions),
		Messages:    agentLLMToolSynthesisHistory(history, memory, toolMessages, language),
		MaxTokens:   1200,
		Temperature: 0.2,
	})
	tool = finishAgentToolCall(tool, err)
	if err != nil {
		tool.Error = &daemonJobError{Code: services.CodeBadRequest, Message: err.Error()}
		return nil, []agentToolCall{tool}, nil, err
	}

	tool.Result = map[string]any{
		"provider":      provider.Name(),
		"model":         strings.TrimSpace(req.Model),
		"input_tokens":  response.Usage.InputTokens,
		"output_tokens": response.Usage.OutputTokens,
		"phase":         "tool_synthesis",
	}

	content := agentLLMText(response)
	plan, parseErr := parseAgentPlannerResponse(content)
	if parseErr == nil && plan != nil {
		tool.Result["planner"] = true
		tool.Result["planner_todos"] = len(plan.Todos)
		if ignored := len(agentPlannerActions(plan)); ignored > 0 {
			tool.Result["planner_actions_ignored"] = ignored
			plan.Actions = nil
			plan.ToolCalls = nil
		}
		if strings.TrimSpace(plan.Answer) != "" {
			content = strings.TrimSpace(plan.Answer)
		} else if content != "" {
			plan.Answer = content
		}
	} else {
		tool.Result["planner"] = false
		if parseErr != nil {
			tool.Result["planner_parse_error"] = parseErr.Error()
		}
	}
	if strings.TrimSpace(content) == "" {
		content = agentText(language, "工具执行完成，但 AI provider 返回了空总结。", "Tools executed, but the AI provider returned an empty synthesis.")
	}

	message := m.message(language, "agent", content, []agentToolCall{tool}, nil)
	if plan != nil {
		message.Reasoning = trimSummary(plan.Reasoning, 800)
	}
	return []agentMessage{message}, []agentToolCall{tool}, plan, nil
}

func (m *agentManager) executeAgentPlannerActions(ctx context.Context, sessionID, operatorPrompt, language, operator string, svc routerServices, plan *agentPlannerResponse) ([]agentMessage, []agentToolCall) {
	actions := agentPlannerActions(plan)
	if len(actions) == 0 {
		return nil, nil
	}
	if len(actions) > 6 {
		actions = actions[:6]
	}

	var messages []agentMessage
	var toolCalls []agentToolCall
	queueID := strings.Replace(runs.NewJobID(time.Now().UTC()), "job_", "queue_", 1)
	actionCount := len(actions)
	for idx, action := range actions {
		actionIndex := idx + 1
		actionName := normalizeAgentPlannerActionName(action.Name)
		if actionName == "" {
			actionName = strings.TrimSpace(action.Name)
		}
		var actionMessages []agentMessage
		var actionToolCalls []agentToolCall
		var err error
		if !agentPlannerActionAllowedByPrompt(operatorPrompt, actionName) {
			err = services.NewError(
				services.CodePolicyDenied,
				"planner action requires explicit operator intent in the latest user message",
				map[string]any{
					"action": actionName,
					"policy": "explicit_operator_intent_required",
				},
				nil,
			)
		} else {
			actionMessages, actionToolCalls, err = m.executeAgentPlannerAction(ctx, sessionID, language, operator, svc, action)
		}
		if err != nil {
			if actionName == "" {
				actionName = "agent.action"
			}
			tool := failedAgentToolCall(actionName, err)
			tool.Result = map[string]any{
				"action": actionName,
			}
			var serviceErr *services.ServiceError
			if errors.As(err, &serviceErr) && len(serviceErr.Details) > 0 {
				tool.Result["details"] = serviceErr.Details
				for key, value := range serviceErr.Details {
					if key == "action" {
						continue
					}
					tool.Result[key] = value
				}
			}
			actionMessages = []agentMessage{m.message(language, "tool", err.Error(), []agentToolCall{tool}, nil)}
			actionToolCalls = []agentToolCall{tool}
		}
		actionMetadata := map[string]interface{}{
			"action_queue_id": queueID,
			"action_index":    actionIndex,
			"action_count":    actionCount,
			"action_name":     actionName,
		}
		for i := range actionToolCalls {
			actionToolCalls[i] = withAgentToolMetadata(actionToolCalls[i], actionMetadata)
		}
		for i := range actionMessages {
			for j := range actionMessages[i].ToolCalls {
				actionMessages[i].ToolCalls[j] = withAgentToolMetadata(actionMessages[i].ToolCalls[j], actionMetadata)
			}
		}
		messages = append(messages, actionMessages...)
		toolCalls = append(toolCalls, actionToolCalls...)
	}
	return messages, toolCalls
}

func (m *agentManager) executeAgentPlannerAction(ctx context.Context, sessionID, language, operator string, svc routerServices, action agentPlannerAction) ([]agentMessage, []agentToolCall, error) {
	name := normalizeAgentPlannerActionName(action.Name)
	args := action.Arguments
	if args == nil {
		args = map[string]any{}
	}

	switch name {
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
	case "daemon.health":
		return m.handleAgentHealth(ctx, language, svc)
	case "plugins.list":
		return m.handleAgentPlugins(ctx, language, svc)
	case "runs.list":
		return m.handleAgentRuns(ctx, language, svc)
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
	default:
		return nil, nil, services.NewError(services.CodeUnsupported, "unsupported planner action", map[string]any{"action": action.Name}, nil)
	}
}

func (m *agentManager) routePrompt(ctx context.Context, sessionID, prompt, language, operator string, svc routerServices) ([]agentMessage, []agentToolCall, error) {
	switch {
	case agentIsResumeApprovedIntent(prompt):
		return m.handleAgentResumeApproved(ctx, sessionID, prompt, language, operator, svc)
	case agentIsApprovalDecisionIntent(prompt):
		return m.handleAgentApprovalDecision(ctx, sessionID, prompt, language, operator, svc)
	case agentIsRunContextAnalysisIntent(prompt):
		return m.handleAgentRunContextAnalysis(ctx, prompt, language, svc)
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
	if agentIsControlIntent(prompt) || agentIsScanIntent(prompt) || agentIsResumeApprovedIntent(prompt) || agentIsApprovalDecisionIntent(prompt) {
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
		return (agentIsWorkspaceListIntent(prompt) || agentIsWorkspaceReadIntent(prompt) || agentIsWorkspacePrepareOnlyIntent(prompt)) &&
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
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
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
	compactAgentSessionMessagesLocked(session, agentSessionMaxMessages)
	_ = m.persistSessionLocked(session)
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

func normalizeAgentControlAction(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "continue", "resume", "next", "next_step", "next-step", "proceed", "继续", "下一步", "继续执行":
		return "continue"
	case "revise", "revision", "adjust", "update_plan", "update-plan", "修订", "修改计划", "调整计划", "更新计划":
		return "revise"
	case "stop", "pause", "cancel", "停止", "暂停", "取消":
		return "stop"
	case "new_task", "new-task", "new", "reset", "新任务", "重置", "清空任务":
		return "new_task"
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
	case "continue", "revise", "stop", "new_task":
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
	if regexp.MustCompile(`(?i)^\s*/?(stop|pause|cancel)\b`).MatchString(prompt) ||
		regexp.MustCompile(`^\s*/?(停止任务|停止当前任务|停止|暂停任务|暂停|取消任务|取消)(?:\s|[:：-]|$)`).MatchString(prompt) {
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
	if remainder != "" && action == "stop" {
		return trimSummary(remainder, 260)
	}
	switch action {
	case "continue":
		return agentText(language, "用户通过聊天请求继续当前任务。", "The operator requested continuing the current task from chat.")
	case "revise":
		return agentText(language, "用户通过聊天请求修订当前任务计划。", "The operator requested revising the current task plan from chat.")
	case "stop":
		return agentText(language, "用户通过聊天请求停止当前任务。", "The operator requested stopping the current task from chat.")
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

func agentLLMConfigured(req sendAgentMessageRequest) bool {
	provider := normalizeAgentProvider(req.Provider)
	if provider == "ollama" {
		return true
	}
	return strings.TrimSpace(req.APIKey) != "" || strings.TrimSpace(req.BaseURL) != ""
}

func agentNeedsDaemonTool(prompt string) bool {
	return agentIsControlIntent(prompt) ||
		agentIsResumeApprovedIntent(prompt) ||
		agentIsRunContextAnalysisIntent(prompt) ||
		agentIsScanIntent(prompt) ||
		agentIsApprovalIntent(prompt) ||
		agentIsHealthIntent(prompt) ||
		agentIsPluginIntent(prompt) ||
		agentIsRunIntent(prompt) ||
		agentIsWorkspaceIntent(prompt) ||
		agentIsCommandIntent(prompt)
}

func normalizeAgentProvider(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "anthropic", "anthropic-compatible":
		return "anthropic"
	case "openai", "openai-compatible":
		return "openai"
	case "ollama":
		return "ollama"
	default:
		return value
	}
}

func buildAgentLLMProvider(name string, cfg llm.ProviderConfig) llm.Provider {
	var provider llm.Provider
	effectiveName := normalizeAgentProvider(name)
	if effectiveName != "openai" && effectiveName != "ollama" && effectiveName != "anthropic" && strings.TrimSpace(cfg.BaseURL) != "" {
		effectiveName = "openai"
	}
	cfg.BaseURL = normalizeAgentProviderBaseURL(effectiveName, cfg.BaseURL)
	switch effectiveName {
	case "openai":
		provider = llm.NewOpenAI(cfg)
	case "ollama":
		provider = llm.NewOllama(cfg)
	case "anthropic":
		provider = llm.NewAnthropic(cfg)
	default:
		if strings.TrimSpace(cfg.BaseURL) == "" {
			provider = llm.NewAnthropic(cfg)
		} else {
			provider = llm.NewOpenAI(cfg)
		}
	}
	return llm.WithRetry(provider, llm.DefaultRetryConfig())
}

func normalizeAgentProviderBaseURL(provider, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	trimmedPath := strings.TrimRight(parsed.Path, "/")

	switch normalizeAgentProvider(provider) {
	case "openai", "ollama":
		if strings.HasSuffix(strings.ToLower(trimmedPath), "/chat/completions") {
			return raw
		}
		switch trimmedPath {
		case "":
			parsed.Path = "/v1/chat/completions"
			return parsed.String()
		case "/v1":
			parsed.Path = "/v1/chat/completions"
			return parsed.String()
		default:
			return raw
		}
	case "anthropic":
		if strings.HasSuffix(strings.ToLower(trimmedPath), "/messages") {
			return raw
		}
		switch trimmedPath {
		case "":
			parsed.Path = "/v1/messages"
			return parsed.String()
		case "/v1":
			parsed.Path = "/v1/messages"
			return parsed.String()
		default:
			return raw
		}
	default:
		return raw
	}
}

func agentSystemPrompt(language, instructions string) string {
	base := agentText(
		language,
		"你是 CyberAgent，一个面向授权目标的 Web/API 安全分析 Agent。你通过聊天帮助操作者澄清目标、scope、策略、风险、证据、修复和复测步骤。回答要简洁、可执行、偏安全工程实践。不要声称已经执行未执行的扫描、审批或工具动作；如果用户需要真实执行，让用户使用扫描、体检、工具、运行或审批类指令，由 Runtime 工具分支完成。默认只讨论和处理已授权目标。",
		"You are CyberAgent, an authorized-target Web/API security analysis Agent. Through chat, help the operator clarify target, scope, policy, risk, evidence, remediation and retest steps. Be concise, actionable, and grounded in security engineering practice. Do not claim that scans, approvals, or tool actions were executed unless they actually were; if real execution is needed, ask the user to use scan, doctor, tools, runs, or approvals instructions so the Runtime tool branch can perform it. Only discuss and handle authorized targets by default.",
	)
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return base
	}
	return base + "\n\n" + agentText(language, "用户附加指令：", "Additional user instructions:") + "\n" + instructions
}

func agentPlannerSystemPrompt(language, instructions string) string {
	base := agentText(
		language,
		`你是 CyberAgent，一个面向授权目标的 Web/API 安全分析 Agent。你不是普通聊天机器人；你需要像 Claude Code / Codex 一样维护当前任务、计划、上下文和下一步动作。

只输出一个 JSON 对象，不要 Markdown，不要代码围栏，不要在 JSON 前后输出解释文本。JSON schema：
{
  "answer": "给操作者看的简洁回复，说明你理解了什么、计划了什么、还需要什么",
  "task": {
    "goal": "当前任务目标",
    "kind": "chat|triage|planning|ops|scan|context_analysis|approval|tools|runs|health",
    "status": "running|blocked|done|idle",
    "summary": "一句话任务摘要"
  },
  "todos": [
    {
      "title": "短任务标题",
      "detail": "可选细节",
      "status": "pending|running|done|blocked|skipped",
      "priority": 1,
      "context_ref_ids": [],
      "blocked_reason": ""
    }
  ],
  "context_refs": [],
  "actions": [
    {
      "name": "workspace.command|workspace.files.list|workspace.files.read|workspace.files.write|workspace.files.patch|workspace.files.diff|workspace.files.diff_many|workspace.files.rebase|workspace.files.apply|workspace.files.discard|daemon.health|plugins.list|runs.list|runs.approvals.list|runs.approvals.approve|runs.approvals.deny|runs.resume_approved",
      "arguments": {"path": ".", "command": "pwd", "content": "notes", "old": "before", "new": "after", "changes": [{"path": "a.md", "old": "before", "new": "after"}], "change_id": "change_xxx", "group_id": "changeset_xxx", "all": true, "run_id": "job_xxx", "approval_id": "approval_xxx", "step_id": "step_xxx", "comment": "operator comment"}
    }
  ],
  "tool_calls": [],
  "next_action": {
    "type": "continue|wait_user|approval_required|finished|blocked",
    "reason": "为什么下一步这样做",
    "suggested_prompt": "操作者可直接发送的下一步提示",
    "auto_continue": true|false
  },
  "reasoning": "Agent 的思考过程：为什么选择这个任务、为什么这样规划、为什么这个下一步"
}

约束：
- 默认只讨论和处理已授权目标；如 scope、授权边界或目标不清晰，应把下一步设为 wait_user。
- 不要声称已经执行未执行的扫描、审批、命令或工具动作。
- auto_continue：当任务尚未完成、有明确下一步、且不需要人工审批或确认时设为 true；当任务已完成、需要人工输入、或存在不确定风险时设为 false。
- reasoning：简要说明你的思考过程，包括任务选择理由、规划理由、下一步理由和风险考虑。
- 可直接请求内置工具：workspace.command、workspace.files.list、workspace.files.read、workspace.files.write、workspace.files.patch、workspace.files.diff、workspace.files.diff_many、workspace.files.rebase、workspace.files.apply、workspace.files.discard、daemon.health、plugins.list、runs.list、runs.approvals.list、runs.approvals.approve、runs.approvals.deny、runs.resume_approved。只有用户明确要求且动作属于这些内置工具时，才写入 actions 或 tool_calls；approval approve/deny 与 resume-approved 必须来自最新用户消息的显式指令；扫描、外部插件和高风险命令不要自动执行，应等待用户确认或建议使用对应 Runtime 指令。
- todos 要体现 Agent 工作台能力：理解目标、确认 scope、选择安全策略、执行/建议 Runtime 动作、收集证据、总结报告、复测。
- answer 使用中文。`,
		`You are CyberAgent, an authorized-target Web/API security analysis Agent. You are not a generic chatbot; maintain the current task, plan, context, and next action like Claude Code / Codex.

Return exactly one JSON object. Do not use Markdown, code fences, or text before/after the JSON. JSON schema:
{
  "answer": "Concise operator-facing reply: what you understood, planned, and still need",
  "task": {
    "goal": "Current task goal",
    "kind": "chat|triage|planning|ops|scan|context_analysis|approval|tools|runs|health",
    "status": "running|blocked|done|idle",
    "summary": "One-sentence task summary"
  },
  "todos": [
    {
      "title": "Short task title",
      "detail": "Optional detail",
      "status": "pending|running|done|blocked|skipped",
      "priority": 1,
      "context_ref_ids": [],
      "blocked_reason": ""
    }
  ],
  "context_refs": [],
  "actions": [
    {
      "name": "workspace.command|workspace.files.list|workspace.files.read|workspace.files.write|workspace.files.patch|workspace.files.diff|workspace.files.diff_many|workspace.files.rebase|workspace.files.apply|workspace.files.discard|daemon.health|plugins.list|runs.list|runs.approvals.list|runs.approvals.approve|runs.approvals.deny|runs.resume_approved",
      "arguments": {"path": ".", "command": "pwd", "content": "notes", "old": "before", "new": "after", "changes": [{"path": "a.md", "old": "before", "new": "after"}], "change_id": "change_xxx", "group_id": "changeset_xxx", "all": true, "run_id": "job_xxx", "approval_id": "approval_xxx", "step_id": "step_xxx", "comment": "operator comment"}
    }
  ],
  "tool_calls": [],
  "next_action": {
    "type": "continue|wait_user|approval_required|finished|blocked",
    "reason": "Why this is the next step",
    "suggested_prompt": "A prompt the operator can send directly",
    "auto_continue": true|false
  },
  "reasoning": "Agent reasoning: why this task, why this plan, why this next step"
}

Constraints:
- Only discuss and handle authorized targets by default; if scope, authorization boundaries, or target are unclear, set next_action.type to wait_user.
- Do not claim that scans, approvals, commands, or tool actions were executed unless they actually were.
- auto_continue: set to true when the task is not finished, there is a clear next step, and no human approval or confirmation is needed; set to false when the task is complete, human input is required, or there is uncertain risk.
- reasoning: briefly explain your thinking process, including task selection reason, planning reason, next step reason, and risk considerations.
- You may request built-in tools directly: workspace.command, workspace.files.list, workspace.files.read, workspace.files.write, workspace.files.patch, workspace.files.diff, workspace.files.diff_many, workspace.files.rebase, workspace.files.apply, workspace.files.discard, daemon.health, plugins.list, runs.list, runs.approvals.list, runs.approvals.approve, runs.approvals.deny, runs.resume_approved. Only populate actions or tool_calls when the user clearly asks for it and the action is one of these built-ins; approval approve/deny and resume-approved must come from an explicit instruction in the latest user message; do not auto-run scans, external plugins, or high-risk commands.
- todos should make the Agent cockpit useful: understand target, confirm scope, choose safety policy, execute/suggest Runtime action, collect evidence, summarize report, retest.
- answer must be in English.`,
	)
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return base
	}
	return base + "\n\n" + agentText(language, "操作者附加指令：", "Additional operator instructions:") + "\n" + instructions
}

func agentToolSynthesisSystemPrompt(language, instructions string) string {
	base := agentText(
		language,
		`你是 CyberAgent，一个面向授权目标的 Web/API 安全分析 Agent。你刚刚请求的低风险内置工具已经执行完毕；现在你要读取 observation，给操作者一个最终总结和下一步建议。

只输出一个 JSON 对象，不要 Markdown，不要代码围栏，不要在 JSON 前后输出解释文本。JSON schema：
{
  "answer": "简洁总结：执行了哪些工具、关键结果是什么、下一步怎么做",
  "task": {
    "goal": "当前任务目标",
    "kind": "chat|triage|planning|ops|scan|context_analysis|approval|tools|runs|health",
    "status": "running|blocked|done|idle",
    "summary": "一句话任务摘要"
  },
  "todos": [
    {
      "title": "短任务标题",
      "detail": "可选细节",
      "status": "pending|running|done|blocked|skipped",
      "priority": 1,
      "context_ref_ids": [],
      "blocked_reason": ""
    }
  ],
  "context_refs": [],
  "actions": [],
  "tool_calls": [],
	  "next_action": {
	    "type": "continue|wait_user|approval_required|finished|blocked",
	    "reason": "为什么下一步这样做",
	    "suggested_prompt": "操作者可直接发送的下一步提示",
	    "auto_continue": true|false
	  },
	  "reasoning": "简要说明如何根据工具结果得出总结和下一步"
	}

约束：
- 这是工具结果 synthesis 阶段；不要再请求或声明执行新的 actions/tool_calls。即使你输出 actions，系统也会忽略。
	- 只能基于 observation 总结，不要编造未出现的命令输出、扫描结果或漏洞。
	- 如工具失败，要说明失败点和可恢复步骤。
	- auto_continue：当 observation 已给出足够上下文、任务仍未完成、有明确的低风险下一步、且不需要人工确认/审批时可设为 true；如果下一步需要用户补充目标、scope、凭据、授权边界、审批或风险确认，必须设为 false。
	- 默认只讨论和处理已授权目标。
	- answer 使用中文。`,
		`You are CyberAgent, an authorized-target Web/API security analysis Agent. The low-risk built-in tools you requested have just completed; read the observations and provide a final synthesis plus next-step recommendation.

Return exactly one JSON object. Do not use Markdown, code fences, or text before/after the JSON. JSON schema:
{
  "answer": "Concise synthesis: which tools ran, key results, and what to do next",
  "task": {
    "goal": "Current task goal",
    "kind": "chat|triage|planning|ops|scan|context_analysis|approval|tools|runs|health",
    "status": "running|blocked|done|idle",
    "summary": "One-sentence task summary"
  },
  "todos": [
    {
      "title": "Short task title",
      "detail": "Optional detail",
      "status": "pending|running|done|blocked|skipped",
      "priority": 1,
      "context_ref_ids": [],
      "blocked_reason": ""
    }
  ],
  "context_refs": [],
  "actions": [],
  "tool_calls": [],
	  "next_action": {
	    "type": "continue|wait_user|approval_required|finished|blocked",
	    "reason": "Why this is the next step",
	    "suggested_prompt": "A prompt the operator can send directly",
	    "auto_continue": true|false
	  },
	  "reasoning": "Briefly explain how the observations drive the synthesis and next step"
	}

Constraints:
- This is the tool-result synthesis phase; do not request or claim new actions/tool_calls. Even if you output actions, the system will ignore them.
	- Synthesize only from observations; do not invent command output, scan results, or vulnerabilities.
	- If a tool failed, state the failure and recovery step.
	- auto_continue: set true only when the observations provide enough context, the task is still unfinished, there is a clear low-risk next step, and no human confirmation/approval is needed. If the next step needs target/scope/credential/authorization input, approval, or risk confirmation, set false.
	- Only discuss and handle authorized targets by default.
	- answer must be in English.`,
	)
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return base
	}
	return base + "\n\n" + agentText(language, "操作者附加指令：", "Additional operator instructions:") + "\n" + instructions
}

func agentLLMToolDefinitions() []llm.ToolDef {
	return []llm.ToolDef{
		{
			Name:        "workspace.command",
			Description: "Run a low-risk allowlisted read-only command in the current Agent workspace. Allowed examples: pwd, ls, cat, head, tail, grep, find, wc, git status.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Low-risk allowlisted command to execute"}},"required":["command"],"additionalProperties":false}`),
		},
		{
			Name:        "workspace.files.list",
			Description: "List files in the current Agent workspace.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative directory path","default":"."}},"additionalProperties":false}`),
		},
		{
			Name:        "workspace.files.read",
			Description: "Read a workspace-relative file from the current Agent workspace.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"}},"required":["path"],"additionalProperties":false}`),
		},
		{
			Name:        "workspace.files.write",
			Description: "Write or replace a workspace-relative file in the current Agent workspace when explicitly requested by the operator.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"},"content":{"type":"string","description":"File content to write"}},"required":["path","content"],"additionalProperties":false}`),
		},
		{
			Name:        "workspace.files.patch",
			Description: "Patch a workspace-relative file by replacing one exact old text block with new text and returning a diff preview.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"},"old":{"type":"string","description":"Exact text to replace; must occur exactly once"},"new":{"type":"string","description":"Replacement text"}},"required":["path","old","new"],"additionalProperties":false}`),
		},
		{
			Name:        "workspace.files.diff",
			Description: "Create a staged workspace file change by replacing one exact old text block with new text; returns a diff preview and change_id without modifying the file.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"},"old":{"type":"string","description":"Exact text to replace; must occur exactly once"},"new":{"type":"string","description":"Replacement text"}},"required":["path","old","new"],"additionalProperties":false}`),
		},
		{
			Name:        "workspace.files.diff_many",
			Description: "Create one staged change set across multiple workspace files. Each item replaces one exact old text block with new text and returns a shared group_id plus per-file change_id values without modifying files.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"changes":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"},"old":{"type":"string","description":"Exact text to replace; must occur exactly once"},"new":{"type":"string","description":"Replacement text"}},"required":["path","old","new"],"additionalProperties":false}}},"required":["changes"],"additionalProperties":false}`),
		},
		{
			Name:        "workspace.files.rebase",
			Description: "Regenerate a pending/conflict staged change by change_id using the current exact old text from the workspace file. The replacement defaults to the original staged new text; returns a new change_id and supersedes the old change.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"change_id":{"type":"string","description":"Existing staged change ID to regenerate"},"old":{"type":"string","description":"Exact current text to replace; must occur exactly once"},"new":{"type":"string","description":"Optional replacement text; defaults to the original staged new text"}},"required":["change_id","old"],"additionalProperties":false}`),
		},
		{
			Name:        "workspace.files.apply",
			Description: "Apply a previously staged workspace file change by change_id, a staged group by group_id, or all pending changes with all=true after validating old text still matches exactly once.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"change_id":{"type":"string","description":"ID returned by workspace.files.diff"},"group_id":{"type":"string","description":"Group ID returned by workspace.files.diff_many"},"all":{"type":"boolean","description":"Apply all pending staged changes when explicitly requested"}},"additionalProperties":false}`),
		},
		{
			Name:        "workspace.files.discard",
			Description: "Discard a previously staged workspace file change by change_id, a staged group by group_id, or all pending/conflict changes with all=true without modifying files.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"change_id":{"type":"string","description":"ID returned by workspace.files.diff"},"group_id":{"type":"string","description":"Group ID returned by workspace.files.diff_many"},"all":{"type":"boolean","description":"Discard all pending/conflict staged changes when explicitly requested"}},"additionalProperties":false}`),
		},
		{
			Name:        "daemon.health",
			Description: "Check CyberAgent daemon health/version/status.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		},
		{
			Name:        "plugins.list",
			Description: "List available CyberAgent plugins/tools.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		},
		{
			Name:        "runs.list",
			Description: "List recent CyberAgent runs/history.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		},
		{
			Name:        "runs.approvals.list",
			Description: "List pending approval requests for a specific run, or summarize the global approval queue when run_id is omitted.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","description":"Optional run ID to inspect"}},"additionalProperties":false}`),
		},
		{
			Name:        "runs.approvals.approve",
			Description: "Approve one pending approval when the operator explicitly asks to approve it. Requires run_id plus approval_id/step_id unless the active run has exactly one pending approval.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","description":"Run ID"},"approval_id":{"type":"string","description":"Approval request ID"},"step_id":{"type":"string","description":"Alternative step ID"},"comment":{"type":"string","description":"Optional approval comment"}},"additionalProperties":false}`),
		},
		{
			Name:        "runs.approvals.deny",
			Description: "Deny one pending approval when the operator explicitly asks to reject it. Requires run_id plus approval_id/step_id unless the active run has exactly one pending approval.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","description":"Run ID"},"approval_id":{"type":"string","description":"Approval request ID"},"step_id":{"type":"string","description":"Alternative step ID"},"comment":{"type":"string","description":"Optional rejection comment"}},"additionalProperties":false}`),
		},
		{
			Name:        "runs.resume_approved",
			Description: "Resume approved steps for a run when the operator explicitly asks to resume approved work.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","description":"Run ID to resume; defaults to active run when omitted"}},"additionalProperties":false}`),
		},
	}
}

func parseAgentPlannerResponse(raw string) (*agentPlannerResponse, error) {
	candidate := extractJSONObject(raw)
	if strings.TrimSpace(candidate) == "" {
		return nil, fmt.Errorf("planner response does not contain a JSON object")
	}
	var plan agentPlannerResponse
	if err := json.Unmarshal([]byte(candidate), &plan); err != nil {
		return nil, fmt.Errorf("decode planner response: %w", err)
	}
	if strings.TrimSpace(plan.Answer) == "" && plan.Task == nil && len(plan.Todos) == 0 && plan.NextAction == nil && len(plan.ContextRefs) == 0 && len(agentPlannerActions(&plan)) == 0 {
		return nil, fmt.Errorf("planner response is empty")
	}
	return &plan, nil
}

func agentPlannerActionsFromLLMToolUse(resp *llm.ChatResponse) []agentPlannerAction {
	if resp == nil {
		return nil
	}
	actions := make([]agentPlannerAction, 0)
	for _, block := range resp.Content {
		switch toolUse := block.(type) {
		case llm.ToolUseBlock:
			action := agentPlannerAction{
				Name:      normalizeAgentPlannerActionName(toolUse.Name),
				Arguments: normalizeAgentPlannerActionArguments(string(toolUse.Input)),
			}
			if action.Name != "" {
				actions = append(actions, action)
			}
		case *llm.ToolUseBlock:
			if toolUse == nil {
				continue
			}
			action := agentPlannerAction{
				Name:      normalizeAgentPlannerActionName(toolUse.Name),
				Arguments: normalizeAgentPlannerActionArguments(string(toolUse.Input)),
			}
			if action.Name != "" {
				actions = append(actions, action)
			}
		}
	}
	return actions
}

func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 3 {
			if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				raw = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
			}
		}
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return ""
	}
	return strings.TrimSpace(raw[start : end+1])
}

func agentLLMHistory(history []agentMessage, memory *agentMemorySummary, language string) []llm.Message {
	start := len(history) - 20
	if start < 0 {
		start = 0
	}

	var memoryMessage *llm.Message
	if memoryPrompt := agentMemorySummaryPrompt(language, memory); memoryPrompt != "" {
		message := llm.Message{
			Role:    llm.RoleUser,
			Content: []llm.ContentBlock{llm.TextBlock{Text: memoryPrompt}},
		}
		memoryMessage = &message
	}

	messages := make([]llm.Message, 0, len(history[start:])+1)
	seenUser := false
	for _, message := range history[start:] {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if len(content) > 4000 {
			content = content[:4000] + "\n…"
		}

		role := llm.RoleAssistant
		switch message.Role {
		case "user":
			role = llm.RoleUser
			seenUser = true
		case "agent":
			if !seenUser {
				continue
			}
		case "tool":
			if !seenUser {
				continue
			}
			content = "Runtime/tool observation:\n" + content
		default:
			continue
		}

		if len(messages) > 0 && messages[len(messages)-1].Role == role {
			previous := messages[len(messages)-1].Content
			if len(previous) > 0 {
				if text, ok := previous[len(previous)-1].(llm.TextBlock); ok {
					text.Text = strings.TrimSpace(text.Text) + "\n\n" + content
					messages[len(messages)-1].Content[len(previous)-1] = text
					continue
				}
			}
		}
		messages = append(messages, llm.Message{Role: role, Content: []llm.ContentBlock{llm.TextBlock{Text: content}}})
	}

	if len(messages) > 17 {
		messages = messages[len(messages)-16:]
	}
	for len(messages) > 0 && messages[0].Role != llm.RoleUser {
		messages = messages[1:]
	}
	if memoryMessage != nil {
		messages = append([]llm.Message{*memoryMessage}, messages...)
	}
	return messages
}

func agentLLMToolSynthesisHistory(history []agentMessage, memory *agentMemorySummary, toolMessages []agentMessage, language string) []llm.Message {
	messages := agentLLMHistory(history, memory, language)
	observation := agentToolObservationPrompt(language, toolMessages)
	if strings.TrimSpace(observation) == "" {
		return messages
	}
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{llm.TextBlock{Text: observation}},
	})
	if len(messages) > 18 {
		messages = trimAgentLLMMessagesPreservingFirst(messages, 18, agentMemorySummaryPrompt(language, memory) != "")
	}
	for len(messages) > 0 && messages[0].Role != llm.RoleUser {
		messages = messages[1:]
	}
	return messages
}

func trimAgentLLMMessagesPreservingFirst(messages []llm.Message, max int, preserveFirst bool) []llm.Message {
	if max <= 0 || len(messages) <= max {
		return messages
	}
	if !preserveFirst || len(messages) == 0 {
		return messages[len(messages)-max:]
	}
	if max == 1 {
		return []llm.Message{messages[0]}
	}
	tail := messages[1:]
	tailMax := max - 1
	if len(tail) > tailMax {
		tail = tail[len(tail)-tailMax:]
	}
	trimmed := make([]llm.Message, 0, 1+len(tail))
	trimmed = append(trimmed, messages[0])
	trimmed = append(trimmed, tail...)
	return trimmed
}

func compactAgentSessionMessagesLocked(session *agentSession, max int) {
	if session == nil {
		return
	}
	if max <= 0 {
		max = agentSessionMaxMessages
	}
	if len(session.Messages) <= max {
		return
	}
	compactCount := len(session.Messages) - max
	if compactCount <= 0 {
		return
	}
	compacted := cloneAgentMessages(session.Messages[:compactCount])
	session.Messages = cloneAgentMessages(session.Messages[compactCount:])
	session.MemorySummary = buildAgentMemorySummary(session, compacted, session.MemorySummary)
}

func buildAgentMemorySummary(session *agentSession, compacted []agentMessage, previous *agentMemorySummary) *agentMemorySummary {
	if len(compacted) == 0 && previous == nil {
		return nil
	}
	parts := make([]string, 0, 12)
	if previous != nil && strings.TrimSpace(previous.Summary) != "" {
		parts = append(parts, "Previous memory: "+agentLimitBlock(strings.TrimSpace(previous.Summary), 1800))
	}
	if session != nil && session.ActiveTask != nil {
		taskBits := []string{
			"goal=" + emptyString(session.ActiveTask.Goal, "-"),
			"kind=" + emptyString(session.ActiveTask.Kind, "-"),
			"status=" + emptyString(session.ActiveTask.Status, "-"),
		}
		if session.ActiveTask.ActiveRunID != "" {
			taskBits = append(taskBits, "active_run="+session.ActiveTask.ActiveRunID)
		}
		if session.ActiveTask.Summary != "" {
			taskBits = append(taskBits, "summary="+agentLimitBlock(session.ActiveTask.Summary, 320))
		}
		parts = append(parts, "Active task: "+strings.Join(taskBits, " | "))
	}
	if session != nil && len(session.Todos) > 0 {
		todoParts := make([]string, 0, minInt(len(session.Todos), 8))
		for idx, todo := range session.Todos {
			if idx >= 8 {
				break
			}
			todoParts = append(todoParts, fmt.Sprintf("%s:%s", emptyString(todo.Title, todo.ID), emptyString(todo.Status, "-")))
		}
		if len(todoParts) > 0 {
			parts = append(parts, "Todos: "+strings.Join(todoParts, "; "))
		}
	}
	if session != nil && len(session.ContextRefs) > 0 {
		refParts := make([]string, 0, minInt(len(session.ContextRefs), 8))
		for idx, ref := range session.ContextRefs {
			if idx >= 8 {
				break
			}
			refText := firstNonEmpty(ref.Label, ref.RefID, ref.ID)
			if ref.Summary != "" {
				refText += " (" + agentLimitBlock(ref.Summary, 180) + ")"
			}
			refParts = append(refParts, refText)
		}
		if len(refParts) > 0 {
			parts = append(parts, "Context refs: "+strings.Join(refParts, "; "))
		}
	}

	messageParts := make([]string, 0, minInt(len(compacted), 24))
	start := 0
	if len(compacted) > 24 {
		start = len(compacted) - 24
	}
	for _, message := range compacted[start:] {
		summary := agentMemoryMessageLine(message)
		if summary != "" {
			messageParts = append(messageParts, summary)
		}
	}
	if len(messageParts) > 0 {
		parts = append(parts, "Compacted conversation:\n"+strings.Join(messageParts, "\n"))
	}

	summary := agentLimitBlock(strings.TrimSpace(strings.Join(parts, "\n\n")), 5000)
	if summary == "" && previous != nil {
		summary = previous.Summary
	}
	var lastID string
	var lastAt *time.Time
	if len(compacted) > 0 {
		last := compacted[len(compacted)-1]
		lastID = last.ID
		if !last.Timestamp.IsZero() {
			ts := last.Timestamp
			lastAt = &ts
		}
	} else if previous != nil {
		lastID = previous.LastMessageID
		if previous.LastMessageAt != nil {
			ts := *previous.LastMessageAt
			lastAt = &ts
		}
	}
	messageCount := len(compacted)
	if previous != nil {
		messageCount += previous.MessageCount
	}
	return &agentMemorySummary{
		Summary:       summary,
		MessageCount:  messageCount,
		LastMessageID: lastID,
		LastMessageAt: lastAt,
		UpdatedAt:     time.Now().UTC(),
	}
}

func agentMemoryMessageLine(message agentMessage) string {
	role := emptyString(message.Role, "message")
	content := strings.TrimSpace(message.Content)
	if content != "" {
		content = strings.ReplaceAll(content, "\r\n", "\n")
		content = strings.ReplaceAll(content, "\r", "\n")
		content = strings.Join(strings.Fields(content), " ")
		content = agentLimitBlock(content, 420)
	}
	toolBits := make([]string, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		name := emptyString(call.Name, "tool")
		status := string(call.Status)
		if status == "" {
			status = "-"
		}
		toolBits = append(toolBits, name+":"+status)
	}
	prefix := "- " + role
	if !message.Timestamp.IsZero() {
		prefix += "@" + message.Timestamp.Format(time.RFC3339)
	}
	switch {
	case content != "" && len(toolBits) > 0:
		return prefix + ": " + content + " [tools: " + strings.Join(toolBits, ", ") + "]"
	case content != "":
		return prefix + ": " + content
	case len(toolBits) > 0:
		return prefix + ": [tools: " + strings.Join(toolBits, ", ") + "]"
	default:
		return ""
	}
}

func agentMemorySummaryPrompt(language string, memory *agentMemorySummary) string {
	if memory == nil || strings.TrimSpace(memory.Summary) == "" {
		return ""
	}
	return agentText(
		language,
		fmt.Sprintf("以下是当前 Agent 会话早期内容的压缩记忆。它来自已从短期聊天窗口裁剪的 %d 条消息；请把它当作已有上下文，不要要求用户重复这些信息。\n\n%s", memory.MessageCount, agentLimitBlock(memory.Summary, 5000)),
		fmt.Sprintf("The following is compressed memory from the earlier part of this Agent session. It comes from %d message(s) that were trimmed out of the short-term chat window; treat it as existing context and do not ask the operator to repeat it.\n\n%s", memory.MessageCount, agentLimitBlock(memory.Summary, 5000)),
	)
}

func agentToolObservationPrompt(language string, toolMessages []agentMessage) string {
	var builder strings.Builder
	builder.WriteString(agentText(
		language,
		"以下是刚刚执行完成的低风险内置工具 observation。请基于这些结果生成最终总结和下一步建议；不要再请求新的 actions/tool_calls。\n",
		"Below are the observations from the low-risk built-in tools that just executed. Synthesize the final answer and next step from these results; do not request new actions/tool_calls.\n",
	))
	for i, message := range toolMessages {
		content := strings.TrimSpace(message.Content)
		if content == "" && len(message.ToolCalls) == 0 {
			continue
		}
		if content != "" {
			content = agentLimitBlock(content, 3000)
		}
		builder.WriteString(fmt.Sprintf("\nObservation %d", i+1))
		if len(message.ToolCalls) > 0 {
			names := make([]string, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				name := strings.TrimSpace(call.Name)
				if name == "" {
					name = "tool"
				}
				names = append(names, fmt.Sprintf("%s:%s", name, call.Status))
			}
			builder.WriteString(" [")
			builder.WriteString(strings.Join(names, ", "))
			builder.WriteString("]")
		}
		builder.WriteString(":\n")
		if content != "" {
			builder.WriteString(content)
			builder.WriteString("\n")
		}
		for _, call := range message.ToolCalls {
			summary := agentSummarizeToolCall(call)
			if summary != "" {
				builder.WriteString(summary)
				builder.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func agentSummarizeToolCall(call agentToolCall) string {
	parts := []string{
		"tool=" + emptyString(call.Name, "tool"),
		"status=" + string(call.Status),
	}
	if detail := strings.TrimSpace(call.Detail); detail != "" {
		parts = append(parts, "detail="+agentLimitBlock(detail, 240))
	}
	if call.Error != nil && strings.TrimSpace(call.Error.Message) != "" {
		if call.Error.Code != "" {
			parts = append(parts, "error_code="+string(call.Error.Code))
		}
		parts = append(parts, "error="+agentLimitBlock(call.Error.Message, 500))
	}
	if len(call.Result) > 0 {
		trimmed := make(map[string]any, len(call.Result))
		keys := make([]string, 0, len(call.Result))
		for key := range call.Result {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := call.Result[key]
			switch v := value.(type) {
			case string:
				trimmed[key] = agentLimitBlock(v, 1000)
			default:
				trimmed[key] = v
			}
		}
		if encoded, err := json.Marshal(trimmed); err == nil {
			parts = append(parts, "result="+agentLimitBlock(string(encoded), 2000))
		}
	}
	return "- " + strings.Join(parts, " | ")
}

func agentLLMText(resp *llm.ChatResponse) string {
	if resp == nil {
		return ""
	}
	parts := make([]string, 0, len(resp.Content))
	for _, block := range resp.Content {
		switch text := block.(type) {
		case llm.TextBlock:
			if strings.TrimSpace(text.Text) != "" {
				parts = append(parts, strings.TrimSpace(text.Text))
			}
		case *llm.TextBlock:
			if text != nil && strings.TrimSpace(text.Text) != "" {
				parts = append(parts, strings.TrimSpace(text.Text))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

var agentURLPattern = regexp.MustCompile(`https?://[^\s"'<>，。)]+`)

func agentExtractURL(prompt string) string {
	return agentCleanToken(agentURLPattern.FindString(prompt))
}

func agentExtractScope(prompt, target string) string {
	lower := strings.ToLower(prompt)
	for _, marker := range []string{"scope=", "scope:", "scope：", "范围=", "范围:", "范围："} {
		index := strings.Index(lower, strings.ToLower(marker))
		if index >= 0 {
			value := prompt[index+len(marker):]
			fields := strings.FieldsFunc(value, func(r rune) bool {
				return r == ' ' || r == '\t' || r == '\n' || r == ',' || r == '，' || r == ';' || r == '；'
			})
			if len(fields) > 0 {
				return agentCleanToken(fields[0])
			}
		}
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func agentCleanToken(value string) string {
	return strings.Trim(strings.TrimSpace(value), "，。),]\"'")
}

func agentWantsExternalTools(prompt string) bool {
	return regexp.MustCompile(`(?i)(external|plugin|tool|插件|外部工具|启用工具)`).MatchString(prompt)
}

func agentIsRunContextAnalysisIntent(prompt string) bool {
	return agentExtractRunID(prompt) != "" &&
		regexp.MustCompile(`(?i)(report|finding|artifact|evidence|events?|timeline|log|approvals?|approve|deny|context|报告|发现|漏洞|证据|工件|产物|事件|时间线|日志|审批|批准|拒绝|上下文|修复建议|误报)`).MatchString(prompt)
}

func agentIsScanIntent(prompt string) bool {
	if agentExtractURL(prompt) == "" {
		return false
	}
	return regexp.MustCompile(`(?i)(scan|test|assess|audit|扫描|测试|检测|分析|安全)`).MatchString(prompt)
}

func agentIsHealthIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(health|doctor|status|daemon|体检|健康|状态|连接)`).MatchString(prompt)
}

func agentIsPluginIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(plugin|tool|capabilit|插件|工具|能力)`).MatchString(prompt)
}

func agentIsRunIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(runs?|history|report|finding|artifact|历史|运行|报告|发现|证据)`).MatchString(prompt)
}

func agentIsApprovalIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(approval|approve|deny|human|审批|批准|拒绝|人工)`).MatchString(prompt)
}

func agentIsResumeApprovedIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(resume-approved|resume\s+approved|resume\s+approval|resume\s+run|恢复已批准|恢复已审批|恢复批准|恢复运行|恢复执行|继续已批准|继续执行已批准)`).MatchString(prompt)
}

func agentIsApprovalDecisionIntent(prompt string) bool {
	if agentApprovalDecisionAction(prompt) == "" {
		return false
	}
	return agentExtractApprovalID(prompt) != "" ||
		agentExtractRunID(prompt) != "" ||
		regexp.MustCompile(`(?i)(current|active|this|当前|这个|该|本次).*(approval|审批)`).MatchString(prompt) ||
		regexp.MustCompile(`(?i)(approval|审批).*(current|active|this|当前|这个|该|本次)`).MatchString(prompt)
}

func agentApprovalDecisionAction(prompt string) string {
	approveRe := regexp.MustCompile(`(?i)(\bapprove(?:d)?\b|\ballow(?:ed)?\b|\bauthori[sz]e(?:d)?\b|批准|同意|允许|授权|通过)`)
	denyRe := regexp.MustCompile(`(?i)(\bdeny\b|\bdenied\b|\breject(?:ed)?\b|\bblock(?:ed)?\b|拒绝|驳回|不同意|不批准|不要执行|禁止)`)
	approve := approveRe.FindStringIndex(prompt)
	deny := denyRe.FindStringIndex(prompt)
	if approve == nil && deny == nil {
		return ""
	}
	if approve != nil && deny != nil {
		lower := strings.ToLower(prompt)
		if strings.Contains(lower, "approve/deny") ||
			strings.Contains(lower, "approve or deny") ||
			strings.Contains(prompt, "批准/拒绝") ||
			strings.Contains(prompt, "批准或拒绝") {
			return ""
		}
		if deny[0] < approve[0] {
			return "deny"
		}
		return "approve"
	}
	if deny != nil {
		return "deny"
	}
	return "approve"
}

func agentExtractApprovalComment(prompt string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)\bcomment\s*[:=]\s*(.+)$`),
		regexp.MustCompile(`(?is)(?:备注|原因|理由)\s*[:：]\s*(.+)$`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return trimSummary(agentCleanToken(match[1]), 260)
		}
	}
	return ""
}

func agentIsCommandIntent(prompt string) bool {
	if regexp.MustCompile("(?i)(`[^`]+`|command\\s*[:=]|shell\\s*[:=]|terminal\\s*[:=]|执行命令|运行命令|命令[:：])").MatchString(prompt) {
		return true
	}
	if len(agentExtractCommands(prompt)) > 0 {
		return true
	}
	return regexp.MustCompile(`(?i)^\s*(pwd|ls|cat|head|tail|grep|find|git\s+status|wc|sed|awk|python3?|node)\b`).MatchString(strings.TrimSpace(prompt))
}

func agentExtractCommand(prompt string) string {
	if commands := agentExtractCommands(prompt); len(commands) > 0 {
		return commands[0]
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile("`([^`]+)`"),
		regexp.MustCompile(`(?i)(?:command|shell|terminal)\s*[:=]\s*(.+)$`),
		regexp.MustCompile(`(?:执行命令|运行命令|命令)\s*[:：]?\s*(.+)$`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return strings.TrimSpace(match[1])
		}
	}
	trimmed := strings.TrimSpace(prompt)
	if regexp.MustCompile(`(?i)^(pwd|ls|cat|head|tail|grep|find|git\s+status|wc|sed|awk|python3?|node)\b`).MatchString(trimmed) {
		return trimmed
	}
	return ""
}

func agentExtractCommands(prompt string) []string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}
	seen := map[string]struct{}{}
	commands := []string{}
	for _, command := range agentExtractBacktickCommands(prompt) {
		if _, ok := seen[command]; ok {
			continue
		}
		seen[command] = struct{}{}
		commands = append(commands, command)
	}
	lower := strings.ToLower(prompt)
	candidates := []string{
		"git status --short",
		"git status",
		"git branch",
		"git diff --stat",
		"git diff",
		"git ls-files",
		"pwd",
		"ls -la",
		"ls -al",
		"ls -a",
		"ls",
	}
	for _, command := range candidates {
		if !agentPromptMentionsCommand(lower, command) {
			continue
		}
		if agentMoreSpecificCommandSeen(seen, command) {
			continue
		}
		if !agentPlannerCommandAllowed(command) {
			continue
		}
		if _, ok := seen[command]; ok {
			continue
		}
		seen[command] = struct{}{}
		commands = append(commands, command)
	}
	if len(commands) > 8 {
		commands = commands[:8]
	}
	return commands
}

func agentMoreSpecificCommandSeen(seen map[string]struct{}, command string) bool {
	switch command {
	case "ls":
		for _, specific := range []string{"ls -la", "ls -al", "ls -a"} {
			if _, ok := seen[specific]; ok {
				return true
			}
		}
	case "git status":
		if _, ok := seen["git status --short"]; ok {
			return true
		}
	case "git diff":
		if _, ok := seen["git diff --stat"]; ok {
			return true
		}
	}
	return false
}

func agentExtractBacktickCommands(prompt string) []string {
	matches := regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(prompt, -1)
	commands := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		command := strings.TrimSpace(match[1])
		if command == "" || !agentPlannerCommandAllowed(command) {
			continue
		}
		commands = append(commands, command)
	}
	return commands
}

func agentPromptMentionsCommand(lowerPrompt, command string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return false
	}
	switch command {
	case "pwd":
		return regexp.MustCompile(`(^|[^a-z0-9_-])pwd([^a-z0-9_-]|$)`).MatchString(lowerPrompt) ||
			strings.Contains(lowerPrompt, "当前目录") ||
			strings.Contains(lowerPrompt, "工作目录")
	case "ls":
		return regexp.MustCompile(`(^|[^a-z0-9_-])ls([^a-z0-9_-]|$)`).MatchString(lowerPrompt) ||
			strings.Contains(lowerPrompt, "列出文件") ||
			strings.Contains(lowerPrompt, "文件列表")
	case "ls -la":
		return strings.Contains(lowerPrompt, "ls -la") || strings.Contains(lowerPrompt, "ls -al")
	case "ls -al":
		return strings.Contains(lowerPrompt, "ls -al")
	case "ls -a":
		return strings.Contains(lowerPrompt, "ls -a")
	case "git status":
		return strings.Contains(lowerPrompt, "git status") || strings.Contains(lowerPrompt, "git 状态")
	case "git status --short":
		return strings.Contains(lowerPrompt, "git status --short") || strings.Contains(lowerPrompt, "git status -s")
	case "git branch":
		return strings.Contains(lowerPrompt, "git branch")
	case "git diff":
		return strings.Contains(lowerPrompt, "git diff")
	case "git diff --stat":
		return strings.Contains(lowerPrompt, "git diff --stat")
	case "git ls-files":
		return strings.Contains(lowerPrompt, "git ls-files")
	default:
		return strings.Contains(lowerPrompt, command)
	}
}

func agentExtractPath(prompt string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile("`([^`]+)`"),
		regexp.MustCompile(`(?i)(?:path|file|directory|folder)\s*[:=]\s*([^\s,，;；]+)`),
		regexp.MustCompile(`(?i)(?:read|cat|open|list|ls)\s+(?:path|file|directory|folder|files?)?\s*([A-Za-z0-9._/\-]+)`),
		regexp.MustCompile(`(?:路径|文件|目录)\s*[:：]?\s*([^\s,，;；]+)`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return agentCleanToken(match[1])
		}
	}
	return "."
}

func agentExtractWorkspaceWrite(prompt string) (string, string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", ""
	}

	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)(?:path|file|filename)\s*[:=]\s*([^\s,，;；]+).*?(?:content|body|text|input)\s*[:=]\s*(.+)$`),
		regexp.MustCompile(`(?is)(?:文件|路径)\s*[:：]\s*([^\s,，;；]+).*?(?:内容|正文)\s*[:：]\s*(.+)$`),
		regexp.MustCompile(`(?is)(?:写入|创建|保存|修改|更新|覆盖)(?:工作空间)?(?:文件|路径)?\s*([^\s,，;；]+)\s*(?:内容|正文)\s*[:：]\s*(.+)$`),
		regexp.MustCompile(`(?is)(?:write|create|save|update|replace)\s+(?:workspace\s+)?(?:file\s+)?([^\s,;]+)\s+(?:content|body|text|input)\s*[:=]\s*(.+)$`),
		regexp.MustCompile(`(?is)(?:写入|创建|保存|修改|更新|覆盖)(?:工作空间)?(?:文件|路径)?\s*([^\s,，;；]+)\s*(?:为|成|到)\s*(.+)$`),
		regexp.MustCompile(`(?is)(?:write|create|save|update|replace)\s+(?:workspace\s+)?(?:file\s+)?([^\s,;]+)\s+(?:with|as|to)\s+(.+)$`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 2 {
			return agentCleanWorkspaceWritePath(match[1]), agentCleanWorkspaceWriteContent(match[2])
		}
	}

	for _, marker := range []string{
		"content=", "content:", "content：",
		"body=", "body:", "body：",
		"text=", "text:", "text：",
		"input=", "input:", "input：",
		"内容=", "内容:", "内容：",
		"正文=", "正文:", "正文：",
	} {
		idx := strings.Index(strings.ToLower(prompt), strings.ToLower(marker))
		if idx < 0 {
			continue
		}
		before := strings.TrimSpace(prompt[:idx])
		content := strings.TrimSpace(prompt[idx+len(marker):])
		relPath := agentExtractWorkspaceWritePath(before)
		if relPath == "" || relPath == "." {
			relPath = agentExtractWorkspaceWritePath(prompt)
		}
		return agentCleanWorkspaceWritePath(relPath), agentCleanWorkspaceWriteContent(content)
	}

	relPath := agentExtractWorkspaceWritePath(prompt)
	return agentCleanWorkspaceWritePath(relPath), ""
}

func agentExtractWorkspaceWritePath(prompt string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile("`([^`]+)`"),
		regexp.MustCompile(`(?i)(?:path|file|filename)\s*[:=]\s*([^\s,，;；]+)`),
		regexp.MustCompile(`(?:文件|路径)\s*[:：]\s*([^\s,，;；]+)`),
		regexp.MustCompile(`(?i)(?:write|create|save|update|replace)\s+(?:workspace\s+)?(?:file\s+)?([^\s,;]+)`),
		regexp.MustCompile(`(?:写入|创建|保存|修改|更新|覆盖)(?:工作空间)?(?:文件|路径)?\s*([^\s,，;；]+)`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return agentCleanWorkspaceWritePath(match[1])
		}
	}
	return "."
}

func agentCleanWorkspaceWritePath(value string) string {
	value = agentCleanToken(strings.Trim(value, "`"))
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "file=")
	value = strings.TrimPrefix(value, "path=")
	value = strings.TrimPrefix(value, "filename=")
	value = agentCleanToken(value)
	switch strings.ToLower(value) {
	case "", "file", "path", "filename", "文件", "路径":
		return ""
	default:
		return value
	}
}

func agentCleanWorkspaceWriteContent(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeft(value, " \t\r\n:=：")
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		pairs := [][2]string{{"`", "`"}, {`"`, `"`}, {"'", "'"}}
		for _, pair := range pairs {
			if strings.HasPrefix(value, pair[0]) && strings.HasSuffix(value, pair[1]) {
				return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, pair[0]), pair[1]))
			}
		}
	}
	return value
}

func agentIsWorkspacePatchIntent(prompt string) bool {
	if !regexp.MustCompile(`(?i)(patch|modify|edit|replace|修改|替换|编辑|修补)`).MatchString(prompt) {
		return false
	}
	hasOld := regexp.MustCompile(`(?i)(old|old_text|before|search|原文|查找|替换前)\s*[:：=]`).MatchString(prompt)
	hasNew := regexp.MustCompile(`(?i)(new|new_text|after|replace|替换为|替换后|新文本)\s*[:：=]`).MatchString(prompt)
	return hasOld && hasNew
}

func agentIsWorkspaceDiffIntent(prompt string) bool {
	oldMarker := regexp.MustCompile(`(?i)(old|old_text|before|search|原文|查找|替换前)\s*[:：=]`)
	oldLoc := oldMarker.FindStringIndex(prompt)
	if oldLoc == nil {
		return false
	}
	head := prompt[:oldLoc[0]]
	if !regexp.MustCompile(`(?i)(diff|preview|stage|plan patch|预览|差异|生成修改|暂存|待应用)`).MatchString(head) {
		return false
	}
	hasOld := true
	hasNew := regexp.MustCompile(`(?i)(new|new_text|after|replace|替换为|替换后|新文本)\s*[:：=]`).MatchString(prompt)
	return hasOld && hasNew
}

func agentIsWorkspaceApplyIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(apply|commit|应用|确认应用|写入修改)`).MatchString(prompt) &&
		(agentExtractWorkspaceChangeID(prompt) != "" || agentExtractWorkspaceGroupID(prompt) != "")
}

func agentIsWorkspaceRebaseIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(rebase|regenerate|refresh|重新生成|重建|刷新|基于当前)`).MatchString(prompt) &&
		agentExtractWorkspaceChangeID(prompt) != ""
}

func agentIsWorkspaceApplyAllIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(apply|commit|应用|确认应用|写入修改)`).MatchString(prompt) &&
		regexp.MustCompile(`(?i)(all|every|pending|staged|全部|所有|所有待应用|待应用|暂存)`).MatchString(prompt)
}

func agentIsWorkspaceDiscardIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(discard|drop|cancel|abandon|丢弃|取消|放弃|不要应用)`).MatchString(prompt) &&
		(agentExtractWorkspaceChangeID(prompt) != "" || agentExtractWorkspaceGroupID(prompt) != "")
}

func agentIsWorkspaceDiscardAllIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(discard|drop|cancel|abandon|丢弃|取消|放弃|不要应用)`).MatchString(prompt) &&
		regexp.MustCompile(`(?i)(all|every|pending|staged|全部|所有|所有待应用|待应用|暂存)`).MatchString(prompt)
}

func agentPlannerActionAllowedByPrompt(prompt, actionName string) bool {
	switch normalizeAgentPlannerActionName(actionName) {
	case "workspace.files.write":
		return agentIsWorkspaceWriteIntent(prompt)
	case "workspace.files.patch":
		return agentIsWorkspacePatchIntent(prompt)
	case "workspace.files.diff", "workspace.files.diff_many":
		return agentIsWorkspaceDiffIntent(prompt) || agentIsWorkspacePatchIntent(prompt)
	case "workspace.files.rebase":
		return agentIsWorkspaceRebaseIntent(prompt)
	case "workspace.files.apply":
		return agentIsWorkspaceApplyIntent(prompt) || agentIsWorkspaceApplyAllIntent(prompt)
	case "workspace.files.discard":
		return agentIsWorkspaceDiscardIntent(prompt) || agentIsWorkspaceDiscardAllIntent(prompt)
	case "runs.approvals.approve":
		return agentApprovalDecisionAction(prompt) == "approve" && agentIsApprovalDecisionIntent(prompt)
	case "runs.approvals.deny":
		return agentApprovalDecisionAction(prompt) == "deny" && agentIsApprovalDecisionIntent(prompt)
	case "runs.resume_approved":
		return agentIsResumeApprovedIntent(prompt)
	default:
		return true
	}
}

func agentExtractWorkspacePatch(prompt string) (string, string, string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", "", ""
	}

	relPath := "."
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:path|file|filename)\s*[:=]\s*([^\s,，;；]+)`),
		regexp.MustCompile(`(?:文件|路径)\s*[:：]\s*([^\s,，;；]+)`),
		regexp.MustCompile(`(?i)(?:patch|modify|edit|replace)\s+(?:workspace\s+)?(?:file\s+)?([^\s,;]+)`),
		regexp.MustCompile(`(?:修改|替换|编辑|修补)(?:工作空间)?(?:文件|路径)?\s*([^\s,，;；]+)`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			relPath = agentCleanWorkspaceWritePath(match[1])
			break
		}
	}
	if relPath == "" || relPath == "." {
		relPath = agentExtractWorkspaceWritePath(prompt)
	}

	oldText, newText := "", ""
	oldMarker := regexp.MustCompile(`(?is)(?:^|[\s,，;；])(?:old|old_text|before|search|原文|查找|替换前)\s*[:：=]\s*`)
	newMarker := regexp.MustCompile(`(?is)(?:^|[\s,，;；])(?:new|new_text|after|replace|替换为|替换后|新文本)\s*[:：=]\s*`)
	if oldLoc := oldMarker.FindStringIndex(prompt); oldLoc != nil {
		tail := prompt[oldLoc[1]:]
		if newLoc := newMarker.FindStringIndex(tail); newLoc != nil {
			oldText = tail[:newLoc[0]]
			newText = tail[newLoc[1]:]
		}
	}

	return agentCleanWorkspaceWritePath(relPath), agentCleanWorkspacePatchText(oldText), agentCleanWorkspacePatchText(newText)
}

func agentExtractWorkspaceRebaseTexts(prompt string) (string, string) {
	oldText, newText := "", ""
	oldMarker := regexp.MustCompile(`(?is)(?:^|[\s,，;；])(?:old|old_text|before|search|当前文本|当前old|当前)\s*[:：=]\s*`)
	newMarker := regexp.MustCompile(`(?is)(?:^|[\s,，;；])(?:new|new_text|after|replace|替换为|替换后|新文本)\s*[:：=]\s*`)
	if oldLoc := oldMarker.FindStringIndex(prompt); oldLoc != nil {
		tail := prompt[oldLoc[1]:]
		if newLoc := newMarker.FindStringIndex(tail); newLoc != nil {
			oldText = tail[:newLoc[0]]
			newText = tail[newLoc[1]:]
		} else {
			oldText = tail
		}
	}
	return agentCleanWorkspacePatchText(oldText), agentCleanWorkspacePatchText(newText)
}

func agentCleanWorkspacePatchText(value string) string {
	return agentCleanWorkspaceWriteContent(value)
}

var agentWorkspaceChangeIDPattern = regexp.MustCompile(`change_[A-Za-z0-9_]+`)
var agentWorkspaceGroupIDPattern = regexp.MustCompile(`changeset_[A-Za-z0-9_]+`)

func agentExtractWorkspaceChangeID(prompt string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile("`(change_[A-Za-z0-9_]+)`"),
		regexp.MustCompile(`(?i)(?:change_id|change|id)\s*[:=]\s*(change_[A-Za-z0-9_]+)`),
		agentWorkspaceChangeIDPattern,
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return agentCleanToken(match[1])
		}
		if found := re.FindString(prompt); found != "" {
			return agentCleanToken(found)
		}
	}
	return ""
}

func agentExtractWorkspaceGroupID(prompt string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile("`(changeset_[A-Za-z0-9_]+)`"),
		regexp.MustCompile(`(?i)(?:group_id|group|changeset|change_set)\s*[:=]\s*(changeset_[A-Za-z0-9_]+)`),
		agentWorkspaceGroupIDPattern,
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return agentCleanToken(match[1])
		}
		if found := re.FindString(prompt); found != "" {
			return agentCleanToken(found)
		}
	}
	return ""
}

var agentRunIDPattern = regexp.MustCompile(`(?:job|run)_[A-Za-z0-9][A-Za-z0-9_-]*`)

func agentExtractRunID(prompt string) string {
	return agentCleanToken(agentRunIDPattern.FindString(prompt))
}

var agentApprovalIDPattern = regexp.MustCompile(`\b(?:approval|step)_[A-Za-z0-9][A-Za-z0-9_-]*`)

func agentExtractApprovalID(prompt string) string {
	return agentCleanToken(agentApprovalIDPattern.FindString(prompt))
}

func agentExtractJobID(prompt string) string {
	return agentCleanToken(regexp.MustCompile(`job_[A-Za-z0-9][A-Za-z0-9_-]*`).FindString(prompt))
}

func agentRunContextKind(prompt string) string {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	switch {
	case regexp.MustCompile(`(?i)(approval|approve|deny|human|审批|批准|拒绝|人工)`).MatchString(prompt):
		return "approvals"
	case regexp.MustCompile(`(?i)(events?|timeline|log|trace|事件|时间线|日志|轨迹)`).MatchString(prompt):
		return "events"
	case regexp.MustCompile(`(?i)(artifact|artifacts|artifact_index|artifact index|证据索引|工件|产物)`).MatchString(prompt):
		return "artifact_index"
	case regexp.MustCompile(`(?i)(findings?|finding|vulnerabilit|issue|发现|漏洞|问题)`).MatchString(prompt):
		return "findings"
	case strings.Contains(lower, "report") || strings.Contains(prompt, "报告"):
		return "report_json"
	default:
		return "report_json"
	}
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
		return agentText(language,
			fmt.Sprintf("已读取运行 %s 的 findings：findings=%d，evidence=%d，artifacts=%d。下一步可按 sample_findings 的 evidence_ids 追溯证据，并按“影响 → 证据 → 误报检查 → 修复 → 复测”生成行动计划。", runID, intFromAny(result["finding_count"]), intFromAny(result["evidence_count"]), intFromAny(result["artifact_count"])),
			fmt.Sprintf("Read findings for run %s: findings=%d, evidence=%d, artifacts=%d. Next, trace evidence_ids from sample_findings and produce an impact → evidence → false-positive check → remediation → retest action plan.", runID, intFromAny(result["finding_count"]), intFromAny(result["evidence_count"]), intFromAny(result["artifact_count"])),
		)
	default:
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
		SessionID:      sessionID,
		Messages:       cloneAgentMessages(messages),
		ToolCalls:      cloneAgentToolCalls(toolCalls),
		ActiveTask:     cloneAgentTask(session.ActiveTask),
		Todos:          cloneAgentTodos(session.Todos),
		ContextRefs:    cloneAgentContextRefs(session.ContextRefs),
		PendingChanges: cloneAgentWorkspaceChanges(session.PendingChanges),
		MemorySummary:  cloneAgentMemorySummary(session.MemorySummary),
		NextAction:     cloneAgentNextAction(session.NextAction),
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
	switch status {
	case contracts.StatusSucceeded, contracts.StatusPartiallySucceeded:
		return normalizeCompletedActiveRunSession(session, status) || changed
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

func normalizeCompletedActiveRunSession(session *agentSession, status contracts.Status) bool {
	if session == nil || session.ActiveTask == nil {
		return false
	}
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
	return true
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
		strings.Contains(name, "workspace.files.read"),
		strings.Contains(name, "workspace.files.write"),
		strings.Contains(name, "workspace.files.patch"),
		strings.Contains(name, "workspace.files.diff"),
		strings.Contains(name, "workspace.files.apply"),
		strings.Contains(name, "workspace.files.discard"):
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
		markAgentTodoByIndex(session.Todos, len(session.Todos)-1, "blocked", now, call.ID, agentToolError(call))
		if session.ActiveTask != nil {
			session.ActiveTask.Status = "blocked"
			session.ActiveTask.UpdatedAt = now
		}
		session.Status = "blocked"
		session.NextAction = &agentNextAction{Type: "blocked", Reason: agentToolError(call)}
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
		strings.Contains(name, "workspace.files.read"),
		strings.Contains(name, "workspace.files.write"),
		strings.Contains(name, "workspace.files.patch"),
		strings.Contains(name, "workspace.files.diff"),
		strings.Contains(name, "workspace.files.apply"),
		strings.Contains(name, "workspace.files.discard"),
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
	switch {
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

func normalizeAgentPlannerActionName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.ReplaceAll(name, "-", ".")
	name = strings.ReplaceAll(name, "_", ".")
	name = strings.Join(strings.Fields(name), ".")
	switch name {
	case "workspace.command", "workspace.commands.run", "workspace.run.command", "command", "commands.run", "shell", "terminal":
		return "workspace.command"
	case "workspace.files.list", "workspace.file.list", "workspace.list", "workspace.ls", "files.list", "file.list", "list.files", "list.file", "ls":
		return "workspace.files.list"
	case "workspace.files.read", "workspace.file.read", "workspace.read", "files.read", "file.read", "read.file", "cat":
		return "workspace.files.read"
	case "workspace.files.patch", "workspace.file.patch", "workspace.patch", "files.patch", "file.patch", "patch.file", "patch", "edit.file", "modify.file", "replace.in.file":
		return "workspace.files.patch"
	case "workspace.files.diff", "workspace.file.diff", "workspace.diff", "files.diff", "file.diff", "diff.file", "diff", "preview.file", "preview.patch", "stage.file", "stage.patch", "staged.diff":
		return "workspace.files.diff"
	case "workspace.files.diff.many", "workspace.files.diffmany", "workspace.files.diff_many", "workspace.file.diff.many", "workspace.diff.many", "files.diff.many", "diff.many", "preview.files", "stage.files", "stage.changeset", "staged.changeset":
		return "workspace.files.diff_many"
	case "workspace.files.rebase", "workspace.file.rebase", "workspace.rebase", "files.rebase", "file.rebase", "rebase.file", "regenerate.change", "regenerate.patch", "refresh.change", "rebase.patch":
		return "workspace.files.rebase"
	case "workspace.files.apply", "workspace.file.apply", "workspace.apply", "files.apply", "file.apply", "apply.file", "apply.patch", "apply.change", "apply", "commit.patch":
		return "workspace.files.apply"
	case "workspace.files.discard", "workspace.file.discard", "workspace.discard", "files.discard", "file.discard", "discard.file", "discard.patch", "discard.change", "discard", "drop.patch", "cancel.patch":
		return "workspace.files.discard"
	case "workspace.files.write", "workspace.file.write", "workspace.write", "files.write", "file.write", "write.file", "write", "save.file", "create.file", "replace.file":
		return "workspace.files.write"
	case "daemon.health", "daemon.doctor", "health", "doctor", "status":
		return "daemon.health"
	case "plugins.list", "plugin.list", "tools.list", "tool.list", "plugins", "tools":
		return "plugins.list"
	case "runs.list", "run.list", "history.list", "runs", "history":
		return "runs.list"
	case "runs.approvals.list", "run.approvals.list", "approvals.list", "approval.list", "approvals", "approval.queue":
		return "runs.approvals.list"
	case "runs.approvals.approve", "run.approvals.approve", "approvals.approve", "approval.approve", "approve.approval":
		return "runs.approvals.approve"
	case "runs.approvals.deny", "run.approvals.deny", "approvals.deny", "approval.deny", "deny.approval", "reject.approval":
		return "runs.approvals.deny"
	case "runs.resume.approved", "run.resume.approved", "resume.approved", "resume.approved.run", "runs.resume.approval":
		return "runs.resume_approved"
	default:
		return name
	}
}

func normalizeAgentPlannerActionArguments(value any) map[string]any {
	switch v := value.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			out[strings.TrimSpace(key)] = val
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, val := range v {
			out[strings.TrimSpace(key)] = val
		}
		return out
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return map[string]any{}
		}
		var parsed map[string]any
		if strings.HasPrefix(trimmed, "{") && json.Unmarshal([]byte(trimmed), &parsed) == nil {
			return normalizeAgentPlannerActionArguments(parsed)
		}
		return map[string]any{"input": trimmed}
	default:
		return map[string]any{"input": v}
	}
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
		if strings.Contains(name, "workspace.files.read") || strings.Contains(name, "workspace.files.write") || strings.Contains(name, "workspace.files.patch") || strings.Contains(name, "workspace.files.diff") || strings.Contains(name, "workspace.files.apply") || strings.Contains(name, "workspace.files.discard") {
			refs = append(refs, agentContextRef{
				ID:      "workspace_file:" + path,
				Kind:    "workspace_file",
				RefID:   path,
				Label:   "file " + path,
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

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
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
	cloned.MemorySummary = cloneAgentMemorySummary(session.MemorySummary)
	cloned.NextAction = cloneAgentNextAction(session.NextAction)
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
		agentIsWorkspaceWriteIntent(prompt) ||
		agentIsWorkspaceDiffIntent(prompt) ||
		agentIsWorkspaceRebaseIntent(prompt) ||
		agentIsWorkspaceApplyAllIntent(prompt) ||
		agentIsWorkspaceApplyIntent(prompt) ||
		agentIsWorkspaceDiscardAllIntent(prompt) ||
		agentIsWorkspaceDiscardIntent(prompt)
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

func agentFormatWorkspaceWrite(language, relPath, content string, size int) string {
	return agentText(language,
		fmt.Sprintf("已写入工作空间文件 `%s`（%d bytes）。\n\n内容预览：\n```text\n%s\n```", relPath, size, agentLimitBlock(content, 1200)),
		fmt.Sprintf("Wrote workspace file `%s` (%d bytes).\n\nPreview:\n```text\n%s\n```", relPath, size, agentLimitBlock(content, 1200)),
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
	case agentIsWorkspaceDiffIntent(prompt):
		relPath, oldText, newText := agentExtractWorkspacePatch(prompt)
		return m.handleAgentWorkspaceDiff(sessionID, relPath, oldText, newText, language)
	case agentIsWorkspacePatchIntent(prompt):
		relPath, oldText, newText := agentExtractWorkspacePatch(prompt)
		return m.handleAgentWorkspacePatch(sessionID, relPath, oldText, newText, language)
	case agentIsWorkspaceWriteIntent(prompt):
		relPath, content := agentExtractWorkspaceWrite(prompt)
		return m.handleAgentWorkspaceWrite(sessionID, relPath, content, language)
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
