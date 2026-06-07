package daemon

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func appendAgentInstructionBlock(base, block string) string {
	base = strings.TrimSpace(base)
	block = strings.TrimSpace(block)
	if block == "" {
		return base
	}
	if base == "" {
		return block
	}
	return base + "\n\n" + block
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
		key := agentPlannerActionDedupeKey(name, action.Arguments)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		actions = append(actions, action)
	}
	return actions
}

func agentPlannerActionDedupeKey(name string, arguments map[string]any) string {
	keyParts := []string{
		name,
		stringFromAny(arguments["host"]),
		agentPortSpecFromPlannerArgument(arguments["ports"]),
		stringFromAny(arguments["url"]),
		stringFromAny(arguments["target"]),
		stringFromAny(arguments["method"]),
		stringFromAny(arguments["query"]),
		stringFromAny(arguments["pattern"]),
		stringFromAny(arguments["command"]),
		stringFromAny(arguments["path"]),
		stringFromAny(arguments["file"]),
		stringFromAny(arguments["content"]),
		stringFromAny(arguments["body"]),
		stringFromAny(arguments["text"]),
		stringFromAny(arguments["old"]),
		stringFromAny(arguments["old_text"]),
		stringFromAny(arguments["before"]),
		stringFromAny(arguments["search"]),
		stringFromAny(arguments["new"]),
		stringFromAny(arguments["new_text"]),
		stringFromAny(arguments["after"]),
		stringFromAny(arguments["replace"]),
		stringFromAny(arguments["change_id"]),
		stringFromAny(arguments["id"]),
	}
	return strings.Join(keyParts, "\x00")
}

