package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentPlannerActionReplanCycleRequestAgentLoopAdapterRoundTripClones(t *testing.T) {
	lastMessageAt := time.Date(2026, 6, 7, 10, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 7, 10, 35, 0, 0, time.UTC)
	llmEnabled := true
	authEnabled := false
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	turnPayload := map[string]any{"payload": "shared"}

	req := agentPlannerActionReplanCycleRequest{
		Turn: runtime.TurnContext{
			SessionID:      "session-loop-adapter",
			TaskID:         "task-loop-adapter",
			RunID:          "run-loop-adapter",
			TurnID:         "turn-loop-adapter",
			Prompt:         "continue daemon health",
			Origin:         "user",
			OperationLevel: "safe_scan",
			Payload:        turnPayload,
			Metadata:       map[string]any{"turn_key": "turn_value"},
		},
		Language: "zh-CN",
		Operator: "operator-loop-adapter",
		LLMRequest: runtime.AgentLoopLLMRequest{
			Origin:         "user",
			OperationLevel: "safe_scan",
			Provider:       "openai",
			Model:          "gpt-loop-adapter",
			APIKey:         "llm-secret",
			AuthProfiles: []runtime.AgentLoopAuthProfile{{
				ID:      "llm-auth-loop-adapter",
				Token:   "llm-token",
				Enabled: &llmEnabled,
			}},
			DefaultAuthProfileID: "llm-auth-loop-adapter",
		},
		Authorization: runtime.AgentLoopAuthorization{
			AuthProfiles: []runtime.AgentLoopAuthProfile{{
				ID:      "action-auth-loop-adapter",
				Token:   "action-token",
				Enabled: &authEnabled,
			}},
			DefaultAuthProfileID: "action-auth-loop-adapter",
		},
		MemorySummary: agentPlannerActionReplanCycleMemorySummaryFromAgentMemorySummary(&agentMemorySummary{
			Summary:       "cycle memory",
			MessageCount:  4,
			LastMessageID: "msg-loop-adapter-history",
			LastMessageAt: &lastMessageAt,
			UpdatedAt:     updatedAt,
		}),
		ToolContext: agentPlannerActionReplanCycleToolContextFromRouterServices(routerServices{jobs: jobs}),
		Plan: agentPlannerActionReplanCyclePlanFromPlannerResponse(&agentPlannerResponse{
			Phase: "step_replan",
			NextAction: &agentNextAction{
				Type:         "continue",
				AutoContinue: true,
			},
			Actions: []agentPlannerAction{{
				Name:      "daemon.health",
				Arguments: map[string]any{"target": "source-target"},
			}},
			ToolCalls: []agentPlannerAction{{
				Name:      "daemon.health",
				Arguments: map[string]any{"target": "source-target-2"},
			}},
		}),
		State: agentPlannerActionReplanCycleStateFromLegacy(
			[]agentMessage{{
				ID:      "msg-loop-adapter-response",
				Role:    "agent",
				Content: "response content",
				ToolCalls: []agentToolCall{{
					ID:       "tool-loop-adapter-message",
					Name:     "daemon.health",
					Result:   map[string]any{"ok": true},
					Metadata: map[string]interface{}{"phase": "response"},
				}},
			}},
			[]agentToolCall{{
				ID:       "tool-loop-adapter-top-level",
				Name:     "daemon.health",
				Result:   map[string]any{"ok": true},
				Metadata: map[string]interface{}{"scope": "top-level"},
			}},
			[]agentMessage{{
				ID:      "msg-loop-adapter-history",
				Role:    "user",
				Content: "history content",
			}},
		),
		Options: agentPlannerActionReplanCycleOptions{
			ActionAppendOrigin:                      "planner_loop",
			AppendActionToolCallsBeforeEmptyMessage: true,
		},
	}

	loopReq := req.agentLoopRequest(runtime.AgentLoopModeLLMToolLoop)
	require.Equal(t, runtime.AgentLoopModeLLMToolLoop, loopReq.Mode)
	require.Equal(t, "zh-CN", loopReq.Language)
	require.Equal(t, "operator-loop-adapter", loopReq.Operator)
	require.Equal(t, "planner_loop", loopReq.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])
	require.Equal(t, true, loopReq.Metadata[agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage])
	require.Equal(t, []string{"msg-loop-adapter-history"}, agentLoopMessageIDs(loopReq.History))
	require.Equal(t, []string{"msg-loop-adapter-history"}, agentLoopMessageIDs(loopReq.State.LoopHistory))
	recoveredServices, ok := routerServicesFromAgentLoopToolServiceBinding(loopReq.ToolServices)
	require.True(t, ok)
	require.Same(t, jobs, recoveredServices.jobs)

	req.Turn.Metadata["turn_key"] = "mutated-turn"
	req.LLMRequest.AuthProfiles[0].Token = "mutated-llm-token"
	*req.LLMRequest.AuthProfiles[0].Enabled = false
	req.Authorization.AuthProfiles[0].Token = "mutated-action-token"
	*req.Authorization.AuthProfiles[0].Enabled = true
	req.MemorySummary.item.Summary = "mutated memory"
	*req.MemorySummary.item.LastMessageAt = lastMessageAt.Add(time.Hour)
	req.ToolContext.services.Metadata["adapter"] = "mutated-adapter"
	req.Plan.item.Actions[0].Arguments["target"] = "mutated-target"
	req.Plan.item.NextAction.AutoContinue = false
	req.State.state.ResponseMessages[0].Content = "mutated response"
	req.State.state.ResponseMessages[0].ToolCalls[0].Result["ok"] = false
	req.State.state.ToolCalls[0].Metadata["scope"] = "mutated-top-level"
	req.State.state.LoopHistory[0].Content = "mutated history"

	require.Equal(t, "turn_value", loopReq.Turn.Metadata["turn_key"])
	require.Equal(t, "llm-token", loopReq.LLMRequest.AuthProfiles[0].Token)
	require.True(t, *loopReq.LLMRequest.AuthProfiles[0].Enabled)
	require.Equal(t, "action-token", loopReq.Authorization.AuthProfiles[0].Token)
	require.False(t, *loopReq.Authorization.AuthProfiles[0].Enabled)
	require.Equal(t, "cycle memory", loopReq.MemorySummary.Summary)
	require.Equal(t, lastMessageAt, *loopReq.MemorySummary.LastMessageAt)
	require.Equal(t, "daemon_legacy_router_services", loopReq.ToolServices.Metadata["adapter"])
	require.Equal(t, "source-target", loopReq.InitialPlan.Actions[0].Arguments["target"])
	require.True(t, loopReq.InitialPlan.NextAction.AutoContinue)
	require.Equal(t, "response content", loopReq.State.ResponseMessages[0].Content)
	require.Equal(t, true, loopReq.State.ResponseMessages[0].ToolCalls[0].Result["ok"])
	require.Equal(t, "top-level", loopReq.State.ToolCalls[0].Metadata["scope"])
	require.Equal(t, "history content", loopReq.State.LoopHistory[0].Content)

	turnPayload["payload"] = "mutated-shared"
	require.Equal(t, "mutated-shared", loopReq.Turn.Payload.(map[string]any)["payload"])

	roundTrip := agentPlannerActionReplanCycleRequestFromAgentLoopRequest(loopReq, agentPlannerActionReplanCycleOptions{})
	require.Equal(t, "session-loop-adapter", roundTrip.Turn.SessionID)
	require.Equal(t, "turn_value", roundTrip.Turn.Metadata["turn_key"])
	require.Equal(t, "gpt-loop-adapter", roundTrip.LLMRequest.Model)
	require.Equal(t, "llm-token", roundTrip.LLMRequest.AuthProfiles[0].Token)
	require.True(t, *roundTrip.LLMRequest.AuthProfiles[0].Enabled)
	require.Equal(t, "action-token", roundTrip.Authorization.AuthProfiles[0].Token)
	require.False(t, *roundTrip.Authorization.AuthProfiles[0].Enabled)
	require.Equal(t, "cycle memory", roundTrip.MemorySummary.legacyAgentMemorySummary().Summary)
	require.Equal(t, lastMessageAt, *roundTrip.MemorySummary.legacyAgentMemorySummary().LastMessageAt)
	require.True(t, roundTrip.Plan.shouldAutoContinue())
	require.Equal(t, 2, roundTrip.Plan.actionCount())
	require.Equal(t, "source-target", roundTrip.Plan.actions()[0].Arguments["target"])
	require.Equal(t, "source-target-2", roundTrip.Plan.actions()[1].Arguments["target"])
	require.Equal(t, "response content", roundTrip.State.legacyResponseMessages()[0].Content)
	require.Equal(t, "history content", roundTrip.State.legacyLoopHistory()[0].Content)
	require.Equal(t, "planner_loop", roundTrip.Options.ActionAppendOrigin)
	require.True(t, roundTrip.Options.AppendActionToolCallsBeforeEmptyMessage)
	require.Same(t, jobs, roundTrip.ToolContext.legacyRouterServices().jobs)

	loopReq.Turn.Metadata["turn_key"] = "mutated-loop-turn"
	loopReq.LLMRequest.AuthProfiles[0].Token = "mutated-loop-llm-token"
	*loopReq.LLMRequest.AuthProfiles[0].Enabled = false
	loopReq.Authorization.AuthProfiles[0].Token = "mutated-loop-action-token"
	loopReq.MemorySummary.Summary = "mutated-loop-memory"
	loopReq.InitialPlan.Actions[0].Arguments["target"] = "mutated-loop-target"
	loopReq.State.ResponseMessages[0].Content = "mutated-loop-response"
	loopReq.State.LoopHistory[0].Content = "mutated-loop-history"

	require.Equal(t, "turn_value", roundTrip.Turn.Metadata["turn_key"])
	require.Equal(t, "llm-token", roundTrip.LLMRequest.AuthProfiles[0].Token)
	require.True(t, *roundTrip.LLMRequest.AuthProfiles[0].Enabled)
	require.Equal(t, "action-token", roundTrip.Authorization.AuthProfiles[0].Token)
	require.Equal(t, "cycle memory", roundTrip.MemorySummary.legacyAgentMemorySummary().Summary)
	require.Equal(t, "source-target", roundTrip.Plan.actions()[0].Arguments["target"])
	require.Equal(t, "response content", roundTrip.State.legacyResponseMessages()[0].Content)
	require.Equal(t, "history content", roundTrip.State.legacyLoopHistory()[0].Content)
}

