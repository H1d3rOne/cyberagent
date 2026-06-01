package daemon

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

func agentLLMConfigured(req sendAgentMessageRequest) bool {
	provider := normalizeAgentProvider(req.Provider)
	if provider == "ollama" {
		return true
	}
	return strings.TrimSpace(req.APIKey) != "" || strings.TrimSpace(req.BaseURL) != ""
}

func agentInstructionsWithAuthProfiles(language, instructions string, profiles []agentHTTPAuthProfile, defaultID string) string {
	hint := agentAuthProfilesPlannerHint(language, profiles, defaultID)
	instructions = strings.TrimSpace(instructions)
	if hint == "" {
		return instructions
	}
	if instructions == "" {
		return hint
	}
	return instructions + "\n\n" + hint
}

func agentAuthProfilesPlannerHint(language string, profiles []agentHTTPAuthProfile, defaultID string) string {
	items := make([]string, 0, minInt(len(profiles), 8))
	for _, profile := range profiles {
		if !agentHTTPAuthProfileEnabled(profile) {
			continue
		}
		id := strings.TrimSpace(profile.ID)
		if id == "" {
			continue
		}
		label := firstNonEmpty(profile.Name, id)
		typ := normalizeAgentHTTPAuthProfileType(profile.Type)
		baseURL := strings.TrimSpace(profile.BaseURL)
		mark := ""
		if strings.EqualFold(id, strings.TrimSpace(defaultID)) {
			mark = " default"
		}
		if baseURL != "" {
			items = append(items, fmt.Sprintf("- %s (%s%s, match=%s)", label, typ, mark, baseURL))
		} else {
			items = append(items, fmt.Sprintf("- %s (%s%s)", label, typ, mark))
		}
		if len(items) >= 8 {
			break
		}
	}
	if len(items) == 0 {
		return ""
	}
	if language == "zh" {
		return "可用 API Auth Profiles（只暴露 ID/名称，不暴露密钥；http.request 需要时在 arguments.auth_profile_id 中填对应 ID，默认 profile 只会在 host 匹配时自动应用）：\n" + strings.Join(items, "\n")
	}
	return "Available API auth profiles (IDs/names only, secrets are not exposed; set arguments.auth_profile_id on http.request when needed. The default profile is only auto-applied when the host matches):\n" + strings.Join(items, "\n")
}

func agentNeedsDaemonTool(prompt string) bool {
	return agentIsControlIntent(prompt) ||
		agentIsResumeApprovedIntent(prompt) ||
		agentIsReportDraftIntent(prompt) ||
		agentIsPortScanIntent(prompt) ||
		agentIsRunContextAnalysisIntent(prompt) ||
		agentIsScanIntent(prompt) ||
		agentIsApprovalIntent(prompt) ||
		agentIsHealthIntent(prompt) ||
		agentIsPluginIntent(prompt) ||
		agentIsRunIntent(prompt) ||
		agentIsHTTPRequestIntent(prompt) ||
		agentIsAPIFunctionalTestIntent(prompt) ||
		agentHasCapabilityGapIntent(prompt) ||
		agentIsWorkspaceSearchIntent(prompt) ||
		agentIsWorkspaceTreeIntent(prompt) ||
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
	return llm.WithRetry(provider, agentLLMRetryConfig())
}

