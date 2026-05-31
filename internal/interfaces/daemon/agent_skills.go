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
			ID:            "workspace_ops",
			Label:         agentText(language, "工作区分析/变更", "Workspace analysis/changes"),
			Description:   agentText(language, "在 Agent 工作区列文件、搜索、读取、生成 diff、patch 和应用变更。", "List, search, read, diff, patch, and apply changes in the Agent workspace."),
			IntentTags:    []string{"workspace", "code_analysis", "file_ops"},
			RequiredTools: []string{"workspace.files.list", "workspace.search", "workspace.files.read"},
			OptionalTools: []string{"workspace.files.diff", "workspace.files.apply"},
			Risk:          "workspace_write",
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
		"workspace_ops":          "skills/workspace-ops/SKILL.md",
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
	out := make([]agentToolCapabilitySpec, 0, len(agentLLMToolDefinitions()))
	for _, tool := range agentLLMToolDefinitions() {
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

func agentToolCapabilityMetadataByName(language string) map[string]agentToolCapabilityMetadata {
	return map[string]agentToolCapabilityMetadata{
		"scan.web.safe": {
			IntentTags: []string{"web_vulnerability_scan", "security_scan", "evidence_scan"},
			CanDo:      []string{"safe profile Web/API vulnerability evidence scan", "runtime run tracking"},
			CannotDo:   []string{"port_scan", "content_discovery", "js_route_analysis", "full auth matrix"},
			Examples:   []string{agentText(language, "对授权目标做 safe Web/API 扫描", "run a safe Web/API scan for an authorized target")},
		},
		"ports.scan.safe": {
			IntentTags: []string{"port_scan", "open_ports", "tcp_connect_probe"},
			CanDo:      []string{"bounded TCP connect open-port probe"},
			CannotDo:   []string{"UDP scan", "nmap service fingerprinting", "very large ranges"},
			Examples:   []string{agentText(language, "扫描 example.com 开放端口 ports=80,443", "scan open ports for example.com ports=80,443")},
		},
		"http.request": {
			IntentTags: []string{"http_probe", "api_functional_test", "single_request"},
			CanDo:      []string{"single authorized HTTP request", "status/header/body snippet", "Auth Profile application"},
			CannotDo:   []string{"crawl", "port_scan", "full auth workflow"},
			Examples:   []string{agentText(language, "获取目标响应头和状态码", "fetch headers and status code")},
		},
		"plugins.list": {
			IntentTags: []string{"plugins", "tools", "capability_discovery"},
			CanDo:      []string{"list available plugins/tools"},
		},
		"runs.context": {
			IntentTags: []string{"run_context", "evidence", "findings", "events"},
			CanDo:      []string{"read run report/findings/artifacts/events/approvals"},
		},
		"reports.draft": {
			IntentTags: []string{"report_generation", "workspace"},
			CanDo:      []string{"generate markdown report draft into workspace"},
		},
	}
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
