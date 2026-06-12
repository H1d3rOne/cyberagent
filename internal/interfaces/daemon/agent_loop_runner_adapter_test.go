package daemon

import (
	"context"
	"errors"
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
		ToolContext: agentPlannerActionReplanCycleToolContextFromToolServices(agentLoopToolServiceBindingFromRouterServices(routerServices{jobs: jobs})),
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
	recoveredJobs, ok := agentLoopActionServiceCapabilityValue[*jobManager](loopReq.ToolServices.ActionServices, runtime.AgentLoopActionServiceJobs)
	require.True(t, ok)
	require.Same(t, jobs, recoveredJobs)

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
	roundTripJobs, ok := agentLoopActionServiceCapabilityValue[*jobManager](roundTrip.ToolContext.services.ActionServices, runtime.AgentLoopActionServiceJobs)
	require.True(t, ok)
	require.Same(t, jobs, roundTripJobs)

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

func TestNormalizeAgentLLMToolLoopRequestDefaultsAndPreservesExplicitMetadata(t *testing.T) {
	loopReq := runtime.AgentLoopRequest{
		MaxLoops: 0,
		Metadata: map[string]any{
			"keep": "value",
		},
	}

	normalized := normalizeAgentLLMToolLoopRequest(loopReq)

	require.Equal(t, runtime.AgentLoopModeLLMToolLoop, normalized.Mode)
	require.Equal(t, 3, normalized.MaxLoops)
	require.Equal(t, "value", normalized.Metadata["keep"])
	require.Equal(t, "planner_loop", normalized.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])
	require.Equal(t, true, normalized.Metadata[agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage])

	loopReq.Metadata["keep"] = "mutated"
	require.Equal(t, "value", normalized.Metadata["keep"])

	explicit := normalizeAgentLLMToolLoopRequest(runtime.AgentLoopRequest{
		MaxLoops: 7,
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataActionAppendOrigin:                      "custom_origin",
			agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage: false,
		},
	})
	require.Equal(t, runtime.AgentLoopModeLLMToolLoop, explicit.Mode)
	require.Equal(t, 7, explicit.MaxLoops)
	require.Equal(t, "custom_origin", explicit.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])
	require.Equal(t, false, explicit.Metadata[agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage])
}

func TestAgentLLMToolLoopStepPreconditionFallbackAndStopBranches(t *testing.T) {
	planOnly := &runtime.AgentLoopPlan{
		Phase:  "plan",
		Answer: "plan only",
	}
	hold := agentLLMToolLoopStepPrecondition(runtime.AgentLoopRequest{
		Turn:     runtime.TurnContext{Prompt: "make a plan for daemon health"},
		Language: "en",
	}, 0, planOnly)
	require.False(t, hold.Continue)
	require.False(t, agentLoopPlanHasActions(hold.Plan))

	run := agentLLMToolLoopStepPrecondition(runtime.AgentLoopRequest{
		Turn:     runtime.TurnContext{Prompt: "run daemon health now"},
		Language: "en",
	}, 0, nil)
	require.True(t, run.Continue)
	require.True(t, agentLoopPlanShouldAutoContinue(run.Plan))
	require.Equal(t, []string{"daemon.health"}, []string{agentLoopPlanActions(run.Plan)[0].Name})

	secondStep := agentLLMToolLoopStepPrecondition(runtime.AgentLoopRequest{
		Turn:     runtime.TurnContext{Prompt: "run daemon health now"},
		Language: "en",
	}, 1, nil)
	require.False(t, secondStep.Continue)
}

func TestAgentLLMToolLoopNextPlanFromCycleResultStopsWithoutAutoContinue(t *testing.T) {
	finalPlan := &runtime.AgentLoopPlan{
		Phase:      "step_replan",
		NextAction: &runtime.AgentLoopNextAction{Type: "continue", AutoContinue: true},
		Actions: []runtime.AgentLoopAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "next"},
		}},
	}
	next, ok := agentLLMToolLoopNextPlanFromCycleResult(runtime.AgentLoopResult{
		FinalPlan: finalPlan,
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataContinue: true,
		},
	})
	require.True(t, ok)
	require.Equal(t, "next", next.Actions[0].Arguments["probe"])

	finalPlan.Actions[0].Arguments["probe"] = "mutated"
	require.Equal(t, "next", next.Actions[0].Arguments["probe"])

	noContinue, ok := agentLLMToolLoopNextPlanFromCycleResult(runtime.AgentLoopResult{
		FinalPlan: finalPlan,
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataContinue: false,
		},
	})
	require.False(t, ok)
	require.Nil(t, noContinue)

	manualContinuePlan := runtime.CloneAgentLoopPlan(finalPlan)
	manualContinuePlan.NextAction.AutoContinue = false
	manualContinue, ok := agentLLMToolLoopNextPlanFromCycleResult(runtime.AgentLoopResult{
		FinalPlan: manualContinuePlan,
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataContinue: true,
		},
	})
	require.False(t, ok)
	require.Nil(t, manualContinue)

	manualDecision := agentLLMToolLoopNextPlanDecisionFromCycleResult(runtime.AgentLoopResult{
		FinalPlan: manualContinuePlan,
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataContinue: true,
		},
	})
	require.False(t, manualDecision.Continue)
	require.Equal(t, runtime.AgentLoopStopReasonAutoContinueDisabled, manualDecision.StopReason)

	noActionPlan := runtime.CloneAgentLoopPlan(finalPlan)
	noActionPlan.Actions = nil
	noActionDecision := agentLLMToolLoopNextPlanDecisionFromCycleResult(runtime.AgentLoopResult{
		FinalPlan: noActionPlan,
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataContinue: true,
		},
	})
	require.False(t, noActionDecision.Continue)
	require.Equal(t, runtime.AgentLoopStopReasonNoActions, noActionDecision.StopReason)

	cycleStopDecision := agentLLMToolLoopNextPlanDecisionFromCycleResult(runtime.AgentLoopResult{
		StopReason: runtime.AgentLoopStopReasonStepReplanSynthesisError,
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataContinue: false,
		},
	})
	require.False(t, cycleStopDecision.Continue)
	require.Equal(t, runtime.AgentLoopStopReasonStepReplanSynthesisError, cycleStopDecision.StopReason)
}

func TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisFailureEgress(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	actionMessage := agentMessage{
		ID:      "msg-cycle-synthesis-failure-action",
		Role:    "tool",
		Content: "action completed before synthesis failure",
		ToolCalls: []agentToolCall{{
			ID:   "tool-cycle-synthesis-failure-action",
			Name: "daemon.health",
		}},
	}
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "action handled",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(actionMessage.ToolCalls),
			},
		}, nil
	}))
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		return runtime.LLMStageResult{}, errors.New("synthetic step replan failure")
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	loopReq := runtime.AgentLoopRequest{
		Mode: runtime.AgentLoopModeReplanContinuation,
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-cycle-synthesis-failure",
			agentRuntimeTurnRef{TaskID: "task-cycle-synthesis-failure", RunID: "run-cycle-synthesis-failure", TurnID: "turn-cycle-synthesis-failure"},
			"continue daemon health",
			"user",
			"read_only",
		),
		Language:   "en",
		Operator:   "tester",
		LLMRequest: runtime.AgentLoopLLMRequest{Model: "gpt-cycle-synthesis-failure"},
		InitialPlan: agentLoopPlanFromPlannerResponse(&agentPlannerResponse{
			Phase: "step_replan",
			NextAction: &agentNextAction{
				Type:         "continue",
				AutoContinue: true,
			},
			Actions: []agentPlannerAction{{Name: "daemon.health"}},
		}),
		State: runtime.AgentLoopState{
			LoopHistory: []runtime.AgentLoopMessage{{ID: "history-cycle-synthesis-failure", Role: "user", Content: "continue daemon health"}},
		},
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataActionAppendOrigin: daemonTurnExecutionReplanActionContinuation,
		},
	}

	result := mgr.runAgentPlannerActionReplanCycleAgentLoop(context.Background(), loopReq)

	require.Equal(t, runtime.AgentLoopStopReasonStepReplanSynthesisError, result.StopReason)
	require.Equal(t, runtime.AgentLoopStopReasonStepReplanSynthesisError, result.Metadata[runtime.AgentLoopEgressMetadataStopReason])
	require.Equal(t, false, result.Metadata[agentPlannerActionReplanCycleMetadataContinue])
	require.Equal(t, "step_replan", result.Metadata["phase"])
	require.Contains(t, stringFromAny(result.Metadata["error"]), "synthetic step replan failure")
	require.Len(t, result.Messages, 2)
	require.Equal(t, actionMessage.ID, result.Messages[0].ID)
	require.Equal(t, "step", result.Messages[0].Phase)
	require.Equal(t, "step_replan", result.Messages[1].Phase)
	require.Contains(t, result.Messages[1].Content, "StepReplan synthesis failed")
	require.Len(t, result.ToolCalls, 2)
	require.Equal(t, "step_replan", firstNonEmpty(stringFromAny(result.ToolCalls[1].Metadata["agent_phase"]), stringFromAny(result.ToolCalls[1].Metadata["phase"])))
	require.Len(t, appendMutations, 2)
	require.Equal(t, daemonTurnExecutionReplanActionContinuation, appendMutations[0].Turn.Origin)
	require.Equal(t, daemonTurnExecutionReplanActionContinuation, appendMutations[1].Turn.Origin)
}