func TestAgentPlannerActionReplanCycleRuntimeRequestHelpersCloneAndDefault(t *testing.T) {
	loopReq := runtime.AgentLoopRequest{
		Turn: runtime.TurnContext{
			SessionID:      "session-runtime-helper",
			TaskID:         "task-runtime-helper",
			RunID:          "run-runtime-helper",
			TurnID:         "turn-runtime-helper",
			Prompt:         "continue daemon health",
			OperationLevel: "read_only",
			Metadata:       map[string]any{"turn_key": "turn_value"},
		},
		History: []runtime.AgentLoopMessage{{
			ID:      "history-runtime-helper",
			Role:    "user",
			Content: "history from request",
		}},
		State: runtime.AgentLoopState{
			ResponseMessages: []runtime.AgentLoopMessage{{
				ID:      "response-runtime-helper",
				Role:    "agent",
				Content: "existing response",
			}},
		},
		InitialPlan: &runtime.AgentLoopPlan{
			Phase:      "step_replan",
			NextAction: &runtime.AgentLoopNextAction{Type: "continue", AutoContinue: true},
			Actions: []runtime.AgentLoopAction{{
				Name:      "daemon.health",
				Arguments: map[string]any{"probe": "runtime-helper"},
			}},
		},
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage: true,
		},
	}

	turn := agentPlannerActionReplanCycleTurnFromAgentLoopRequest(loopReq)
	turnRef := agentPlannerActionReplanCycleTurnRefFromAgentLoopRequest(loopReq)
	options := agentPlannerActionReplanCycleOptionsFromAgentLoopRequest(loopReq)
	state := agentPlannerActionReplanCycleStateFromAgentLoopRequest(loopReq)
	plan := agentPlannerActionReplanCycleInitialAgentLoopPlanFromRequest(loopReq)
	actions := agentLoopPlanActions(plan)

	require.Equal(t, "turn_value", turn.Metadata["turn_key"])
	require.Equal(t, agentRuntimeTurnRef{TaskID: "task-runtime-helper", RunID: "run-runtime-helper", TurnID: "turn-runtime-helper"}, turnRef)
	require.Equal(t, "planner_loop", options.ActionAppendOrigin)
	require.True(t, options.AppendActionToolCallsBeforeEmptyMessage)
	require.Equal(t, []string{"history-runtime-helper"}, agentLoopMessageIDs(state.LoopHistory))
	require.Equal(t, []string{"response-runtime-helper"}, agentLoopMessageIDs(state.ResponseMessages))
	require.True(t, agentLoopPlanShouldAutoContinue(plan))
	require.Equal(t, "runtime-helper", actions[0].Arguments["probe"])

	loopReq.Turn.Metadata["turn_key"] = "mutated-turn"
	loopReq.History[0].Content = "mutated history"
	loopReq.State.ResponseMessages[0].Content = "mutated response"
	loopReq.InitialPlan.Actions[0].Arguments["probe"] = "mutated-probe"

	require.Equal(t, "turn_value", turn.Metadata["turn_key"])
	require.Equal(t, "history from request", state.LoopHistory[0].Content)
	require.Equal(t, "existing response", state.ResponseMessages[0].Content)
	require.Equal(t, "runtime-helper", actions[0].Arguments["probe"])
	require.Equal(t, "runtime-helper", plan.Actions[0].Arguments["probe"])
}

