package daemon

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/stretchr/testify/require"
)

type recordingPortScannerActionService struct {
	calls []recordingPortScannerActionCall
}

type recordingPortScannerActionCall struct {
	prompt   string
	language string
}

func (service *recordingPortScannerActionService) PortScan(ctx context.Context, prompt, language string) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	service.calls = append(service.calls, recordingPortScannerActionCall{
		prompt:   prompt,
		language: language,
	})
	tool := runningAgentToolCall("ports.scan.safe", prompt)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method": "PortScan",
		"prompt": prompt,
	}
	return []agentMessage{{
		ID:        "msg-port-scanner",
		Role:      "tool",
		Content:   "port scanner service PortScan",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func TestAgentLoopToolServiceBindingWithPortScannerActionServiceInjectsRuntimeCapability(t *testing.T) {
	portScanner := &recordingPortScannerActionService{}
	binding := runtime.AgentLoopToolServiceBinding{
		Kind: daemonToolServiceBindingKind,
		ActionServices: runtime.AgentLoopActionServiceSet{
			Capabilities: map[runtime.AgentLoopActionServiceCapability]any{},
			Metadata:     map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
		},
		Metadata: map[string]any{"adapter": "test"},
	}

	injected := agentLoopToolServiceBindingWithPortScannerActionService(binding, portScanner)
	resolver, ok := agentLoopActionServiceResolverFromActionServices(injected.ActionServices)
	require.True(t, ok)
	resolved, ok := resolver.PortScanner()
	require.True(t, ok)
	require.Same(t, portScanner, resolved)
	require.Equal(t, "port_scanner", injected.ActionServices.Metadata["port_scanner_capability"])
	require.Equal(t, "daemon_port_scanner_action_service", injected.ActionServices.Metadata["port_scanner_adapter"])
	require.Equal(t, "runtime_capability", injected.Metadata["port_scanner_action_service"])

	_, originalHasPortScanner := binding.ActionServices.Capabilities[runtime.AgentLoopActionServicePortScanner]
	require.False(t, originalHasPortScanner, "port scanner injection must clone instead of mutating the input binding")

	existing := &recordingPortScannerActionService{}
	withExisting := injected.Clone()
	withExisting.ActionServices.Capabilities[runtime.AgentLoopActionServicePortScanner] = existing
	reinjected := agentLoopToolServiceBindingWithPortScannerActionService(withExisting, portScanner)
	resolver, ok = agentLoopActionServiceResolverFromActionServices(reinjected.ActionServices)
	require.True(t, ok)
	resolved, ok = resolver.PortScanner()
	require.True(t, ok)
	require.Same(t, existing, resolved, "existing port scanner capability must not be overwritten")
}

func TestExecuteRegistryHandlerAttachesPortScannerActionServiceCapability(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	called := false
	spec := &toolregistry.ToolSpec{
		Name: "test.port.scanner.capability.injected",
		Handler: func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
			_ = ctx
			called = true
			rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
			require.True(t, ok)
			resolver, ok := rt.actionServiceResolver()
			require.True(t, ok)
			portScanner, ok := resolver.PortScanner()
			require.True(t, ok)
			adapter, ok := portScanner.(daemonPortScannerManagerActionService)
			require.True(t, ok)
			require.Same(t, mgr, adapter.manager)
			return map[string]any{"port_scanner_capability": true}, nil
		},
	}

	result, err := mgr.executeAgentRegistryHandlerResultWithToolServices(context.Background(), "port_scanner_capability_injected", "en", "tester", "safe_scan", testAgentLoopToolServices(), spec, nil, runtime.AgentLoopAuthorization{})
	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, true, result.ToolCalls[0].Result["port_scanner_capability"])
}

func TestPortScannerRegistryHandlerUsesRuntimeActionServiceCapability(t *testing.T) {
	portScanner := &recordingPortScannerActionService{}
	spec, ok := toolRegistry().Lookup("ports.scan.safe")
	require.True(t, ok, "ports.scan.safe missing from registry")
	output, err := spec.Handler(context.Background(), toolregistry.HandlerInput{
		SessionID:      "port_scanner_capability_handler",
		Operator:       "tester",
		Language:       "zh",
		OperationLevel: "safe_scan",
		RawArgs: map[string]any{
			"host":  "example.com",
			"ports": []any{float64(80), float64(443)},
		},
		Runtime: agentRegistryHandlerRuntime{
			Services: runtime.AgentLoopToolServiceBinding{
				Kind:   daemonToolServiceBindingKind,
				Handle: registryPanicActionServiceFallback{},
				ActionServices: runtime.AgentLoopActionServiceSet{
					Capabilities: map[runtime.AgentLoopActionServiceCapability]any{
						runtime.AgentLoopActionServicePortScanner: portScanner,
					},
					Metadata: map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
				},
				Metadata: map[string]any{"action_service_contract": runtime.AgentLoopActionServiceContractName},
			},
		},
	})
	require.NoError(t, err)
	result, ok := output.(agentRegistryHandlerResult)
	require.True(t, ok)
	require.Len(t, portScanner.calls, 1)
	require.Equal(t, "zh", portScanner.calls[0].language)
	require.Equal(t, "扫描 example.com 开放端口 ports=80,443", portScanner.calls[0].prompt)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, "PortScan", result.ToolCalls[0].Result["method"])
	require.Equal(t, "扫描 example.com 开放端口 ports=80,443", result.ToolCalls[0].Result["prompt"])
}

func TestPortScannerRegistryHandlerSourceAvoidsDirectManagerPortScanCalls(t *testing.T) {
	data, err := os.ReadFile("agent_registry_handlers.go")
	require.NoError(t, err)
	src := string(data)
	require.Contains(t, src, "portScannerActionServiceFromRegistryRuntime")
	require.NotContains(t, src, "rt.Manager.handleAgentPortScan(")
	require.Contains(t, src, "portScannerActionServiceFromRegistryRuntime(in, \"ports.scan.safe\")")

	adapter, err := os.ReadFile("agent_port_scanner_action_service.go")
	require.NoError(t, err)
	adapterSrc := string(adapter)
	require.True(t, strings.Contains(adapterSrc, "handleAgentPortScan("), "port scan manager call should be isolated to the daemon port scanner adapter")
}