func TestAgentManagerRunAgentLLMToolLoopResultStopsAtMaxLoops(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	planMessage := agentMessage{ID: "msg-llm-max-plan", Role: "agent", Content: "plan", Phase: "plan"}
	actionMessage := agentMessage{
		ID:      "msg-llm-max-action",
		Role:    "tool",
		Content: "action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-llm-max-action",
			Name: "daemon.health",
		}},
	}
	synthesisMessage := agentMessage{ID: "msg-llm-max-synthesis", Role: "agent", Content: "continue", Phase: "step_replan"}
	initialPlan := &agentPlannerResponse{
		Phase: "plan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{Name: "daemon.health", Arguments: map[string]any{"probe": "first"}}},
	}
	nextPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{Name: "daemon.health", Arguments: map[string]any{"probe": "second"}}},
	}

	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "action handled",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(actionMessage.ToolCalls),
			},
		}, nil
	}))
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		switch req.Stage {
		case runtime.LLMStagePlannerChat:
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "planner handled",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{planMessage}, nil, initialPlan),
			}, nil
		case runtime.LLMStageToolSynthesis:
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "synthesis handled",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, nil, nextPlan),
			}, nil
		default:
			t.Fatalf("unexpected stage %s", req.Stage)
			return runtime.LLMStageResult{}, nil
		}
	}))
	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	result, err := mgr.runAgentLLMToolLoopAgentLoopResult(context.Background(), runtime.AgentLoopRequest{
		Mode: runtime.AgentLoopModeLLMToolLoop,
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-llm-max",
			agentRuntimeTurnRef{TaskID: "task-llm-max", RunID: "run-llm-max", TurnID: "turn-llm-max"},
			"run daemon health twice",
			"user",
			"read_only",
		),
		Language:   "en",
		Operator:   "tester",
		LLMRequest: runtime.AgentLoopLLMRequest{Model: "gpt-llm-max"},
		History:    []runtime.AgentLoopMessage{{ID: "history-llm-max", Role: "user", Content: "run daemon health twice"}},
		MaxLoops:   1,
	})

	require.NoError(t, err)
	require.Equal(t, runtime.AgentLoopStopReasonMaxLoops, result.StopReason)
	require.Equal(t, runtime.AgentLoopStopReasonMaxLoops, result.Metadata[runtime.AgentLoopEgressMetadataStopReason])
	require.Equal(t, 1, result.LoopCount)
	require.Equal(t, 1, result.Metadata[runtime.AgentLoopEgressMetadataLoopCount])
	require.Equal(t, 1, result.Metadata[runtime.AgentLoopEgressMetadataMaxLoops])
	require.Equal(t, string(runtime.AgentLoopModeLLMToolLoop), result.Metadata["mode"])
	require.NotNil(t, result.FinalPlan)
	require.Equal(t, "second", result.FinalPlan.Actions[0].Arguments["probe"])
	require.Equal(t, []string{planMessage.ID, actionMessage.ID, synthesisMessage.ID}, agentLoopMessageIDs(result.Messages))
	require.Len(t, appendMutations, 3)
}

func TestAgentManagerRunAgentReplanActionContinuationResultStopEgress(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	plan := agentLoopPlanFromPlannerResponse(&agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{Name: "daemon.health"}},
	})

	maxZero := mgr.runAgentReplanActionContinuationAgentLoopResult(context.Background(), runtime.AgentLoopRequest{
		Mode:        runtime.AgentLoopModeReplanContinuation,
		InitialPlan: runtime.CloneAgentLoopPlan(plan),
		MaxLoops:    0,
	})
	require.Equal(t, runtime.AgentLoopStopReasonMaxLoops, maxZero.StopReason)
	require.Equal(t, runtime.AgentLoopStopReasonMaxLoops, maxZero.Metadata[runtime.AgentLoopEgressMetadataStopReason])
	require.Equal(t, 0, maxZero.Metadata[runtime.AgentLoopEgressMetadataLoopCount])
	require.Equal(t, 0, maxZero.Metadata[runtime.AgentLoopEgressMetadataMaxLoops])

	manualPlan := runtime.CloneAgentLoopPlan(plan)
	manualPlan.NextAction.AutoContinue = false
	manual := mgr.runAgentReplanActionContinuationAgentLoopResult(context.Background(), runtime.AgentLoopRequest{
		Mode:        runtime.AgentLoopModeReplanContinuation,
		InitialPlan: manualPlan,
		MaxLoops:    1,
	})
	require.Equal(t, runtime.AgentLoopStopReasonAutoContinueDisabled, manual.StopReason)
	require.Equal(t, runtime.AgentLoopStopReasonAutoContinueDisabled, manual.Metadata[runtime.AgentLoopEgressMetadataStopReason])
	require.Equal(t, 0, manual.LoopCount)
	require.Equal(t, 1, manual.Metadata[runtime.AgentLoopEgressMetadataMaxLoops])
}

