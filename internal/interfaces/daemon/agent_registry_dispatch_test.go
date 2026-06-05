package daemon

import (
	"context"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func TestRegistryHandlerDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("daemon.health")
	if !ok {
		t.Fatal("daemon.health missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		if in.SessionID != "agent_registry_dispatch" || in.Operator != "tester" || in.Language != "en" || in.OperationLevel != "safe_scan" {
			t.Fatalf("unexpected handler input: %#v", in)
		}
		if got := stringFromAny(in.RawArgs["probe"]); got != "ok" {
			t.Fatalf("handler RawArgs probe = %q, want ok", got)
		}
		tool := runningAgentToolCall("daemon.health", "registry.handler")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"dispatched": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "registry handler dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, calls, err := mgr.executeAgentPlannerAction(context.Background(), "agent_registry_dispatch", "en", "tester", "safe_scan", routerServices{}, agentPlannerAction{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "ok"},
	}, nil, "")
	if err != nil {
		t.Fatalf("executeAgentPlannerAction() error = %v", err)
	}
	if !called {
		t.Fatal("registry handler was not called")
	}
	if len(messages) != 1 || messages[0].Content != "registry handler dispatched" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if len(calls) != 1 || calls[0].Result["dispatched"] != true {
		t.Fatalf("unexpected tool calls: %#v", calls)
	}
}

func TestMCPToolNamesUseDoubleUnderscoreNamespace(t *testing.T) {
	name := agentMCPToolName("preset-ffuf", "content.discovery")
	if name != "mcp__preset-ffuf__content.discovery" {
		t.Fatalf("agentMCPToolName() = %q", name)
	}
	if normalized := normalizeAgentPlannerActionName("mcp.preset-ffuf.content.discovery"); normalized != name {
		t.Fatalf("legacy MCP action normalized to %q, want %q", normalized, name)
	}
	if normalized := normalizeAgentPlannerActionName("mcp__preset-ffuf__content.discovery"); normalized != name {
		t.Fatalf("V1 MCP action normalized to %q, want %q", normalized, name)
	}
}

func TestRegistryBuiltinHandlersAreAttached(t *testing.T) {
	for _, builtin := range toolregistry.Builtins() {
		spec, ok := toolRegistry().Lookup(builtin.Name)
		if !ok {
			t.Fatalf("%s missing from registry", builtin.Name)
		}
		if spec.Handler == nil {
			t.Fatalf("%s Handler is nil", builtin.Name)
		}
	}
}

