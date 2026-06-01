package daemon

import (
	"fmt"
	"regexp"
	"strings"

	agenttools "github.com/H1d3rOne/cyberagent/internal/agent/tools"
)

// Deprecated: use tools.ToolCapabilitySpec from internal/agent/tools.
type agentToolCapabilitySpec = agenttools.ToolCapabilitySpec

// Deprecated: use tools.ToolCapabilityMetadata from internal/agent/tools.
type agentToolCapabilityMetadata = agenttools.ToolCapabilityMetadata

type agentCapabilitiesResponse struct {
	APIVersion    string                    `json:"api_version"`
	Name          string                    `json:"name"`
	Description   string                    `json:"description"`
	EntryPoint    string                    `json:"entry_point"`
	Endpoints     []agentCapabilityEndpoint `json:"endpoints"`
	Capabilities  []agentCapability         `json:"capabilities"`
	Stages        []agentCapabilityStage    `json:"stages"`
	BuiltinTools  []agentCapabilityTool     `json:"builtin_tools"`
	Skills        []agentSkillSpec          `json:"skills,omitempty"`
	ToolCaps      []agentToolCapabilitySpec `json:"tool_capabilities,omitempty"`
	SecurityTools []agentSecurityToolSpec   `json:"security_tools,omitempty"`
	MCP           agentMCPIntegrationSpec   `json:"mcp,omitempty"`
	Safety        agentCapabilitySafety     `json:"safety"`
	Integrations  []agentCapabilityItem     `json:"integrations"`
}

// Deprecated: use tools.Capability from internal/agent/tools.
type agentCapability = agenttools.Capability

// Deprecated: use tools.CapabilityEndpoint from internal/agent/tools.
type agentCapabilityEndpoint = agenttools.CapabilityEndpoint

// Deprecated: use tools.CapabilityStage from internal/agent/tools.
type agentCapabilityStage = agenttools.CapabilityStage

// Deprecated: use tools.CapabilityTool from internal/agent/tools.
type agentCapabilityTool = agenttools.CapabilityTool

// Deprecated: use tools.CapabilitySafety from internal/agent/tools.
type agentCapabilitySafety = agenttools.CapabilitySafety

// Deprecated: use tools.CapabilityItem from internal/agent/tools.
type agentCapabilityItem = agenttools.CapabilityItem

func agentCapabilities(language string) agentCapabilitiesResponse {
	language = normalizeAgentLanguage(language)
	registry := agentSkillRegistry(language)
	return agentCapabilitiesResponse{
		APIVersion: agentAPIVersion,
		Name:       "CyberAgent",
		Description: agentText(
			language,
			"面向授权目标的 Web/API 安全分析 Agent：通过自然语言维护任务、规划步骤、调用工具、沉淀上下文并生成可复现结论。",
			"Authorized-target Web/API security analysis Agent: maintains tasks, plans steps, calls tools, preserves context, and produces reproducible conclusions through natural language.",
		),
		EntryPoint:    "/api/v1/agent/sessions/{session_id}/messages",
		Endpoints:     agenttools.DefaultCapabilityEndpoints(language),
		Capabilities:  agenttools.DefaultCapabilities(language),
		Stages:        agenttools.DefaultCapabilityStages(language),
		BuiltinTools:  agenttools.BuildCapabilityTools(toolRegistry().LLMDefinitions(), agentCapabilityToolPolicy),
		Skills:        registry.Skills,
		ToolCaps:      registry.ToolCapabilities,
		SecurityTools: registry.SecurityTools,
		MCP:           registry.MCP,
		Safety:        agenttools.DefaultCapabilitySafety(language, agentPlannerCommandAllowlist()),
		Integrations:  agenttools.DefaultCapabilityIntegrations(),
	}
}

func agentCapabilityToolPolicy(name string) (string, bool) {
	spec, ok := toolRegistry().Lookup(name)
	if !ok {
		return "", false
	}
	return spec.Risk, spec.AutoRun
}

func agentToolCapabilities(language string) []agentToolCapabilitySpec {
	metadata := agentToolCapabilityMetadataByName(language)
	securityToolCatalog := agentSecurityToolCatalogByName()
	catalogPaths := make(map[string]string, len(securityToolCatalog))
	for name, tool := range securityToolCatalog {
		catalogPaths[name] = tool.Path
	}
	return agenttools.BuildToolCapabilities(
		toolRegistry().LLMDefinitions(),
		metadata,
		catalogPaths,
		normalizeAgentPlannerActionName,
		agentCapabilityToolPolicy,
		agentOperationLevelRequiredForAction,
	)
}