func TestNormalizeAgentReplanActionContinuationRequestDefaultsAndPreconditions(t *testing.T) {
	loopReq := runtime.AgentLoopRequest{
		Mode:     runtime.AgentLoopModeLLMToolLoop,
		MaxLoops: 0,
		Metadata: map[string]any{
			"keep": "value",
		},
	}

	normalized := normalizeAgentReplanActionContinuationRequest(loopReq)

	require.Equal(t, runtime.AgentLoopModeReplanContinuation, normalized.Mode)
	require.Equal(t, 0, normalized.MaxLoops)
	require.Equal(t, "value", normalized.Metadata["keep"])
	require.Equal(t, daemonTurnExecutionReplanActionContinuation, normalized.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])

	loopReq.Metadata["keep"] = "mutated"
	require.Equal(t, "value", normalized.Metadata["keep"])

	explicit := normalizeAgentReplanActionContinuationRequest(runtime.AgentLoopRequest{
		Metadata: map[string]any{
			agentPlannerActionReplanCycleMetadataActionAppendOrigin: "custom_continuation",
		},
	})
	require.Equal(t, "custom_continuation", explicit.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])

	plan := &runtime.AgentLoopPlan{
		Phase:      "step_replan",
		NextAction: &runtime.AgentLoopNextAction{Type: "continue", AutoContinue: true},
		Actions: []runtime.AgentLoopAction{{
			Name: "daemon.health",
		}},
	}
	require.False(t, agentReplanActionContinuationStepPrecondition(normalized, plan))

	normalized.MaxLoops = 1
	require.True(t, agentReplanActionContinuationStepPrecondition(normalized, plan))

	manualContinuePlan := runtime.CloneAgentLoopPlan(plan)
	manualContinuePlan.NextAction.AutoContinue = false
	require.False(t, agentReplanActionContinuationStepPrecondition(normalized, manualContinuePlan))

	noActionPlan := runtime.CloneAgentLoopPlan(plan)
	noActionPlan.Actions = nil
	require.False(t, agentReplanActionContinuationStepPrecondition(normalized, noActionPlan))
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

	require.True(t, agentLoopPlanHasActions(plan))
	require.True(t, agentLoopPlanShouldAutoContinue(plan))
	require.Len(t, actions, 2)
	require.Equal(t, []string{"daemon.health", "workspace.read.file"}, []string{actions[0].Name, actions[1].Name})
	require.Equal(t, "same-target", actions[0].Arguments["target"])
	require.Equal(t, "README.md", actions[1].Arguments["path"])
	plan.Actions[0].Arguments["target"] = "mutated-target"
	plan.ToolCalls[1].Arguments["path"] = "mutated.md"

	require.Equal(t, "same-target", actions[0].Arguments["target"])
	require.Equal(t, "README.md", actions[1].Arguments["path"])
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
		require.Equal(t, runtime.AgentLoopModePlannerActionReplanCycle, loopReq.Mode)
		require.Equal(t, string(runtime.AgentLoopModeLLMToolLoop), loopReq.Metadata[agentPlannerActionReplanCycleMetadataParentMode])
		require.Equal(t, "session-runtime-cycle", loopReq.Turn.SessionID)
		require.Equal(t, "task-runtime-cycle", loopReq.Turn.TaskID)
		require.Equal(t, "run-runtime-cycle", loopReq.Turn.RunID)
		require.Equal(t, "turn-runtime-cycle", loopReq.Turn.TurnID)
		require.Equal(t, "planner_loop", loopReq.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])
		require.Equal(t, true, loopReq.Metadata[agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage])
		require.Equal(t, []string{"history-runtime-cycle"}, agentLoopMessageIDs(loopReq.History))
		require.Equal(t, "source", loopReq.InitialPlan.Actions[0].Arguments["probe"])
		resolvedJobs, ok := agentLoopActionServiceCapabilityValue[*jobManager](loopReq.ToolServices.ActionServices, runtime.AgentLoopActionServiceJobs)
		require.True(t, ok)
		require.Same(t, jobs, resolvedJobs)

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
		require.Equal(t, runtime.AgentLoopModePlannerActionReplanCycle, loopReq.Mode)
		require.Equal(t, string(runtime.AgentLoopModeLLMToolLoop), loopReq.Metadata[agentPlannerActionReplanCycleMetadataParentMode])
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

	messages, toolCalls, err := mgr.runAgentLLMToolLoopWithToolServices(
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
		testAgentLoopToolServices(),
	)

	require.NoError(t, err)
	require.Len(t, loopRequests, 2)
	require.Len(t, appendMutations, 1)
	require.Equal(t, string(runtime.LLMStagePlannerChat), appendMutations[0].Turn.Origin)
	require.Equal(t, []string{planMessage.ID, "msg-runtime-loop-dto-cycle-1", "msg-runtime-loop-dto-cycle-2"}, agentMessageIDs(agentMessagesFromAgentLoopMessages(messages)))
	require.Equal(t, []string{"tool-runtime-loop-dto-cycle-1", "tool-runtime-loop-dto-cycle-2"}, agentToolCallIDs(agentLoopToolCallItemsToAgentToolCalls(toolCalls)))
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
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		actionCalled = true
		require.Equal(t, "session-loop-runner", turn.SessionID)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "runtime-request", stringFromAny(payload.Action.Arguments["probe"]))

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "runtime request action handled",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(actionMessage.ToolCalls),
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
		Mode: runtime.AgentLoopModePlannerActionReplanCycle,
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