func TestRegistryBuiltinMissingHandlerFailsWithoutLegacyFallback(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("workspace.files.list")
	if !ok {
		t.Fatal("workspace.files.list missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()
	spec.Handler = nil

	_, _, err := mgr.executeAgentPlannerAction(context.Background(), "agent_registry_dispatch_no_fallback", "en", "tester", "read_only", routerServices{}, agentPlannerAction{
		Name:      "workspace.files.list",
		Arguments: map[string]any{"path": "."},
	}, nil, "")
	if err == nil {
		t.Fatal("executeAgentPlannerAction() error = nil, want missing handler error")
	}
	if got := services.ErrorCodeOf(err); got != services.CodeInternal {
		t.Fatalf("ErrorCodeOf(err) = %q, want %q (err=%v)", got, services.CodeInternal, err)
	}
}

func TestRoutePromptReadOnlySystemUsesHeuristicRegistryDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("daemon.health")
	if !ok {
		t.Fatal("daemon.health missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		tool := runningAgentToolCall("daemon.health", "route.registry")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"route_registry": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("en", "tool", "route registry dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_registry", "daemon health", "en", "tester", "read_only", routerServices{})
	if err != nil {
		t.Fatalf("routePrompt() error = %v", err)
	}
	if !called {
		t.Fatal("routePrompt did not dispatch through the registry handler")
	}
	if len(messages) != 1 || messages[0].Content != "route registry dispatched" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if len(calls) != 1 || calls[0].Name != "daemon.health" || calls[0].Result["route_registry"] != true {
		t.Fatalf("unexpected tool calls: %#v", calls)
	}
	if calls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("route registry tool source metadata = %#v", calls[0].Metadata)
	}
	if messages[0].ToolCalls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("route registry message tool source metadata = %#v", messages[0].ToolCalls[0].Metadata)
	}
}

func TestRoutePromptSafeHTTPUsesHeuristicRegistryDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("http.request")
	if !ok {
		t.Fatal("http.request missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		if in.SessionID != "agent_route_http_registry" || in.Operator != "tester" || in.Language != "zh" || in.OperationLevel != "safe_scan" {
			t.Fatalf("unexpected handler input: %#v", in)
		}
		if got := stringFromAny(in.RawArgs["url"]); got != "https://example.com/api" {
			t.Fatalf("handler RawArgs url = %q, want https://example.com/api", got)
		}
		if got := stringFromAny(in.RawArgs["method"]); got != "HEAD" {
			t.Fatalf("handler RawArgs method = %q, want HEAD", got)
		}
		tool := runningAgentToolCall("http.request", "route.http.registry")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"route_registry": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("zh", "tool", "safe http registry dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_http_registry", "获取 https://example.com/api 的响应头和状态码", "zh", "tester", "safe_scan", routerServices{})
	if err != nil {
		t.Fatalf("routePrompt() error = %v", err)
	}
	if !called {
		t.Fatal("routePrompt did not dispatch safe http.request through the registry handler")
	}
	if len(messages) != 1 || messages[0].Content != "safe http registry dispatched" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if len(calls) != 1 || calls[0].Name != "http.request" || calls[0].Result["route_registry"] != true {
		t.Fatalf("unexpected tool calls: %#v", calls)
	}
	if calls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("safe http route tool source metadata = %#v", calls[0].Metadata)
	}
	if messages[0].ToolCalls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("safe http route message tool source metadata = %#v", messages[0].ToolCalls[0].Metadata)
	}
	if agentSafeHTTPRouteAction("http.request", map[string]any{"method": "POST"}) {
		t.Fatal("POST http.request must stay outside the safe HTTP registry route slice")
	}
}

func TestRoutePromptSafePortScanUsesHeuristicRegistryDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("ports.scan.safe")
	if !ok {
		t.Fatal("ports.scan.safe missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		if in.SessionID != "agent_route_port_registry" || in.Operator != "tester" || in.Language != "zh" || in.OperationLevel != "safe_scan" {
			t.Fatalf("unexpected handler input: %#v", in)
		}
		if got := stringFromAny(in.RawArgs["host"]); got != "example.com" {
			t.Fatalf("handler RawArgs host = %q, want example.com", got)
		}
		if got := agentPortSpecFromPlannerArgument(in.RawArgs["ports"]); got != "80,443" {
			t.Fatalf("handler RawArgs ports = %q, want 80,443 (raw=%#v)", got, in.RawArgs["ports"])
		}
		tool := runningAgentToolCall("ports.scan.safe", "route.port.registry")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"route_registry": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("zh", "tool", "safe port registry dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_port_registry", "扫描 example.com 开放端口 ports=80,443", "zh", "tester", "safe_scan", routerServices{})
	if err != nil {
		t.Fatalf("routePrompt() error = %v", err)
	}
	if !called {
		t.Fatal("routePrompt did not dispatch ports.scan.safe through the registry handler")
	}
	if len(messages) != 1 || messages[0].Content != "safe port registry dispatched" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if len(calls) != 1 || calls[0].Name != "ports.scan.safe" || calls[0].Result["route_registry"] != true {
		t.Fatalf("unexpected tool calls: %#v", calls)
	}
	if calls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("safe port route tool source metadata = %#v", calls[0].Metadata)
	}
	if messages[0].ToolCalls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("safe port route message tool source metadata = %#v", messages[0].ToolCalls[0].Metadata)
	}
	if agentSafePortScanRouteAction("scan.web.safe", map[string]any{"host": "example.com"}) {
		t.Fatal("scan.web.safe must stay outside the safe port scan registry route slice")
	}
}

