package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentllm "github.com/H1d3rOne/cyberagent/internal/agent/llm"
	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/events"
	"github.com/H1d3rOne/cyberagent/internal/core/reporting"
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	webiface "github.com/H1d3rOne/cyberagent/internal/interfaces/web"
	artifactruntime "github.com/H1d3rOne/cyberagent/internal/runtime/artifacts"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
	runtimesqlite "github.com/H1d3rOne/cyberagent/internal/runtime/state/sqlite"
)

func TestNewServerCreatesTokenFileAndNormalizesDefaults(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "config", "token")
	server, err := NewServer(Config{RunsDir: t.TempDir(), TokenFile: tokenFile})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer server.Close()
	if server.Addr() != DefaultListen {
		t.Fatalf("Addr() = %q, want %q", server.Addr(), DefaultListen)
	}
	if server.TokenFile() != tokenFile {
		t.Fatalf("TokenFile() = %q, want %q", server.TokenFile(), tokenFile)
	}
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if len(data) < 32 {
		t.Fatalf("token file too short: %q", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(tokenFile)
		if err != nil {
			t.Fatalf("stat token file: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("token file permissions = %o, want 600", got)
		}
	}
}

func TestServerServeSupportsEmbeddedEphemeralLoopbackListener(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "config", "token")
	server, err := NewServer(Config{RunsDir: t.TempDir(), TokenFile: tokenFile})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = server.Close()
		t.Fatalf("listen ephemeral loopback: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ctx, ln)
	}()

	token, err := os.ReadFile(tokenFile)
	if err != nil {
		cancel()
		t.Fatalf("read token file: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ln.Addr().String()+"/api/v1/healthz", nil)
	if err != nil {
		cancel()
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("embedded health request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve() after cancel error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve() did not stop after context cancellation")
	}
}

func TestAgentRuntimeEngineEnabledFromEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty defaults to engine", value: "", want: true},
		{name: "engine explicit", value: "engine", want: true},
		{name: "true explicit", value: "true", want: true},
		{name: "legacy opt out", value: "legacy", want: false},
		{name: "off opt out", value: "off", want: false},
		{name: "disabled opt out", value: "disabled", want: false},
		{name: "false opt out", value: "false", want: false},
		{name: "zero opt out", value: "0", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := agentRuntimeEngineEnabledFromEnv(tc.value); got != tc.want {
				t.Fatalf("agentRuntimeEngineEnabledFromEnv(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestNewServerDefaultRuntimeEngineCanOptOutToLegacy(t *testing.T) {
	t.Setenv("CYBERAGENT_RUNTIME", "")
	server, err := NewServer(Config{RunsDir: t.TempDir(), TokenFile: filepath.Join(t.TempDir(), "token")})
	if err != nil {
		t.Fatalf("NewServer() default engine error = %v", err)
	}
	if server.runtimeStore == nil {
		t.Fatal("default NewServer() should open the runtime engine store")
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() default engine error = %v", err)
	}

	t.Setenv("CYBERAGENT_RUNTIME", "legacy")
	legacy, err := NewServer(Config{RunsDir: t.TempDir(), TokenFile: filepath.Join(t.TempDir(), "token")})
	if err != nil {
		t.Fatalf("NewServer() legacy opt-out error = %v", err)
	}
	defer legacy.Close()
	if legacy.runtimeStore != nil {
		t.Fatal("CYBERAGENT_RUNTIME=legacy should leave runtimeStore nil")
	}
}

func TestNewServerEngineRuntimeRecoversInterruptedRuns(t *testing.T) {
	t.Setenv("CYBERAGENT_RUNTIME", "")
	runsDir := t.TempDir()
	tokenFile := filepath.Join(t.TempDir(), "config", "token")
	ctx := context.Background()

	store, err := runtimesqlite.Open(agentRuntimeSQLitePath(runsDir))
	if err != nil {
		t.Fatalf("open runtime store: %v", err)
	}
	task := agentruntime.Task{
		ID:        "task-daemon-recover",
		SessionID: "session-daemon-recover",
		Origin:    "user",
		Goal:      "recover interrupted daemon run",
		Created:   time.Now().UTC().Add(-2 * time.Minute),
	}
	if err := store.SaveTask(ctx, task); err != nil {
		t.Fatalf("SaveTask() error = %v", err)
	}
	run := agentruntime.Run{
		ID:        "run-daemon-recover",
		TaskID:    task.ID,
		State:     string(agentruntime.RunRunning),
		CreatedAt: time.Now().UTC().Add(-time.Minute),
	}
	if err := store.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seed runtime store: %v", err)
	}

	server, err := NewServer(Config{RunsDir: runsDir, TokenFile: tokenFile})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := runtimesqlite.Open(agentRuntimeSQLitePath(runsDir))
	if err != nil {
		t.Fatalf("reopen runtime store: %v", err)
	}
	defer reopened.Close()
	recovered, err := reopened.LoadRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if recovered.State != string(agentruntime.RunFailed) {
		t.Fatalf("recovered run state = %q, want %q", recovered.State, agentruntime.RunFailed)
	}
	events, err := reopened.ListEvents(ctx, agentruntime.EventFilter{TaskID: task.ID}, 10)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	for _, ev := range events {
		if ev.Type != "task_failed" || ev.RunID != run.ID {
			continue
		}
		if ev.SessionID != task.SessionID {
			t.Fatalf("recovery event SessionID = %q, want %q", ev.SessionID, task.SessionID)
		}
		if got, _ := ev.Data["previous_state"].(string); got != string(agentruntime.RunRunning) {
			t.Fatalf("recovery event previous_state = %q, want %q", got, agentruntime.RunRunning)
		}
		if got, _ := ev.Data["recovery"].(bool); !got {
			t.Fatalf("recovery event recovery flag = %v, want true", got)
		}
		return
	}
	t.Fatalf("missing task_failed recovery event in %#v", events)
}

func TestParseAgentPlannerResponseUnwrapsOpenAIChatCompletions(t *testing.T) {
	raw := `{
		"id":"chatcmpl-test",
		"object":"chat.completion",
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":"{\"answer\":\"只展示给用户的主要内容\",\"task\":{\"goal\":\"整理输出\",\"kind\":\"chat\",\"status\":\"done\",\"summary\":\"已解析 OpenAI envelope\"},\"todos\":[],\"next_action\":{\"type\":\"finished\",\"reason\":\"done\",\"auto_continue\":false}}"
			},
			"finish_reason":"stop"
		}]
	}`

	plan, err := parseAgentPlannerResponse(raw)
	if err != nil {
		t.Fatalf("parseAgentPlannerResponse() error = %v", err)
	}
	if plan.Answer != "只展示给用户的主要内容" {
		t.Fatalf("Answer = %q", plan.Answer)
	}
	if plan.Task == nil || plan.Task.Goal != "整理输出" {
		t.Fatalf("Task = %#v", plan.Task)
	}
	if plan.NextAction == nil || plan.NextAction.Type != "finished" {
		t.Fatalf("NextAction = %#v", plan.NextAction)
	}
}

func TestParseAgentPlannerResponseUnwrapsOpenAIChatCompletionsSuggestPrompt(t *testing.T) {
	raw := `{
		"id":"chatcmpl-test",
		"object":"chat.completion",
		"suggest_prompt":"继续分析 HTTP 响应头",
		"choices":[{
			"index":0,
			"message":{
				"role":"assistant",
				"content":[{"type":"text","text":"{\"answer\":\"HEAD 请求完成\",\"task\":{\"goal\":\"HTTP 探测\",\"kind\":\"ops\",\"status\":\"running\",\"summary\":\"ok\"}}"}]
			},
			"finish_reason":"stop"
		}]
	}`

	plan, err := parseAgentPlannerResponse(raw)
	if err != nil {
		t.Fatalf("parseAgentPlannerResponse() error = %v", err)
	}
	if plan.Answer != "HEAD 请求完成" {
		t.Fatalf("Answer = %q", plan.Answer)
	}
	if plan.NextAction == nil || plan.NextAction.SuggestedPrompt != "继续分析 HTTP 响应头" {
		t.Fatalf("NextAction suggested prompt not merged: %#v", plan.NextAction)
	}
}

func TestParseAgentPlannerResponseFindsPlannerJSONAfterOperatorText(t *testing.T) {
	raw := "我先说明一下。\n```bash\npwd\n```\n\n" + `{
		"answer":"已提取主要 answer，不展示整段 JSON。",
		"task":{"goal":"解析混合输出","kind":"chat","status":"done","summary":"ok"},
		"todos":[],
		"next_action":{"type":"finished","reason":"ok","auto_continue":false}
	}`

	plan, err := parseAgentPlannerResponse(raw)
	if err != nil {
		t.Fatalf("parseAgentPlannerResponse() error = %v", err)
	}
	if plan.Answer != "已提取主要 answer，不展示整段 JSON。" {
		t.Fatalf("Answer = %q", plan.Answer)
	}
}

func TestParseAgentPlannerResponseAcceptsSuggestPromptAlias(t *testing.T) {
	raw := `{
		"answer":"建议下一步操作。",
		"suggest_prompt":"继续读取最近运行上下文并总结下一步",
		"task":{"goal":"解析建议","kind":"chat","status":"running","summary":"ok"}
	}`

	plan, err := parseAgentPlannerResponse(raw)
	if err != nil {
		t.Fatalf("parseAgentPlannerResponse() error = %v", err)
	}
	if plan.NextAction == nil {
		t.Fatalf("NextAction should be synthesized from suggest_prompt: %#v", plan)
	}
	if plan.NextAction.SuggestedPrompt != "继续读取最近运行上下文并总结下一步" {
		t.Fatalf("SuggestedPrompt = %q", plan.NextAction.SuggestedPrompt)
	}
}

func TestPortScanIntentUsesDedicatedSafePortTool(t *testing.T) {
	prompt := "扫描 ai.usmercari.com 开了哪些端口"
	if !agentIsPortScanIntent(prompt) {
		t.Fatalf("expected port scan intent")
	}
	if agentIsScanIntent(prompt) {
		t.Fatalf("port scan intent must not be classified as web safe scan")
	}
	actions := agentHeuristicActionsFromPrompt(prompt)
	if len(actions) != 1 || actions[0].Name != "ports.scan.safe" {
		t.Fatalf("port scan should produce ports.scan.safe action: %#v", actions)
	}
	plan := agentFallbackPlannerFromPrompt(prompt, "", "zh")
	if plan == nil || plan.Task == nil || plan.Task.Kind != "port_scan" || len(agentPlannerActions(plan)) != 1 {
		t.Fatalf("unexpected port scan fallback plan: %#v", plan)
	}
	if _, _, mismatch := agentPlannerActionCapabilityMismatch(prompt, "scan.web.safe", "zh"); !mismatch {
		t.Fatalf("scan.web.safe must be rejected as non-equivalent for port_scan")
	}
}

func TestGenericAnalysisDoesNotAutoStartWebSafeScan(t *testing.T) {
	if agentIsScanIntent("分析 ai.usmercari.com") {
		t.Fatalf("generic analysis must not be classified as web safe scan")
	}
	if agentIsScanIntent("检测 ai.usmercari.com") {
		t.Fatalf("generic detection must not be classified as web safe scan without security/vulnerability intent")
	}
	if !agentIsScanIntent("对 ai.usmercari.com 做安全扫描") {
		t.Fatalf("explicit security scan should be classified as web safe scan")
	}
	if !agentIsScanIntent("扫描 ai.usmercari.com 漏洞") {
		t.Fatalf("explicit vulnerability scan should be classified as web safe scan")
	}
}

func TestCapabilityClassifierPreventsCommonNonEquivalentTools(t *testing.T) {
	cases := []struct {
		name       string
		prompt     string
		intent     string
		wrongTool  string
		shouldScan bool
	}{
		{
			name:      "content discovery is not web safe scan",
			prompt:    "帮我找 https://example.com 的隐藏目录和备份文件",
			intent:    agentIntentContentDiscovery,
			wrongTool: "scan.web.safe",
		},
		{
			name:      "js route analysis is not web safe scan",
			prompt:    "分析 https://example.com 的 JS 路由和前端接口",
			intent:    agentIntentJSRouteAnalysis,
			wrongTool: "scan.web.safe",
		},
		{
			name:      "api auth test is not generic web safe scan",
			prompt:    "测试 https://api.example.com/login 是否存在认证绕过和越权",
			intent:    agentIntentAPIAuthTest,
			wrongTool: "scan.web.safe",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := agentClassifyCapabilityIntent(tc.prompt)
			if decision.ID != tc.intent {
				t.Fatalf("intent = %q, want %q", decision.ID, tc.intent)
			}
			if decision.Supported {
				t.Fatalf("intent %q should be blocked until an equivalent tool/input exists", decision.ID)
			}
			if agentIsScanIntent(tc.prompt) {
				t.Fatalf("%q must not be classified as scan.web.safe intent", tc.prompt)
			}
			plan := agentFallbackPlannerFromPrompt(tc.prompt, "", "zh")
			if plan == nil || plan.Task == nil || plan.Task.Status != "blocked" || len(agentPlannerActions(plan)) != 0 {
				t.Fatalf("expected blocked fallback plan without actions: %#v", plan)
			}
			if _, _, mismatch := agentPlannerActionCapabilityMismatch(tc.prompt, tc.wrongTool, "zh"); !mismatch {
				t.Fatalf("expected %s to be rejected as non-equivalent for %s", tc.wrongTool, tc.intent)
			}
		})
	}
}

func TestHTTPProbeAndPortFallbackRouting(t *testing.T) {
	statusPrompt := "获取 https://example.com 的响应头和状态码"
	if agentIsScanIntent(statusPrompt) {
		t.Fatalf("HTTP status/header probe must not be classified as web safe scan")
	}
	statusDecision := agentClassifyCapabilityIntent(statusPrompt)
	if statusDecision.ID != agentIntentHTTPProbe || !statusDecision.Supported {
		t.Fatalf("unexpected status probe decision: %#v", statusDecision)
	}
	statusActions := agentHeuristicActionsFromPrompt(statusPrompt)
	if len(statusActions) != 1 || statusActions[0].Name != "http.request" || statusActions[0].Arguments["method"] != http.MethodHead {
		t.Fatalf("unexpected status probe actions: %#v", statusActions)
	}

	portFallbackPrompt := "只验证常见 Web 端口 HTTP 响应，目标 example.com"
	if agentIsPortScanIntent(portFallbackPrompt) {
		t.Fatalf("explicit HTTP web-port fallback should not be treated as full port_scan")
	}
	actions := agentHeuristicActionsFromPrompt(portFallbackPrompt)
	if len(actions) != 2 {
		t.Fatalf("expected https/http HEAD probes for web-port fallback, got %#v", actions)
	}
	for _, action := range actions {
		if action.Name != "http.request" || action.Arguments["method"] != http.MethodHead {
			t.Fatalf("unexpected web-port fallback action: %#v", action)
		}
	}
}

func TestSafePortScanDetectsLocalOpenPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	results := agentScanTCPPorts(context.Background(), "127.0.0.1", []int{port}, 300*time.Millisecond)
	if len(results) != 1 {
		t.Fatalf("results = %#v", results)
	}
	if !results[0].Open || results[0].Port != port {
		t.Fatalf("expected local port %d to be open, got %#v", port, results[0])
	}

	prompt := fmt.Sprintf("扫描 127.0.0.1 开放端口 ports=%d", port)
	if !agentIsPortScanIntent(prompt) {
		t.Fatalf("expected IP host port_scan intent")
	}
	actions := agentPortScanActionsFromPrompt(prompt)
	if len(actions) != 1 || actions[0].Name != "ports.scan.safe" {
		t.Fatalf("unexpected port scan action: %#v", actions)
	}
}

func TestSafePortScanRejectsTooBroadRange(t *testing.T) {
	if _, err := parseAgentPortSpec("1-200", 128); err == nil {
		t.Fatalf("expected broad range to be rejected by safe cap")
	}
	plan := agentFallbackPlannerFromPrompt("扫描 example.com 开放端口 ports=1-200", "", "zh")
	if plan == nil || plan.Task == nil || plan.Task.Status != "blocked" || len(agentPlannerActions(plan)) != 0 {
		t.Fatalf("expected blocked plan for too broad range: %#v", plan)
	}
}

func TestAgentSkillRegistryExposesToolsAndSkills(t *testing.T) {
	registry := agentSkillRegistry("zh")
	if len(registry.Skills) == 0 {
		t.Fatalf("expected builtin skills")
	}
	if len(registry.ToolCapabilities) == 0 {
		t.Fatalf("expected tool capabilities")
	}
	seenSkill := map[string]bool{}
	for _, skill := range registry.Skills {
		seenSkill[skill.ID] = true
	}
	for _, id := range []string{"port_scan", "content_discovery", "js_route_analysis", "api_auth_test"} {
		if !seenSkill[id] {
			t.Fatalf("missing skill %s", id)
		}
	}
	seenTool := map[string]agentToolCapabilitySpec{}
	for _, tool := range registry.ToolCapabilities {
		seenTool[tool.Name] = tool
	}
	portTool, ok := seenTool["ports.scan.safe"]
	if !ok {
		t.Fatalf("missing ports.scan.safe capability")
	}
	if portTool.RequiredLevel != "safe_scan" || len(portTool.CannotDo) == 0 {
		t.Fatalf("unexpected ports.scan.safe capability: %#v", portTool)
	}
	prompt := agentCapabilityRegistryPrompt("zh")
	if !strings.Contains(prompt, "Skill / Tool Capability Registry") || !strings.Contains(prompt, "ports.scan.safe") {
		t.Fatalf("registry prompt missing expected content: %s", prompt)
	}
}

func TestAgentMCPRegistryMapsDeclaredToolsToCapabilities(t *testing.T) {
	manager := newAgentManager(filepath.Join(t.TempDir(), "runs"))
	initial, err := manager.ListMCPRegistry(context.Background())
	if err != nil {
		t.Fatalf("initial ListMCPRegistry() error = %v", err)
	}
	if initial.Count < 5 || initial.EnabledCount != 0 || len(initial.ToolCapabilities) != 0 {
		t.Fatalf("expected disabled MCP presets only, got %#v", initial)
	}
	presetIDs := map[string]bool{}
	for _, server := range initial.Servers {
		if server.Preset {
			presetIDs[server.ID] = true
		}
	}
	for _, id := range []string{"preset-nmap", "preset-ffuf", "preset-nuclei", "preset-semgrep", "preset-trivy"} {
		if !presetIDs[id] {
			t.Fatalf("missing MCP preset %s", id)
		}
	}

	server, err := manager.AddMCPServer(context.Background(), agentMCPServerConfig{
		Name:      "local-ffuf",
		Transport: "stdio",
		Command:   "ffuf",
		Enabled:   true,
		Tools: []agentMCPToolConfig{{
			Name:          "content.discovery",
			Description:   "Run bounded content discovery",
			IntentTags:    []string{"content_discovery", "directory_enum"},
			CanDo:         []string{"discover hidden paths with a wordlist"},
			CannotDo:      []string{"port_scan"},
			Risk:          "medium",
			RequiredLevel: "safe_scan",
			Enabled:       true,
		}},
	})
	if err != nil {
		t.Fatalf("AddMCPServer() error = %v", err)
	}
	if server.ID == "" {
		t.Fatalf("server ID should be generated")
	}
	view, err := manager.ListMCPRegistry(context.Background())
	if err != nil {
		t.Fatalf("ListMCPRegistry() error = %v", err)
	}
	if view.Count < 6 || view.EnabledCount != 1 || len(view.ToolCapabilities) != 1 {
		t.Fatalf("unexpected MCP view: %#v", view)
	}
	capability := view.ToolCapabilities[0]
	if capability.Source != "mcp:"+server.ID || capability.RequiredLevel != "safe_scan" {
		t.Fatalf("unexpected MCP capability: %#v", capability)
	}
	prompt := manager.agentMCPRegistryPrompt("zh")
	if !strings.Contains(prompt, "mcp__"+server.ID+"__content.discovery") || !strings.Contains(prompt, "content_discovery") {
		t.Fatalf("MCP prompt missing capability: %s", prompt)
	}
	discovery, err := manager.DiscoverMCPTools(context.Background(), server.ID)
	if err != nil {
		t.Fatalf("DiscoverMCPTools() error = %v", err)
	}
	if discovery.Status != "ok" || len(discovery.ToolCapabilities) != 1 {
		t.Fatalf("unexpected discovery response: %#v", discovery)
	}
}

func TestHTTPAuthEnvelopeHealthAndVersion(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("test-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	server, err := NewServer(Config{
		RunsDir:   t.TempDir(),
		TokenFile: tokenFile,
		Build: services.BuildInfo{
			Name:    "CyberAgent",
			Version: "vdaemon-test",
			Commit:  "abc123",
			Date:    "2026-05-24T00:00:00Z",
			Module:  "github.com/H1d3rOne/cyberagent",
		},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer server.Close()

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	noAuth := doDaemonRequest(t, httpServer.URL+"/api/v1/version", "")
	if noAuth.StatusCode != http.StatusUnauthorized || noAuth.Body.OK || noAuth.Body.Error.Code != services.CodeUnauthorized {
		t.Fatalf("unexpected no-auth response: status=%d body=%#v", noAuth.StatusCode, noAuth.Body)
	}

	badAuth := doDaemonRequest(t, httpServer.URL+"/api/v1/version", "wrong-token")
	if badAuth.StatusCode != http.StatusUnauthorized || badAuth.Body.Error.Code != services.CodeUnauthorized {
		t.Fatalf("unexpected bad-auth response: status=%d body=%#v", badAuth.StatusCode, badAuth.Body)
	}

	version := doDaemonRequest(t, httpServer.URL+"/api/v1/version", "test-token")
	if version.StatusCode != http.StatusOK || !version.Body.OK || version.Body.Error != nil {
		t.Fatalf("unexpected version response: status=%d body=%#v", version.StatusCode, version.Body)
	}
	if version.Body.Data["version"] != "vdaemon-test" || version.Body.Data["commit"] != "abc123" {
		t.Fatalf("unexpected version data: %#v", version.Body.Data)
	}

	health := doDaemonRequest(t, httpServer.URL+"/api/v1/healthz", "test-token")
	if health.StatusCode != http.StatusOK || !health.Body.OK || health.Body.Data["status"] != "ok" {
		t.Fatalf("unexpected health response: status=%d body=%#v", health.StatusCode, health.Body)
	}

	missing := doDaemonRequest(t, httpServer.URL+"/api/v1/missing", "test-token")
	if missing.StatusCode != http.StatusNotFound || missing.Body.OK || missing.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("unexpected missing response: status=%d body=%#v", missing.StatusCode, missing.Body)
	}
}

func TestHTTPCORSPreflightBypassesAuthForLoopbackOrigin(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("test-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	server, err := NewServer(Config{RunsDir: t.TempDir(), TokenFile: tokenFile})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer server.Close()

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	req, err := http.NewRequest(http.MethodOptions, httpServer.URL+"/api/v1/healthz", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "authorization")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "authorization") {
		t.Fatalf("allow headers = %q, want authorization", got)
	}
}

func TestHTTPReadOnlyAPIs(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><head><script src="/app.js"></script></head><body><a href="/admin">Admin</a></body></html>`))
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch("/api/status")`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte("<urlset></urlset>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	runID := "run_readonly"
	if _, err := coreruntime.RunWebScan(context.Background(), coreruntime.WebScanRequest{
		Target:          target.URL,
		Scope:           target.URL,
		Out:             filepath.Join(runsDir, runID),
		Profile:         "safe",
		NoExternalTools: true,
		BuildInfo:       coreruntime.BuildInfo{Version: "v0.1.2-test", Commit: "abc123", Date: "2026-05-24T00:00:00Z", Module: "github.com/H1d3rOne/cyberagent"},
		CommandName:     "scan web",
		CommandArgs:     []string{"scan", "web", "--target", target.URL, "--scope", target.URL, "--no-external-tools"},
	}); err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}

	pluginRoot := filepath.Join(root, "plugins")
	writeDaemonTestPlugin(t, pluginRoot)

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vread-api"}),
		plugins:   services.NewPluginService(pluginRoot),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	doctor := doDaemonRequest(t, api.URL+"/api/v1/doctor", "test-token")
	if doctor.StatusCode != http.StatusOK || !doctor.Body.OK || doctor.Body.Data["checks"] == nil {
		t.Fatalf("unexpected doctor response: status=%d body=%#v", doctor.StatusCode, doctor.Body)
	}

	plugins := doDaemonRequest(t, api.URL+"/api/v1/plugins", "test-token")
	if plugins.StatusCode != http.StatusOK || !plugins.Body.OK || len(asList(t, plugins.Body.Data["plugins"])) != 1 {
		t.Fatalf("unexpected plugins response: status=%d body=%#v", plugins.StatusCode, plugins.Body)
	}
	pluginDoctor := doDaemonRequest(t, api.URL+"/api/v1/plugins/doctor", "test-token")
	if pluginDoctor.StatusCode != http.StatusOK || !pluginDoctor.Body.OK || pluginDoctor.Body.Data["count"].(float64) != 1 {
		t.Fatalf("unexpected plugins doctor response: status=%d body=%#v", pluginDoctor.StatusCode, pluginDoctor.Body)
	}

	runs := doDaemonRequest(t, api.URL+"/api/v1/runs?limit=10", "test-token")
	if runs.StatusCode != http.StatusOK || !runs.Body.OK {
		t.Fatalf("unexpected runs response: status=%d body=%#v", runs.StatusCode, runs.Body)
	}
	items := asList(t, runs.Body.Data["items"])
	if len(items) != 1 || asMap(t, items[0])["run_id"] != runID {
		t.Fatalf("unexpected runs items: %#v", items)
	}

	detail := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID, "test-token")
	if detail.StatusCode != http.StatusOK || !detail.Body.OK || detail.Body.Data["run_id"] != runID {
		t.Fatalf("unexpected run detail response: status=%d body=%#v", detail.StatusCode, detail.Body)
	}

	reportJSON := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/report?format=json", "test-token")
	if reportJSON.StatusCode != http.StatusOK || !reportJSON.Body.OK || reportJSON.Body.Data["format"] != "json" || reportJSON.Body.Data["report"] == nil {
		t.Fatalf("unexpected JSON report response: status=%d body=%#v", reportJSON.StatusCode, reportJSON.Body)
	}

	reportMD := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/report?format=md", "test-token")
	if reportMD.StatusCode != http.StatusOK || !reportMD.Body.OK || reportMD.Body.Data["format"] != "md" || !strings.Contains(reportMD.Body.Data["content"].(string), "CyberAgent Report") {
		t.Fatalf("unexpected markdown report response: status=%d body=%#v", reportMD.StatusCode, reportMD.Body)
	}

	artifacts := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/artifacts", "test-token")
	if artifacts.StatusCode != http.StatusOK || !artifacts.Body.OK || artifacts.Body.Data["count"].(float64) <= 0 {
		t.Fatalf("unexpected artifacts response: status=%d body=%#v", artifacts.StatusCode, artifacts.Body)
	}

	events := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/events?limit=2", "test-token")
	if events.StatusCode != http.StatusOK || !events.Body.OK || len(asList(t, events.Body.Data["items"])) != 2 || events.Body.Data["next_cursor"] == "" {
		t.Fatalf("unexpected events response: status=%d body=%#v", events.StatusCode, events.Body)
	}

	approvals := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/approvals", "test-token")
	if approvals.StatusCode != http.StatusOK || !approvals.Body.OK || len(asList(t, approvals.Body.Data["approvals"])) != 0 {
		t.Fatalf("unexpected approvals response: status=%d body=%#v", approvals.StatusCode, approvals.Body)
	}

	traversal := doDaemonRequest(t, api.URL+"/api/v1/runs/../secret", "test-token")
	if traversal.StatusCode != http.StatusNotFound && traversal.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected traversal response: status=%d body=%#v", traversal.StatusCode, traversal.Body)
	}

	badLimit := doDaemonRequest(t, api.URL+"/api/v1/runs?limit=bad", "test-token")
	if badLimit.StatusCode != http.StatusBadRequest || badLimit.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected bad limit response: status=%d body=%#v", badLimit.StatusCode, badLimit.Body)
	}
}

