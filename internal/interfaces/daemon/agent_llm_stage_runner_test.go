package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerRunRuntimeAgentLLMStageFallsBackWithoutRunner(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	expectedMessage := agentMessage{ID: "msg-fallback", Role: "agent", Content: "fallback response"}
	expectedToolCall := agentToolCall{ID: "tool-fallback", Name: "LLM chat"}
	expectedPlan := &agentPlannerResponse{Phase: "plan", Answer: "fallback response"}

	payload, err := mgr.runRuntimeAgentLLMStage(context.Background(), runtime.LLMStagePlannerChat, agentLLMStageDispatchInput{
		SessionID:      "session-stage-fallback",
		TurnRef:        agentRuntimeTurnRef{TaskID: "task-stage-fallback", RunID: "run-stage-fallback", TurnID: "turn-stage-fallback"},
		Prompt:         "make a fallback plan",
		Language:       "en",
		OperationLevel: "read_only",
		Request:        sendAgentMessageRequest{Model: "gpt-fallback"},
	}, func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
		return []agentMessage{expectedMessage}, []agentToolCall{expectedToolCall}, expectedPlan, nil
	})

	require.NoError(t, err)
	require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), payload.Messages)
	require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), payload.ToolCalls)
	require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), payload.Plan)
}

func TestDaemonLLMStageCallerFallbackOwnershipTableCoversReasons(t *testing.T) {
	wantReasons := map[daemonLLMStageCallerFallbackReason]string{
		daemonLLMStageCallerFallbackEngineNotAttached: daemonLLMStageCallerFallbackOwnerLegacyCaller,
	}
	seenReasons := map[daemonLLMStageCallerFallbackReason]string{}

	for _, entry := range daemonLLMStageCallerFallbackOwnershipTable {
		require.NotEmpty(t, entry.reason)
		require.NotEmpty(t, entry.owner)
		require.Empty(t, seenReasons[entry.reason], "duplicate llm stage fallback ownership entry for %q", entry.reason)
		seenReasons[entry.reason] = entry.owner

		owner, ok := daemonLLMStageCallerFallbackOwnerForReason(entry.reason)
		require.True(t, ok)
		require.Equal(t, entry.owner, owner)
		require.True(t, daemonLLMStageCallerFallbackOwnedByLegacyCaller(entry.reason))
	}

	require.Equal(t, wantReasons, seenReasons)
	_, ok := daemonLLMStageCallerFallbackOwnerForReason(daemonLLMStageCallerFallbackReason("unsupported"))
	require.False(t, ok)
	require.False(t, daemonLLMStageCallerFallbackOwnedByLegacyCaller(daemonLLMStageCallerFallbackReason("unsupported")))
	require.False(t, daemonLLMStageCallerFallbackOwnedByLegacyCaller(daemonLLMStageCallerFallbackRunnerNotAttached))
	require.ErrorContains(t, daemonLLMStageCallerFallbackOwnershipError(daemonLLMStageCallerFallbackReason("unsupported")), "fallback ownership missing")

	reason, fallback := daemonLLMStageCallerPreDispatchFallbackReason(nil)
	require.True(t, fallback)
	require.Equal(t, daemonLLMStageCallerFallbackEngineNotAttached, reason)

	eng := runtime.NewEngine(runtime.NewMemStore())
	reason, fallback = daemonLLMStageCallerPreDispatchFallbackReason(eng)
	require.True(t, fallback)
	require.Equal(t, daemonLLMStageCallerFallbackRunnerNotAttached, reason)

	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(context.Context, runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		return runtime.LLMStageResult{}, nil
	}))
	_, fallback = daemonLLMStageCallerPreDispatchFallbackReason(eng)
	require.False(t, fallback)

	reason, fallback = daemonLLMStageCallerResultFallbackReason(runtime.LLMStageResult{}, runtime.ErrLLMStageRunnerNotAttached)
	require.True(t, fallback)
	require.Equal(t, daemonLLMStageCallerFallbackRunnerNotAttached, reason)

	reason, fallback = daemonLLMStageCallerResultFallbackReason(runtime.LLMStageResult{Error: runtime.ErrLLMStageRunnerNotAttached}, nil)
	require.True(t, fallback)
	require.Equal(t, daemonLLMStageCallerFallbackRunnerNotAttached, reason)

	blocked, ok := daemonLLMStageCallerRunnerNotAttachedBlockedResult(runtime.LLMStageToolSynthesis, runtime.TurnContext{
		SessionID: "session-blocked",
		TaskID:    "task-blocked",
		RunID:     "run-blocked",
		TurnID:    "turn-blocked",
	}, runtime.LLMStageResult{}, runtime.ErrLLMStageRunnerNotAttached)
	require.True(t, ok)
	require.Equal(t, runtime.TurnResultBlocked, blocked.Status)
	require.ErrorIs(t, blocked.Error, runtime.ErrLLMStageRunnerNotAttached)
	require.Equal(t, runtime.ErrLLMStageRunnerNotAttached.Error(), blocked.Reason)
	require.Equal(t, string(runtime.LLMStageToolSynthesis), blocked.Data["stage"])
	require.Equal(t, runtime.ErrLLMStageRunnerNotAttached.Error(), blocked.Data["error"])
	require.Equal(t, string(daemonLLMStageCallerFallbackRunnerNotAttached), blocked.Data["blocked_reason"])
	require.Equal(t, "session-blocked", blocked.Data["session_id"])

	_, ok = daemonLLMStageCallerRunnerNotAttachedBlockedResult(runtime.LLMStagePlannerChat, runtime.TurnContext{}, runtime.LLMStageResult{}, errors.New("stage hard error"))
	require.False(t, ok)

	_, fallback = daemonLLMStageCallerResultFallbackReason(runtime.LLMStageResult{Status: runtime.TurnResultBlocked, Reason: "blocked by runner"}, nil)
	require.False(t, fallback)
}