func TestRoutePromptSafeScanUsesHeuristicRegistryDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("scan.web.safe")
	if !ok {
		t.Fatal("scan.web.safe missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		if in.SessionID != "agent_route_scan_registry" || in.Operator != "tester" || in.Language != "zh" || in.OperationLevel != "safe_scan" {
			t.Fatalf("unexpected handler input: %#v", in)
		}
		if got := stringFromAny(in.RawArgs["target"]); got != "https://example.com" {
			t.Fatalf("handler RawArgs target = %q, want https://example.com", got)
		}
		if got := stringFromAny(in.RawArgs["scope"]); got != "https://example.com" {
			t.Fatalf("handler RawArgs scope = %q, want https://example.com", got)
		}
		tool := runningAgentToolCall("POST /api/v1/scans/web", "route.scan.registry")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"route_registry": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("zh", "tool", "safe scan registry dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_scan_registry", "对 https://example.com 做安全扫描 scope=https://example.com", "zh", "tester", "safe_scan", routerServices{})
	if err != nil {
		t.Fatalf("routePrompt() error = %v", err)
	}
	if !called {
		t.Fatal("routePrompt did not dispatch scan.web.safe through the registry handler")
	}
	if len(messages) != 1 || messages[0].Content != "safe scan registry dispatched" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if len(calls) != 1 || calls[0].Name != "POST /api/v1/scans/web" || calls[0].Result["route_registry"] != true {
		t.Fatalf("unexpected tool calls: %#v", calls)
	}
	if calls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("safe scan route tool source metadata = %#v", calls[0].Metadata)
	}
	if messages[0].ToolCalls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("safe scan route message tool source metadata = %#v", messages[0].ToolCalls[0].Metadata)
	}
	if agentSafeScanRouteAction("ports.scan.safe", map[string]any{"target": "https://example.com"}) {
		t.Fatal("ports.scan.safe must stay outside the safe scan registry route slice")
	}
}

func TestRoutePromptWorkspaceDiffUsesHeuristicRegistryDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("workspace.files.diff")
	if !ok {
		t.Fatal("workspace.files.diff missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		if in.SessionID != "agent_route_diff_registry" || in.Operator != "tester" || in.Language != "zh" || in.OperationLevel != "workspace_write" {
			t.Fatalf("unexpected handler input: %#v", in)
		}
		if got := stringFromAny(in.RawArgs["path"]); got != "notes.md" {
			t.Fatalf("handler RawArgs path = %q, want notes.md", got)
		}
		if got := stringFromAny(in.RawArgs["old"]); got != "hello" {
			t.Fatalf("handler RawArgs old = %q, want hello", got)
		}
		if got := stringFromAny(in.RawArgs["new"]); got != "world" {
			t.Fatalf("handler RawArgs new = %q, want world", got)
		}
		tool := runningAgentToolCall("workspace.files.diff", "notes.md")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"route_registry": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("zh", "tool", "workspace diff registry dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	actions := agentHeuristicActionsFromPrompt("预览修改文件 notes.md old: hello new: world")
	if len(actions) != 1 || normalizeAgentPlannerActionName(actions[0].Name) != "workspace.files.diff" {
		t.Fatalf("diff prompt should produce workspace.files.diff action before patch: %#v", actions)
	}

	messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_diff_registry", "预览修改文件 notes.md old: hello new: world", "zh", "tester", "workspace_write", routerServices{})
	if err != nil {
		t.Fatalf("routePrompt() error = %v", err)
	}
	if !called {
		t.Fatal("routePrompt did not dispatch workspace.files.diff through the registry handler")
	}
	if len(messages) != 1 || messages[0].Content != "workspace diff registry dispatched" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if len(calls) != 1 || calls[0].Name != "workspace.files.diff" || calls[0].Result["route_registry"] != true {
		t.Fatalf("unexpected tool calls: %#v", calls)
	}
	if calls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("workspace diff route tool source metadata = %#v", calls[0].Metadata)
	}
	if messages[0].ToolCalls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("workspace diff route message tool source metadata = %#v", messages[0].ToolCalls[0].Metadata)
	}
	if agentWorkspaceStagedChangeRouteAction("workspace.files.patch", map[string]any{"path": "notes.md"}) {
		t.Fatal("workspace.files.patch must stay outside the staged-change registry route slice")
	}
}