// agentToolCapabilityMetadataByName returns per-tool capability metadata for
// the agent's prompt and skill registry. The data is now derived from the
// canonical tool registry (toolregistry.Registry) so adding a new tool means
// editing one ToolSpec instead of also editing this map. Tools without any
// capability hints (IntentTags / CanDo / CannotDo / Examples) are dropped to
// preserve the legacy "tool absent from map" semantics — the parity test in
// agent_toolregistry_capability_parity_test.go pins this.
func agentToolCapabilityMetadataByName(language string) map[string]agentToolCapabilityMetadata {
	derived := toolRegistry().CapabilityMetadataByName(normalizeAgentLanguage(language))
	out := make(map[string]agentToolCapabilityMetadata, len(derived))
	for name, meta := range derived {
		if len(meta.IntentTags) == 0 && len(meta.CanDo) == 0 && len(meta.CannotDo) == 0 && len(meta.Examples) == 0 {
			continue
		}
		out[name] = agentToolCapabilityMetadata{
			IntentTags: meta.IntentTags,
			CanDo:      meta.CanDo,
			CannotDo:   meta.CannotDo,
			Examples:   meta.Examples,
		}
	}
	return out
}

func agentCapabilityRegistryPrompt(language string) string {
	registry := agentSkillRegistry(language)
	skillLines := make([]string, 0, len(registry.Skills))
	for _, skill := range registry.Skills {
		skillLines = append(skillLines, fmt.Sprintf("- %s: intents=%s tools=%s risk=%s", skill.ID, strings.Join(skill.IntentTags, ","), strings.Join(skill.RequiredTools, ","), skill.Risk))
	}
	toolLines := make([]string, 0, len(registry.ToolCapabilities))
	for _, tool := range registry.ToolCapabilities {
		if len(tool.IntentTags) == 0 && len(tool.CanDo) == 0 {
			continue
		}
		cannot := ""
		if len(tool.CannotDo) > 0 {
			cannot = " cannot=" + strings.Join(tool.CannotDo, ",")
		}
		toolLines = append(toolLines, fmt.Sprintf("- %s: intents=%s can=%s%s level=%s risk=%s", tool.Name, strings.Join(tool.IntentTags, ","), strings.Join(tool.CanDo, ","), cannot, tool.RequiredLevel, tool.Risk))
	}
	if normalizeAgentLanguage(language) == "en" {
		return "Skill / Tool Capability Registry:\nSkills:\n" + strings.Join(skillLines, "\n") + "\nTools:\n" + strings.Join(toolLines, "\n") + "\nUse skills to understand how to do the task; use tool capabilities to decide what can actually be executed. If a required tool is missing, stop at Plan and ask for the matching MCP/plugin or exact fallback input."
	}
	return "Skill / Tool Capability Registry：\nSkills:\n" + strings.Join(skillLines, "\n") + "\nTools:\n" + strings.Join(toolLines, "\n") + "\n用 skill 理解任务怎么做；用 tool capability 判断当前到底能执行什么。缺少 required tool 时停在 Plan，要求接入对应 MCP/plugin 或提供精确 fallback 输入。"
}

func agentWantsExternalTools(prompt string) bool {
	return regexp.MustCompile(`(?i)(external|plugin|tool|插件|外部工具|启用工具)`).MatchString(prompt)
}

const (
	agentIntentGeneralChat          = "general_chat"
	agentIntentWebVulnerabilityScan = "web_vulnerability_scan"
	agentIntentPortScan             = "port_scan"
	agentIntentHTTPProbe            = "http_probe"
	agentIntentContentDiscovery     = "content_discovery"
	agentIntentJSRouteAnalysis      = "js_route_analysis"
	agentIntentAPIAuthTest          = "api_auth_test"
	agentIntentAPIFunctionalTest    = "api_functional_test"
	agentIntentWebSearch            = "web_search"
	agentIntentWebFetch             = "web_fetch"
	agentIntentRunContext           = "run_context"
	agentIntentReportGeneration     = "report_generation"
	agentIntentWorkspace            = "workspace"
	agentIntentApproval             = "approval"
	agentIntentHealth               = "health"
	agentIntentPlugins              = "plugins"
	agentIntentRuns                 = "runs"
	agentIntentScheduleCreate       = "schedule_create"
	agentIntentScheduleList         = "schedule_list"
	agentIntentScheduleDelete       = "schedule_delete"
)

