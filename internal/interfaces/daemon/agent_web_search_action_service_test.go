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

type recordingWebSearchActionService struct {
	calls []recordingWebSearchActionCall
}

type recordingWebSearchActionCall struct {
	language    string
	args        map[string]any
	hasProvider bool
}

func (service *recordingWebSearchActionService) WebSearch(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	_, hasProvider := resolver.ProviderStore()
	copiedArgs := cloneMapStringAny(args)
	service.calls = append(service.calls, recordingWebSearchActionCall{
		language:    language,
		args:        copiedArgs,
		hasProvider: hasProvider,
	})
	tool := runningAgentToolCall("web.search", stringFromAny(args["query"]))
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method":       "WebSearch",
		"args":         cloneMapStringAny(args),
		"has_provider": hasProvider,
	}
	return []agentMessage{{
		ID:        "msg-web-search",
		Role:      "tool",
		Content:   "web search service WebSearch",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func TestAgentLoopToolServiceBindingWithWebSearchActionServiceInjectsRuntimeCapability(t *testing.T) {
	webSearch := &recordingWebSearchActionService{}
	binding := runtime.AgentLoopToolServiceBinding{
		Kind: daemonToolServiceBindingKind,
		ActionServices: runtime.AgentLoopActionServiceSet{
			Capabilities: map[runtime.AgentLoopActionServiceCapability]any{},
			Metadata:     map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
		},
		Metadata: map[string]any{"adapter": "test"},
	}

	injected := agentLoopToolServiceBindingWithWebSearchActionService(binding, webSearch)
	resolver, ok := agentLoopActionServiceResolverFromActionServices(injected.ActionServices)
	require.True(t, ok)
	resolved, ok := resolver.WebSearch()
	require.True(t, ok)
	require.Same(t, webSearch, resolved)
	require.Equal(t, "web_search", injected.ActionServices.Metadata["web_search_capability"])
	require.Equal(t, "daemon_web_search_action_service", injected.ActionServices.Metadata["web_search_adapter"])
	require.Equal(t, "runtime_capability", injected.Metadata["web_search_action_service"])

	_, originalHasWebSearch := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceWebSearch]
	require.False(t, originalHasWebSearch, "web search injection must clone instead of mutating the input binding")

	existing := &recordingWebSearchActionService{}
	withExisting := injected.Clone()
	withExisting.ActionServices.Capabilities[runtime.AgentLoopActionServiceWebSearch] = existing
	reinjected := agentLoopToolServiceBindingWithWebSearchActionService(withExisting, webSearch)
	resolver, ok = agentLoopActionServiceResolverFromActionServices(reinjected.ActionServices)
	require.True(t, ok)
	resolved, ok = resolver.WebSearch()
	require.True(t, ok)
	require.Same(t, existing, resolved, "existing web search capability must not be overwritten")
}

func TestExecuteRegistryHandlerAttachesWebSearchActionServiceCapability(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	called := false
	spec := &toolregistry.ToolSpec{
		Name: "test.web.search.capability.injected",
		Handler: func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
			_ = ctx
			called = true
			rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
			require.True(t, ok)
			resolver, ok := rt.actionServiceResolver()
			require.True(t, ok)
			webSearch, ok := resolver.WebSearch()
			require.True(t, ok)
			adapter, ok := webSearch.(daemonWebSearchManagerActionService)
			require.True(t, ok)
			require.Same(t, mgr, adapter.manager)
			return map[string]any{"web_search_capability": true}, nil
		},
	}

	result, err := mgr.executeAgentRegistryHandlerResultWithToolServices(context.Background(), "web_search_capability_injected", "en", "tester", "read_only", testAgentLoopToolServices(), spec, nil, runtime.AgentLoopAuthorization{})
	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, true, result.ToolCalls[0].Result["web_search_capability"])
}

func TestWebSearchRegistryHandlerUsesRuntimeActionServiceCapability(t *testing.T) {
	webSearch := &recordingWebSearchActionService{}
	providers := newProviderStore(t.TempDir() + "/providers.json")
	spec, ok := toolRegistry().Lookup("web.search")
	require.True(t, ok, "web.search missing from registry")
	output, err := spec.Handler(context.Background(), toolregistry.HandlerInput{
		SessionID:      "web_search_capability_handler",
		Operator:       "tester",
		Language:       "zh",
		OperationLevel: "read_only",
		RawArgs: map[string]any{
			"query":       "runtime web search capability",
			"max_results": float64(3),
		},
		Runtime: agentRegistryHandlerRuntime{
			Services: runtime.AgentLoopToolServiceBinding{
				Kind:   daemonToolServiceBindingKind,
				Handle: registryPanicActionServiceFallback{},
				ActionServices: runtime.AgentLoopActionServiceSet{
					Capabilities: map[runtime.AgentLoopActionServiceCapability]any{
						runtime.AgentLoopActionServiceWebSearch: webSearch,
						runtime.AgentLoopActionServiceProviders: providers,
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
	require.Len(t, webSearch.calls, 1)
	require.Equal(t, "zh", webSearch.calls[0].language)
	require.Equal(t, "runtime web search capability", stringFromAny(webSearch.calls[0].args["query"]))
	require.Equal(t, float64(3), webSearch.calls[0].args["max_results"])
	require.True(t, webSearch.calls[0].hasProvider)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, "WebSearch", result.ToolCalls[0].Result["method"])
	require.Equal(t, true, result.ToolCalls[0].Result["has_provider"])
}

func TestWebSearchRegistryHandlerSourceAvoidsDirectManagerWebSearchCalls(t *testing.T) {
	data, err := os.ReadFile("agent_registry_handlers.go")
	require.NoError(t, err)
	src := string(data)
	require.Contains(t, src, "webSearchActionServiceFromRegistryRuntime")
	require.NotContains(t, src, "rt.Manager.handleAgentWebSearch(")
	require.Contains(t, src, "webSearchActionServiceFromRegistryRuntime(in, \"web.search\")")

	adapter, err := os.ReadFile("agent_web_search_action_service.go")
	require.NoError(t, err)
	adapterSrc := string(adapter)
	require.True(t, strings.Contains(adapterSrc, "handleAgentWebSearch("), "web search manager call should be isolated to the daemon web search adapter")
}