func TestRoutePromptWorkspaceRebaseUsesHeuristicRegistryDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("workspace.files.rebase")
	if !ok {
		t.Fatal("workspace.files.rebase missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		if in.SessionID != "agent_route_rebase_registry" || in.Operator != "tester" || in.Language != "zh" || in.OperationLevel != "workspace_write" {
			t.Fatalf("unexpected handler input: %#v", in)
		}
		if got := stringFromAny(in.RawArgs["change_id"]); got != "change_20260605_notes" {
			t.Fatalf("handler RawArgs change_id = %q, want change_20260605_notes", got)
		}
		if got := stringFromAny(in.RawArgs["old"]); got != "current text" {
			t.Fatalf("handler RawArgs old = %q, want current text", got)
		}
		if got := stringFromAny(in.RawArgs["new"]); got != "updated text" {
			t.Fatalf("handler RawArgs new = %q, want updated text", got)
		}
		tool := runningAgentToolCall("workspace.files.rebase", "change_20260605_notes")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"route_registry": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("zh", "tool", "workspace rebase registry dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_rebase_registry", "重新生成 change_id: change_20260605_notes old: current text new: updated text", "zh", "tester", "workspace_write", routerServices{})
	if err != nil {
		t.Fatalf("routePrompt() error = %v", err)
	}
	if !called {
		t.Fatal("routePrompt did not dispatch workspace.files.rebase through the registry handler")
	}
	if len(messages) != 1 || messages[0].Content != "workspace rebase registry dispatched" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if len(calls) != 1 || calls[0].Name != "workspace.files.rebase" || calls[0].Result["route_registry"] != true {
		t.Fatalf("unexpected tool calls: %#v", calls)
	}
	if calls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("workspace rebase route tool source metadata = %#v", calls[0].Metadata)
	}
	if messages[0].ToolCalls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("workspace rebase route message tool source metadata = %#v", messages[0].ToolCalls[0].Metadata)
	}
}

