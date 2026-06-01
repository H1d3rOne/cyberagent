package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// SkillSpec describes one agent skill surfaced by the daemon skill registry.
// It lives in internal/agent/tools so daemon-facing compatibility shims can
// delegate to a package-level source of truth while preserving the legacy JSON
// shape.
type SkillSpec struct {
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

func BuiltinSkills(language string, assetRoot string) []SkillSpec {
	return AttachSkillFileSources([]SkillSpec{
		{
			ID:          "web_vulnerability_scan",
			Label:       skillText(language, "Web/API 安全扫描", "Web/API security scan"),
			Description: skillText(language, "对明确授权的 HTTP(S) 目标启动 safe profile，收集事件、证据、finding 和报告上下文。", "Start a safe profile for an explicitly authorized HTTP(S) target and collect events, evidence, findings, and report context."),
			IntentTags:  []string{"web_vulnerability_scan", "security_scan", "vulnerability_scan"},
			RequiredTools: []string{
				"scan.web.safe",
				"runs.context",
			},
			OptionalTools: []string{"reports.draft"},
			Risk:          "safe_scan",
			Examples: []string{
				skillText(language, "对 https://example.com 做安全扫描 scope=example.com", "scan https://example.com with scope=example.com"),
			},
		},
		{
			ID:            "port_scan",
			Label:         skillText(language, "端口枚举", "Port enumeration"),
			Description:   skillText(language, "使用有界 TCP connect 探测识别开放 TCP 端口；不替代 nmap 全量服务识别或 UDP 扫描。", "Use bounded TCP connect probes to identify open TCP ports; does not replace full nmap service fingerprinting or UDP scanning."),
			IntentTags:    []string{"port_scan", "open_ports", "tcp_connect_probe"},
			RequiredTools: []string{"ports.scan.safe"},
			OptionalTools: []string{"plugins.list"},
			Risk:          "safe_scan",
			Examples: []string{
				skillText(language, "扫描 example.com 开放端口 ports=80,443,8080", "scan open ports for example.com ports=80,443,8080"),
			},
		},
		{
			ID:              "content_discovery",
			Label:           skillText(language, "内容发现", "Content discovery"),
			Description:     skillText(language, "发现隐藏目录、路径和敏感文件；当前需要 MCP/plugin 提供 ffuf/gobuster/dirsearch 等价工具。", "Discover hidden directories, paths, and sensitive files; currently needs an MCP/plugin equivalent to ffuf/gobuster/dirsearch."),
			IntentTags:      []string{"content_discovery", "directory_enum", "path_fuzzing"},
			RequiredTools:   []string{"content.discovery"},
			OptionalTools:   []string{"http.request", "plugins.list"},
			FallbackPrompts: []string{skillText(language, "列出插件工具，检查是否有 ffuf/gobuster/dirsearch", "list plugins and check for ffuf/gobuster/dirsearch")},
			Risk:            "safe_scan",
			Examples:        []string{skillText(language, "找 https://example.com 的隐藏目录", "find hidden directories on https://example.com")},
		},
		{
			ID:              "js_route_analysis",
			Label:           skillText(language, "JS 路由分析", "JS route analysis"),
			Description:     skillText(language, "提取前端 JS bundle 中的路由、API endpoint 和潜在接口；本阶段不依赖 Browser/Playwright。", "Extract routes, API endpoints, and potential interfaces from front-end JS bundles; Browser/Playwright is intentionally not required in this phase."),
			IntentTags:      []string{"js_route_analysis", "frontend_routes", "api_endpoint_extraction"},
			RequiredTools:   []string{"js.routes.analyze"},
			OptionalTools:   []string{"workspace.search", "workspace.files.read", "http.request"},
			FallbackPrompts: []string{skillText(language, "把 JS 文件放进工作区，请搜索接口路径", "place JS files in the workspace and search for API routes")},
			Risk:            "read_only",
			Examples:        []string{skillText(language, "分析 https://example.com 的 JS 路由", "analyze JS routes for https://example.com")},
		},
		{
			ID:              "api_auth_test",
			Label:           skillText(language, "API 认证/鉴权测试", "API auth/authorization test"),
			Description:     skillText(language, "围绕 Auth Profile、角色、token 和 endpoint 做认证/鉴权验证；需要具体 endpoint/method 或 MCP/plugin 测试流。", "Validate authentication/authorization around Auth Profiles, roles, tokens, and endpoints; needs exact endpoint/method or an MCP/plugin test flow."),
			IntentTags:      []string{"api_auth_test", "authorization", "authentication", "idor"},
			RequiredTools:   []string{"api.auth.test"},
			OptionalTools:   []string{"http.request"},
			FallbackPrompts: []string{skillText(language, "使用已配置 Auth Profile 请求具体 API endpoint", "use a configured Auth Profile to request an exact API endpoint")},
			Risk:            "advanced",
			Examples:        []string{skillText(language, "测试 /api/user 是否存在越权", "test whether /api/user has authorization bypass")},
		},
		{
			ID:            "web_search",
			Label:         skillText(language, "Web 搜索", "Web search"),
			Description:   skillText(language, "使用配置的 Web Search provider 检索公开 Web 信息；需要先在 Settings 配置 provider/API key。", "Search public web information through the configured Web Search provider; requires provider/API key configuration in Settings."),
			IntentTags:    []string{"web_search", "search", "lookup", "查询", "搜索"},
			RequiredTools: []string{"web.search"},
			Risk:          "safe_scan",
			Examples: []string{
				skillText(language, "查一下 CVE-2024-1234 的细节", "look up details for CVE-2024-1234"),
			},
		},
		{
			ID:            "web_fetch",
			Label:         skillText(language, "读取 Web 页面", "Web page fetch"),
			Description:   skillText(language, "读取一个明确给出的公开 HTTP(S) URL，并返回可读文本或 raw 内容；只做 GET，带 SSRF 防护。", "Read an explicitly provided public HTTP(S) URL and return readable text or raw content; GET-only with SSRF protections."),
			IntentTags:    []string{"web_fetch", "page_read", "read_url", "读取网页"},
			RequiredTools: []string{"web.fetch"},
			Risk:          "safe_scan",
			Examples: []string{
				skillText(language, "读取 https://example.com/blog/x 并总结", "read https://example.com/blog/x and summarize it"),
			},
		},
		{
			ID:            "workspace_ops",
			Label:         skillText(language, "工作区分析/变更", "Workspace analysis/changes"),
			Description:   skillText(language, "在 Agent 工作区列文件、搜索、读取、生成 diff、patch 和应用变更。", "List, search, read, diff, patch, and apply changes in the Agent workspace."),
			IntentTags:    []string{"workspace", "code_analysis", "file_ops"},
			RequiredTools: []string{"workspace.files.list", "workspace.search", "workspace.files.read"},
			OptionalTools: []string{"workspace.files.diff", "workspace.files.apply"},
			Risk:          "workspace_write",
		},
		{
			ID:            "schedule_management",
			Label:         skillText(language, "定时任务管理", "Schedule management"),
			Description:   skillText(language, "把任意 Agent 指令或明确的 safe Web 扫描注册为按 cron 触发的定时任务，并可列出/删除既有定时任务；不会立即执行扫描。", "Register any Agent instruction or an explicit safe Web scan as a cron-driven schedule, and list or cancel existing schedules; does not execute the scan immediately."),
			IntentTags:    []string{"schedule", "cron", "recurring", "periodic", "scheduled_scan", "scheduled_prompt"},
			RequiredTools: []string{"schedule.create"},
			OptionalTools: []string{"schedule.list", "schedule.delete"},
			Risk:          "advanced",
			Examples: []string{
				skillText(language, "每天 7:20 扫描 example.com 开放端口 → schedule.create cron=\"20 7 * * *\" action=agent_prompt prompt=\"扫描 example.com 开放端口\"", "daily 7:20 scan open ports for example.com → schedule.create cron=\"20 7 * * *\" action=agent_prompt prompt=\"scan open ports for example.com\""),
				skillText(language, "每小时对 https://example.com 做 safe 漏扫 → schedule.create cron=\"0 * * * *\" action=scan_web target=https://example.com", "every hour run a safe vulnerability scan against https://example.com → schedule.create cron=\"0 * * * *\" action=scan_web target=https://example.com"),
				skillText(language, "列出/删除现有定时 → schedule.list / schedule.delete id=...", "list/cancel schedules → schedule.list / schedule.delete id=..."),
			},
		},
	}, assetRoot)
}

func AttachSkillFileSources(skills []SkillSpec, assetRoot string) []SkillSpec {
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
			skills[idx].Source = firstSkillNonEmpty(skills[idx].Source, "builtin")
			continue
		}
		skills[idx].Path = path
		if _, err := os.Stat(skillAssetPath(assetRoot, path)); err == nil {
			skills[idx].Source = "filesystem"
		} else {
			skills[idx].Source = "builtin"
		}
	}
	return skills
}

func normalizeSkillLanguage(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "en", "en-us":
		return "en"
	default:
		return "zh"
	}
}

func skillText(language, zh, en string) string {
	if normalizeSkillLanguage(language) == "en" {
		return en
	}
	return zh
}

func firstSkillNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func skillAssetPath(assetRoot string, elem ...string) string {
	if strings.TrimSpace(assetRoot) == "" {
		assetRoot = "."
	}
	return filepath.Join(append([]string{assetRoot}, elem...)...)
}