// normalizeAgentPlannerActionName collapses planner-emitted tool names
// (canonical, alias, casing variant, dash/underscore variants) into the
// canonical name held by the daemon tool registry. Unknown names pass through
// folded so plugin/MCP names still flow downstream.
//
// The implementation is a thin wrapper over toolregistry.Registry.Normalize
// to keep a single source of truth; a parity test in
// agent_toolregistry_parity_test.go guards against drift while remaining
// callers migrate to the registry directly.
func normalizeAgentPlannerActionName(raw string) string {
	if mcpName := normalizeAgentMCPToolActionName(raw); mcpName != "" {
		return mcpName
	}
	return toolRegistry().Normalize(raw)
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
		if strings.Contains(name, "workspace.files.read") || strings.Contains(name, "workspace.files.write") || strings.Contains(name, "workspace.files.patch") || strings.Contains(name, "workspace.files.diff") || strings.Contains(name, "workspace.files.apply") || strings.Contains(name, "workspace.files.discard") || strings.Contains(name, "workspace.files.delete") || strings.Contains(name, "workspace.files.move") || strings.Contains(name, "workspace.files.copy") || strings.Contains(name, "workspace.files.mkdir") || strings.Contains(name, "reports.draft") {
			kind := "workspace_file"
			label := "file " + path
			if strings.Contains(name, "workspace.files.mkdir") {
				kind = "workspace_dir"
				label = "dir " + path
			}
			refs = append(refs, agentContextRef{
				ID:      "workspace_file:" + path,
				Kind:    kind,
				RefID:   path,
				Label:   label,
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

type agentToolFailureRecovery struct {
	Type            string
	Reason          string
	SuggestedPrompt string
	TodoTitle       string
	TodoDetail      string
	AutoContinue    bool
}

func agentRecoveryForFailedTool(language string, call agentToolCall) agentToolFailureRecovery {
	action := firstNonEmpty(stringFromAny(call.Result["action"]), stringFromAny(call.Result["action_name"]), call.Name)
	code := ""
	if call.Error != nil {
		code = string(call.Error.Code)
	}
	details := mapFromAny(call.Result["details"])
	requiredLevel := firstNonEmpty(stringFromAny(call.Result["required_operation_level"]), stringFromAny(details["required_operation_level"]))
	policy := firstNonEmpty(stringFromAny(call.Result["policy"]), stringFromAny(details["policy"]))
	reason := agentToolError(call)
	displayAction := agentRecoveryActionLabel(action, language)

	switch {
	case requiredLevel != "":
		return agentToolFailureRecovery{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "切换 Agent 操作权限到 "+requiredLevel+" 后重试："+displayAction, "switch Agent operation level to "+requiredLevel+" and retry: "+displayAction),
			TodoTitle:       agentText(language, "调整 Agent 操作权限后重试", "Adjust Agent operation level and retry"),
			TodoDetail:      reason,
		}
	case policy == "explicit_operator_intent_required":
		return agentToolFailureRecovery{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "请明确确认要执行 "+displayAction+"，并重新说明目标、范围和参数", "explicitly confirm "+displayAction+" and restate the target, scope, and parameters"),
			TodoTitle:       agentText(language, "等待明确的执行确认", "Wait for explicit execution confirmation"),
			TodoDetail:      reason,
		}
	case action == "http.request" && (strings.Contains(reason, "auth profile") || strings.Contains(reason, "Authorization") || strings.Contains(reason, "header")):
		return agentToolFailureRecovery{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "到设置页修正 API Auth Profile / Base URL 后，重新请求同一授权 URL", "fix the API auth profile / Base URL in Settings, then retry the same authorized URL"),
			TodoTitle:       agentText(language, "修正 API Auth Profile", "Fix API auth profile"),
			TodoDetail:      reason,
		}
	case code == string(services.CodeConflict) || strings.Contains(strings.ToLower(reason), "conflict"):
		return agentToolFailureRecovery{
			Type:            "continue",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "重新读取相关文件，使用当前内容 rebase 待应用变更，再预览 diff", "reread the related file, rebase the pending change with current content, then preview the diff"),
			TodoTitle:       agentText(language, "重新基于当前状态生成变更", "Regenerate change from current state"),
			TodoDetail:      reason,
			AutoContinue:    true,
		}
	case code == string(services.CodeNotFound):
		return agentToolFailureRecovery{
			Type:            "continue",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "先列出当前可用资源或工作区文件，再选择正确 ID / 路径重试", "list current resources or workspace files first, then retry with the correct ID/path"),
			TodoTitle:       agentText(language, "重新定位正确资源", "Relocate the correct resource"),
			TodoDetail:      reason,
			AutoContinue:    true,
		}
	case code == string(services.CodeToolDependencyMissing):
		return agentToolFailureRecovery{
			Type:            "wait_user",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "安装或启用缺失工具后重试，或让我改用已有内置工具", "install/enable the missing tool and retry, or ask me to use available built-in tools"),
			TodoTitle:       agentText(language, "补齐缺失工具依赖", "Install missing tool dependency"),
			TodoDetail:      reason,
		}
	case code == string(services.CodeUnsupported):
		return agentToolFailureRecovery{
			Type:            "continue",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "列出可用内置工具，并选择一个支持的替代动作", "list available built-in tools and choose a supported alternative"),
			TodoTitle:       agentText(language, "选择支持的替代工具", "Choose a supported alternative tool"),
			TodoDetail:      reason,
			AutoContinue:    true,
		}
	default:
		return agentToolFailureRecovery{
			Type:            "blocked",
			Reason:          reason,
			SuggestedPrompt: agentText(language, "检查失败原因，调整参数后重试："+displayAction, "inspect the failure, adjust arguments, then retry: "+displayAction),
			TodoTitle:       agentText(language, "分析失败并准备重试", "Analyze failure and prepare retry"),
			TodoDetail:      reason,
		}
	}
}