func TestAgentLoopPlanActionsNormalizeDedupeAndClone(t *testing.T) {
	plan := &runtime.AgentLoopPlan{
		Phase:      "step_replan",
		NextAction: &runtime.AgentLoopNextAction{Type: "continue", AutoContinue: true},
		Actions: []runtime.AgentLoopAction{{
			Arguments: map[string]any{"name": "daemon.health", "target": "same-target"},
		}},
		ToolCalls: []runtime.AgentLoopAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"target": "same-target"},
		}, {
			Name:      "workspace.read.file",
			Arguments: map[string]any{"path": "README.md"},
		}},
	}

	actions := agentLoopPlanActions(plan)
	legacyActions := agentLoopPlanLegacyActions(plan)

	require.True(t, agentLoopPlanHasActions(plan))
	require.True(t, agentLoopPlanShouldAutoContinue(plan))
	require.Len(t, actions, 2)
	require.Equal(t, []string{"daemon.health", "workspace.read.file"}, []string{actions[0].Name, actions[1].Name})
	require.Equal(t, "same-target", actions[0].Arguments["target"])
	require.Equal(t, "README.md", actions[1].Arguments["path"])
	require.Equal(t, []agentPlannerAction{{
		Name:      "daemon.health",
		Arguments: map[string]any{"name": "daemon.health", "target": "same-target"},
	}, {
		Name:      "workspace.read.file",
		Arguments: map[string]any{"path": "README.md"},
	}}, legacyActions)

	plan.Actions[0].Arguments["target"] = "mutated-target"
	plan.ToolCalls[1].Arguments["path"] = "mutated.md"

	require.Equal(t, "same-target", actions[0].Arguments["target"])
	require.Equal(t, "README.md", actions[1].Arguments["path"])
	require.Equal(t, "same-target", legacyActions[0].Arguments["target"])
	require.Equal(t, "README.md", legacyActions[1].Arguments["path"])
}