func TestHTTPAgentSessionAndMessageAPIs(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><head><script src="/app.js"></script></head><body><a href="/admin">Admin</a></body></html>`))
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch("/api/status")`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte("<urlset></urlset>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-api"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-api"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	session := asMap(t, create.Body.Data["session"])
	sessionID := stringValue(t, session["session_id"])
	if sessionID == "" || session["operator"] != "tester" || session["language"] != "zh" {
		t.Fatalf("unexpected agent session: %#v", session)
	}
	welcome := asList(t, session["messages"])
	if len(welcome) != 1 || asMap(t, welcome[0])["role"] != "agent" {
		t.Fatalf("unexpected welcome messages: %#v", welcome)
	}

	read := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if read.StatusCode != http.StatusOK || !read.Body.OK {
		t.Fatalf("unexpected get agent session response: status=%d body=%#v", read.StatusCode, read.Body)
	}
	readSession := asMap(t, read.Body.Data["session"])
	if readSession["session_id"] != sessionID {
		t.Fatalf("unexpected read session: %#v", readSession)
	}

	send := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"扫描 `+target.URL+` scope=`+target.URL+`"}`,
	)
	if send.StatusCode != http.StatusOK || !send.Body.OK || send.Body.Data["session_id"] != sessionID {
		t.Fatalf("unexpected send agent message response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	messages := asList(t, send.Body.Data["messages"])
	if len(messages) < 3 || asMap(t, messages[0])["role"] != "user" {
		t.Fatalf("unexpected agent messages: %#v", messages)
	}
	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) != 1 {
		t.Fatalf("unexpected tool calls: %#v", toolCalls)
	}
	tool := asMap(t, toolCalls[0])
	if tool["name"] != "POST /api/v1/scans/web" || tool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected scan tool call: %#v", tool)
	}
	result := asMap(t, tool["result"])
	jobID := stringValue(t, result["job_id"])
	runID := stringValue(t, result["run_id"])
	if jobID == "" || runID == "" || result["run_url"] != "/api/v1/runs/"+runID {
		t.Fatalf("unexpected scan tool result: %#v", result)
	}

	job := waitDaemonJobStatus(t, api.URL, "test-token", jobID, 8*time.Second, contracts.StatusSucceeded)
	if job["run_id"] != runID || job["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected terminal agent-started job: %#v", job)
	}

	contextAnalysis := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"分析运行 `+runID+` 在 `+target.URL+` 上的这个 finding：title=\"Missing X-Content-Type-Options Header\"。请解释影响、关键证据、误报检查、修复建议。"}`,
	)
	if contextAnalysis.StatusCode != http.StatusOK || !contextAnalysis.Body.OK {
		t.Fatalf("unexpected context analysis response: status=%d body=%#v", contextAnalysis.StatusCode, contextAnalysis.Body)
	}
	contextToolCalls := asList(t, contextAnalysis.Body.Data["tool_calls"])
	if len(contextToolCalls) != 1 {
		t.Fatalf("unexpected context tool calls: %#v", contextToolCalls)
	}
	contextTool := asMap(t, contextToolCalls[0])
	if contextTool["name"] != "GET /api/v1/runs/{run_id}/context" || contextTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("context prompt should analyze run context instead of starting a scan: %#v", contextTool)
	}
	contextMessages := asList(t, contextAnalysis.Body.Data["messages"])
	if !strings.Contains(stringValue(t, asMap(t, contextMessages[len(contextMessages)-1])["content"]), "误报") {
		t.Fatalf("context analysis did not include finding guidance: %#v", contextMessages)
	}

	state := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID+"/state", "test-token")
	if state.StatusCode != http.StatusOK || !state.Body.OK {
		t.Fatalf("unexpected agent state alias response: status=%d body=%#v", state.StatusCode, state.Body)
	}
	if state.Body.Data["session_id"] != sessionID || state.Body.Data["status"] == "" {
		t.Fatalf("agent state alias missing session metadata: %#v", state.Body.Data)
	}
	stateSession := asMap(t, state.Body.Data["session"])
	if stateSession["session_id"] != sessionID {
		t.Fatalf("agent state alias should include full session: %#v", stateSession)
	}
	stateTask := asMap(t, state.Body.Data["active_task"])
	if stateTask["active_run_id"] != runID {
		t.Fatalf("agent state alias should expose active run context: %#v", stateTask)
	}
	stateRefs := asList(t, state.Body.Data["context_refs"])
	if !listOfMapsContains(t, stateRefs, "id", "run:"+runID) {
		t.Fatalf("agent state alias should expose context refs: %#v", stateRefs)
	}
	stateRunSummary := asMap(t, state.Body.Data["run_summary"])
	if stateRunSummary["run_id"] != runID {
		t.Fatalf("agent state alias should expose run summary: %#v", stateRunSummary)
	}

	contextAlias := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID+"/context", "test-token")
	if contextAlias.StatusCode != http.StatusOK || !contextAlias.Body.OK {
		t.Fatalf("unexpected agent context alias response: status=%d body=%#v", contextAlias.StatusCode, contextAlias.Body)
	}
	if contextAlias.Body.Data["session_id"] != sessionID {
		t.Fatalf("agent context alias missing session id: %#v", contextAlias.Body.Data)
	}
	contextRefs := asList(t, contextAlias.Body.Data["context_refs"])
	if !listOfMapsContains(t, contextRefs, "id", "run:"+runID) {
		t.Fatalf("agent context alias should expose context refs: %#v", contextRefs)
	}
	if _, ok := contextAlias.Body.Data["messages"]; ok {
		t.Fatalf("agent context alias should not duplicate full chat history: %#v", contextAlias.Body.Data)
	}

	trace := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID+"/trace", "test-token")
	if trace.StatusCode != http.StatusOK || !trace.Body.OK {
		t.Fatalf("unexpected agent trace response: status=%d body=%#v", trace.StatusCode, trace.Body)
	}
	if trace.Body.Data["trace_version"] != "agent.trace.v1" || trace.Body.Data["session_id"] != sessionID {
		t.Fatalf("agent trace missing metadata: %#v", trace.Body.Data)
	}
	traceMessages := asList(t, trace.Body.Data["messages"])
	traceTools := asList(t, trace.Body.Data["tool_calls"])
	traceTimeline := asList(t, trace.Body.Data["timeline"])
	if len(traceMessages) == 0 || len(traceTools) == 0 || len(traceTimeline) < len(traceMessages)+len(traceTools) {
		t.Fatalf("agent trace should include messages, tool calls, and combined timeline: messages=%#v tools=%#v timeline=%#v", traceMessages, traceTools, traceTimeline)
	}
	traceSummary := asMap(t, trace.Body.Data["summary"])
	if int(traceSummary["message_count"].(float64)) != len(traceMessages) || int(traceSummary["tool_call_count"].(float64)) != len(traceTools) {
		t.Fatalf("agent trace summary counts mismatch: %#v", traceSummary)
	}
	findTraceTool := func(name string) map[string]any {
		t.Helper()
		for _, raw := range traceTimeline {
			item := asMap(t, raw)
			if itemName, _ := item["name"].(string); itemName == name {
				return item
			}
		}
		t.Fatalf("missing trace tool item name=%q in %#v", name, traceTimeline)
		return nil
	}
	scanTraceItem := findTraceTool("POST /api/v1/scans/web")
	if scanTraceItem["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("scan trace item should expose registry route source: %#v", scanTraceItem)
	}
	contextTraceItem := findTraceTool("GET /api/v1/runs/{run_id}/context")
	if contextTraceItem["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("context trace item should expose registry route source: %#v", contextTraceItem)
	}

	continueControl := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/control",
		"test-token",
		`{"operator":"tester","language":"zh","action":"continue"}`,
	)
	if continueControl.StatusCode != http.StatusOK || !continueControl.Body.OK {
		t.Fatalf("unexpected continue control response: status=%d body=%#v", continueControl.StatusCode, continueControl.Body)
	}
	continueSession := asMap(t, continueControl.Body.Data["session"])
	if continueSession["status"] != "running" {
		t.Fatalf("continue control did not keep session running: %#v", continueSession)
	}
	continueNextAction := asMap(t, continueSession["next_action"])
	if continueNextAction["type"] != "continue" {
		t.Fatalf("continue control did not set next action: %#v", continueNextAction)
	}
	continueTodos := asList(t, continueSession["todos"])
	if len(continueTodos) == 0 {
		t.Fatalf("continue control lost todos: %#v", continueSession)
	}

	continueAlias := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/continue",
		"test-token",
		`{"operator":"tester","language":"zh","action":"stop","prompt":"alias continue prompt"}`,
	)
	if continueAlias.StatusCode != http.StatusOK || !continueAlias.Body.OK {
		t.Fatalf("unexpected continue alias response: status=%d body=%#v", continueAlias.StatusCode, continueAlias.Body)
	}
	continueAliasSession := asMap(t, continueAlias.Body.Data["session"])
	continueAliasNext := asMap(t, continueAliasSession["next_action"])
	if continueAliasSession["status"] != "running" || continueAliasNext["type"] != "continue" || continueAliasNext["suggested_prompt"] != "alias continue prompt" {
		t.Fatalf("continue alias should force continue action from path: session=%#v next=%#v", continueAliasSession, continueAliasNext)
	}

	pauseAlias := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/pause",
		"test-token",
		`{"operator":"tester","language":"zh","action":"continue","reason":"operator pause without cancel"}`,
	)
	if pauseAlias.StatusCode != http.StatusOK || !pauseAlias.Body.OK {
		t.Fatalf("unexpected pause alias response: status=%d body=%#v", pauseAlias.StatusCode, pauseAlias.Body)
	}
	pauseSession := asMap(t, pauseAlias.Body.Data["session"])
	pauseNext := asMap(t, pauseSession["next_action"])
	if pauseSession["status"] != "paused" || pauseNext["type"] != "continue" || pauseNext["reason"] != "operator pause without cancel" {
		t.Fatalf("pause alias should pause but keep resumable next action: session=%#v next=%#v", pauseSession, pauseNext)
	}

	resumeAlias := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/resume",
		"test-token",
		`{"operator":"tester","language":"zh","action":"pause","prompt":"resume prompt"}`,
	)
	if resumeAlias.StatusCode != http.StatusOK || !resumeAlias.Body.OK {
		t.Fatalf("unexpected resume alias response: status=%d body=%#v", resumeAlias.StatusCode, resumeAlias.Body)
	}
	resumeSession := asMap(t, resumeAlias.Body.Data["session"])
	resumeNext := asMap(t, resumeSession["next_action"])
	if resumeSession["status"] != "running" || resumeNext["type"] != "continue" || resumeNext["suggested_prompt"] != "resume prompt" {
		t.Fatalf("resume alias should force continue action from path: session=%#v next=%#v", resumeSession, resumeNext)
	}

	revisePrompt := "修订计划：优先分析认证与越权风险，暂缓低风险安全头说明"
	reviseControl := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/control",
		"test-token",
		`{"operator":"tester","language":"zh","action":"revise","prompt":"`+revisePrompt+`"}`,
	)
	if reviseControl.StatusCode != http.StatusOK || !reviseControl.Body.OK {
		t.Fatalf("unexpected revise control response: status=%d body=%#v", reviseControl.StatusCode, reviseControl.Body)
	}
	reviseSession := asMap(t, reviseControl.Body.Data["session"])
	if reviseSession["status"] != "running" {
		t.Fatalf("revise control did not keep session running: %#v", reviseSession)
	}
	reviseTask := asMap(t, reviseSession["active_task"])
	if !strings.Contains(stringValue(t, reviseTask["summary"]), "认证与越权风险") {
		t.Fatalf("revise control did not update task summary: %#v", reviseTask)
	}
	reviseNextAction := asMap(t, reviseSession["next_action"])
	if reviseNextAction["type"] != "continue" || reviseNextAction["suggested_prompt"] != revisePrompt {
		t.Fatalf("revise control did not set next action: %#v", reviseNextAction)
	}
	reviseMessages := asList(t, reviseSession["messages"])
	if !strings.Contains(stringValue(t, asMap(t, reviseMessages[len(reviseMessages)-1])["content"]), "修订") {
		t.Fatalf("revise control did not append revise message: %#v", reviseMessages[len(reviseMessages)-1])
	}

	stopControl := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/stop",
		"test-token",
		`{"operator":"tester","language":"zh","action":"continue","job_id":"`+jobID+`","run_id":"`+runID+`","reason":"operator pause"}`,
	)
	if stopControl.StatusCode != http.StatusOK || !stopControl.Body.OK {
		t.Fatalf("unexpected stop alias response: status=%d body=%#v", stopControl.StatusCode, stopControl.Body)
	}
	stopSession := asMap(t, stopControl.Body.Data["session"])
	if stopSession["status"] != "blocked" {
		t.Fatalf("stop alias did not block session: %#v", stopSession)
	}
	stopNextAction := asMap(t, stopSession["next_action"])
	if stopNextAction["type"] != "blocked" || stopNextAction["reason"] != "operator pause" {
		t.Fatalf("stop alias did not set blocked next action: %#v", stopNextAction)
	}

	cancelAlias := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/cancel",
		"test-token",
		`{"operator":"tester","language":"zh","action":"continue","reason":"operator cancel"}`,
	)
	if cancelAlias.StatusCode != http.StatusOK || !cancelAlias.Body.OK {
		t.Fatalf("unexpected cancel alias response: status=%d body=%#v", cancelAlias.StatusCode, cancelAlias.Body)
	}
	cancelSession := asMap(t, cancelAlias.Body.Data["session"])
	cancelNext := asMap(t, cancelSession["next_action"])
	if cancelSession["status"] != "cancelled" || cancelNext["type"] != "wait_user" || cancelNext["reason"] != "operator cancel" {
		t.Fatalf("cancel alias should mark session cancelled and wait for next task: session=%#v next=%#v", cancelSession, cancelNext)
	}

	newTaskControl := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/new-task",
		"test-token",
		`{"operator":"tester","language":"zh","action":"continue","clear_input":true}`,
	)
	if newTaskControl.StatusCode != http.StatusOK || !newTaskControl.Body.OK {
		t.Fatalf("unexpected new-task alias response: status=%d body=%#v", newTaskControl.StatusCode, newTaskControl.Body)
	}
	newTaskSession := asMap(t, newTaskControl.Body.Data["session"])
	if newTaskSession["status"] != "idle" {
		t.Fatalf("new-task alias did not reset session status: %#v", newTaskSession)
	}
	if _, ok := newTaskSession["active_task"]; ok {
		t.Fatalf("new-task alias did not clear active task: %#v", newTaskSession["active_task"])
	}
	if todosValue, ok := newTaskSession["todos"]; ok {
		if todos := asList(t, todosValue); len(todos) != 0 {
			t.Fatalf("new-task alias did not clear todos: %#v", todos)
		}
	}
	if toolCallsValue, ok := newTaskSession["tool_calls"]; ok {
		if toolCalls := asList(t, toolCallsValue); len(toolCalls) != 0 {
			t.Fatalf("new-task alias did not clear tool calls: %#v", toolCalls)
		}
	}
	newTaskMessages := asList(t, newTaskSession["messages"])
	if len(newTaskMessages) != 1 || asMap(t, newTaskMessages[0])["role"] != "agent" {
		t.Fatalf("new-task alias clear_input should keep only reset message: %#v", newTaskMessages)
	}
	newTaskNextAction := asMap(t, newTaskSession["next_action"])
	if newTaskNextAction["type"] != "wait_user" {
		t.Fatalf("new-task alias did not wait for user: %#v", newTaskNextAction)
	}

	updated := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if updated.StatusCode != http.StatusOK || !updated.Body.OK {
		t.Fatalf("unexpected updated agent session response: status=%d body=%#v", updated.StatusCode, updated.Body)
	}
	updatedMessages := asList(t, asMap(t, updated.Body.Data["session"])["messages"])
	if len(updatedMessages) != 1 {
		t.Fatalf("updated session did not persist conversation: %#v", updatedMessages)
	}

	list := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions?limit=10", "test-token")
	if list.StatusCode != http.StatusOK || !list.Body.OK {
		t.Fatalf("unexpected list agent sessions response: status=%d body=%#v", list.StatusCode, list.Body)
	}
	items := asList(t, list.Body.Data["items"])
	if len(items) != 1 || list.Body.Data["count"].(float64) != 1 || list.Body.Data["limit"].(float64) != 10 {
		t.Fatalf("unexpected agent sessions list envelope: %#v", list.Body.Data)
	}
	item := asMap(t, items[0])
	if item["session_id"] != sessionID || item["status"] != "idle" || item["operator"] != "tester" {
		t.Fatalf("unexpected agent session summary: %#v", item)
	}
	if item["message_count"].(float64) != 1 || stringValue(t, item["last_message"]) == "" {
		t.Fatalf("agent session summary missing counters or last message: %#v", item)
	}

	badListLimit := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions?limit=bad", "test-token")
	if badListLimit.StatusCode != http.StatusBadRequest || badListLimit.Body.OK || badListLimit.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected bad agent list limit response: status=%d body=%#v", badListLimit.StatusCode, badListLimit.Body)
	}

	missing := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/missing/messages", "test-token", `{"message":"status"}`)
	if missing.StatusCode != http.StatusNotFound || missing.Body.OK || missing.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("unexpected missing session response: status=%d body=%#v", missing.StatusCode, missing.Body)
	}
}

func TestHTTPAgentHealthSeparatesOptionalMissingTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-health"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	send := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"健康检查 daemon 状态"}`,
	)
	if send.StatusCode != http.StatusOK || !send.Body.OK {
		t.Fatalf("unexpected health agent response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	messages := asList(t, send.Body.Data["messages"])
	if len(messages) < 2 {
		t.Fatalf("expected user and health message: %#v", messages)
	}
	content := stringValue(t, asMap(t, messages[len(messages)-1])["content"])
	if !strings.Contains(content, "必需检查通过") || !strings.Contains(content, "可选工具缺失") {
		t.Fatalf("health message should distinguish required checks from optional missing tools: %q", content)
	}
	if strings.Contains(content, "异常 4") || strings.Contains(content, "异常 3") {
		t.Fatalf("health message should not count optional missing tools as exceptions: %q", content)
	}

	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) != 1 {
		t.Fatalf("unexpected health tool calls: %#v", toolCalls)
	}
	tool := asMap(t, toolCalls[0])
	if tool["name"] != "GET /api/v1/doctor" || tool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected health tool call: %#v", tool)
	}
	result := asMap(t, tool["result"])
	if result["version"] != "vagent-health" || int(result["checks"].(float64)) == 0 {
		t.Fatalf("unexpected health result basics: %#v", result)
	}
	if int(result["required_issues"].(float64)) != 0 {
		t.Fatalf("optional missing tools should not be required issues: %#v", result)
	}
	if int(result["optional_missing"].(float64)) == 0 {
		t.Fatalf("expected optional missing tools in isolated PATH: %#v", result)
	}
	if int(result["warnings"].(float64)) != 0 {
		t.Fatalf("unexpected warning count for isolated PATH doctor run: %#v", result)
	}

	if send.Body.Data["active_task"] == nil {
		t.Fatalf("health check should keep an active task snapshot: %#v", send.Body.Data)
	}
	activeTask := asMap(t, send.Body.Data["active_task"])
	if activeTask["kind"] != "health" || activeTask["status"] != "done" {
		t.Fatalf("health check should finish its local task: %#v", activeTask)
	}
	if send.Body.Data["next_action"] == nil {
		t.Fatalf("health check should return a terminal next action: %#v", send.Body.Data)
	}
	nextAction := asMap(t, send.Body.Data["next_action"])
	if nextAction["type"] != "finished" {
		t.Fatalf("health check should not leave the agent in continue state: %#v", nextAction)
	}
	todos := asList(t, send.Body.Data["todos"])
	if len(todos) == 0 {
		t.Fatalf("health check should expose completed runbook todos: %#v", send.Body.Data)
	}
	for _, raw := range todos {
		todo := asMap(t, raw)
		if todo["status"] != "done" {
			t.Fatalf("health check should mark local-task todos done: %#v", todos)
		}
	}

	persisted := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if persisted.StatusCode != http.StatusOK || !persisted.Body.OK {
		t.Fatalf("unexpected persisted health session response: status=%d body=%#v", persisted.StatusCode, persisted.Body)
	}
	persistedSession := asMap(t, persisted.Body.Data["session"])
	if persistedSession["status"] != "idle" {
		t.Fatalf("persisted health session should be idle after one-shot tool: %#v", persistedSession)
	}
	persistedTask := asMap(t, persistedSession["active_task"])
	if persistedTask["status"] != "done" {
		t.Fatalf("persisted health task should be done: %#v", persistedTask)
	}
}