func TestRoutePromptWorkspaceMutationUsesHeuristicRegistryDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	tests := []struct {
		name      string
		prompt    string
		tool      string
		wantArgs  map[string]string
		wantBools map[string]bool
	}{
		{
			name:     "write",
			prompt:   "写入文件 notes.md 内容: hello",
			tool:     "workspace.files.write",
			wantArgs: map[string]string{"path": "notes.md", "content": "hello"},
		},
		{
			name:     "patch",
			prompt:   "修改文件 notes.md old: hello new: world",
			tool:     "workspace.files.patch",
			wantArgs: map[string]string{"path": "notes.md", "old": "hello", "new": "world"},
		},
		{
			name:     "apply",
			prompt:   "应用 change_id: change_20260605_notes",
			tool:     "workspace.files.apply",
			wantArgs: map[string]string{"change_id": "change_20260605_notes"},
		},
		{
			name:      "apply_all",
			prompt:    "应用所有待应用变更",
			tool:      "workspace.files.apply",
			wantBools: map[string]bool{"all": true},
		},
		{
			name:     "discard",
			prompt:   "丢弃 change_id: change_20260605_notes",
			tool:     "workspace.files.discard",
			wantArgs: map[string]string{"change_id": "change_20260605_notes"},
		},
		{
			name:      "discard_all",
			prompt:    "丢弃所有待应用变更",
			tool:      "workspace.files.discard",
			wantBools: map[string]bool{"all": true},
		},
		{
			name:     "delete",
			prompt:   "删除文件 notes.md",
			tool:     "workspace.files.delete",
			wantArgs: map[string]string{"path": "notes.md"},
		},
		{
			name:     "move",
			prompt:   "重命名文件 notes.md 为 archive/notes.md",
			tool:     "workspace.files.move",
			wantArgs: map[string]string{"from": "notes.md", "to": "archive/notes.md"},
		},
		{
			name:     "copy",
			prompt:   "复制文件 notes.md 到 archive/notes-copy.md",
			tool:     "workspace.files.copy",
			wantArgs: map[string]string{"from": "notes.md", "to": "archive/notes-copy.md"},
		},
		{
			name:     "mkdir",
			prompt:   "创建目录 reports",
			tool:     "workspace.files.mkdir",
			wantArgs: map[string]string{"path": "reports"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := toolRegistry().Lookup(tt.tool)
			if !ok {
				t.Fatalf("%s missing from registry", tt.tool)
			}
			oldHandler := spec.Handler
			t.Cleanup(func() { spec.Handler = oldHandler })

			called := false
			spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
				called = true
				if in.SessionID != "agent_route_workspace_mutation_"+tt.name || in.Operator != "tester" || in.Language != "zh" || in.OperationLevel != "workspace_write" {
					t.Fatalf("unexpected handler input: %#v", in)
				}
				for key, want := range tt.wantArgs {
					if got := stringFromAny(in.RawArgs[key]); got != want {
						t.Fatalf("handler RawArgs[%q] = %q, want %q (raw=%#v)", key, got, want, in.RawArgs)
					}
				}
				for key, want := range tt.wantBools {
					if got := boolFromAny(in.RawArgs[key]); got != want {
						t.Fatalf("handler RawArgs[%q] = %v, want %v (raw=%#v)", key, got, want, in.RawArgs)
					}
				}
				tool := runningAgentToolCall(tt.tool, "route.workspace.mutation.registry")
				tool = finishAgentToolCall(tool, nil)
				tool.Result = map[string]any{"route_registry": true}
				return agentRegistryHandlerResult{
					Messages:  []agentMessage{mgr.message("zh", "tool", "workspace mutation registry dispatched", []agentToolCall{tool}, nil)},
					ToolCalls: []agentToolCall{tool},
				}, nil
			}

			actions := agentHeuristicActionsFromPrompt(tt.prompt)
			if len(actions) != 1 || normalizeAgentPlannerActionName(actions[0].Name) != tt.tool {
				t.Fatalf("workspace mutation prompt should produce %s action: %#v", tt.tool, actions)
			}

			messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_workspace_mutation_"+tt.name, tt.prompt, "zh", "tester", "workspace_write", routerServices{})
			if err != nil {
				t.Fatalf("routePrompt() error = %v", err)
			}
			if !called {
				t.Fatalf("routePrompt did not dispatch %s through the registry handler", tt.tool)
			}
			if len(messages) != 1 || messages[0].Content != "workspace mutation registry dispatched" {
				t.Fatalf("unexpected messages: %#v", messages)
			}
			if len(calls) != 1 || calls[0].Name != tt.tool || calls[0].Result["route_registry"] != true {
				t.Fatalf("unexpected tool calls: %#v", calls)
			}
			if calls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
				t.Fatalf("workspace mutation route tool source metadata = %#v", calls[0].Metadata)
			}
			if messages[0].ToolCalls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
				t.Fatalf("workspace mutation route message tool source metadata = %#v", messages[0].ToolCalls[0].Metadata)
			}
		})
	}

	if agentWorkspaceMutationRouteAction("workspace.files.diff", map[string]any{"path": "notes.md"}) {
		t.Fatal("workspace.files.diff must stay outside the mutation registry route slice")
	}
	if agentWorkspaceMutationRouteAction("workspace.files.apply", map[string]any{}) {
		t.Fatal("workspace.files.apply without change/group/all must stay outside the mutation registry route slice")
	}
}