func TestDaemonLLMStageCallerFallbackBoundaryCoversStages(t *testing.T) {
	want := map[runtime.LLMStage]daemonLLMStageCallerFallbackBoundaryEntry{
		runtime.LLMStagePlannerChat: {
			stage:          runtime.LLMStagePlannerChat,
			callerSurface:  "runRuntimeAgentLLMPlannerChat -> runRuntimeAgentLLMStage",
			legacyFallback: "handleAgentLLMChat",
			owner:          daemonLLMStageCallerFallbackOwnerLegacyCaller,
			scope:          "planner_chat compatibility fallback only when runtime engine is not attached",
		},
		runtime.LLMStageToolSynthesis: {
			stage:          runtime.LLMStageToolSynthesis,
			callerSurface:  "runRuntimeAgentLLMToolSynthesis -> runRuntimeAgentLLMStage",
			legacyFallback: "synthesizeAgentToolResults",
			owner:          daemonLLMStageCallerFallbackOwnerLegacyCaller,
			scope:          "tool_synthesis compatibility fallback for planner_action_replan_cycle and LLM follow-up callers only when runtime engine is not attached",
		},
	}
	seen := map[runtime.LLMStage]daemonLLMStageCallerFallbackBoundaryEntry{}

	for _, entry := range daemonLLMStageCallerFallbackBoundaryTable {
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.callerSurface)
		require.NotEmpty(t, entry.legacyFallback)
		require.Equal(t, daemonLLMStageCallerFallbackOwnerLegacyCaller, entry.owner)
		require.NotEmpty(t, entry.scope)
		require.Empty(t, seen[entry.stage].stage, "duplicate llm stage fallback boundary entry for %q", entry.stage)
		seen[entry.stage] = entry

		got, ok := daemonLLMStageCallerFallbackBoundaryForStage(entry.stage)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}

	require.Equal(t, want, seen)
	toolSynthesis, ok := daemonLLMStageCallerFallbackBoundaryForStage(runtime.LLMStageToolSynthesis)
	require.True(t, ok)
	require.Equal(t, "synthesizeAgentToolResults", toolSynthesis.legacyFallback)
	require.Contains(t, toolSynthesis.scope, "planner_action_replan_cycle")

	_, ok = daemonLLMStageCallerFallbackBoundaryForStage(runtime.LLMStage("unsupported"))
	require.False(t, ok)
}

