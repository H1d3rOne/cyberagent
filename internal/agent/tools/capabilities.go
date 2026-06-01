package tools

import (
	"encoding/json"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

// ToolCapabilitySpec is the JSON shape surfaced by daemon capability and skill
// registry endpoints for each executable tool.
type ToolCapabilitySpec struct {
	Name          string          `json:"name"`
	Source        string          `json:"source"`
	CatalogPath   string          `json:"catalog_path,omitempty"`
	IntentTags    []string        `json:"intent_tags"`
	CanDo         []string        `json:"can_do"`
	CannotDo      []string        `json:"cannot_do,omitempty"`
	Risk          string          `json:"risk"`
	RequiredLevel string          `json:"required_level"`
	AutoRun       bool            `json:"auto_run"`
	Description   string          `json:"description"`
	InputSchema   json.RawMessage `json:"input_schema,omitempty"`
	Examples      []string        `json:"examples,omitempty"`
}

// ToolCapabilityMetadata holds derived, localized hints for capability output.
type ToolCapabilityMetadata struct {
	IntentTags []string
	CanDo      []string
	CannotDo   []string
	Examples   []string
	Source     string
}

type Capability struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}

type CapabilityEndpoint struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

type CapabilityStage struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type CapabilityTool struct {
	Name        string          `json:"name"`
	Risk        string          `json:"risk"`
	AutoRun     bool            `json:"auto_run"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type CapabilitySafety struct {
	ScopePolicy       string   `json:"scope_policy"`
	AutoExecutePolicy string   `json:"auto_execute_policy"`
	ApprovalRequired  []string `json:"approval_required"`
	CommandAllowlist  []string `json:"command_allowlist"`
}

type CapabilityItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func BuildCapabilityTools(defs []llm.ToolDef, policy func(name string) (risk string, autoRun bool)) []CapabilityTool {
	tools := make([]CapabilityTool, 0, len(defs))
	for _, tool := range defs {
		risk, autoRun := policy(tool.Name)
		tools = append(tools, CapabilityTool{
			Name:        tool.Name,
			Risk:        risk,
			AutoRun:     autoRun,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return tools
}

func BuildToolCapabilities(defs []llm.ToolDef, metadata map[string]ToolCapabilityMetadata, catalogPaths map[string]string, normalize func(string) string, policy func(string) (string, bool), requiredLevel func(string) string) []ToolCapabilitySpec {
	out := make([]ToolCapabilitySpec, 0, len(defs))
	for _, tool := range defs {
		name := normalize(tool.Name)
		if name == "" {
			name = tool.Name
		}
		risk, autoRun := policy(name)
		meta := metadata[name]
		source := meta.Source
		if source == "" {
			source = "builtin"
		}
		out = append(out, ToolCapabilitySpec{
			Name:          name,
			Source:        source,
			CatalogPath:   catalogPaths[name],
			IntentTags:    append([]string{}, meta.IntentTags...),
			CanDo:         append([]string{}, meta.CanDo...),
			CannotDo:      append([]string{}, meta.CannotDo...),
			Risk:          risk,
			RequiredLevel: requiredLevel(name),
			AutoRun:       autoRun,
			Description:   tool.Description,
			InputSchema:   tool.InputSchema,
			Examples:      append([]string{}, meta.Examples...),
		})
	}
	return out
}

func DefaultCapabilityEndpoints(language string) []CapabilityEndpoint {
	return []CapabilityEndpoint{
		{Method: "POST", Path: "/api/v1/agent/sessions", Description: capabilityText(language, "创建 Agent 会话。", "Create an Agent session.")},
		{Method: "GET", Path: "/api/v1/agent/sessions", Description: capabilityText(language, "列出 Agent 会话摘要。", "List Agent session summaries.")},
		{Method: "GET", Path: "/api/v1/agent/sessions/{session_id}", Description: capabilityText(language, "读取完整 Agent 会话。", "Read the full Agent session.")},
		{Method: "GET", Path: "/api/v1/agent/sessions/{session_id}/state", Description: capabilityText(language, "读取可恢复 Agent state：task、todos、tool calls、context refs、next action。", "Read recoverable Agent state: task, todos, tool calls, context refs, and next action.")},
		{Method: "GET", Path: "/api/v1/agent/sessions/{session_id}/context", Description: capabilityText(language, "读取轻量工作上下文：context refs、pending changes 和 run summary。", "Read lightweight working context: context refs, pending changes, and run summary.")},
		{Method: "GET", Path: "/api/v1/agent/sessions/{session_id}/trace", Description: capabilityText(language, "导出可回放 Agent trace：消息、工具、阶段、状态和上下文。", "Export replayable Agent trace: messages, tools, phases, state, and context.")},
		{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/messages", Description: capabilityText(language, "发送自然语言消息并触发 Agent 执行循环。", "Send a natural-language message and trigger the Agent execution loop.")},
		{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/control", Description: capabilityText(language, "通用控制入口：continue、revise、pause、stop、cancel、new_task。", "Generic control entrypoint: continue, revise, pause, stop, cancel, new_task.")},
		{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/continue", Description: capabilityText(language, "继续当前任务。", "Continue the current task.")},
		{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/authorization/approve", Description: capabilityText(language, "批准当前挂起的 Agent 工具授权，并恢复执行原动作。", "Approve the pending Agent tool authorization and resume the original action.")},
		{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/authorization/deny", Description: capabilityText(language, "拒绝当前挂起的 Agent 工具授权，并停止该动作。", "Deny the pending Agent tool authorization and stop that action.")},
		{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/pause", Description: capabilityText(language, "暂停当前会话但保留上下文。", "Pause the current session while preserving context.")},
		{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/stop", Description: capabilityText(language, "停止当前任务并尝试取消活动 daemon job。", "Stop the current task and try to cancel the active daemon job.")},
		{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/cancel", Description: capabilityText(language, "取消当前任务并保留会话记录。", "Cancel the current task while preserving session history.")},
		{Method: "POST", Path: "/api/v1/agent/sessions/{session_id}/new-task", Description: capabilityText(language, "开启新的任务上下文。", "Start a new task context.")},
		{Method: "GET", Path: "/api/v1/agent/sessions/{session_id}/stream", Description: capabilityText(language, "订阅 Agent session/activity 事件流。", "Subscribe to Agent session/activity events.")},
	}
}

func DefaultCapabilities(language string) []Capability {
	return []Capability{
		{
			ID:          "natural_language_ops",
			Label:       capabilityText(language, "自然语言运维/测试", "Natural-language ops/testing"),
			Description: capabilityText(language, "将操作者意图转为任务、待办、工具调用和下一步建议。", "Turns operator intent into tasks, todos, tool calls, and next-step suggestions."),
			Examples: []string{
				capabilityText(language, "检查 daemon 状态", "check daemon status"),
				capabilityText(language, "列出最近运行并总结失败项", "list recent runs and summarize failures"),
			},
		},
		{
			ID:          "agent_memory",
			Label:       capabilityText(language, "会话与上下文保持", "Session/context memory"),
			Description: capabilityText(language, "保持聊天会话、任务状态、工具轨迹、上下文引用和下一步动作。", "Keeps chat sessions, task state, tool traces, context references, and next actions."),
		},
		{
			ID:          "planner_tool_loop",
			Label:       capabilityText(language, "规划→工具→总结闭环", "Planner→tools→synthesis loop"),
			Description: capabilityText(language, "支持 JSON actions 与模型原生 tool_calls，执行低风险工具后回灌 observation 并生成最终总结。", "Supports JSON actions and native LLM tool_calls; after low-risk tool execution, observations are fed back for final synthesis."),
		},
		{
			ID:          "intent_capability_matching",
			Label:       capabilityText(language, "意图→能力匹配", "Intent→capability matching"),
			Description: capabilityText(language, "在执行前区分 Web 漏洞扫描、端口枚举、HTTP 探测、内容发现、JS 路由分析、认证/鉴权测试等意图；缺少等价工具时停在 Plan，不用相似工具替代。", "Distinguishes Web vulnerability scan, port enumeration, HTTP probe, content discovery, JS route analysis, auth/authorization testing, and other intents before execution; when no equivalent tool exists, stops at Plan instead of substituting a similar tool."),
			Examples: []string{
				capabilityText(language, "扫描目标有哪些开放端口 → port_scan，等待 nmap/masscan 插件或 HTTP fallback", "which ports are open → port_scan, wait for nmap/masscan plugin or HTTP fallback"),
				capabilityText(language, "找隐藏目录 → content_discovery，等待 ffuf/gobuster/dirsearch 等价工具", "find hidden directories → content_discovery, wait for ffuf/gobuster/dirsearch equivalent"),
				capabilityText(language, "获取响应头/状态码 → http_probe，调用 http.request", "fetch headers/status → http_probe, call http.request"),
			},
		},
		{
			ID:          "auto_continue_stepper",
			Label:       capabilityText(language, "自动续行步进器", "Auto-continue stepper"),
			Description: capabilityText(language, "当 next_action 标记为 auto_continue 且下一步是低风险只读动作时，“继续”会直接执行下一步工具并写回 observation。", "When next_action is marked auto_continue and the next step is a low-risk read-only action, Continue directly executes the next tool and persists the observation."),
		},
		{
			ID:          "web_api_security_runtime",
			Label:       capabilityText(language, "Web/API 安全运行时", "Web/API security runtime"),
			Description: capabilityText(language, "可按明确授权指令启动 safe profile 扫描、跟踪运行、收集证据、处理审批并复测。", "Can start safe-profile scans on explicit authorized instructions, track runs, collect evidence, handle approvals, and retest."),
		},
		{
			ID:          "workspace_tools",
			Label:       capabilityText(language, "Agent 工作区", "Agent workspace"),
			Description: capabilityText(language, "为每个会话提供隔离工作区，支持文件列表、读取和低风险白名单命令。", "Provides an isolated workspace per session with file listing, reading, and low-risk allowlisted commands."),
		},
	}
}

func DefaultCapabilityStages(language string) []CapabilityStage {
	return []CapabilityStage{
		{ID: "understand", Label: capabilityText(language, "理解", "Understand"), Description: capabilityText(language, "解析目标、scope、授权边界和操作者意图。", "Parse target, scope, authorization boundary, and operator intent.")},
		{ID: "plan", Label: capabilityText(language, "规划", "Plan"), Description: capabilityText(language, "生成 task/todos/next_action，并判断是否需要工具或人工确认。", "Generate task/todos/next_action and decide whether tools or human confirmation are needed.")},
		{ID: "tool_call", Label: capabilityText(language, "工具调用", "Tool call"), Description: capabilityText(language, "执行低风险内置工具；扫描和高风险动作需明确指令或审批。", "Execute low-risk built-ins; scans and higher-risk actions require explicit instruction or approval.")},
		{ID: "observation", Label: capabilityText(language, "观测", "Observation"), Description: capabilityText(language, "把工具结果沉淀为消息、工具轨迹和上下文引用。", "Persist tool results as messages, tool traces, and context references.")},
		{ID: "synthesis", Label: capabilityText(language, "总结", "Synthesis"), Description: capabilityText(language, "把 observation 回灌给 LLM，生成最终结论和下一步。", "Feed observations back to the LLM to produce final conclusions and next steps.")},
	}
}

func DefaultCapabilitySafety(language string, commandAllowlist []string) CapabilitySafety {
	return CapabilitySafety{
		ScopePolicy: capabilityText(
			language,
			"默认只处理授权目标；目标、scope 或授权边界不清晰时等待用户确认。",
			"Only authorized targets are handled by default; if target, scope, or authorization boundary is unclear, wait for user confirmation.",
		),
		AutoExecutePolicy: capabilityText(
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
		CommandAllowlist: commandAllowlist,
	}
}

func DefaultCapabilityIntegrations() []CapabilityItem {
	return []CapabilityItem{
		{ID: "llm_providers", Label: "LLM providers", Description: "OpenAI-compatible, Anthropic-compatible, and Ollama provider adapters."},
		{ID: "daemon_runtime", Label: "Daemon runtime", Description: "Jobs, runs, events, reports, approvals, and safe-profile Web/API scan runtime."},
		{ID: "plugins", Label: "Plugin registry", Description: "Built-in and external tool/plugin registry with policy gates."},
		{ID: "workspace", Label: "Workspace", Description: "Per-session files and allowlisted command execution sandbox."},
	}
}

func capabilityText(language, zh, en string) string {
	if strings.EqualFold(strings.TrimSpace(language), "en") || strings.EqualFold(strings.TrimSpace(language), "en-us") {
		return en
	}
	return zh
}