type agentCapabilityIntentDecision struct {
	ID           string
	Kind         string
	Target       string
	Supported    bool
	Tools        []string
	MissingTools []string
	Actions      []agentPlannerAction
	Reason       string
}

func agentClassifyCapabilityIntent(prompt string) agentCapabilityIntentDecision {
	prompt = strings.TrimSpace(prompt)
	target := agentExtractURL(prompt)
	switch {
	case agentIsScheduleCreateIntent(prompt):
		return agentCapabilityIntentDecision{
			ID:        agentIntentScheduleCreate,
			Kind:      "schedule",
			Target:    target,
			Supported: true,
			Tools:     []string{"schedule.create"},
			Reason:    "configure cron-driven recurring action",
		}
	case agentIsScheduleListIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentScheduleList, Kind: "schedule", Supported: true, Tools: []string{"schedule.list"}}
	case agentIsScheduleDeleteIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentScheduleDelete, Kind: "schedule", Supported: true, Tools: []string{"schedule.delete"}}
	case agentIsWebSearchIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentWebSearch, Kind: "web_search", Supported: true, Tools: []string{"web.search"}, Reason: "public web search / lookup"}
	case agentIsWebFetchIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentWebFetch, Kind: "web_fetch", Target: target, Supported: target != "", Tools: []string{"web.fetch"}, Reason: "read a specific public URL"}
	case agentIsPortScanIntent(prompt):
		if target == "" {
			target = agentExtractPortScanHostFromPrompt(prompt)
		}
		actions := agentPortScanActionsFromPrompt(prompt)
		return agentCapabilityIntentDecision{
			ID:        agentIntentPortScan,
			Kind:      "port_scan",
			Target:    target,
			Supported: len(actions) > 0,
			Tools:     []string{"ports.scan.safe"},
			Actions:   actions,
			Reason:    "safe TCP connect open-port probe",
		}
	case agentIsContentDiscoveryIntent(prompt):
		return agentCapabilityIntentDecision{
			ID:           agentIntentContentDiscovery,
			Kind:         "content_discovery",
			Target:       target,
			Supported:    false,
			MissingTools: []string{"content.discovery", "ffuf", "gobuster", "dirsearch"},
			Reason:       "hidden path/directory discovery requires a content-discovery tool",
		}
	case agentIsJSRouteAnalysisIntent(prompt):
		return agentCapabilityIntentDecision{
			ID:           agentIntentJSRouteAnalysis,
			Kind:         "js_route_analysis",
			Target:       target,
			Supported:    false,
			MissingTools: []string{"js.routes.analyze", "js_route_analyzer"},
			Reason:       "front-end JavaScript route extraction requires a JS route analyzer",
		}
	case agentIsAPIAuthTestIntent(prompt) && !agentIsExplicitHTTPRequestWithMethodOrRequest(prompt):
		return agentCapabilityIntentDecision{
			ID:           agentIntentAPIAuthTest,
			Kind:         "api_auth_test",
			Target:       target,
			Supported:    false,
			MissingTools: []string{"api.auth.test", "auth-flow runner"},
			Reason:       "authentication/authorization testing needs an auth-aware workflow, credentials, and exact endpoints",
		}
	case agentIsScanIntent(prompt):
		actions := agentHeuristicScanActionsFromPrompt(prompt)
		return agentCapabilityIntentDecision{
			ID:        agentIntentWebVulnerabilityScan,
			Kind:      "scan",
			Target:    target,
			Supported: len(actions) > 0,
			Tools:     []string{"scan.web.safe"},
			Actions:   actions,
			Reason:    "explicit Web/API safe-profile vulnerability/evidence scan",
		}
	case agentIsHTTPRequestIntent(prompt) || agentIsAPIFunctionalTestIntent(prompt):
		actions := agentHTTPProbeActionsFromPrompt(prompt)
		return agentCapabilityIntentDecision{
			ID:        agentIntentHTTPProbe,
			Kind:      "http_probe",
			Target:    target,
			Supported: len(actions) > 0,
			Tools:     []string{"http.request"},
			Actions:   actions,
			Reason:    "single HTTP request/probe is equivalent to the operator request",
		}
	case agentIsReportDraftIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentReportGeneration, Kind: "report", Supported: true, Tools: []string{"reports.draft"}}
	case agentIsRunContextAnalysisIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentRunContext, Kind: "context_analysis", Supported: true, Tools: []string{"runs.context"}}
	case agentIsApprovalIntent(prompt) || agentIsApprovalDecisionIntent(prompt) || agentIsResumeApprovedIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentApproval, Kind: "approval", Supported: true, Tools: []string{"runs.approvals.list", "runs.approvals.approve", "runs.approvals.deny", "runs.resume_approved"}}
	case agentIsHealthIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentHealth, Kind: "health", Supported: true, Tools: []string{"daemon.health"}}
	case agentIsPluginIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentPlugins, Kind: "tools", Supported: true, Tools: []string{"plugins.list"}}
	case agentIsRunIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentRuns, Kind: "runs", Supported: true, Tools: []string{"runs.list"}}
	case agentIsWorkspaceIntent(prompt) || agentIsCommandIntent(prompt):
		return agentCapabilityIntentDecision{ID: agentIntentWorkspace, Kind: "workspace", Supported: true, Tools: []string{"workspace.*"}}
	default:
		return agentCapabilityIntentDecision{ID: agentIntentGeneralChat, Kind: "chat", Target: target, Supported: true}
	}
}