func TestHTTPAgentNaturalLanguageControlIntents(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-chat-control"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-chat-control"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	sendHealth := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"健康检查 daemon 状态"}`,
	)
	if sendHealth.StatusCode != http.StatusOK || !sendHealth.Body.OK {
		t.Fatalf("unexpected health message response: status=%d body=%#v", sendHealth.StatusCode, sendHealth.Body)
	}
	if next := asMap(t, sendHealth.Body.Data["next_action"]); next["type"] != "finished" {
		t.Fatalf("health should leave a finished local task before control intents: %#v", next)
	}

	continueMsg := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"继续：检查最近证据并给出下一步"}`,
	)
	if continueMsg.StatusCode != http.StatusOK || !continueMsg.Body.OK {
		t.Fatalf("unexpected natural-language continue response: status=%d body=%#v", continueMsg.StatusCode, continueMsg.Body)
	}
	continueMessages := asList(t, continueMsg.Body.Data["messages"])
	if len(continueMessages) != 2 || asMap(t, continueMessages[0])["role"] != "user" || asMap(t, continueMessages[1])["role"] != "agent" {
		t.Fatalf("natural-language continue should return user + control agent message: %#v", continueMessages)
	}
	continueNext := asMap(t, continueMsg.Body.Data["next_action"])
	if continueNext["type"] != "continue" || continueNext["suggested_prompt"] != "检查最近证据并给出下一步" {
		t.Fatalf("natural-language continue should set suggested prompt: %#v", continueNext)
	}
	if rawToolCalls, ok := continueMsg.Body.Data["tool_calls"]; ok && rawToolCalls != nil {
		if len(asList(t, rawToolCalls)) != 0 {
			t.Fatalf("natural-language continue should not call tools directly: %#v", rawToolCalls)
		}
	}

	reviseMsg := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"修订计划：优先分析认证和越权风险"}`,
	)
	if reviseMsg.StatusCode != http.StatusOK || !reviseMsg.Body.OK {
		t.Fatalf("unexpected natural-language revise response: status=%d body=%#v", reviseMsg.StatusCode, reviseMsg.Body)
	}
	reviseTask := asMap(t, reviseMsg.Body.Data["active_task"])
	if !strings.Contains(stringValue(t, reviseTask["summary"]), "认证和越权风险") {
		t.Fatalf("natural-language revise should update active task summary: %#v", reviseTask)
	}
	reviseNext := asMap(t, reviseMsg.Body.Data["next_action"])
	if reviseNext["type"] != "continue" || reviseNext["suggested_prompt"] != "优先分析认证和越权风险" {
		t.Fatalf("natural-language revise should set revised prompt: %#v", reviseNext)
	}

	stopMsg := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"停止：等待我确认授权范围"}`,
	)
	if stopMsg.StatusCode != http.StatusOK || !stopMsg.Body.OK {
		t.Fatalf("unexpected natural-language stop response: status=%d body=%#v", stopMsg.StatusCode, stopMsg.Body)
	}
	if stopMsg.Body.Data["active_task"] != nil {
		stopTask := asMap(t, stopMsg.Body.Data["active_task"])
		if stopTask["status"] != "blocked" {
			t.Fatalf("natural-language stop should block active task: %#v", stopTask)
		}
	}
	stopNext := asMap(t, stopMsg.Body.Data["next_action"])
	if stopNext["type"] != "blocked" || stopNext["reason"] != "等待我确认授权范围" {
		t.Fatalf("natural-language stop should use prompt remainder as reason: %#v", stopNext)
	}

	newTaskMsg := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"新任务：清空任务，重新开始"}`,
	)
	if newTaskMsg.StatusCode != http.StatusOK || !newTaskMsg.Body.OK {
		t.Fatalf("unexpected natural-language new-task response: status=%d body=%#v", newTaskMsg.StatusCode, newTaskMsg.Body)
	}
	if newTaskMsg.Body.Data["active_task"] != nil {
		t.Fatalf("natural-language new-task should clear active task: %#v", newTaskMsg.Body.Data["active_task"])
	}
	newTaskNext := asMap(t, newTaskMsg.Body.Data["next_action"])
	if newTaskNext["type"] != "wait_user" {
		t.Fatalf("natural-language new-task should wait for next user task: %#v", newTaskNext)
	}
	newTaskSessionRead := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if newTaskSessionRead.StatusCode != http.StatusOK || !newTaskSessionRead.Body.OK {
		t.Fatalf("unexpected natural-language new-task session read: status=%d body=%#v", newTaskSessionRead.StatusCode, newTaskSessionRead.Body)
	}
	newTaskSession := asMap(t, newTaskSessionRead.Body.Data["session"])
	newTaskMessages := asList(t, newTaskSession["messages"])
	if len(newTaskMessages) != 1 || asMap(t, newTaskMessages[0])["role"] != "agent" {
		t.Fatalf("natural-language new-task clear request should reset visible conversation: %#v", newTaskMessages)
	}
}

func TestHTTPAgentNaturalLanguageContinueExecutesAutoContinuePrompt(t *testing.T) {
	var llmRequests atomic.Int32
	plannerContent := `{
		"answer":"已建立一个低风险健康检查任务，下一步可以自动检查 daemon 状态。",
		"task":{
			"goal":"检查 daemon 健康状态",
			"kind":"ops",
			"status":"running",
			"summary":"使用低风险内置工具检查 daemon 健康状态。"
		},
		"todos":[
			{"title":"检查 daemon 健康状态","status":"running","priority":1}
		],
		"next_action":{
			"type":"continue",
			"reason":"下一步是低风险只读健康检查。",
			"suggested_prompt":"继续检查 daemon 健康状态",
			"auto_continue":true
		}
	}`
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmRequests.Add(1)
		encodedContent, err := json.Marshal(plannerContent)
		if err != nil {
			t.Errorf("marshal planner content: %v", err)
			http.Error(w, "bad json", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-auto-continue",
			"choices":[{"message":{"role":"assistant","content":` + string(encodedContent) + `},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":8,"completion_tokens":18}
		}`))
	}))
	defer llmServer.Close()

	root := t.TempDir()
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-auto-continue"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(filepath.Join(root, "runs")),
		approvals: services.NewApprovalService(filepath.Join(root, "runs")),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"message":  "建立一个 daemon 健康检查任务",
		"provider": "openai",
		"model":    "auto-continue-test",
		"api_key":  "planner-key",
		"base_url": llmServer.URL,
	})
	if err != nil {
		t.Fatalf("marshal planner payload: %v", err)
	}
	planned := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if planned.StatusCode != http.StatusOK || !planned.Body.OK {
		t.Fatalf("unexpected planner response: status=%d body=%#v", planned.StatusCode, planned.Body)
	}
	plannedNext := asMap(t, planned.Body.Data["next_action"])
	if plannedNext["type"] != "continue" || !boolValue(t, plannedNext["auto_continue"]) {
		t.Fatalf("planner should persist auto_continue next action: %#v", plannedNext)
	}

	continued := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"继续"}`,
	)
	if continued.StatusCode != http.StatusOK || !continued.Body.OK {
		t.Fatalf("unexpected continue response: status=%d body=%#v", continued.StatusCode, continued.Body)
	}
	if llmRequests.Load() != 1 {
		t.Fatalf("continue without provider should not call LLM again; got %d requests", llmRequests.Load())
	}
	messages := asList(t, continued.Body.Data["messages"])
	if len(messages) < 3 || asMap(t, messages[0])["role"] != "user" || asMap(t, messages[1])["role"] != "agent" || asMap(t, messages[2])["role"] != "tool" {
		t.Fatalf("auto-continue should return user + control + tool observation: %#v", messages)
	}
	toolCalls := asList(t, continued.Body.Data["tool_calls"])
	if len(toolCalls) != 1 {
		t.Fatalf("auto-continue should execute exactly one low-risk tool: %#v", toolCalls)
	}
	tool := asMap(t, toolCalls[0])
	if tool["name"] != "GET /api/v1/doctor" || tool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("auto-continue should execute daemon health tool: %#v", tool)
	}
	metadata := asMap(t, tool["metadata"])
	if metadata["phase"] != "auto_continue" || metadata["auto_continue"] != true {
		t.Fatalf("auto-continue tool metadata mismatch: %#v", metadata)
	}
	next := asMap(t, continued.Body.Data["next_action"])
	if next["type"] != "finished" {
		t.Fatalf("auto-continue health check should finish local task: %#v", next)
	}
}

func TestHTTPAgentNaturalLanguageRunWorkspaceContext(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	runID := "job_context_workspace_001"
	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	writeDaemonRunJob(t, runsDir, runID, contracts.StatusSucceeded, now, now.Add(time.Minute))
	writeDaemonRunReport(t, runsDir, runID, now)
	writeDaemonApprovalRun(t, runsDir, runID, []contracts.HITLRequest{
		daemonTestApproval("approval_context_001", runID, "step_001"),
	})
	layout := runs.NewLayout(runsDir, runID)
	if err := events.NewStore(layout.EventsPath).Append(contracts.NewEvent("event_context_001", runID, "step_001", contracts.EventFindingCreated, contracts.EventLevelInfo, "Missing CSP finding created")); err != nil {
		t.Fatalf("append context event: %v", err)
	}
	if err := events.NewStore(layout.EventsPath).Append(contracts.NewEvent("event_context_002", runID, "step_001", contracts.EventReportCreated, contracts.EventLevelInfo, "report generated")); err != nil {
		t.Fatalf("append report event: %v", err)
	}

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-run-context"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-run-context"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	cases := []struct {
		name          string
		message       string
		wantContext   string
		wantResultKey string
		wantContent   string
	}{
		{
			name:          "report json",
			message:       "读取运行 " + runID + " 的 report json 上下文",
			wantContext:   "report_json",
			wantResultKey: "sample_findings",
			wantContent:   "report.json",
		},
		{
			name:          "artifact index",
			message:       "读取运行 " + runID + " 的 artifact index 证据索引",
			wantContext:   "artifact_index",
			wantResultKey: "sample_artifacts",
			wantContent:   "artifact index",
		},
		{
			name:          "findings",
			message:       "读取运行 " + runID + " 的 findings 并总结误报检查",
			wantContext:   "findings",
			wantResultKey: "sample_findings",
			wantContent:   "误报",
		},
		{
			name:          "events",
			message:       "读取运行 " + runID + " 的 events 时间线",
			wantContext:   "events",
			wantResultKey: "sample_events",
			wantContent:   "事件流",
		},
		{
			name:          "approvals",
			message:       "读取运行 " + runID + " 的 approvals 审批队列",
			wantContext:   "approvals",
			wantResultKey: "sample_approvals",
			wantContent:   "审批队列",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{
				"operator": "tester",
				"language": "zh",
				"message":  tc.message,
			})
			if err != nil {
				t.Fatalf("marshal context payload: %v", err)
			}
			send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
			if send.StatusCode != http.StatusOK || !send.Body.OK {
				t.Fatalf("unexpected run context response: status=%d body=%#v", send.StatusCode, send.Body)
			}
			tools := asList(t, send.Body.Data["tool_calls"])
			if len(tools) != 1 {
				t.Fatalf("expected one run context tool call: %#v", tools)
			}
			tool := asMap(t, tools[0])
			if tool["name"] != "GET /api/v1/runs/{run_id}/context" || tool["status"] != string(contracts.StatusSucceeded) {
				t.Fatalf("unexpected run context tool: %#v", tool)
			}
			result := asMap(t, tool["result"])
			if result["run_id"] != runID || result["context"] != tc.wantContext {
				t.Fatalf("unexpected run context result: %#v", result)
			}
			if _, ok := result[tc.wantResultKey]; !ok {
				t.Fatalf("run context result missing %s: %#v", tc.wantResultKey, result)
			}
			messages := asList(t, send.Body.Data["messages"])
			last := asMap(t, messages[len(messages)-1])
			if last["role"] != "tool" || !strings.Contains(stringValue(t, last["content"]), tc.wantContent) {
				t.Fatalf("run context observation should mention %q: %#v", tc.wantContent, last)
			}
		})
	}

	persisted := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if persisted.StatusCode != http.StatusOK || !persisted.Body.OK {
		t.Fatalf("unexpected persisted run context session response: status=%d body=%#v", persisted.StatusCode, persisted.Body)
	}
	refs := asList(t, asMap(t, persisted.Body.Data["session"])["context_refs"])
	for _, id := range []string{"run:" + runID, "report_json:" + runID, "artifact_index:" + runID, "findings:" + runID, "events:" + runID, "approvals:" + runID} {
		if !listOfMapsContains(t, refs, "id", id) {
			t.Fatalf("persisted context refs missing %s: %#v", id, refs)
		}
	}

	draftPayload, err := json.Marshal(map[string]string{
		"operator":        "tester",
		"language":        "zh",
		"operation_level": "workspace_write",
		"message":         "生成报告草稿 " + runID,
	})
	if err != nil {
		t.Fatalf("marshal report draft payload: %v", err)
	}
	draft := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(draftPayload))
	if draft.StatusCode != http.StatusOK || !draft.Body.OK {
		t.Fatalf("unexpected report draft response: status=%d body=%#v", draft.StatusCode, draft.Body)
	}
	draftTools := asList(t, draft.Body.Data["tool_calls"])
	if len(draftTools) != 1 {
		t.Fatalf("expected one report draft tool call: %#v", draftTools)
	}
	draftTool := asMap(t, draftTools[0])
	if draftTool["name"] != "reports.draft" || draftTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected report draft tool: %#v", draftTool)
	}
	draftMetadata := asMap(t, draftTool["metadata"])
	if draftMetadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("report draft tool should expose registry route source metadata: %#v", draftMetadata)
	}
	draftResult := asMap(t, draftTool["result"])
	draftPath := stringValue(t, draftResult["path"])
	if draftResult["run_id"] != runID || draftPath == "" || draftResult["draft_format"] != "markdown" {
		t.Fatalf("unexpected report draft result: %#v", draftResult)
	}
	draftBytes, err := os.ReadFile(filepath.Join(runsDir, ".agent", "workspaces", sessionID, "workspace", filepath.FromSlash(draftPath)))
	if err != nil {
		t.Fatalf("read generated report draft: %v", err)
	}
	draftContent := string(draftBytes)
	if !strings.Contains(draftContent, "CyberAgent 报告草稿") ||
		!strings.Contains(draftContent, "Missing CSP") ||
		!strings.Contains(draftContent, "Add a CSP header") {
		t.Fatalf("report draft content missing expected finding/remediation: %s", draftContent)
	}
}

func TestHTTPAgentLegacyOneShotSessionsNormalizeOnLoad(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	storeDir := filepath.Join(runsDir, ".agent", "sessions")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("create legacy agent store: %v", err)
	}
	now := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	finished := now.Add(time.Second)

	legacyHealth := agentSession{
		SessionID: "agent_legacy_health",
		CreatedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		Operator:  "tester",
		Language:  "zh",
		Status:    "running",
		Messages: []agentMessage{
			{ID: "msg_01", Role: "user", Content: "健康检查 daemon 状态", Timestamp: now},
			{
				ID:        "msg_02",
				Role:      "tool",
				Content:   "Daemon 在线，版本 v0.1.0。体检 通过，检查项 5，异常 3。",
				Timestamp: finished,
				ToolCalls: []agentToolCall{{
					ID:         "tool_legacy_doctor",
					Name:       "GET /api/v1/doctor",
					Status:     contracts.StatusSucceeded,
					Detail:     "daemon",
					StartedAt:  now,
					FinishedAt: &finished,
					Result:     map[string]any{"version": "v0.1.0", "required_issues": 0, "optional_missing": 3},
				}},
			},
		},
		ActiveTask: &agentTask{
			ID:        "task_legacy_health",
			Goal:      "检查 daemon 健康状态",
			Kind:      "health",
			Status:    "running",
			CreatedAt: now,
			UpdatedAt: finished,
		},
		Todos: []agentTodo{
			{ID: "todo_01", Title: "理解用户意图", Status: "done", Priority: 1, UpdatedAt: now},
			{ID: "todo_02", Title: "选择必要工具或本地回答", Status: "pending", Priority: 2, UpdatedAt: now},
			{ID: "todo_03", Title: "返回结果和下一步", Status: "pending", Priority: 3, UpdatedAt: now},
		},
		ToolCalls: []agentToolCall{{
			ID:         "tool_legacy_doctor",
			Name:       "GET /api/v1/doctor",
			Status:     contracts.StatusSucceeded,
			Detail:     "daemon",
			StartedAt:  now,
			FinishedAt: &finished,
			Result:     map[string]any{"version": "v0.1.0", "required_issues": 0, "optional_missing": 3},
		}},
		NextAction: &agentNextAction{Type: "continue", Reason: "task has pending or running steps", SuggestedPrompt: "继续当前任务"},
	}
	writeAgentSessionFile(t, storeDir, legacyHealth)

	legacyScan := agentSession{
		SessionID: "agent_legacy_scan",
		CreatedAt: now,
		UpdatedAt: now.Add(3 * time.Second),
		Operator:  "tester",
		Language:  "zh",
		Status:    "running",
		ActiveTask: &agentTask{
			ID:          "task_legacy_scan",
			Goal:        "扫描授权目标 http://127.0.0.1:18080 scope=127.0.0.1:18080",
			Kind:        "scan",
			Status:      "running",
			ActiveRunID: "job_legacy_run",
			CreatedAt:   now,
			UpdatedAt:   finished,
		},
		Todos: []agentTodo{
			{ID: "todo_01", Title: "确认目标与 scope", Status: "done", Priority: 1, UpdatedAt: now},
			{ID: "todo_02", Title: "跟踪运行事件、证据和审批", Status: "running", Priority: 2, UpdatedAt: now},
		},
		ToolCalls: []agentToolCall{{
			ID:         "tool_legacy_scan",
			Name:       "POST /api/v1/scans/web",
			Status:     contracts.StatusSucceeded,
			Detail:     "127.0.0.1:18080",
			StartedAt:  now,
			FinishedAt: &finished,
			Result:     map[string]any{"run_id": "job_legacy_run", "job_id": "job_legacy_job"},
		}},
		NextAction: &agentNextAction{Type: "continue", Reason: "task has pending or running steps", SuggestedPrompt: "继续跟踪运行"},
	}
	writeAgentSessionFile(t, storeDir, legacyScan)

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-legacy"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	healthRead := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/agent_legacy_health", "test-token")
	if healthRead.StatusCode != http.StatusOK || !healthRead.Body.OK {
		t.Fatalf("unexpected legacy health read response: status=%d body=%#v", healthRead.StatusCode, healthRead.Body)
	}
	healthSession := asMap(t, healthRead.Body.Data["session"])
	if healthSession["status"] != "idle" {
		t.Fatalf("legacy one-shot session should be normalized to idle: %#v", healthSession)
	}
	healthTask := asMap(t, healthSession["active_task"])
	if healthTask["status"] != "done" {
		t.Fatalf("legacy one-shot task should be done: %#v", healthTask)
	}
	healthNext := asMap(t, healthSession["next_action"])
	if healthNext["type"] != "finished" {
		t.Fatalf("legacy one-shot next action should be finished: %#v", healthNext)
	}
	for _, raw := range asList(t, healthSession["todos"]) {
		if todo := asMap(t, raw); todo["status"] != "done" {
			t.Fatalf("legacy one-shot todos should all be done: %#v", healthSession["todos"])
		}
	}
	healthMessages := asList(t, healthSession["messages"])
	healthLastMessage := asMap(t, healthMessages[len(healthMessages)-1])
	healthContent := stringValue(t, healthLastMessage["content"])
	if strings.Contains(healthContent, "异常") || !strings.Contains(healthContent, "可选工具缺失") {
		t.Fatalf("legacy doctor content should be normalized away from misleading issues: %q", healthContent)
	}

	healthOnDisk := readAgentSessionFile(t, storeDir, "agent_legacy_health")
	if healthOnDisk.Status != "idle" || healthOnDisk.ActiveTask == nil || healthOnDisk.ActiveTask.Status != "done" || healthOnDisk.NextAction == nil || healthOnDisk.NextAction.Type != "finished" {
		t.Fatalf("normalized legacy one-shot session should be persisted: %#v", healthOnDisk)
	}
	if len(healthOnDisk.Messages) < 2 || strings.Contains(healthOnDisk.Messages[len(healthOnDisk.Messages)-1].Content, "异常") {
		t.Fatalf("normalized legacy doctor message should be persisted: %#v", healthOnDisk.Messages)
	}

	scanRead := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/agent_legacy_scan", "test-token")
	if scanRead.StatusCode != http.StatusOK || !scanRead.Body.OK {
		t.Fatalf("unexpected legacy scan read response: status=%d body=%#v", scanRead.StatusCode, scanRead.Body)
	}
	scanSession := asMap(t, scanRead.Body.Data["session"])
	if scanSession["status"] != "running" {
		t.Fatalf("legacy scan session should not be normalized as one-shot: %#v", scanSession)
	}
	scanTask := asMap(t, scanSession["active_task"])
	if scanTask["status"] != "running" || scanTask["active_run_id"] != "job_legacy_run" {
		t.Fatalf("legacy scan task should keep active run context: %#v", scanTask)
	}
}

func TestHTTPAgentTerminalActiveRunSyncsOnReadAndList(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	storeDir := filepath.Join(runsDir, ".agent", "sessions")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("create agent store: %v", err)
	}
	now := time.Date(2026, 5, 26, 11, 0, 0, 0, time.UTC)
	finished := now.Add(2 * time.Minute)

	completedRunID := "job_terminal_succeeded"
	failedRunID := "job_terminal_failed"
	writeDaemonRunJob(t, runsDir, completedRunID, contracts.StatusSucceeded, now, finished)
	writeDaemonRunJob(t, runsDir, failedRunID, contracts.StatusFailed, now, finished)

	completedSession := agentSession{
		SessionID: "agent_terminal_succeeded",
		CreatedAt: now,
		UpdatedAt: now,
		Operator:  "tester",
		Language:  "zh",
		Status:    "running",
		Messages: []agentMessage{
			{ID: "msg_completed_user", Role: "user", Content: "扫描授权目标", Timestamp: now},
		},
		ActiveTask: &agentTask{
			ID:          "task_terminal_succeeded",
			Goal:        "扫描授权目标并跟踪运行",
			Kind:        "scan",
			Status:      "running",
			ActiveRunID: completedRunID,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		Todos: []agentTodo{
			{ID: "todo_completed_done", Title: "确认目标与 scope", Status: "done", Priority: 1, UpdatedAt: now},
			{ID: "todo_completed_running", Title: "跟踪运行事件、证据和审批", Status: "running", Priority: 2, UpdatedAt: now},
			{ID: "todo_completed_pending", Title: "总结证据和下一步", Status: "pending", Priority: 3, UpdatedAt: now},
		},
		NextAction: &agentNextAction{Type: "continue", Reason: "task has pending or running steps", SuggestedPrompt: "继续跟踪运行"},
	}
	writeAgentSessionFile(t, storeDir, completedSession)

	failedSession := agentSession{
		SessionID: "agent_terminal_failed",
		CreatedAt: now,
		UpdatedAt: now.Add(time.Second),
		Operator:  "tester",
		Language:  "zh",
		Status:    "running",
		Messages: []agentMessage{
			{ID: "msg_failed_user", Role: "user", Content: "扫描授权目标", Timestamp: now},
		},
		ActiveTask: &agentTask{
			ID:          "task_terminal_failed",
			Goal:        "扫描授权目标并跟踪运行",
			Kind:        "scan",
			Status:      "running",
			ActiveRunID: failedRunID,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		Todos: []agentTodo{
			{ID: "todo_failed_done", Title: "确认目标与 scope", Status: "done", Priority: 1, UpdatedAt: now},
			{ID: "todo_failed_running", Title: "跟踪运行事件、证据和审批", Status: "running", Priority: 2, UpdatedAt: now},
			{ID: "todo_failed_pending", Title: "总结失败原因", Status: "pending", Priority: 3, UpdatedAt: now},
		},
		NextAction: &agentNextAction{Type: "continue", Reason: "task has pending or running steps", SuggestedPrompt: "继续跟踪运行"},
	}
	writeAgentSessionFile(t, storeDir, failedSession)

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-terminal-run"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	continueControl := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/agent_terminal_succeeded/control",
		"test-token",
		`{"operator":"tester","language":"zh","action":"continue"}`,
	)
	if continueControl.StatusCode != http.StatusOK || !continueControl.Body.OK {
		t.Fatalf("unexpected continue control response: status=%d body=%#v", continueControl.StatusCode, continueControl.Body)
	}
	continueSession := asMap(t, continueControl.Body.Data["session"])
	if continueSession["status"] != "running" {
		t.Fatalf("explicit continue response should preserve control intent before read/list sync: %#v", continueSession)
	}
	continueTask := asMap(t, continueSession["active_task"])
	if continueTask["status"] != "running" || continueTask["active_run_id"] != completedRunID {
		t.Fatalf("continue response should keep active run context: %#v", continueTask)
	}
	continueNext := asMap(t, continueSession["next_action"])
	if continueNext["type"] != "continue" {
		t.Fatalf("continue response should keep continue next action: %#v", continueNext)
	}

	completedRead := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/agent_terminal_succeeded", "test-token")
	if completedRead.StatusCode != http.StatusOK || !completedRead.Body.OK {
		t.Fatalf("unexpected completed read response: status=%d body=%#v", completedRead.StatusCode, completedRead.Body)
	}
	completed := asMap(t, completedRead.Body.Data["session"])
	if completed["status"] != "idle" {
		t.Fatalf("completed terminal run should sync session to idle on read: %#v", completed)
	}
	completedTask := asMap(t, completed["active_task"])
	if completedTask["status"] != "idle" || completedTask["active_run_id"] != completedRunID {
		t.Fatalf("completed terminal run should keep active run context while idling task: %#v", completedTask)
	}
	completedNext := asMap(t, completed["next_action"])
	if completedNext["type"] != "wait_user" || !strings.Contains(stringValue(t, completedNext["suggested_prompt"]), completedRunID) {
		t.Fatalf("completed terminal run should suggest evidence summary: %#v", completedNext)
	}
	for _, raw := range asList(t, completed["todos"]) {
		todo := asMap(t, raw)
		if todo["status"] != "done" {
			t.Fatalf("completed terminal run should mark pending/running todos done: %#v", completed["todos"])
		}
	}
	completedOnDisk := readAgentSessionFile(t, storeDir, "agent_terminal_succeeded")
	if completedOnDisk.Status != "idle" || completedOnDisk.ActiveTask == nil || completedOnDisk.ActiveTask.Status != "idle" || completedOnDisk.ActiveTask.ActiveRunID != completedRunID {
		t.Fatalf("completed terminal sync should persist normalized state: %#v", completedOnDisk)
	}

	failedRead := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/agent_terminal_failed", "test-token")
	if failedRead.StatusCode != http.StatusOK || !failedRead.Body.OK {
		t.Fatalf("unexpected failed read response: status=%d body=%#v", failedRead.StatusCode, failedRead.Body)
	}
	failed := asMap(t, failedRead.Body.Data["session"])
	if failed["status"] != "blocked" {
		t.Fatalf("failed terminal run should sync session to blocked on read: %#v", failed)
	}
	failedTask := asMap(t, failed["active_task"])
	if failedTask["status"] != "blocked" || failedTask["active_run_id"] != failedRunID {
		t.Fatalf("failed terminal run should keep active run context while blocking task: %#v", failedTask)
	}
	failedNext := asMap(t, failed["next_action"])
	if failedNext["type"] != "blocked" || !strings.Contains(stringValue(t, failedNext["suggested_prompt"]), failedRunID) {
		t.Fatalf("failed terminal run should suggest recovery analysis: %#v", failedNext)
	}
	for _, raw := range asList(t, failed["todos"]) {
		todo := asMap(t, raw)
		if todo["status"] == "running" || todo["status"] == "pending" {
			t.Fatalf("failed terminal run should block pending/running todos: %#v", failed["todos"])
		}
	}

	list := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions?limit=10", "test-token")
	if list.StatusCode != http.StatusOK || !list.Body.OK {
		t.Fatalf("unexpected terminal list response: status=%d body=%#v", list.StatusCode, list.Body)
	}
	items := asList(t, list.Body.Data["items"])
	summaries := map[string]map[string]any{}
	for _, raw := range items {
		item := asMap(t, raw)
		summaries[stringValue(t, item["session_id"])] = item
	}
	if summaries["agent_terminal_succeeded"]["status"] != "idle" {
		t.Fatalf("list should expose completed terminal run as idle: %#v", summaries["agent_terminal_succeeded"])
	}
	if summaries["agent_terminal_failed"]["status"] != "blocked" {
		t.Fatalf("list should expose failed terminal run as blocked: %#v", summaries["agent_terminal_failed"])
	}
}

func TestHTTPAgentActiveRunApprovalGateAndContextRefs(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	storeDir := filepath.Join(runsDir, ".agent", "sessions")
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	runID := "job_approval_gate"
	writeDaemonRunJob(t, runsDir, runID, contracts.StatusRunning, now, time.Time{})
	writeDaemonRunReport(t, runsDir, runID, now)
	writeDaemonApprovalRun(t, runsDir, runID, []contracts.HITLRequest{
		daemonTestApproval("approval_gate_001", runID, "step_gate_001"),
	})
	session := agentSession{
		SessionID: "agent_approval_gate",
		CreatedAt: now,
		UpdatedAt: now,
		Operator:  "tester",
		Language:  "zh",
		Status:    "running",
		Messages: []agentMessage{
			{ID: "msg_approval_gate_user", Role: "user", Content: "扫描授权目标", Timestamp: now},
		},
		ActiveTask: &agentTask{
			ID:          "task_approval_gate",
			Goal:        "扫描授权目标并跟踪运行",
			Kind:        "scan",
			Status:      "running",
			ActiveRunID: runID,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		Todos: []agentTodo{
			{ID: "todo_approval_done", Title: "启动 daemon Web 扫描", Status: "done", Priority: 1, UpdatedAt: now},
			{ID: "todo_approval_running", Title: "跟踪运行事件、证据和审批", Status: "running", Priority: 2, UpdatedAt: now},
		},
		NextAction: &agentNextAction{Type: "continue", Reason: "tracking run", SuggestedPrompt: "继续跟踪运行"},
	}
	writeAgentSessionFile(t, storeDir, session)

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vapproval-gate"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	read := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/agent_approval_gate", "test-token")
	if read.StatusCode != http.StatusOK || !read.Body.OK {
		t.Fatalf("unexpected read approval gate session response: status=%d body=%#v", read.StatusCode, read.Body)
	}
	readSession := asMap(t, read.Body.Data["session"])
	next := asMap(t, readSession["next_action"])
	if next["type"] != "approval_required" || !strings.Contains(stringValue(t, next["reason"]), "待审批") {
		t.Fatalf("session should surface approval_required next action: %#v", next)
	}
	todos := asList(t, readSession["todos"])
	if asMap(t, todos[1])["status"] != "blocked" {
		t.Fatalf("approval tracking todo should be blocked by approval gate: %#v", todos)
	}

	payload := `{"operator":"tester","language":"zh","message":"查看审批队列"}`
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/agent_approval_gate/messages", "test-token", payload)
	if send.StatusCode != http.StatusOK || !send.Body.OK {
		t.Fatalf("unexpected approval queue message response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) == 0 {
		t.Fatalf("expected approval tool call: %#v", send.Body.Data)
	}
	tool := asMap(t, toolCalls[0])
	result := asMap(t, tool["result"])
	if tool["name"] != "GET /api/v1/runs/*/approvals" || result["pending"].(float64) != 1 || result["run_id"] != runID || result["context"] != "approval_queue" {
		t.Fatalf("unexpected approval queue tool result: tool=%#v result=%#v", tool, result)
	}
	contextRefs := asList(t, send.Body.Data["context_refs"])
	if !listOfMapsContains(t, contextRefs, "id", "run:"+runID) || !listOfMapsContains(t, contextRefs, "id", "approval:"+runID) || !listOfMapsContains(t, contextRefs, "id", "approval_queue:"+runID) {
		t.Fatalf("approval tool should persist run and approval context refs: %#v", contextRefs)
	}
	readRefs := asList(t, readSession["context_refs"])
	if !listOfMapsContains(t, readRefs, "id", "run:"+runID) ||
		!listOfMapsContains(t, readRefs, "id", "approval:"+runID) ||
		!listOfMapsContains(t, readRefs, "id", "report_json:"+runID) ||
		!listOfMapsContains(t, readRefs, "id", "artifact_index:"+runID) ||
		!listOfMapsContains(t, readRefs, "id", "finding:"+runID) {
		t.Fatalf("session read should auto hydrate active run context refs: %#v", readRefs)
	}
	readTask := asMap(t, readSession["active_task"])
	if !strings.Contains(stringValue(t, readTask["summary"]), "pending_approvals=1") ||
		!strings.Contains(stringValue(t, readTask["summary"]), "Missing CSP") ||
		!strings.Contains(stringValue(t, readTask["summary"]), "HTTP status") {
		t.Fatalf("session read should refresh active run context summary: %#v", readTask)
	}
	reportRef := findMapByKey(t, readRefs, "id", "report_json:"+runID)
	if !strings.Contains(stringValue(t, reportRef["summary"]), "Missing CSP") || !strings.Contains(stringValue(t, reportRef["summary"]), "HTTP status") {
		t.Fatalf("report context ref should include report/evidence summary: %#v", reportRef)
	}
	findingRef := findMapByKey(t, readRefs, "id", "finding:"+runID)
	if !strings.Contains(stringValue(t, findingRef["summary"]), "Missing CSP") || !strings.Contains(stringValue(t, findingRef["summary"]), "Add a CSP header") {
		t.Fatalf("finding context ref should include lead finding summary: %#v", findingRef)
	}
	artifactRef := findMapByKey(t, readRefs, "id", "artifact_index:"+runID)
	if !strings.Contains(stringValue(t, artifactRef["summary"]), "headers.json") || !strings.Contains(stringValue(t, artifactRef["summary"]), "evidence_001") {
		t.Fatalf("artifact context ref should include artifact/evidence summary: %#v", artifactRef)
	}

	runDetail := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID, "test-token")
	if runDetail.StatusCode != http.StatusOK || !runDetail.Body.OK {
		t.Fatalf("unexpected run detail response: status=%d body=%#v", runDetail.StatusCode, runDetail.Body)
	}
	summary := asMap(t, runDetail.Body.Data["summary"])
	counts := asMap(t, summary["counts"])
	if counts["approvals"].(float64) != 1 || counts["pending_approvals"].(float64) != 1 {
		t.Fatalf("run counts should include pending approvals: %#v", counts)
	}

	list := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions?limit=10", "test-token")
	if list.StatusCode != http.StatusOK || !list.Body.OK {
		t.Fatalf("unexpected approval gate list response: status=%d body=%#v", list.StatusCode, list.Body)
	}
	items := asList(t, list.Body.Data["items"])
	listItem := findMapByKey(t, items, "session_id", "agent_approval_gate")
	runSummary := asMap(t, listItem["run_summary"])
	if runSummary["run_id"] != runID || runSummary["pending_approvals"].(float64) != 1 || runSummary["findings"].(float64) != 1 {
		t.Fatalf("session list should expose active run summary counts: %#v", runSummary)
	}
	for _, key := range []string{"summary", "lead_finding", "lead_evidence", "lead_artifact"} {
		if strings.TrimSpace(stringValue(t, runSummary[key])) == "" {
			t.Fatalf("session list run_summary should include %s: %#v", key, runSummary)
		}
	}
	if !strings.Contains(stringValue(t, runSummary["lead_finding"]), "Missing CSP") ||
		!strings.Contains(stringValue(t, runSummary["lead_evidence"]), "HTTP status") ||
		!strings.Contains(stringValue(t, runSummary["lead_artifact"]), "headers.json") {
		t.Fatalf("session list run_summary should expose lead context: %#v", runSummary)
	}
}

func TestHTTPAgentChatNativeApprovalDecisions(t *testing.T) {
	cases := []struct {
		name       string
		action     string
		status     contracts.HITLStatus
		messageTpl string
		comment    string
	}{
		{
			name:       "approve",
			action:     "approve",
			status:     contracts.HITLApproved,
			messageTpl: "批准运行 %s 的 %s，comment=%s",
			comment:    "approved from agent chat",
		},
		{
			name:       "deny",
			action:     "deny",
			status:     contracts.HITLRejected,
			messageTpl: "拒绝运行 %s 的 %s，comment=%s",
			comment:    "denied from agent chat",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			runsDir := filepath.Join(root, "runs")
			storeDir := filepath.Join(runsDir, ".agent", "sessions")
			now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
			runID := "job_agent_approval_" + tc.name
			approvalID := "approval_chat_" + tc.name
			writeDaemonRunJob(t, runsDir, runID, contracts.StatusRunning, now, time.Time{})
			writeDaemonRunReport(t, runsDir, runID, now)
			writeDaemonApprovalRun(t, runsDir, runID, []contracts.HITLRequest{
				daemonTestApproval(approvalID, runID, "step_chat_"+tc.name),
			})
			sessionID := "agent_chat_approval_" + tc.name
			session := agentSession{
				SessionID: sessionID,
				CreatedAt: now,
				UpdatedAt: now,
				Operator:  "tester",
				Language:  "zh",
				Status:    "running",
				Messages: []agentMessage{
					{ID: "msg_" + tc.name + "_user", Role: "user", Content: "跟踪审批 gate", Timestamp: now},
				},
				ActiveTask: &agentTask{
					ID:          "task_" + tc.name,
					Goal:        "通过聊天处理审批 gate",
					Kind:        "scan",
					Status:      "running",
					ActiveRunID: runID,
					CreatedAt:   now,
					UpdatedAt:   now,
				},
				Todos: []agentTodo{
					{ID: "todo_start_" + tc.name, Title: "启动 daemon Web 扫描", Status: "done", Priority: 1, UpdatedAt: now},
					{ID: "todo_approval_" + tc.name, Title: "处理待审批 approval gate", Status: "blocked", Priority: 2, BlockedReason: "等待人工审批", UpdatedAt: now},
					{ID: "todo_track_" + tc.name, Title: "继续跟踪运行证据", Status: "pending", Priority: 3, UpdatedAt: now},
				},
				NextAction: &agentNextAction{Type: "approval_required", Reason: "等待审批", SuggestedPrompt: "处理待审批"},
			}
			writeAgentSessionFile(t, storeDir, session)

			handler := newRouter(routerServices{
				version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vapproval-chat"}),
				plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
				runs:      services.NewRunService(runsDir),
				approvals: services.NewApprovalService(runsDir),
			}, "test-token")
			api := httptest.NewServer(handler)
			defer api.Close()

			payload, err := json.Marshal(map[string]any{
				"operator": "tester",
				"language": "zh",
				"message":  fmt.Sprintf(tc.messageTpl, runID, approvalID, tc.comment),
			})
			if err != nil {
				t.Fatalf("marshal approval decision message: %v", err)
			}
			send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
			if send.StatusCode != http.StatusOK || !send.Body.OK {
				t.Fatalf("unexpected chat approval decision response: status=%d body=%#v", send.StatusCode, send.Body)
			}
			toolCalls := asList(t, send.Body.Data["tool_calls"])
			if len(toolCalls) == 0 {
				t.Fatalf("expected approval decision tool call: %#v", send.Body.Data)
			}
			tool := asMap(t, toolCalls[0])
			result := asMap(t, tool["result"])
			wantTool := "POST /api/v1/runs/{run_id}/approvals/{approval_id}/" + tc.action
			if tool["name"] != wantTool || tool["status"] != string(contracts.StatusSucceeded) {
				t.Fatalf("unexpected approval decision tool: %#v", tool)
			}
			if result["run_id"] != runID ||
				result["approval_id"] != approvalID ||
				result["status"] != string(tc.status) ||
				result["context"] != "approval_decision" ||
				result["pending"].(float64) != 0 {
				t.Fatalf("unexpected approval decision result: %#v", result)
			}

			next := asMap(t, send.Body.Data["next_action"])
			if next["type"] != "continue" || !boolValue(t, next["auto_continue"]) {
				t.Fatalf("approval decision should refresh gate back to continue: %#v", next)
			}
			todos := asList(t, send.Body.Data["todos"])
			if asMap(t, todos[1])["status"] != "done" || asMap(t, todos[2])["status"] != "running" {
				t.Fatalf("approval gate todo should resolve and next todo should run: %#v", todos)
			}
			contextRefs := asList(t, send.Body.Data["context_refs"])
			if !listOfMapsContains(t, contextRefs, "id", "run:"+runID) ||
				!listOfMapsContains(t, contextRefs, "id", "approval:"+runID) ||
				!listOfMapsContains(t, contextRefs, "id", "approval_decision:"+runID) {
				t.Fatalf("approval decision should persist run/approval refs: %#v", contextRefs)
			}

			list := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/approvals", "test-token")
			if list.StatusCode != http.StatusOK || !list.Body.OK {
				t.Fatalf("unexpected approval list after chat decision: status=%d body=%#v", list.StatusCode, list.Body)
			}
			approvals := asList(t, list.Body.Data["approvals"])
			resolved := asMap(t, approvals[0])
			if resolved["status"] != string(tc.status) {
				t.Fatalf("approval should be persisted as %s: %#v", tc.status, resolved)
			}
			resolution := asMap(t, resolved["resolution"])
			if resolution["comment"] != tc.comment || resolution["resolved_by"] != "tester" {
				t.Fatalf("approval resolution metadata should come from chat: %#v", resolution)
			}
		})
	}
}

func TestHTTPAgentChatNativeResumeApproved(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte("<urlset></urlset>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	storeDir := filepath.Join(runsDir, ".agent", "sessions")
	now := time.Date(2026, 5, 28, 11, 0, 0, 0, time.UTC)
	runID := "run_agent_resume_chat"
	runDir := filepath.Join(runsDir, runID)
	if _, err := coreruntime.RunWebScan(context.Background(), coreruntime.WebScanRequest{
		Target:          target.URL,
		Scope:           target.URL,
		Out:             runDir,
		Profile:         "safe",
		NoExternalTools: true,
		BuildInfo:       coreruntime.BuildInfo{Version: "vagent-resume-test"},
		CommandName:     "scan web",
	}); err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}
	jobPath := filepath.Join(runDir, "job.json")
	var job contracts.Job
	jobData, err := os.ReadFile(jobPath)
	if err != nil {
		t.Fatalf("read resume test job: %v", err)
	}
	if err := json.Unmarshal(jobData, &job); err != nil {
		t.Fatalf("decode resume test job: %v", err)
	}
	job.Status = contracts.StatusRunning
	job.FinishedAt = nil
	encodedJob, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		t.Fatalf("encode resume test job: %v", err)
	}
	if err := os.WriteFile(jobPath, append(encodedJob, '\n'), 0o644); err != nil {
		t.Fatalf("write resume test job: %v", err)
	}

	sessionID := "agent_resume_approved_chat"
	session := agentSession{
		SessionID: sessionID,
		CreatedAt: now,
		UpdatedAt: now,
		Operator:  "tester",
		Language:  "zh",
		Status:    "running",
		Messages: []agentMessage{
			{ID: "msg_resume_user", Role: "user", Content: "审批已经处理，准备恢复", Timestamp: now},
		},
		ActiveTask: &agentTask{
			ID:          "task_resume",
			Goal:        "恢复已批准运行步骤并继续跟踪",
			Kind:        "approval",
			Status:      "running",
			ActiveRunID: runID,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		Todos: []agentTodo{
			{ID: "todo_resume", Title: "恢复已批准步骤", Status: "running", Priority: 1, UpdatedAt: now},
			{ID: "todo_track_after_resume", Title: "继续跟踪运行证据和报告", Status: "pending", Priority: 2, UpdatedAt: now},
		},
		NextAction: &agentNextAction{Type: "continue", Reason: "审批已处理", SuggestedPrompt: "恢复已批准运行 " + runID},
	}
	writeAgentSessionFile(t, storeDir, session)

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vresume-chat"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"message":  "恢复已批准运行 " + runID,
	})
	if err != nil {
		t.Fatalf("marshal resume-approved message: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK {
		t.Fatalf("unexpected chat resume-approved response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) == 0 {
		t.Fatalf("expected resume-approved tool call: %#v", send.Body.Data)
	}
	tool := asMap(t, toolCalls[0])
	result := asMap(t, tool["result"])
	if tool["name"] != "POST /api/v1/runs/{run_id}/resume-approved" || tool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected resume-approved tool: %#v", tool)
	}
	if result["run_id"] != runID || result["context"] != "resume_approved" || result["resumed_count"].(float64) != 0 || result["skipped_count"].(float64) != 0 {
		t.Fatalf("unexpected resume-approved result: %#v", result)
	}
	metadata := asMap(t, tool["metadata"])
	if metadata["phase"] != "approval_resume" || metadata["approval_policy"] != "explicit_user_instruction" {
		t.Fatalf("resume-approved tool should be classified as approval_resume: %#v", metadata)
	}
	next := asMap(t, send.Body.Data["next_action"])
	if next["type"] != "continue" || !boolValue(t, next["auto_continue"]) {
		t.Fatalf("resume-approved should keep active run continuation: %#v", next)
	}
	todos := asList(t, send.Body.Data["todos"])
	if asMap(t, todos[0])["status"] != "done" || asMap(t, todos[1])["status"] != "running" {
		t.Fatalf("resume-approved should complete resume todo and run tracking todo: %#v", todos)
	}
	contextRefs := asList(t, send.Body.Data["context_refs"])
	if !listOfMapsContains(t, contextRefs, "id", "run:"+runID) ||
		!listOfMapsContains(t, contextRefs, "id", "resume_approved:"+runID) {
		t.Fatalf("resume-approved should persist run/context refs: %#v", contextRefs)
	}
}

func TestHTTPAgentPlannerApprovalActionRequiresExplicitPrompt(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	storeDir := filepath.Join(runsDir, ".agent", "sessions")
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	runID := "job_planner_approval"
	approvalID := "approval_planner_001"
	writeDaemonRunJob(t, runsDir, runID, contracts.StatusRunning, now, time.Time{})
	writeDaemonRunReport(t, runsDir, runID, now)
	writeDaemonApprovalRun(t, runsDir, runID, []contracts.HITLRequest{
		daemonTestApproval(approvalID, runID, "step_planner_001"),
	})
	sessionID := "agent_planner_approval"
	writeAgentSessionFile(t, storeDir, agentSession{
		SessionID: sessionID,
		CreatedAt: now,
		UpdatedAt: now,
		Operator:  "tester",
		Language:  "zh",
		Status:    "running",
		Messages: []agentMessage{
			{ID: "msg_planner_approval_user", Role: "user", Content: "等待审批", Timestamp: now},
		},
		ActiveTask: &agentTask{
			ID:          "task_planner_approval",
			Goal:        "通过 Planner action 处理审批",
			Kind:        "approval",
			Status:      "running",
			ActiveRunID: runID,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		Todos: []agentTodo{
			{ID: "todo_planner_approval", Title: "处理待审批 approval gate", Status: "blocked", Priority: 1, BlockedReason: "等待人工审批", UpdatedAt: now},
			{ID: "todo_planner_track", Title: "继续跟踪运行证据", Status: "pending", Priority: 2, UpdatedAt: now},
		},
		NextAction: &agentNextAction{Type: "approval_required", Reason: "等待审批", SuggestedPrompt: "处理待审批"},
	})

	var llmRequests atomic.Int32
	plannerContent := fmt.Sprintf(`{
		"answer":"收到明确审批指令，我将调用 approval decision 工具。",
		"task":{"goal":"处理审批","kind":"approval","status":"running","summary":"批准指定 approval 后继续跟踪运行。"},
		"todos":[
			{"title":"处理待审批 approval gate","status":"running","priority":1},
			{"title":"继续跟踪运行证据","status":"pending","priority":2}
		],
		"actions":[
			{"name":"runs.approvals.approve","arguments":{"run_id":%q,"approval_id":%q,"comment":"planner approved"}}
		],
		"next_action":{"type":"continue","reason":"审批工具执行后继续跟踪。","suggested_prompt":"继续跟踪运行 %s","auto_continue":true}
	}`, runID, approvalID, runID)
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := llmRequests.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode LLM request: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		encodedMessages, _ := json.Marshal(body["messages"])
		if requestNumber == 1 && (!strings.Contains(string(encodedMessages), "runs.approvals.approve") || !strings.Contains(string(encodedMessages), "显式指令")) {
			t.Errorf("planner prompt should expose approval action policy: %s", encodedMessages)
		}
		responseContent := plannerContent
		if requestNumber == 2 {
			if !strings.Contains(string(encodedMessages), "approval decision") && !strings.Contains(string(encodedMessages), "审批") {
				t.Errorf("synthesis prompt should include approval observation: %s", encodedMessages)
			}
			responseContent = `{
				"answer":"审批已处理，approval gate 已解除，下一步继续跟踪运行证据。",
				"task":{"goal":"处理审批","kind":"approval","status":"running","summary":"审批完成，继续跟踪运行。"},
				"todos":[
					{"title":"处理待审批 approval gate","status":"done","priority":1},
					{"title":"继续跟踪运行证据","status":"running","priority":2}
				],
				"next_action":{"type":"continue","reason":"可继续跟踪运行。","suggested_prompt":"继续跟踪运行","auto_continue":true}
			}`
		}
		encodedContent, err := json.Marshal(responseContent)
		if err != nil {
			t.Errorf("marshal planner approval content: %v", err)
			http.Error(w, "bad json", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-approval-action",
			"choices":[{"message":{"role":"assistant","content":` + string(encodedContent) + `},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":12,"completion_tokens":30}
		}`))
	}))
	defer llmServer.Close()

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vplanner-approval"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"message":  "修订计划：批准运行 " + runID + " 的 " + approvalID + "，comment=planner approved",
		"provider": "openai",
		"model":    "planner-approval-test",
		"api_key":  "planner-approval-key",
		"base_url": llmServer.URL,
	})
	if err != nil {
		t.Fatalf("marshal planner approval payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK {
		t.Fatalf("unexpected planner approval response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if llmRequests.Load() != 1 {
		t.Fatalf("LLM server received %d requests before authorization, want 1", llmRequests.Load())
	}
	pendingAuth := asMap(t, send.Body.Data["pending_authorization"])
	if pendingAuth["action_name"] != "runs.approvals.approve" || pendingAuth["status"] != "pending" {
		t.Fatalf("planner approval should pause at authorization gate: %#v", pendingAuth)
	}

	approvePayload, err := json.Marshal(map[string]any{
		"operator":        "tester",
		"language":        "zh",
		"operation_level": "approval_ops",
		"provider":        "openai",
		"model":           "planner-approval-test",
		"api_key":         "planner-approval-key",
		"base_url":        llmServer.URL,
	})
	if err != nil {
		t.Fatalf("marshal planner approval authorize payload: %v", err)
	}
	approve := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/authorization/approve", "test-token", string(approvePayload))
	if approve.StatusCode != http.StatusOK || !approve.Body.OK {
		t.Fatalf("unexpected planner approval authorize response: status=%d body=%#v", approve.StatusCode, approve.Body)
	}
	if llmRequests.Load() != 2 {
		t.Fatalf("LLM server received %d requests after authorization, want 2", llmRequests.Load())
	}
	toolCalls := asList(t, asMap(t, approve.Body.Data["session"])["tool_calls"])
	approvalTool := findMapByKey(t, toolCalls, "name", "POST /api/v1/runs/{run_id}/approvals/{approval_id}/approve")
	if approvalTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("planner approval tool should succeed: %#v", approvalTool)
	}
	approvalResult := asMap(t, approvalTool["result"])
	if approvalResult["run_id"] != runID || approvalResult["approval_id"] != approvalID || approvalResult["status"] != string(contracts.HITLApproved) {
		t.Fatalf("unexpected planner approval result: %#v", approvalResult)
	}
	metadata := asMap(t, approvalTool["metadata"])
	if metadata["action_name"] != "runs.approvals.approve" || metadata["phase"] != "approval_decision" {
		t.Fatalf("planner approval action metadata mismatch: %#v", metadata)
	}

	list := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/approvals", "test-token")
	approvals := asList(t, list.Body.Data["approvals"])
	resolved := asMap(t, approvals[0])
	if resolved["status"] != string(contracts.HITLApproved) {
		t.Fatalf("planner approval should persist approved status: %#v", resolved)
	}
	resolution := asMap(t, resolved["resolution"])
	if resolution["resolved_by"] != "tester" || resolution["comment"] != "planner approved" {
		t.Fatalf("planner approval should persist operator/comment metadata: %#v", resolution)
	}
}