func TestAgentPlannerActionReplanCycleResultAgentLoopAdapterRoundTripClones(t *testing.T) {
	result := agentPlannerActionReplanCycleResult{
		State: agentPlannerActionReplanCycleStateFromLegacy(
			[]agentMessage{{
				ID:      "msg-loop-result-response",
				Role:    "agent",
				Content: "result response",
				ToolCalls: []agentToolCall{{
					ID:     "tool-loop-result-message",
					Name:   "daemon.health",
					Result: map[string]any{"ok": true},
				}},
			}},
			[]agentToolCall{{
				ID:       "tool-loop-result-top-level",
				Name:     "daemon.health",
				Result:   map[string]any{"ok": true},
				Metadata: map[string]interface{}{"scope": "top-level"},
			}},
			[]agentMessage{{
				ID:      "msg-loop-result-history",
				Role:    "tool",
				Content: "result history",
			}},
		),
		Plan: agentPlannerActionReplanCyclePlanFromPlannerResponse(&agentPlannerResponse{
			Phase: "step_replan",
			NextAction: &agentNextAction{
				Type:         "continue",
				AutoContinue: true,
			},
			Actions: []agentPlannerAction{{
				Name:      "workspace.read_file",
				Arguments: map[string]any{"path": "README.md"},
			}},
		}),
		Continue: true,
	}

	loopResult := result.agentLoopResult("cycle_complete")
	require.Equal(t, "cycle_complete", loopResult.StopReason)
	require.Equal(t, true, loopResult.Metadata[agentPlannerActionReplanCycleMetadataContinue])
	require.Equal(t, loopResult.State.ResponseMessages, loopResult.Messages)
	require.Equal(t, loopResult.State.ToolCalls, loopResult.ToolCalls)
	require.Equal(t, "README.md", loopResult.FinalPlan.Actions[0].Arguments["path"])

	result.State.state.ResponseMessages[0].Content = "mutated response"
	result.State.state.ResponseMessages[0].ToolCalls[0].Result["ok"] = false
	result.State.state.ToolCalls[0].Metadata["scope"] = "mutated-top-level"
	result.State.state.LoopHistory[0].Content = "mutated history"
	result.Plan.item.Actions[0].Arguments["path"] = "mutated.md"
	result.Plan.item.NextAction.AutoContinue = false

	require.Equal(t, "result response", loopResult.Messages[0].Content)
	require.Equal(t, true, loopResult.Messages[0].ToolCalls[0].Result["ok"])
	require.Equal(t, "top-level", loopResult.ToolCalls[0].Metadata["scope"])
	require.Equal(t, "result history", loopResult.State.LoopHistory[0].Content)
	require.Equal(t, "README.md", loopResult.FinalPlan.Actions[0].Arguments["path"])
	require.True(t, loopResult.FinalPlan.NextAction.AutoContinue)

	roundTrip := agentPlannerActionReplanCycleResultFromAgentLoopResult(loopResult)
	require.True(t, roundTrip.Continue)
	require.Equal(t, "result response", roundTrip.State.legacyResponseMessages()[0].Content)
	require.Equal(t, "top-level", roundTrip.State.legacyToolCalls()[0].Metadata["scope"])
	require.Equal(t, "result history", roundTrip.State.legacyLoopHistory()[0].Content)
	require.True(t, roundTrip.Plan.shouldAutoContinue())
	require.Equal(t, "README.md", roundTrip.Plan.actions()[0].Arguments["path"])

	loopResult.State.ResponseMessages[0].Content = "mutated-loop-result-response"
	loopResult.State.ToolCalls[0].Metadata["scope"] = "mutated-loop-result-top-level"
	loopResult.FinalPlan.Actions[0].Arguments["path"] = "mutated-loop-result.md"
	loopResult.Metadata[agentPlannerActionReplanCycleMetadataContinue] = false

	require.True(t, roundTrip.Continue)
	require.Equal(t, "result response", roundTrip.State.legacyResponseMessages()[0].Content)
	require.Equal(t, "top-level", roundTrip.State.legacyToolCalls()[0].Metadata["scope"])
	require.Equal(t, "README.md", roundTrip.Plan.actions()[0].Arguments["path"])
}