func TestDaemonLLMStageRunnerNotAttachedRetirementAuditNarrowsFallback(t *testing.T) {
	wantStages := map[string]daemonLLMStageRunnerNotAttachedRetirementStage{
		"no_engine_llm_stage_legacy_fallback":      daemonLLMStageRunnerNotAttachedStageNoEngineFallbackRetained,
		"attached_engine_missing_llm_stage_runner": daemonLLMStageRunnerNotAttachedStageMissingRunnerBlocked,
		"detached_llm_stage_runner_result":         daemonLLMStageRunnerNotAttachedStageDetachedResultBlocked,
		"llm_stage_next_fallback_boundary":         daemonLLMStageRunnerNotAttachedStageNextBoundaryChosen,
	}
	seenStages := map[string]daemonLLMStageRunnerNotAttachedRetirementStage{}
	for _, entry := range daemonLLMStageRunnerNotAttachedRetirementTable {
		require.NotEmpty(t, entry.component)
		require.NotEmpty(t, entry.stage)
		require.NotEmpty(t, entry.disposition)
		require.NotEmpty(t, entry.evidence)
		require.Equal(t, daemonLLMStageNextFallbackRetirementBoundaryLLMTurnRunnerCompatibility, entry.nextBoundary)
		require.Empty(t, seenStages[entry.component], "duplicate llm stage retirement entry for %q", entry.component)
		seenStages[entry.component] = entry.stage

		got, ok := daemonLLMStageRunnerNotAttachedRetirementEntryForComponent(entry.component)
		require.True(t, ok)
		require.Equal(t, entry, got)
	}
	require.Equal(t, wantStages, seenStages)

	_, ok := daemonLLMStageRunnerNotAttachedRetirementEntryForComponent("unsupported")
	require.False(t, ok)
	require.Empty(t, daemonLLMStageRunnerNotAttachedRetirementEntriesForStage(daemonLLMStageRunnerNotAttachedRetirementStage("unsupported")))
	require.Len(t, daemonLLMStageRunnerNotAttachedRetirementEntriesForStage(daemonLLMStageRunnerNotAttachedStageNoEngineFallbackRetained), 1)
	require.Len(t, daemonLLMStageRunnerNotAttachedRetirementEntriesForStage(daemonLLMStageRunnerNotAttachedStageMissingRunnerBlocked), 1)
	require.Len(t, daemonLLMStageRunnerNotAttachedRetirementEntriesForStage(daemonLLMStageRunnerNotAttachedStageDetachedResultBlocked), 1)
	require.Len(t, daemonLLMStageRunnerNotAttachedRetirementEntriesForStage(daemonLLMStageRunnerNotAttachedStageNextBoundaryChosen), 1)

	selectedBoundary, ok := daemonLLMStageRunnerNotAttachedRetirementSelectedFallbackBoundary()
	require.True(t, ok)
	require.Equal(t, daemonLLMStageNextFallbackRetirementBoundaryLLMTurnRunnerCompatibility, selectedBoundary)
	require.True(t, daemonLLMStageRunnerNotAttachedRetirementAllowsLegacyFallback(daemonLLMStageCallerFallbackEngineNotAttached))
	require.False(t, daemonLLMStageRunnerNotAttachedRetirementAllowsLegacyFallback(daemonLLMStageCallerFallbackRunnerNotAttached))

	missing, ok := daemonLLMStageRunnerNotAttachedRetirementEntryForComponent("attached_engine_missing_llm_stage_runner")
	require.True(t, ok)
	require.Equal(t, daemonLLMStageRunnerNotAttachedDispositionBlockedError, missing.disposition)
	require.Contains(t, missing.evidence, "runtime.ErrLLMStageRunnerNotAttached")

	detached, ok := daemonLLMStageRunnerNotAttachedRetirementEntryForComponent("detached_llm_stage_runner_result")
	require.True(t, ok)
	require.Equal(t, daemonLLMStageRunnerNotAttachedDispositionBlockedError, detached.disposition)

	next, ok := daemonLLMStageRunnerNotAttachedRetirementEntryForComponent("llm_stage_next_fallback_boundary")
	require.True(t, ok)
	require.Contains(t, next.nextStep, "P0.5-N218")
}