func TestRoutePromptWorkspaceMutationRegistryDispatchPreservesOperationGate(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("workspace.files.write")
	if !ok {
		t.Fatal("workspace.files.write missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		tool := runningAgentToolCall("workspace.files.write", "route.workspace.mutation.registry")
		tool = finishAgentToolCall(tool, nil)
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("zh", "tool", "unexpected write", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	prompt := "写入文件 notes.md 内容: hello"
	requiredLevel := agentOperationLevelRequiredForPrompt(prompt)
	if requiredLevel != "workspace_write" {
		t.Fatalf("workspace mutation prompt required level = %q, want workspace_write", requiredLevel)
	}
	if agentOperationLevelAtLeast("read_only", requiredLevel) {
		t.Fatalf("read_only unexpectedly satisfies required level %q", requiredLevel)
	}

	messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_workspace_mutation_gate", prompt, "zh", "tester", "read_only", routerServices{})
	if err != nil {
		t.Fatalf("routePrompt() error = %v", err)
	}
	if called {
		t.Fatal("workspace mutation registry handler was called despite insufficient operation level")
	}
	if len(calls) != 0 {
		t.Fatalf("operation gate should not return tool calls: %#v", calls)
	}
	if len(messages) != 1 || messages[0].Role != "agent" || messages[0].Content == "" {
		t.Fatalf("unexpected operation gate messages: %#v", messages)
	}
}

func TestRoutePromptApprovalDecisionUsesHeuristicRegistryDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	tests := []struct {
		name        string
		prompt      string
		tool        string
		wantComment string
	}{
		{
			name:        "approve",
			prompt:      "批准 run_abc approval_123 comment: approved from registry route",
			tool:        "runs.approvals.approve",
			wantComment: "approved from registry route",
		},
		{
			name:        "deny",
			prompt:      "deny run_abc approval_123 comment: denied from registry route",
			tool:        "runs.approvals.deny",
			wantComment: "denied from registry route",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := toolRegistry().Lookup(tt.tool)
			if !ok {
				t.Fatalf("%s missing from registry", tt.tool)
			}
			oldHandler := spec.Handler
			t.Cleanup(func() { spec.Handler = oldHandler })

			called := false
			spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
				called = true
				if in.SessionID != "agent_route_approval_"+tt.name || in.Operator != "tester" || in.Language != "zh" || in.OperationLevel != "approval_ops" {
					t.Fatalf("unexpected handler input: %#v", in)
				}
				if got := stringFromAny(in.RawArgs["run_id"]); got != "run_abc" {
					t.Fatalf("handler RawArgs run_id = %q, want run_abc", got)
				}
				if got := stringFromAny(in.RawArgs["approval_id"]); got != "approval_123" {
					t.Fatalf("handler RawArgs approval_id = %q, want approval_123", got)
				}
				if got := stringFromAny(in.RawArgs["comment"]); !strings.Contains(got, tt.wantComment) {
					t.Fatalf("handler RawArgs comment = %q, want %q", got, tt.wantComment)
				}
				tool := runningAgentToolCall(tt.tool, "route.approval.registry")
				tool = finishAgentToolCall(tool, nil)
				tool.Result = map[string]any{"route_registry": true}
				return agentRegistryHandlerResult{
					Messages:  []agentMessage{mgr.message("zh", "tool", "approval operation registry dispatched", []agentToolCall{tool}, nil)},
					ToolCalls: []agentToolCall{tool},
				}, nil
			}

			actions := agentHeuristicActionsFromPrompt(tt.prompt)
			if len(actions) != 1 || normalizeAgentPlannerActionName(actions[0].Name) != tt.tool {
				t.Fatalf("approval prompt should produce %s action: %#v", tt.tool, actions)
			}

			messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_approval_"+tt.name, tt.prompt, "zh", "tester", "approval_ops", routerServices{})
			if err != nil {
				t.Fatalf("routePrompt() error = %v", err)
			}
			if !called {
				t.Fatalf("routePrompt did not dispatch %s through the registry handler", tt.tool)
			}
			if len(messages) != 1 || messages[0].Content != "approval operation registry dispatched" {
				t.Fatalf("unexpected messages: %#v", messages)
			}
			if len(calls) != 1 || calls[0].Name != tt.tool || calls[0].Result["route_registry"] != true {
				t.Fatalf("unexpected tool calls: %#v", calls)
			}
			if calls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
				t.Fatalf("approval route tool source metadata = %#v", calls[0].Metadata)
			}
			if messages[0].ToolCalls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
				t.Fatalf("approval route message tool source metadata = %#v", messages[0].ToolCalls[0].Metadata)
			}
		})
	}
}