func agentLLMRetryConfig() llm.RetryConfig {
	return llm.RetryConfig{
		MaxRetries:     2,
		BaseDelay:      800 * time.Millisecond,
		MaxDelay:       5 * time.Second,
		AttemptTimeout: 25 * time.Second,
	}
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

	switch normalizeAgentProvider(provider) {
	case "openai", "ollama":
		return llm.NormalizeOpenAIBaseURL(raw)
	case "anthropic":
		return llm.NormalizeAnthropicBaseURL(raw)
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

func agentOperationPermissionPrompt(language, operationLevel string) string {
	operationLevel = normalizeAgentOperationLevel(operationLevel)
	switch operationLevel {
	case "read_only":
		return agentText(language,
			"\n\n当前 Agent 操作权限：read_only（只读）。只能请求 daemon.health、plugins.list、runs.list、runs.context、runs.approvals.list、workspace.files.list、workspace.files.tree、workspace.files.read、workspace.search、workspace.command（白名单只读命令）。不要输出会修改文件、发起 HTTP 请求、启动扫描、批准/拒绝审批或恢复运行的 actions；如用户要求这些操作，answer 要明确提示需要切换到对应权限级别。",
			"\n\nCurrent Agent operation level: read_only. You may only request daemon.health, plugins.list, runs.list, runs.context, runs.approvals.list, workspace.files.list, workspace.files.tree, workspace.files.read, workspace.search, and workspace.command with allowlisted read-only commands. Do not output actions that modify files, make HTTP requests, start scans, approve/deny approvals, or resume runs; if requested, tell the operator which higher level is required.",
		)
	case "workspace_write":
		return agentText(language,
			"\n\n当前 Agent 操作权限：workspace_write（工作区写入）。可以执行只读动作，并可在 Agent 工作区写入、patch、diff、rebase、apply、discard、delete、move、copy、mkdir。不能启动 Web/API 扫描，不能批准/拒绝审批或恢复运行；如用户要求这些操作，提示切换到 safe_scan 或 approval_ops。",
			"\n\nCurrent Agent operation level: workspace_write. You may perform read-only actions and workspace write/patch/diff/rebase/apply/discard/delete/move/copy/mkdir actions. You may not start Web/API scans, approve/deny approvals, or resume runs; if requested, ask the operator to switch to safe_scan or approval_ops.",
		)
	case "safe_scan":
		return agentText(language,
			"\n\n当前 Agent 操作权限：safe_scan（安全扫描）。可以执行工作区能力，并可在用户明确给出授权目标与 scope 时发起低风险 HTTP 请求、执行有界 TCP 端口探测或通过 Runtime 启动 safe profile Web/API 扫描。不能批准/拒绝审批或恢复运行；如用户要求审批操作，提示切换到 approval_ops。",
			"\n\nCurrent Agent operation level: safe_scan. You may use workspace capabilities and make low-risk HTTP requests, run bounded TCP port probes, or start safe-profile Web/API scans through the Runtime when the operator clearly provides an authorized target and scope. You may not approve/deny approvals or resume runs; ask for approval_ops when needed.",
		)
	case "approval_ops":
		return agentText(language,
			"\n\n当前 Agent 操作权限：approval_ops（审批操作）。可以执行 safe_scan 及以下能力，并可在用户明确指令下批准/拒绝审批、恢复已批准运行。仍需遵守授权目标、scope、白名单命令和显式用户意图。",
			"\n\nCurrent Agent operation level: approval_ops. You may use safe_scan and lower capabilities, and approve/deny approvals or resume approved runs only on explicit operator instruction. Still enforce authorized target, scope, command allowlists, and explicit operator intent.",
		)
	case "advanced":
		return agentText(language,
			"\n\n当前 Agent 操作权限：advanced（高级）。可请求当前已实现的内置工具动作，但仍要遵守授权目标、scope、白名单命令、审批 gate 和显式用户意图；不要越过尚未实现或未授权的能力。",
			"\n\nCurrent Agent operation level: advanced. You may request currently implemented built-in tool actions, but must still enforce authorized targets, scope, command allowlists, approval gates, and explicit operator intent; do not bypass unimplemented or unauthorized capabilities.",
		)
	case "bypass":
		return agentText(language,
			"\n\n当前 Agent 操作权限：bypass（跳过权限确认）。这是类似 Claude Code bypass permissions 的本地沙箱模式：在当前会话中，已授权范围内的内置工具动作可跳过前置人工确认和权限升级弹窗，直接执行。仍需保持目标/scope 语义清晰，并只处理操作者授权目标。",
			"\n\nCurrent Agent operation level: bypass. This is a local sandbox mode similar to Claude Code bypass permissions: built-in tool actions within the authorized scope may skip pre-execution manual confirmation and operation-level upgrade prompts. Keep target/scope semantics clear and only handle operator-authorized targets.",
		)
	default:
		return agentOperationPermissionPrompt(language, "read_only")
	}
}

func agentPlannerSystemPrompt(language, instructions, operationLevel string) string {
	base := agentText(
		language,
		`你是 CyberAgent，一个面向授权目标的 Web/API 安全分析 Agent。你不是普通聊天机器人；你需要像 Claude Code / Codex 一样维护当前任务、计划、上下文和下一步动作。

只输出一个 JSON 对象，不要 Markdown，不要代码围栏，不要在 JSON 前后输出解释文本。JSON schema：
{
  "phase": "plan",
  "answer": "给操作者看的简洁回复，说明你理解了什么、计划了什么、还需要什么",
  "task": {
    "goal": "当前任务目标",
    "kind": "chat|triage|planning|ops|scan|port_scan|context_analysis|approval|tools|runs|health",
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
      "name": "scan.web.safe|ports.scan.safe|http.request|web.search|web.fetch|workspace.command|workspace.files.list|workspace.files.tree|workspace.files.read|workspace.search|workspace.files.write|workspace.files.patch|workspace.files.diff|workspace.files.diff_many|workspace.files.rebase|workspace.files.apply|workspace.files.discard|workspace.files.delete|workspace.files.move|workspace.files.copy|workspace.files.mkdir|daemon.health|plugins.list|runs.list|runs.context|reports.draft|runs.approvals.list|runs.approvals.approve|runs.approvals.deny|runs.resume_approved|schedule.create|schedule.list|schedule.delete",
      "arguments": {"url":"https://example.com","method":"GET","auth_profile_id":"optional_profile_id","headers":{"X-Test":"value"},"body":"optional POST body","content_type":"application/json","path": ".", "query":"TODO", "command": "pwd", "content": "notes", "old": "before", "new": "after", "changes": [{"path": "a.md", "old": "before", "new": "after"}], "change_id": "change_xxx", "group_id": "changeset_xxx", "all": true, "run_id": "job_xxx", "approval_id": "approval_xxx", "step_id": "step_xxx", "comment": "operator comment"}
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
- 工具选择前必须先做“任务意图 → 能力匹配”：区分 web_vulnerability_scan、port_scan、http_probe、content_discovery、js_route_analysis、api_auth_test、api_functional_test、run_context、workspace、approval、report、health、plugins、general_chat。只有当用户目标与工具能力等价时才调用工具；如果没有等价工具，必须说明能力缺口并给出选项，不允许用相似但不等价的工具替代。
- auto_continue：当任务尚未完成、有明确下一步、且不需要人工审批或确认时设为 true；当任务已完成、需要人工输入、或存在不确定风险时设为 false。
- reasoning：简要说明你的思考过程，包括任务选择理由、规划理由、下一步理由和风险考虑。
- 可直接请求内置工具：scan.web.safe、ports.scan.safe、http.request、web.search、web.fetch、workspace.command、workspace.files.list、workspace.files.tree、workspace.files.read、workspace.search、workspace.files.write、workspace.files.patch、workspace.files.diff、workspace.files.diff_many、workspace.files.rebase、workspace.files.apply、workspace.files.discard、workspace.files.delete、workspace.files.move、workspace.files.copy、workspace.files.mkdir、daemon.health、plugins.list、runs.list、runs.context、reports.draft、runs.approvals.list、runs.approvals.approve、runs.approvals.deny、runs.resume_approved、schedule.create、schedule.list、schedule.delete。只有用户明确要求且动作属于这些内置工具时，才写入 actions 或 tool_calls；approval approve/deny 与 resume-approved 必须来自最新用户消息的显式指令；当用户给出明确授权目标/scope 且当前权限允许 safe_scan 时，可以直接输出 scan.web.safe、ports.scan.safe、web.search、web.fetch 或 GET/HEAD/OPTIONS http.request；POST/PUT/PATCH/DELETE http.request 需要 advanced；workspace.files.delete、workspace.files.move、workspace.files.copy、workspace.files.mkdir 和 reports.draft 需要 workspace_write；schedule.create 与 schedule.delete 需要 advanced；外部插件和高风险命令不要自动执行，应等待用户确认或建议使用对应 Runtime 指令。
- 当用户出现“定时/周期性/每天/每小时/每隔/每周/cron/scheduled/recurring/7:20执行”等关键词且明确要让某动作在未来或循环执行时，使用 schedule.create，不能调用立即执行工具。参数必须含 cron（5 字段标准 cron 或 @every 1h/@daily 这类宏）+ action。默认 action=agent_prompt，并把 prompt 设为去掉定时描述后的可执行指令（例如“端口扫描 example.com ports=80,443”）；这条路径可定时执行任意 Agent 能力。定时 safe Web/API vulnerability scan 也使用 action=agent_prompt，并把 prompt 写成可执行扫描指令。"列出所有定时/查看定时/有哪些定时" → schedule.list；"删除定时 X/取消定时 X" → schedule.delete + id。
- 端口枚举/开放端口查询（如“开了哪些端口”“open ports”“nmap”）必须归类为 port_scan，不要用 scan.web.safe 代替。可用 ports.scan.safe 做有界 TCP connect 安全探测；如果用户要求 nmap 全量服务识别、UDP 或超大范围扫描，必须说明内置工具限制并建议接入 nmap/masscan 插件。
- 内容发现/目录枚举（ffuf/gobuster/dirsearch/隐藏目录/路径爆破）、JS 路由提取、认证/鉴权矩阵测试都不是 scan.web.safe 的等价能力；缺少对应插件或具体 endpoint/method/Auth Profile 时必须停在 Plan/wait_user，不要申请授权去执行错误工具。
- 固定执行范式：Plan=分析任务全貌/选择工具/制定顺序；Step=逐步执行工具；StepReplan=每步后根据 observation 评估并调整后续；FinalAnswer=独立评估完成度，若遗漏则回流 Plan。
- 首次规划 phase 必须是 plan；如需要工具，actions 必须给出明确工具名和参数。
- todos 要体现 Agent 工作台能力：理解目标、确认 scope、选择安全策略、执行/建议 Runtime 动作、收集证据、总结报告、复测。
- answer 使用中文。`,
		`You are CyberAgent, an authorized-target Web/API security analysis Agent. You are not a generic chatbot; maintain the current task, plan, context, and next action like Claude Code / Codex.

Return exactly one JSON object. Do not use Markdown, code fences, or text before/after the JSON. JSON schema:
{
  "phase": "plan",
  "answer": "Concise operator-facing reply: what you understood, planned, and still need",
  "task": {
    "goal": "Current task goal",
    "kind": "chat|triage|planning|ops|scan|port_scan|context_analysis|approval|tools|runs|health",
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
      "name": "scan.web.safe|ports.scan.safe|http.request|web.search|web.fetch|workspace.command|workspace.files.list|workspace.files.tree|workspace.files.read|workspace.search|workspace.files.write|workspace.files.patch|workspace.files.diff|workspace.files.diff_many|workspace.files.rebase|workspace.files.apply|workspace.files.discard|workspace.files.delete|workspace.files.move|workspace.files.copy|workspace.files.mkdir|daemon.health|plugins.list|runs.list|runs.context|reports.draft|runs.approvals.list|runs.approvals.approve|runs.approvals.deny|runs.resume_approved|schedule.create|schedule.list|schedule.delete",
      "arguments": {"url":"https://example.com","method":"GET","auth_profile_id":"optional_profile_id","headers":{"X-Test":"value"},"body":"optional POST body","content_type":"application/json","path": ".", "query":"TODO", "command": "pwd", "content": "notes", "old": "before", "new": "after", "changes": [{"path": "a.md", "old": "before", "new": "after"}], "change_id": "change_xxx", "group_id": "changeset_xxx", "all": true, "run_id": "job_xxx", "approval_id": "approval_xxx", "step_id": "step_xxx", "comment": "operator comment"}
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
- Before choosing tools, perform "task intent → capability matching": distinguish web_vulnerability_scan, port_scan, http_probe, content_discovery, js_route_analysis, api_auth_test, api_functional_test, run_context, workspace, approval, report, health, plugins, and general_chat. Only call a tool when the operator goal is equivalent to that tool's capability; if no equivalent tool exists, explain the capability gap and offer options. Never substitute a similar but non-equivalent tool.
- auto_continue: set to true when the task is not finished, there is a clear next step, and no human approval or confirmation is needed; set to false when the task is complete, human input is required, or there is uncertain risk.
- reasoning: briefly explain your thinking process, including task selection reason, planning reason, next step reason, and risk considerations.
- You may request built-in tools directly: scan.web.safe, ports.scan.safe, http.request, web.search, web.fetch, workspace.command, workspace.files.list, workspace.files.tree, workspace.files.read, workspace.search, workspace.files.write, workspace.files.patch, workspace.files.diff, workspace.files.diff_many, workspace.files.rebase, workspace.files.apply, workspace.files.discard, workspace.files.delete, workspace.files.move, workspace.files.copy, workspace.files.mkdir, daemon.health, plugins.list, runs.list, runs.context, reports.draft, runs.approvals.list, runs.approvals.approve, runs.approvals.deny, runs.resume_approved, schedule.create, schedule.list, schedule.delete. Only populate actions or tool_calls when the user clearly asks for it and the action is one of these built-ins; approval approve/deny and resume-approved must come from an explicit instruction in the latest user message; when the operator provides an explicitly authorized target/scope and the current permission allows safe_scan, you may directly output scan.web.safe, ports.scan.safe, web.search, web.fetch, or GET/HEAD/OPTIONS http.request; POST/PUT/PATCH/DELETE http.request requires advanced; workspace.files.delete, workspace.files.move, workspace.files.copy, workspace.files.mkdir and reports.draft require workspace_write; schedule.create and schedule.delete require advanced; do not auto-run external plugins or high-risk commands.
- When the user asks for a scheduled / recurring / every-N / cron / daily / hourly / weekly action, or a future-time trigger like "run at 7:20", use schedule.create and do not call the immediate tool. Required args: cron (standard 5-field cron, or @every 1h / @daily macros) and action. Default to action=agent_prompt with prompt set to the executable instruction after stripping schedule wording (for example: "port scan example.com ports=80,443"); this supports arbitrary scheduled Agent capabilities. Scheduled safe Web/API vulnerability scans also use action=agent_prompt with an executable scan prompt. "list schedules / show schedules" → schedule.list; "delete schedule X / cancel schedule X" → schedule.delete with id.
- Port enumeration / open-port requests (for example "open ports", "which ports", "nmap") must be classified as port_scan and must not be substituted with scan.web.safe. Use ports.scan.safe for bounded TCP connect safe probes; if the operator asks for full nmap service fingerprinting, UDP, or very large ranges, state the built-in limitation and suggest an nmap/masscan plugin.
- Content discovery/directory enumeration (ffuf/gobuster/dirsearch/hidden directories/path brute force), JS route extraction, and auth/authorization matrix testing are not equivalent to scan.web.safe; without the matching plugin or exact endpoint/method/Auth Profile, stop at Plan/wait_user instead of asking approval for the wrong tool.
- Fixed execution pattern: Plan=analyze full task/select tools/order steps; Step=execute tools step by step; StepReplan=after each step, evaluate observations and adjust remaining steps; FinalAnswer=independently evaluate completion, and if something is missing, flow back to Plan.
- The first planning response phase must be plan; if tools are needed, actions must contain explicit tool names and arguments.
- todos should make the Agent cockpit useful: understand target, confirm scope, choose safety policy, execute/suggest Runtime action, collect evidence, summarize report, retest.
- answer must be in English.`,
	)
	base = base + "\n\n" + agentCapabilityRegistryPrompt(language)
	instructions = strings.TrimSpace(instructions)
	permission := agentOperationPermissionPrompt(language, operationLevel)
	if instructions == "" {
		return applyToolEnumPlaceholders(base + permission)
	}
	return applyToolEnumPlaceholders(base + permission + "\n\n" + agentText(language, "操作者附加指令：", "Additional operator instructions:") + "\n" + instructions)
}

func agentToolSynthesisSystemPrompt(language, instructions, operationLevel string) string {
	base := agentText(
		language,
		`你是 CyberAgent，一个面向授权目标的 Web/API 安全分析 Agent。你刚刚请求的低风险内置工具已经执行完毕；现在你要读取 observation，给操作者一个最终总结和下一步建议。

只输出一个 JSON 对象，不要 Markdown，不要代码围栏，不要在 JSON 前后输出解释文本。JSON schema：
{
  "phase": "step_replan|final_answer",
  "answer": "简洁总结：执行了哪些工具、关键结果是什么、下一步怎么做",
  "task": {
    "goal": "当前任务目标",
    "kind": "chat|triage|planning|ops|scan|port_scan|context_analysis|approval|tools|runs|health",
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
      "name": "scan.web.safe|ports.scan.safe|http.request|web.search|web.fetch|workspace.command|workspace.files.list|workspace.files.tree|workspace.files.read|workspace.search|workspace.files.write|workspace.files.patch|workspace.files.diff|workspace.files.diff_many|workspace.files.rebase|workspace.files.apply|workspace.files.discard|workspace.files.delete|workspace.files.move|workspace.files.copy|workspace.files.mkdir|daemon.health|plugins.list|runs.list|runs.context|reports.draft|runs.approvals.list|runs.approvals.approve|runs.approvals.deny|runs.resume_approved|schedule.create|schedule.list|schedule.delete",
      "arguments": {"target":"https://example.com","scope":"https://example.com","url":"https://example.com","method":"GET","auth_profile_id":"optional_profile_id","headers":{"X-Test":"value"},"body":"optional POST body","content_type":"application/json","path":".","query":"TODO","command":"pwd","run_id":"job_xxx"}
    }
  ],
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
- 这是 StepReplan/FinalAnswer 阶段：必须先基于 observation 判断任务是否完成。若仍需低风险下一步且不需要人工确认，可输出 phase=step_replan 并给出 actions；若已完成，输出 phase=final_answer 且 actions 为空。
	- 只能基于 observation 总结，不要编造未出现的命令输出、扫描结果或漏洞。
	- 如工具失败，要说明失败点和可恢复步骤。
	- 重新规划时必须检查“用户目标是否与已执行工具能力等价”；如果 observation 来自相近但不等价工具，必须说明覆盖范围和缺口，不要把部分结果包装成完整完成。
	- auto_continue：当 observation 已给出足够上下文、任务仍未完成、有明确的低风险下一步、且不需要人工确认/审批时可设为 true；如果下一步需要用户补充目标、scope、凭据、授权边界、审批或风险确认，必须设为 false。
	- 默认只讨论和处理已授权目标。
	- 如果用户目标是端口枚举/开放端口查询，只能把 ports.scan.safe 结果总结为“有界 TCP connect 安全探测”；不要把 HTTP 响应或 Web safe scan 总结成端口扫描，也不要把该结果夸大成 nmap 全量服务识别。
	- 如果用户目标是内容发现、JS 路由提取或认证/鉴权测试，不要把 safe Web/API 扫描或单次 HTTP 请求总结成完整完成；必须说明覆盖范围、缺口和需要的插件/输入。
	- answer 使用中文。`,
		`You are CyberAgent, an authorized-target Web/API security analysis Agent. The low-risk built-in tools you requested have just completed; read the observations and provide a final synthesis plus next-step recommendation.

Return exactly one JSON object. Do not use Markdown, code fences, or text before/after the JSON. JSON schema:
{
  "phase": "step_replan|final_answer",
  "answer": "Concise synthesis: which tools ran, key results, and what to do next",
  "task": {
    "goal": "Current task goal",
    "kind": "chat|triage|planning|ops|scan|port_scan|context_analysis|approval|tools|runs|health",
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
      "name": "scan.web.safe|ports.scan.safe|http.request|web.search|web.fetch|workspace.command|workspace.files.list|workspace.files.tree|workspace.files.read|workspace.search|workspace.files.write|workspace.files.patch|workspace.files.diff|workspace.files.diff_many|workspace.files.rebase|workspace.files.apply|workspace.files.discard|workspace.files.delete|workspace.files.move|workspace.files.copy|workspace.files.mkdir|daemon.health|plugins.list|runs.list|runs.context|reports.draft|runs.approvals.list|runs.approvals.approve|runs.approvals.deny|runs.resume_approved|schedule.create|schedule.list|schedule.delete",
      "arguments": {"target":"https://example.com","scope":"https://example.com","url":"https://example.com","method":"GET","auth_profile_id":"optional_profile_id","headers":{"X-Test":"value"},"body":"optional POST body","content_type":"application/json","path":".","query":"TODO","command":"pwd","run_id":"job_xxx"}
    }
  ],
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
- This is the StepReplan/FinalAnswer phase: first evaluate whether the task is complete from observations. If a low-risk next step is still needed and does not require human confirmation, output phase=step_replan with actions; if complete, output phase=final_answer with empty actions.
	- Synthesize only from observations; do not invent command output, scan results, or vulnerabilities.
	- If a tool failed, state the failure and recovery step.
	- During replanning, verify whether the operator goal is equivalent to the executed tool capability. If the observation came from a similar but non-equivalent tool, explicitly state coverage and gaps; do not present partial results as full completion.
	- auto_continue: set true only when the observations provide enough context, the task is still unfinished, there is a clear low-risk next step, and no human confirmation/approval is needed. If the next step needs target/scope/credential/authorization input, approval, or risk confirmation, set false.
	- Only discuss and handle authorized targets by default.
	- If the operator's goal is port enumeration / open-port discovery, summarize ports.scan.safe only as a bounded TCP connect safe probe; do not summarize HTTP responses or Web safe scans as port scanning, and do not overstate it as full nmap service fingerprinting.
	- If the operator's goal is content discovery, JS route extraction, or auth/authorization testing, do not summarize a safe Web/API scan or a single HTTP request as complete coverage; state the coverage, gaps, and needed plugin/inputs.
	- answer must be in English.`,
	)
	base = base + "\n\n" + agentCapabilityRegistryPrompt(language)
	instructions = strings.TrimSpace(instructions)
	permission := agentOperationPermissionPrompt(language, operationLevel)
	if instructions == "" {
		return applyToolEnumPlaceholders(base + permission)
	}
	return applyToolEnumPlaceholders(base + permission + "\n\n" + agentText(language, "操作者附加指令：", "Additional operator instructions:") + "\n" + instructions)
}

func agentPlannerFromPlainAnswer(answer, phase, language string) *agentPlannerResponse {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return nil
	}
	phase = firstNonEmpty(agentNormalizePhase(phase), "final_answer")
	status := "done"
	if phase == "plan" || phase == "step_replan" {
		status = "running"
	}
	return &agentPlannerResponse{
		Phase:  phase,
		Answer: answer,
		Task: &agentPlannerTask{
			Goal:    agentText(language, "回答并澄清安全任务", "Answer and clarify the security task"),
			Kind:    "chat",
			Status:  status,
			Summary: trimSummary(answer, 220),
		},
		Todos: []agentPlannerTodo{
			{Title: agentText(language, "理解用户意图", "Understand the operator intent"), Status: "done", Priority: 1},
			{Title: agentText(language, "返回主要结论", "Return the main conclusion"), Status: status, Priority: 2},
		},
		NextAction: &agentNextAction{
			Type:         "finished",
			Reason:       agentText(language, "模型返回了普通文本，已作为最终回答处理。", "The model returned plain text, so it was treated as the final answer."),
			AutoContinue: false,
		},
	}
}

func agentFallbackPlannerFromPrompt(prompt, modelAnswer, language string) *agentPlannerResponse {
	prompt = strings.TrimSpace(prompt)
	modelAnswer = strings.TrimSpace(modelAnswer)
	if prompt == "" {
		return agentPlannerFromPlainAnswer(modelAnswer, "final_answer", language)
	}

	actions := agentHeuristicActionsFromPrompt(prompt)
	if len(actions) == 0 {
		if gap := agentCapabilityGapPlannerFromPrompt(prompt, modelAnswer, language); gap != nil {
			return gap
		}
	}

	if len(actions) == 0 {
		if modelAnswer != "" {
			return agentPlannerFromPlainAnswer(modelAnswer, "final_answer", language)
		}
		return nil
	}

	answer := agentText(
		language,
		"我会按 Agent 执行环处理：先规划任务，再逐步调用内置工具，完成后根据 observation 重规划，最后给出独立总结。",
		"I will use the Agent execution loop: plan the task, call built-in tools step by step, replan from observations, then provide an independent final answer.",
	)
	if modelAnswer != "" {
		answer = trimSummary(modelAnswer, 360)
	}
	kind := agentTaskKind(prompt)
	return &agentPlannerResponse{
		Phase:  "plan",
		Answer: answer,
		Task: &agentPlannerTask{
			Goal:    agentTaskGoal(language, kind, prompt),
			Kind:    kind,
			Status:  "running",
			Summary: agentText(language, "已根据用户指令生成可执行工具计划。", "Generated an executable tool plan from the operator instruction."),
		},
		Todos: []agentPlannerTodo{
			{Title: agentText(language, "Plan：分析任务与选择工具", "Plan: analyze task and choose tools"), Status: "done", Priority: 1},
			{Title: agentText(language, "Step：执行内置工具", "Step: execute built-in tools"), Status: "running", Priority: 2},
			{Title: agentText(language, "StepReplan：根据 observation 调整后续", "StepReplan: adjust from observations"), Status: "pending", Priority: 3},
			{Title: agentText(language, "FinalAnswer：独立评估完成度并总结", "FinalAnswer: independently evaluate completion and summarize"), Status: "pending", Priority: 4},
		},
		Actions: actions,
		NextAction: &agentNextAction{
			Type:         "continue",
			Reason:       agentText(language, "已有明确低风险工具步骤，可以自动进入执行阶段。", "There are clear low-risk tool steps, so the Agent can enter execution."),
			AutoContinue: true,
		},
		Reasoning: agentText(language,
			"LLM 输出未形成可执行 JSON 时，系统基于最新用户指令生成保守工具计划，避免 Agent 退化成普通聊天。",
			"When the LLM output is not executable JSON, the system derives a conservative tool plan from the latest operator instruction so the Agent does not degrade into plain chat.",
		),
	}
}

func agentPortScanPlannerFromPrompt(prompt, modelAnswer, language string) *agentPlannerResponse {
	target := firstNonEmpty(agentExtractURL(prompt), agentExtractPortScanHostFromPrompt(prompt))
	answer := agentText(language,
		fmt.Sprintf("我理解这是端口枚举任务，不是 Web 漏洞扫描。当前已内置 ports.scan.safe，可做有界 TCP connect 安全探测；如果你要求 nmap/masscan 全量服务识别、UDP 或超大范围扫描，需要缩小端口范围或接入专用插件。目标：%s。", emptyDash(target)),
		fmt.Sprintf("I understand this as port enumeration, not a Web vulnerability scan. ports.scan.safe is available for bounded TCP connect safe probes; if you need full nmap/masscan service fingerprinting, UDP, or a very large range, narrow the port set or add a dedicated plugin. Target: %s.", emptyDash(target)),
	)
	if strings.TrimSpace(modelAnswer) != "" {
		answer = trimSummary(modelAnswer, 480)
	}
	return &agentPlannerResponse{
		Phase:  "plan",
		Answer: answer,
		Task: &agentPlannerTask{
			Goal:    agentText(language, "枚举开放端口 "+emptyDash(target), "Enumerate open ports for "+emptyDash(target)),
			Kind:    "port_scan",
			Status:  "blocked",
			Summary: agentText(language, "当前端口请求超出内置有界 TCP 探测能力，需缩小范围或接入 nmap/masscan 插件。", "The current port request exceeds the bounded TCP probe capability; narrow the range or add an nmap/masscan plugin."),
		},
		Todos: []agentPlannerTodo{
			{Title: agentText(language, "识别任务为端口枚举", "Classify task as port enumeration"), Status: "done", Priority: 1},
			{Title: agentText(language, "避免误启动 Web 漏洞扫描", "Avoid starting Web vulnerability scan"), Status: "done", Priority: 2},
			{Title: agentText(language, "等待操作者缩小端口范围或接入 nmap/masscan", "Wait for operator to narrow ports or add nmap/masscan"), Status: "blocked", Priority: 3, BlockedReason: agentText(language, "端口范围超出内置安全上限", "Port range exceeds the built-in safe limit")},
		},
		NextAction: &agentNextAction{
			Type:            "blocked",
			Reason:          agentText(language, "端口枚举不能用 scan.web.safe 代替；请缩小到 128 个以内的 TCP 端口或接入专用插件。", "Port enumeration must not be substituted with scan.web.safe; narrow to at most 128 TCP ports or add a dedicated plugin."),
			SuggestedPrompt: agentText(language, "扫描 "+emptyDash(target)+" 开放端口 ports=80,443,8080,8443", "scan open ports for "+emptyDash(target)+" ports=80,443,8080,8443"),
			AutoContinue:    false,
		},
		Reasoning: agentText(language,
			"用户询问开放端口，任务类型是端口枚举；当前 Web/API safe scan 是漏洞/证据扫描，不等价于端口扫描。",
			"The operator asked for open ports, which is port enumeration; the Web/API safe scan is vulnerability/evidence scanning and is not equivalent.",
		),
	}
}