func TestHTTPAgentEvidenceToActionControlPreservesRunAndApprovalContext(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	storeDir := filepath.Join(runsDir, ".agent", "sessions")
	now := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	runID := "job_evidence_action"
	writeDaemonRunJob(t, runsDir, runID, contracts.StatusRunning, now, time.Time{})
	writeDaemonRunReport(t, runsDir, runID, now)
	writeDaemonApprovalRun(t, runsDir, runID, []contracts.HITLRequest{
		daemonTestApproval("approval_evidence_action", runID, "step_gate_evidence"),
	})
	session := agentSession{
		SessionID: "agent_evidence_action",
		CreatedAt: now,
		UpdatedAt: now,
		Operator:  "tester",
		Language:  "zh",
		Status:    "running",
		Messages: []agentMessage{
			{ID: "msg_evidence_action_user", Role: "user", Content: "请跟踪运行证据", Timestamp: now},
		},
		ActiveTask: &agentTask{
			ID:          "task_evidence_action",
			Goal:        "跟踪运行证据并生成下一步",
			Kind:        "scan",
			Status:      "running",
			ActiveRunID: runID,
			Summary:     "跟踪运行证据",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		Todos: []agentTodo{
			{ID: "todo_evidence_action", Title: "复盘证据并决定下一步", Status: "running", Priority: 1, UpdatedAt: now},
		},
		NextAction: &agentNextAction{Type: "continue", Reason: "tracking evidence", SuggestedPrompt: "继续跟踪证据"},
	}
	writeAgentSessionFile(t, storeDir, session)

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vevidence-action"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	read := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/agent_evidence_action", "test-token")
	if read.StatusCode != http.StatusOK || !read.Body.OK {
		t.Fatalf("unexpected evidence action read response: status=%d body=%#v", read.StatusCode, read.Body)
	}
	readSession := asMap(t, read.Body.Data["session"])
	readRefs := asList(t, readSession["context_refs"])
	for _, id := range []string{"run:" + runID, "approval:" + runID, "report_json:" + runID, "artifact_index:" + runID, "finding:" + runID} {
		if !listOfMapsContains(t, readRefs, "id", id) {
			t.Fatalf("read should hydrate context ref %s: %#v", id, readRefs)
		}
	}

	evidenceActionPrompt := "请把当前运行证据转成可执行的 Cybersecurity Agent 下一步行动计划。\nActive run: " + runID + " target=http://127.0.0.1:8080 status=running findings=1 artifacts=1 pending_approvals=1\nContext refs:\n1. [finding] Missing CSP — HTTP status evidence_001\nPending approvals:\n1. execute_step status=pending step=step_gate_evidence reason=requires approval\n计划要求：\n1. 将证据转成 3-5 个有顺序的 TODO。\n2. 如果存在待审批项，把审批处理作为第一个动作。\n3. 最后给出一个可被继续直接执行的 suggested_prompt。"
	body, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"action":   "revise",
		"prompt":   evidenceActionPrompt,
	})
	if err != nil {
		t.Fatalf("marshal evidence action control: %v", err)
	}
	revise := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/agent_evidence_action/control", "test-token", string(body))
	if revise.StatusCode != http.StatusOK || !revise.Body.OK {
		t.Fatalf("unexpected evidence action revise response: status=%d body=%#v", revise.StatusCode, revise.Body)
	}
	reviseSession := asMap(t, revise.Body.Data["session"])
	if reviseSession["status"] != "running" {
		t.Fatalf("evidence action revise should keep session running: %#v", reviseSession)
	}
	reviseTask := asMap(t, reviseSession["active_task"])
	if reviseTask["active_run_id"] != runID || reviseTask["status"] != "running" {
		t.Fatalf("evidence action revise should preserve active run task: %#v", reviseTask)
	}
	if !strings.Contains(stringValue(t, reviseTask["summary"]), "下一步行动计划") ||
		!strings.Contains(stringValue(t, reviseTask["summary"]), runID) ||
		!strings.Contains(stringValue(t, reviseTask["summary"]), "pending_approvals=1") {
		t.Fatalf("evidence action revise should summarize run evidence prompt: %#v", reviseTask)
	}
	reviseNext := asMap(t, reviseSession["next_action"])
	if reviseNext["type"] != "approval_required" ||
		!strings.Contains(stringValue(t, reviseNext["reason"]), "审批") ||
		!strings.Contains(stringValue(t, reviseNext["suggested_prompt"]), "待审批") ||
		!strings.Contains(stringValue(t, reviseNext["suggested_prompt"]), "TODO") {
		t.Fatalf("evidence action revise should gate on pending approval: %#v", reviseNext)
	}
	reviseTodos := asList(t, reviseSession["todos"])
	if len(reviseTodos) < 4 || asMap(t, reviseTodos[0])["status"] != "running" {
		t.Fatalf("evidence action revise should keep a running todo: %#v", reviseTodos)
	}
	if !strings.Contains(stringValue(t, asMap(t, reviseTodos[0])["title"]), "审批") ||
		!strings.Contains(stringValue(t, asMap(t, reviseTodos[1])["title"]), "证据") ||
		!strings.Contains(stringValue(t, asMap(t, reviseTodos[2])["title"]), "finding") {
		t.Fatalf("evidence action revise should turn evidence prompt into ordered todos: %#v", reviseTodos)
	}
	reviseRefs := asList(t, reviseSession["context_refs"])
	if !listOfMapsContains(t, reviseRefs, "id", "approval:"+runID) || !listOfMapsContains(t, reviseRefs, "id", "finding:"+runID) {
		t.Fatalf("evidence action revise should preserve hydrated context refs: %#v", reviseRefs)
	}
	reviseMessages := asList(t, reviseSession["messages"])
	if !strings.Contains(stringValue(t, asMap(t, reviseMessages[len(reviseMessages)-1])["content"]), "修订") {
		t.Fatalf("evidence action revise should append control message: %#v", reviseMessages[len(reviseMessages)-1])
	}

	continueBody, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"action":   "continue",
		"prompt":   stringValue(t, reviseNext["suggested_prompt"]),
	})
	if err != nil {
		t.Fatalf("marshal evidence action continue: %v", err)
	}
	cont := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/agent_evidence_action/control", "test-token", string(continueBody))
	if cont.StatusCode != http.StatusOK || !cont.Body.OK {
		t.Fatalf("unexpected evidence action continue response: status=%d body=%#v", cont.StatusCode, cont.Body)
	}
	continueSession := asMap(t, cont.Body.Data["session"])
	continueTask := asMap(t, continueSession["active_task"])
	continueNext := asMap(t, continueSession["next_action"])
	if continueTask["active_run_id"] != runID || continueNext["type"] != "approval_required" || !strings.Contains(stringValue(t, continueNext["suggested_prompt"]), "待审批") {
		t.Fatalf("evidence action continue should preserve approval gate and run context: task=%#v next=%#v", continueTask, continueNext)
	}

	approve := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/"+runID+"/approvals/approval_evidence_action/approve", "test-token", `{"operator":"tester","comment":"approved from agent test"}`)
	if approve.StatusCode != http.StatusOK || !approve.Body.OK || approve.Body.Data["status"] != string(contracts.HITLApproved) {
		t.Fatalf("unexpected evidence action approval response: status=%d body=%#v", approve.StatusCode, approve.Body)
	}
	afterApproval := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/agent_evidence_action", "test-token")
	if afterApproval.StatusCode != http.StatusOK || !afterApproval.Body.OK {
		t.Fatalf("unexpected evidence action post-approval read response: status=%d body=%#v", afterApproval.StatusCode, afterApproval.Body)
	}
	afterApprovalSession := asMap(t, afterApproval.Body.Data["session"])
	afterApprovalTask := asMap(t, afterApprovalSession["active_task"])
	afterApprovalNext := asMap(t, afterApprovalSession["next_action"])
	if afterApprovalTask["active_run_id"] != runID || afterApprovalTask["status"] != "running" || afterApprovalNext["type"] != "continue" {
		t.Fatalf("post-approval session should converge back to continue: task=%#v next=%#v", afterApprovalTask, afterApprovalNext)
	}
	if !boolValue(t, afterApprovalNext["auto_continue"]) || !strings.Contains(stringValue(t, afterApprovalNext["suggested_prompt"]), runID) {
		t.Fatalf("post-approval next action should be auto-continue for active run: %#v", afterApprovalNext)
	}
	afterApprovalTodos := asList(t, afterApprovalSession["todos"])
	if len(afterApprovalTodos) < 4 ||
		asMap(t, afterApprovalTodos[0])["status"] != "done" ||
		asMap(t, afterApprovalTodos[1])["status"] != "running" ||
		asMap(t, afterApprovalTodos[0])["blocked_reason"] != nil {
		t.Fatalf("post-approval todos should clear blocked approval gate and resume next step: %#v", afterApprovalTodos)
	}
	afterApprovalRefs := asList(t, afterApprovalSession["context_refs"])
	if !listOfMapsContains(t, afterApprovalRefs, "id", "approval:"+runID) || !listOfMapsContains(t, afterApprovalRefs, "id", "finding:"+runID) {
		t.Fatalf("post-approval read should keep run evidence context refs: %#v", afterApprovalRefs)
	}
	afterApprovalList := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions?limit=10", "test-token")
	if afterApprovalList.StatusCode != http.StatusOK || !afterApprovalList.Body.OK {
		t.Fatalf("unexpected evidence action post-approval list response: status=%d body=%#v", afterApprovalList.StatusCode, afterApprovalList.Body)
	}
	afterApprovalItems := asList(t, afterApprovalList.Body.Data["items"])
	afterApprovalItem := findMapByKey(t, afterApprovalItems, "session_id", "agent_evidence_action")
	afterApprovalRunSummary := asMap(t, afterApprovalItem["run_summary"])
	if afterApprovalRunSummary["pending_approvals"].(float64) != 0 {
		t.Fatalf("post-approval run summary should have no pending approvals: %#v", afterApprovalRunSummary)
	}
}

func TestHTTPAgentSessionStreamAPI(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-stream"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs:      newJobManager(jobManagerConfig{RunsDir: runsDir, Build: services.BuildInfo{Name: "CyberAgent", Version: "vagent-stream"}}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.URL+"/api/v1/agent/sessions/"+sessionID+"/stream", nil)
	if err != nil {
		t.Fatalf("new stream request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected stream status: %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("unexpected stream content-type: %q", got)
	}

	reader := bufio.NewReader(resp.Body)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for session stream event")
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		if strings.TrimSpace(line) != "event: session" {
			continue
		}
		dataLine, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream data: %v", err)
		}
		if !strings.HasPrefix(dataLine, "data: ") {
			t.Fatalf("unexpected stream data line: %q", dataLine)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(dataLine, "data: "))), &payload); err != nil {
			t.Fatalf("decode stream payload: %v", err)
		}
		streamSession := asMap(t, payload["session"])
		if streamSession["session_id"] != sessionID {
			t.Fatalf("unexpected stream session: %#v", streamSession)
		}
		break
	}
}

func TestHTTPAgentCapabilitiesAPI(t *testing.T) {
	root := t.TempDir()
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-capabilities"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(filepath.Join(root, "runs")),
		approvals: services.NewApprovalService(filepath.Join(root, "runs")),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	noAuth := doDaemonRequest(t, api.URL+"/api/v1/agent/capabilities?language=zh", "")
	if noAuth.StatusCode != http.StatusUnauthorized || noAuth.Body.OK || noAuth.Body.Error.Code != services.CodeUnauthorized {
		t.Fatalf("unexpected no-auth agent capabilities response: status=%d body=%#v", noAuth.StatusCode, noAuth.Body)
	}

	capabilities := doDaemonRequest(t, api.URL+"/api/v1/agent/capabilities?language=zh", "test-token")
	if capabilities.StatusCode != http.StatusOK || !capabilities.Body.OK {
		t.Fatalf("unexpected agent capabilities response: status=%d body=%#v", capabilities.StatusCode, capabilities.Body)
	}
	data := capabilities.Body.Data
	if data["name"] != "CyberAgent" || data["api_version"] != agentAPIVersion || data["entry_point"] != "/api/v1/agent/sessions/{session_id}/messages" {
		t.Fatalf("unexpected agent capabilities metadata: %#v", data)
	}

	endpoints := asList(t, data["endpoints"])
	for _, path := range []string{
		"/api/v1/agent/sessions",
		"/api/v1/agent/sessions/{session_id}",
		"/api/v1/agent/sessions/{session_id}/state",
		"/api/v1/agent/sessions/{session_id}/context",
		"/api/v1/agent/sessions/{session_id}/messages",
		"/api/v1/agent/sessions/{session_id}/control",
		"/api/v1/agent/sessions/{session_id}/continue",
		"/api/v1/agent/sessions/{session_id}/pause",
		"/api/v1/agent/sessions/{session_id}/stop",
		"/api/v1/agent/sessions/{session_id}/cancel",
		"/api/v1/agent/sessions/{session_id}/new-task",
		"/api/v1/agent/sessions/{session_id}/stream",
	} {
		if !listOfMapsContains(t, endpoints, "path", path) {
			t.Fatalf("agent capabilities missing endpoint %s: %#v", path, endpoints)
		}
	}
	continueEndpoint := findMapByKey(t, endpoints, "path", "/api/v1/agent/sessions/{session_id}/continue")
	if continueEndpoint["method"] != http.MethodPost || stringValue(t, continueEndpoint["description"]) == "" {
		t.Fatalf("continue endpoint metadata mismatch: %#v", continueEndpoint)
	}
	stateEndpoint := findMapByKey(t, endpoints, "path", "/api/v1/agent/sessions/{session_id}/state")
	if stateEndpoint["method"] != http.MethodGet || !strings.Contains(stringValue(t, stateEndpoint["description"]), "state") {
		t.Fatalf("state endpoint metadata mismatch: %#v", stateEndpoint)
	}

	capabilityItems := asList(t, data["capabilities"])
	if !listOfMapsContains(t, capabilityItems, "id", "planner_tool_loop") {
		t.Fatalf("agent capabilities missing planner_tool_loop: %#v", capabilityItems)
	}

	tools := asList(t, data["builtin_tools"])
	if !listOfMapsContains(t, tools, "name", "workspace.command") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.write") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.patch") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.diff") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.diff_many") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.rebase") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.apply") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.discard") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.delete") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.move") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.copy") ||
		!listOfMapsContains(t, tools, "name", "workspace.files.mkdir") ||
		!listOfMapsContains(t, tools, "name", "runs.approvals.list") ||
		!listOfMapsContains(t, tools, "name", "runs.approvals.approve") ||
		!listOfMapsContains(t, tools, "name", "runs.approvals.deny") ||
		!listOfMapsContains(t, tools, "name", "runs.resume_approved") ||
		!listOfMapsContains(t, tools, "name", "runs.context") ||
		!listOfMapsContains(t, tools, "name", "daemon.health") {
		t.Fatalf("agent builtin tools missing required tools: %#v", tools)
	}
	approveTool := findMapByKey(t, tools, "name", "runs.approvals.approve")
	if approveTool["risk"] != "medium" || boolValue(t, approveTool["auto_run"]) {
		t.Fatalf("approval write tool should be medium risk and not auto-run: %#v", approveTool)
	}
	resumeTool := findMapByKey(t, tools, "name", "runs.resume_approved")
	if resumeTool["risk"] != "medium" || boolValue(t, resumeTool["auto_run"]) {
		t.Fatalf("resume-approved tool should be medium risk and not auto-run: %#v", resumeTool)
	}

	safety := asMap(t, data["safety"])
	allowlist := asStringList(t, safety["command_allowlist"])
	if !stringListContains(allowlist, "pwd") || !stringListContains(allowlist, "git status") {
		t.Fatalf("agent command allowlist missing expected commands: %#v", allowlist)
	}
}

func TestHTTPAgentMessageUsesConfiguredLLMProvider(t *testing.T) {
	var llmRequests atomic.Int32
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmRequests.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("LLM method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("LLM path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-llm-key" {
			t.Errorf("LLM Authorization = %q, want Bearer test-llm-key", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode LLM request: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if body["model"] != "gpt-test" {
			t.Errorf("LLM model = %#v, want gpt-test", body["model"])
		}
		encodedMessages, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(encodedMessages), "authorized web/API security triage") {
			t.Errorf("LLM request did not include user prompt: %s", encodedMessages)
		}
		if !strings.Contains(string(encodedMessages), "answer with LLM agent reply") {
			t.Errorf("LLM request did not include custom instructions: %s", encodedMessages)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"choices":[{"message":{"role":"assistant","content":"LLM agent reply"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5}
		}`))
	}))
	defer llmServer.Close()

	root := t.TempDir()
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-llm-api"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(filepath.Join(root, "runs")),
		approvals: services.NewApprovalService(filepath.Join(root, "runs")),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"en"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	payload, err := json.Marshal(map[string]any{
		"operator":     "tester",
		"language":     "en",
		"message":      "Explain how CyberAgent helps with authorized web/API security triage",
		"provider":     "openai",
		"model":        "gpt-test",
		"api_key":      "test-llm-key",
		"base_url":     llmServer.URL + "/v1",
		"instructions": "answer with LLM agent reply",
	})
	if err != nil {
		t.Fatalf("marshal agent payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK || send.Body.Data["session_id"] != sessionID {
		t.Fatalf("unexpected send agent LLM message response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if llmRequests.Load() != 1 {
		t.Fatalf("LLM server received %d requests, want 1", llmRequests.Load())
	}

	messages := asList(t, send.Body.Data["messages"])
	if len(messages) != 2 || asMap(t, messages[0])["role"] != "user" {
		t.Fatalf("unexpected LLM agent messages: %#v", messages)
	}
	lastMessage := asMap(t, messages[len(messages)-1])
	if lastMessage["role"] != "agent" || !strings.Contains(stringValue(t, lastMessage["content"]), "LLM agent reply") {
		t.Fatalf("unexpected LLM agent reply: %#v", lastMessage)
	}

	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) != 1 {
		t.Fatalf("unexpected LLM tool calls: %#v", toolCalls)
	}
	tool := asMap(t, toolCalls[0])
	if tool["name"] != "LLM chat" || tool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected LLM tool call: %#v", tool)
	}
	result := asMap(t, tool["result"])
	if result["provider"] != "openai" || result["model"] != "gpt-test" {
		t.Fatalf("unexpected LLM tool result: %#v", result)
	}
}

func TestHTTPAgentMessageFallsBackToSecondaryProviderOnTransientError(t *testing.T) {
	var primaryRequests atomic.Int32
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryRequests.Add(1)
		http.Error(w, "upstream temporarily unavailable", http.StatusBadGateway)
	}))
	defer primaryServer.Close()

	var secondaryRequests atomic.Int32
	secondaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryRequests.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer secondary-key" {
			t.Errorf("fallback Authorization = %q, want Bearer secondary-key", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode fallback LLM request: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if body["model"] != "fallback-model" {
			t.Errorf("fallback model = %#v, want fallback-model", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-fallback",
			"choices":[{"message":{"role":"assistant","content":"fallback provider reply"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":7,"completion_tokens":3}
		}`))
	}))
	defer secondaryServer.Close()

	root := t.TempDir()
	providers := newProviderStore(filepath.Join(root, "providers.json"))
	if err := providers.Save(webiface.ProviderConfig{
		ActiveID: "primary",
		Providers: []webiface.ProviderProfile{
			{ID: "primary", Name: "Primary", Type: "openai", Model: "primary-model", APIKey: "primary-key", BaseURL: primaryServer.URL + "/v1"},
			{ID: "secondary", Name: "Secondary", Type: "openai", Model: "fallback-model", APIKey: "secondary-key", BaseURL: secondaryServer.URL + "/v1"},
		},
	}); err != nil {
		t.Fatalf("save provider config: %v", err)
	}

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-provider-fallback"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(filepath.Join(root, "runs")),
		approvals: services.NewApprovalService(filepath.Join(root, "runs")),
		providers: providers,
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"en"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "en",
		"message":  "Use the AI planner and answer through provider fallback",
		"provider": "openai",
		"model":    "primary-model",
		"api_key":  "primary-key",
		"base_url": primaryServer.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("marshal fallback payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK || send.Body.Data["session_id"] != sessionID {
		t.Fatalf("unexpected send fallback response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if primaryRequests.Load() == 0 {
		t.Fatalf("primary provider was not attempted")
	}
	if secondaryRequests.Load() != 1 {
		t.Fatalf("secondary provider requests = %d, want 1", secondaryRequests.Load())
	}

	messages := asList(t, send.Body.Data["messages"])
	lastMessage := asMap(t, messages[len(messages)-1])
	if !strings.Contains(stringValue(t, lastMessage["content"]), "fallback provider reply") {
		t.Fatalf("unexpected fallback agent reply: %#v", lastMessage)
	}
	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) != 1 {
		t.Fatalf("unexpected fallback tool calls: %#v", toolCalls)
	}
	result := asMap(t, asMap(t, toolCalls[0])["result"])
	if result["model"] != "fallback-model" || !boolValue(t, result["provider_fallback"]) {
		t.Fatalf("fallback metadata missing from result: %#v", result)
	}
	attempts := asList(t, result["provider_attempts"])
	if len(attempts) != 1 || stringValue(t, asMap(t, attempts[0])["model"]) != "primary-model" {
		t.Fatalf("unexpected provider attempts metadata: %#v", attempts)
	}
}

func TestHTTPAgentSessionCompactsLongConversationWithMemorySummary(t *testing.T) {
	var llmRequests atomic.Int32
	const anchor = "memory-anchor-early-context"
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmRequests.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("LLM method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("LLM path = %q, want /v1/chat/completions", r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode LLM request: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		encodedMessages, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(encodedMessages), anchor) {
			t.Errorf("LLM request did not include compacted memory anchor: %s", encodedMessages)
		}
		if !strings.Contains(string(encodedMessages), "compressed memory") {
			t.Errorf("LLM request did not include memory summary prompt: %s", encodedMessages)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-memory",
			"choices":[{"message":{"role":"assistant","content":"memory aware reply"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":20,"completion_tokens":5}
		}`))
	}))
	defer llmServer.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	storeDir := filepath.Join(runsDir, ".agent", "sessions")
	now := time.Now().UTC().Add(-2 * time.Hour)
	messages := make([]agentMessage, 0, agentSessionMaxMessages+10)
	for i := 0; i < agentSessionMaxMessages+10; i++ {
		role := "user"
		if i%2 == 1 {
			role = "agent"
		}
		content := fmt.Sprintf("historical message %02d", i)
		if i == 5 {
			content += " " + anchor
		}
		messages = append(messages, agentMessage{
			ID:        fmt.Sprintf("msg_%03d", i),
			Role:      role,
			Content:   content,
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}
	writeAgentSessionFile(t, storeDir, agentSession{
		SessionID: "agent_memory_summary",
		CreatedAt: now,
		UpdatedAt: now.Add(time.Duration(len(messages)) * time.Second),
		Operator:  "tester",
		Language:  "en",
		Status:    "running",
		Messages:  messages,
		ActiveTask: &agentTask{
			ID:        "task_memory",
			Goal:      "preserve long conversation context",
			Kind:      "analysis",
			Status:    "running",
			CreatedAt: now,
			UpdatedAt: now,
		},
	})

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-memory"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "en",
		"message":  "Continue the analysis using our earlier constraints.",
		"provider": "openai",
		"model":    "memory-test",
		"api_key":  "memory-key",
		"base_url": llmServer.URL,
	})
	if err != nil {
		t.Fatalf("marshal memory payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/agent_memory_summary/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK || send.Body.Data["session_id"] != "agent_memory_summary" {
		t.Fatalf("unexpected send memory response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if llmRequests.Load() != 1 {
		t.Fatalf("LLM server received %d requests, want 1", llmRequests.Load())
	}
	sendMemory := asMap(t, send.Body.Data["memory_summary"])
	if !strings.Contains(stringValue(t, sendMemory["summary"]), anchor) || int(sendMemory["message_count"].(float64)) == 0 {
		t.Fatalf("send response memory summary missing compacted anchor: %#v", sendMemory)
	}

	read := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/agent_memory_summary", "test-token")
	if read.StatusCode != http.StatusOK || !read.Body.OK {
		t.Fatalf("unexpected get memory session response: status=%d body=%#v", read.StatusCode, read.Body)
	}
	session := asMap(t, read.Body.Data["session"])
	if got := len(asList(t, session["messages"])); got > agentSessionMaxMessages {
		t.Fatalf("session messages were not compacted: got=%d max=%d", got, agentSessionMaxMessages)
	}
	readMemory := asMap(t, session["memory_summary"])
	if !strings.Contains(stringValue(t, readMemory["summary"]), anchor) {
		t.Fatalf("persisted memory summary missing compacted anchor: %#v", readMemory)
	}

	state := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/agent_memory_summary/state", "test-token")
	if state.StatusCode != http.StatusOK || !state.Body.OK {
		t.Fatalf("unexpected memory state response: status=%d body=%#v", state.StatusCode, state.Body)
	}
	if !strings.Contains(stringValue(t, asMap(t, state.Body.Data["memory_summary"])["summary"]), anchor) {
		t.Fatalf("state payload missing memory summary: %#v", state.Body.Data)
	}
	contextPayload := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/agent_memory_summary/context", "test-token")
	if contextPayload.StatusCode != http.StatusOK || !contextPayload.Body.OK {
		t.Fatalf("unexpected memory context response: status=%d body=%#v", contextPayload.StatusCode, contextPayload.Body)
	}
	if !strings.Contains(stringValue(t, asMap(t, contextPayload.Body.Data["memory_summary"])["summary"]), anchor) {
		t.Fatalf("context payload missing memory summary: %#v", contextPayload.Body.Data)
	}

	list := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions?limit=10", "test-token")
	if list.StatusCode != http.StatusOK || !list.Body.OK {
		t.Fatalf("unexpected memory session list response: status=%d body=%#v", list.StatusCode, list.Body)
	}
	items := asList(t, list.Body.Data["items"])
	listed := findMapByKey(t, items, "session_id", "agent_memory_summary")
	listMemory := asMap(t, listed["memory_summary"])
	if !strings.Contains(stringValue(t, listMemory["summary"]), anchor) || int(listMemory["message_count"].(float64)) == 0 {
		t.Fatalf("session list summary missing memory summary: %#v", listed)
	}
}

func TestAgentLLMToolSynthesisHistoryPreservesMemoryWhenTrimming(t *testing.T) {
	const anchor = "memory-anchor-synthesis-trim"
	now := time.Now().UTC()
	history := make([]agentMessage, 0, 40)
	for i := 0; i < 40; i++ {
		role := "user"
		if i%2 == 1 {
			role = "agent"
		}
		history = append(history, agentMessage{
			ID:        fmt.Sprintf("hist_%02d", i),
			Role:      role,
			Content:   fmt.Sprintf("recent history %02d", i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}
	memory := &agentMemorySummary{
		Summary:       "Compacted conversation includes " + anchor,
		MessageCount:  30,
		LastMessageID: "hist_09",
		UpdatedAt:     now,
	}
	toolMessages := []agentMessage{{
		ID:        "tool_01",
		Role:      "tool",
		Content:   "workspace.command observation",
		Timestamp: now.Add(time.Minute),
	}}

	messages := agentLLMToolSynthesisHistory(history, memory, toolMessages, "en")
	if len(messages) > 18 {
		t.Fatalf("synthesis history was not trimmed to max length: got=%d", len(messages))
	}
	if len(messages) == 0 || messages[0].Role != agentllm.RoleUser {
		t.Fatalf("synthesis history should start with a user memory message: %#v", messages)
	}
	var firstText string
	for _, block := range messages[0].Content {
		if text, ok := block.(agentllm.TextBlock); ok {
			firstText += text.Text
		}
	}
	if !strings.Contains(firstText, anchor) {
		t.Fatalf("trimmed synthesis history lost memory anchor: first=%q messages=%#v", firstText, messages)
	}
	var allText strings.Builder
	for _, message := range messages {
		for _, block := range message.Content {
			if text, ok := block.(agentllm.TextBlock); ok {
				allText.WriteString(text.Text)
				allText.WriteByte('\n')
			}
		}
	}
	if !strings.Contains(allText.String(), "Observation 1") {
		t.Fatalf("synthesis history lost latest tool observation: %s", allText.String())
	}
}

func TestHTTPAgentMessageAppliesLLMPlannerState(t *testing.T) {
	var llmRequests atomic.Int32
	plannerContent := `{
			"answer":"已生成 API 安全测试计划。我会先确认授权边界，再梳理 API 资产与风险优先级。",
			"reasoning":"先确认授权边界，再梳理 API 资产，因为后续测试必须限定在授权 scope 内，并按风险优先级推进。",
			"task":{
				"goal":"制定 API 安全测试计划",
			"kind":"planning",
			"status":"running",
			"summary":"先确认授权边界，再安排安全测试步骤。"
		},
		"todos":[
			{"title":"确认授权边界","status":"done","priority":1},
			{"title":"梳理 API 资产","detail":"收集 base URL、认证方式、关键接口和测试账号。","status":"running","priority":2},
			{"title":"制定 safe profile 执行步骤","status":"pending","priority":3}
		],
		"context_refs":[
			{"id":"target:api-service","kind":"target","ref_id":"api-service","label":"API service","to":"/settings","summary":"待补充 API base URL 和 scope"}
		],
		"next_action":{
			"type":"continue",
			"reason":"需要继续补充目标与 scope",
			"suggested_prompt":"补充 API base URL、授权 scope、认证方式和测试账号"
		}
	}`
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmRequests.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("LLM method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("LLM path = %q, want /v1/chat/completions", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode LLM request: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		encodedMessages, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(encodedMessages), "帮我制定 API 安全测试计划") {
			t.Errorf("LLM request did not include planner prompt: %s", encodedMessages)
		}
		if !strings.Contains(string(encodedMessages), "只输出一个 JSON 对象") {
			t.Errorf("LLM request did not include planner system prompt: %s", encodedMessages)
		}

		encodedContent, err := json.Marshal(plannerContent)
		if err != nil {
			t.Errorf("marshal planner content: %v", err)
			http.Error(w, "bad json", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-planner",
			"choices":[{"message":{"role":"assistant","content":` + string(encodedContent) + `},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":20}
		}`))
	}))
	defer llmServer.Close()

	root := t.TempDir()
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-planner-api"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(filepath.Join(root, "runs")),
		approvals: services.NewApprovalService(filepath.Join(root, "runs")),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"message":  "帮我制定 API 安全测试计划",
		"provider": "openai",
		"model":    "planner-test",
		"api_key":  "planner-key",
		"base_url": llmServer.URL,
	})
	if err != nil {
		t.Fatalf("marshal planner payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK || send.Body.Data["session_id"] != sessionID {
		t.Fatalf("unexpected send planner message response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if llmRequests.Load() != 1 {
		t.Fatalf("LLM server received %d requests, want 1", llmRequests.Load())
	}

	messages := asList(t, send.Body.Data["messages"])
	if len(messages) != 2 || asMap(t, messages[0])["role"] != "user" {
		t.Fatalf("unexpected planner messages: %#v", messages)
	}
	lastMessage := asMap(t, messages[len(messages)-1])
	if lastMessage["role"] != "agent" || !strings.Contains(stringValue(t, lastMessage["content"]), "已生成 API 安全测试计划") {
		t.Fatalf("unexpected planner reply: %#v", lastMessage)
	}
	if reasoning := stringValue(t, lastMessage["reasoning"]); !strings.Contains(reasoning, "授权边界") || !strings.Contains(reasoning, "风险优先级") {
		t.Fatalf("planner reasoning was not returned with reply: %#v", lastMessage)
	}

	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) != 1 {
		t.Fatalf("unexpected planner tool calls: %#v", toolCalls)
	}
	tool := asMap(t, toolCalls[0])
	if tool["name"] != "LLM chat" || tool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected planner tool call: %#v", tool)
	}
	result := asMap(t, tool["result"])
	if result["planner"] != true || result["planner_todos"].(float64) != 3 {
		t.Fatalf("LLM tool result did not mark planner metadata: %#v", result)
	}

	activeTask := asMap(t, send.Body.Data["active_task"])
	if activeTask["goal"] != "制定 API 安全测试计划" || activeTask["kind"] != "planning" || activeTask["status"] != "running" {
		t.Fatalf("unexpected planner active task: %#v", activeTask)
	}
	todos := asList(t, send.Body.Data["todos"])
	if len(todos) != 3 {
		t.Fatalf("unexpected planner todos: %#v", todos)
	}
	if first := asMap(t, todos[0]); first["id"] != "todo_01" || first["title"] != "确认授权边界" || first["status"] != "done" {
		t.Fatalf("unexpected first planner todo: %#v", first)
	}
	if second := asMap(t, todos[1]); second["title"] != "梳理 API 资产" || second["status"] != "running" {
		t.Fatalf("unexpected second planner todo: %#v", second)
	}
	nextAction := asMap(t, send.Body.Data["next_action"])
	if nextAction["type"] != "continue" || nextAction["suggested_prompt"] != "补充 API base URL、授权 scope、认证方式和测试账号" {
		t.Fatalf("unexpected planner next action: %#v", nextAction)
	}
	contextRefs := asList(t, send.Body.Data["context_refs"])
	if len(contextRefs) == 0 || asMap(t, contextRefs[0])["id"] != "target:api-service" {
		t.Fatalf("unexpected planner context refs: %#v", contextRefs)
	}

	read := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if read.StatusCode != http.StatusOK || !read.Body.OK {
		t.Fatalf("unexpected persisted planner session response: status=%d body=%#v", read.StatusCode, read.Body)
	}
	persisted := asMap(t, read.Body.Data["session"])
	persistedTask := asMap(t, persisted["active_task"])
	if persistedTask["goal"] != "制定 API 安全测试计划" {
		t.Fatalf("planner task was not persisted: %#v", persistedTask)
	}
	persistedTodos := asList(t, persisted["todos"])
	if len(persistedTodos) != 3 || asMap(t, persistedTodos[1])["status"] != "running" {
		t.Fatalf("planner todos were not persisted: %#v", persistedTodos)
	}
	persistedNextAction := asMap(t, persisted["next_action"])
	if persistedNextAction["suggested_prompt"] != "补充 API base URL、授权 scope、认证方式和测试账号" {
		t.Fatalf("planner next action was not persisted: %#v", persistedNextAction)
	}
	persistedMessages := asList(t, persisted["messages"])
	persistedLastMessage := asMap(t, persistedMessages[len(persistedMessages)-1])
	if reasoning := stringValue(t, persistedLastMessage["reasoning"]); !strings.Contains(reasoning, "授权边界") || !strings.Contains(reasoning, "风险优先级") {
		t.Fatalf("planner reasoning was not persisted: %#v", persistedLastMessage)
	}
}

