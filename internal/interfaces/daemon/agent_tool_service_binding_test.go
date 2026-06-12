package daemon

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentLoopToolServiceBindingFromRouterServicesUsesActionServicesWithoutOpaqueHandle(t *testing.T) {
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	svc := routerServices{jobs: jobs}

	binding := agentLoopToolServiceBindingFromRouterServices(svc)
	resolvedJobs, ok := agentLoopActionServiceCapabilityValue[*jobManager](binding.ActionServices, runtime.AgentLoopActionServiceJobs)
	require.True(t, ok)
	require.Same(t, jobs, resolvedJobs)
	require.Equal(t, daemonToolServiceBindingKind, binding.Kind)
	require.Equal(t, "daemon_legacy_router_services", binding.Metadata["adapter"])
	require.Equal(t, runtime.AgentLoopActionServiceContractName, binding.Metadata["action_service_contract"])
	require.Equal(t, "runtime_action_services", binding.Metadata["service_surface"])
	require.Equal(t, runtime.AgentLoopActionServiceContractName, binding.ActionServices.Metadata["contract"])
	require.Equal(t, "daemon_router_services_to_action_services", binding.ActionServices.Metadata["adapter"])
	require.NotContains(t, binding.Metadata, "provider_contract")
	require.NotContains(t, binding.ActionServices.Metadata, "provider_adapter")
	require.Nil(t, binding.Handle)

	resolver, ok := agentLoopActionServiceResolverFromActionServices(binding.ActionServices)
	require.True(t, ok)
	resolverJobs, ok := resolver.JobManager()
	require.True(t, ok)
	require.Same(t, jobs, resolverJobs)

	cloned := binding.Clone()
	binding.Metadata["adapter"] = "mutated"
	clonedJobs, ok := agentLoopActionServiceCapabilityValue[*jobManager](cloned.ActionServices, runtime.AgentLoopActionServiceJobs)
	require.True(t, ok)
	require.Same(t, jobs, clonedJobs)
	require.Equal(t, "daemon_legacy_router_services", cloned.Metadata["adapter"])
	require.Equal(t, runtime.AgentLoopActionServiceContractName, cloned.Metadata["action_service_contract"])
	require.Equal(t, "runtime_action_services", cloned.Metadata["service_surface"])
	require.Equal(t, runtime.AgentLoopActionServiceContractName, cloned.ActionServices.Metadata["contract"])
	require.Equal(t, "daemon_router_services_to_action_services", cloned.ActionServices.Metadata["adapter"])
	require.NotContains(t, cloned.Metadata, "provider_contract")
	require.NotContains(t, cloned.ActionServices.Metadata, "provider_adapter")
}

func TestAgentLoopToolServiceBindingRuntimeActionServicesTakePrecedenceOverOpaqueHandle(t *testing.T) {
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	svc := routerServices{jobs: jobs}

	binding := agentLoopToolServiceBindingFromRouterServices(svc)
	binding.Handle = registryPanicActionServiceFallback{}

	resolvedJobs, ok := agentLoopActionServiceCapabilityValue[*jobManager](binding.ActionServices, runtime.AgentLoopActionServiceJobs)
	require.True(t, ok)
	require.Same(t, jobs, resolvedJobs)

}

func TestAgentManagerRouterServicesIngressStaysAtHTTPWrappers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	re := regexp.MustCompile(`func \(m \*agentManager\) ([A-Za-z0-9_]+)\([^)]*routerServices`)
	got := []string{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		for _, match := range re.FindAllStringSubmatch(string(data), -1) {
			got = append(got, match[1])
		}
	}
	sort.Strings(got)
	require.Equal(t, []string{
		"ControlSession",
		"DecideAgentAuthorization",
		"SendMessage",
	}, got)
}

func TestRouterServicesProductionIngressStaysAtWiringAndBindingBoundaries(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	got := []string{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		require.NoError(t, err)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !agentFunctionHasRouterServicesParam(fn) {
				continue
			}
			got = append(got, file+":"+fn.Name.Name)
		}
	}
	sort.Strings(got)
	require.Equal(t, []string{
		"agent_session_flow.go:ControlSession",
		"agent_session_flow.go:DecideAgentAuthorization",
		"agent_session_flow.go:SendMessage",
		"agent_tool_service_binding.go:agentLoopToolServiceBindingFromRouterServices",
		"agent_tool_service_binding.go:daemonActionServiceSetFromRouterServices",
		"router.go:agentSessionContextPayload",
		"router.go:agentSessionStatePayload",
		"router.go:agentSessionTracePayload",
		"router.go:emitApprovalRuntimeResolved",
		"router.go:handleAgentMCPRoute",
		"router.go:handleAgentSessionStream",
		"router.go:handleAgentSessionsRoute",
		"router.go:handleJobRoute",
		"router.go:handleRunRead",
		"router.go:handleRunRoute",
		"router.go:handleRunWrite",
		"router.go:handleScanWebRoute",
		"router.go:handleScheduleRoute",
		"router.go:handleWorkspaceRoute",
		"router.go:newRouter",
	}, got)
}

func agentFunctionHasRouterServicesParam(fn *ast.FuncDecl) bool {
	if fn.Type == nil || fn.Type.Params == nil {
		return false
	}
	for _, field := range fn.Type.Params.List {
		if agentExprIsRouterServices(field.Type) {
			return true
		}
	}
	return false
}

func agentExprIsRouterServices(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "routerServices"
}

func TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		src := string(data)
		for _, forbidden := range []string{
			"routerServicesFromAgentLoopToolServiceBinding(",
			"agentLoopActionServiceResolverFromAgentLoopToolServiceBinding(",
			"agentLoopToolServiceProviderFromAgentLoopToolServiceBinding(",
			"agentLoopToolServiceProviderFromHandle(",
			"routerServicesFromAgentLoopToolServiceProvider(",
			"daemonAgentLoopToolServiceProviderFromActionServiceSet(",
			"daemonAgentLoopToolServiceProviderFromRouterServices(",
			"type daemonActionServiceSetToolServiceProvider",
			"type daemonRouterServicesToolServiceProvider",
			"type daemonAgentLoopToolServiceProvider interface",
			"daemonToolServiceProviderContractName",
			"daemonAgentLoopToolServiceProviderContract",
			"daemonAgentLoopToolServiceProvider",
			"\"provider_contract\"",
			"\"provider_adapter\"",
			"func (m *agentManager) routePrompt(",
			"func (m *agentManager) routePromptForTurn(",
			"func (m *agentManager) routeHeuristicRegistryActions(",
			"func (m *agentManager) runRuntimeDeterministicRoute(",
			"func (m *agentManager) routePromptWithRuntimeDeterministicRoute(",
			"func agentLLMToolLoopRequestFromLegacy(",
			"func agentReplanActionContinuationRequestFromLegacy(",
			"func agentPlannerActionReplanCycleToolContextFromRouterServices(",
			"func (m *agentManager) runAgentLLMToolLoop(",
			"func (m *agentManager) runAgentReplanActionContinuation(",
			"func (m *agentManager) executeAgentPlannerActions(",
			"func (m *agentManager) handleAgentActiveRunContinueForTurn(",
			"func (m *agentManager) runAgentSendMessageTurn(",
		} {
			require.NotContains(t, src, forbidden, "%s must not retain provider/routerServices compatibility helper %q in production runtime files", file, forbidden)
		}
	}
}