func TestDaemonAgentLoopRunnerExecutesPlannerActionExecutionMode(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	expectedMessage := runtime.AgentLoopMessage{
		ID:      "msg-loop-runner-planner-action",
		Role:    "tool",
		Content: "planner action executed through AgentLoop mode",
	}
	expectedToolCall := runtime.AgentLoopToolCall{
		ID:   "tool-loop-runner-planner-action",
		Name: "daemon.health",
	}
	enabled := true
	action := runtime.AgentLoopAction{
		Name:      "daemon.health",
		Arguments: map[string]any{"probe": "planner-action-mode"},
	}
	authorization := runtime.AgentLoopAuthorization{
		AuthProfiles: []runtime.AgentLoopAuthProfile{{
			ID:      "auth-planner-action-mode",
			Token:   "secret-token",
			Enabled: &enabled,
		}},
		DefaultAuthProfileID: "auth-planner-action-mode",
	}
	toolServices := agentLoopToolServiceBindingFromRouterServices(routerServices{})
	req := runtime.AgentLoopPlannerActionExecutionRequest(
		runtime.TurnContext{
			SessionID:      "session-planner-action-mode",
			TaskID:         "task-planner-action-mode",
			RunID:          "run-planner-action-mode",
			TurnID:         "turn-planner-action-mode",
			Prompt:         "execute daemon health",
			Origin:         daemonTurnExecutionPlannerActionExecution,
			OperationLevel: "safe_scan",
			Metadata:       map[string]any{"turn_key": "turn_value"},
		},
		"en",
		"tester",
		toolServices,
		authorization,
		action,
		map[string]any{"request_key": "request_value"},
	)

	actionCalled := false
	turnCalled := false
	eng.AttachAgentLoopActionExecutor(runtime.AgentLoopActionExecutorFunc(func(ctx context.Context, got runtime.AgentLoopActionExecutorRequest) (runtime.AgentLoopActionExecutorResult, error) {
		actionCalled = true
		require.NotNil(t, ctx)
		require.Equal(t, runtime.AgentLoopModePlannerActionExecution, got.Request.Mode)
		require.Equal(t, "session-planner-action-mode", got.Request.Turn.SessionID)
		require.Equal(t, "task-planner-action-mode", got.Request.Turn.TaskID)
		require.Equal(t, "run-planner-action-mode", got.Request.Turn.RunID)
		require.Equal(t, "turn-planner-action-mode", got.Request.Turn.TurnID)
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, got.Request.Turn.Origin)
		require.Equal(t, "safe_scan", got.Request.Turn.OperationLevel)
		require.Equal(t, "turn_value", got.Request.Turn.Metadata["turn_key"])
		require.Equal(t, "request_value", got.Request.Metadata["request_key"])
		require.Equal(t, "en", got.Request.Language)
		require.Equal(t, "tester", got.Request.Operator)
		require.Equal(t, daemonToolServiceBindingKind, got.Request.ToolServices.Kind)
		require.Equal(t, "daemon_legacy_router_services", got.Request.ToolServices.Metadata["adapter"])
		require.Equal(t, authorization.DefaultAuthProfileID, got.Request.Authorization.DefaultAuthProfileID)
		require.Equal(t, authorization.AuthProfiles, got.Request.Authorization.AuthProfiles)
		require.Equal(t, action, got.Action)

		return runtime.AgentLoopActionExecutorResult{
			Status:    runtime.TurnResultFinished,
			Reason:    "planner action mode handled",
			Messages:  []runtime.AgentLoopMessage{expectedMessage},
			ToolCalls: []runtime.AgentLoopToolCall{expectedToolCall},
			Metadata:  map[string]any{"executor": "test_action_executor"},
		}, nil
	}))
	eng.AttachTurnExecutor(runtime.TurnExecutorFunc(func(_ context.Context, _ runtime.TurnContext) (runtime.TurnResult, error) {
		turnCalled = true
		return runtime.TurnResult{Status: runtime.TurnResultFailed}, errors.New("turn executor should not be called for planner_action_execution AgentLoop mode")
	}))

	loopResult, err := eng.RunAgentLoop(context.Background(), req)

	require.NoError(t, err)
	require.True(t, actionCalled)
	require.False(t, turnCalled)
	require.Equal(t, "planner action mode handled", loopResult.StopReason)
	require.False(t, loopResult.CapabilityBlocked)
	require.Equal(t, []runtime.AgentLoopMessage{expectedMessage}, loopResult.Messages)
	require.Equal(t, []runtime.AgentLoopToolCall{expectedToolCall}, loopResult.ToolCalls)
	require.Equal(t, []runtime.AgentLoopMessage{expectedMessage}, loopResult.State.ResponseMessages)
	require.Equal(t, []runtime.AgentLoopToolCall{expectedToolCall}, loopResult.State.ToolCalls)
	require.Equal(t, string(runtime.AgentLoopModePlannerActionExecution), loopResult.Metadata["mode"])
	require.Equal(t, "daemon.health", loopResult.Metadata["action_name"])
	require.Equal(t, 1, loopResult.Metadata["message_count"])
	require.Equal(t, 1, loopResult.Metadata["tool_call_count"])
	require.Equal(t, string(runtime.TurnResultFinished), loopResult.Metadata["action_executor_status"])
	require.Equal(t, "test_action_executor", loopResult.Metadata["executor"])
}

