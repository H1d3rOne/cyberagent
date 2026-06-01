package daemon

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

type agentTCPPortProbeResult struct {
	Port       int
	Open       bool
	Error      string
	DurationMS int64
}

func agentScanTCPPorts(ctx context.Context, host string, ports []int, timeout time.Duration) []agentTCPPortProbeResult {
	ports = normalizeAgentPortList(ports)
	if len(ports) == 0 {
		return nil
	}
	type job struct {
		index int
		port  int
	}
	jobs := make(chan job)
	results := make([]agentTCPPortProbeResult, len(ports))
	workers := minInt(24, len(ports))
	if workers < 1 {
		workers = 1
	}
	done := make(chan struct{}, workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for item := range jobs {
				start := time.Now()
				address := net.JoinHostPort(host, strconv.Itoa(item.port))
				dialer := net.Dialer{Timeout: timeout}
				conn, err := dialer.DialContext(ctx, "tcp", address)
				result := agentTCPPortProbeResult{Port: item.port, DurationMS: time.Since(start).Milliseconds()}
				if err == nil {
					result.Open = true
					_ = conn.Close()
				} else {
					result.Error = trimSummary(err.Error(), 120)
				}
				results[item.index] = result
			}
		}()
	}
	for idx, port := range ports {
		select {
		case <-ctx.Done():
			results[idx] = agentTCPPortProbeResult{Port: port, Error: ctx.Err().Error()}
		case jobs <- job{index: idx, port: port}:
		}
	}
	close(jobs)
	for worker := 0; worker < workers; worker++ {
		<-done
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Port < results[j].Port })
	return results
}

func agentExtractPortScanHost(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if strings.Contains(target, "://") {
		parsed, err := url.Parse(target)
		if err == nil {
			return strings.TrimSpace(parsed.Hostname())
		}
	}
	host := target
	if slash := strings.Index(host, "/"); slash >= 0 {
		host = host[:slash]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if colon := strings.LastIndex(host, ":"); colon > strings.LastIndex(host, "]") {
		if _, atoiErr := strconv.Atoi(host[colon+1:]); atoiErr == nil {
			host = host[:colon]
		}
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" || strings.ContainsAny(host, " \t\r\n") {
		return ""
	}
	return host
}

func agentDefaultSafePortList() []int {
	return []int{21, 22, 25, 53, 80, 110, 143, 443, 445, 465, 587, 993, 995, 1433, 1521, 2049, 2375, 2376, 3000, 3306, 5000, 5432, 5601, 5672, 5900, 6379, 8000, 8080, 8443, 9000, 9200, 9300, 11211, 27017}
}

func agentExtractSafePortScanPorts(prompt string) ([]int, string, error) {
	if token := agentExtractPortSpecToken(prompt); token != "" {
		ports, err := parseAgentPortSpec(token, 128)
		if err != nil {
			return nil, "", err
		}
		return ports, token, nil
	}
	ports := agentDefaultSafePortList()
	return ports, "common_safe", nil
}

func agentExtractPortSpecToken(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:^|\s)-p\s+([0-9][0-9,\-\s]{0,120})`),
		regexp.MustCompile(`(?i)\bports?\s*[:=]\s*([0-9][0-9,\-\s]{0,120})`),
		regexp.MustCompile(`(?:端口|端口范围)\s*[:：]\s*([0-9][0-9,\-\s]{0,120})`),
		regexp.MustCompile(`(?i)\b([0-9]{1,5}\s*-\s*[0-9]{1,5})\b`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return strings.Join(strings.Fields(strings.TrimSpace(match[1])), "")
		}
	}
	return ""
}

func parseAgentPortSpec(spec string, maxPorts int) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, services.NewError(services.CodeBadRequest, "port spec is required", nil, nil)
	}
	seen := map[int]struct{}{}
	ports := []int{}
	addPort := func(port int) error {
		if port < 1 || port > 65535 {
			return services.NewError(services.CodeBadRequest, "port out of range", map[string]any{"port": port}, nil)
		}
		if _, ok := seen[port]; ok {
			return nil
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
		if maxPorts > 0 && len(ports) > maxPorts {
			return services.NewError(services.CodePolicyDenied, "too many ports for safe built-in scan; narrow the range or use a dedicated nmap/masscan plugin", map[string]any{"max_ports": maxPorts}, nil)
		}
		return nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil {
				return nil, services.NewError(services.CodeBadRequest, "invalid port range", map[string]any{"range": part}, err)
			}
			end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, services.NewError(services.CodeBadRequest, "invalid port range", map[string]any{"range": part}, err)
			}
			if start > end {
				start, end = end, start
			}
			for port := start; port <= end; port++ {
				if err := addPort(port); err != nil {
					return nil, err
				}
			}
			continue
		}
		port, err := strconv.Atoi(part)
		if err != nil {
			return nil, services.NewError(services.CodeBadRequest, "invalid port", map[string]any{"port": part}, err)
		}
		if err := addPort(port); err != nil {
			return nil, err
		}
	}
	return normalizeAgentPortList(ports), nil
}

func agentPortSpecFromPlannerArgument(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []int:
		parts := make([]string, 0, len(typed))
		for _, port := range typed {
			parts = append(parts, strconv.Itoa(port))
		}
		return strings.Join(parts, ",")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			switch v := item.(type) {
			case float64:
				parts = append(parts, strconv.Itoa(int(v)))
			case int:
				parts = append(parts, strconv.Itoa(v))
			case string:
				if strings.TrimSpace(v) != "" {
					parts = append(parts, strings.TrimSpace(v))
				}
			}
		}
		return strings.Join(parts, ",")
	default:
		return ""
	}
}

func normalizeAgentPortList(ports []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(ports))
	for _, port := range ports {
		if port < 1 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}
	sort.Ints(out)
	return out
}

func agentExtractHTTPMethod(prompt string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(GET|HEAD|OPTIONS|POST|PUT|PATCH|DELETE)\b`),
		regexp.MustCompile(`(?i)method\s*[:=]\s*(GET|HEAD|OPTIONS|POST|PUT|PATCH|DELETE)`),
		regexp.MustCompile(`(?i)方法\s*[:：]\s*(GET|HEAD|OPTIONS|POST|PUT|PATCH|DELETE)`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return strings.ToUpper(match[1])
		}
	}
	if regexp.MustCompile(`(?i)(headers?|响应头|只看头)`).MatchString(prompt) {
		return http.MethodHead
	}
	return http.MethodGet
}