func TestAgentManagerRunRuntimeAgentPlannerActionReplanCycleDispatchesEngineRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	called := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, loopReq runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		called = true
		require.Equal(t, runtime.AgentLoopModeLLMToolLoop, loopReq.Mode)
		require.Equal(t, "session-runtime-cycle", loopReq.Turn.SessionID)
		require.Equal(t, "task-runtime-cycle", loopReq.Turn.TaskID)
		require.Equal(t, "run-runtime-cycle", loopReq.Turn.RunID)
		require.Equal(t, "turn-runtime-cycle", loopReq.Turn.TurnID)
		require.Equal(t, "planner_loop", loopReq.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])
		require.Equal(t, true, loopReq.Metadata[agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage])
		require.Equal(t, []string{"history-runtime-cycle"}, agentLoopMessageIDs(loopReq.History))
		require.Equal(t, "source", loopReq.InitialPlan.Actions[0].Arguments["probe"])
		services, ok := routerServicesFromAgentLoopToolServiceBinding(loopReq.ToolServices)
		require.True(t, ok)
		require.Same(t, jobs, services.jobs)

		state := loopReq.State.Clone()
		message := runtime.AgentLoopMessage{
			ID:      "msg-runtime-cycle-runner",
			Role:    "tool",
			Content: "runner handled cycle",
		}
		tool := runtime.AgentLoopToolCall{
			ID:       "tool-runtime-cycle-runner",
			Name:     "daemon.health",
			Metadata: map[string]any{"phase": "runner"},
		}
		state.ResponseMessages = append(state.ResponseMessages, message)
		state.LoopHistory = append(state.LoopHistory, message)
		state.ToolCalls = append(state.ToolCalls, tool)
		return runtime.AgentLoopResult{
			State:      state,
			Messages:   runtime.CloneAgentLoopMessages(state.ResponseMessages),
			ToolCalls:  runtime.CloneAgentLoopToolCalls(state.ToolCalls),
			FinalPlan:  &runtime.AgentLoopPlan{Phase: "step_replan", NextAction: &runtime.AgentLoopNextAction{Type: "continue", AutoContinue: true}, Actions: []runtime.AgentLoopAction{{Name: "daemon.health", Arguments: map[string]any{"probe": "next"}}}},
			StopReason: "runner_cycle_complete",
			Metadata: map[string]any{
				agentPlannerActionReplanCycleMetadataContinue: true,
			},
		}, nil
	}))

	loopReq := runtime.AgentLoopRequest{
		Mode:          runtime.AgentLoopModeLLMToolLoop,
		Turn:          agentPlannerActionReplanCycleTurnContext("session-runtime-cycle", agentRuntimeTurnRef{TaskID: "task-runtime-cycle", RunID: "run-runtime-cycle", TurnID: "turn-runtime-cycle"}, "daemon health", "user", "read_only"),
		Language:      "en",
		Operator:      "tester",
		LLMRequest:    runtime.AgentLoopLLMRequest{Model: "gpt-runtime-cycle"},
		Authorization: runtime.AgentLoopAuthorization{},
		ToolServices:  agentLoopToolServiceBindingFromRouterServices(routerServices{jobs: jobs}),
		InitialPlan: agentLoopPlanFromPlannerResponse(&agentPlannerResponse{
			Phase: "plan",
			Actions: []agentPlannerAction{{
				Name:      "daemon.health",
				Arguments: map[string]any{"probe": "source"},
			}},
		}),
		State: agentLoopStateFromLegacy(
			[]agentMessage{{ID: "msg-runtime-cycle-plan", Role: "agent", Content: "plan"}},
			nil,
			[]agentMessage{{ID: "history-runtime-cycle", Role: "user", Content: "daemon health"}},
		),
		History: []runtime.AgentLoopMessage{{ID: "history-runtime-cycle", Role: "user", Content: "daemon health"}},
		Metadata: agentPlannerActionReplanCycleMetadataFromOptions(agentPlannerActionReplanCycleOptions{
			ActionAppendOrigin:                      "planner_loop",
			AppendActionToolCallsBeforeEmptyMessage: true,
		}),
	}
	loopResult := mgr.runRuntimeAgentPlannerActionReplanCycleAgentLoop(context.Background(), loopReq)
	result := agentPlannerActionReplanCycleResultFromAgentLoopResult(loopResult)

	require.True(t, called)
	require.Equal(t, "runner_cycle_complete", loopResult.StopReason)
	require.True(t, result.Continue)
	require.True(t, result.Plan.shouldAutoContinue())
	require.Equal(t, "next", result.Plan.actions()[0].Arguments["probe"])
	responses := result.State.legacyResponseMessages()
	require.Len(t, responses, 2)
	require.Equal(t, "msg-runtime-cycle-runner", responses[1].ID)
	require.Equal(t, "runner", result.State.legacyToolCalls()[0].Metadata["phase"])
	require.Equal(t, []string{"history-runtime-cycle", "msg-runtime-cycle-runner"}, agentMessageIDs(result.State.legacyLoopHistory()))
}