func TestHTTPAgentLLMPlannerInvokesStructuredToolActions(t *testing.T) {
	var llmRequests atomic.Int32
	plannerContent := `{
		"answer":"我会先执行低风险内置动作，检查当前执行环境与工作空间。",
		"task":{
			"goal":"检查 Agent 工作空间与 daemon 状态",
			"kind":"ops",
			"status":"running",
			"summary":"使用低风险内置工具确认工作目录、文件列表和 daemon 健康状态。"
		},
		"todos":[
			{"title":"确认工作目录","status":"running","priority":1},
			{"title":"列出工作空间文件","status":"pending","priority":2},
			{"title":"检查 daemon 健康状态","status":"pending","priority":3}
		],
		"actions":[
			{"name":"workspace.command","arguments":{"command":"pwd"}},
			{"name":"workspace.files.list","arguments":{"path":"."}}
		],
		"tool_calls":[
			{"name":"daemon.health","arguments":{}}
		],
		"next_action":{
			"type":"continue",
			"reason":"工具结果会用于下一步分析。",
			"suggested_prompt":"继续分析这些工具结果"
		}
	}`
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := llmRequests.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("LLM method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("LLM path = %q, want /v1/chat/completions", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode LLM request: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		encodedMessages, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(encodedMessages), "请继续分析当前任务上下文并采取必要的低风险步骤") {
			t.Errorf("LLM request did not include operator prompt: %s", encodedMessages)
		}
		if !strings.Contains(string(encodedMessages), "actions") || !strings.Contains(string(encodedMessages), "workspace.command") {
			t.Errorf("LLM request did not include structured action schema: %s", encodedMessages)
		}

		responseContent := plannerContent
		if requestNumber == 2 {
			if !strings.Contains(string(encodedMessages), "Observation 1") || !strings.Contains(string(encodedMessages), "workspace.command") {
				t.Errorf("LLM synthesis request did not include tool observations: %s", encodedMessages)
			}
			responseContent = `{
					"answer":"工具执行完成：已确认工作目录、列出工作空间文件，并检查 daemon 健康状态。下一步可以自动继续读取最近运行，补充安全分析上下文。",
					"task":{
						"goal":"检查 Agent 工作空间与 daemon 状态",
						"kind":"ops",
						"status":"running",
						"summary":"低风险内置工具检查完成，继续收集最近运行上下文。"
					},
					"todos":[
						{"title":"确认工作目录","status":"done","priority":1},
						{"title":"列出工作空间文件","status":"done","priority":2},
						{"title":"检查 daemon 健康状态","status":"done","priority":3},
						{"title":"读取最近运行上下文","status":"pending","priority":4}
					],
					"next_action":{
						"type":"continue",
						"reason":"当前 observation 已足够支持继续执行低风险上下文读取。",
						"suggested_prompt":"继续读取最近运行上下文并总结下一步",
						"auto_continue":true
					}
				}`
		}

		encodedContent, err := json.Marshal(responseContent)
		if err != nil {
			t.Errorf("marshal planner content: %v", err)
			http.Error(w, "bad json", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-actions",
			"choices":[{"message":{"role":"assistant","content":` + string(encodedContent) + `},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":12,"completion_tokens":30}
		}`))
	}))
	defer llmServer.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-actions"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"message":  "请继续分析当前任务上下文并采取必要的低风险步骤",
		"provider": "openai",
		"model":    "planner-actions-test",
		"api_key":  "planner-actions-key",
		"base_url": llmServer.URL,
	})
	if err != nil {
		t.Fatalf("marshal planner actions payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK || send.Body.Data["session_id"] != sessionID {
		t.Fatalf("unexpected send planner actions response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if llmRequests.Load() != 2 {
		t.Fatalf("LLM server received %d requests, want 2", llmRequests.Load())
	}

	messages := asList(t, send.Body.Data["messages"])
	if len(messages) != 6 || asMap(t, messages[0])["role"] != "user" {
		t.Fatalf("expected user, planner, three tool messages, and synthesis: %#v", messages)
	}
	if planner := asMap(t, messages[1]); planner["role"] != "agent" || !strings.Contains(stringValue(t, planner["content"]), "低风险内置动作") {
		t.Fatalf("unexpected planner action message: %#v", planner)
	}
	if synthesis := asMap(t, messages[len(messages)-1]); synthesis["role"] != "agent" || !strings.Contains(stringValue(t, synthesis["content"]), "工具执行完成") {
		t.Fatalf("unexpected synthesis message: %#v", synthesis)
	}

	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) != 5 {
		t.Fatalf("expected LLM planner, three structured tool calls, and synthesis: %#v", toolCalls)
	}
	seenTools := map[string]map[string]any{}
	for _, raw := range toolCalls {
		tool := asMap(t, raw)
		seenTools[stringValue(t, tool["name"])] = tool
		if tool["status"] != string(contracts.StatusSucceeded) {
			t.Fatalf("structured tool should succeed: %#v", tool)
		}
	}
	for _, name := range []string{"LLM chat", "workspace.command", "workspace.files.list", "GET /api/v1/doctor", "LLM synthesis"} {
		if _, ok := seenTools[name]; !ok {
			t.Fatalf("missing structured tool call %q in %#v", name, toolCalls)
		}
	}
	for _, name := range []string{"LLM chat", "workspace.command", "workspace.files.list", "GET /api/v1/doctor", "LLM synthesis"} {
		metadata := asMap(t, seenTools[name]["metadata"])
		if stringValue(t, metadata["risk"]) == "" || stringValue(t, metadata["phase"]) == "" {
			t.Fatalf("tool metadata missing risk/phase for %s: %#v", name, metadata)
		}
		_ = boolValue(t, metadata["auto_run"])
		_ = boolValue(t, metadata["approval_required"])
	}
	plannerMetadata := asMap(t, seenTools["LLM chat"]["metadata"])
	if plannerMetadata["phase"] != "planner" || plannerMetadata["risk"] != "low" || !boolValue(t, plannerMetadata["auto_run"]) {
		t.Fatalf("unexpected planner tool metadata: %#v", plannerMetadata)
	}
	commandMetadata := asMap(t, seenTools["workspace.command"]["metadata"])
	if commandMetadata["phase"] != "workspace" || commandMetadata["risk"] != "low" || !boolValue(t, commandMetadata["auto_run"]) || boolValue(t, commandMetadata["approval_required"]) {
		t.Fatalf("unexpected workspace command metadata: %#v", commandMetadata)
	}
	listMetadata := asMap(t, seenTools["workspace.files.list"]["metadata"])
	healthMetadata := asMap(t, seenTools["GET /api/v1/doctor"]["metadata"])
	queueID := stringValue(t, commandMetadata["action_queue_id"])
	if queueID == "" ||
		stringValue(t, listMetadata["action_queue_id"]) != queueID ||
		stringValue(t, healthMetadata["action_queue_id"]) != queueID {
		t.Fatalf("structured action queue id mismatch: command=%#v list=%#v health=%#v", commandMetadata, listMetadata, healthMetadata)
	}
	if int(commandMetadata["action_index"].(float64)) != 1 ||
		int(listMetadata["action_index"].(float64)) != 2 ||
		int(healthMetadata["action_index"].(float64)) != 3 ||
		int(commandMetadata["action_count"].(float64)) != 3 ||
		int(listMetadata["action_count"].(float64)) != 3 ||
		int(healthMetadata["action_count"].(float64)) != 3 {
		t.Fatalf("structured action queue ordinal mismatch: command=%#v list=%#v health=%#v", commandMetadata, listMetadata, healthMetadata)
	}
	if commandMetadata["action_name"] != "workspace.command" ||
		listMetadata["action_name"] != "workspace.files.list" ||
		healthMetadata["action_name"] != "daemon.health" {
		t.Fatalf("structured action names mismatch: command=%#v list=%#v health=%#v", commandMetadata, listMetadata, healthMetadata)
	}
	llmResult := asMap(t, seenTools["LLM chat"]["result"])
	if llmResult["planner"] != true || llmResult["planner_actions"].(float64) != 3 {
		t.Fatalf("LLM planner metadata should record structured actions: %#v", llmResult)
	}
	commandResult := asMap(t, seenTools["workspace.command"]["result"])
	if commandResult["command"] != "pwd" || int(commandResult["exit_code"].(float64)) != 0 || stringValue(t, commandResult["workdir"]) == "" {
		t.Fatalf("unexpected structured command result: %#v", commandResult)
	}
	listResult := asMap(t, seenTools["workspace.files.list"]["result"])
	if listResult["path"] != "." {
		t.Fatalf("unexpected structured workspace list result: %#v", listResult)
	}
	healthResult := asMap(t, seenTools["GET /api/v1/doctor"]["result"])
	if healthResult["version"] != "vagent-actions" {
		t.Fatalf("unexpected structured health result: %#v", healthResult)
	}
	synthesisMetadata := asMap(t, seenTools["LLM synthesis"]["metadata"])
	if synthesisMetadata["phase"] != "tool_synthesis" || synthesisMetadata["risk"] != "low" || !boolValue(t, synthesisMetadata["auto_run"]) {
		t.Fatalf("unexpected synthesis metadata: %#v", synthesisMetadata)
	}
	nextAction := asMap(t, send.Body.Data["next_action"])
	if nextAction["type"] != "continue" ||
		nextAction["suggested_prompt"] != "继续读取最近运行上下文并总结下一步" ||
		!boolValue(t, nextAction["auto_continue"]) {
		t.Fatalf("synthesis auto-continue next action not returned: %#v", nextAction)
	}

	persisted := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if persisted.StatusCode != http.StatusOK || !persisted.Body.OK {
		t.Fatalf("unexpected persisted actions session response: status=%d body=%#v", persisted.StatusCode, persisted.Body)
	}
	session := asMap(t, persisted.Body.Data["session"])
	persistedMessages := asList(t, session["messages"])
	if len(persistedMessages) < 6 {
		t.Fatalf("persisted session should include welcome, user, planner, and tool messages: %#v", persistedMessages)
	}
	persistedToolCalls := asList(t, session["tool_calls"])
	if len(persistedToolCalls) < 5 {
		t.Fatalf("persisted session should keep structured tool calls: %#v", persistedToolCalls)
	}
	persistedCommandMetadata := asMap(t, findMapByKey(t, persistedToolCalls, "name", "workspace.command")["metadata"])
	persistedListMetadata := asMap(t, findMapByKey(t, persistedToolCalls, "name", "workspace.files.list")["metadata"])
	persistedHealthMetadata := asMap(t, findMapByKey(t, persistedToolCalls, "name", "GET /api/v1/doctor")["metadata"])
	if stringValue(t, persistedCommandMetadata["action_queue_id"]) != queueID ||
		stringValue(t, persistedListMetadata["action_queue_id"]) != queueID ||
		stringValue(t, persistedHealthMetadata["action_queue_id"]) != queueID ||
		int(persistedCommandMetadata["action_index"].(float64)) != 1 ||
		int(persistedListMetadata["action_index"].(float64)) != 2 ||
		int(persistedHealthMetadata["action_index"].(float64)) != 3 {
		t.Fatalf("persisted action queue metadata mismatch: command=%#v list=%#v health=%#v", persistedCommandMetadata, persistedListMetadata, persistedHealthMetadata)
	}
	persistedNextAction := asMap(t, session["next_action"])
	if persistedNextAction["type"] != "continue" ||
		persistedNextAction["suggested_prompt"] != "继续读取最近运行上下文并总结下一步" ||
		!boolValue(t, persistedNextAction["auto_continue"]) {
		t.Fatalf("synthesis auto-continue next action was not persisted: %#v", persistedNextAction)
	}
}

func TestHTTPAgentLLMPlannerInvokesRunContextTool(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	runID := "job_planner_context_001"
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	writeDaemonRunJob(t, runsDir, runID, contracts.StatusSucceeded, now, now.Add(time.Minute))
	writeDaemonRunReport(t, runsDir, runID, now)

	var llmRequests atomic.Int32
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := llmRequests.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("LLM method = %s, want POST", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode LLM request: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		encoded, _ := json.Marshal(body)
		if requestNumber == 1 && !strings.Contains(string(encoded), "runs.context") {
			t.Errorf("planner request should advertise runs.context tool: %s", encoded)
		}
		responseContent := fmt.Sprintf(`{
			"phase":"plan",
			"answer":"我会读取该运行的 findings 上下文，再基于证据总结。",
			"task":{"goal":"读取运行上下文","kind":"analysis","status":"running","summary":"读取 findings 上下文。"},
			"todos":[{"title":"读取 findings 上下文","status":"running","priority":1}],
			"actions":[{"name":"runs.context","arguments":{"run_id":%q,"context":"findings"}}],
			"next_action":{"type":"continue","reason":"需要先读取运行上下文","auto_continue":true}
		}`, runID)
		if requestNumber == 2 {
			encodedMessages, _ := json.Marshal(body["messages"])
			if !strings.Contains(string(encodedMessages), "Observation 1") || !strings.Contains(string(encodedMessages), "Missing CSP") {
				t.Errorf("synthesis request missing run context observation: %s", encodedMessages)
			}
			responseContent = `{
				"phase":"final_answer",
				"answer":"已读取 findings 上下文，关键发现是 Missing CSP，并可按修复建议补充 CSP 后复测。",
				"task":{"goal":"读取运行上下文","kind":"analysis","status":"done","summary":"已完成 findings 分析。"},
				"todos":[{"title":"读取 findings 上下文","status":"done","priority":1}],
				"actions":[],
				"next_action":{"type":"wait_user","reason":"等待用户要求生成报告或复测。"}
			}`
		}
		encodedContent, err := json.Marshal(responseContent)
		if err != nil {
			t.Errorf("marshal planner content: %v", err)
			http.Error(w, "bad json", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-run-context",
			"choices":[{"message":{"role":"assistant","content":` + string(encodedContent) + `},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":20,"completion_tokens":30}
		}`))
	}))
	defer llmServer.Close()

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-run-context"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])
	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"message":  "请读取运行 " + runID + " 的 findings 上下文并总结",
		"provider": "openai",
		"model":    "planner-run-context-test",
		"api_key":  "planner-run-context-key",
		"base_url": llmServer.URL,
	})
	if err != nil {
		t.Fatalf("marshal run context planner payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK {
		t.Fatalf("unexpected run context planner response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if llmRequests.Load() != 2 {
		t.Fatalf("LLM server received %d requests, want 2", llmRequests.Load())
	}
	toolCalls := asList(t, send.Body.Data["tool_calls"])
	contextTool := findMapByKey(t, toolCalls, "name", "GET /api/v1/runs/{run_id}/context")
	if contextTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("runs.context action should execute run context tool: %#v", contextTool)
	}
	result := asMap(t, contextTool["result"])
	if result["run_id"] != runID || result["context"] != "findings" || int(result["finding_count"].(float64)) < 1 {
		t.Fatalf("unexpected run context result: %#v", result)
	}
	messages := asList(t, send.Body.Data["messages"])
	last := asMap(t, messages[len(messages)-1])
	if last["phase"] != "final_answer" || !strings.Contains(stringValue(t, last["content"]), "Missing CSP") {
		t.Fatalf("unexpected run context final answer: %#v", last)
	}
}

func TestHTTPAgentLLMNativeToolCallsAreExecutedAndSynthesized(t *testing.T) {
	var llmRequests atomic.Int32
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := llmRequests.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("LLM method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("LLM path = %q, want /v1/chat/completions", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode LLM request: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}

		encodedTools, _ := json.Marshal(body["tools"])
		if requestNumber == 1 {
			if !strings.Contains(string(encodedTools), "workspace.command") || !strings.Contains(string(encodedTools), "daemon.health") {
				t.Errorf("LLM request did not include native tool definitions: %s", encodedTools)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"chatcmpl-native-tools",
				"choices":[{
					"message":{
						"role":"assistant",
						"content":"我先检查工作目录。",
						"tool_calls":[{
							"id":"call_pwd",
							"type":"function",
							"function":{"name":"workspace.command","arguments":"{\"command\":\"pwd\"}"}
						}]
					},
					"finish_reason":"tool_calls"
				}],
				"usage":{"prompt_tokens":14,"completion_tokens":7}
			}`))
			return
		}

		encodedMessages, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(encodedMessages), "Observation 1") || !strings.Contains(string(encodedMessages), "workspace.command") {
			t.Errorf("LLM synthesis request did not include native tool observation: %s", encodedMessages)
		}
		synthesisContent := `{
			"answer":"原生工具调用已完成：当前工作目录可用，后续可以继续读取工作空间文件或制定授权测试计划。",
			"task":{"goal":"检查工作空间","kind":"ops","status":"done","summary":"已通过原生 tool_calls 执行低风险检查。"},
			"todos":[{"title":"检查工作目录","status":"done","priority":1}],
			"next_action":{"type":"finished","reason":"工作目录检查完成。","suggested_prompt":"列出工作空间文件","auto_continue":false}
		}`
		encodedContent, err := json.Marshal(synthesisContent)
		if err != nil {
			t.Errorf("marshal synthesis content: %v", err)
			http.Error(w, "bad json", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-native-synthesis",
			"choices":[{"message":{"role":"assistant","content":` + string(encodedContent) + `},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":18,"completion_tokens":16}
		}`))
	}))
	defer llmServer.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-native-tools"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"message":  "请先确认当前上下文是否可用",
		"provider": "openai",
		"model":    "native-tools-test",
		"api_key":  "native-tools-key",
		"base_url": llmServer.URL,
	})
	if err != nil {
		t.Fatalf("marshal native tools payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK {
		t.Fatalf("unexpected native tools response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if llmRequests.Load() != 2 {
		t.Fatalf("LLM server received %d requests, want 2", llmRequests.Load())
	}

	messages := asList(t, send.Body.Data["messages"])
	if len(messages) != 4 || asMap(t, messages[0])["role"] != "user" {
		t.Fatalf("expected user, native planner, tool, and synthesis messages: %#v", messages)
	}
	if synthesis := asMap(t, messages[len(messages)-1]); synthesis["role"] != "agent" || !strings.Contains(stringValue(t, synthesis["content"]), "原生工具调用已完成") {
		t.Fatalf("unexpected native synthesis message: %#v", synthesis)
	}

	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) != 3 {
		t.Fatalf("expected LLM chat, workspace.command, and LLM synthesis tool calls: %#v", toolCalls)
	}
	seenTools := map[string]map[string]any{}
	for _, raw := range toolCalls {
		tool := asMap(t, raw)
		seenTools[stringValue(t, tool["name"])] = tool
		if tool["status"] != string(contracts.StatusSucceeded) {
			t.Fatalf("native tool flow should succeed: %#v", tool)
		}
	}
	llmResult := asMap(t, seenTools["LLM chat"]["result"])
	if llmResult["planner_native_tool_calls"].(float64) != 1 || llmResult["planner_actions"].(float64) != 1 {
		t.Fatalf("LLM native tool metadata missing: %#v", llmResult)
	}
	commandResult := asMap(t, seenTools["workspace.command"]["result"])
	if commandResult["command"] != "pwd" || int(commandResult["exit_code"].(float64)) != 0 {
		t.Fatalf("unexpected native command result: %#v", commandResult)
	}
	nativePlannerMetadata := asMap(t, seenTools["LLM chat"]["metadata"])
	if nativePlannerMetadata["phase"] != "planner" ||
		nativePlannerMetadata["risk"] != "low" ||
		!boolValue(t, nativePlannerMetadata["auto_run"]) ||
		boolValue(t, nativePlannerMetadata["approval_required"]) {
		t.Fatalf("unexpected native planner metadata: %#v", nativePlannerMetadata)
	}
	nativeCommandMetadata := asMap(t, seenTools["workspace.command"]["metadata"])
	if nativeCommandMetadata["phase"] != "workspace" ||
		nativeCommandMetadata["risk"] != "low" ||
		!boolValue(t, nativeCommandMetadata["auto_run"]) ||
		boolValue(t, nativeCommandMetadata["approval_required"]) {
		t.Fatalf("unexpected native command metadata: %#v", nativeCommandMetadata)
	}
	synthesisMetadata := asMap(t, seenTools["LLM synthesis"]["metadata"])
	if synthesisMetadata["phase"] != "tool_synthesis" ||
		synthesisMetadata["risk"] != "low" ||
		!boolValue(t, synthesisMetadata["auto_run"]) ||
		boolValue(t, synthesisMetadata["approval_required"]) {
		t.Fatalf("unexpected native synthesis metadata: %#v", synthesisMetadata)
	}
}