func TestAgentManagerRunRuntimeAgentLLMStageToolSynthesisFallbackBoundary(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*agentManager)
	}{
		{
			name: "missing_engine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newAgentManager(t.TempDir())
			if tt.setup != nil {
				tt.setup(mgr)
			}
			expectedMessage := agentMessage{ID: "msg-tool-synthesis-fallback", Role: "agent", Content: "fallback synthesis"}
			expectedToolCall := agentToolCall{ID: "tool-tool-synthesis-fallback", Name: "LLM synthesis"}
			expectedPlan := &agentPlannerResponse{Phase: "step_replan", Answer: "fallback synthesis"}
			fallbackCalled := false

			payload, err := mgr.runRuntimeAgentLLMStage(context.Background(), runtime.LLMStageToolSynthesis, agentLLMStageDispatchInput{
				SessionID:      "session-tool-synthesis-fallback",
				TurnRef:        agentRuntimeTurnRef{TaskID: "task-tool-synthesis-fallback", RunID: "run-tool-synthesis-fallback", TurnID: "turn-tool-synthesis-fallback"},
				Prompt:         "summarize executed tools",
				Language:       "en",
				OperationLevel: "read_only",
				Request:        sendAgentMessageRequest{Model: "gpt-tool-synthesis-fallback"},
				ToolMessages:   []runtime.AgentLoopMessage{{ID: "msg-tool-observation", Role: "tool", Content: "tool observation"}},
			}, func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
				fallbackCalled = true
				return []agentMessage{expectedMessage}, []agentToolCall{expectedToolCall}, expectedPlan, nil
			})

			require.NoError(t, err)
			require.True(t, fallbackCalled)
			require.Equal(t, agentLoopMessagesFromAgentMessages([]agentMessage{expectedMessage}), payload.Messages)
			require.Equal(t, agentLoopToolCallItemsFromAgentToolCalls([]agentToolCall{expectedToolCall}), payload.ToolCalls)
			require.Equal(t, agentLoopPlanPtrFromAgentPlannerResponse(expectedPlan), payload.Plan)
		})
	}
}

func TestAgentManagerRunRuntimeAgentLLMStageRunnerNotAttachedBlocksWithoutLegacyFallback(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*agentManager)
	}{
		{
			name: "missing_runner",
			setup: func(mgr *agentManager) {
				mgr.engine = runtime.NewEngine(runtime.NewMemStore())
			},
		},
		{
			name: "runner_not_attached_error",
			setup: func(mgr *agentManager) {
				eng := runtime.NewEngine(runtime.NewMemStore())
				eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(context.Context, runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
					return runtime.LLMStageResult{
						Status: runtime.TurnResultBlocked,
						Reason: "runner_not_attached_result",
						Error:  runtime.ErrLLMStageRunnerNotAttached,
						Data:   map[string]any{"runner": "detached-test"},
					}, runtime.ErrLLMStageRunnerNotAttached
				}))
				mgr.engine = eng
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newAgentManager(t.TempDir())
			tt.setup(mgr)
			fallbackCalled := false

			payload, err := mgr.runRuntimeAgentLLMStage(context.Background(), runtime.LLMStageToolSynthesis, agentLLMStageDispatchInput{
				SessionID:      "session-tool-synthesis-blocked",
				TurnRef:        agentRuntimeTurnRef{TaskID: "task-tool-synthesis-blocked", RunID: "run-tool-synthesis-blocked", TurnID: "turn-tool-synthesis-blocked"},
				Prompt:         "summarize executed tools",
				Language:       "en",
				OperationLevel: "read_only",
				Request:        sendAgentMessageRequest{Model: "gpt-tool-synthesis-blocked"},
				ToolMessages:   []runtime.AgentLoopMessage{{ID: "msg-tool-observation", Role: "tool", Content: "tool observation"}},
			}, func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
				fallbackCalled = true
				return []agentMessage{{ID: "msg-should-not-fallback", Role: "agent", Content: "unexpected fallback"}}, nil, nil, nil
			})

			require.ErrorIs(t, err, runtime.ErrLLMStageRunnerNotAttached)
			require.False(t, fallbackCalled)
			require.Empty(t, payload.Messages)
			require.Empty(t, payload.ToolCalls)
			require.Nil(t, payload.Plan)
		})
	}
}

