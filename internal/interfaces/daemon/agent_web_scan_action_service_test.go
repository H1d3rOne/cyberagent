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

type recordingWebScanActionService struct {
	calls []recordingWebScanActionCall
}

type recordingWebScanActionCall struct {
	prompt        string
	language      string
	hasJobManager bool
}

func (service *recordingWebScanActionService) WebScan(ctx context.Context, prompt, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	_, hasJobManager := resolver.JobManager()
	service.calls = append(service.calls, recordingWebScanActionCall{
		prompt:        prompt,
		language:      language,
		hasJobManager: hasJobManager,
	})
	tool := runningAgentToolCall("scan.web.safe", prompt)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method":          "WebScan",
		"prompt":          prompt,
		"has_job_manager": hasJobManager,
	}
	return []agentMessage{{
		ID:        "msg-web-scan",
		Role:      "tool",
		Content:   "web scan service WebScan",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func TestAgentLoopToolServiceBindingWithWebScanActionServiceInjectsRuntimeCapability(t *testing.T) {
	webScan := &recordingWebScanActionService{}
	binding := runtime.AgentLoopToolServiceBinding{
		Kind: daemonToolServiceBindingKind,
		ActionServices: runtime.AgentLoopActionServiceSet{
			Capabilities: map[runtime.AgentLoopActionServiceCapability]any{},
			Metadata:     map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
		},
		Metadata: map[string]any{"adapter": "test"},
	}

	injected := agentLoopToolServiceBindingWithWebScanActionService(binding, webScan)
	resolver, ok := agentLoopActionServiceResolverFromActionServices(injected.ActionServices)
	require.True(t, ok)
	resolved, ok := resolver.WebScan()
	require.True(t, ok)
	require.Same(t, webScan, resolved)
	require.Equal(t, "web_scan", injected.ActionServices.Metadata["web_scan_capability"])
	require.Equal(t, "daemon_web_scan_action_service", injected.ActionServices.Metadata["web_scan_adapter"])
	require.Equal(t, "runtime_capability", injected.Metadata["web_scan_action_service"])

	_, originalHasWebScan := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceWebScan]
	require.False(t, originalHasWebScan, "web scan injection must clone instead of mutating the input binding")

	existing := &recordingWebScanActionService{}
	withExisting := injected.Clone()
	withExisting.ActionServices.Capabilities[runtime.AgentLoopActionServiceWebScan] = existing
	reinjected := agentLoopToolServiceBindingWithWebScanActionService(withExisting, webScan)
	resolver, ok = agentLoopActionServiceResolverFromActionServices(reinjected.ActionServices)
	require.True(t, ok)
	resolved, ok = resolver.WebScan()
	require.True(t, ok)
	require.Same(t, existing, resolved, "existing web scan capability must not be overwritten")
}

func TestExecuteRegistryHandlerAttachesWebScanActionServiceCapability(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	called := false
	spec := &toolregistry.ToolSpec{
		Name: "test.web.scan.capability.injected",
		Handler: func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
			_ = ctx
			called = true
			rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
			require.True(t, ok)
			resolver, ok := rt.actionServiceResolver()
			require.True(t, ok)
			webScan, ok := resolver.WebScan()
			require.True(t, ok)
			adapter, ok := webScan.(daemonWebScanManagerActionService)
			require.True(t, ok)
			require.Same(t, mgr, adapter.manager)
			return map[string]any{"web_scan_capability": true}, nil
		},
	}

	result, err := mgr.executeAgentRegistryHandlerResultWithToolServices(context.Background(), "web_scan_capability_injected", "en", "tester", "safe_scan", testAgentLoopToolServices(), spec, nil, runtime.AgentLoopAuthorization{})
	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, true, result.ToolCalls[0].Result["web_scan_capability"])
}

func TestWebScanRegistryHandlerUsesRuntimeActionServiceCapability(t *testing.T) {
	webScan := &recordingWebScanActionService{}
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	spec, ok := toolRegistry().Lookup("scan.web.safe")
	require.True(t, ok, "scan.web.safe missing from registry")
	output, err := spec.Handler(context.Background(), toolregistry.HandlerInput{
		SessionID:      "web_scan_capability_handler",
		Operator:       "tester",
		Language:       "zh",
		OperationLevel: "safe_scan",
		RawArgs: map[string]any{
			"target": "https://example.com",
			"scope":  "https://example.com/app",
		},
		Runtime: agentRegistryHandlerRuntime{
			Services: runtime.AgentLoopToolServiceBinding{
				Kind:   daemonToolServiceBindingKind,
				Handle: registryPanicActionServiceFallback{},
				ActionServices: runtime.AgentLoopActionServiceSet{
					Capabilities: map[runtime.AgentLoopActionServiceCapability]any{
						runtime.AgentLoopActionServiceWebScan: webScan,
						runtime.AgentLoopActionServiceJobs:    jobs,
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
	require.Len(t, webScan.calls, 1)
	require.Equal(t, "zh", webScan.calls[0].language)
	require.Equal(t, "扫描 https://example.com scope=https://example.com/app", webScan.calls[0].prompt)
	require.True(t, webScan.calls[0].hasJobManager)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, "WebScan", result.ToolCalls[0].Result["method"])
	require.Equal(t, "扫描 https://example.com scope=https://example.com/app", result.ToolCalls[0].Result["prompt"])
	require.Equal(t, true, result.ToolCalls[0].Result["has_job_manager"])
}

func TestWebScanRegistryHandlerSourceAvoidsDirectManagerScanCalls(t *testing.T) {
	data, err := os.ReadFile("agent_registry_handlers.go")
	require.NoError(t, err)
	src := string(data)
	require.Contains(t, src, "webScanActionServiceFromRegistryRuntime")
	require.NotContains(t, src, "rt.Manager.handleAgentScan(")
	require.Contains(t, src, "webScanActionServiceFromRegistryRuntime(in, \"scan.web.safe\")")

	adapter, err := os.ReadFile("agent_web_scan_action_service.go")
	require.NoError(t, err)
	adapterSrc := string(adapter)
	require.True(t, strings.Contains(adapterSrc, "handleAgentScan("), "web scan manager call should be isolated to the daemon web scan adapter")
}
