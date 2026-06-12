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

type recordingRunContextApprovalActionService struct {
	calls []recordingRunContextApprovalActionCall
}

type recordingRunContextApprovalActionCall struct {
	method             string
	sessionID          string
	runID              string
	contextKind        string
	language           string
	operator           string
	prompt             string
	outputPath         string
	hasRunService      bool
	hasApprovalService bool
}

func (service *recordingRunContextApprovalActionService) appendCall(method, language string, resolver daemonAgentLoopActionServiceResolver) recordingRunContextApprovalActionCall {
	call := recordingRunContextApprovalActionCall{
		method:   method,
		language: language,
	}
	_, call.hasRunService = resolver.RunService()
	_, call.hasApprovalService = resolver.ApprovalService()
	service.calls = append(service.calls, call)
	return call
}

func runContextApprovalActionServiceResult(method string, call recordingRunContextApprovalActionCall) ([]agentMessage, []agentToolCall, error) {
	tool := runningAgentToolCall("run_context_approval."+method, "runtime")
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method":               method,
		"session_id":           call.sessionID,
		"run_id":               call.runID,
		"context_kind":         call.contextKind,
		"language":             call.language,
		"operator":             call.operator,
		"prompt":               call.prompt,
		"output_path":          call.outputPath,
		"has_run_service":      call.hasRunService,
		"has_approval_service": call.hasApprovalService,
	}
	return []agentMessage{{
		ID:        "msg-" + method,
		Role:      "tool",
		Content:   "run context approval service " + method,
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func (service *recordingRunContextApprovalActionService) RunContext(ctx context.Context, sessionID, runID, contextKind, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	call := service.appendCall("RunContext", language, resolver)
	call.sessionID = sessionID
	call.runID = runID
	call.contextKind = contextKind
	service.calls[len(service.calls)-1] = call
	return runContextApprovalActionServiceResult("RunContext", call)
}

func (service *recordingRunContextApprovalActionService) ReportDraft(ctx context.Context, sessionID, runID, language string, resolver daemonAgentLoopActionServiceResolver, outputPath string) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	call := service.appendCall("ReportDraft", language, resolver)
	call.sessionID = sessionID
	call.runID = runID
	call.outputPath = outputPath
	service.calls[len(service.calls)-1] = call
	return runContextApprovalActionServiceResult("ReportDraft", call)
}

func (service *recordingRunContextApprovalActionService) ApprovalsList(ctx context.Context, runID, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	call := service.appendCall("ApprovalsList", language, resolver)
	call.runID = runID
	service.calls[len(service.calls)-1] = call
	return runContextApprovalActionServiceResult("ApprovalsList", call)
}

func (service *recordingRunContextApprovalActionService) ResumeApproved(ctx context.Context, sessionID, prompt, language, operator string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	call := service.appendCall("ResumeApproved", language, resolver)
	call.sessionID = sessionID
	call.prompt = prompt
	call.operator = operator
	service.calls[len(service.calls)-1] = call
	return runContextApprovalActionServiceResult("ResumeApproved", call)
}

func (service *recordingRunContextApprovalActionService) ApprovalDecision(ctx context.Context, sessionID, prompt, language, operator string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	call := service.appendCall("ApprovalDecision", language, resolver)
	call.sessionID = sessionID
	call.prompt = prompt
	call.operator = operator
	service.calls[len(service.calls)-1] = call
	return runContextApprovalActionServiceResult("ApprovalDecision", call)
}

func TestAgentLoopToolServiceBindingWithRunContextApprovalActionServiceInjectsRuntimeCapability(t *testing.T) {
	runContextApproval := &recordingRunContextApprovalActionService{}
	binding := runtime.AgentLoopToolServiceBinding{
		Kind: daemonToolServiceBindingKind,
		ActionServices: runtime.AgentLoopActionServiceSet{
			Capabilities: map[runtime.AgentLoopActionServiceCapability]any{},
			Metadata:     map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
		},
		Metadata: map[string]any{"adapter": "test"},
	}

	injected := agentLoopToolServiceBindingWithRunContextApprovalActionService(binding, runContextApproval)
	resolver, ok := agentLoopActionServiceResolverFromActionServices(injected.ActionServices)
	require.True(t, ok)
	resolved, ok := resolver.RunContextApproval()
	require.True(t, ok)
	require.Same(t, runContextApproval, resolved)
	require.Equal(t, "run_context_approval_action", injected.ActionServices.Metadata["run_context_approval_action_capability"])
	require.Equal(t, "daemon_run_context_approval_action_service", injected.ActionServices.Metadata["run_context_approval_action_adapter"])
	require.Equal(t, "runtime_capability", injected.Metadata["run_context_approval_action_service"])

	_, originalHasRunContextApproval := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceRunContextApproval]
	require.False(t, originalHasRunContextApproval, "run-context approval injection must clone instead of mutating the input binding")

	existing := &recordingRunContextApprovalActionService{}
	withExisting := injected.Clone()
	withExisting.ActionServices.Capabilities[runtime.AgentLoopActionServiceRunContextApproval] = existing
	reinjected := agentLoopToolServiceBindingWithRunContextApprovalActionService(withExisting, runContextApproval)
	resolver, ok = agentLoopActionServiceResolverFromActionServices(reinjected.ActionServices)
	require.True(t, ok)
	resolved, ok = resolver.RunContextApproval()
	require.True(t, ok)
	require.Same(t, existing, resolved, "existing run-context approval capability must not be overwritten")
}

