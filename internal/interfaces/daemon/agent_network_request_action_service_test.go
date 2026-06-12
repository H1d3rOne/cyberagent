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

type recordingNetworkRequestActionService struct {
	calls []recordingNetworkRequestActionCall
}

type recordingNetworkRequestActionCall struct {
	method      string
	language    string
	httpMethod  string
	url         string
	headers     map[string]string
	body        string
	authProfile *agentHTTPAuthProfileUsage
	rawArgs     map[string]any
}

func (service *recordingNetworkRequestActionService) HTTPRequest(ctx context.Context, method, targetURL string, requestHeaders map[string]string, requestBodyText, language string, authProfile *agentHTTPAuthProfileUsage) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	copiedHeaders := make(map[string]string, len(requestHeaders))
	for key, value := range requestHeaders {
		copiedHeaders[key] = value
	}
	var copiedUsage *agentHTTPAuthProfileUsage
	if authProfile != nil {
		usage := *authProfile
		copiedUsage = &usage
	}
	service.calls = append(service.calls, recordingNetworkRequestActionCall{
		method:      "HTTPRequest",
		language:    language,
		httpMethod:  method,
		url:         targetURL,
		headers:     copiedHeaders,
		body:        requestBodyText,
		authProfile: copiedUsage,
	})
	tool := runningAgentToolCall("http.request", method+" "+targetURL)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method":      "HTTPRequest",
		"http_method": method,
		"url":         targetURL,
		"headers":     copiedHeaders,
		"body":        requestBodyText,
	}
	return []agentMessage{{
		ID:        "msg-network-http-request",
		Role:      "tool",
		Content:   "network service HTTPRequest",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func (service *recordingNetworkRequestActionService) WebFetch(ctx context.Context, language string, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	service.calls = append(service.calls, recordingNetworkRequestActionCall{
		method:   "WebFetch",
		language: language,
		rawArgs:  cloneMapStringAny(args),
	})
	tool := runningAgentToolCall("web.fetch", stringFromAny(args["url"]))
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method": "WebFetch",
		"args":   cloneMapStringAny(args),
	}
	return []agentMessage{{
		ID:        "msg-network-web-fetch",
		Role:      "tool",
		Content:   "network service WebFetch",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func TestAgentLoopToolServiceBindingWithNetworkRequestActionServiceInjectsRuntimeCapability(t *testing.T) {
	network := &recordingNetworkRequestActionService{}
	binding := runtime.AgentLoopToolServiceBinding{
		Kind: daemonToolServiceBindingKind,
		ActionServices: runtime.AgentLoopActionServiceSet{
			Capabilities: map[runtime.AgentLoopActionServiceCapability]any{},
			Metadata:     map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
		},
		Metadata: map[string]any{"adapter": "test"},
	}

	injected := agentLoopToolServiceBindingWithNetworkRequestActionService(binding, network)
	resolver, ok := agentLoopActionServiceResolverFromActionServices(injected.ActionServices)
	require.True(t, ok)
	resolved, ok := resolver.NetworkRequest()
	require.True(t, ok)
	require.Same(t, network, resolved)
	require.Equal(t, "network_request", injected.ActionServices.Metadata["network_request_capability"])
	require.Equal(t, "daemon_network_request_action_service", injected.ActionServices.Metadata["network_request_adapter"])
	require.Equal(t, "runtime_capability", injected.Metadata["network_request_action_service"])

	_, originalHasNetwork := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceNetworkRequest]
	require.False(t, originalHasNetwork, "network request injection must clone instead of mutating the input binding")

	existing := &recordingNetworkRequestActionService{}
	withExisting := injected.Clone()
	withExisting.ActionServices.Capabilities[runtime.AgentLoopActionServiceNetworkRequest] = existing
	reinjected := agentLoopToolServiceBindingWithNetworkRequestActionService(withExisting, network)
	resolver, ok = agentLoopActionServiceResolverFromActionServices(reinjected.ActionServices)
	require.True(t, ok)
	resolved, ok = resolver.NetworkRequest()
	require.True(t, ok)
	require.Same(t, existing, resolved, "existing network request capability must not be overwritten")
}

func TestExecuteRegistryHandlerAttachesNetworkRequestActionServiceCapability(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	called := false
	spec := &toolregistry.ToolSpec{
		Name: "test.network.capability.injected",
		Handler: func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
			_ = ctx
			called = true
			rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
			require.True(t, ok)
			resolver, ok := rt.actionServiceResolver()
			require.True(t, ok)
			network, ok := resolver.NetworkRequest()
			require.True(t, ok)
			adapter, ok := network.(daemonNetworkRequestManagerActionService)
			require.True(t, ok)
			require.Same(t, mgr, adapter.manager)
			return map[string]any{"network_request_capability": true}, nil
		},
	}

	result, err := mgr.executeAgentRegistryHandlerResultWithToolServices(context.Background(), "network_capability_injected", "en", "tester", "read_only", testAgentLoopToolServices(), spec, nil, runtime.AgentLoopAuthorization{})
	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, true, result.ToolCalls[0].Result["network_request_capability"])
}

func TestNetworkRequestRegistryHandlersUseRuntimeActionServiceCapability(t *testing.T) {
	enabled := true
	tests := []struct {
		name       string
		args       map[string]any
		auth       runtime.AgentLoopAuthorization
		wantMethod string
	}{
		{
			name: "http.request",
			args: map[string]any{
				"url":             "https://runtime-auth.example/api",
				"method":          "POST",
				"body":            "payload",
				"auth_profile_id": "runtime-profile",
			},
			auth: runtime.AgentLoopAuthorization{
				DefaultAuthProfileID: "runtime-profile",
				AuthProfiles: []runtime.AgentLoopAuthProfile{{
					ID:      "runtime-profile",
					Type:    "bearer",
					BaseURL: "https://runtime-auth.example",
					Token:   "runtime-token",
					Enabled: &enabled,
				}},
			},
			wantMethod: "HTTPRequest",
		},
		{
			name: "web.fetch",
			args: map[string]any{
				"url":       "https://example.com/docs",
				"mode":      "raw",
				"max_chars": 1234,
			},
			wantMethod: "WebFetch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network := &recordingNetworkRequestActionService{}
			spec, ok := toolRegistry().Lookup(tt.name)
			require.True(t, ok, "%s missing from registry", tt.name)
			output, err := spec.Handler(context.Background(), toolregistry.HandlerInput{
				SessionID:      "network_capability_handlers",
				Operator:       "tester",
				Language:       "en",
				OperationLevel: "advanced",
				RawArgs:        cloneMapStringAny(tt.args),
				Runtime: agentRegistryHandlerRuntime{
					Services: runtime.AgentLoopToolServiceBinding{
						Kind:   daemonToolServiceBindingKind,
						Handle: registryPanicActionServiceFallback{},
						ActionServices: runtime.AgentLoopActionServiceSet{
							Capabilities: map[runtime.AgentLoopActionServiceCapability]any{
								runtime.AgentLoopActionServiceNetworkRequest: network,
							},
							Metadata: map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
						},
						Metadata: map[string]any{"action_service_contract": runtime.AgentLoopActionServiceContractName},
					},
					Authorization: tt.auth,
					AuthProfiles: []agentHTTPAuthProfile{{
						ID:      "legacy-profile",
						Type:    "bearer",
						BaseURL: "https://runtime-auth.example",
						Token:   "legacy-token",
					}},
					DefaultAuthProfileID: "legacy-profile",
				},
			})
			require.NoError(t, err)
			result, ok := output.(agentRegistryHandlerResult)
			require.True(t, ok)
			require.Len(t, network.calls, 1)
			require.Equal(t, tt.wantMethod, network.calls[0].method)
			require.Equal(t, "en", network.calls[0].language)
			require.Len(t, result.ToolCalls, 1)
			require.Equal(t, tt.wantMethod, result.ToolCalls[0].Result["method"])
			if tt.name == "http.request" {
				require.Equal(t, "POST", network.calls[0].httpMethod)
				require.Equal(t, "https://runtime-auth.example/api", network.calls[0].url)
				require.Equal(t, "payload", network.calls[0].body)
				require.Equal(t, "Bearer runtime-token", network.calls[0].headers["Authorization"])
				require.NotContains(t, network.calls[0].headers["Authorization"], "legacy-token")
				require.NotNil(t, network.calls[0].authProfile)
				require.Equal(t, "runtime-profile", network.calls[0].authProfile.ID)
			}
			if tt.name == "web.fetch" {
				require.Equal(t, "https://example.com/docs", stringFromAny(network.calls[0].rawArgs["url"]))
				require.Equal(t, "raw", stringFromAny(network.calls[0].rawArgs["mode"]))
			}
		})
	}
}

func TestNetworkRequestRegistryHandlerSourceAvoidsDirectManagerNetworkCalls(t *testing.T) {
	data, err := os.ReadFile("agent_registry_handlers.go")
	require.NoError(t, err)
	src := string(data)
	require.Contains(t, src, "networkRequestActionServiceFromRegistryRuntime")
	require.NotContains(t, src, "rt.Manager.handleAgentHTTPRequest(")
	require.NotContains(t, src, "rt.Manager.handleAgentWebFetch(")
	require.Contains(t, src, "networkRequestActionServiceFromRegistryRuntime(in, \"http.request\")")
	require.Contains(t, src, "networkRequestActionServiceFromRegistryRuntime(in, \"web.fetch\")")

	adapter, err := os.ReadFile("agent_network_request_action_service.go")
	require.NoError(t, err)
	adapterSrc := string(adapter)
	require.True(t, strings.Contains(adapterSrc, "handleAgentHTTPRequest(") && strings.Contains(adapterSrc, "handleAgentWebFetch("), "network manager calls should be isolated to the daemon network request adapter")
}
