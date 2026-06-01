package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type agentSkillSpec struct {
	ID              string   `json:"id"`
	Source          string   `json:"source,omitempty"`
	Path            string   `json:"path,omitempty"`
	Label           string   `json:"label"`
	Description     string   `json:"description"`
	IntentTags      []string `json:"intent_tags"`
	RequiredTools   []string `json:"required_tools,omitempty"`
	OptionalTools   []string `json:"optional_tools,omitempty"`
	FallbackPrompts []string `json:"fallback_prompts,omitempty"`
	Risk            string   `json:"risk"`
	Examples        []string `json:"examples,omitempty"`
}

type agentToolCapabilitySpec struct {
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

type agentMCPIntegrationSpec struct {
	Status      string   `json:"status"`
	Description string   `json:"description"`
	NextSteps   []string `json:"next_steps"`
}

type agentSkillRegistryResponse struct {
	Skills           []agentSkillSpec          `json:"skills"`
	ToolCapabilities []agentToolCapabilitySpec `json:"tool_capabilities"`
	SecurityTools    []agentSecurityToolSpec   `json:"security_tools,omitempty"`
	MCP              agentMCPIntegrationSpec   `json:"mcp"`
}

type agentToolCapabilityMetadata struct {
	IntentTags []string
	CanDo      []string
	CannotDo   []string
	Examples   []string
	Source     string
}

func agentSkillRegistry(language string) agentSkillRegistryResponse {
	language = normalizeAgentLanguage(language)
	return agentSkillRegistryResponse{
		Skills:           agentBuiltinSkills(language),
		ToolCapabilities: agentToolCapabilities(language),
		SecurityTools:    agentSecurityToolCatalog(),
		MCP: agentMCPIntegrationSpec{
			Status: agentText(language, "planned", "planned"),
			Description: agentText(
				language,
				"MCP 将作为外部工具发现与调用层接入；MCP tool 会先映射为 ToolCapability，再经过 CyberAgent 权限与策略网关。",
				"MCP will be integrated as the external tool discovery/invocation layer; MCP tools will first be mapped to ToolCapability and then pass through CyberAgent's permission and policy gateway.",
			),
			NextSteps: []string{
				agentText(language, "增加 MCP server 配置模型", "Add MCP server configuration model"),
				agentText(language, "实现 MCP tools/list discover", "Implement MCP tools/list discovery"),
				agentText(language, "把 MCP tool 映射到 ToolCapability 并接入审批", "Map MCP tools to ToolCapability and approval gates"),
			},
		},
	}
}

func agentBuiltinSkills(language string) []agentSkillSpec {
	return attachAgentSkillFileSources([]agentSkillSpec{
		{
			ID:          "web_vulnerability_scan",
			Label:       agentText(language, "Web/API 安全扫描", "Web/API security scan"),
			Description: agentText(language, "对明确授权的 HTTP(S) 目标启动 safe profile，收集事件、证据、finding 和报告上下文。", "Start a safe profile for an explicitly authorized HTTP(S) target and collect events, evidence, findings, and report context."),
			IntentTags:  []string{"web_vulnerability_scan", "security_scan", "vulnerability_scan"},
			RequiredTools: []string{
				"scan.web.safe",
				"runs.context",
			},
			OptionalTools: []string{"reports.draft"},
			Risk:          "safe_scan",
			Examples: []string{
				agentText(language, "对 https://example.com 做安全扫描 scope=example.com", "scan https://example.com with scope=example.com"),
			},
		},
		{
			ID:            "port_scan",
			Label:         agentText(language, "端口枚举", "Port enumeration"),
			Description:   agentText(language, "使用有界 TCP connect 探测识别开放 TCP 端口；不替代 nmap 全量服务识别或 UDP 扫描。", "Use bounded TCP connect probes to identify open TCP ports; does not replace full nmap service fingerprinting or UDP scanning."),
			IntentTags:    []string{"port_scan", "open_ports", "tcp_connect_probe"},
			RequiredTools: []string{"ports.scan.safe"},
			OptionalTools: []string{"plugins.list"},
			Risk:          "safe_scan",
			Examples: []string{
				agentText(language, "扫描 example.com 开放端口 ports=80,443,8080", "scan open ports for example.com ports=80,443,8080"),
			},
		},
		{
			ID:              "content_discovery",
			Label:           agentText(language, "内容发现", "Content discovery"),
			Description:     agentText(language, "发现隐藏目录、路径和敏感文件；当前需要 MCP/plugin 提供 ffuf/gobuster/dirsearch 等价工具。", "Discover hidden directories, paths, and sensitive files; currently needs an MCP/plugin equivalent to ffuf/gobuster/dirsearch."),
			IntentTags:      []string{"content_discovery", "directory_enum", "path_fuzzing"},
			RequiredTools:   []string{"content.discovery"},
			OptionalTools:   []string{"http.request", "plugins.list"},
			FallbackPrompts: []string{agentText(language, "列出插件工具，检查是否有 ffuf/gobuster/dirsearch", "list plugins and check for ffuf/gobuster/dirsearch")},
			Risk:            "safe_scan",
			Examples:        []string{agentText(language, "找 https://example.com 的隐藏目录", "find hidden directories on https://example.com")},
		},
		{
			ID:              "js_route_analysis",
			Label:           agentText(language, "JS 路由分析", "JS route analysis"),
			Description:     agentText(language, "提取前端 JS bundle 中的路由、API endpoint 和潜在接口；本阶段不依赖 Browser/Playwright。", "Extract routes, API endpoints, and potential interfaces from front-end JS bundles; Browser/Playwright is intentionally not required in this phase."),
			IntentTags:      []string{"js_route_analysis", "frontend_routes", "api_endpoint_extraction"},
			RequiredTools:   []string{"js.routes.analyze"},
			OptionalTools:   []string{"workspace.search", "workspace.files.read", "http.request"},
			FallbackPrompts: []string{agentText(language, "把 JS 文件放进工作区，请搜索接口路径", "place JS files in the workspace and search for API routes")},
			Risk:            "read_only",
			Examples:        []string{agentText(language, "分析 https://example.com 的 JS 路由", "analyze JS routes for https://example.com")},
		},
		{
			ID:              "api_auth_test",
			Label:           agentText(language, "API 认证/鉴权测试", "API auth/authorization test"),
			Description:     agentText(language, "围绕 Auth Profile、角色、token 和 endpoint 做认证/鉴权验证；需要具体 endpoint/method 或 MCP/plugin 测试流。", "Validate authentication/authorization around Auth Profiles, roles, tokens, and endpoints; needs exact endpoint/method or an MCP/plugin test flow."),
			IntentTags:      []string{"api_auth_test", "authorization", "authentication", "idor"},
			RequiredTools:   []string{"api.auth.test"},
			OptionalTools:   []string{"http.request"},
			FallbackPrompts: []string{agentText(language, "使用已配置 Auth Profile 请求具体 API endpoint", "use a configured Auth Profile to request an exact API endpoint")},
			Risk:            "advanced",
			Examples:        []string{agentText(language, "测试 /api/user 是否存在越权", "test whether /api/user has authorization bypass")},
		},
		{
			ID:            "web_search",
			Label:         agentText(language, "Web 搜索", "Web search"),
			Description:   agentText(language, "使用配置的 Web Search provider 检索公开 Web 信息；需要先在 Settings 配置 provider/API key。", "Search public web information through the configured Web Search provider; requires provider/API key configuration in Settings."),
			IntentTags:    []string{"web_search", "search", "lookup", "查询", "搜索"},
			RequiredTools: []string{"web.search"},
			Risk:          "safe_scan",
			Examples: []string{
				agentText(language, "查一下 CVE-2024-1234 的细节", "look up details for CVE-2024-1234"),
			},
		},
		{
			ID:            "web_fetch",
			Label:         agentText(language, "读取 Web 页面", "Web page fetch"),
			Description:   agentText(language, "读取一个明确给出的公开 HTTP(S) URL，并返回可读文本或 raw 内容；只做 GET，带 SSRF 防护。", "Read an explicitly provided public HTTP(S) URL and return readable text or raw content; GET-only with SSRF protections."),
			IntentTags:    []string{"web_fetch", "page_read", "read_url", "读取网页"},
			RequiredTools: []string{"web.fetch"},
			Risk:          "safe_scan",
			Examples: []string{
				agentText(language, "读取 https://example.com/blog/x 并总结", "read https://example.com/blog/x and summarize it"),
			},
		},
		{
			ID:            "workspace_ops",
			Label:         agentText(language, "工作区分析/变更", "Workspace analysis/changes"),
			Description:   agentText(language, "在 Agent 工作区列文件、搜索、读取、生成 diff、patch 和应用变更。", "List, search, read, diff, patch, and apply changes in the Agent workspace."),
			IntentTags:    []string{"workspace", "code_analysis", "file_ops"},
			RequiredTools: []string{"workspace.files.list", "workspace.search", "workspace.files.read"},
			OptionalTools: []string{"workspace.files.diff", "workspace.files.apply"},
			Risk:          "workspace_write",
		},
		{
			ID:            "schedule_management",
			Label:         agentText(language, "定时任务管理", "Schedule management"),
			Description:   agentText(language, "把任意 Agent 指令或明确的 safe Web 扫描注册为按 cron 触发的定时任务，并可列出/删除既有定时任务；不会立即执行扫描。", "Register any Agent instruction or an explicit safe Web scan as a cron-driven schedule, and list or cancel existing schedules; does not execute the scan immediately."),
			IntentTags:    []string{"schedule", "cron", "recurring", "periodic", "scheduled_scan", "scheduled_prompt"},
			RequiredTools: []string{"schedule.create"},
			OptionalTools: []string{"schedule.list", "schedule.delete"},
			Risk:          "advanced",
			Examples: []string{
				agentText(language, "每天 7:20 扫描 example.com 开放端口 → schedule.create cron=\"20 7 * * *\" action=agent_prompt prompt=\"扫描 example.com 开放端口\"", "daily 7:20 scan open ports for example.com → schedule.create cron=\"20 7 * * *\" action=agent_prompt prompt=\"scan open ports for example.com\""),
				agentText(language, "每小时对 https://example.com 做 safe 漏扫 → schedule.create cron=\"0 * * * *\" action=scan_web target=https://example.com", "every hour run a safe vulnerability scan against https://example.com → schedule.create cron=\"0 * * * *\" action=scan_web target=https://example.com"),
				agentText(language, "列出/删除现有定时 → schedule.list / schedule.delete id=...", "list/cancel schedules → schedule.list / schedule.delete id=..."),
			},
		},
	})
}

func attachAgentSkillFileSources(skills []agentSkillSpec) []agentSkillSpec {
	paths := map[string]string{
		"web_vulnerability_scan": "skills/web-vulnerability-scan/SKILL.md",
		"port_scan":              "skills/port-scan/SKILL.md",
		"content_discovery":      "skills/content-discovery/SKILL.md",
		"js_route_analysis":      "skills/js-route-analysis/SKILL.md",
		"api_auth_test":          "skills/api-auth-test/SKILL.md",
		"web_search":             "skills/web-search/SKILL.md",
		"web_fetch":              "skills/web-fetch/SKILL.md",
		"workspace_ops":          "skills/workspace-ops/SKILL.md",
		"schedule_management":    "skills/schedule-management/SKILL.md",
	}
	for idx := range skills {
		path := paths[skills[idx].ID]
		if path == "" {
			skills[idx].Source = firstNonEmpty(skills[idx].Source, "builtin")
			continue
		}
		skills[idx].Path = path
		if _, err := os.Stat(agentAssetPath(path)); err == nil {
			skills[idx].Source = "filesystem"
		} else {
			skills[idx].Source = "builtin"
		}
	}
	return skills
}

func agentToolCapabilities(language string) []agentToolCapabilitySpec {
	metadata := agentToolCapabilityMetadataByName(language)
	securityToolCatalog := agentSecurityToolCatalogByName()
	defs := toolRegistry().LLMDefinitions()
	out := make([]agentToolCapabilitySpec, 0, len(defs))
	for _, tool := range defs {
		name := normalizeAgentPlannerActionName(tool.Name)
		if name == "" {
			name = tool.Name
		}
		risk, autoRun := agentCapabilityToolPolicy(name)
		meta := metadata[name]
		source := meta.Source
		if source == "" {
			source = "builtin"
		}
		catalogPath := ""
		if catalogTool, ok := securityToolCatalog[name]; ok {
			catalogPath = catalogTool.Path
		}
		out = append(out, agentToolCapabilitySpec{
			Name:          name,
			Source:        source,
			CatalogPath:   catalogPath,
			IntentTags:    append([]string{}, meta.IntentTags...),
			CanDo:         append([]string{}, meta.CanDo...),
			CannotDo:      append([]string{}, meta.CannotDo...),
			Risk:          risk,
			RequiredLevel: agentOperationLevelRequiredForAction(name),
			AutoRun:       autoRun,
			Description:   tool.Description,
			InputSchema:   tool.InputSchema,
			Examples:      append([]string{}, meta.Examples...),
		})
	}
	return out
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