func TestExecuteRegistryHandlerAttachesRunContextApprovalActionServiceCapability(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	called := false
	spec := &toolregistry.ToolSpec{
		Name: "test.run.context.approval.capability.injected",
		Handler: func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
			_ = ctx
			called = true
			rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
			require.True(t, ok)
			resolver, ok := rt.actionServiceResolver()
			require.True(t, ok)
			runContextApproval, ok := resolver.RunContextApproval()
			require.True(t, ok)
			adapter, ok := runContextApproval.(daemonRunContextApprovalManagerActionService)
			require.True(t, ok)
			require.Same(t, mgr, adapter.manager)
			return map[string]any{"run_context_approval_capability": true}, nil
		},
	}

	result, err := mgr.executeAgentRegistryHandlerResultWithToolServices(context.Background(), "run_context_approval_capability_injected", "en", "tester", "read_only", testAgentLoopToolServices(), spec, nil, runtime.AgentLoopAuthorization{})
	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, true, result.ToolCalls[0].Result["run_context_approval_capability"])
}

func TestRunContextApprovalRegistryHandlersUseRuntimeActionServiceCapability(t *testing.T) {
	runContextApproval := &recordingRunContextApprovalActionService{}
	runsDir := t.TempDir()
	runtimeBinding := runtime.AgentLoopToolServiceBinding{
		Kind:   daemonToolServiceBindingKind,
		Handle: registryPanicActionServiceFallback{},
		ActionServices: runtime.AgentLoopActionServiceSet{
			Capabilities: map[runtime.AgentLoopActionServiceCapability]any{
				runtime.AgentLoopActionServiceRunContextApproval: runContextApproval,
				runtime.AgentLoopActionServiceRuns:               services.NewRunService(runsDir),
				runtime.AgentLoopActionServiceApprovals:          services.NewApprovalService(runsDir),
			},
			Metadata: map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
		},
		Metadata: map[string]any{"action_service_contract": runtime.AgentLoopActionServiceContractName},
	}
	runHandler := func(name string, args map[string]any) agentRegistryHandlerResult {
		t.Helper()
		spec, ok := toolRegistry().Lookup(name)
		require.True(t, ok, "%s missing from registry", name)
		output, err := spec.Handler(context.Background(), toolregistry.HandlerInput{
			SessionID:      "run_context_approval_capability_handler",
			Operator:       "operator-1",
			Language:       "zh",
			OperationLevel: "read_only",
			RawArgs:        args,
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

	runHandler("runs.context", map[string]any{"run_id": "run-1", "context": "events"})
	runHandler("reports.draft", map[string]any{"run_id": "run-2", "output_path": "report.md"})
	runHandler("runs.approvals.list", map[string]any{})
	runHandler("runs.approvals.list", map[string]any{"run_id": "run-3"})
	runHandler("runs.resume_approved", map[string]any{"run_id": "run-4"})
	runHandler("runs.approvals.approve", map[string]any{"run_id": "run-5", "approval_id": "approval-5", "comment": "ok"})
	runHandler("runs.approvals.deny", map[string]any{"approval_id": "approval-6"})

	require.Len(t, runContextApproval.calls, 7)
	for _, call := range runContextApproval.calls {
		require.Equal(t, "zh", call.language)
		require.True(t, call.hasRunService)
		require.True(t, call.hasApprovalService)
	}
	require.Equal(t, recordingRunContextApprovalActionCall{
		method:             "RunContext",
		sessionID:          "run_context_approval_capability_handler",
		runID:              "run-1",
		contextKind:        "events",
		language:           "zh",
		hasRunService:      true,
		hasApprovalService: true,
	}, runContextApproval.calls[0])
	require.Equal(t, "ReportDraft", runContextApproval.calls[1].method)
	require.Equal(t, "run-2", runContextApproval.calls[1].runID)
	require.Equal(t, "report.md", runContextApproval.calls[1].outputPath)
	require.Equal(t, "ApprovalsList", runContextApproval.calls[2].method)
	require.Empty(t, runContextApproval.calls[2].runID)
	require.Equal(t, "ApprovalsList", runContextApproval.calls[3].method)
	require.Equal(t, "run-3", runContextApproval.calls[3].runID)
	require.Equal(t, "ResumeApproved", runContextApproval.calls[4].method)
	require.Equal(t, "恢复已批准运行 run-4", runContextApproval.calls[4].prompt)
	require.Equal(t, "operator-1", runContextApproval.calls[4].operator)
	require.Equal(t, "ApprovalDecision", runContextApproval.calls[5].method)
	require.Equal(t, "批准运行 run-5 的 approval-5 comment=ok", runContextApproval.calls[5].prompt)
	require.Equal(t, "ApprovalDecision", runContextApproval.calls[6].method)
	require.Equal(t, "拒绝 approval-6", runContextApproval.calls[6].prompt)
}

func TestRunContextApprovalRegistryHandlerSourceAvoidsDirectManagerRunContextApprovalCalls(t *testing.T) {
	data, err := os.ReadFile("agent_registry_handlers.go")
	require.NoError(t, err)
	src := string(data)
	require.Contains(t, src, "runContextApprovalActionServiceFromRegistryRuntime")
	require.Contains(t, src, "runContextApprovalActionServiceFromRegistryRuntime(in, \"runs.context\")")
	require.Contains(t, src, "runContextApprovalActionServiceFromRegistryRuntime(in, \"reports.draft\")")
	require.Contains(t, src, "runContextApprovalActionServiceFromRegistryRuntime(in, \"runs.approvals.list\")")
	require.Contains(t, src, "runContextApprovalActionServiceFromRegistryRuntime(in, \"runs.resume_approved\")")
	require.Contains(t, src, "runContextApprovalActionServiceFromRegistryRuntime(in, name)")
	require.NotContains(t, src, "rt.Manager.handleAgentRunContextTool(")
	require.NotContains(t, src, "rt.Manager.handleAgentReportDraft(")
	require.NotContains(t, src, "rt.Manager.handleAgentApprovals(")
	require.NotContains(t, src, "rt.Manager.handleAgentRunContextAnalysis(")
	require.NotContains(t, src, "rt.Manager.handleAgentResumeApproved(")
	require.NotContains(t, src, "rt.Manager.handleAgentApprovalDecision(")

	adapter, err := os.ReadFile("agent_run_context_approval_action_service.go")
	require.NoError(t, err)
	adapterSrc := string(adapter)
	for _, method := range []string{
		"handleAgentRunContextTool(",
		"handleAgentReportDraft(",
		"handleAgentApprovals(",
		"handleAgentRunContextAnalysis(",
		"handleAgentResumeApproved(",
		"handleAgentApprovalDecision(",
	} {
		require.True(t, strings.Contains(adapterSrc, method), "%s manager call should be isolated to the run-context approval adapter", method)
	}
}