func agentRecoveryActionLabel(action, language string) string {
	normalized := normalizeAgentPlannerActionName(action)
	switch normalized {
	case "workspace.files.write":
		return agentText(language, "写入文件（workspace.files.write）", "write file (workspace.files.write)")
	case "workspace.files.patch":
		return agentText(language, "直接修改文件（workspace.files.patch）", "patch file (workspace.files.patch)")
	case "workspace.files.diff", "workspace.files.diff_many":
		return agentText(language, "预览工作区变更（"+normalized+"）", "preview workspace change ("+normalized+")")
	case "workspace.files.apply":
		return agentText(language, "应用待处理变更（workspace.files.apply）", "apply pending change (workspace.files.apply)")
	case "workspace.files.discard":
		return agentText(language, "丢弃待处理变更（workspace.files.discard）", "discard pending change (workspace.files.discard)")
	case "workspace.files.delete":
		return agentText(language, "删除工作区文件（workspace.files.delete）", "delete workspace file (workspace.files.delete)")
	case "workspace.files.move":
		return agentText(language, "移动/重命名工作区文件（workspace.files.move）", "move/rename workspace file (workspace.files.move)")
	case "workspace.files.copy":
		return agentText(language, "复制工作区文件（workspace.files.copy）", "copy workspace file (workspace.files.copy)")
	case "workspace.files.mkdir":
		return agentText(language, "创建工作区目录（workspace.files.mkdir）", "create workspace directory (workspace.files.mkdir)")
	case "http.request":
		return agentText(language, "HTTP 请求（http.request）", "HTTP request (http.request)")
	case "scan.web.safe":
		return agentText(language, "safe Web/API 扫描（scan.web.safe）", "safe Web/API scan (scan.web.safe)")
	case "runs.context":
		return agentText(language, "读取运行上下文（runs.context）", "read run context (runs.context)")
	default:
		if strings.TrimSpace(action) == "" {
			return agentText(language, "该动作", "the action")
		}
		return action
	}
}

func mapFromAny(value any) map[string]any {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case map[string]any:
		return v
	default:
		return nil
	}
}

func upsertAgentRecoveryTodo(session *agentSession, call agentToolCall, recovery agentToolFailureRecovery, now time.Time) {
	title := strings.TrimSpace(recovery.TodoTitle)
	if session == nil || title == "" {
		return
	}
	id := "todo_recover_" + agentCleanToken(call.ID)
	todo := agentTodo{
		ID:            id,
		Title:         trimSummary(title, 120),
		Detail:        trimSummary(recovery.TodoDetail, 220),
		Status:        "blocked",
		Priority:      len(session.Todos) + 1,
		ToolCallID:    call.ID,
		ContextRefIDs: contextRefIDsFromToolCall(call),
		BlockedReason: trimSummary(recovery.Reason, 220),
		UpdatedAt:     now,
	}
	if recovery.AutoContinue {
		todo.Status = "pending"
		todo.BlockedReason = ""
	}
	for idx := range session.Todos {
		if session.Todos[idx].ID == id {
			session.Todos[idx] = todo
			return
		}
	}
	session.Todos = append(session.Todos, todo)
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func stringMapFromAny(value any) map[string]string {
	out := map[string]string{}
	switch v := value.(type) {
	case map[string]string:
		for key, val := range v {
			key = strings.TrimSpace(key)
			if key != "" && !strings.ContainsAny(key, "\r\n:") && !strings.ContainsAny(val, "\r\n") {
				out[key] = val
			}
		}
	case map[string]any:
		for key, val := range v {
			key = strings.TrimSpace(key)
			value := stringFromAny(val)
			if key != "" && value != "" && !strings.ContainsAny(key, "\r\n:") && !strings.ContainsAny(value, "\r\n") {
				out[key] = value
			}
		}
	}
	if len(out) > 20 {
		keys := make([]string, 0, len(out))
		for key := range out {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		trimmed := map[string]string{}
		for _, key := range keys[:20] {
			trimmed[key] = out[key]
		}
		return trimmed
	}
	return out
}

func agentRedactHTTPHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "token") || strings.Contains(lower, "key") {
			out[key] = "[redacted]"
			continue
		}
		out[key] = value
	}
	return out
}