func TestDaemonAgentLoopRunnerExecutesLLMToolOuterLoopFromRuntimeRequest(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	baseRunner := daemonAgentLoopRunner{manager: mgr}
	loopModes := []runtime.AgentLoopMode{}
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(ctx context.Context, loopReq runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		loopModes = append(loopModes, loopReq.Mode)
		switch loopReq.Mode {
		case runtime.AgentLoopModeLLMToolLoop:
			require.Empty(t, stringFromAny(loopReq.Metadata[agentPlannerActionReplanCycleMetadataParentMode]))
		case runtime.AgentLoopModePlannerActionReplanCycle:
			require.Equal(t, string(runtime.AgentLoopModeLLMToolLoop), loopReq.Metadata[agentPlannerActionReplanCycleMetadataParentMode])
			require.Equal(t, "planner_loop", loopReq.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])
			require.Equal(t, true, loopReq.Metadata[agentPlannerActionReplanCycleMetadataAppendActionToolCallsBeforeEmptyMessage])
		case runtime.AgentLoopModePlannerActionExecution:
			require.Equal(t, "session-loop-outer", loopReq.Turn.SessionID)
			require.Equal(t, daemonTurnExecutionPlannerActionExecution, loopReq.Turn.Origin)
			require.Equal(t, "outer", stringFromAny(loopReq.Action.Arguments["probe"]))
		default:
			t.Fatalf("unexpected loop mode %s", loopReq.Mode)
		}
		return baseRunner.RunAgentLoop(ctx, loopReq)
	}))

	planMessage := agentMessage{ID: "msg-loop-outer-plan", Role: "agent", Content: "plan outer", Phase: "plan"}
	actionMessage := agentMessage{
		ID:      "msg-loop-outer-action",
		Role:    "tool",
		Content: "outer action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-loop-outer-action",
			Name: "daemon.health",
		}},
	}
	synthesisMessage := agentMessage{ID: "msg-loop-outer-synthesis", Role: "agent", Content: "outer done", Phase: "step_replan"}
	initialPlan := &agentPlannerResponse{
		Phase: "plan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "outer"},
		}},
	}
	finalPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "outer done"}

	actionCalled := false
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		actionCalled = true
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "outer", stringFromAny(payload.Action.Arguments["probe"]))

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "outer action handled",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(actionMessage.ToolCalls),
			},
		}, nil
	}))

	stageCalls := []runtime.LLMStage{}
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		stageCalls = append(stageCalls, req.Stage)
		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, "session-loop-outer", payload.Turn.SessionID)
		require.Equal(t, "gpt-loop-outer", payload.LLMRequest.Model)

		switch req.Stage {
		case runtime.LLMStagePlannerChat:
			require.Equal(t, []string{"history-loop-outer"}, agentLoopMessageIDs(payload.History))
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "outer planner handled",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{planMessage}, nil, initialPlan),
			}, nil
		case runtime.LLMStageToolSynthesis:
			require.Equal(t, []string{"history-loop-outer", planMessage.ID, actionMessage.ID}, agentLoopMessageIDs(payload.History))
			require.Equal(t, []string{actionMessage.ID}, agentLoopMessageIDs(payload.ToolMessages))
			return runtime.LLMStageResult{
				Status:  runtime.TurnResultFinished,
				Reason:  "outer synthesis handled",
				Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, nil, finalPlan),
			}, nil
		default:
			t.Fatalf("unexpected stage %s", req.Stage)
			return runtime.LLMStageResult{}, nil
		}
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	loopResult, err := eng.RunAgentLoop(context.Background(), runtime.AgentLoopRequest{
		Mode: runtime.AgentLoopModeLLMToolLoop,
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-loop-outer",
			agentRuntimeTurnRef{TaskID: "task-loop-outer", RunID: "run-loop-outer", TurnID: "turn-loop-outer"},
			"run daemon health through outer runner",
			"user",
			"read_only",
		),
		Language:   "en",
		Operator:   "tester",
		LLMRequest: runtime.AgentLoopLLMRequest{Model: "gpt-loop-outer"},
		History: []runtime.AgentLoopMessage{{
			ID:      "history-loop-outer",
			Role:    "user",
			Content: "run daemon health through outer runner",
		}},
		MaxLoops: 2,
	})

	require.NoError(t, err)
	require.True(t, actionCalled)
	require.Equal(t, []runtime.AgentLoopMode{runtime.AgentLoopModeLLMToolLoop, runtime.AgentLoopModePlannerActionReplanCycle, runtime.AgentLoopModePlannerActionExecution}, loopModes)
	require.Equal(t, []runtime.LLMStage{runtime.LLMStagePlannerChat, runtime.LLMStageToolSynthesis}, stageCalls)
	require.Equal(t, runtime.AgentLoopStopReasonNoActions, loopResult.StopReason)
	require.Equal(t, 1, loopResult.LoopCount)
	require.Equal(t, 2, loopResult.Metadata[runtime.AgentLoopEgressMetadataMaxLoops])
	require.Equal(t, string(runtime.AgentLoopModeLLMToolLoop), loopResult.Metadata["mode"])
	require.Equal(t, []string{planMessage.ID, actionMessage.ID, synthesisMessage.ID}, agentLoopMessageIDs(loopResult.Messages))
	require.Equal(t, []string{"history-loop-outer", planMessage.ID, actionMessage.ID, synthesisMessage.ID}, agentLoopMessageIDs(loopResult.State.LoopHistory))
	require.Equal(t, actionMessage.ToolCalls[0].ID, loopResult.ToolCalls[0].ID)
	require.Equal(t, finalPlan.Answer, loopResult.FinalPlan.Answer)
	require.Len(t, appendMutations, 3)
	require.Equal(t, string(runtime.LLMStagePlannerChat), appendMutations[0].Turn.Origin)
	require.Equal(t, "planner_loop", appendMutations[1].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[2].Turn.Origin)
}

