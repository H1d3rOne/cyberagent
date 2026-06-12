package daemon

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/stretchr/testify/require"
)

type recordingDaemonStatusActionService struct {
	healthCalls  []recordingDaemonStatusActionCall
	pluginsCalls []recordingDaemonStatusActionCall
	runsCalls    []recordingDaemonStatusActionCall
}

type recordingDaemonStatusActionCall struct {
	language          string
	hasVersionService bool
	hasPluginService  bool
	hasRunService     bool
}

func (service *recordingDaemonStatusActionService) Health(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	call := recordingDaemonStatusActionCall{language: language}
	_, call.hasVersionService = resolver.VersionService()
	_, call.hasPluginService = resolver.PluginService()
	_, call.hasRunService = resolver.RunService()
	service.healthCalls = append(service.healthCalls, call)
	tool := runningAgentToolCall("daemon.health", "daemon")
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method":              "Health",
		"language":            language,
		"has_version_service": call.hasVersionService,
		"has_plugin_service":  call.hasPluginService,
		"has_run_service":     call.hasRunService,
	}
	return []agentMessage{{
		ID:        "msg-daemon-health",
		Role:      "tool",
		Content:   "daemon status service Health",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func (service *recordingDaemonStatusActionService) Plugins(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	call := recordingDaemonStatusActionCall{language: language}
	_, call.hasVersionService = resolver.VersionService()
	_, call.hasPluginService = resolver.PluginService()
	_, call.hasRunService = resolver.RunService()
	service.pluginsCalls = append(service.pluginsCalls, call)
	tool := runningAgentToolCall("plugins.list", "registry")
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method":              "Plugins",
		"language":            language,
		"has_version_service": call.hasVersionService,
		"has_plugin_service":  call.hasPluginService,
		"has_run_service":     call.hasRunService,
	}
	return []agentMessage{{
		ID:        "msg-plugins-list",
		Role:      "tool",
		Content:   "daemon status service Plugins",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func (service *recordingDaemonStatusActionService) Runs(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	call := recordingDaemonStatusActionCall{language: language}
	_, call.hasVersionService = resolver.VersionService()
	_, call.hasPluginService = resolver.PluginService()
	_, call.hasRunService = resolver.RunService()
	service.runsCalls = append(service.runsCalls, call)
	tool := runningAgentToolCall("runs.list", "runs-dir")
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method":              "Runs",
		"language":            language,
		"has_version_service": call.hasVersionService,
		"has_plugin_service":  call.hasPluginService,
		"has_run_service":     call.hasRunService,
	}
	return []agentMessage{{
		ID:        "msg-runs-list",
		Role:      "tool",
		Content:   "daemon status service Runs",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func TestAgentLoopToolServiceBindingWithDaemonStatusActionServiceInjectsRuntimeCapability(t *testing.T) {
	status := &recordingDaemonStatusActionService{}
	binding := runtime.AgentLoopToolServiceBinding{
		Kind: daemonToolServiceBindingKind,
		ActionServices: runtime.AgentLoopActionServiceSet{
			Capabilities: map[runtime.AgentLoopActionServiceCapability]any{},
			Metadata:     map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
		},
		Metadata: map[string]any{"adapter": "test"},
	}

	injected := agentLoopToolServiceBindingWithDaemonStatusActionService(binding, status)
	resolver, ok := agentLoopActionServiceResolverFromActionServices(injected.ActionServices)
	require.True(t, ok)
	resolved, ok := resolver.DaemonStatus()
	require.True(t, ok)
	require.Same(t, status, resolved)
	require.Equal(t, "daemon_status_action", injected.ActionServices.Metadata["daemon_status_action_capability"])
	require.Equal(t, "daemon_status_action_service", injected.ActionServices.Metadata["daemon_status_action_adapter"])
	require.Equal(t, "runtime_capability", injected.Metadata["daemon_status_action_service"])

	_, originalHasStatus := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceDaemonStatusAction]
	require.False(t, originalHasStatus, "daemon status injection must clone instead of mutating the input binding")

	existing := &recordingDaemonStatusActionService{}
	withExisting := injected.Clone()
	withExisting.ActionServices.Capabilities[runtime.AgentLoopActionServiceDaemonStatusAction] = existing
	reinjected := agentLoopToolServiceBindingWithDaemonStatusActionService(withExisting, status)
	resolver, ok = agentLoopActionServiceResolverFromActionServices(reinjected.ActionServices)
	require.True(t, ok)
	resolved, ok = resolver.DaemonStatus()
	require.True(t, ok)
	require.Same(t, existing, resolved, "existing daemon status capability must not be overwritten")
}

func TestExecuteRegistryHandlerAttachesDaemonStatusActionServiceCapability(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	called := false
	spec := &toolregistry.ToolSpec{
		Name: "test.daemon.status.capability.injected",
		Handler: func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
			_ = ctx
			called = true
			rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
			require.True(t, ok)
			resolver, ok := rt.actionServiceResolver()
			require.True(t, ok)
			status, ok := resolver.DaemonStatus()
			require.True(t, ok)
			adapter, ok := status.(daemonStatusManagerActionService)
			require.True(t, ok)
			require.Same(t, mgr, adapter.manager)
			return map[string]any{"daemon_status_capability": true}, nil
		},
	}

	result, err := mgr.executeAgentRegistryHandlerResultWithToolServices(context.Background(), "daemon_status_capability_injected", "en", "tester", "read_only", testAgentLoopToolServices(), spec, nil, runtime.AgentLoopAuthorization{})
	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, true, result.ToolCalls[0].Result["daemon_status_capability"])
}

func TestDaemonStatusRegistryHandlersUseRuntimeActionServiceCapability(t *testing.T) {
	status := &recordingDaemonStatusActionService{}
	runtimeBinding := runtime.AgentLoopToolServiceBinding{
		Kind:   daemonToolServiceBindingKind,
		Handle: registryPanicActionServiceFallback{},
		ActionServices: runtime.AgentLoopActionServiceSet{
			Capabilities: map[runtime.AgentLoopActionServiceCapability]any{
				runtime.AgentLoopActionServiceDaemonStatusAction: status,
				runtime.AgentLoopActionServiceVersion:            services.NewVersionService(services.BuildInfo{Name: "N209", Version: "vN209"}),
				runtime.AgentLoopActionServicePlugins:            services.NewPluginService(t.TempDir()),
				runtime.AgentLoopActionServiceRuns:               services.NewRunService(t.TempDir()),
			},
			Metadata: map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
		},
		Metadata: map[string]any{"action_service_contract": runtime.AgentLoopActionServiceContractName},
	}
	runHandler := func(name string) agentRegistryHandlerResult {
		t.Helper()
		spec, ok := toolRegistry().Lookup(name)
		require.True(t, ok, "%s missing from registry", name)
		output, err := spec.Handler(context.Background(), toolregistry.HandlerInput{
			SessionID:      "daemon_status_capability_handler",
			Operator:       "tester",
			Language:       "zh",
			OperationLevel: "read_only",
			RawArgs:        map[string]any{},
			Runtime: agentRegistryHandlerRuntime{
				Services: runtimeBinding,
			},
		})
		require.NoError(t, err)
		result, ok := output.(agentRegistryHandlerResult)
		require.True(t, ok)
		require.Len(t, result.ToolCalls, 1)
		return result
	}

	health := runHandler("daemon.health")
	plugins := runHandler("plugins.list")
	runs := runHandler("runs.list")

	require.Len(t, status.healthCalls, 1)
	require.Equal(t, "zh", status.healthCalls[0].language)
	require.True(t, status.healthCalls[0].hasVersionService)
	require.True(t, status.healthCalls[0].hasPluginService)
	require.True(t, status.healthCalls[0].hasRunService)
	require.Equal(t, "Health", health.ToolCalls[0].Result["method"])

	require.Len(t, status.pluginsCalls, 1)
	require.Equal(t, "zh", status.pluginsCalls[0].language)
	require.True(t, status.pluginsCalls[0].hasPluginService)
	require.Equal(t, "Plugins", plugins.ToolCalls[0].Result["method"])

	require.Len(t, status.runsCalls, 1)
	require.Equal(t, "zh", status.runsCalls[0].language)
	require.True(t, status.runsCalls[0].hasRunService)
	require.Equal(t, "Runs", runs.ToolCalls[0].Result["method"])
}

func TestDaemonStatusRegistryHandlerSourceAvoidsDirectManagerStatusCalls(t *testing.T) {
	data, err := os.ReadFile("agent_registry_handlers.go")
	require.NoError(t, err)
	src := string(data)
	require.Contains(t, src, "daemonStatusActionServiceFromRegistryRuntime")
	require.Contains(t, src, "daemonStatusActionServiceFromRegistryRuntime(in, \"daemon.health\")")
	require.Contains(t, src, "daemonStatusActionServiceFromRegistryRuntime(in, \"plugins.list\")")
	require.Contains(t, src, "daemonStatusActionServiceFromRegistryRuntime(in, \"runs.list\")")
	require.NotContains(t, src, "rt.Manager.handleAgentHealth(")
	require.NotContains(t, src, "rt.Manager.handleAgentPlugins(")
	require.NotContains(t, src, "rt.Manager.handleAgentRuns(")

	adapter, err := os.ReadFile("agent_daemon_status_action_service.go")
	require.NoError(t, err)
	adapterSrc := string(adapter)
	require.True(t, strings.Contains(adapterSrc, "handleAgentHealth("), "health manager call should be isolated to the daemon status adapter")
	require.True(t, strings.Contains(adapterSrc, "handleAgentPlugins("), "plugins manager call should be isolated to the daemon status adapter")
	require.True(t, strings.Contains(adapterSrc, "handleAgentRuns("), "runs manager call should be isolated to the daemon status adapter")
}
