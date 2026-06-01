package toolregistry

import (
	"encoding/json"
)

// NewWithBuiltins returns a registry pre-populated with every daemon-side
// builtin tool. The descriptions and schemas mirror the legacy
// agentLLMToolDefinitions() literal byte-for-byte; a parity test in the
// daemon package guards against drift while the migration is in flight.
//
// The aliases below are the union of every entry that
// normalizeAgentPlannerActionName() collapses to the canonical name.
// Required levels mirror agentOperationLevelRequiredForAction(). Risk and
// AutoRun mirror agentCapabilityToolPolicy().
func NewWithBuiltins() *Registry {
	r := New()
	for _, spec := range Builtins() {
		r.MustRegister(spec)
	}
	return r
}

// Builtins returns the canonical tool specs in a stable order. Exported so
// tests can iterate without spinning up a registry.
func Builtins() []ToolSpec {
	return []ToolSpec{
		{
			Name: "scan.web.safe",
			Aliases: []string{
				"web.scan.safe", "scan.web", "web.scan", "scans.web",
				"scan", "start.scan", "start-web-scan", "start.web.scan",
			},
			Description: LocalizedText{
				ZH: "Start a safe-profile Web/API vulnerability/evidence scan for an explicitly authorized http/https target and scope. This is not a port scanner and must not be used for open-port enumeration. Requires safe_scan operation level.",
				EN: "Start a safe-profile Web/API vulnerability/evidence scan for an explicitly authorized http/https target and scope. This is not a port scanner and must not be used for open-port enumeration. Requires safe_scan operation level.",
			},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"target":{"type":"string","description":"Authorized http/https target URL"},"scope":{"type":"string","description":"Authorization boundary or URL prefix; defaults to target"}},"required":["target"],"additionalProperties":false}`),
			Risk:          RiskSafeScan,
			RequiredLevel: "safe_scan",
			AutoRun:       true,
			IntentTags:    []string{"web_vulnerability_scan", "security_scan", "evidence_scan"},
			CanDo:         []string{"safe profile Web/API vulnerability evidence scan", "runtime run tracking"},
			CannotDo:      []string{"port_scan", "content_discovery", "js_route_analysis", "full auth matrix"},
			Examples: LocalizedExamples{
				{ZH: "对授权目标做 safe Web/API 扫描", EN: "run a safe Web/API scan for an authorized target"},
			},
			SkillID: "web_vulnerability_scan",
		},
		{
			Name: "ports.scan.safe",
			Aliases: []string{
				"port.scan.safe", "ports.scan", "port.scan", "portscan",
				"port.scan.tcp", "tcp.scan", "scan.ports", "nmap",
			},
			Description: LocalizedText{
				ZH: "Run a bounded safe TCP connect probe against an explicitly authorized host to identify open TCP ports. Defaults to a small common_safe port set; custom ranges are capped. This is not nmap service fingerprinting and does not scan UDP.",
				EN: "Run a bounded safe TCP connect probe against an explicitly authorized host to identify open TCP ports. Defaults to a small common_safe port set; custom ranges are capped. This is not nmap service fingerprinting and does not scan UDP.",
			},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"target":{"type":"string","description":"Authorized target URL or host"},"host":{"type":"string","description":"Authorized host name or IP; resolved from target when omitted"},"ports":{"oneOf":[{"type":"string","description":"Comma/range spec such as 80,443,8000-8010; capped by policy"},{"type":"array","items":{"type":"integer","minimum":1,"maximum":65535},"maxItems":128}],"description":"Optional TCP ports to probe; defaults to common_safe"},"port_profile":{"type":"string","description":"Optional label for the port set"}},"required":["target"],"additionalProperties":false}`),
			Risk:          RiskSafeScan, // capability policy reports "medium"; required_level still safe_scan
			RequiredLevel: "safe_scan",
			AutoRun:       false, // matches agentCapabilityToolPolicy "ports.scan.safe"
			IntentTags:    []string{"port_scan", "open_ports", "tcp_connect_probe"},
			CanDo:         []string{"bounded TCP connect open-port probe"},
			CannotDo:      []string{"UDP scan", "nmap service fingerprinting", "very large ranges"},
			Examples: LocalizedExamples{
				{ZH: "扫描 example.com 开放端口 ports=80,443", EN: "scan open ports for example.com ports=80,443"},
			},
			SkillID: "port_scan",
		},
		{
			Name: "http.request",
			Aliases: []string{
				"http", "request", "curl", "web.request", "http.get", "http.head",
			},
			Description: LocalizedText{
				ZH: "Make an HTTP request to an explicitly authorized http/https URL and return status, headers, and a truncated body. GET/HEAD/OPTIONS require safe_scan; POST/PUT/PATCH/DELETE require advanced.",
				EN: "Make an HTTP request to an explicitly authorized http/https URL and return status, headers, and a truncated body. GET/HEAD/OPTIONS require safe_scan; POST/PUT/PATCH/DELETE require advanced.",
			},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"Authorized http/https URL"},"method":{"type":"string","enum":["GET","HEAD","OPTIONS","POST","PUT","PATCH","DELETE"],"description":"HTTP method; defaults to GET"},"auth_profile_id":{"type":"string","description":"Optional configured API auth profile ID/name; secrets are resolved by the daemon and are not exposed to the model"},"headers":{"type":"object","description":"Optional request headers for authorized testing; sensitive values are redacted in stored results","additionalProperties":{"type":"string"}},"body":{"type":"string","description":"Optional request body for POST/PUT/PATCH/DELETE"},"content_type":{"type":"string","description":"Optional Content-Type header when body is provided"}},"required":["url"],"additionalProperties":false}`),
			Risk:          RiskSafeScan,
			RequiredLevel: "safe_scan",
			AutoRun:       true,
			IntentTags:    []string{"http_probe", "api_functional_test", "single_request"},
			CanDo:         []string{"single authorized HTTP request", "status/header/body snippet", "Auth Profile application"},
			CannotDo:      []string{"crawl", "port_scan", "full auth workflow"},
			Examples: LocalizedExamples{
				{ZH: "获取目标响应头和状态码", EN: "fetch headers and status code"},
			},
		},
		{
			Name: "web.search",
			Aliases: []string{
				"websearch", "search.web", "google.search", "google",
				"search.engine", "tavily.search",
			},
			Description: LocalizedText{
				ZH: "Search the public web for lookup/research queries using the configured Web Search provider. Use for search/query intents, not for probing a specific target. Requires safe_scan operation level and configured Web Search settings.",
				EN: "Search the public web for lookup/research queries using the configured Web Search provider. Use for search/query intents, not for probing a specific target. Requires safe_scan operation level and configured Web Search settings.",
			},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query"},"max_results":{"type":"integer","minimum":1,"maximum":10,"description":"Maximum results to return; defaults to configured value or 5"},"locale":{"type":"string","description":"Optional locale/language hint such as en-US or zh-CN"}},"required":["query"],"additionalProperties":false}`),
			Risk:          RiskSafeScan,
			RequiredLevel: "safe_scan",
			AutoRun:       true,
			IntentTags:    []string{"web_search", "search", "lookup", "查询", "搜索"},
			CanDo:         []string{"search public web information through the configured provider", "return title/url/snippet/source results"},
			CannotDo:      []string{"read a known URL directly", "scan or probe a target", "work without Web Search settings"},
			Examples: LocalizedExamples{
				{ZH: "搜索 CVE 详情和参考来源", EN: "search for CVE details and references"},
			},
			SkillID: "web_search",
		},
		{
			Name: "web.fetch",
			Aliases: []string{
				"webfetch", "fetch.url", "fetch.page", "fetch.web",
				"page.fetch", "open.url", "fetch",
			},
			Description: LocalizedText{
				ZH: "Fetch and read a specific public http/https URL with SSRF protections. Use for reading a known page URL, not for arbitrary API testing or non-GET HTTP requests. Requires safe_scan operation level.",
				EN: "Fetch and read a specific public http/https URL with SSRF protections. Use for reading a known page URL, not for arbitrary API testing or non-GET HTTP requests. Requires safe_scan operation level.",
			},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"Public http/https URL to fetch"},"mode":{"type":"string","enum":["readable","raw"],"description":"readable extracts text from HTML; raw returns truncated response text. Defaults to readable"},"max_chars":{"type":"integer","minimum":1,"maximum":20000,"description":"Maximum response characters to return; defaults to 8000"}},"required":["url"],"additionalProperties":false}`),
			Risk:          RiskSafeScan,
			RequiredLevel: "safe_scan",
			AutoRun:       true,
			IntentTags:    []string{"web_fetch", "page_read", "read_url", "读取网页"},
			CanDo:         []string{"GET a specific public http/https URL", "extract readable text from HTML", "return truncated raw content"},
			CannotDo:      []string{"POST/PUT/PATCH/DELETE", "fetch private/internal/metadata URLs", "search the web by query"},
			Examples: LocalizedExamples{
				{ZH: "读取指定网页并总结", EN: "read a specified web page and summarize it"},
			},
			SkillID: "web_fetch",
		},
		{
			Name: "workspace.command",
			Aliases: []string{
				"workspace.commands.run", "workspace.run.command",
				"command", "commands.run", "shell", "terminal",
			},
			Description:   LocalizedText{ZH: "Run a low-risk allowlisted read-only command in the current Agent workspace. Allowed examples: pwd, ls, cat, head, tail, grep, find, wc, git status."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Low-risk allowlisted command to execute"}},"required":["command"],"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.list",
			Aliases: []string{
				"workspace.file.list", "workspace.list", "workspace.ls",
				"files.list", "file.list", "list.files", "list.file", "ls",
			},
			Description:   LocalizedText{ZH: "List files in the current Agent workspace."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative directory path","default":"."}},"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.tree",
			Aliases: []string{
				"workspace.file.tree", "workspace.tree", "files.tree",
				"file.tree", "tree", "list.tree", "directory.tree",
			},
			Description:   LocalizedText{ZH: "Show a recursive workspace file tree with depth and result limits."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative directory path","default":"."},"depth":{"type":"integer","minimum":1,"maximum":8,"default":3},"limit":{"type":"integer","minimum":1,"maximum":500,"default":120},"include_hidden":{"type":"boolean","default":false}},"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.read",
			Aliases: []string{
				"workspace.file.read", "workspace.read", "files.read",
				"file.read", "read.file", "cat",
			},
			Description:   LocalizedText{ZH: "Read a workspace-relative file from the current Agent workspace."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"}},"required":["path"],"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
		},
		{
			Name: "workspace.search",
			Aliases: []string{
				"workspace.grep", "workspace.find.text", "files.search",
				"file.search", "search.files", "grep", "search",
			},
			Description:   LocalizedText{ZH: "Search text in workspace files without modifying files; returns file paths, line numbers, and matching lines."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Text or regex to search for"},"path":{"type":"string","description":"Workspace-relative directory path","default":"."},"regex":{"type":"boolean","default":false},"case_sensitive":{"type":"boolean","default":false},"limit":{"type":"integer","minimum":1,"maximum":300,"default":80}},"required":["query"],"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.write",
			Aliases: []string{
				"workspace.file.write", "workspace.write", "files.write",
				"file.write", "write.file", "write", "save.file",
				"create.file", "replace.file",
			},
			Description:   LocalizedText{ZH: "Write or replace a workspace-relative file in the current Agent workspace when explicitly requested by the operator."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"},"content":{"type":"string","description":"File content to write"}},"required":["path","content"],"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.patch",
			Aliases: []string{
				"workspace.file.patch", "workspace.patch", "files.patch",
				"file.patch", "patch.file", "patch", "edit.file",
				"modify.file", "replace.in.file",
			},
			Description:   LocalizedText{ZH: "Patch a workspace-relative file by replacing one exact old text block with new text and returning a diff preview."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"},"old":{"type":"string","description":"Exact text to replace; must occur exactly once"},"new":{"type":"string","description":"Replacement text"}},"required":["path","old","new"],"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.diff",
			Aliases: []string{
				"workspace.file.diff", "workspace.diff", "files.diff",
				"file.diff", "diff.file", "diff", "preview.file",
				"preview.patch", "stage.file", "stage.patch", "staged.diff",
			},
			Description:   LocalizedText{ZH: "Create a staged workspace file change by replacing one exact old text block with new text; returns a diff preview and change_id without modifying the file."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"},"old":{"type":"string","description":"Exact text to replace; must occur exactly once"},"new":{"type":"string","description":"Replacement text"}},"required":["path","old","new"],"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.diff_many",
			Aliases: []string{
				"workspace.files.diff.many", "workspace.files.diffmany",
				"workspace.file.diff.many", "workspace.diff.many",
				"files.diff.many", "diff.many", "preview.files",
				"stage.files", "stage.changeset", "staged.changeset",
			},
			Description:   LocalizedText{ZH: "Create one staged change set across multiple workspace files. Each item replaces one exact old text block with new text and returns a shared group_id plus per-file change_id values without modifying files."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"changes":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"},"old":{"type":"string","description":"Exact text to replace; must occur exactly once"},"new":{"type":"string","description":"Replacement text"}},"required":["path","old","new"],"additionalProperties":false}}},"required":["changes"],"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.rebase",
			Aliases: []string{
				"workspace.file.rebase", "workspace.rebase", "files.rebase",
				"file.rebase", "rebase.file", "regenerate.change",
				"regenerate.patch", "refresh.change", "rebase.patch",
			},
			Description:   LocalizedText{ZH: "Regenerate a pending/conflict staged change by change_id using the current exact old text from the workspace file. The replacement defaults to the original staged new text; returns a new change_id and supersedes the old change."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"change_id":{"type":"string","description":"Existing staged change ID to regenerate"},"old":{"type":"string","description":"Exact current text to replace; must occur exactly once"},"new":{"type":"string","description":"Optional replacement text; defaults to the original staged new text"}},"required":["change_id","old"],"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.apply",
			Aliases: []string{
				"workspace.file.apply", "workspace.apply", "files.apply",
				"file.apply", "apply.file", "apply.patch", "apply.change",
				"apply", "commit.patch",
			},
			Description:   LocalizedText{ZH: "Apply a previously staged workspace file change by change_id, a staged group by group_id, or all pending changes with all=true after validating old text still matches exactly once."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"change_id":{"type":"string","description":"ID returned by workspace.files.diff"},"group_id":{"type":"string","description":"Group ID returned by workspace.files.diff_many"},"all":{"type":"boolean","description":"Apply all pending staged changes when explicitly requested"}},"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.discard",
			Aliases: []string{
				"workspace.file.discard", "workspace.discard", "files.discard",
				"file.discard", "discard.file", "discard.patch",
				"discard.change", "discard", "drop.patch", "cancel.patch",
			},
			Description:   LocalizedText{ZH: "Discard a previously staged workspace file change by change_id, a staged group by group_id, or all pending/conflict changes with all=true without modifying files."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"change_id":{"type":"string","description":"ID returned by workspace.files.diff"},"group_id":{"type":"string","description":"Group ID returned by workspace.files.diff_many"},"all":{"type":"boolean","description":"Discard all pending/conflict staged changes when explicitly requested"}},"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.delete",
			Aliases: []string{
				"workspace.file.delete", "workspace.delete", "files.delete",
				"file.delete", "delete.file", "remove.file", "rm.file",
				"unlink.file", "delete", "remove", "rm",
			},
			Description:   LocalizedText{ZH: "Delete a workspace-relative file when the operator explicitly requests deletion. Directories are rejected."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative file path"}},"required":["path"],"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.move",
			Aliases: []string{
				"workspace.file.move", "workspace.move", "files.move",
				"file.move", "move.file", "rename.file", "mv.file",
				"move", "rename", "relocate", "mv",
			},
			Description:   LocalizedText{ZH: "Move or rename a workspace-relative file when the operator explicitly requests it. Directories are rejected."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"from":{"type":"string","description":"Source workspace-relative file path"},"to":{"type":"string","description":"Destination workspace-relative file path"}},"required":["from","to"],"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.copy",
			Aliases: []string{
				"workspace.file.copy", "workspace.copy", "files.copy",
				"file.copy", "copy.file", "duplicate.file", "cp.file",
				"copy", "duplicate", "cp",
			},
			Description:   LocalizedText{ZH: "Copy a workspace-relative file when the operator explicitly requests it. Directories are rejected and destination must not already exist."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"from":{"type":"string","description":"Source workspace-relative file path"},"to":{"type":"string","description":"Destination workspace-relative file path"}},"required":["from","to"],"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "workspace.files.mkdir",
			Aliases: []string{
				"workspace.file.mkdir", "workspace.mkdir", "files.mkdir",
				"file.mkdir", "mkdir", "make.dir", "make.directory",
				"create.dir", "create.directory",
			},
			Description:   LocalizedText{ZH: "Create a workspace-relative directory when the operator explicitly requests it."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Workspace-relative directory path"}},"required":["path"],"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
		},
		{
			Name: "daemon.health",
			Aliases: []string{
				"daemon.doctor", "health", "doctor", "status",
			},
			Description:   LocalizedText{ZH: "Check CyberAgent daemon health/version/status."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
		},
		{
			Name: "plugins.list",
			Aliases: []string{
				"plugin.list", "tools.list", "tool.list", "plugins", "tools",
			},
			Description:   LocalizedText{ZH: "List available CyberAgent plugins/tools."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
			IntentTags:    []string{"plugins", "tools", "capability_discovery"},
			CanDo:         []string{"list available plugins/tools"},
		},
		{
			Name: "runs.list",
			Aliases: []string{
				"run.list", "history.list", "runs", "history",
			},
			Description:   LocalizedText{ZH: "List recent CyberAgent runs/history."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
		},
		{
			Name: "runs.context",
			Aliases: []string{
				"run.context", "runs.get", "run.get", "runs.report",
				"run.report", "runs.findings", "run.findings",
				"runs.events", "run.events", "runs.artifacts", "run.artifacts",
			},
			Description:   LocalizedText{ZH: "Read structured context for a run: report_json, findings, artifact_index, events, or approvals. Defaults to the active run when run_id is omitted."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","description":"Run ID to inspect; defaults to active run when omitted"},"context":{"type":"string","enum":["report_json","findings","artifact_index","events","approvals"],"description":"Context section to read"}},"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
			IntentTags:    []string{"run_context", "evidence", "findings", "events"},
			CanDo:         []string{"read run report/findings/artifacts/events/approvals"},
		},
		{
			Name: "reports.draft",
			Aliases: []string{
				"report.draft", "draft.report", "report.generate",
				"reports.generate", "generate.report", "write.report",
			},
			Description:   LocalizedText{ZH: "Generate a Markdown report draft for a run from runtime report/context and save it into the Agent workspace. Requires workspace_write operation level."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","description":"Run ID to draft; defaults to the active run when omitted"},"path":{"type":"string","description":"Optional workspace-relative markdown path"}},"additionalProperties":false}`),
			Risk:          RiskWorkspaceWrite,
			RequiredLevel: "workspace_write",
			AutoRun:       true,
			IntentTags:    []string{"report_generation", "workspace"},
			CanDo:         []string{"generate markdown report draft into workspace"},
		},
		{
			Name: "runs.approvals.list",
			Aliases: []string{
				"run.approvals.list", "approvals.list", "approval.list",
				"approvals", "approval.queue",
			},
			Description:   LocalizedText{ZH: "List pending approval requests for a specific run, or summarize the global approval queue when run_id is omitted."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","description":"Optional run ID to inspect"}},"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
		},
		{
			Name: "runs.approvals.approve",
			Aliases: []string{
				"run.approvals.approve", "approvals.approve",
				"approval.approve", "approve.approval",
			},
			Description:   LocalizedText{ZH: "Approve one pending approval when the operator explicitly asks to approve it. Requires run_id plus approval_id/step_id unless the active run has exactly one pending approval."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","description":"Run ID"},"approval_id":{"type":"string","description":"Approval request ID"},"step_id":{"type":"string","description":"Alternative step ID"},"comment":{"type":"string","description":"Optional approval comment"}},"additionalProperties":false}`),
			Risk:          RiskApprovalOps,
			RequiredLevel: "approval_ops",
			AutoRun:       false,
		},
		{
			Name: "runs.approvals.deny",
			Aliases: []string{
				"run.approvals.deny", "approvals.deny", "approval.deny",
				"deny.approval", "reject.approval",
			},
			Description:   LocalizedText{ZH: "Deny one pending approval when the operator explicitly asks to reject it. Requires run_id plus approval_id/step_id unless the active run has exactly one pending approval."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","description":"Run ID"},"approval_id":{"type":"string","description":"Approval request ID"},"step_id":{"type":"string","description":"Alternative step ID"},"comment":{"type":"string","description":"Optional rejection comment"}},"additionalProperties":false}`),
			Risk:          RiskApprovalOps,
			RequiredLevel: "approval_ops",
			AutoRun:       false,
		},
		{
			Name: "runs.resume_approved",
			Aliases: []string{
				"runs.resume.approved", "run.resume.approved",
				"resume.approved", "resume.approved.run",
				"runs.resume.approval",
			},
			Description:   LocalizedText{ZH: "Resume approved steps for a run when the operator explicitly asks to resume approved work."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"run_id":{"type":"string","description":"Run ID to resume; defaults to active run when omitted"}},"additionalProperties":false}`),
			Risk:          RiskApprovalOps,
			RequiredLevel: "approval_ops",
			AutoRun:       false,
		},
		{
			Name: "schedule.create",
			Aliases: []string{
				"schedule.add", "schedule.set", "create.schedule",
				"set.schedule", "cron.create", "cron.add",
				"schedules.create", "add.schedule",
			},
			Description:   LocalizedText{ZH: "Create a recurring schedule. Prefer action=agent_prompt with the executable instruction for arbitrary scheduled Agent work (including port scans, HTTP checks, searches, workspace tasks); use action=scan_web only for an explicit recurring safe Web/API vulnerability scan. Do NOT call the immediate tool for scheduled requests. Requires advanced operation level."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"cron":{"type":"string","description":"Standard 5-field cron expression or @every / @hourly / @daily / @weekly / @monthly macro"},"action":{"type":"string","enum":["agent_prompt","scan_web"],"description":"What the schedule fires: agent_prompt sends an executable instruction into the Agent session and supports arbitrary future Agent work; scan_web is only for recurring safe Web/API vulnerability scans"},"name":{"type":"string","description":"Optional schedule name; defaults are derived from action+target/prompt"},"target":{"type":"string","description":"Authorized http/https target URL (required when action=scan_web)"},"scope":{"type":"string","description":"Authorization boundary; defaults to target"},"profile":{"type":"string","description":"Scan profile (defaults to safe)"},"prompt":{"type":"string","description":"Executable prompt to dispatch at trigger time (required when action=agent_prompt). Strip schedule wording; keep only the action to perform."},"session_id":{"type":"string","description":"Target agent session id; defaults to the current session for agent_prompt"},"enabled":{"type":"boolean","description":"Whether the schedule is enabled on creation; defaults to true"}},"required":["cron","action"],"additionalProperties":false}`),
			Risk:          RiskAdvanced,
			RequiredLevel: "advanced",
			AutoRun:       false,
			IntentTags:    []string{"schedule", "cron", "recurring", "periodic", "scheduled_scan", "scheduled_prompt"},
			CanDo:         []string{"register a cron-driven recurring agent_prompt for arbitrary Agent work", "register optimized scan_web schedules for explicit recurring safe Web/API scans"},
			CannotDo:      []string{"run scan immediately (use the immediate tool for one-off work)"},
			Examples: LocalizedExamples{
				{ZH: "每天 7:20 扫描 example.com 开放端口 → schedule.create cron=\"20 7 * * *\" action=agent_prompt", EN: "daily 7:20 scan open ports for example.com → schedule.create cron=\"20 7 * * *\" action=agent_prompt"},
				{ZH: "每小时安全漏扫 https://example.com → schedule.create cron=\"0 * * * *\" action=scan_web", EN: "every hour vulnerability scan https://example.com → schedule.create cron=\"0 * * * *\" action=scan_web"},
			},
			SkillID: "schedule_management",
		},
		{
			Name: "schedule.list",
			Aliases: []string{
				"schedules.list", "list.schedules", "list.schedule",
				"cron.list", "schedules",
			},
			Description:   LocalizedText{ZH: "List currently configured schedules together with their cron expression, action, last run, and next-run time."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Risk:          RiskReadOnly,
			RequiredLevel: "read_only",
			AutoRun:       true,
			IntentTags:    []string{"schedule", "cron", "list_schedules"},
			CanDo:         []string{"list configured schedules with cron, action, last/next run"},
		},
		{
			Name: "schedule.delete",
			Aliases: []string{
				"schedule.remove", "schedules.delete", "delete.schedule",
				"remove.schedule", "cron.delete", "cron.remove",
			},
			Description:   LocalizedText{ZH: "Delete an existing schedule by id when the operator explicitly asks to cancel/remove it. Requires advanced operation level."},
			InputSchema:   json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Schedule id returned by schedule.list"}},"required":["id"],"additionalProperties":false}`),
			Risk:          RiskAdvanced,
			RequiredLevel: "advanced",
			AutoRun:       false,
			IntentTags:    []string{"schedule", "cron", "cancel_schedule"},
			CanDo:         []string{"cancel a configured schedule by id"},
		},
	}
}