func TestRoutePromptResumeApprovedUsesHeuristicRegistryDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("runs.resume_approved")
	if !ok {
		t.Fatal("runs.resume_approved missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		if in.SessionID != "agent_route_resume_approved" || in.Operator != "tester" || in.Language != "zh" || in.OperationLevel != "approval_ops" {
			t.Fatalf("unexpected handler input: %#v", in)
		}
		if got := stringFromAny(in.RawArgs["run_id"]); got != "run_abc" {
			t.Fatalf("handler RawArgs run_id = %q, want run_abc", got)
		}
		tool := runningAgentToolCall("runs.resume_approved", "route.resume.registry")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"route_registry": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("zh", "tool", "resume-approved registry dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	actions := agentHeuristicActionsFromPrompt("resume-approved run_abc")
	if len(actions) != 1 || normalizeAgentPlannerActionName(actions[0].Name) != "runs.resume_approved" {
		t.Fatalf("resume-approved prompt should produce runs.resume_approved action: %#v", actions)
	}

	messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_resume_approved", "resume-approved run_abc", "zh", "tester", "approval_ops", routerServices{})
	if err != nil {
		t.Fatalf("routePrompt() error = %v", err)
	}
	if !called {
		t.Fatal("routePrompt did not dispatch runs.resume_approved through the registry handler")
	}
	if len(messages) != 1 || messages[0].Content != "resume-approved registry dispatched" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if len(calls) != 1 || calls[0].Name != "runs.resume_approved" || calls[0].Result["route_registry"] != true {
		t.Fatalf("unexpected tool calls: %#v", calls)
	}
	if calls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("resume-approved route tool source metadata = %#v", calls[0].Metadata)
	}
	if messages[0].ToolCalls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("resume-approved route message tool source metadata = %#v", messages[0].ToolCalls[0].Metadata)
	}
}

func TestRoutePromptApprovalOperationRegistryDispatchPreservesOperationGate(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	for _, toolName := range []string{"runs.approvals.approve", "runs.resume_approved"} {
		spec, ok := toolRegistry().Lookup(toolName)
		if !ok {
			t.Fatalf("%s missing from registry", toolName)
		}
		oldHandler := spec.Handler
		t.Cleanup(func() { spec.Handler = oldHandler })
		spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
			t.Fatalf("%s registry handler was called despite insufficient operation level", toolName)
			return nil, nil
		}
	}

	tests := []struct {
		name   string
		prompt string
	}{
		{name: "approve", prompt: "approve run_abc approval_123"},
		{name: "resume", prompt: "resume-approved run_abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requiredLevel := agentOperationLevelRequiredForPrompt(tt.prompt)
			if requiredLevel != "approval_ops" {
				t.Fatalf("approval operation prompt required level = %q, want approval_ops", requiredLevel)
			}
			if agentOperationLevelAtLeast("read_only", requiredLevel) {
				t.Fatalf("read_only unexpectedly satisfies required level %q", requiredLevel)
			}

			messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_approval_gate_"+tt.name, tt.prompt, "zh", "tester", "read_only", routerServices{})
			if err != nil {
				t.Fatalf("routePrompt() error = %v", err)
			}
			if len(calls) != 0 {
				t.Fatalf("operation gate should not return tool calls: %#v", calls)
			}
			if len(messages) != 1 || messages[0].Role != "agent" || messages[0].Content == "" {
				t.Fatalf("unexpected operation gate messages: %#v", messages)
			}
		})
	}
}