func agentHTTPMethodIsSafe(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "", http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func agentIsHealthIntent(prompt string) bool {
	if agentExtractURL(prompt) != "" && regexp.MustCompile(`(?i)(http|headers?|status\s*code|response|响应|状态码|响应头)`).MatchString(prompt) {
		return false
	}
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
		regexp.MustCompile(`(?i)(?:read|cat|open|list|ls|delete|remove|unlink|rm)\s+(?:path|file|directory|folder|files?)?\s*([A-Za-z0-9._/\-]+)`),
		regexp.MustCompile(`(?:路径|文件|目录)\s*[:：]?\s*([^\s,，;；]+)`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return agentCleanToken(match[1])
		}
	}
	return "."
}

func agentExtractWorkspaceSearchQuery(prompt string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile("`([^`]+)`"),
		regexp.MustCompile(`(?i)(?:query|pattern|text|string)\s*[:=]\s*([^\n]+)$`),
		regexp.MustCompile(`(?:查询|搜索|查找|检索|关键词|模式)\s*[:：]\s*([^\n]+)$`),
		regexp.MustCompile(`(?i)(?:search|grep|find\s+text|find\s+string)\s+(?:for\s+)?["“]?([^"”\n]+)["”]?`),
		regexp.MustCompile(`(?:搜索|查找|检索)\s*["“]?([^"”\n]+)["”]?`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			value := strings.TrimSpace(match[1])
			value = regexp.MustCompile(`(?i)\s+(?:in|under|path|directory|folder|workspace)\b.*$`).ReplaceAllString(value, "")
			value = regexp.MustCompile(`\s+(?:在|目录|路径|工作空间).*$`).ReplaceAllString(value, "")
			return agentCleanToken(value)
		}
	}
	return ""
}

