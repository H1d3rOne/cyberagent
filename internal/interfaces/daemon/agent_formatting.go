package daemon

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/command"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	doctorruntime "github.com/H1d3rOne/cyberagent/internal/runtime/doctor"
	"github.com/H1d3rOne/cyberagent/internal/workspace"
)

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
		agentIsWorkspaceTreeIntent(prompt) ||
		agentIsWorkspaceSearchIntent(prompt) ||
		agentIsWorkspaceWriteIntent(prompt) ||
		agentIsWorkspaceDiffIntent(prompt) ||
		agentIsWorkspaceRebaseIntent(prompt) ||
		agentIsWorkspaceApplyAllIntent(prompt) ||
		agentIsWorkspaceApplyIntent(prompt) ||
		agentIsWorkspaceDiscardAllIntent(prompt) ||
		agentIsWorkspaceDiscardIntent(prompt) ||
		agentIsWorkspaceDeleteIntent(prompt) ||
		agentIsWorkspaceMoveIntent(prompt) ||
		agentIsWorkspaceCopyIntent(prompt) ||
		agentIsWorkspaceMkdirIntent(prompt)
}

func agentIsWorkspaceTreeIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(tree|file\s+tree|directory\s+tree|文件树|目录树|结构树|递归列出)`).MatchString(prompt)
}

func agentIsWorkspaceSearchIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(search|grep|find\s+text|find\s+string|查找|搜索|检索|全文搜索)`).MatchString(prompt) &&
		regexp.MustCompile(`(?i)(workspace|工作空间|file|files|文件|目录|path|路径|内容|text|string|pattern|query)`).MatchString(prompt)
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

func agentFormatWorkspaceTree(language, relPath string, entries []agentWorkspaceTreeEntry, truncated bool) string {
	if len(entries) == 0 {
		return agentText(language,
			fmt.Sprintf("工作空间目录 `%s` 没有可展示的文件树条目。", relPath),
			fmt.Sprintf("Workspace directory `%s` has no tree entries to show.", relPath),
		)
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		prefix := strings.Repeat("  ", maxInt(entry.Depth-1, 0))
		icon := "📄"
		if entry.IsDir {
			icon = "📁"
		}
		lines = append(lines, fmt.Sprintf("%s- %s `%s`", prefix, icon, entry.Path))
	}
	if truncated {
		lines = append(lines, agentText(language, "- … 已达到展示上限", "- … display limit reached"))
	}
	return agentText(language,
		fmt.Sprintf("工作空间目录 `%s` 文件树：\n\n%s", relPath, strings.Join(lines, "\n")),
		fmt.Sprintf("Workspace directory `%s` tree:\n\n%s", relPath, strings.Join(lines, "\n")),
	)
}

func agentFormatWorkspaceSearch(language, query string, matches []agentWorkspaceSearchMatch, filesScanned int, truncated bool) string {
	if len(matches) == 0 {
		return agentText(language,
			fmt.Sprintf("工作空间搜索 `%s` 没有命中（扫描文件 %d 个）。", query, filesScanned),
			fmt.Sprintf("Workspace search `%s` found no matches (%d file(s) scanned).", query, filesScanned),
		)
	}
	limit := minInt(len(matches), 40)
	lines := make([]string, 0, limit+1)
	for _, match := range matches[:limit] {
		lines = append(lines, fmt.Sprintf("- `%s:%d` %s", match.Path, match.Line, match.Text))
	}
	if truncated || len(matches) > limit {
		lines = append(lines, agentText(language, "- … 已达到搜索结果上限", "- … search result limit reached"))
	}
	return agentText(language,
		fmt.Sprintf("工作空间搜索 `%s` 命中 %d 条（扫描文件 %d 个）：\n\n%s", query, len(matches), filesScanned, strings.Join(lines, "\n")),
		fmt.Sprintf("Workspace search `%s` found %d match(es) (%d file(s) scanned):\n\n%s", query, len(matches), filesScanned, strings.Join(lines, "\n")),
	)
}

func agentFormatHTTPRequest(language string, result map[string]any) string {
	method := stringFromAny(result["method"])
	targetURL := stringFromAny(result["url"])
	status := stringFromAny(result["status"])
	body := strings.TrimSpace(stringFromAny(result["body"]))
	truncated := boolFromAny(result["body_truncated"])
	if body != "" {
		body = agentLimitBlock(body, 900)
		if truncated {
			body += agentText(language, "\n…（响应体已截断）", "\n…(response body truncated)")
		}
	}
	if body == "" {
		return agentText(language,
			fmt.Sprintf("HTTP 响应：`%s %s`，status: `%s`。", method, targetURL, status),
			fmt.Sprintf("HTTP response: `%s %s`, status: `%s`.", method, targetURL, status),
		)
	}
	return agentText(language,
		fmt.Sprintf("HTTP 响应：`%s %s`，status: `%s`。\n\nbody 预览：\n```text\n%s\n```", method, targetURL, status, body),
		fmt.Sprintf("HTTP response: `%s %s`, status: `%s`.\n\nBody preview:\n```text\n%s\n```", method, targetURL, status, body),
	)
}

func agentFormatWorkspaceWrite(language, relPath, content string, size int) string {
	return agentText(language,
		fmt.Sprintf("已写入工作空间文件 `%s`（%d bytes）。\n\n内容预览：\n```text\n%s\n```", relPath, size, agentLimitBlock(content, 1200)),
		fmt.Sprintf("Wrote workspace file `%s` (%d bytes).\n\nPreview:\n```text\n%s\n```", relPath, size, agentLimitBlock(content, 1200)),
	)
}

func agentFormatWorkspaceDelete(language, relPath string) string {
	return agentText(language,
		fmt.Sprintf("已删除工作空间文件 `%s`。", relPath),
		fmt.Sprintf("Deleted workspace file `%s`.", relPath),
	)
}

func agentFormatWorkspaceMove(language, fromPath, toPath string) string {
	return agentText(language,
		fmt.Sprintf("已移动工作空间文件 `%s` → `%s`。", fromPath, toPath),
		fmt.Sprintf("Moved workspace file `%s` → `%s`.", fromPath, toPath),
	)
}

func agentFormatWorkspaceCopy(language, fromPath, toPath string) string {
	return agentText(language,
		fmt.Sprintf("已复制工作空间文件 `%s` → `%s`。", fromPath, toPath),
		fmt.Sprintf("Copied workspace file `%s` → `%s`.", fromPath, toPath),
	)
}

func agentFormatWorkspaceMkdir(language, relPath string) string {
	return agentText(language,
		fmt.Sprintf("已创建工作空间目录 `%s`。", relPath),
		fmt.Sprintf("Created workspace directory `%s`.", relPath),
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