func TestApprovalOperationRouteActionAllowlist(t *testing.T) {
	if agentApprovalOperationRouteAction("runs.approvals.list", map[string]any{"run_id": "run_abc"}) {
		t.Fatal("runs.approvals.list must stay outside the approval operation registry route slice")
	}
	if agentApprovalOperationRouteAction("runs.approvals.approve", map[string]any{}) {
		t.Fatal("approval decision without run/approval/current target must stay outside the approval operation registry route slice")
	}
	if agentApprovalOperationRouteAction("runs.approvals.deny", map[string]any{}) {
		t.Fatal("approval deny without run/approval/current target must stay outside the approval operation registry route slice")
	}
	if agentApprovalOperationRouteAction("runs.resume_approved", map[string]any{}) {
		t.Fatal("resume-approved without run/current target must stay outside the approval operation registry route slice")
	}
	if !agentApprovalOperationRouteAction("runs.approvals.approve", map[string]any{"approval_id": "approval_123"}) {
		t.Fatal("approval decision with approval_id should be inside the approval operation registry route slice")
	}
	if !agentApprovalOperationRouteAction("runs.approvals.deny", map[string]any{"run_id": "run_abc"}) {
		t.Fatal("approval decision with run_id should be inside the approval operation registry route slice")
	}
	if !agentApprovalOperationRouteAction("runs.resume_approved", map[string]any{"current": true}) {
		t.Fatal("resume-approved current target should be inside the approval operation registry route slice")
	}
}

func TestRoutePromptReportDraftUsesHeuristicRegistryDispatch(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	spec, ok := toolRegistry().Lookup("reports.draft")
	if !ok {
		t.Fatal("reports.draft missing from registry")
	}
	oldHandler := spec.Handler
	defer func() { spec.Handler = oldHandler }()

	called := false
	spec.Handler = func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		called = true
		if in.SessionID != "agent_route_report_registry" || in.Operator != "tester" || in.Language != "zh" || in.OperationLevel != "workspace_write" {
			t.Fatalf("unexpected handler input: %#v", in)
		}
		if got := stringFromAny(in.RawArgs["run_id"]); got != "run_report_registry" {
			t.Fatalf("handler RawArgs run_id = %q, want run_report_registry", got)
		}
		tool := runningAgentToolCall("reports.draft", "run_report_registry")
		tool = finishAgentToolCall(tool, nil)
		tool.Result = map[string]any{"route_registry": true}
		return agentRegistryHandlerResult{
			Messages:  []agentMessage{mgr.message("zh", "tool", "report draft registry dispatched", []agentToolCall{tool}, nil)},
			ToolCalls: []agentToolCall{tool},
		}, nil
	}

	messages, calls, err := mgr.routePrompt(context.Background(), "agent_route_report_registry", "生成报告草稿 run_report_registry", "zh", "tester", "workspace_write", routerServices{})
	if err != nil {
		t.Fatalf("routePrompt() error = %v", err)
	}
	if !called {
		t.Fatal("routePrompt did not dispatch reports.draft through the registry handler")
	}
	if len(messages) != 1 || messages[0].Content != "report draft registry dispatched" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if len(calls) != 1 || calls[0].Name != "reports.draft" || calls[0].Result["route_registry"] != true {
		t.Fatalf("unexpected tool calls: %#v", calls)
	}
	if calls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("report draft route tool source metadata = %#v", calls[0].Metadata)
	}
	if messages[0].ToolCalls[0].Metadata["source"] != "deterministic_route/registry_dispatch" {
		t.Fatalf("report draft route message tool source metadata = %#v", messages[0].ToolCalls[0].Metadata)
	}
	if agentReportDraftRouteAction("workspace.files.write", map[string]any{"path": "report.md"}) {
		t.Fatal("workspace.files.write must stay outside the report draft registry route slice")
	}
}