func TestAgentManagerRunRuntimeAgentLLMStageInvalidPayloadDoesNotFallback(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)
	eng.AttachLLMStageRunner(runtime.LLMStageRunnerFunc(func(_ context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
		require.Equal(t, runtime.LLMStageToolSynthesis, req.Stage)
		return runtime.LLMStageResult{Status: runtime.TurnResultFinished, Reason: "missing payload"}, nil
	}))

	payload, err := mgr.runRuntimeAgentLLMStage(context.Background(), runtime.LLMStageToolSynthesis, agentLLMStageDispatchInput{
		SessionID:      "session-stage-invalid",
		TurnRef:        agentRuntimeTurnRef{TaskID: "task-stage-invalid", RunID: "run-stage-invalid", TurnID: "turn-stage-invalid"},
		Prompt:         "summarize",
		Language:       "en",
		OperationLevel: "read_only",
		Request:        sendAgentMessageRequest{Model: "gpt-invalid"},
	}, func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
		t.Fatalf("fallback should not run for non-error invalid payload")
		return nil, nil, nil, nil
	})

	require.ErrorContains(t, err, "llm tool_synthesis runner returned invalid payload")
	require.Empty(t, payload.Messages)
	require.Empty(t, payload.ToolCalls)
	require.Nil(t, payload.Plan)
}

func TestAgentLLMStagePayloadFromDispatchInputClonesMutableFields(t *testing.T) {
	lastMessageAt := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	input := agentLLMStageDispatchInput{
		SessionID:      "session-stage-clone",
		TurnRef:        agentRuntimeTurnRef{TaskID: "task-stage-clone", RunID: "run-stage-clone", TurnID: "turn-stage-clone"},
		Prompt:         "clone stage payload",
		Language:       "zh-CN",
		OperationLevel: "safe_scan",
		Request: sendAgentMessageRequest{
			Model: "gpt-clone",
			AuthProfiles: []agentHTTPAuthProfile{{
				ID:    "auth-1",
				Token: "secret-token",
			}},
		},
		History:       []agentMessage{{ID: "history-1", Role: "user", Content: "hello"}},
		MemorySummary: &agentMemorySummary{Summary: "memory", LastMessageAt: &lastMessageAt},
		ToolMessages:  []runtime.AgentLoopMessage{{ID: "tool-message-1", Role: "tool", Content: "observation"}},
	}
	turn := agentLLMStageTurnContext(runtime.LLMStageToolSynthesis, input)

	payload := agentLLMStagePayloadFromDispatchInput(turn, input)

	require.Equal(t, "session-stage-clone", payload.Turn.SessionID)
	require.Equal(t, "zh-CN", payload.Language)
	require.Equal(t, "gpt-clone", payload.LLMRequest.Model)
	require.Len(t, payload.History, 1)
	require.Len(t, payload.ToolMessages, 1)
	require.NotNil(t, payload.MemorySummary)
	require.Equal(t, "memory", payload.MemorySummary.Summary)

	payload.History[0].Content = "mutated history"
	payload.ToolMessages[0].Content = "mutated tool message"
	payload.MemorySummary.Summary = "mutated memory"
	payload.LLMRequest.AuthProfiles[0].Token = "mutated-token"

	require.Equal(t, "hello", input.History[0].Content)
	require.Equal(t, "observation", input.ToolMessages[0].Content)
	require.Equal(t, "memory", input.MemorySummary.Summary)
	require.Equal(t, "secret-token", input.Request.AuthProfiles[0].Token)
}