func TestHTTPAgentLLMPlannerMutatingWorkspaceActionRequiresExplicitIntent(t *testing.T) {
	var llmRequests atomic.Int32
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := llmRequests.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("LLM method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("LLM path = %q, want /v1/chat/completions", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode LLM request: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		encodedMessages, _ := json.Marshal(body["messages"])
		if requestNumber == 1 {
			if !strings.Contains(string(encodedMessages), "请先确认当前上下文是否可用") {
				t.Errorf("LLM planner request did not include operator prompt: %s", encodedMessages)
			}
			plannerContent := `{
				"answer":"我会顺手写入一个说明文件。",
				"task":{"goal":"检查上下文","kind":"ops","status":"running","summary":"确认当前上下文并记录说明。"},
				"todos":[{"title":"确认上下文","status":"running","priority":1}],
				"actions":[
					{"name":"workspace.files.write","arguments":{"path":"llm-auto.md","content":"should not be written"}}
				],
				"next_action":{"type":"continue","reason":"需要观察工具结果。","suggested_prompt":"总结策略拒绝结果","auto_continue":true}
			}`
			encodedContent, err := json.Marshal(plannerContent)
			if err != nil {
				t.Errorf("marshal planner content: %v", err)
				http.Error(w, "bad json", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"chatcmpl-mutating-policy-planner",
				"choices":[{"message":{"role":"assistant","content":` + string(encodedContent) + `},"finish_reason":"stop"}],
				"usage":{"prompt_tokens":13,"completion_tokens":24}
			}`))
			return
		}

		t.Errorf("unexpected extra LLM request after authorization gate; messages=%s", encodedMessages)
		http.Error(w, "unexpected extra request", http.StatusBadRequest)
	}))
	defer llmServer.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-mutating-policy"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"message":  "请先确认当前上下文是否可用",
		"provider": "openai",
		"model":    "planner-mutating-policy-test",
		"api_key":  "planner-mutating-policy-key",
		"base_url": llmServer.URL,
	})
	if err != nil {
		t.Fatalf("marshal planner mutating policy payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK {
		t.Fatalf("unexpected planner mutating policy response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if llmRequests.Load() != 1 {
		t.Fatalf("LLM server received %d requests, want 1 before authorization", llmRequests.Load())
	}

	messages := asList(t, send.Body.Data["messages"])
	if len(messages) != 3 || asMap(t, messages[0])["role"] != "user" {
		t.Fatalf("expected user, planner, and authorization gate messages: %#v", messages)
	}
	if authMessage := asMap(t, messages[len(messages)-1]); authMessage["role"] != "agent" || !strings.Contains(stringValue(t, authMessage["content"]), "需要授权确认") {
		t.Fatalf("unexpected authorization message: %#v", authMessage)
	}

	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) != 1 {
		t.Fatalf("expected only LLM chat before authorization: %#v", toolCalls)
	}
	if asMap(t, toolCalls[0])["name"] != "LLM chat" {
		t.Fatalf("unexpected pre-authorization tool calls: %#v", toolCalls)
	}
	nextAction := asMap(t, send.Body.Data["next_action"])
	if nextAction["type"] != "authorization_required" {
		t.Fatalf("policy gate should pause at authorization_required next action: %#v", nextAction)
	}
	pendingAuth := asMap(t, send.Body.Data["pending_authorization"])
	if pendingAuth["action_name"] != "workspace.files.write" ||
		pendingAuth["policy"] != "explicit_operator_intent_required" ||
		pendingAuth["status"] != "pending" {
		t.Fatalf("pending authorization should preserve exact planner action: %#v", pendingAuth)
	}
	if send.Body.Data["status"] == "running" {
		t.Fatalf("authorization gate should not continue running before approval: %#v", send.Body.Data)
	}

	writtenPath := filepath.Join(runsDir, ".agent", "workspaces", sessionID, "workspace", "llm-auto.md")
	if _, err := os.Stat(writtenPath); !os.IsNotExist(err) {
		t.Fatalf("workspace file should not be written at %s; stat err=%v", writtenPath, err)
	}

	approve := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/authorization/approve",
		"test-token",
		`{"operator":"tester","language":"zh","operation_level":"workspace_write","skip_synthesis":true}`,
	)
	if approve.StatusCode != http.StatusOK || !approve.Body.OK {
		t.Fatalf("unexpected authorization approve response: status=%d body=%#v", approve.StatusCode, approve.Body)
	}
	if llmRequests.Load() != 1 {
		t.Fatalf("approve with skip_synthesis should not call LLM again; got %d requests", llmRequests.Load())
	}
	approveSession := asMap(t, approve.Body.Data["session"])
	if _, ok := approveSession["pending_authorization"]; ok {
		t.Fatalf("approve should clear pending authorization: %#v", approveSession["pending_authorization"])
	}
	approvedTools := asList(t, approveSession["tool_calls"])
	approvedWrite := findMapByKey(t, approvedTools, "name", "workspace.files.write")
	if approvedWrite["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("approved workspace write should execute successfully: %#v", approvedWrite)
	}
	if data, err := os.ReadFile(writtenPath); err != nil || string(data) != "should not be written" {
		t.Fatalf("approved workspace file content mismatch: data=%q err=%v", string(data), err)
	}

	persisted := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if persisted.StatusCode != http.StatusOK || !persisted.Body.OK {
		t.Fatalf("unexpected persisted policy session response: status=%d body=%#v", persisted.StatusCode, persisted.Body)
	}
	session := asMap(t, persisted.Body.Data["session"])
	if _, ok := session["pending_authorization"]; ok {
		t.Fatalf("persisted session should not keep pending authorization after approve: %#v", session["pending_authorization"])
	}
	persistedToolCalls := asList(t, session["tool_calls"])
	persistedWrite := findMapByKey(t, persistedToolCalls, "name", "workspace.files.write")
	if persistedWrite["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("persisted workspace write should stay succeeded after approve: %#v", persistedWrite)
	}
}

func TestHTTPAgentRoutedToolsAreSynthesizedWhenProviderConfigured(t *testing.T) {
	var llmRequests atomic.Int32
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llmRequests.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("LLM method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("LLM path = %q, want /v1/chat/completions", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode LLM request: %v", err)
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		encodedMessages, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(encodedMessages), "Observation 1") ||
			!strings.Contains(string(encodedMessages), "workspace.command") ||
			!strings.Contains(string(encodedMessages), "pwd") ||
			!strings.Contains(string(encodedMessages), "ls -la") {
			t.Errorf("LLM routed synthesis request missing command observations: %s", encodedMessages)
		}
		synthesisContent := `{
			"answer":"我已观察顺序命令结果：pwd 确认了工作目录，ls -la 返回了当前工作区文件列表。下一步可读取关键文件或继续生成测试计划。",
			"task":{"goal":"检查工作区结构","kind":"ops","status":"running","summary":"已完成低风险命令观察并生成下一步建议。"},
			"todos":[
				{"title":"执行 pwd","status":"done","priority":1},
				{"title":"执行 ls -la","status":"done","priority":2},
				{"title":"读取关键文件或生成测试计划","status":"pending","priority":3}
			],
			"next_action":{"type":"continue","reason":"可继续低风险工作区分析。","suggested_prompt":"读取关键工作区文件并总结下一步","auto_continue":true}
		}`
		encodedContent, err := json.Marshal(synthesisContent)
		if err != nil {
			t.Errorf("marshal routed synthesis content: %v", err)
			http.Error(w, "bad json", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-routed-synthesis",
			"choices":[{"message":{"role":"assistant","content":` + string(encodedContent) + `},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":18,"completion_tokens":16}
		}`))
	}))
	defer llmServer.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-routed-synthesis"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-routed-synthesis"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	payload, err := json.Marshal(map[string]any{
		"operator": "tester",
		"language": "zh",
		"message":  "检查当前工作区目录结构，先执行 pwd 和 ls -la，再总结关键文件",
		"provider": "openai",
		"model":    "routed-synthesis-test",
		"api_key":  "routed-synthesis-key",
		"base_url": llmServer.URL,
	})
	if err != nil {
		t.Fatalf("marshal routed synthesis payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK {
		t.Fatalf("unexpected routed synthesis response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if llmRequests.Load() != 1 {
		t.Fatalf("LLM server received %d requests, want 1", llmRequests.Load())
	}
	toolCalls := asList(t, send.Body.Data["tool_calls"])
	if len(toolCalls) != 3 {
		t.Fatalf("expected two routed command tools plus synthesis: %#v", toolCalls)
	}
	seenTools := map[string]int{}
	for _, raw := range toolCalls {
		tool := asMap(t, raw)
		seenTools[stringValue(t, tool["name"])]++
		if tool["status"] != string(contracts.StatusSucceeded) {
			t.Fatalf("routed synthesis flow should succeed: %#v", tool)
		}
	}
	if seenTools["workspace.command"] != 2 || seenTools["LLM synthesis"] != 1 {
		t.Fatalf("unexpected routed synthesis tools: %#v", toolCalls)
	}
	messages := asList(t, send.Body.Data["messages"])
	if len(messages) != 3 || asMap(t, messages[0])["role"] != "user" {
		t.Fatalf("expected user, command summary, and synthesis messages: %#v", messages)
	}
	if synthesis := asMap(t, messages[len(messages)-1]); synthesis["role"] != "agent" || !strings.Contains(stringValue(t, synthesis["content"]), "我已观察顺序命令结果") {
		t.Fatalf("unexpected routed synthesis message: %#v", synthesis)
	}
	nextAction := asMap(t, send.Body.Data["next_action"])
	if nextAction["type"] != "continue" ||
		nextAction["suggested_prompt"] != "读取关键工作区文件并总结下一步" ||
		!boolValue(t, nextAction["auto_continue"]) {
		t.Fatalf("routed synthesis should persist auto-continue next action: %#v", nextAction)
	}
}

func TestHTTPAgentHTTPRequestUsesAuthProfileWithoutLeakingSecret(t *testing.T) {
	const secretToken = "secret-token-for-api-profile"
	var targetRequests atomic.Int32
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+secretToken {
			t.Errorf("Authorization header = %q, want bearer profile", r.Header.Get("Authorization"))
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer targetServer.Close()

	var llmRequests atomic.Int32
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := llmRequests.Add(1)
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read LLM body: %v", err)
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if strings.Contains(string(rawBody), secretToken) {
			t.Errorf("LLM request leaked auth secret: %s", rawBody)
		}
		var content string
		if count == 1 {
			content = fmt.Sprintf(`{
				"phase":"plan",
				"answer":"我将使用配置好的 API auth profile 对授权目标发起 GET 请求。",
				"task":{"goal":"验证 API 授权请求","kind":"ops","status":"running","summary":"准备调用 http.request。"},
				"actions":[{"name":"http.request","arguments":{"url":%q,"method":"GET","auth_profile_id":"api-auth"}}],
				"next_action":{"type":"continue","reason":"请求执行后需要总结响应。","suggested_prompt":"总结 API 响应","auto_continue":true}
			}`, targetServer.URL+"/api/me")
		} else {
			content = `{
				"phase":"final_answer",
				"answer":"已完成授权 API 请求，响应为 200 OK，且请求头中的凭据已脱敏保存。",
				"task":{"goal":"验证 API 授权请求","kind":"ops","status":"done","summary":"http.request 已成功使用 auth profile。"},
				"todos":[{"title":"执行授权 GET 请求","status":"done","priority":1}],
				"next_action":{"type":"finished","reason":"目标请求已完成。","auto_continue":false}
			}`
		}
		encodedContent, err := json.Marshal(content)
		if err != nil {
			t.Errorf("marshal LLM content: %v", err)
			http.Error(w, "bad json", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-auth-profile",
			"choices":[{"message":{"role":"assistant","content":` + string(encodedContent) + `},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":24,"completion_tokens":18}
		}`))
	}))
	defer llmServer.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-auth-profile"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-auth-profile"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh","operation_level":"safe_scan"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	requestPayload := map[string]any{
		"operator":        "tester",
		"language":        "zh",
		"operation_level": "safe_scan",
		"message":         "请 GET 请求授权目标 " + targetServer.URL + "/api/me，并使用 api-auth 授权 profile",
		"provider":        "openai",
		"model":           "auth-profile-test",
		"api_key":         "auth-profile-key",
		"base_url":        llmServer.URL,
		"auth_profiles": []map[string]any{
			{
				"id":       "api-auth",
				"name":     "Demo API",
				"type":     "bearer",
				"base_url": targetServer.URL,
				"token":    secretToken,
				"enabled":  true,
			},
		},
		"default_auth_profile_id": "api-auth",
	}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		t.Fatalf("marshal auth profile payload: %v", err)
	}
	send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
	if send.StatusCode != http.StatusOK || !send.Body.OK {
		t.Fatalf("unexpected auth profile response: status=%d body=%#v", send.StatusCode, send.Body)
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("target server should not be called before authorization; got %d requests", targetRequests.Load())
	}
	if llmRequests.Load() != 1 {
		t.Fatalf("LLM server received %d requests before authorization, want 1", llmRequests.Load())
	}
	pendingAuth := asMap(t, send.Body.Data["pending_authorization"])
	if pendingAuth["action_name"] != "http.request" || pendingAuth["status"] != "pending" {
		t.Fatalf("http.request should pause at authorization gate: %#v", pendingAuth)
	}

	approvePayload, err := json.Marshal(requestPayload)
	if err != nil {
		t.Fatalf("marshal auth profile authorize payload: %v", err)
	}
	approve := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/authorization/approve", "test-token", string(approvePayload))
	if approve.StatusCode != http.StatusOK || !approve.Body.OK {
		t.Fatalf("unexpected auth profile authorize response: status=%d body=%#v", approve.StatusCode, approve.Body)
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("target server should be blocked by SSRF guard; got %d requests", targetRequests.Load())
	}
	if llmRequests.Load() != 1 {
		t.Fatalf("LLM server received %d requests after blocked tool, want 1", llmRequests.Load())
	}
	encodedResponse, _ := json.Marshal(approve.Body.Data)
	if strings.Contains(string(encodedResponse), secretToken) {
		t.Fatalf("agent response leaked auth secret: %s", encodedResponse)
	}
	toolCalls := asList(t, asMap(t, approve.Body.Data["session"])["tool_calls"])
	httpTool := findMapByKey(t, toolCalls, "name", "http.request")
	if httpTool["status"] != string(contracts.StatusFailed) {
		t.Fatalf("http.request should fail on blocked loopback target: %#v", httpTool)
	}
	toolError := asMap(t, httpTool["error"])
	if toolError["code"] != string(services.CodePolicyDenied) || !strings.Contains(stringValue(t, toolError["message"]), "non-public ip") {
		t.Fatalf("http.request should report SSRF policy denial: %#v", toolError)
	}
}

func TestNormalizeAgentProviderBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		raw      string
		want     string
	}{
		{
			name:     "openai root",
			provider: "openai",
			raw:      "http://127.0.0.1:8080",
			want:     "http://127.0.0.1:8080/v1/chat/completions",
		},
		{
			name:     "openai v1",
			provider: "openai-compatible",
			raw:      "http://127.0.0.1:8080/v1",
			want:     "http://127.0.0.1:8080/v1/chat/completions",
		},
		{
			name:     "openai prefixed v1",
			provider: "openai-compatible",
			raw:      "http://127.0.0.1:8080/api/v1",
			want:     "http://127.0.0.1:8080/api/v1/chat/completions",
		},
		{
			name:     "openai full path unchanged",
			provider: "openai",
			raw:      "http://127.0.0.1:8080/v1/chat/completions",
			want:     "http://127.0.0.1:8080/v1/chat/completions",
		},
		{
			name:     "anthropic root",
			provider: "anthropic",
			raw:      "http://127.0.0.1:8080",
			want:     "http://127.0.0.1:8080/v1/messages",
		},
		{
			name:     "anthropic v1",
			provider: "anthropic-compatible",
			raw:      "http://127.0.0.1:8080/v1",
			want:     "http://127.0.0.1:8080/v1/messages",
		},
		{
			name:     "anthropic prefixed v1",
			provider: "anthropic-compatible",
			raw:      "http://127.0.0.1:8080/anthropic/v1",
			want:     "http://127.0.0.1:8080/anthropic/v1/messages",
		},
		{
			name:     "anthropic sdk base",
			provider: "anthropic-compatible",
			raw:      "http://127.0.0.1:8080/anthropic",
			want:     "http://127.0.0.1:8080/anthropic/v1/messages",
		},
		{
			name:     "custom path unchanged",
			provider: "openai",
			raw:      "http://127.0.0.1:8080/custom/chat",
			want:     "http://127.0.0.1:8080/custom/chat",
		},
		{
			name:     "invalid URL unchanged",
			provider: "openai",
			raw:      "127.0.0.1:8080/v1",
			want:     "127.0.0.1:8080/v1",
		},
		{
			name:     "empty unchanged",
			provider: "openai",
			raw:      "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAgentProviderBaseURL(tt.provider, tt.raw); got != tt.want {
				t.Fatalf("normalizeAgentProviderBaseURL(%q, %q) = %q, want %q", tt.provider, tt.raw, got, tt.want)
			}
		})
	}
}

func TestHTTPApprovalWriteAPIs(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	writeDaemonApprovalRun(t, runsDir, "run_approval_write", []contracts.HITLRequest{
		daemonTestApproval("approval_001", "job_001", "step_001"),
		daemonTestApproval("approval_002", "job_001", "step_002"),
	})

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vapproval-api"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	approve := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_write/approvals/approval_001/approve", "test-token", `{"operator":"local-user","comment":"approved from http"}`)
	if approve.StatusCode != http.StatusOK || !approve.Body.OK || approve.Body.Data["status"] != string(contracts.HITLApproved) {
		t.Fatalf("unexpected approve response: status=%d body=%#v", approve.StatusCode, approve.Body)
	}
	approveResolution := asMap(t, approve.Body.Data["resolution"])
	if approveResolution["resolved_by"] != "local-user" || approveResolution["comment"] != "approved from http" {
		t.Fatalf("unexpected approve resolution: %#v", approveResolution)
	}

	reapprove := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_write/approvals/approval_001/approve", "test-token", `{"operator":"local-user"}`)
	if reapprove.StatusCode != http.StatusConflict || reapprove.Body.OK || reapprove.Body.Error.Code != services.CodeConflict {
		t.Fatalf("unexpected re-approve response: status=%d body=%#v", reapprove.StatusCode, reapprove.Body)
	}

	deny := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_write/approvals/approval_002/deny", "test-token", `{"operator":"local-user","comment":"not needed"}`)
	if deny.StatusCode != http.StatusOK || !deny.Body.OK || deny.Body.Data["status"] != string(contracts.HITLRejected) {
		t.Fatalf("unexpected deny response: status=%d body=%#v", deny.StatusCode, deny.Body)
	}
	denyResolution := asMap(t, deny.Body.Data["resolution"])
	if denyResolution["resolved_by"] != "local-user" || denyResolution["comment"] != "not needed" {
		t.Fatalf("unexpected deny resolution: %#v", denyResolution)
	}

	list := doDaemonRequest(t, api.URL+"/api/v1/runs/run_approval_write/approvals", "test-token")
	if list.StatusCode != http.StatusOK || !list.Body.OK {
		t.Fatalf("unexpected approval list response: status=%d body=%#v", list.StatusCode, list.Body)
	}
	approvals := asList(t, list.Body.Data["approvals"])
	if len(approvals) != 2 || asMap(t, approvals[0])["status"] != string(contracts.HITLApproved) || asMap(t, approvals[1])["status"] != string(contracts.HITLRejected) {
		t.Fatalf("unexpected persisted approvals: %#v", approvals)
	}

	missing := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_write/approvals/missing/approve", "test-token", `{}`)
	if missing.StatusCode != http.StatusNotFound || missing.Body.OK || missing.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("unexpected missing approval response: status=%d body=%#v", missing.StatusCode, missing.Body)
	}

	badJSON := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_write/approvals/missing/approve", "test-token", `{"operator":`)
	if badJSON.StatusCode != http.StatusBadRequest || badJSON.Body.OK || badJSON.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected bad JSON response: status=%d body=%#v", badJSON.StatusCode, badJSON.Body)
	}

	traversal := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/../secret/approvals/approval_001/approve", "test-token", `{}`)
	if traversal.StatusCode != http.StatusNotFound && traversal.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected traversal response: status=%d body=%#v", traversal.StatusCode, traversal.Body)
	}

	wrongMethod := doDaemonRequestWithBody(t, http.MethodDelete, api.URL+"/api/v1/runs/run_approval_write/approvals/approval_001/approve", "test-token", `{}`)
	if wrongMethod.StatusCode != http.StatusBadRequest || wrongMethod.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected wrong method response: status=%d body=%#v", wrongMethod.StatusCode, wrongMethod.Body)
	}
}

func TestHTTPApprovalCancelAndEditArgsAPIs(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	writeDaemonApprovalRun(t, runsDir, "run_approval_edit", []contracts.HITLRequest{
		daemonTestApproval("approval_cancel", "job_001", "step_cancel"),
		daemonTestApproval("approval_edit", "job_001", "step_edit"),
	})

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vapproval-edit-api"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	cancel := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_edit/approvals/approval_cancel/cancel", "test-token", `{"operator":"local-user","comment":"cancel from http"}`)
	if cancel.StatusCode != http.StatusOK || !cancel.Body.OK || cancel.Body.Data["status"] != string(contracts.HITLCancelled) {
		t.Fatalf("unexpected cancel response: status=%d body=%#v", cancel.StatusCode, cancel.Body)
	}
	cancelResolution := asMap(t, cancel.Body.Data["resolution"])
	if cancelResolution["decision"] != "cancel" || cancelResolution["resolved_by"] != "local-user" || cancelResolution["comment"] != "cancel from http" {
		t.Fatalf("unexpected cancel resolution: %#v", cancelResolution)
	}

	edit := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_edit/approvals/approval_edit/edit-args", "test-token", `{"operator":"local-user","edit_args":{"target":"https://example.com/edited","method":"POST"}}`)
	if edit.StatusCode != http.StatusOK || !edit.Body.OK || edit.Body.Data["status"] != string(contracts.HITLModified) {
		t.Fatalf("unexpected edit-args response: status=%d body=%#v", edit.StatusCode, edit.Body)
	}
	editResolution := asMap(t, edit.Body.Data["resolution"])
	if editResolution["decision"] != "edit_args" || editResolution["resolved_by"] != "local-user" {
		t.Fatalf("unexpected edit-args resolution: %#v", editResolution)
	}
	editArgs := asMap(t, editResolution["edit_args"])
	if editArgs["target"] != "https://example.com/edited" || editArgs["method"] != "POST" {
		t.Fatalf("unexpected edit_args: %#v", editArgs)
	}

	list := doDaemonRequest(t, api.URL+"/api/v1/runs/run_approval_edit/approvals", "test-token")
	if list.StatusCode != http.StatusOK || !list.Body.OK {
		t.Fatalf("unexpected approval list response: status=%d body=%#v", list.StatusCode, list.Body)
	}
	approvals := asList(t, list.Body.Data["approvals"])
	if len(approvals) != 2 || asMap(t, approvals[0])["status"] != string(contracts.HITLCancelled) || asMap(t, approvals[1])["status"] != string(contracts.HITLModified) {
		t.Fatalf("unexpected persisted approvals: %#v", approvals)
	}
}

func TestHTTPResumeApprovedAPI(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte("<urlset></urlset>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	runID := "run_resume_http"
	runDir := filepath.Join(runsDir, runID)
	if _, err := coreruntime.RunWebScan(context.Background(), coreruntime.WebScanRequest{
		Target:          target.URL,
		Scope:           target.URL,
		Out:             runDir,
		Profile:         "safe",
		NoExternalTools: true,
		BuildInfo:       coreruntime.BuildInfo{Version: "v0.1.2-test"},
		CommandName:     "scan web",
	}); err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vresume-api"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	resume := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/"+runID+"/resume-approved", "test-token", `{"operator":"local-user"}`)
	if resume.StatusCode != http.StatusOK || !resume.Body.OK || resume.Body.Data["run_id"] != runID || resume.Body.Data["run_dir"] != runDir {
		t.Fatalf("unexpected resume response: status=%d body=%#v", resume.StatusCode, resume.Body)
	}
	if len(asList(t, resume.Body.Data["resumed"])) != 0 || len(asList(t, resume.Body.Data["skipped"])) != 0 {
		t.Fatalf("unexpected resume result: %#v", resume.Body.Data)
	}

	missing := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/missing/resume-approved", "test-token", `{}`)
	if missing.StatusCode != http.StatusNotFound || missing.Body.OK || missing.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("unexpected missing resume response: status=%d body=%#v", missing.StatusCode, missing.Body)
	}
}

func TestHTTPScanWebAsyncJobAPIValidationAndCompletion(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><head><script src="/app.js"></script></head><body><a href="/admin">Admin</a></body></html>`))
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch("/api/status")`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte("<urlset></urlset>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vscan-api"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vscan-api"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	missingTarget := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/scans/web", "test-token", `{"scope":"127.0.0.1"}`)
	if missingTarget.StatusCode != http.StatusBadRequest || missingTarget.Body.OK || missingTarget.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected missing target response: status=%d body=%#v", missingTarget.StatusCode, missingTarget.Body)
	}

	missingScope := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/scans/web", "test-token", `{"target":"`+target.URL+`"}`)
	if missingScope.StatusCode != http.StatusUnprocessableEntity || missingScope.Body.OK || missingScope.Body.Error.Code != services.CodeScopeRequired {
		t.Fatalf("unexpected missing scope response: status=%d body=%#v", missingScope.StatusCode, missingScope.Body)
	}

	badJSON := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/scans/web", "test-token", `{"target":`)
	if badJSON.StatusCode != http.StatusBadRequest || badJSON.Body.OK || badJSON.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected bad JSON response: status=%d body=%#v", badJSON.StatusCode, badJSON.Body)
	}

	unsupportedProfile := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/scans/web", "test-token", `{"target":"`+target.URL+`","scope":"`+target.URL+`","profile":"aggressive"}`)
	if unsupportedProfile.StatusCode != http.StatusBadRequest || unsupportedProfile.Body.OK || unsupportedProfile.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected unsupported profile response: status=%d body=%#v", unsupportedProfile.StatusCode, unsupportedProfile.Body)
	}

	start := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/scans/web", "test-token", `{"target":"`+target.URL+`","scope":"`+target.URL+`"}`)
	if start.StatusCode != http.StatusOK || !start.Body.OK {
		t.Fatalf("unexpected scan start response: status=%d body=%#v", start.StatusCode, start.Body)
	}
	jobID := stringValue(t, start.Body.Data["job_id"])
	runID := stringValue(t, start.Body.Data["run_id"])
	runDir := stringValue(t, start.Body.Data["run_dir"])
	if jobID == "" || runID == "" || runDir == "" || start.Body.Data["kind"] != scanWebJobKind || start.Body.Data["status"] != string(contracts.StatusRunning) {
		t.Fatalf("unexpected scan start data: %#v", start.Body.Data)
	}
	if start.Body.Data["status_url"] != "/api/v1/jobs/"+jobID || start.Body.Data["run_url"] != "/api/v1/runs/"+runID {
		t.Fatalf("unexpected scan URLs: %#v", start.Body.Data)
	}

	job := waitDaemonJobStatus(t, api.URL, "test-token", jobID, 8*time.Second, contracts.StatusSucceeded)
	if job["run_id"] != runID || job["run_dir"] != runDir || job["status"] != string(contracts.StatusSucceeded) || job["error"] != nil {
		t.Fatalf("unexpected terminal job: %#v", job)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Fatalf("run dir was not created: %v", err)
	}
	detail := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID, "test-token")
	if detail.StatusCode != http.StatusOK || !detail.Body.OK || detail.Body.Data["run_id"] != runID {
		t.Fatalf("unexpected run detail after scan: status=%d body=%#v", detail.StatusCode, detail.Body)
	}
	report := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/report?format=json", "test-token")
	if report.StatusCode != http.StatusOK || !report.Body.OK || report.Body.Data["format"] != "json" || report.Body.Data["report"] == nil {
		t.Fatalf("unexpected run report after scan: status=%d body=%#v", report.StatusCode, report.Body)
	}

	finishedCancel := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/jobs/"+jobID+"/cancel", "test-token", `{}`)
	if finishedCancel.StatusCode != http.StatusConflict || finishedCancel.Body.OK || finishedCancel.Body.Error.Code != services.CodeConflict {
		t.Fatalf("unexpected finished cancel response: status=%d body=%#v", finishedCancel.StatusCode, finishedCancel.Body)
	}
}

func TestHTTPAgentSessionContinuityAcrossMessages(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><body>continuity</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-continuity"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-continuity"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	scan := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"扫描 `+target.URL+` scope=`+target.URL+`"}`,
	)
	if scan.StatusCode != http.StatusOK || !scan.Body.OK {
		t.Fatalf("unexpected agent scan message response: status=%d body=%#v", scan.StatusCode, scan.Body)
	}
	scanToolCalls := asList(t, scan.Body.Data["tool_calls"])
	if len(scanToolCalls) == 0 {
		t.Fatalf("expected scan tool call: %#v", scan.Body.Data["tool_calls"])
	}
	scanTool := asMap(t, scanToolCalls[0])
	if scanTool["name"] != "POST /api/v1/scans/web" || scanTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected scan tool call: %#v", scanTool)
	}
	scanMetadata := asMap(t, scanTool["metadata"])
	if scanMetadata["phase"] != "daemon_job" ||
		scanMetadata["risk"] != "medium" ||
		boolValue(t, scanMetadata["auto_run"]) ||
		scanMetadata["approval_policy"] != "explicit_user_instruction" ||
		scanMetadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("scan tool should expose explicit daemon-job metadata: %#v", scanMetadata)
	}
	scanResult := asMap(t, scanTool["result"])
	jobID := stringValue(t, scanResult["job_id"])
	runID := stringValue(t, scanResult["run_id"])
	if jobID == "" || runID == "" {
		t.Fatalf("expected job and run id from scan tool result: %#v", scanResult)
	}
	if scanResult["target"] != target.URL ||
		scanResult["scope"] != target.URL ||
		scanResult["profile"] != "safe" ||
		scanResult["next_step"] != "track_run_context" ||
		!boolValue(t, scanResult["no_external_tools"]) ||
		stringValue(t, scanResult["run_url"]) == "" ||
		stringValue(t, scanResult["status_url"]) == "" {
		t.Fatalf("scan result should carry workflow context: %#v", scanResult)
	}
	waitDaemonJobStatus(t, api.URL, "test-token", jobID, 8*time.Second, contracts.StatusSucceeded)

	firstTask := asMap(t, scan.Body.Data["active_task"])
	firstTaskID := stringValue(t, firstTask["id"])
	firstGoal := stringValue(t, firstTask["goal"])
	firstRunID := stringValue(t, firstTask["active_run_id"])
	if firstTask["kind"] != "scan" || firstTask["status"] != "running" || !strings.Contains(stringValue(t, firstTask["summary"]), runID) || !strings.Contains(firstGoal, target.URL) {
		t.Fatalf("scan should keep an active Agent task with run context: %#v", firstTask)
	}
	firstTodos := asList(t, scan.Body.Data["todos"])
	if len(firstTodos) < 5 {
		t.Fatalf("scan should expose workflow todos: %#v", firstTodos)
	}
	if asMap(t, firstTodos[0])["status"] != "done" ||
		asMap(t, firstTodos[1])["status"] != "done" ||
		asMap(t, firstTodos[2])["status"] != "done" ||
		asMap(t, firstTodos[3])["status"] != "running" ||
		asMap(t, firstTodos[4])["status"] != "pending" {
		t.Fatalf("scan todos should show plan->start done and tracking/report pending: %#v", firstTodos)
	}
	reportTodoRefs := asStringList(t, asMap(t, firstTodos[4])["context_ref_ids"])
	if !stringListContains(reportTodoRefs, "run:"+runID) || !stringListContains(reportTodoRefs, "job:"+jobID) {
		t.Fatalf("report follow-up todo should link run/job context: refs=%#v todos=%#v", reportTodoRefs, firstTodos)
	}
	firstRefs := asList(t, scan.Body.Data["context_refs"])
	runRef := findMapByKey(t, firstRefs, "id", "run:"+runID)
	jobRef := findMapByKey(t, firstRefs, "id", "job:"+jobID)
	if runRef["kind"] != "run" || runRef["to"] != "/scan/"+runID || jobRef["kind"] != "job" {
		t.Fatalf("scan should persist run/job context refs: run=%#v job=%#v", runRef, jobRef)
	}
	firstNext := asMap(t, scan.Body.Data["next_action"])
	if firstNext["type"] != "continue" ||
		!boolValue(t, firstNext["auto_continue"]) ||
		!strings.Contains(stringValue(t, firstNext["suggested_prompt"]), runID) {
		t.Fatalf("scan next_action should drive run tracking: %#v", firstNext)
	}
	firstMessages := asList(t, scan.Body.Data["messages"])
	if firstTaskID == "" || firstRunID == "" || len(firstMessages) < 3 {
		t.Fatalf("expected initial scan to set active task, run id, and messages: %#v", scan.Body.Data)
	}

	followUp := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"请继续监控当前任务并汇总下一步建议。"}`,
	)
	if followUp.StatusCode != http.StatusOK || !followUp.Body.OK {
		t.Fatalf("unexpected agent follow-up message response: status=%d body=%#v", followUp.StatusCode, followUp.Body)
	}
	if followUp.Body.Data["session_id"] != sessionID {
		t.Fatalf("follow-up session id mismatch: %#v", followUp.Body.Data)
	}
	followTask := asMap(t, followUp.Body.Data["active_task"])
	if stringValue(t, followTask["id"]) != firstTaskID {
		t.Fatalf("follow-up should preserve active task id: first=%s follow=%#v", firstTaskID, followTask)
	}
	if stringValue(t, followTask["active_run_id"]) != firstRunID {
		t.Fatalf("follow-up should preserve active run id: first=%s follow=%#v", firstRunID, followTask)
	}
	if stringValue(t, followTask["goal"]) != firstGoal {
		t.Fatalf("follow-up should preserve active goal: first=%s follow=%#v", firstGoal, followTask)
	}
	followMessages := asList(t, followUp.Body.Data["messages"])
	if len(followMessages) < 3 {
		t.Fatalf("follow-up should return user + control + run context observation: %#v", followMessages)
	}
	followTools := asList(t, followUp.Body.Data["tool_calls"])
	if len(followTools) != 1 {
		t.Fatalf("continue active run should read run context exactly once: %#v", followTools)
	}
	followTool := asMap(t, followTools[0])
	if followTool["name"] != "GET /api/v1/runs/{run_id}/context" || followTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("continue active run should produce run context tool observation: %#v", followTool)
	}
	followResult := asMap(t, followTool["result"])
	if followResult["run_id"] != firstRunID || followResult["context"] != "report_json" {
		t.Fatalf("continue active run should read the completed run report context: %#v", followResult)
	}
	var foundReportObservation bool
	for _, item := range followMessages {
		message := asMap(t, item)
		if message["role"] == "tool" && strings.Contains(stringValue(t, message["content"]), "report.json") {
			foundReportObservation = true
			break
		}
	}
	if !foundReportObservation {
		t.Fatalf("continue active run should return a report observation message: %#v", followMessages)
	}
	followConclusion := asMap(t, followMessages[len(followMessages)-1])
	if followConclusion["role"] != "agent" || !strings.Contains(stringValue(t, followConclusion["content"]), "后续行动计划") {
		t.Fatalf("continue active run should append an evidence action planning conclusion: %#v", followConclusion)
	}
	followRefs := asList(t, followUp.Body.Data["context_refs"])
	if !listOfMapsContains(t, followRefs, "id", "report_json:"+firstRunID) {
		t.Fatalf("continue active run should persist report context ref: %#v", followRefs)
	}
	followTodos := asList(t, followUp.Body.Data["todos"])
	if !listOfMapsContains(t, followTodos, "id", "todo_run_action_01") ||
		!listOfMapsContains(t, followTodos, "id", "todo_run_action_02") ||
		!listOfMapsContains(t, followTodos, "id", "todo_run_action_03") {
		t.Fatalf("continue active run should auto-create evidence action todos: %#v", followTodos)
	}
	actionTodo := findMapByKey(t, followTodos, "id", "todo_run_action_01")
	if actionTodo["status"] != "running" ||
		!strings.Contains(stringValue(t, actionTodo["title"]), "finding") ||
		!stringListContains(asStringList(t, actionTodo["context_ref_ids"]), "report_json:"+firstRunID) {
		t.Fatalf("first evidence action todo should be running and linked to report context: %#v", actionTodo)
	}
	followNext := asMap(t, followUp.Body.Data["next_action"])
	if followNext["type"] != "continue" ||
		!boolValue(t, followNext["auto_continue"]) ||
		!strings.Contains(stringValue(t, followNext["suggested_prompt"]), "误报检查") {
		t.Fatalf("continue active run should produce evidence action next prompt: %#v", followNext)
	}

	nextActionStep := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"继续"}`,
	)
	if nextActionStep.StatusCode != http.StatusOK || !nextActionStep.Body.OK {
		t.Fatalf("unexpected evidence action continue response: status=%d body=%#v", nextActionStep.StatusCode, nextActionStep.Body)
	}
	stepTodos := asList(t, nextActionStep.Body.Data["todos"])
	if findMapByKey(t, stepTodos, "id", "todo_run_action_01")["status"] != "done" ||
		findMapByKey(t, stepTodos, "id", "todo_run_action_02")["status"] != "running" ||
		findMapByKey(t, stepTodos, "id", "todo_run_action_03")["status"] != "pending" {
		t.Fatalf("second continue should advance one evidence action todo: %#v", stepTodos)
	}
	stepTools := asList(t, nextActionStep.Body.Data["tool_calls"])
	if len(stepTools) != 1 || asMap(t, stepTools[0])["name"] != "GET /api/v1/runs/{run_id}/context" {
		t.Fatalf("second continue should still ground progress in run context: %#v", stepTools)
	}
	stepMessages := asList(t, nextActionStep.Body.Data["messages"])
	stepConclusion := asMap(t, stepMessages[len(stepMessages)-1])
	if stepConclusion["role"] != "agent" || !strings.Contains(stringValue(t, stepConclusion["content"]), "证据链关联结论") {
		t.Fatalf("second continue should append a stage conclusion message: %#v", stepConclusion)
	}

	falsePositiveStep := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"继续"}`,
	)
	if falsePositiveStep.StatusCode != http.StatusOK || !falsePositiveStep.Body.OK {
		t.Fatalf("unexpected false-positive step response: status=%d body=%#v", falsePositiveStep.StatusCode, falsePositiveStep.Body)
	}
	falsePositiveTodos := asList(t, falsePositiveStep.Body.Data["todos"])
	if findMapByKey(t, falsePositiveTodos, "id", "todo_run_action_01")["status"] != "done" ||
		findMapByKey(t, falsePositiveTodos, "id", "todo_run_action_02")["status"] != "done" ||
		findMapByKey(t, falsePositiveTodos, "id", "todo_run_action_03")["status"] != "running" {
		t.Fatalf("third continue should advance to remediation/retest todo: %#v", falsePositiveTodos)
	}
	falsePositiveMessages := asList(t, falsePositiveStep.Body.Data["messages"])
	falsePositiveConclusion := asMap(t, falsePositiveMessages[len(falsePositiveMessages)-1])
	if falsePositiveConclusion["role"] != "agent" || !strings.Contains(stringValue(t, falsePositiveConclusion["content"]), "误报检查与影响边界") {
		t.Fatalf("third continue should append false-positive/impact conclusion: %#v", falsePositiveConclusion)
	}

	remediationStep := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"继续"}`,
	)
	if remediationStep.StatusCode != http.StatusOK || !remediationStep.Body.OK {
		t.Fatalf("unexpected remediation step response: status=%d body=%#v", remediationStep.StatusCode, remediationStep.Body)
	}
	remediationTodos := asList(t, remediationStep.Body.Data["todos"])
	if findMapByKey(t, remediationTodos, "id", "todo_run_action_01")["status"] != "done" ||
		findMapByKey(t, remediationTodos, "id", "todo_run_action_02")["status"] != "done" ||
		findMapByKey(t, remediationTodos, "id", "todo_run_action_03")["status"] != "done" {
		t.Fatalf("fourth continue should complete all evidence action todos: %#v", remediationTodos)
	}
	remediationMessages := asList(t, remediationStep.Body.Data["messages"])
	remediationConclusion := asMap(t, remediationMessages[len(remediationMessages)-1])
	if remediationConclusion["role"] != "agent" || !strings.Contains(stringValue(t, remediationConclusion["content"]), "修复与复测计划") {
		t.Fatalf("fourth continue should append remediation/retest conclusion: %#v", remediationConclusion)
	}
	remediationNext := asMap(t, remediationStep.Body.Data["next_action"])
	if remediationNext["type"] != "wait_user" || !strings.Contains(stringValue(t, remediationNext["suggested_prompt"]), firstRunID) {
		t.Fatalf("fourth continue should converge to wait_user with summary prompt: %#v", remediationNext)
	}

	read := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if read.StatusCode != http.StatusOK || !read.Body.OK {
		t.Fatalf("unexpected read agent session response: status=%d body=%#v", read.StatusCode, read.Body)
	}
	readSession := asMap(t, read.Body.Data["session"])
	if stringValue(t, asMap(t, readSession["active_task"])["id"]) != firstTaskID {
		t.Fatalf("persisted session should keep active task id: %#v", readSession["active_task"])
	}
	if stringValue(t, asMap(t, readSession["active_task"])["active_run_id"]) != firstRunID {
		t.Fatalf("persisted session should keep active run id: %#v", readSession["active_task"])
	}
	readMessages := asList(t, readSession["messages"])
	if len(readMessages) <= len(firstMessages) {
		t.Fatalf("persisted session should keep appended messages: first=%d read=%d", len(firstMessages), len(readMessages))
	}
}