// agentURLOutboundAllowed rejects URLs whose host resolves to non-public IP
// space (loopback, RFC1918, link-local, CGNAT, multicast, unspecified) or to
// the well-known cloud metadata endpoints. Used by web.fetch / web.search /
// http.request to keep the agent from probing the host's internal network.
func agentURLOutboundAllowed(parsed *url.URL) error {
	if parsed == nil {
		return services.NewError(services.CodeBadRequest, "missing url", nil, nil)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return services.NewError(services.CodeBadRequest, "url has no host", map[string]any{"url": parsed.String()}, nil)
	}
	switch host {
	case "localhost", "metadata", "metadata.google.internal", "metadata.goog", "instance-data":
		return services.NewError(services.CodePolicyDenied, "outbound to internal host blocked", map[string]any{"host": host}, nil)
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := agentIPOutboundAllowed(ip, host); err != nil {
			return err
		}
		return nil
	}
	resolveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err != nil {
		return services.NewError(services.CodeBadRequest, "dns resolution failed: "+err.Error(), map[string]any{"host": host}, err)
	}
	if len(ips) == 0 {
		return services.NewError(services.CodeBadRequest, "host did not resolve", map[string]any{"host": host}, nil)
	}
	for _, addr := range ips {
		if err := agentIPOutboundAllowed(addr.IP, host); err != nil {
			return err
		}
	}
	return nil
}

func agentIPOutboundAllowed(ip net.IP, host string) error {
	if ip == nil {
		return services.NewError(services.CodeBadRequest, "invalid ip", map[string]any{"host": host}, nil)
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() || ip.IsInterfaceLocalMulticast() {
		return services.NewError(services.CodePolicyDenied, "outbound to non-public ip blocked", map[string]any{"host": host, "ip": ip.String()}, nil)
	}
	// CGNAT 100.64.0.0/10
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return services.NewError(services.CodePolicyDenied, "outbound to CGNAT ip blocked", map[string]any{"host": host, "ip": ip.String()}, nil)
	}
	// 169.254.169.254 covered by IsLinkLocalUnicast above.
	return nil
}

func agentHTTPAuthProfileEnabled(profile agentHTTPAuthProfile) bool {
	return profile.Enabled == nil || *profile.Enabled
}

func normalizeAgentHTTPAuthProfileType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "basic":
		return "basic"
	case "api_key", "apikey", "api-key":
		return "api_key"
	case "cookie":
		return "cookie"
	case "custom", "header":
		return "custom"
	default:
		return "bearer"
	}
}

func agentResolveHTTPAuthProfileHeaders(profiles []agentHTTPAuthProfile, requestedID, defaultID, targetURL string) (map[string]string, *agentHTTPAuthProfileUsage, error) {
	requestedID = strings.TrimSpace(requestedID)
	defaultID = strings.TrimSpace(defaultID)
	explicit := requestedID != ""
	profileID := requestedID
	if profileID == "" {
		profileID = defaultID
	}
	if profileID == "" || len(profiles) == 0 {
		return nil, nil, nil
	}

	profile, ok := agentFindHTTPAuthProfile(profiles, profileID)
	if !ok {
		if explicit {
			return nil, nil, services.NewError(services.CodeBadRequest, "http.request auth profile was not found", map[string]any{"auth_profile_id": profileID}, nil)
		}
		return nil, nil, nil
	}
	if !agentHTTPAuthProfileEnabled(profile) {
		if explicit {
			return nil, nil, services.NewError(services.CodePolicyDenied, "http.request auth profile is disabled", map[string]any{"auth_profile_id": profileID}, nil)
		}
		return nil, nil, nil
	}
	if !explicit && strings.TrimSpace(profile.BaseURL) == "" {
		// Default auth profiles must be host-scoped to avoid accidentally
		// attaching credentials to an unrelated URL selected by the model.
		return nil, nil, nil
	}
	matches, host := agentHTTPAuthProfileMatchesTarget(profile, targetURL)
	if !matches {
		if explicit {
			return nil, nil, services.NewError(
				services.CodePolicyDenied,
				"http.request auth profile does not match the request host",
				map[string]any{"auth_profile_id": firstNonEmpty(profile.ID, profile.Name), "base_url": profile.BaseURL, "url": targetURL},
				nil,
			)
		}
		return nil, nil, nil
	}
	headers, err := agentHTTPAuthProfileHeaders(profile)
	if err != nil {
		return nil, nil, err
	}
	if len(headers) == 0 {
		return nil, nil, nil
	}
	usage := &agentHTTPAuthProfileUsage{
		ID:      strings.TrimSpace(profile.ID),
		Name:    strings.TrimSpace(profile.Name),
		Type:    normalizeAgentHTTPAuthProfileType(profile.Type),
		BaseURL: strings.TrimSpace(profile.BaseURL),
		Host:    host,
		Applied: true,
	}
	return headers, usage, nil
}