func TestDaemonAgentLoopRunnerExecutesReplanContinuationOuterLoopFromRuntimeRequest(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	baseRunner := daemonAgentLoopRunner{manager: mgr}
	loopModes := []runtime.AgentLoopMode{}
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(ctx context.Context, loopReq runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		loopModes = append(loopModes, loopReq.Mode)
		switch loopReq.Mode {
		case runtime.AgentLoopModeReplanContinuation:
			require.Empty(t, stringFromAny(loopReq.Metadata[agentPlannerActionReplanCycleMetadataParentMode]))
		case runtime.AgentLoopModePlannerActionReplanCycle:
			require.Equal(t, string(runtime.AgentLoopModeReplanContinuation), loopReq.Metadata[agentPlannerActionReplanCycleMetadataParentMode])
			require.Equal(t, daemonTurnExecutionReplanActionContinuation, loopReq.Metadata[agentPlannerActionReplanCycleMetadataActionAppendOrigin])
		case runtime.AgentLoopModePlannerActionExecution:
			require.Equal(t, "session-loop-continuation", loopReq.Turn.SessionID)
			require.Equal(t, daemonTurnExecutionPlannerActionExecution, loopReq.Turn.Origin)
			require.Equal(t, "continuation", stringFromAny(loopReq.Action.Arguments["probe"]))
		default:
			t.Fatalf("unexpected loop mode %s", loopReq.Mode)
		}
		return baseRunner.RunAgentLoop(ctx, loopReq)
	}))

	actionMessage := agentMessage{
		ID:      "msg-loop-continuation-action",
		Role:    "tool",
		Content: "continuation action executed",
		ToolCalls: []agentToolCall{{
			ID:   "tool-loop-continuation-action",
			Name: "daemon.health",
		}},
	}
	synthesisMessage := agentMessage{ID: "msg-loop-continuation-synthesis", Role: "agent", Content: "continuation done", Phase: "step_replan"}
	initialPlan := &agentPlannerResponse{
		Phase: "step_replan",
		NextAction: &agentNextAction{
			Type:         "continue",
			AutoContinue: true,
		},
		Actions: []agentPlannerAction{{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "continuation"},
		}},
	}
	finalPlan := &agentPlannerResponse{Phase: "final_answer", Answer: "continuation done"}

	actionCalled := false
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		actionCalled = true
		require.Equal(t, daemonTurnExecutionPlannerActionExecution, turn.Origin)
		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		require.Equal(t, "continuation", stringFromAny(payload.Action.Arguments["probe"]))

		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Reason: "continuation action handled",
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages:  agentLoopMessagesFromAgentMessages([]agentMessage{actionMessage}),
				ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(actionMessage.ToolCalls),
			},
		}, nil
	}))

	stageCalls := []runtime.LLMStage{}
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		stageCalls = append(stageCalls, req.Stage)
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		payload, ok := req.Payload.(runtime.AgentLoopLLMStagePayload)
		require.True(t, ok, "payload type = %T", req.Payload)
		require.Equal(t, []string{"history-loop-continuation", actionMessage.ID}, agentLoopMessageIDs(payload.History))
		require.Equal(t, []string{actionMessage.ID}, agentLoopMessageIDs(payload.ToolMessages))

		return runtime.LLMStageResult{
			Status:  runtime.TurnResultFinished,
			Reason:  "continuation synthesis handled",
			Payload: agentLoopLLMStageResultPayloadFromLegacy([]agentMessage{synthesisMessage}, nil, finalPlan),
		}, nil
	}))

	appendMutations := []runtime.SessionMutationRequest{}
	attachRecordingSessionMutator(t, eng, mgr, &appendMutations)

	loopResult, err := eng.RunAgentLoop(context.Background(), runtime.AgentLoopRequest{
		Mode: runtime.AgentLoopModeReplanContinuation,
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-loop-continuation",
			agentRuntimeTurnRef{TaskID: "task-loop-continuation", RunID: "run-loop-continuation", TurnID: "turn-loop-continuation"},
			"continue daemon health through outer runner",
			"user",
			"read_only",
		),
		Language:    "en",
		Operator:    "tester",
		LLMRequest:  runtime.AgentLoopLLMRequest{Model: "gpt-loop-continuation"},
		InitialPlan: agentLoopPlanFromPlannerResponse(initialPlan),
		State: runtime.AgentLoopState{
			LoopHistory: []runtime.AgentLoopMessage{{
				ID:      "history-loop-continuation",
				Role:    "agent",
				Content: "previous step",
			}},
		},
		MaxLoops: 2,
	})

	require.NoError(t, err)
	require.True(t, actionCalled)
	require.Equal(t, []runtime.AgentLoopMode{runtime.AgentLoopModeReplanContinuation, runtime.AgentLoopModePlannerActionReplanCycle, runtime.AgentLoopModePlannerActionExecution}, loopModes)
	require.Equal(t, []runtime.LLMStage{runtime.LLMStageToolSynthesis}, stageCalls)
	require.Equal(t, runtime.AgentLoopStopReasonNoActions, loopResult.StopReason)
	require.Equal(t, 1, loopResult.LoopCount)
	require.Equal(t, 2, loopResult.Metadata[runtime.AgentLoopEgressMetadataMaxLoops])
	require.Equal(t, string(runtime.AgentLoopModeReplanContinuation), loopResult.Metadata["mode"])
	require.Equal(t, []string{actionMessage.ID, synthesisMessage.ID}, agentLoopMessageIDs(loopResult.Messages))
	require.Equal(t, []string{"history-loop-continuation", actionMessage.ID, synthesisMessage.ID}, agentLoopMessageIDs(loopResult.State.LoopHistory))
	require.Equal(t, actionMessage.ToolCalls[0].ID, loopResult.ToolCalls[0].ID)
	require.Equal(t, finalPlan.Answer, loopResult.FinalPlan.Answer)
	require.Len(t, appendMutations, 2)
	require.Equal(t, daemonTurnExecutionReplanActionContinuation, appendMutations[0].Turn.Origin)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), appendMutations[1].Turn.Origin)
}