func agentHasCapabilityGapIntent(prompt string) bool {
	decision := agentClassifyCapabilityIntent(prompt)
	return decision.ID != "" && decision.ID != agentIntentGeneralChat && !decision.Supported
}

func agentCapabilityGapPlannerFromPrompt(prompt, modelAnswer, language string) *agentPlannerResponse {
	decision := agentClassifyCapabilityIntent(prompt)
	if decision.Supported || decision.ID == "" || decision.ID == agentIntentGeneralChat {
		return nil
	}
	target := emptyDash(decision.Target)
	var answer string
	var suggestions []string
	var blockedTitle string
	var reason string
	switch decision.ID {
	case agentIntentPortScan:
		return agentPortScanPlannerFromPrompt(prompt, modelAnswer, language)
	case agentIntentContentDiscovery:
		answer = agentText(language,
			fmt.Sprintf("我理解这是内容发现/目录枚举任务，不是 Web 漏洞扫描。当前内置工具没有 ffuf/gobuster/dirsearch 这类内容发现器，因此不会用 scan.web.safe 代替目录爆破。目标：%s。可选：1）接入 ffuf/gobuster/dirsearch 插件后让我调用；2）提供具体路径，我用 HTTP 请求逐个验证；3）如果你的目标是漏洞证据扫描，改用 safe Web/API 扫描。", target),
			fmt.Sprintf("I understand this as content discovery/directory enumeration, not a Web vulnerability scan. The current built-ins do not include ffuf/gobuster/dirsearch-style content discovery, so I will not substitute scan.web.safe for directory brute force. Target: %s. Options: 1) add an ffuf/gobuster/dirsearch plugin; 2) provide exact paths and I can validate them with HTTP requests; 3) if you want vulnerability evidence scanning, switch to a safe Web/API scan.", target),
		)
		suggestions = []string{
			agentText(language, "列出插件工具，检查是否有 ffuf/gobuster/dirsearch", "list plugins and check for ffuf/gobuster/dirsearch"),
			agentText(language, "用 HTTP 请求验证我提供的具体路径", "validate the exact paths I provide with HTTP requests"),
			agentText(language, "改为对 "+target+" 做 safe Web/API 安全扫描", "switch to a safe Web/API scan for "+target),
		}
		blockedTitle = agentText(language, "等待内容发现工具或具体路径", "Wait for content-discovery tool or exact paths")
		reason = agentText(language, "缺少内容发现/目录枚举专用工具", "No dedicated content-discovery/directory-enumeration tool")
	case agentIntentJSRouteAnalysis:
		answer = agentText(language,
			fmt.Sprintf("我理解这是 JS 路由/前端接口提取任务。当前内置工具没有 js_route_analyzer，也没有浏览器/Playwright 工具（本阶段不做 Browser/Playwright），因此不会用 safe Web/API 扫描或单次 HTTP 请求冒充 JS 路由分析。目标：%s。可选：1）接入 js_route_analyzer 插件；2）把 JS bundle 放到 Agent 工作区，我用工作区搜索/读文件辅助提取；3）只请求入口页面并人工确认下一步。", target),
			fmt.Sprintf("I understand this as JS route/front-end endpoint extraction. The current built-ins do not include js_route_analyzer and Browser/Playwright tools are intentionally out of scope for now, so I will not pretend a safe Web/API scan or a single HTTP request is JS route analysis. Target: %s. Options: 1) add a js_route_analyzer plugin; 2) put JS bundles in the Agent workspace and I can use workspace search/read tools; 3) fetch only the entry page and let the operator confirm the next step.", target),
		)
		suggestions = []string{
			agentText(language, "列出插件工具，检查是否有 js_route_analyzer", "list plugins and check for js_route_analyzer"),
			agentText(language, "我会把 JS 文件放进工作区，请搜索接口路径", "I will place JS files in the workspace; search for API routes"),
			agentText(language, "只请求入口页面并返回响应摘要", "only request the entry page and summarize the response"),
		}
		blockedTitle = agentText(language, "等待 JS 分析插件或 JS 文件", "Wait for JS analyzer plugin or JS files")
		reason = agentText(language, "缺少 JS 路由分析专用工具", "No dedicated JS route analysis tool")
	case agentIntentAPIAuthTest:
		answer = agentText(language,
			fmt.Sprintf("我理解这是 API 认证/鉴权测试，不是普通漏洞扫描。当前内置工具只有单次 http.request 和 safe Web/API 扫描，没有完整的登录态、Token 轮换、越权矩阵或角色对比测试工作流，因此不会把它误当作 scan.web.safe。目标：%s。可选：1）先在设置里配置 Auth Profile，并给出具体 endpoint/method，我用 http.request 验证；2）接入 auth-flow 测试插件；3）如果只是做泛化安全扫描，再明确要求 safe scan。", target),
			fmt.Sprintf("I understand this as API authentication/authorization testing, not a generic vulnerability scan. Current built-ins only provide single http.request and safe Web/API scan; there is no full login-state/token-rotation/authorization-matrix workflow, so I will not misroute it to scan.web.safe. Target: %s. Options: 1) configure an Auth Profile and provide exact endpoint/method for http.request; 2) add an auth-flow testing plugin; 3) explicitly request safe scan if you only want a generic scan.", target),
		)
		suggestions = []string{
			agentText(language, "使用已配置 Auth Profile 请求具体 API endpoint", "use a configured Auth Profile to request an exact API endpoint"),
			agentText(language, "列出插件工具，检查是否有 auth-flow 测试插件", "list plugins and check for an auth-flow testing plugin"),
			agentText(language, "改为对 "+target+" 做 safe Web/API 安全扫描", "switch to a safe Web/API scan for "+target),
		}
		blockedTitle = agentText(language, "等待具体认证测试输入", "Wait for exact auth-test inputs")
		reason = agentText(language, "缺少认证/鉴权测试工作流与具体 endpoint/method/凭据上下文", "Missing auth/authorization workflow and exact endpoint/method/credential context")
	default:
		answer = agentText(language,
			"我识别到该任务当前没有等价的内置工具；为了避免误执行，我会停在 Plan 阶段等待你选择替代方案。",
			"I detected that this task has no equivalent built-in tool; to avoid incorrect execution I will stop at Plan and wait for an alternative.",
		)
		blockedTitle = agentText(language, "等待操作者选择替代方案", "Wait for operator alternative")
		reason = agentText(language, "没有等价内置工具", "No equivalent built-in tool")
	}
	if strings.TrimSpace(modelAnswer) != "" {
		answer = trimSummary(modelAnswer, 520)
	}
	nextPrompt := ""
	if len(suggestions) > 0 {
		nextPrompt = suggestions[0]
	}
	return &agentPlannerResponse{
		Phase:            "plan",
		Answer:           answer,
		SuggestedPrompts: suggestions,
		Task: &agentPlannerTask{
			Goal:    agentCapabilityGoal(language, decision, prompt),
			Kind:    decision.Kind,
			Status:  "blocked",
			Summary: reason,
		},
		Todos: []agentPlannerTodo{
			{Title: agentText(language, "Plan：识别真实任务意图", "Plan: identify the real task intent"), Status: "done", Priority: 1},
			{Title: agentText(language, "Plan：校验工具能力是否等价", "Plan: verify tool capability equivalence"), Status: "done", Priority: 2},
			{Title: blockedTitle, Status: "blocked", Priority: 3, BlockedReason: reason},
		},
		NextAction: &agentNextAction{
			Type:            "blocked",
			Reason:          reason,
			SuggestedPrompt: nextPrompt,
			AutoContinue:    false,
		},
		Reasoning: agentText(language,
			"该意图与现有内置工具不等价；Agent 停在 Plan 阶段，避免用相似工具制造错误结果。",
			"This intent is not equivalent to current built-in tools; the Agent stops at Plan to avoid producing incorrect results with a similar tool.",
		),
	}
}

