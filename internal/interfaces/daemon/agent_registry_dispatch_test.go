package daemon

import (
	"context"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
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
	for _, name := range []string{"daemon.health", "plugins.list", "runs.list"} {
		spec, ok := toolRegistry().Lookup(name)
		if !ok {
			t.Fatalf("%s missing from registry", name)
		}
		if spec.Handler == nil {
			t.Fatalf("%s Handler is nil", name)
		}
	}
}
