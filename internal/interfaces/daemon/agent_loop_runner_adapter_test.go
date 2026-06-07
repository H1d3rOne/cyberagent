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

	result := mgr.runRuntimeAgentPlannerActionReplanCycle(context.Background(), runtime.AgentLoopModeLLMToolLoop, agentPlannerActionReplanCycleRequest{
		Turn:          agentPlannerActionReplanCycleTurnContext("session-runtime-cycle", agentRuntimeTurnRef{TaskID: "task-runtime-cycle", RunID: "run-runtime-cycle", TurnID: "turn-runtime-cycle"}, "daemon health", "user", "read_only"),
		Language:      "en",
		Operator:      "tester",
		LLMRequest:    runtime.AgentLoopLLMRequest{Model: "gpt-runtime-cycle"},
		Authorization: runtime.AgentLoopAuthorization{},
		ToolContext:   agentPlannerActionReplanCycleToolContextFromRouterServices(routerServices{jobs: jobs}),
		Plan: agentPlannerActionReplanCyclePlanFromPlannerResponse(&agentPlannerResponse{
			Phase: "plan",
			Actions: []agentPlannerAction{{
				Name:      "daemon.health",
				Arguments: map[string]any{"probe": "source"},
			}},
		}),
		State: agentPlannerActionReplanCycleStateFromLegacy(
			[]agentMessage{{ID: "msg-runtime-cycle-plan", Role: "agent", Content: "plan"}},
			nil,
			[]agentMessage{{ID: "history-runtime-cycle", Role: "user", Content: "daemon health"}},
		),
		Options: agentPlannerActionReplanCycleOptions{
			ActionAppendOrigin:                      "planner_loop",
			AppendActionToolCallsBeforeEmptyMessage: true,
		},
	})

	require.True(t, called)
	require.True(t, result.Continue)
	require.True(t, result.Plan.shouldAutoContinue())
	require.Equal(t, "next", result.Plan.actions()[0].Arguments["probe"])
	responses := result.State.legacyResponseMessages()
	require.Len(t, responses, 2)
	require.Equal(t, "msg-runtime-cycle-runner", responses[1].ID)
	require.Equal(t, "runner", result.State.legacyToolCalls()[0].Metadata["phase"])
	require.Equal(t, []string{"history-runtime-cycle", "msg-runtime-cycle-runner"}, agentMessageIDs(result.State.legacyLoopHistory()))
}