func agentFindHTTPAuthProfile(profiles []agentHTTPAuthProfile, id string) (agentHTTPAuthProfile, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, profile := range profiles {
		if strings.ToLower(strings.TrimSpace(profile.ID)) == id || strings.ToLower(strings.TrimSpace(profile.Name)) == id {
			return profile, true
		}
	}
	return agentHTTPAuthProfile{}, false
}

func agentHTTPAuthProfileMatchesTarget(profile agentHTTPAuthProfile, targetURL string) (bool, string) {
	parsedTarget, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || parsedTarget == nil || parsedTarget.Host == "" {
		return false, ""
	}
	targetHost := strings.ToLower(parsedTarget.Hostname())
	if strings.TrimSpace(profile.BaseURL) == "" {
		return true, targetHost
	}
	base := strings.TrimSpace(profile.BaseURL)
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	parsedBase, err := url.Parse(base)
	if err != nil || parsedBase == nil || parsedBase.Host == "" {
		return false, targetHost
	}
	baseHost := strings.ToLower(parsedBase.Hostname())
	if baseHost == "" || baseHost != targetHost {
		return false, targetHost
	}
	basePort := parsedBase.Port()
	if basePort != "" && basePort != parsedTarget.Port() {
		return false, targetHost
	}
	return true, targetHost
}

func agentHTTPAuthProfileHeaders(profile agentHTTPAuthProfile) (map[string]string, error) {
	headers := map[string]string{}
	profileID := firstNonEmpty(profile.ID, profile.Name)
	switch normalizeAgentHTTPAuthProfileType(profile.Type) {
	case "bearer":
		token := strings.TrimSpace(profile.Token)
		if token == "" {
			token = strings.TrimSpace(profile.HeaderValue)
		}
		if token == "" {
			return nil, services.NewError(services.CodeBadRequest, "bearer auth profile requires token", map[string]any{"auth_profile_id": profileID}, nil)
		}
		headers["Authorization"] = "Bearer " + token
	case "basic":
		username := strings.TrimSpace(profile.Username)
		if username == "" {
			return nil, services.NewError(services.CodeBadRequest, "basic auth profile requires username", map[string]any{"auth_profile_id": profileID}, nil)
		}
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+profile.Password))
	case "api_key":
		headerName := firstNonEmpty(profile.HeaderName, "X-API-Key")
		value := firstNonEmpty(profile.HeaderValue, profile.Token)
		if value == "" {
			return nil, services.NewError(services.CodeBadRequest, "api_key auth profile requires header_value or token", map[string]any{"auth_profile_id": profileID}, nil)
		}
		headers[headerName] = value
	case "cookie":
		cookie := strings.TrimSpace(profile.Cookie)
		if cookie == "" {
			cookie = strings.TrimSpace(profile.HeaderValue)
		}
		if cookie == "" {
			return nil, services.NewError(services.CodeBadRequest, "cookie auth profile requires cookie", map[string]any{"auth_profile_id": profileID}, nil)
		}
		headers["Cookie"] = cookie
	case "custom":
		headerName := strings.TrimSpace(profile.HeaderName)
		value := strings.TrimSpace(profile.HeaderValue)
		if headerName == "" || value == "" {
			return nil, services.NewError(services.CodeBadRequest, "custom auth profile requires header_name and header_value", map[string]any{"auth_profile_id": profileID}, nil)
		}
		headers[headerName] = value
	}
	for key, value := range headers {
		if strings.ContainsAny(key, "\r\n:") || strings.ContainsAny(value, "\r\n") {
			return nil, services.NewError(services.CodeBadRequest, "auth profile contains an invalid header", map[string]any{"auth_profile_id": profileID, "header": key}, nil)
		}
	}
	return headers, nil
}