func TestAgentManagerRunAgentLLMToolLoopCarriesRuntimeLoopDTOBetweenCycles(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	created, err := mgr.CreateSession(createAgentSessionRequest{
		Operator:       "tester",
		Language:       "en",
		OperationLevel: "read_only",
	})
	require.NoError(t, err)
	sessionID := created.Session.SessionID

	planMessage := agentMessage{ID: "msg-runtime-loop-dto-plan", Role: "agent", Content: "plan first", Phase: "plan"}
	firstPlan := &agentPlannerResponse{
		Phase: "plan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "first"},
		}},
	}

	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		require.Equal(t, runtime.LLMStagePlannerChat, req.Stage)
		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "gpt-runtime-loop-dto", payload.LLMRequest.Model)
		require.Equal(t, []string{"history-runtime-loop-dto"}, agentLoopMessageIDs(payload.History))

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "planner handled by runtime DTO test",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{planMessage}, nil, firstPlan),
		}, nil
	}))

	loopRequests := []runtime.AgentLoopRequest{}
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, loopReq runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		loopRequests = append(loopRequests, loopReq.Clone())
		require.Equal(t, runtime.AgentLoopModeLLMToolLoop, loopReq.Mode)
		require.Equal(t, sessionID, loopReq.Turn.SessionID)
		require.Equal(t, "task-runtime-loop-dto", loopReq.Turn.TaskID)
		require.Equal(t, "run-runtime-loop-dto", loopReq.Turn.RunID)
		require.Equal(t, "turn-runtime-loop-dto", loopReq.Turn.TurnID)
		require.Equal(t, "planner_loop", loopReq.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])
		require.Equal(t, true, loopReq.Metadata[agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage])

		state := loopReq.State.Clone()
		switch len(loopRequests) {
		case 1:
			require.Equal(t, []string{"history-runtime-loop-dto", planMessage.ID}, agentLoopMessageIDs(loopReq.History))
			require.Equal(t, []string{planMessage.ID}, agentLoopMessageIDs(loopReq.State.ResponseMessages))
			require.Equal(t, "first", loopReq.InitialPlan.Actions[0].Arguments["probe"])

			message := runtime.AgentLoopMessage{ID: "msg-runtime-loop-dto-cycle-1", Role: "tool", Content: "first cycle handled"}
			tool := runtime.AgentLoopToolCall{ID: "tool-runtime-loop-dto-cycle-1", Name: "daemon.health"}
			state.ResponseMessages = append(state.ResponseMessages, message)
			state.LoopHistory = append(state.LoopHistory, message)
			state.ToolCalls = append(state.ToolCalls, tool)
			return runtime.AgentLoopResult{
				State:     state,
				Messages:  runtime.CloneAgentLoopMessages(state.ResponseMessages),
				ToolCalls: runtime.CloneAgentLoopToolCalls(state.ToolCalls),
				FinalPlan: &runtime.AgentLoopPlan{
					Phase:      "step_replan",
					NextAction: &runtime.AgentLoopNextAction{Type: "continue", AutoContinue: true},
					Actions: []runtime.AgentLoopAction{{
						Name:      "daemon.health",
						Arguments: map[string]any{"probe": "second"},
					}},
				},
				StopReason: "first runtime loop cycle",
				Metadata: map[string]any{
					agentPlannerActionReplanCycleMetadataContinue: true,
				},
			}, nil
		case 2:
			require.Equal(t, []string{"history-runtime-loop-dto", planMessage.ID, "msg-runtime-loop-dto-cycle-1"}, agentLoopMessageIDs(loopReq.History))
			require.Equal(t, []string{planMessage.ID, "msg-runtime-loop-dto-cycle-1"}, agentLoopMessageIDs(loopReq.State.ResponseMessages))
			require.Equal(t, "second", loopReq.InitialPlan.Actions[0].Arguments["probe"])

			message := runtime.AgentLoopMessage{ID: "msg-runtime-loop-dto-cycle-2", Role: "agent", Content: "done"}
			tool := runtime.AgentLoopToolCall{ID: "tool-runtime-loop-dto-cycle-2", Name: "daemon.health"}
			state.ResponseMessages = append(state.ResponseMessages, message)
			state.LoopHistory = append(state.LoopHistory, message)
			state.ToolCalls = append(state.ToolCalls, tool)
			return runtime.AgentLoopResult{
				State:      state,
				Messages:   runtime.CloneAgentLoopMessages(state.ResponseMessages),
				ToolCalls:  runtime.CloneAgentLoopToolCalls(state.ToolCalls),
				FinalPlan:  &runtime.AgentLoopPlan{Phase: "final_answer", Answer: "done"},
				StopReason: "final runtime loop cycle",
				Metadata: map[string]any{
					agentPlannerActionReplanCycleMetadataContinue: false,
				},
			}, nil
		default:
			t.Fatalf("unexpected loop request count %d", len(loopRequests))
			return runtime.AgentLoopResult{}, nil
		}
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	messages, toolCalls, err := mgr.runAgentLLMToolLoop(
		context.Background(),
		sessionID,
		agentRuntimeTurnRef{TaskID: "task-runtime-loop-dto", RunID: "run-runtime-loop-dto", TurnID: "turn-runtime-loop-dto"},
		"daemon health twice through runtime DTO",
		"en",
		"tester",
		"read_only",
		sendAgentMessageRequest{Model: "gpt-runtime-loop-dto"},
		[]agentMessage{{ID: "history-runtime-loop-dto", Role: "user", Content: "daemon health twice through runtime DTO"}},
		&agentMemorySummary{Summary: "runtime loop memory"},
		routerServices{},
	)

	require.NoError(t, err)
	require.Len(t, loopRequests, 2)
	require.Len(t, appendMutations, 1)
	require.Equal(t, string(runtime.LLMStagePlannerChat), appendMutations[0].Turn.Origin)
	require.Equal(t, []string{planMessage.ID, "msg-runtime-loop-dto-cycle-1", "msg-runtime-loop-dto-cycle-2"}, agentMessageIDs(messages))
	require.Equal(t, []string{"tool-runtime-loop-dto-cycle-1", "tool-runtime-loop-dto-cycle-2"}, agentToolCallIDs(toolCalls))
}

func TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	actionMessage := agentMessage{
		ID:      "msg-loop-runner-action",
		Role:    "tool",
		Content: "runtime request action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-loop-runner-action",
			Name: "daemon.health",
		}},
	}
	synthesisMessage := agentMessage{ID: "msg-loop-runner-synthesis", Role: "agent", Content: "runtime request summarized"}
	finalPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "runtime request summarized"}

	actionCalled := false
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		actionCalled = true
		require.Equal(t, "session-loop-runner", turn.SessionID)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		payload, ok := turn.Payload.(daemonPlannerActionExecutionTurnPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "runtime-request", stringFromAny(payload.Action.Arguments["probe"]))

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "runtime request action handled",
			Payload: daemonPlannerActionExecutionTurnResultPayload{
				Messages:  []agentMessage{actionMessage},
				ToolCalls: actionMessage.ToolCalls,
			},
		}, nil
	}))

	synthesisCalled := false
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		synthesisCalled = true
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "gpt-loop-runner", payload.LLMRequest.Model)
		require.Equal(t, []string{"history-loop-runner", "msg-loop-runner-plan", actionMessage.ID}, agentLoopMessageIDs(payload.History))
		require.Equal(t, actionMessage.ID, payload.ToolMessages[0].ID)

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "runtime request synthesis handled",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, nil, finalPlan),
		}, nil
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	initialPlan := agentPlannerActionReplanCyclePlanFromPlannerResponse(&agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "runtime-request"},
		}},
	})
	loopReq := runtime.AgentLoopRequest{
		Mode: runtime.AgentLoopModeReplanContinuation,
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-loop-runner",
			agentRuntimeTurnRef{TaskID: "task-loop-runner", RunID: "run-loop-runner", TurnID: "turn-loop-runner"},
			"continue daemon health",
			"user",
			"read_only",
		),
		Language:      "en",
		Operator:      "tester",
		LLMRequest:    runtime.AgentLoopLLMRequest{Model: "gpt-loop-runner"},
		Authorization: runtime.AgentLoopAuthorization{},
		InitialPlan:   initialPlan.agentLoopPlan(),
		State: runtime.AgentLoopState{
			ResponseMessages: []runtime.AgentLoopMessage{{ID: "msg-loop-runner-plan", Role: "agent", Content: "plan"}},
			LoopHistory:      []runtime.AgentLoopMessage{{ID: "history-loop-runner", Role: "user", Content: "continue daemon health"}, {ID: "msg-loop-runner-plan", Role: "agent", Content: "plan"}},
		},
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataActionAppendOrigin: daemonTurnExecutionReplanActionContinuation,
		},
	}

	loopResult, err := daemonAgentLoopRunner{manager: mgr}.RunAgentLoop(context.Background(), loopReq)

	require.NoError(t, err)
	require.True(t, actionCalled)
	require.True(t, synthesisCalled)
	require.Equal(t, agentPlannerActionReplanCycleStopReasonLegacyDaemonLoopExecuted, loopResult.StopReason)
	require.Equal(t, true, loopResult.Metadata[agentPlannerActionReplanCycleMetadataContinue])
	require.Equal(t, "runtime-request", loopReq.InitialPlan.Actions[0].Arguments["probe"])
	require.Len(t, appendMutations, 2)
	require.Equal(t, daemonTurnExecutionReplanActionContinuation, appendMutations[0].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[1].Turn.Origin)

	require.Equal(t, []string{"msg-loop-runner-plan", actionMessage.ID, synthesisMessage.ID}, agentLoopMessageIDs(loopResult.Messages))
	require.Equal(t, []string{"history-loop-runner", "msg-loop-runner-plan", actionMessage.ID, synthesisMessage.ID}, agentLoopMessageIDs(loopResult.State.LoopHistory))
	require.Equal(t, actionMessage.ToolCalls[0].ID, loopResult.ToolCalls[0].ID)
	require.Equal(t, finalPlan.Answer, loopResult.FinalPlan.Answer)
}

func agentToolCallIDs(calls []agentToolCall) []string {
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		ids = append(ids, call.ID)
	}
	return ids
}
