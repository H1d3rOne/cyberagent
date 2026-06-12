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

type recordingScheduleActionService struct {
	createCalls []recordingScheduleCreateActionCall
	listCalls   []recordingScheduleListActionCall
	deleteCalls []recordingScheduleDeleteActionCall
}

type recordingScheduleCreateActionCall struct {
	sessionID          string
	language           string
	operator           string
	hasScheduleManager bool
	args               map[string]any
}

type recordingScheduleListActionCall struct {
	language           string
	hasScheduleManager bool
}

type recordingScheduleDeleteActionCall struct {
	language           string
	hasScheduleManager bool
	args               map[string]any
}

func (service *recordingScheduleActionService) ScheduleCreate(ctx context.Context, sessionID, language, operator string, resolver daemonAgentLoopActionServiceResolver, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	_, hasScheduleManager := resolver.ScheduleManager()
	service.createCalls = append(service.createCalls, recordingScheduleCreateActionCall{
		sessionID:          sessionID,
		language:           language,
		operator:           operator,
		hasScheduleManager: hasScheduleManager,
		args:               cloneMapStringAny(args),
	})
	tool := runningAgentToolCall("schedule.create", stringFromAny(args["prompt"]))
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method":               "ScheduleCreate",
		"session_id":           sessionID,
		"language":             language,
		"operator":             operator,
		"has_schedule_manager": hasScheduleManager,
	}
	return []agentMessage{{
		ID:        "msg-schedule-create",
		Role:      "tool",
		Content:   "schedule service ScheduleCreate",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func (service *recordingScheduleActionService) ScheduleList(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	_, hasScheduleManager := resolver.ScheduleManager()
	service.listCalls = append(service.listCalls, recordingScheduleListActionCall{
		language:           language,
		hasScheduleManager: hasScheduleManager,
	})
	tool := runningAgentToolCall("schedule.list", "")
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method":               "ScheduleList",
		"language":             language,
		"has_schedule_manager": hasScheduleManager,
	}
	return []agentMessage{{
		ID:        "msg-schedule-list",
		Role:      "tool",
		Content:   "schedule service ScheduleList",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func (service *recordingScheduleActionService) ScheduleDelete(ctx context.Context, language string, resolver daemonAgentLoopActionServiceResolver, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	_, hasScheduleManager := resolver.ScheduleManager()
	service.deleteCalls = append(service.deleteCalls, recordingScheduleDeleteActionCall{
		language:           language,
		hasScheduleManager: hasScheduleManager,
		args:               cloneMapStringAny(args),
	})
	tool := runningAgentToolCall("schedule.delete", stringFromAny(args["id"]))
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method":               "ScheduleDelete",
		"language":             language,
		"has_schedule_manager": hasScheduleManager,
		"id":                   stringFromAny(args["id"]),
	}
	return []agentMessage{{
		ID:        "msg-schedule-delete",
		Role:      "tool",
		Content:   "schedule service ScheduleDelete",
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func TestAgentLoopToolServiceBindingWithScheduleActionServiceInjectsRuntimeCapability(t *testing.T) {
	schedule := &recordingScheduleActionService{}
	binding := runtime.AgentLoopToolServiceBinding{
		Kind: daemonToolServiceBindingKind,
		ActionServices: runtime.AgentLoopActionServiceSet{
			Capabilities: map[runtime.AgentLoopActionServiceCapability]any{},
			Metadata:     map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
		},
		Metadata: map[string]any{"adapter": "test"},
	}

	injected := agentLoopToolServiceBindingWithScheduleActionService(binding, schedule)
	resolver, ok := agentLoopActionServiceResolverFromActionServices(injected.ActionServices)
	require.True(t, ok)
	resolved, ok := resolver.ScheduleAction()
	require.True(t, ok)
	require.Same(t, schedule, resolved)
	require.Equal(t, "schedule_action", injected.ActionServices.Metadata["schedule_action_capability"])
	require.Equal(t, "daemon_schedule_action_service", injected.ActionServices.Metadata["schedule_action_adapter"])
	require.Equal(t, "runtime_capability", injected.Metadata["schedule_action_service"])

	_, originalHasSchedule := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceScheduleAction]
	require.False(t, originalHasSchedule, "schedule action injection must clone instead of mutating the input binding")

	existing := &recordingScheduleActionService{}
	withExisting := injected.Clone()
	withExisting.ActionServices.Capabilities[runtime.AgentLoopActionServiceScheduleAction] = existing
	reinjected := agentLoopToolServiceBindingWithScheduleActionService(withExisting, schedule)
	resolver, ok = agentLoopActionServiceResolverFromActionServices(reinjected.ActionServices)
	require.True(t, ok)
	resolved, ok = resolver.ScheduleAction()
	require.True(t, ok)
	require.Same(t, existing, resolved, "existing schedule action capability must not be overwritten")
}

func TestExecuteRegistryHandlerAttachesScheduleActionServiceCapability(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	called := false
	spec := &toolregistry.ToolSpec{
		Name: "test.schedule.capability.injected",
		Handler: func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
			_ = ctx
			called = true
			rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
			require.True(t, ok)
			resolver, ok := rt.actionServiceResolver()
			require.True(t, ok)
			schedule, ok := resolver.ScheduleAction()
			require.True(t, ok)
			adapter, ok := schedule.(daemonScheduleManagerActionService)
			require.True(t, ok)
			require.Same(t, mgr, adapter.manager)
			return map[string]any{"schedule_action_capability": true}, nil
		},
	}

	result, err := mgr.executeAgentRegistryHandlerResultWithToolServices(context.Background(), "schedule_capability_injected", "en", "tester", "read_only", testAgentLoopToolServices(), spec, nil, runtime.AgentLoopAuthorization{})
	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, true, result.ToolCalls[0].Result["schedule_action_capability"])
}

func TestScheduleRegistryHandlersUseRuntimeActionServiceCapability(t *testing.T) {
	schedule := &recordingScheduleActionService{}
	schedules := newScheduleManager(t.TempDir())
	baseInput := func(args map[string]any) toolregistry.HandlerInput {
		return toolregistry.HandlerInput{
			SessionID:      "schedule_capability_handler",
			Operator:       "tester",
			Language:       "zh",
			OperationLevel: "read_only",
			RawArgs:        args,
			Runtime: agentRegistryHandlerRuntime{
				Services: runtime.AgentLoopToolServiceBinding{
					Kind:   daemonToolServiceBindingKind,
					Handle: registryPanicActionServiceFallback{},
					ActionServices: runtime.AgentLoopActionServiceSet{
						Capabilities: map[runtime.AgentLoopActionServiceCapability]any{
							runtime.AgentLoopActionServiceScheduleAction: schedule,
							runtime.AgentLoopActionServiceSchedules:      schedules,
						},
						Metadata: map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
					},
					Metadata: map[string]any{"action_service_contract": runtime.AgentLoopActionServiceContractName},
				},
			},
		}
	}
	runHandler := func(name string, args map[string]any) agentRegistryHandlerResult {
		t.Helper()
		spec, ok := toolRegistry().Lookup(name)
		require.True(t, ok, "%s missing from registry", name)
		output, err := spec.Handler(context.Background(), baseInput(args))
		require.NoError(t, err)
		result, ok := output.(agentRegistryHandlerResult)
		require.True(t, ok)
		require.Len(t, result.ToolCalls, 1)
		return result
	}

	created := runHandler("schedule.create", map[string]any{"cron": "*/5 * * * *", "prompt": "daemon health", "name": "runtime-action-service"})
	listed := runHandler("schedule.list", map[string]any{})
	deleted := runHandler("schedule.delete", map[string]any{"id": "schedule-123"})

	require.Len(t, schedule.createCalls, 1)
	require.Equal(t, "schedule_capability_handler", schedule.createCalls[0].sessionID)
	require.Equal(t, "zh", schedule.createCalls[0].language)
	require.Equal(t, "tester", schedule.createCalls[0].operator)
	require.True(t, schedule.createCalls[0].hasScheduleManager)
	require.Equal(t, "daemon health", schedule.createCalls[0].args["prompt"])
	require.Equal(t, "ScheduleCreate", created.ToolCalls[0].Result["method"])
	require.Equal(t, true, created.ToolCalls[0].Result["has_schedule_manager"])

	require.Len(t, schedule.listCalls, 1)
	require.Equal(t, "zh", schedule.listCalls[0].language)
	require.True(t, schedule.listCalls[0].hasScheduleManager)
	require.Equal(t, "ScheduleList", listed.ToolCalls[0].Result["method"])

	require.Len(t, schedule.deleteCalls, 1)
	require.Equal(t, "zh", schedule.deleteCalls[0].language)
	require.True(t, schedule.deleteCalls[0].hasScheduleManager)
	require.Equal(t, "schedule-123", schedule.deleteCalls[0].args["id"])
	require.Equal(t, "ScheduleDelete", deleted.ToolCalls[0].Result["method"])
}

func TestScheduleRegistryHandlerSourceAvoidsDirectManagerScheduleCalls(t *testing.T) {
	data, err := os.ReadFile("agent_registry_handlers.go")
	require.NoError(t, err)
	src := string(data)
	require.Contains(t, src, "scheduleActionServiceFromRegistryRuntime")
	require.Contains(t, src, "scheduleActionServiceFromRegistryRuntime(in, \"schedule.create\")")
	require.Contains(t, src, "scheduleActionServiceFromRegistryRuntime(in, \"schedule.list\")")
	require.Contains(t, src, "scheduleActionServiceFromRegistryRuntime(in, \"schedule.delete\")")
	require.NotContains(t, src, "rt.Manager.handleAgentScheduleCreate(")
	require.NotContains(t, src, "rt.Manager.handleAgentScheduleList(")
	require.NotContains(t, src, "rt.Manager.handleAgentScheduleDelete(")

	adapter, err := os.ReadFile("agent_schedule_action_service.go")
	require.NoError(t, err)
	adapterSrc := string(adapter)
	require.True(t, strings.Contains(adapterSrc, "handleAgentScheduleCreate("), "schedule create manager call should be isolated to the daemon schedule adapter")
	require.True(t, strings.Contains(adapterSrc, "handleAgentScheduleList("), "schedule list manager call should be isolated to the daemon schedule adapter")
	require.True(t, strings.Contains(adapterSrc, "handleAgentScheduleDelete("), "schedule delete manager call should be isolated to the daemon schedule adapter")
}