func agentExtractWorkspaceSearchPath(prompt, query string) string {
	path := agentExtractPath(prompt)
	if strings.TrimSpace(path) == "" || path == "." || path == query {
		return "."
	}
	return path
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

func agentExtractWorkspaceMovePaths(prompt string) (string, string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", ""
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)(?:from|source|old|path)\s*[:=]\s*([^\s,，;；]+).*?(?:to|target|new|dest|destination)\s*[:=]\s*([^\s,，;；]+)$`),
		regexp.MustCompile(`(?is)(?:文件|路径)\s*[:：]\s*([^\s,，;；]+).*?(?:到|为|改为|移动到|重命名为)\s*([^\s,，;；]+)$`),
		regexp.MustCompile(`(?is)(?:move|rename|relocate|copy|duplicate|mv|cp)\s+(?:workspace\s+)?(?:file\s+)?([^\s,;]+)\s+(?:to|as|into)\s+([^\s,;]+)$`),
		regexp.MustCompile(`(?is)(?:重命名|移动|改名|转移|复制|拷贝)(?:文件|路径)?\s*([^\s,，;；]+)\s*(?:为|成|到|到路径)\s*([^\s,，;；]+)$`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 2 {
			return agentCleanWorkspaceWritePath(match[1]), agentCleanWorkspaceWritePath(match[2])
		}
	}
	return agentExtractWorkspaceWritePath(prompt), ""
}

func agentExtractWorkspaceMkdirPath(prompt string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile("`([^`]+)`"),
		regexp.MustCompile(`(?i)(?:path|directory|dir|folder)\s*[:=]\s*([^\s,，;；]+)`),
		regexp.MustCompile(`(?:目录|文件夹|路径)\s*[:：]\s*([^\s,，;；]+)`),
		regexp.MustCompile(`(?i)(?:mkdir|make\s+dir|make\s+directory|create\s+dir|create\s+directory)\s+([^\s,;]+)`),
		regexp.MustCompile(`(?:创建目录|新建目录|创建文件夹|新建文件夹)\s*([^\s,，;；]+)`),
	} {
		if match := re.FindStringSubmatch(prompt); len(match) > 1 {
			return agentCleanWorkspaceWritePath(match[1])
		}
	}
	return "."
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

func agentIsWorkspaceDeleteIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(delete|remove|unlink|rm|删除|移除)`).MatchString(prompt) &&
		regexp.MustCompile(`(?i)(workspace|工作空间|file|path\s*[:=]|file\s*[:=]|文件|路径|[A-Za-z0-9._/\-]+\.[A-Za-z0-9][A-Za-z0-9._\-]*)`).MatchString(prompt)
}

func agentIsWorkspaceMoveIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(move|rename|relocate|mv|重命名|移动|改名|转移)`).MatchString(prompt) &&
		regexp.MustCompile(`(?i)(workspace|工作空间|file|path\s*[:=]|file\s*[:=]|文件|路径|[A-Za-z0-9._/\-]+\.[A-Za-z0-9][A-Za-z0-9._\-]*)`).MatchString(prompt)
}

func agentIsWorkspaceCopyIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(copy|duplicate|cp|复制|拷贝|副本)`).MatchString(prompt) &&
		regexp.MustCompile(`(?i)(workspace|工作空间|file|path\s*[:=]|file\s*[:=]|文件|路径|[A-Za-z0-9._/\-]+\.[A-Za-z0-9][A-Za-z0-9._\-]*)`).MatchString(prompt)
}

func agentIsWorkspaceMkdirIntent(prompt string) bool {
	return regexp.MustCompile(`(?i)(mkdir|make\s+dir|make\s+directory|create\s+dir|create\s+directory|创建目录|新建目录|创建文件夹|新建文件夹)`).MatchString(prompt)
}

func agentPlannerActionAllowedByPrompt(prompt, actionName string) bool {
	switch normalizeAgentPlannerActionName(actionName) {
	case "scan.web.safe":
		return agentIsScanIntent(prompt)
	case "ports.scan.safe":
		return agentIsPortScanIntent(prompt)
	case "http.request":
		return agentIsHTTPRequestIntent(prompt) || agentIsAPIFunctionalTestIntent(prompt) || agentIsScanIntent(prompt)
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
	case "workspace.files.delete":
		return agentIsWorkspaceDeleteIntent(prompt)
	case "workspace.files.move":
		return agentIsWorkspaceMoveIntent(prompt)
	case "workspace.files.copy":
		return agentIsWorkspaceCopyIntent(prompt)
	case "workspace.files.mkdir":
		return agentIsWorkspaceMkdirIntent(prompt)
	case "runs.approvals.approve":
		return agentApprovalDecisionAction(prompt) == "approve" && agentIsApprovalDecisionIntent(prompt)
	case "runs.approvals.deny":
		return agentApprovalDecisionAction(prompt) == "deny" && agentIsApprovalDecisionIntent(prompt)
	case "runs.resume_approved":
		return agentIsResumeApprovedIntent(prompt)
	case "runs.context":
		return agentIsRunContextAnalysisIntent(prompt)
	case "reports.draft":
		return agentIsReportDraftIntent(prompt)
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

func agentNormalizeRunContextKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "artifact", "artifacts", "artifact_index", "artifact-index", "artifact index", "evidence_index", "evidence-index", "evidence index", "工件", "产物", "证据索引":
		return "artifact_index"
	case "event", "events", "timeline", "log", "logs", "trace", "事件", "时间线", "日志", "轨迹":
		return "events"
	case "approval", "approvals", "approve", "deny", "human", "hitl", "审批", "批准", "拒绝", "人工":
		return "approvals"
	case "finding", "findings", "vulnerability", "vulnerabilities", "issue", "issues", "发现", "漏洞", "问题":
		return "findings"
	case "report", "report_json", "report-json", "json", "报告":
		return "report_json"
	default:
		return "report_json"
	}
}