func TestDaemonLLMTurnRunnerDispatchesThroughRuntimeAgentLoopRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	loopMessage := runtime.AgentLoopMessage{ID: "msg-llm-turn-loop-runner", Role: "agent", Content: "llm loop done"}
	called := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, loopReq runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		called = true
		require.Equal(t, runtime.AgentLoopModeLLMToolLoop, loopReq.Mode)
		require.Equal(t, "session-llm-turn-loop-runner", loopReq.Turn.SessionID)
		return runtime.AgentLoopEgressResult{
			State: runtime.AgentLoopState{
				ResponseMessages: []runtime.AgentLoopMessage{loopMessage},
				LoopHistory:      []runtime.AgentLoopMessage{loopMessage},
			},
			StopReason: "llm_turn_runner_stop",
			LoopCount:  2,
			MaxLoops:   5,
		}.AgentLoopResult(), nil
	}))

	result, err := daemonAgentLLMTurnRunner{manager: mgr}.RunLLMTurn(context.Background(), runtime.LLMTurnRequest{
		Turn: runtime.TurnContext{
			SessionID: "session-llm-turn-loop-runner",
			TaskID:    "task-llm-turn-loop-runner",
			RunID:     "run-llm-turn-loop-runner",
			TurnID:    "turn-llm-turn-loop-runner",
			Prompt:    "run loop",
		},
		Payload: runtime.AgentLoopRequest{
			Mode: runtime.AgentLoopModeLLMToolLoop,
			Turn: runtime.TurnContext{
				SessionID: "session-llm-turn-loop-runner",
				TaskID:    "task-llm-turn-loop-runner",
				RunID:     "run-llm-turn-loop-runner",
				TurnID:    "turn-llm-turn-loop-runner",
				Prompt:    "run loop",
			},
			MaxLoops: 5,
		},
	})

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, runtime.TurnResultFinished, result.Status)
	require.Equal(t, "llm_turn_runner_stop", result.Data["stop_reason"])
	require.Equal(t, 2, result.Data["loop_count"])
	require.Equal(t, 5, result.Data["max_loops"])
	payload, ok := result.Payload.(runtime.AgentLoopTurnResultPayload)
	require.True(t, ok, "payload type = %T", result.Payload)
	require.Equal(t, "llm_turn_runner_stop", payload.StopReason)
	require.Equal(t, 2, payload.LoopCount)
	require.Equal(t, 5, payload.MaxLoops)
	require.Equal(t, []string{loopMessage.ID}, agentLoopMessageIDs(payload.Messages))
}

func TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	loopMessage := runtime.AgentLoopMessage{ID: "msg-turn-executor-loop-runner", Role: "agent", Content: "continuation loop done"}
	called := false
	eng.AttachAgentLoopRunner(runtime.AgentLoopRunnerFunc(func(_ context.Context, loopReq runtime.AgentLoopRequest) (runtime.AgentLoopResult, error) {
		called = true
		require.Equal(t, runtime.AgentLoopModeReplanContinuation, loopReq.Mode)
		require.Equal(t, "session-turn-executor-loop-runner", loopReq.Turn.SessionID)
		return runtime.AgentLoopEgressResult{
			State: runtime.AgentLoopState{
				ResponseMessages: []runtime.AgentLoopMessage{loopMessage},
				LoopHistory:      []runtime.AgentLoopMessage{loopMessage},
			},
			StopReason: "turn_executor_loop_stop",
			LoopCount:  1,
			MaxLoops:   3,
		}.AgentLoopResult(), nil
	}))

	loopReq := runtime.AgentLoopRequest{
		Mode: runtime.AgentLoopModeReplanContinuation,
		Turn: agentPlannerActionReplanCycleTurnContext(
			"session-turn-executor-loop-runner",
			agentRuntimeTurnRef{TaskID: "task-turn-executor-loop-runner", RunID: "run-turn-executor-loop-runner", TurnID: "turn-turn-executor-loop-runner"},
			"continue loop",
			"user",
			"read_only",
		),
		MaxLoops: 3,
	}
	result, err := daemonAgentTurnExecutor{manager: mgr}.ExecuteTurn(context.Background(), agentReplanActionContinuationTurnContextFromRequest(loopReq))

	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, runtime.TurnResultFinished, result.Status)
	require.Equal(t, "turn_executor_loop_stop", result.Data["stop_reason"])
	require.Equal(t, 1, result.Data["loop_count"])
	require.Equal(t, 3, result.Data["max_loops"])
	payload, ok := result.Payload.(runtime.AgentLoopTurnResultPayload)
	require.True(t, ok, "payload type = %T", result.Payload)
	require.Equal(t, "turn_executor_loop_stop", payload.StopReason)
	require.Equal(t, 1, payload.LoopCount)
	require.Equal(t, 3, payload.MaxLoops)
	require.Equal(t, []string{loopMessage.ID}, agentLoopMessageIDs(payload.Messages))
}

func agentToolCallIDs(calls []agentToolCall) []string {
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		ids = append(ids, call.ID)
	}
	return ids
}