func TestHTTPAgentNaturalLanguageInvokesWorkspaceTools(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-tools"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-tools"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	initialList := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/workspace/files/list",
		"test-token",
		`{"path":"."}`,
	)
	if initialList.StatusCode != http.StatusOK || !initialList.Body.OK {
		t.Fatalf("workspace list should prepare a new session workspace: status=%d body=%#v", initialList.StatusCode, initialList.Body)
	}
	initialListData := initialList.Body.Data
	if initialListData["path"] != "." {
		t.Fatalf("unexpected initial workspace list path: %#v", initialListData)
	}

	commandRun := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"命令: pwd"}`,
	)
	if commandRun.StatusCode != http.StatusOK || !commandRun.Body.OK {
		t.Fatalf("unexpected command agent message response: status=%d body=%#v", commandRun.StatusCode, commandRun.Body)
	}
	commandTools := asList(t, commandRun.Body.Data["tool_calls"])
	if len(commandTools) != 1 {
		t.Fatalf("expected command tool call: %#v", commandRun.Body.Data["tool_calls"])
	}
	commandTool := asMap(t, commandTools[0])
	if commandTool["name"] != "workspace.command" || commandTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected command tool call: %#v", commandTool)
	}
	commandResult := asMap(t, commandTool["result"])
	if commandResult["command"] != "pwd" || int(commandResult["exit_code"].(float64)) != 0 || stringValue(t, commandResult["workdir"]) == "" {
		t.Fatalf("unexpected command tool result: %#v", commandResult)
	}
	commandMessages := asList(t, commandRun.Body.Data["messages"])
	if !strings.Contains(stringValue(t, asMap(t, commandMessages[len(commandMessages)-1])["content"]), "命令已执行") {
		t.Fatalf("command response should summarize execution: %#v", commandMessages)
	}

	multiCommand := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"检查当前工作区目录结构，先执行 pwd 和 ls -la，再总结关键文件"}`,
	)
	if multiCommand.StatusCode != http.StatusOK || !multiCommand.Body.OK {
		t.Fatalf("unexpected multi-command agent response: status=%d body=%#v", multiCommand.StatusCode, multiCommand.Body)
	}
	multiTools := asList(t, multiCommand.Body.Data["tool_calls"])
	if len(multiTools) != 2 {
		t.Fatalf("expected two workspace command tool calls: %#v", multiTools)
	}
	firstCommandTool := asMap(t, multiTools[0])
	secondCommandTool := asMap(t, multiTools[1])
	if firstCommandTool["name"] != "workspace.command" || secondCommandTool["name"] != "workspace.command" ||
		firstCommandTool["status"] != string(contracts.StatusSucceeded) || secondCommandTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected multi-command tool status: %#v", multiTools)
	}
	firstCommandResult := asMap(t, firstCommandTool["result"])
	secondCommandResult := asMap(t, secondCommandTool["result"])
	if firstCommandResult["command"] != "pwd" ||
		secondCommandResult["command"] != "ls -la" ||
		int(firstCommandResult["command_index"].(float64)) != 1 ||
		int(secondCommandResult["command_index"].(float64)) != 2 ||
		int(secondCommandResult["command_count"].(float64)) != 2 {
		t.Fatalf("unexpected multi-command result ordering: first=%#v second=%#v", firstCommandResult, secondCommandResult)
	}
	multiMessages := asList(t, multiCommand.Body.Data["messages"])
	lastMultiMessage := asMap(t, multiMessages[len(multiMessages)-1])
	if !strings.Contains(stringValue(t, lastMultiMessage["content"]), "按顺序执行 2 个低风险工作区命令") ||
		!strings.Contains(stringValue(t, lastMultiMessage["content"]), "`pwd`") ||
		!strings.Contains(stringValue(t, lastMultiMessage["content"]), "`ls -la`") {
		t.Fatalf("multi-command response should summarize the ordered commands: %#v", lastMultiMessage)
	}

	write := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"写入文件 notes.md 内容: agent workspace note"}`,
	)
	if write.StatusCode != http.StatusOK || !write.Body.OK {
		t.Fatalf("unexpected write agent message response: status=%d body=%#v", write.StatusCode, write.Body)
	}
	writeTools := asList(t, write.Body.Data["tool_calls"])
	if len(writeTools) != 1 {
		t.Fatalf("expected workspace write tool call: %#v", write.Body.Data["tool_calls"])
	}
	writeTool := asMap(t, writeTools[0])
	if writeTool["name"] != "workspace.files.write" || writeTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace write tool call: %#v", writeTool)
	}
	writeResult := asMap(t, writeTool["result"])
	if writeResult["path"] != "notes.md" || int(writeResult["size"].(float64)) != len("agent workspace note") || !strings.Contains(stringValue(t, writeResult["content_preview"]), "agent workspace note") {
		t.Fatalf("unexpected workspace write result: %#v", writeResult)
	}
	writeMessages := asList(t, write.Body.Data["messages"])
	if !strings.Contains(stringValue(t, asMap(t, writeMessages[len(writeMessages)-1])["content"]), "已写入工作空间文件") {
		t.Fatalf("write response should summarize file write: %#v", writeMessages)
	}

	diff := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"预览修改文件 notes.md old: agent workspace note new: staged workspace note"}`,
	)
	if diff.StatusCode != http.StatusOK || !diff.Body.OK {
		t.Fatalf("unexpected diff agent message response: status=%d body=%#v", diff.StatusCode, diff.Body)
	}
	diffTools := asList(t, diff.Body.Data["tool_calls"])
	if len(diffTools) != 1 {
		t.Fatalf("expected workspace diff tool call: %#v", diff.Body.Data["tool_calls"])
	}
	diffTool := asMap(t, diffTools[0])
	if diffTool["name"] != "workspace.files.diff" || diffTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace diff tool call: %#v", diffTool)
	}
	diffResult := asMap(t, diffTool["result"])
	changeID := stringValue(t, diffResult["change_id"])
	if changeID == "" || diffResult["path"] != "notes.md" || diffResult["status"] != "pending" {
		t.Fatalf("unexpected workspace diff result: %#v", diffResult)
	}
	if !strings.Contains(stringValue(t, diffResult["diff_preview"]), "-agent workspace note") || !strings.Contains(stringValue(t, diffResult["diff_preview"]), "+staged workspace note") {
		t.Fatalf("workspace diff preview should show staged replacement: %#v", diffResult)
	}
	diffSession := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if diffSession.StatusCode != http.StatusOK || !diffSession.Body.OK {
		t.Fatalf("unexpected diff session response: status=%d body=%#v", diffSession.StatusCode, diffSession.Body)
	}
	diffPending := asList(t, asMap(t, diffSession.Body.Data["session"])["pending_changes"])
	if !listOfMapsContains(t, diffPending, "id", changeID) || !listOfMapsContains(t, diffPending, "status", "pending") {
		t.Fatalf("session should persist pending staged change: %#v", diffPending)
	}
	beforeApplyRead := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"读取文件 notes.md"}`,
	)
	if beforeApplyRead.StatusCode != http.StatusOK || !beforeApplyRead.Body.OK {
		t.Fatalf("unexpected before-apply read response: status=%d body=%#v", beforeApplyRead.StatusCode, beforeApplyRead.Body)
	}
	beforeApplyReadTool := asMap(t, asList(t, beforeApplyRead.Body.Data["tool_calls"])[0])
	beforeApplyReadResult := asMap(t, beforeApplyReadTool["result"])
	if !strings.Contains(stringValue(t, beforeApplyReadResult["content"]), "agent workspace note") || strings.Contains(stringValue(t, beforeApplyReadResult["content"]), "staged workspace note") {
		t.Fatalf("diff preview must not modify file before apply: %#v", beforeApplyReadResult)
	}
	apply := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		fmt.Sprintf(`{"operator":"tester","language":"zh","message":"应用 change_id: %s"}`, changeID),
	)
	if apply.StatusCode != http.StatusOK || !apply.Body.OK {
		t.Fatalf("unexpected apply agent message response: status=%d body=%#v", apply.StatusCode, apply.Body)
	}
	applyTool := asMap(t, asList(t, apply.Body.Data["tool_calls"])[0])
	if applyTool["name"] != "workspace.files.apply" || applyTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace apply tool call: %#v", applyTool)
	}
	applyResult := asMap(t, applyTool["result"])
	if applyResult["change_id"] != changeID || applyResult["status"] != "applied" {
		t.Fatalf("unexpected workspace apply result: %#v", applyResult)
	}

	patch := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"修改文件 notes.md old: staged workspace note new: patched workspace note"}`,
	)
	if patch.StatusCode != http.StatusOK || !patch.Body.OK {
		t.Fatalf("unexpected patch agent message response: status=%d body=%#v", patch.StatusCode, patch.Body)
	}
	patchTools := asList(t, patch.Body.Data["tool_calls"])
	if len(patchTools) != 1 {
		t.Fatalf("expected workspace patch tool call: %#v", patch.Body.Data["tool_calls"])
	}
	patchTool := asMap(t, patchTools[0])
	if patchTool["name"] != "workspace.files.patch" || patchTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace patch tool call: %#v", patchTool)
	}
	patchResult := asMap(t, patchTool["result"])
	if patchResult["path"] != "notes.md" || int(patchResult["replacements"].(float64)) != 1 {
		t.Fatalf("unexpected workspace patch result: %#v", patchResult)
	}
	diffPreview := stringValue(t, patchResult["diff_preview"])
	if !strings.Contains(diffPreview, "-staged workspace note") || !strings.Contains(diffPreview, "+patched workspace note") {
		t.Fatalf("workspace patch diff preview should show exact replacement: %#v", patchResult)
	}
	if !strings.Contains(stringValue(t, patchResult["content_preview"]), "patched workspace note") {
		t.Fatalf("workspace patch content preview should show patched content: %#v", patchResult)
	}
	patchMessages := asList(t, patch.Body.Data["messages"])
	if !strings.Contains(stringValue(t, asMap(t, patchMessages[len(patchMessages)-1])["content"]), "已修改工作空间文件") {
		t.Fatalf("patch response should summarize file patch: %#v", patchMessages)
	}

	discardDiff := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"预览修改文件 notes.md old: patched workspace note new: discarded workspace note"}`,
	)
	if discardDiff.StatusCode != http.StatusOK || !discardDiff.Body.OK {
		t.Fatalf("unexpected discard-diff agent message response: status=%d body=%#v", discardDiff.StatusCode, discardDiff.Body)
	}
	discardDiffTool := asMap(t, asList(t, discardDiff.Body.Data["tool_calls"])[0])
	discardDiffResult := asMap(t, discardDiffTool["result"])
	discardChangeID := stringValue(t, discardDiffResult["change_id"])
	discard := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		fmt.Sprintf(`{"operator":"tester","language":"zh","message":"丢弃 change_id: %s"}`, discardChangeID),
	)
	if discard.StatusCode != http.StatusOK || !discard.Body.OK {
		t.Fatalf("unexpected discard agent message response: status=%d body=%#v", discard.StatusCode, discard.Body)
	}
	discardTool := asMap(t, asList(t, discard.Body.Data["tool_calls"])[0])
	if discardTool["name"] != "workspace.files.discard" || discardTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace discard tool call: %#v", discardTool)
	}
	discardResult := asMap(t, discardTool["result"])
	if discardResult["change_id"] != discardChangeID || discardResult["status"] != "discarded" {
		t.Fatalf("unexpected workspace discard result: %#v", discardResult)
	}

	for _, fixture := range []struct {
		path    string
		content string
	}{
		{path: "bulk-a.md", content: "bulk a old"},
		{path: "bulk-b.md", content: "bulk b old"},
	} {
		bulkWrite := doDaemonRequestWithBody(
			t,
			http.MethodPost,
			api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
			"test-token",
			fmt.Sprintf(`{"operator":"tester","language":"zh","message":"写入文件 %s 内容: %s"}`, fixture.path, fixture.content),
		)
		if bulkWrite.StatusCode != http.StatusOK || !bulkWrite.Body.OK {
			t.Fatalf("unexpected bulk write response for %s: status=%d body=%#v", fixture.path, bulkWrite.StatusCode, bulkWrite.Body)
		}
		bulkDiff := doDaemonRequestWithBody(
			t,
			http.MethodPost,
			api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
			"test-token",
			fmt.Sprintf(`{"operator":"tester","language":"zh","message":"预览修改文件 %s old: old new: new"}`, fixture.path),
		)
		if bulkDiff.StatusCode != http.StatusOK || !bulkDiff.Body.OK {
			t.Fatalf("unexpected bulk diff response for %s: status=%d body=%#v", fixture.path, bulkDiff.StatusCode, bulkDiff.Body)
		}
	}
	applyAll := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"应用所有待应用修改"}`,
	)
	if applyAll.StatusCode != http.StatusOK || !applyAll.Body.OK {
		t.Fatalf("unexpected apply-all agent message response: status=%d body=%#v", applyAll.StatusCode, applyAll.Body)
	}
	applyAllTool := asMap(t, asList(t, applyAll.Body.Data["tool_calls"])[0])
	if applyAllTool["name"] != "workspace.files.apply" || applyAllTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace apply-all tool call: %#v", applyAllTool)
	}
	applyAllResult := asMap(t, applyAllTool["result"])
	if applyAllResult["action"] != "apply_all" || applyAllResult["status"] != "succeeded" ||
		int(applyAllResult["requested_count"].(float64)) != 2 || int(applyAllResult["succeeded_count"].(float64)) != 2 {
		t.Fatalf("unexpected workspace apply-all result: %#v", applyAllResult)
	}
	bulkRead := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/workspace/files/read", "test-token", `{"path":"bulk-a.md"}`)
	if bulkRead.StatusCode != http.StatusOK || !bulkRead.Body.OK || !strings.Contains(stringValue(t, bulkRead.Body.Data["content"]), "bulk a new") {
		t.Fatalf("apply-all should mutate bulk-a.md: status=%d body=%#v", bulkRead.StatusCode, bulkRead.Body)
	}

	for _, fixture := range []struct {
		path string
		old  string
	}{
		{path: "bulk-a.md", old: "new"},
		{path: "bulk-b.md", old: "new"},
	} {
		bulkDiff := doDaemonRequestWithBody(
			t,
			http.MethodPost,
			api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
			"test-token",
			fmt.Sprintf(`{"operator":"tester","language":"zh","message":"预览修改文件 %s old: %s new: discarded"}`, fixture.path, fixture.old),
		)
		if bulkDiff.StatusCode != http.StatusOK || !bulkDiff.Body.OK {
			t.Fatalf("unexpected discard-all fixture diff for %s: status=%d body=%#v", fixture.path, bulkDiff.StatusCode, bulkDiff.Body)
		}
	}
	discardAll := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"丢弃所有待应用修改"}`,
	)
	if discardAll.StatusCode != http.StatusOK || !discardAll.Body.OK {
		t.Fatalf("unexpected discard-all agent message response: status=%d body=%#v", discardAll.StatusCode, discardAll.Body)
	}
	discardAllTool := asMap(t, asList(t, discardAll.Body.Data["tool_calls"])[0])
	if discardAllTool["name"] != "workspace.files.discard" || discardAllTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace discard-all tool call: %#v", discardAllTool)
	}
	discardAllResult := asMap(t, discardAllTool["result"])
	if discardAllResult["action"] != "discard_all" || discardAllResult["status"] != "succeeded" ||
		int(discardAllResult["requested_count"].(float64)) != 2 || int(discardAllResult["succeeded_count"].(float64)) != 2 {
		t.Fatalf("unexpected workspace discard-all result: %#v", discardAllResult)
	}
	bulkReadAfterDiscard := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/workspace/files/read", "test-token", `{"path":"bulk-a.md"}`)
	if bulkReadAfterDiscard.StatusCode != http.StatusOK || !bulkReadAfterDiscard.Body.OK || strings.Contains(stringValue(t, bulkReadAfterDiscard.Body.Data["content"]), "discarded") {
		t.Fatalf("discard-all should not mutate bulk-a.md: status=%d body=%#v", bulkReadAfterDiscard.StatusCode, bulkReadAfterDiscard.Body)
	}

	read := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"读取文件 notes.md"}`,
	)
	if read.StatusCode != http.StatusOK || !read.Body.OK {
		t.Fatalf("unexpected read agent message response: status=%d body=%#v", read.StatusCode, read.Body)
	}
	readTools := asList(t, read.Body.Data["tool_calls"])
	if len(readTools) != 1 {
		t.Fatalf("expected workspace read tool call: %#v", read.Body.Data["tool_calls"])
	}
	readTool := asMap(t, readTools[0])
	if readTool["name"] != "workspace.files.read" || readTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace read tool call: %#v", readTool)
	}
	readResult := asMap(t, readTool["result"])
	if readResult["path"] != "notes.md" || !strings.Contains(stringValue(t, readResult["content"]), "patched workspace note") {
		t.Fatalf("unexpected workspace read result: %#v", readResult)
	}

	deleteFile := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"删除文件 notes.md"}`,
	)
	if deleteFile.StatusCode != http.StatusOK || !deleteFile.Body.OK {
		t.Fatalf("unexpected delete agent message response: status=%d body=%#v", deleteFile.StatusCode, deleteFile.Body)
	}
	deleteTools := asList(t, deleteFile.Body.Data["tool_calls"])
	if len(deleteTools) != 1 {
		t.Fatalf("expected workspace delete tool call: %#v", deleteFile.Body.Data["tool_calls"])
	}
	deleteTool := asMap(t, deleteTools[0])
	if deleteTool["name"] != "workspace.files.delete" || deleteTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace delete tool call: %#v", deleteTool)
	}
	deleteResult := asMap(t, deleteTool["result"])
	if deleteResult["path"] != "notes.md" || deleteResult["status"] != "deleted" {
		t.Fatalf("unexpected workspace delete result: %#v", deleteResult)
	}
	deleteMessages := asList(t, deleteFile.Body.Data["messages"])
	if !strings.Contains(stringValue(t, asMap(t, deleteMessages[len(deleteMessages)-1])["content"]), "已删除工作空间文件") {
		t.Fatalf("delete response should summarize file delete: %#v", deleteMessages)
	}
	readDeleted := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/workspace/files/read", "test-token", `{"path":"notes.md"}`)
	if readDeleted.StatusCode == http.StatusOK && readDeleted.Body.OK {
		t.Fatalf("deleted workspace file should not be readable: %#v", readDeleted.Body)
	}

	persisted := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if persisted.StatusCode != http.StatusOK || !persisted.Body.OK {
		t.Fatalf("unexpected persisted session response: status=%d body=%#v", persisted.StatusCode, persisted.Body)
	}
	persistedMessages := asList(t, asMap(t, persisted.Body.Data["session"])["messages"])
	if len(persistedMessages) < 5 {
		t.Fatalf("agent session should persist natural-language tool messages: %#v", persistedMessages)
	}
	persistedSession := asMap(t, persisted.Body.Data["session"])
	persistedToolCalls := asList(t, persistedSession["tool_calls"])
	if !listOfMapsContains(t, persistedToolCalls, "name", "workspace.files.write") {
		t.Fatalf("persisted session should keep workspace write tool call: %#v", persistedToolCalls)
	}
	if !listOfMapsContains(t, persistedToolCalls, "name", "workspace.files.patch") {
		t.Fatalf("persisted session should keep workspace patch tool call: %#v", persistedToolCalls)
	}
	if !listOfMapsContains(t, persistedToolCalls, "name", "workspace.files.diff") {
		t.Fatalf("persisted session should keep workspace diff tool call: %#v", persistedToolCalls)
	}
	if !listOfMapsContains(t, persistedToolCalls, "name", "workspace.files.apply") {
		t.Fatalf("persisted session should keep workspace apply tool call: %#v", persistedToolCalls)
	}
	if !listOfMapsContains(t, persistedToolCalls, "name", "workspace.files.discard") {
		t.Fatalf("persisted session should keep workspace discard tool call: %#v", persistedToolCalls)
	}
	if !listOfMapsContains(t, persistedToolCalls, "name", "workspace.files.delete") {
		t.Fatalf("persisted session should keep workspace delete tool call: %#v", persistedToolCalls)
	}
	persistedPending := asList(t, persistedSession["pending_changes"])
	if !listOfMapsContains(t, persistedPending, "id", changeID) || !listOfMapsContains(t, persistedPending, "status", "applied") {
		t.Fatalf("persisted session should keep applied staged change: %#v", persistedPending)
	}
	if !listOfMapsContains(t, persistedPending, "id", discardChangeID) || !listOfMapsContains(t, persistedPending, "status", "discarded") {
		t.Fatalf("persisted session should keep discarded staged change: %#v", persistedPending)
	}
	contextRefs := asList(t, persistedSession["context_refs"])
	if !listOfMapsContains(t, contextRefs, "ref_id", "notes.md") {
		t.Fatalf("persisted session should keep workspace file context ref: %#v", contextRefs)
	}
}

func TestHTTPAgentNaturalLanguageMovesWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-move"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	write := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/workspace/files/write",
		"test-token",
		`{"path":"notes.md","content":"move me"}`,
	)
	if write.StatusCode != http.StatusOK || !write.Body.OK {
		t.Fatalf("unexpected write response: status=%d body=%#v", write.StatusCode, write.Body)
	}

	mkdir := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"创建目录 archive"}`,
	)
	if mkdir.StatusCode != http.StatusOK || !mkdir.Body.OK {
		t.Fatalf("unexpected mkdir agent message response: status=%d body=%#v", mkdir.StatusCode, mkdir.Body)
	}
	mkdirTool := asMap(t, asList(t, mkdir.Body.Data["tool_calls"])[0])
	if mkdirTool["name"] != "workspace.files.mkdir" || mkdirTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace mkdir tool call: %#v", mkdirTool)
	}

	copyFile := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"复制文件 notes.md 到 archive/notes-copy.md"}`,
	)
	if copyFile.StatusCode != http.StatusOK || !copyFile.Body.OK {
		t.Fatalf("unexpected copy agent message response: status=%d body=%#v", copyFile.StatusCode, copyFile.Body)
	}
	copyTool := asMap(t, asList(t, copyFile.Body.Data["tool_calls"])[0])
	if copyTool["name"] != "workspace.files.copy" || copyTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace copy tool call: %#v", copyTool)
	}
	copyResult := asMap(t, copyTool["result"])
	if copyResult["from"] != "notes.md" || copyResult["path"] != "archive/notes-copy.md" || copyResult["status"] != "copied" {
		t.Fatalf("unexpected workspace copy result: %#v", copyResult)
	}
	copyRead := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/workspace/files/read", "test-token", `{"path":"archive/notes-copy.md"}`)
	if copyRead.StatusCode != http.StatusOK || !copyRead.Body.OK || !strings.Contains(stringValue(t, copyRead.Body.Data["content"]), "move me") {
		t.Fatalf("copied file should be readable at destination: status=%d body=%#v", copyRead.StatusCode, copyRead.Body)
	}

	move := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"重命名文件 notes.md 为 archive/notes-renamed.md"}`,
	)
	if move.StatusCode != http.StatusOK || !move.Body.OK {
		t.Fatalf("unexpected move agent message response: status=%d body=%#v", move.StatusCode, move.Body)
	}
	moveTools := asList(t, move.Body.Data["tool_calls"])
	if len(moveTools) != 1 {
		t.Fatalf("expected workspace move tool call: %#v", move.Body.Data["tool_calls"])
	}
	moveTool := asMap(t, moveTools[0])
	if moveTool["name"] != "workspace.files.move" || moveTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected workspace move tool call: %#v", moveTool)
	}
	moveResult := asMap(t, moveTool["result"])
	if moveResult["from"] != "notes.md" || moveResult["path"] != "archive/notes-renamed.md" || moveResult["status"] != "moved" {
		t.Fatalf("unexpected workspace move result: %#v", moveResult)
	}
	moveMessages := asList(t, move.Body.Data["messages"])
	if !strings.Contains(stringValue(t, asMap(t, moveMessages[len(moveMessages)-1])["content"]), "已移动工作空间文件") {
		t.Fatalf("move response should summarize file move: %#v", moveMessages)
	}

	movedRead := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/workspace/files/read", "test-token", `{"path":"archive/notes-renamed.md"}`)
	if movedRead.StatusCode != http.StatusOK || !movedRead.Body.OK || !strings.Contains(stringValue(t, movedRead.Body.Data["content"]), "move me") {
		t.Fatalf("moved file should be readable at destination: status=%d body=%#v", movedRead.StatusCode, movedRead.Body)
	}
	oldRead := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/workspace/files/read", "test-token", `{"path":"notes.md"}`)
	if oldRead.StatusCode == http.StatusOK && oldRead.Body.OK {
		t.Fatalf("old path should no longer exist after move: %#v", oldRead.Body)
	}
}

func TestHTTPAgentCockpitShortcutPromptsRouteToLocalTools(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	pluginsDir := filepath.Join(root, "plugins")
	writeDaemonTestPlugin(t, pluginsDir)
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	writeDaemonRunJob(t, runsDir, "job_shortcut_001", contracts.StatusSucceeded, now, now.Add(time.Minute))
	writeDaemonRunReport(t, runsDir, "job_shortcut_001", now)
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-shortcuts"}),
		plugins:   services.NewPluginService(pluginsDir),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-shortcuts"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])

	cases := []struct {
		name           string
		message        string
		wantToolNames  []string
		wantToolDetail []string
		wantContent    string
	}{
		{
			name:           "health shortcut",
			message:        "daemon.health：检查 daemon 健康状态、版本和 Agent runtime 可用性",
			wantToolNames:  []string{"GET /api/v1/doctor"},
			wantToolDetail: []string{"daemon"},
			wantContent:    "Daemon 在线",
		},
		{
			name:           "tools shortcut",
			message:        "plugins.list：列出当前可用的内置工具、插件和安全边界",
			wantToolNames:  []string{"GET /api/v1/plugins"},
			wantToolDetail: []string{"registry"},
			wantContent:    "插件/工具",
		},
		{
			name:           "runs shortcut",
			message:        "runs.list：查看最近运行、失败项和需要处理的任务",
			wantToolNames:  []string{"GET /api/v1/runs"},
			wantToolDetail: []string{"runs-dir"},
			wantContent:    "最近 1 个运行",
		},
		{
			name:           "workspace multi-command shortcut",
			message:        "执行命令 `pwd` 和 `ls -la`",
			wantToolNames:  []string{"workspace.command", "workspace.command"},
			wantToolDetail: []string{"pwd", "ls -la"},
			wantContent:    "按顺序执行 2 个低风险工作区命令",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{
				"operator": "tester",
				"language": "zh",
				"message":  tc.message,
			})
			if err != nil {
				t.Fatalf("marshal shortcut payload: %v", err)
			}
			send := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", string(payload))
			if send.StatusCode != http.StatusOK || !send.Body.OK {
				t.Fatalf("unexpected shortcut message response: status=%d body=%#v", send.StatusCode, send.Body)
			}
			tools := asList(t, send.Body.Data["tool_calls"])
			if len(tools) != len(tc.wantToolNames) {
				t.Fatalf("unexpected shortcut tool count: got=%d want=%d tools=%#v", len(tools), len(tc.wantToolNames), tools)
			}
			for idx, raw := range tools {
				tool := asMap(t, raw)
				if tool["name"] != tc.wantToolNames[idx] || tool["status"] != string(contracts.StatusSucceeded) {
					t.Fatalf("unexpected shortcut tool[%d]: %#v", idx, tool)
				}
				if got := stringValue(t, tool["detail"]); got != tc.wantToolDetail[idx] {
					t.Fatalf("unexpected shortcut tool[%d] detail = %q, want %q; tool=%#v", idx, got, tc.wantToolDetail[idx], tool)
				}
				metadata := asMap(t, tool["metadata"])
				if metadata["risk"] != "low" || metadata["auto_run"] != true {
					t.Fatalf("shortcut tool should be low-risk auto metadata: %#v", metadata)
				}
				if metadata["source"] != "deterministic_route/registry_dispatch" {
					t.Fatalf("shortcut tool should expose deterministic registry source metadata: %#v", metadata)
				}
			}
			messages := asList(t, send.Body.Data["messages"])
			last := asMap(t, messages[len(messages)-1])
			if last["role"] != "tool" || !strings.Contains(stringValue(t, last["content"]), tc.wantContent) {
				t.Fatalf("shortcut should return tool observation containing %q: %#v", tc.wantContent, last)
			}
		})
	}
}