func agentCapabilityGoal(language string, decision agentCapabilityIntentDecision, prompt string) string {
	target := emptyDash(firstNonEmpty(decision.Target, agentExtractURL(prompt)))
	switch decision.ID {
	case agentIntentContentDiscovery:
		return agentText(language, "发现隐藏路径/目录 "+target, "Discover hidden paths/directories for "+target)
	case agentIntentJSRouteAnalysis:
		return agentText(language, "提取 JS 路由和前端接口 "+target, "Extract JS routes and front-end endpoints for "+target)
	case agentIntentAPIAuthTest:
		return agentText(language, "测试 API 认证/鉴权 "+target, "Test API authentication/authorization for "+target)
	case agentIntentWebSearch:
		return agentText(language, "搜索公开 Web 信息", "Search public web information")
	case agentIntentWebFetch:
		return agentText(language, "读取公开 URL "+target, "Read public URL "+target)
	case agentIntentPortScan:
		return agentText(language, "枚举开放端口 "+target, "Enumerate open ports for "+target)
	default:
		return agentTaskGoal(language, decision.Kind, prompt)
	}
}

func agentPlannerActionCapabilityMismatch(prompt, actionName, language string) (string, string, bool) {
	actionName = normalizeAgentPlannerActionName(actionName)
	decision := agentClassifyCapabilityIntent(prompt)
	if decision.ID == "" || decision.ID == agentIntentGeneralChat {
		return "", "", false
	}
	allowed := false
	switch decision.ID {
	case agentIntentScheduleCreate:
		allowed = actionName == "schedule.create"
	case agentIntentScheduleList:
		allowed = actionName == "schedule.list"
	case agentIntentScheduleDelete:
		allowed = actionName == "schedule.delete"
	case agentIntentWebVulnerabilityScan:
		allowed = actionName == "scan.web.safe" || actionName == "http.request" || agentPlannerActionAllowedByPrompt(prompt, actionName)
	case agentIntentHTTPProbe:
		allowed = actionName == "http.request" || actionName == "plugins.list" || agentPlannerActionAllowedByPrompt(prompt, actionName)
	case agentIntentWebSearch:
		allowed = actionName == "web.search"
	case agentIntentWebFetch:
		allowed = actionName == "web.fetch"
	case agentIntentPortScan:
		allowed = actionName == "ports.scan.safe" || actionName == "plugins.list" || (actionName == "http.request" && agentIsHTTPPortFallbackIntent(prompt))
	case agentIntentContentDiscovery:
		allowed = actionName == "plugins.list" || actionName == "workspace.search" || actionName == "workspace.files.read" || (actionName == "http.request" && agentPromptMentionsSpecificPathProbe(prompt))
	case agentIntentJSRouteAnalysis:
		allowed = actionName == "plugins.list" || actionName == "workspace.search" || actionName == "workspace.files.read" || (actionName == "http.request" && agentPromptRequestsEntryPageOnly(prompt))
	case agentIntentAPIAuthTest:
		allowed = actionName == "plugins.list" || (actionName == "http.request" && agentIsExplicitHTTPRequestWithMethodOrRequest(prompt))
	default:
		return "", "", false
	}
	if allowed {
		return "", "", false
	}
	reason := agentText(language,
		fmt.Sprintf("已阻止非等价工具动作：当前任务意图是 `%s`，但模型计划执行 `%s`。这类任务必须使用等价工具或等待你选择替代方案，不能用相似工具代替。", decision.ID, actionName),
		fmt.Sprintf("Blocked a non-equivalent tool action: current task intent is `%s`, but the model planned `%s`. This task needs an equivalent tool or an operator-chosen fallback; a similar tool must not be substituted.", decision.ID, actionName),
	)
	suggested := ""
	if gap := agentCapabilityGapPlannerFromPrompt(prompt, "", language); gap != nil && gap.NextAction != nil {
		suggested = gap.NextAction.SuggestedPrompt
	}
	return reason, suggested, true
}