func TestHTTPAgentWorkspacePatchAPI(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-workspace-patch"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-workspace-patch"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])
	workspaceURL := api.URL + "/api/v1/agent/sessions/" + sessionID + "/workspace"

	write := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/write", "test-token", `{"path":"notes.md","content":"alpha beta gamma"}`)
	if write.StatusCode != http.StatusOK || !write.Body.OK {
		t.Fatalf("unexpected workspace write response: status=%d body=%#v", write.StatusCode, write.Body)
	}

	missingOld := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/patch", "test-token", `{"path":"notes.md","new":"delta"}`)
	if missingOld.StatusCode != http.StatusBadRequest || missingOld.Body.OK || missingOld.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected missing-old patch response: status=%d body=%#v", missingOld.StatusCode, missingOld.Body)
	}

	notFound := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/patch", "test-token", `{"path":"notes.md","old":"missing","new":"delta"}`)
	if notFound.StatusCode != http.StatusConflict || notFound.Body.OK || notFound.Body.Error.Code != services.CodeConflict {
		t.Fatalf("unexpected not-found patch response: status=%d body=%#v", notFound.StatusCode, notFound.Body)
	}

	duplicateWrite := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/write", "test-token", `{"path":"notes.md","content":"same same"}`)
	if duplicateWrite.StatusCode != http.StatusOK || !duplicateWrite.Body.OK {
		t.Fatalf("unexpected duplicate fixture write response: status=%d body=%#v", duplicateWrite.StatusCode, duplicateWrite.Body)
	}
	duplicate := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/patch", "test-token", `{"path":"notes.md","old":"same","new":"once"}`)
	if duplicate.StatusCode != http.StatusConflict || duplicate.Body.OK || duplicate.Body.Error.Code != services.CodeConflict {
		t.Fatalf("unexpected duplicate-old patch response: status=%d body=%#v", duplicate.StatusCode, duplicate.Body)
	}

	reset := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/write", "test-token", `{"path":"notes.md","content":"alpha beta gamma"}`)
	if reset.StatusCode != http.StatusOK || !reset.Body.OK {
		t.Fatalf("unexpected reset write response: status=%d body=%#v", reset.StatusCode, reset.Body)
	}
	patch := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/patch", "test-token", `{"path":"notes.md","old":"beta","new":"delta"}`)
	if patch.StatusCode != http.StatusOK || !patch.Body.OK {
		t.Fatalf("unexpected workspace patch response: status=%d body=%#v", patch.StatusCode, patch.Body)
	}
	result := patch.Body.Data
	if result["path"] != "notes.md" ||
		int(result["size_before"].(float64)) != len("alpha beta gamma") ||
		int(result["size_after"].(float64)) != len("alpha delta gamma") ||
		int(result["replacements"].(float64)) != 1 {
		t.Fatalf("unexpected workspace patch result metadata: %#v", result)
	}
	diffPreview := stringValue(t, result["diff_preview"])
	if !strings.Contains(diffPreview, "-beta") || !strings.Contains(diffPreview, "+delta") {
		t.Fatalf("workspace patch result should include diff preview: %#v", result)
	}
	if !strings.Contains(stringValue(t, result["content_preview"]), "alpha delta gamma") {
		t.Fatalf("workspace patch result should include content preview: %#v", result)
	}

	read := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"notes.md"}`)
	if read.StatusCode != http.StatusOK || !read.Body.OK || !strings.Contains(stringValue(t, read.Body.Data["content"]), "alpha delta gamma") {
		t.Fatalf("unexpected post-patch read response: status=%d body=%#v", read.StatusCode, read.Body)
	}

	mkdir := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/mkdir", "test-token", `{"path":"reports"}`)
	if mkdir.StatusCode != http.StatusOK || !mkdir.Body.OK || mkdir.Body.Data["status"] != "created" {
		t.Fatalf("unexpected workspace mkdir response: status=%d body=%#v", mkdir.StatusCode, mkdir.Body)
	}
	copyFile := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/copy", "test-token", `{"from":"notes.md","to":"reports/notes-copy.md"}`)
	if copyFile.StatusCode != http.StatusOK || !copyFile.Body.OK {
		t.Fatalf("unexpected workspace copy response: status=%d body=%#v", copyFile.StatusCode, copyFile.Body)
	}
	copyResult := asMap(t, copyFile.Body.Data)
	if copyResult["from"] != "notes.md" || copyResult["path"] != "reports/notes-copy.md" || copyResult["status"] != "copied" {
		t.Fatalf("unexpected workspace copy result metadata: %#v", copyResult)
	}
	copiedRead := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"reports/notes-copy.md"}`)
	if copiedRead.StatusCode != http.StatusOK || !copiedRead.Body.OK || !strings.Contains(stringValue(t, copiedRead.Body.Data["content"]), "alpha delta gamma") {
		t.Fatalf("unexpected copied-file read response: status=%d body=%#v", copiedRead.StatusCode, copiedRead.Body)
	}

	move := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/move", "test-token", `{"from":"notes.md","to":"archive/notes-renamed.md"}`)
	if move.StatusCode != http.StatusOK || !move.Body.OK {
		t.Fatalf("unexpected workspace move response: status=%d body=%#v", move.StatusCode, move.Body)
	}
	moveResult := asMap(t, move.Body.Data)
	if moveResult["from"] != "notes.md" || moveResult["path"] != "archive/notes-renamed.md" || moveResult["status"] != "moved" {
		t.Fatalf("unexpected workspace move result metadata: %#v", moveResult)
	}
	movedRead := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"archive/notes-renamed.md"}`)
	if movedRead.StatusCode != http.StatusOK || !movedRead.Body.OK || !strings.Contains(stringValue(t, movedRead.Body.Data["content"]), "alpha delta gamma") {
		t.Fatalf("unexpected moved-file read response: status=%d body=%#v", movedRead.StatusCode, movedRead.Body)
	}
}

func TestHTTPAgentWorkspaceCommandPolicy(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-command-policy"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-command-policy"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])
	workspaceURL := api.URL + "/api/v1/agent/sessions/" + sessionID + "/workspace"

	pwd := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/command", "test-token", `{"command":"pwd"}`)
	if pwd.StatusCode != http.StatusOK || !pwd.Body.OK || pwd.Body.Data["command"] != "pwd" {
		t.Fatalf("unexpected workspace command response: status=%d body=%#v", pwd.StatusCode, pwd.Body)
	}

	denied := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/command", "test-token", `{"command":"rm notes.md"}`)
	if denied.StatusCode != http.StatusUnprocessableEntity || denied.Body.OK || denied.Body.Error.Code != services.CodePolicyDenied {
		t.Fatalf("destructive command should be policy denied: status=%d body=%#v", denied.StatusCode, denied.Body)
	}

	escapeArg := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/command", "test-token", `{"command":"cat ../secret.txt"}`)
	if escapeArg.StatusCode != http.StatusBadRequest || escapeArg.Body.OK || escapeArg.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("escaping argument should be bad request: status=%d body=%#v", escapeArg.StatusCode, escapeArg.Body)
	}

	escapeDir := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/command", "test-token", `{"command":"pwd","workdir":".."}`)
	if escapeDir.StatusCode != http.StatusBadRequest || escapeDir.Body.OK || escapeDir.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("escaping workdir should be bad request: status=%d body=%#v", escapeDir.StatusCode, escapeDir.Body)
	}

	naturalDenied := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		`{"operator":"tester","language":"zh","message":"命令: rm notes.md"}`,
	)
	if naturalDenied.StatusCode != http.StatusOK || !naturalDenied.Body.OK {
		t.Fatalf("unexpected natural denied command response: status=%d body=%#v", naturalDenied.StatusCode, naturalDenied.Body)
	}
	tool := asMap(t, asList(t, naturalDenied.Body.Data["tool_calls"])[0])
	if tool["name"] != "workspace.command" || tool["status"] != string(contracts.StatusFailed) {
		t.Fatalf("natural denied command should be a failed workspace command tool call: %#v", tool)
	}
	errBody := asMap(t, tool["error"])
	if errBody["code"] != string(services.CodePolicyDenied) {
		t.Fatalf("natural denied command should expose policy_denied: %#v", tool)
	}
}

func TestHTTPAgentWorkspaceDiffApplyAPI(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-workspace-diff-apply"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-workspace-diff-apply"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])
	workspaceURL := api.URL + "/api/v1/agent/sessions/" + sessionID + "/workspace"

	write := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/write", "test-token", `{"path":"notes.md","content":"alpha beta gamma"}`)
	if write.StatusCode != http.StatusOK || !write.Body.OK {
		t.Fatalf("unexpected workspace write response: status=%d body=%#v", write.StatusCode, write.Body)
	}

	diff := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/diff", "test-token", `{"path":"notes.md","old":"beta","new":"delta"}`)
	if diff.StatusCode != http.StatusOK || !diff.Body.OK {
		t.Fatalf("unexpected workspace diff response: status=%d body=%#v", diff.StatusCode, diff.Body)
	}
	changeID := stringValue(t, diff.Body.Data["change_id"])
	if changeID == "" || diff.Body.Data["path"] != "notes.md" || diff.Body.Data["status"] != "pending" {
		t.Fatalf("unexpected workspace diff result: %#v", diff.Body.Data)
	}
	if !strings.Contains(stringValue(t, diff.Body.Data["diff_preview"]), "-beta") || !strings.Contains(stringValue(t, diff.Body.Data["diff_preview"]), "+delta") {
		t.Fatalf("workspace diff should include exact diff preview: %#v", diff.Body.Data)
	}

	readBefore := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"notes.md"}`)
	if readBefore.StatusCode != http.StatusOK || !readBefore.Body.OK || stringValue(t, readBefore.Body.Data["content"]) != "alpha beta gamma" {
		t.Fatalf("workspace diff must not mutate file: status=%d body=%#v", readBefore.StatusCode, readBefore.Body)
	}

	persisted := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if persisted.StatusCode != http.StatusOK || !persisted.Body.OK {
		t.Fatalf("unexpected persisted session response: status=%d body=%#v", persisted.StatusCode, persisted.Body)
	}
	pending := asList(t, asMap(t, persisted.Body.Data["session"])["pending_changes"])
	if !listOfMapsContains(t, pending, "id", changeID) || !listOfMapsContains(t, pending, "status", "pending") {
		t.Fatalf("session should persist pending diff: %#v", pending)
	}

	apply := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/apply", "test-token", fmt.Sprintf(`{"change_id":"%s"}`, changeID))
	if apply.StatusCode != http.StatusOK || !apply.Body.OK {
		t.Fatalf("unexpected workspace apply response: status=%d body=%#v", apply.StatusCode, apply.Body)
	}
	if apply.Body.Data["change_id"] != changeID || apply.Body.Data["status"] != "applied" || apply.Body.Data["path"] != "notes.md" {
		t.Fatalf("unexpected workspace apply result: %#v", apply.Body.Data)
	}

	readAfter := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"notes.md"}`)
	if readAfter.StatusCode != http.StatusOK || !readAfter.Body.OK || stringValue(t, readAfter.Body.Data["content"]) != "alpha delta gamma" {
		t.Fatalf("workspace apply should mutate file: status=%d body=%#v", readAfter.StatusCode, readAfter.Body)
	}

	duplicateApply := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/apply", "test-token", fmt.Sprintf(`{"change_id":"%s"}`, changeID))
	if duplicateApply.StatusCode != http.StatusConflict || duplicateApply.Body.OK || duplicateApply.Body.Error.Code != services.CodeConflict {
		t.Fatalf("duplicate apply should conflict: status=%d body=%#v", duplicateApply.StatusCode, duplicateApply.Body)
	}

	discardDiff := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/diff", "test-token", `{"path":"notes.md","old":"delta","new":"epsilon"}`)
	if discardDiff.StatusCode != http.StatusOK || !discardDiff.Body.OK {
		t.Fatalf("unexpected discard fixture diff response: status=%d body=%#v", discardDiff.StatusCode, discardDiff.Body)
	}
	discardID := stringValue(t, discardDiff.Body.Data["change_id"])
	discard := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/discard", "test-token", fmt.Sprintf(`{"change_id":"%s"}`, discardID))
	if discard.StatusCode != http.StatusOK || !discard.Body.OK || discard.Body.Data["status"] != "discarded" {
		t.Fatalf("unexpected workspace discard response: status=%d body=%#v", discard.StatusCode, discard.Body)
	}
	readAfterDiscard := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"notes.md"}`)
	if readAfterDiscard.StatusCode != http.StatusOK || !readAfterDiscard.Body.OK || stringValue(t, readAfterDiscard.Body.Data["content"]) != "alpha delta gamma" {
		t.Fatalf("workspace discard must not mutate file: status=%d body=%#v", readAfterDiscard.StatusCode, readAfterDiscard.Body)
	}
	duplicateDiscard := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/discard", "test-token", fmt.Sprintf(`{"change_id":"%s"}`, discardID))
	if duplicateDiscard.StatusCode != http.StatusConflict || duplicateDiscard.Body.OK || duplicateDiscard.Body.Error.Code != services.CodeConflict {
		t.Fatalf("duplicate discard should conflict: status=%d body=%#v", duplicateDiscard.StatusCode, duplicateDiscard.Body)
	}

	missingApply := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/apply", "test-token", `{"change_id":"change_missing"}`)
	if missingApply.StatusCode != http.StatusNotFound || missingApply.Body.OK || missingApply.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("missing apply should return not found: status=%d body=%#v", missingApply.StatusCode, missingApply.Body)
	}

	missingDiscard := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/discard", "test-token", `{"change_id":"change_missing"}`)
	if missingDiscard.StatusCode != http.StatusNotFound || missingDiscard.Body.OK || missingDiscard.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("missing discard should return not found: status=%d body=%#v", missingDiscard.StatusCode, missingDiscard.Body)
	}

	conflictDiff := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/diff", "test-token", `{"path":"notes.md","old":"delta","new":"zeta"}`)
	if conflictDiff.StatusCode != http.StatusOK || !conflictDiff.Body.OK {
		t.Fatalf("unexpected conflict fixture diff response: status=%d body=%#v", conflictDiff.StatusCode, conflictDiff.Body)
	}
	conflictID := stringValue(t, conflictDiff.Body.Data["change_id"])
	conflictWrite := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/write", "test-token", `{"path":"notes.md","content":"alpha changed gamma"}`)
	if conflictWrite.StatusCode != http.StatusOK || !conflictWrite.Body.OK {
		t.Fatalf("unexpected conflict fixture write response: status=%d body=%#v", conflictWrite.StatusCode, conflictWrite.Body)
	}
	conflictApply := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/apply", "test-token", fmt.Sprintf(`{"change_id":"%s"}`, conflictID))
	if conflictApply.StatusCode != http.StatusConflict || conflictApply.Body.OK || conflictApply.Body.Error.Code != services.CodeConflict {
		t.Fatalf("conflicting apply should return conflict: status=%d body=%#v", conflictApply.StatusCode, conflictApply.Body)
	}
	conflictSession := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	if conflictSession.StatusCode != http.StatusOK || !conflictSession.Body.OK {
		t.Fatalf("unexpected conflict session response: status=%d body=%#v", conflictSession.StatusCode, conflictSession.Body)
	}
	conflictPending := asList(t, asMap(t, conflictSession.Body.Data["session"])["pending_changes"])
	if !listOfMapsContains(t, conflictPending, "id", conflictID) || !listOfMapsContains(t, conflictPending, "status", "conflict") {
		t.Fatalf("conflicting apply should mark pending change as conflict: %#v", conflictPending)
	}
	var conflictItem map[string]any
	for _, item := range conflictPending {
		candidate := asMap(t, item)
		if stringValue(t, candidate["id"]) == conflictID {
			conflictItem = candidate
			break
		}
	}
	if conflictItem == nil || !strings.Contains(stringValue(t, conflictItem["error"]), "old text") {
		t.Fatalf("conflicting pending change should keep error detail: %#v", conflictItem)
	}
	conflictDiscard := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/discard", "test-token", fmt.Sprintf(`{"change_id":"%s"}`, conflictID))
	if conflictDiscard.StatusCode != http.StatusOK || !conflictDiscard.Body.OK || conflictDiscard.Body.Data["status"] != "discarded" {
		t.Fatalf("conflict discard should mark change as discarded: status=%d body=%#v", conflictDiscard.StatusCode, conflictDiscard.Body)
	}
	if stringValue(t, conflictDiscard.Body.Data["error"]) != "" {
		t.Fatalf("discarded conflict should clear error detail: %#v", conflictDiscard.Body.Data)
	}
	duplicateConflictDiscard := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/discard", "test-token", fmt.Sprintf(`{"change_id":"%s"}`, conflictID))
	if duplicateConflictDiscard.StatusCode != http.StatusConflict || duplicateConflictDiscard.Body.OK || duplicateConflictDiscard.Body.Error.Code != services.CodeConflict {
		t.Fatalf("duplicate conflict discard should conflict: status=%d body=%#v", duplicateConflictDiscard.StatusCode, duplicateConflictDiscard.Body)
	}

	rebaseDiff := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/diff", "test-token", `{"path":"notes.md","old":"changed","new":"omega"}`)
	if rebaseDiff.StatusCode != http.StatusOK || !rebaseDiff.Body.OK {
		t.Fatalf("unexpected rebase fixture diff response: status=%d body=%#v", rebaseDiff.StatusCode, rebaseDiff.Body)
	}
	rebaseID := stringValue(t, rebaseDiff.Body.Data["change_id"])
	rebaseWrite := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/write", "test-token", `{"path":"notes.md","content":"alpha current gamma"}`)
	if rebaseWrite.StatusCode != http.StatusOK || !rebaseWrite.Body.OK {
		t.Fatalf("unexpected rebase fixture write response: status=%d body=%#v", rebaseWrite.StatusCode, rebaseWrite.Body)
	}
	rebaseConflict := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/apply", "test-token", fmt.Sprintf(`{"change_id":"%s"}`, rebaseID))
	if rebaseConflict.StatusCode != http.StatusConflict || rebaseConflict.Body.OK || rebaseConflict.Body.Error.Code != services.CodeConflict {
		t.Fatalf("rebase fixture apply should conflict: status=%d body=%#v", rebaseConflict.StatusCode, rebaseConflict.Body)
	}
	rebase := doDaemonRequestWithBody(
		t,
		http.MethodPost,
		api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages",
		"test-token",
		fmt.Sprintf(`{"operator":"tester","language":"zh","message":"重新生成 change_id: %s old: current"}`, rebaseID),
	)
	if rebase.StatusCode != http.StatusOK || !rebase.Body.OK {
		t.Fatalf("unexpected natural rebase response: status=%d body=%#v", rebase.StatusCode, rebase.Body)
	}
	rebaseTool := asMap(t, asList(t, rebase.Body.Data["tool_calls"])[0])
	if rebaseTool["name"] != "workspace.files.rebase" || rebaseTool["status"] != string(contracts.StatusSucceeded) {
		t.Fatalf("unexpected rebase tool call: %#v", rebaseTool)
	}
	rebaseResult := asMap(t, rebaseTool["result"])
	rebaseNewID := stringValue(t, rebaseResult["change_id"])
	if rebaseNewID == "" || rebaseResult["rebased_from"] != rebaseID || rebaseResult["status"] != "pending" {
		t.Fatalf("unexpected rebase result: %#v", rebaseResult)
	}
	applyRebased := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/apply", "test-token", fmt.Sprintf(`{"change_id":"%s"}`, rebaseNewID))
	if applyRebased.StatusCode != http.StatusOK || !applyRebased.Body.OK || applyRebased.Body.Data["status"] != "applied" {
		t.Fatalf("rebased change should apply: status=%d body=%#v", applyRebased.StatusCode, applyRebased.Body)
	}
	readRebased := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"notes.md"}`)
	if readRebased.StatusCode != http.StatusOK || !readRebased.Body.OK || stringValue(t, readRebased.Body.Data["content"]) != "alpha omega gamma" {
		t.Fatalf("rebased apply should mutate file with original new text: status=%d body=%#v", readRebased.StatusCode, readRebased.Body)
	}
}

func TestHTTPAgentWorkspaceDiffManyGroupAPI(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vagent-workspace-diff-many"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vagent-workspace-diff-many"},
			MaxConcurrentJobs: 1,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	create := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions", "test-token", `{"operator":"tester","language":"zh"}`)
	if create.StatusCode != http.StatusCreated || !create.Body.OK {
		t.Fatalf("unexpected create agent session response: status=%d body=%#v", create.StatusCode, create.Body)
	}
	sessionID := stringValue(t, asMap(t, create.Body.Data["session"])["session_id"])
	workspaceURL := api.URL + "/api/v1/agent/sessions/" + sessionID + "/workspace"

	for _, fixture := range []struct {
		path    string
		content string
	}{
		{path: "one.md", content: "alpha one"},
		{path: "two.md", content: "beta two"},
	} {
		write := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/write", "test-token", fmt.Sprintf(`{"path":"%s","content":"%s"}`, fixture.path, fixture.content))
		if write.StatusCode != http.StatusOK || !write.Body.OK {
			t.Fatalf("unexpected fixture write for %s: status=%d body=%#v", fixture.path, write.StatusCode, write.Body)
		}
	}

	diffMany := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/diff-many", "test-token", `{"changes":[{"path":"one.md","old":"one","new":"uno"},{"path":"two.md","old":"two","new":"dos"}]}`)
	if diffMany.StatusCode != http.StatusOK || !diffMany.Body.OK {
		t.Fatalf("unexpected diff-many response: status=%d body=%#v", diffMany.StatusCode, diffMany.Body)
	}
	groupID := stringValue(t, diffMany.Body.Data["group_id"])
	if groupID == "" || diffMany.Body.Data["status"] != "pending" || int(diffMany.Body.Data["count"].(float64)) != 2 {
		t.Fatalf("unexpected diff-many result: %#v", diffMany.Body.Data)
	}
	changes := asList(t, diffMany.Body.Data["changes"])
	if len(changes) != 2 || !listOfMapsContains(t, changes, "group_id", groupID) {
		t.Fatalf("diff-many should return grouped changes: %#v", changes)
	}
	readBefore := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"one.md"}`)
	if readBefore.StatusCode != http.StatusOK || !readBefore.Body.OK || stringValue(t, readBefore.Body.Data["content"]) != "alpha one" {
		t.Fatalf("diff-many must not mutate files before apply: status=%d body=%#v", readBefore.StatusCode, readBefore.Body)
	}

	applyGroup := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/apply-group", "test-token", fmt.Sprintf(`{"group_id":"%s"}`, groupID))
	if applyGroup.StatusCode != http.StatusOK || !applyGroup.Body.OK {
		t.Fatalf("unexpected apply-group response: status=%d body=%#v", applyGroup.StatusCode, applyGroup.Body)
	}
	if applyGroup.Body.Data["group_id"] != groupID || applyGroup.Body.Data["status"] != "succeeded" ||
		int(applyGroup.Body.Data["succeeded_count"].(float64)) != 2 {
		t.Fatalf("unexpected apply-group result: %#v", applyGroup.Body.Data)
	}
	readAfterOne := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"one.md"}`)
	readAfterTwo := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"two.md"}`)
	if stringValue(t, readAfterOne.Body.Data["content"]) != "alpha uno" || stringValue(t, readAfterTwo.Body.Data["content"]) != "beta dos" {
		t.Fatalf("apply-group should mutate both files: one=%#v two=%#v", readAfterOne.Body, readAfterTwo.Body)
	}

	discardFixture := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/diff-many", "test-token", `{"changes":[{"path":"one.md","old":"uno","new":"discarded-one"},{"path":"two.md","old":"dos","new":"discarded-two"}]}`)
	if discardFixture.StatusCode != http.StatusOK || !discardFixture.Body.OK {
		t.Fatalf("unexpected discard fixture diff-many response: status=%d body=%#v", discardFixture.StatusCode, discardFixture.Body)
	}
	discardGroupID := stringValue(t, discardFixture.Body.Data["group_id"])
	discardGroup := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/discard-group", "test-token", fmt.Sprintf(`{"group_id":"%s"}`, discardGroupID))
	if discardGroup.StatusCode != http.StatusOK || !discardGroup.Body.OK || discardGroup.Body.Data["status"] != "succeeded" {
		t.Fatalf("unexpected discard-group response: status=%d body=%#v", discardGroup.StatusCode, discardGroup.Body)
	}
	readAfterDiscard := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"one.md"}`)
	if strings.Contains(stringValue(t, readAfterDiscard.Body.Data["content"]), "discarded") {
		t.Fatalf("discard-group should not mutate file: %#v", readAfterDiscard.Body)
	}

	duplicatePath := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/diff-many", "test-token", `{"changes":[{"path":"one.md","old":"uno","new":"x"},{"path":"one.md","old":"uno","new":"y"}]}`)
	if duplicatePath.StatusCode != http.StatusConflict || duplicatePath.Body.OK || duplicatePath.Body.Error.Code != services.CodeConflict {
		t.Fatalf("duplicate path in diff-many should conflict: status=%d body=%#v", duplicatePath.StatusCode, duplicatePath.Body)
	}

	atomicFixture := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/diff-many", "test-token", `{"changes":[{"path":"one.md","old":"uno","new":"eins"},{"path":"two.md","old":"dos","new":"zwei"}]}`)
	if atomicFixture.StatusCode != http.StatusOK || !atomicFixture.Body.OK {
		t.Fatalf("unexpected atomic fixture diff-many response: status=%d body=%#v", atomicFixture.StatusCode, atomicFixture.Body)
	}
	atomicGroupID := stringValue(t, atomicFixture.Body.Data["group_id"])
	conflictWrite := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/write", "test-token", `{"path":"two.md","content":"beta changed"}`)
	if conflictWrite.StatusCode != http.StatusOK || !conflictWrite.Body.OK {
		t.Fatalf("unexpected conflict fixture write: status=%d body=%#v", conflictWrite.StatusCode, conflictWrite.Body)
	}
	atomicApply := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/apply-group", "test-token", fmt.Sprintf(`{"group_id":"%s"}`, atomicGroupID))
	if atomicApply.StatusCode != http.StatusConflict || atomicApply.Body.OK || atomicApply.Body.Error.Code != services.CodeConflict {
		t.Fatalf("conflicting apply-group should return conflict: status=%d body=%#v", atomicApply.StatusCode, atomicApply.Body)
	}
	if failedRaw, ok := atomicApply.Body.Error.Details["failed"]; !ok || len(asList(t, failedRaw)) == 0 {
		t.Fatalf("conflicting apply-group should return failed details: %#v", atomicApply.Body.Error.Details)
	}
	readAtomicOne := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"one.md"}`)
	readAtomicTwo := doDaemonRequestWithBody(t, http.MethodPost, workspaceURL+"/files/read", "test-token", `{"path":"two.md"}`)
	if stringValue(t, readAtomicOne.Body.Data["content"]) != "alpha uno" || stringValue(t, readAtomicTwo.Body.Data["content"]) != "beta changed" {
		t.Fatalf("failed apply-group must not partially apply files: one=%#v two=%#v", readAtomicOne.Body, readAtomicTwo.Body)
	}
	atomicSession := doDaemonRequest(t, api.URL+"/api/v1/agent/sessions/"+sessionID, "test-token")
	atomicPending := asList(t, asMap(t, atomicSession.Body.Data["session"])["pending_changes"])
	if !listOfMapsContains(t, atomicPending, "group_id", atomicGroupID) || !listOfMapsContains(t, atomicPending, "status", "conflict") {
		t.Fatalf("failed apply-group should mark conflicting item while leaving group visible: %#v", atomicPending)
	}

	naturalApplyMissing := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/agent/sessions/"+sessionID+"/messages", "test-token", fmt.Sprintf(`{"operator":"tester","language":"zh","message":"应用 group_id: %s"}`, discardGroupID))
	if naturalApplyMissing.StatusCode != http.StatusOK || !naturalApplyMissing.Body.OK {
		t.Fatalf("unexpected natural apply-group response: status=%d body=%#v", naturalApplyMissing.StatusCode, naturalApplyMissing.Body)
	}
	tool := asMap(t, asList(t, naturalApplyMissing.Body.Data["tool_calls"])[0])
	if tool["name"] != "workspace.files.apply" || tool["status"] != string(contracts.StatusFailed) {
		t.Fatalf("natural apply-group with no pending changes should fail visibly: %#v", tool)
	}
}

func TestHTTPScanJobConcurrencyAndCancel(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	started := make(chan struct{}, 2)
	runner := func(ctx context.Context, req coreruntime.WebScanRequest) (coreruntime.WebScanResult, error) {
		started <- struct{}{}
		<-ctx.Done()
		return coreruntime.WebScanResult{}, ctx.Err()
	}

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vcancel-api"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		jobs: newJobManager(jobManagerConfig{
			RunsDir:           runsDir,
			Build:             services.BuildInfo{Name: "CyberAgent", Version: "vcancel-api"},
			MaxConcurrentJobs: 1,
			Runner:            runner,
		}),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	start := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/scans/web", "test-token", `{"target":"http://127.0.0.1:8080","scope":"127.0.0.1"}`)
	if start.StatusCode != http.StatusOK || !start.Body.OK {
		t.Fatalf("unexpected scan start response: status=%d body=%#v", start.StatusCode, start.Body)
	}
	jobID := stringValue(t, start.Body.Data["job_id"])
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}

	conflict := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/scans/web", "test-token", `{"target":"http://127.0.0.1:8081","scope":"127.0.0.1"}`)
	if conflict.StatusCode != http.StatusConflict || conflict.Body.OK || conflict.Body.Error.Code != services.CodeConflict {
		t.Fatalf("unexpected concurrent scan response: status=%d body=%#v", conflict.StatusCode, conflict.Body)
	}

	cancel := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/jobs/"+jobID+"/cancel", "test-token", `{}`)
	if cancel.StatusCode != http.StatusOK || !cancel.Body.OK || cancel.Body.Data["job_id"] != jobID || cancel.Body.Data["status"] != string(contracts.StatusCancelled) {
		t.Fatalf("unexpected cancel response: status=%d body=%#v", cancel.StatusCode, cancel.Body)
	}
	job := waitDaemonJobStatus(t, api.URL, "test-token", jobID, 2*time.Second, contracts.StatusCancelled)
	if job["status"] != string(contracts.StatusCancelled) || asMap(t, job["error"])["code"] != string(services.CodeJobCancelled) {
		t.Fatalf("unexpected cancelled job: %#v", job)
	}

	missing := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/jobs/missing/cancel", "test-token", `{}`)
	if missing.StatusCode != http.StatusNotFound || missing.Body.OK || missing.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("unexpected missing cancel response: status=%d body=%#v", missing.StatusCode, missing.Body)
	}

	traversal := doDaemonRequest(t, api.URL+"/api/v1/jobs/../secret", "test-token")
	if traversal.StatusCode != http.StatusNotFound && traversal.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected job traversal response: status=%d body=%#v", traversal.StatusCode, traversal.Body)
	}

	wrongMethod := doDaemonRequestWithBody(t, http.MethodDelete, api.URL+"/api/v1/jobs/"+jobID, "test-token", `{}`)
	if wrongMethod.StatusCode != http.StatusBadRequest || wrongMethod.Body.OK || wrongMethod.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected wrong method response: status=%d body=%#v", wrongMethod.StatusCode, wrongMethod.Body)
	}
}

func TestJobManagerMarksFailedRunnerErrors(t *testing.T) {
	wantErr := errors.New("runner failed")
	manager := newJobManager(jobManagerConfig{
		RunsDir:           t.TempDir(),
		MaxConcurrentJobs: 1,
		Runner: func(context.Context, coreruntime.WebScanRequest) (coreruntime.WebScanResult, error) {
			return coreruntime.WebScanResult{}, wantErr
		},
	})
	start, err := manager.StartWebScan(context.Background(), startWebScanJobRequest{
		Target:          "http://127.0.0.1:8080",
		Scope:           "127.0.0.1",
		NoExternalTools: true,
	})
	if err != nil {
		t.Fatalf("StartWebScan() error = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		job, err := manager.GetJob(context.Background(), start.JobID)
		if err != nil {
			t.Fatalf("GetJob() error = %v", err)
		}
		if job.Status == contracts.StatusFailed {
			if job.Error == nil || job.Error.Code != services.CodeInternal || !strings.Contains(job.Error.Message, wantErr.Error()) {
				t.Fatalf("unexpected failed job error: %#v", job.Error)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not fail before deadline: %#v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type daemonHTTPResponse struct {
	StatusCode int
	Body       daemonTestEnvelope
}

type daemonTestEnvelope struct {
	OK    bool           `json:"ok"`
	Data  map[string]any `json:"data"`
	Error *ErrorBody     `json:"error"`
}

func doDaemonRequest(t *testing.T, url, token string) daemonHTTPResponse {
	t.Helper()
	return doDaemonRequestWithBody(t, http.MethodGet, url, token, "")
}

func doDaemonRequestWithBody(t *testing.T, method, url, token, body string) daemonHTTPResponse {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	var decoded daemonTestEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return daemonHTTPResponse{StatusCode: resp.StatusCode, Body: decoded}
}

func writeAgentSessionFile(t *testing.T, storeDir string, session agentSession) {
	t.Helper()
	if session.SessionID == "" {
		t.Fatal("agent session id is required")
	}
	session.normalize()
	encoded, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		t.Fatalf("marshal agent session: %v", err)
	}
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("create agent session store: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, session.SessionID+".json"), encoded, 0o600); err != nil {
		t.Fatalf("write agent session file: %v", err)
	}
}

func readAgentSessionFile(t *testing.T, storeDir, sessionID string) agentSession {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(storeDir, sessionID+".json"))
	if err != nil {
		t.Fatalf("read agent session file: %v", err)
	}
	var session agentSession
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("decode agent session file: %v", err)
	}
	session.normalize()
	return session
}

func writeDaemonRunJob(t *testing.T, root, runID string, status contracts.Status, createdAt, finishedAt time.Time) {
	t.Helper()
	layout, err := runs.NewManager(root).Create(runID)
	if err != nil {
		t.Fatalf("create run job: %v", err)
	}
	startedAt := createdAt.Add(time.Second)
	job := contracts.Job{
		ID:         runID,
		Kind:       contracts.JobKindWebScan,
		Status:     status,
		TargetID:   "target_" + runID,
		ScopeID:    "scope_" + runID,
		Profile:    "safe",
		Workspace:  layout.Path,
		CreatedAt:  createdAt,
		StartedAt:  &startedAt,
		FinishedAt: &finishedAt,
		Options: contracts.JobOptions{
			Interactive:        true,
			UseLLM:             false,
			AllowExternalTools: false,
			ReportFormats:      []string{"json", "markdown"},
		},
		Summary: contracts.JobSummary{
			StepsTotal:     3,
			StepsSucceeded: 3,
			StepsFailed:    0,
			FindingsTotal:  1,
		},
		Metadata: contracts.Metadata{},
	}
	if status == contracts.StatusFailed {
		job.Summary.StepsSucceeded = 1
		job.Summary.StepsFailed = 2
	}
	encoded, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		t.Fatalf("marshal run job: %v", err)
	}
	if err := os.WriteFile(layout.JobPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write run job: %v", err)
	}
}

func writeDaemonRunReport(t *testing.T, root, runID string, generated time.Time) {
	t.Helper()
	layout := runs.NewLayout(root, runID)
	job := contracts.NewWebScanJob(runID, "target_"+runID, "scope_"+runID, "safe", layout.Path)
	job.Status = contracts.StatusRunning
	job.CreatedAt = generated
	target, err := contracts.NewWebTarget("target_"+runID, "http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("create report target: %v", err)
	}
	scope := contracts.NewScope("scope_"+runID, contracts.ScopeRules{Hosts: []string{"127.0.0.1"}})
	plan := contracts.NewPlan("plan_"+runID, runID, "test", []string{"step_001"})
	artifact := contracts.Artifact{
		ID:        "artifact_headers_001",
		JobID:     runID,
		StepID:    "step_001",
		Type:      contracts.ArtifactJSON,
		Path:      "artifacts/http/headers.json",
		MIMEType:  "application/json",
		CreatedAt: generated,
	}
	artifactStore := artifactruntime.NewStore(layout.Path)
	if err := artifactStore.WriteJSON("artifacts/http/headers.json", map[string]string{
		"status":                  "200 OK",
		"content-security-policy": "",
	}); err != nil {
		t.Fatalf("write report artifact: %v", err)
	}
	if _, err := artifactStore.WriteIndex([]contracts.Artifact{artifact}); err != nil {
		t.Fatalf("write report artifact index: %v", err)
	}
	evidence := contracts.Evidence{
		ID:         "evidence_001",
		JobID:      runID,
		StepID:     "step_001",
		ArtifactID: artifact.ID,
		Type:       contracts.EvidenceHTTPResponse,
		Title:      "HTTP status",
		Summary:    "200 OK with missing Content-Security-Policy header",
		Confidence: contracts.ConfidenceHigh,
		CreatedAt:  generated,
	}
	finding := contracts.Finding{
		ID:              "finding_001",
		JobID:           runID,
		Title:           "Missing CSP",
		Severity:        contracts.SeverityLow,
		Confidence:      contracts.ConfidenceHigh,
		Category:        "headers",
		AffectedTargets: []string{target.Value},
		EvidenceIDs:     []string{evidence.ID},
		Description:     "CSP is missing.",
		Impact:          "Browser hardening is weaker.",
		Remediation:     "Add a CSP header",
		Source:          contracts.FindingSource{Tool: "builtin.header_checker", StepID: "step_001"},
		CreatedAt:       generated,
	}
	if _, err := reporting.Generate(layout.Path, reporting.Input{
		Job:           job,
		Target:        target,
		Scope:         scope,
		Plan:          plan,
		Artifacts:     []contracts.Artifact{artifact},
		Evidence:      []contracts.Evidence{evidence},
		Findings:      []contracts.Finding{finding},
		GeneratedTime: generated,
	}); err != nil {
		t.Fatalf("write run report: %v", err)
	}
	encodedFindings, err := json.MarshalIndent([]contracts.Finding{finding}, "", "  ")
	if err != nil {
		t.Fatalf("marshal findings path: %v", err)
	}
	if err := os.WriteFile(layout.FindingsPath, append(encodedFindings, '\n'), 0o644); err != nil {
		t.Fatalf("write findings path: %v", err)
	}
}

func writeDaemonApprovalRun(t *testing.T, root, runID string, requests []contracts.HITLRequest) {
	t.Helper()
	layout, err := runs.NewManager(root).Create(runID)
	if err != nil {
		t.Fatalf("create approval run: %v", err)
	}
	encoded, err := json.MarshalIndent(requests, "", "  ")
	if err != nil {
		t.Fatalf("marshal approvals: %v", err)
	}
	if err := os.WriteFile(layout.ApprovalsPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write approvals: %v", err)
	}
}

func daemonTestApproval(id, jobID, stepID string) contracts.HITLRequest {
	return contracts.HITLRequest{
		ID:               id,
		JobID:            jobID,
		StepID:           stepID,
		PolicyDecisionID: "policy_" + stepID,
		Status:           contracts.HITLPending,
		Reason:           "requires approval",
		RequestedAction:  "execute_step",
		Options:          []string{"approve", "deny"},
		CreatedAt:        contracts.Now(),
	}
}

func writeDaemonTestPlugin(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	manifest := `id: plugin_alpha
name: alpha
version: 0.1.0
type: tool
runtime: external_command
risk: low
requires_approval: false
dependencies:
  - name: definitely-missing-cyberagent-test-binary
    kind: binary
    required: false
entrypoint:
  kind: command_builder
  path: plugins/alpha/plugin.yaml
artifact_policy:
  store_stdout: true
  store_stderr: true
  redact: true
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
}

func asList(t *testing.T, value any) []any {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value is %T, want []any: %#v", value, value)
	}
	return items
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	item, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value is %T, want map[string]any: %#v", value, value)
	}
	return item
}

func asStringList(t *testing.T, value any) []string {
	t.Helper()
	items := asList(t, value)
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, stringValue(t, item))
	}
	return out
}

func stringListContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func listOfMapsContains(t *testing.T, items []any, key, want string) bool {
	t.Helper()
	for _, item := range items {
		if stringValue(t, asMap(t, item)[key]) == want {
			return true
		}
	}
	return false
}

func findMapByKey(t *testing.T, items []any, key, want string) map[string]any {
	t.Helper()
	for _, item := range items {
		mapped := asMap(t, item)
		if stringValue(t, mapped[key]) == want {
			return mapped
		}
	}
	t.Fatalf("missing map with %s=%q in %#v", key, want, items)
	return nil
}

func stringValue(t *testing.T, value any) string {
	t.Helper()
	s, ok := value.(string)
	if !ok {
		t.Fatalf("value is %T, want string: %#v", value, value)
	}
	return s
}

func boolValue(t *testing.T, value any) bool {
	t.Helper()
	b, ok := value.(bool)
	if !ok {
		t.Fatalf("value is %T, want bool: %#v", value, value)
	}
	return b
}

func waitDaemonJobStatus(t *testing.T, baseURL, token, jobID string, timeout time.Duration, want ...contracts.Status) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	wanted := map[string]struct{}{}
	for _, status := range want {
		wanted[string(status)] = struct{}{}
	}
	for {
		resp := doDaemonRequest(t, baseURL+"/api/v1/jobs/"+jobID, token)
		if resp.StatusCode != http.StatusOK || !resp.Body.OK {
			t.Fatalf("unexpected job status response: status=%d body=%#v", resp.StatusCode, resp.Body)
		}
		status := stringValue(t, resp.Body.Data["status"])
		if _, ok := wanted[status]; ok {
			return resp.Body.Data
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s did not reach %v before deadline, last=%#v", jobID, want, resp.Body.Data)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
