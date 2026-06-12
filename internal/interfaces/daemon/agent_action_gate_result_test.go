package daemon

import (
	"context"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/stretchr/testify/require"
)

func TestAgentManagerExecutePlannerActionBatchGateResultParity(t *testing.T) {
	cases := []struct {
		name             string
		prompt           string
		language         string
		operationLevel   string
		action           runtime.AgentLoopAction
		wantKind         runtime.AgentLoopActionGateKind
		wantMutationKind runtime.SessionMutationKind
		wantPolicy       string
		wantRequired     string
		wantMessagePhase string
		wantToolCalls    int
		wantSessionState string
	}{
		{
			name:             "capability mismatch",
			prompt:           "run ffuf directory enumeration on https://example.com",
			language:         "en",
			operationLevel:   "safe_scan",
			action:           runtime.AgentLoopAction{Name: "scan.web.safe", Arguments: map[string]any{"target": "https://example.com"}},
			wantKind:         runtime.AgentLoopActionGateCapabilityBlocked,
			wantMutationKind: runtime.SessionMutationCapabilityBlocked,
			wantRequired:     "safe_scan",
			wantMessagePhase: "step_replan",
			wantToolCalls:    1,
			wantSessionState: "blocked",
		},
		{
			name:             "explicit operator intent required",
			prompt:           "summarize the project",
			language:         "en",
			operationLevel:   "workspace_write",
			action:           runtime.AgentLoopAction{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.md", "content": "hello"}},
			wantKind:         runtime.AgentLoopActionGateExplicitIntentRequired,
			wantMutationKind: runtime.SessionMutationRequestAuthorization,
			wantPolicy:       "explicit_operator_intent_required",
			wantRequired:     "workspace_write",
			wantMessagePhase: "step",
			wantSessionState: "waiting_authorization",
		},
		{
			name:             "operation level required",
			prompt:           "scan https://example.com",
			language:         "en",
			operationLevel:   "read_only",
			action:           runtime.AgentLoopAction{Name: "scan.web.safe", Arguments: map[string]any{"target": "https://example.com"}},
			wantKind:         runtime.AgentLoopActionGateOperationLevelRequired,
			wantMutationKind: runtime.SessionMutationRequestAuthorization,
			wantPolicy:       "operation_level_required",
			wantRequired:     "safe_scan",
			wantMessagePhase: "step",
			wantSessionState: "waiting_authorization",
		},
		{
			name:             "manual authorization required",
			prompt:           "write file notes.md content: hello",
			language:         "en",
			operationLevel:   "workspace_write",
			action:           runtime.AgentLoopAction{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.md", "content": "hello"}},
			wantKind:         runtime.AgentLoopActionGateManualAuthorizationRequired,
			wantMutationKind: runtime.SessionMutationRequestAuthorization,
			wantPolicy:       "manual_tool_authorization_required",
			wantRequired:     "workspace_write",
			wantMessagePhase: "step",
			wantSessionState: "waiting_authorization",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng := runtime.NewEngine(runtime.NewMemStore())
			mgr := newAgentManager(t.TempDir())
			mgr.AttachEngine(eng)

			created, err := mgr.CreateSession(createAgentSessionRequest{
				Operator:       "tester",
				Language:       tc.language,
				OperationLevel: tc.operationLevel,
			})
			require.NoError(t, err)
			sessionID := created.Session.SessionID
			turnRef := agentRuntimeTurnRef{
				TaskID: "task-gate-result",
				RunID:  "run-gate-result",
				TurnID: "turn-gate-result",
			}

			records := []runtime.SessionMutationRequest{}
			legacy := daemonSessionMutator{manager: mgr}
			eng.AttachSessionMutator(runtime.SessionMutatorFunc(func(ctx context.Context, req runtime.SessionMutationRequest) (runtime.SessionMutationResult, error) {
				if req.Kind == runtime.SessionMutationRequestAuthorization || req.Kind == runtime.SessionMutationCapabilityBlocked {
					records = append(records, req)
				}
				return legacy.MutateSession(ctx, req)
			}))

			decision := decideAgentPlannerActionGateFromAgentLoopAction(tc.prompt, tc.language, tc.operationLevel, tc.action)
			require.Equal(t, tc.wantKind, decision.Kind)
			require.Equal(t, tc.wantRequired, decision.RequiredLevel)

			result := mgr.executeAgentPlannerActionBatch(
				context.Background(),
				sessionID,
				turnRef,
				tc.prompt,
				tc.language,
				"tester",
				tc.operationLevel,
				runtime.AgentLoopToolServiceBinding{},
				[]runtime.AgentLoopAction{tc.action},
				runtime.AgentLoopAuthorization{},
			)

			require.True(t, result.AuthorizationRequired)
			require.Equal(t, 1, result.ActionCount)
			require.Equal(t, 1, result.ProcessedActionCount)
			require.NotNil(t, result.StopGate)
			require.True(t, result.StopGate.Handled)
			require.True(t, result.StopGate.AuthorizationRequired)
			require.Equal(t, decision, result.StopGate.Decision)
			require.Len(t, result.Messages, 1)
			require.Equal(t, tc.wantMessagePhase, result.Messages[0].Phase)
			require.Len(t, result.ToolCalls, tc.wantToolCalls)
			require.Len(t, records, 1)
			require.Equal(t, tc.wantMutationKind, records[0].Kind)
			require.Equal(t, sessionID, records[0].Turn.SessionID)
			require.Equal(t, turnRef.TaskID, records[0].Turn.TaskID)
			require.Equal(t, turnRef.RunID, records[0].Turn.RunID)
			require.Equal(t, turnRef.TurnID, records[0].Turn.TurnID)
			require.Equal(t, tc.prompt, records[0].Turn.Prompt)
			require.Equal(t, tc.operationLevel, records[0].Turn.OperationLevel)
			require.Equal(t, runtime.AgentLoopSessionMutationMutatorMetadataValue, records[0].Turn.Metadata[runtime.AgentLoopSessionMutationMutatorMetadataKey])
			require.Equal(t, string(tc.wantMutationKind), records[0].Turn.Metadata[runtime.AgentLoopSessionMutationKindMetadataKey])
			require.Equal(t, runtime.AgentLoopSessionMutationMutatorMetadataValue, records[0].Metadata[runtime.AgentLoopSessionMutationMutatorMetadataKey])
			require.Equal(t, string(tc.wantMutationKind), records[0].Metadata[runtime.AgentLoopSessionMutationKindMetadataKey])

			switch tc.wantMutationKind {
			case runtime.SessionMutationCapabilityBlocked:
				require.Equal(t, string(runtime.SessionMutationCapabilityBlocked), records[0].Turn.Origin)
				payload, ok := records[0].Payload.(runtime.AgentLoopCapabilityBlockedPayload)
				require.True(t, ok, "payload type = %T", records[0].Payload)
				require.Equal(t, tc.language, payload.Language)
				require.Equal(t, decision.ActionName, payload.ActionName)
				require.Equal(t, decision.IntentID, payload.IntentID)
				require.Equal(t, decision.Reason, payload.Reason)
				require.Equal(t, decision.SuggestedPrompt, payload.SuggestedPrompt)
				require.Equal(t, decision.ActionName, records[0].Metadata[runtime.AgentLoopActionNameMetadataKey])
				require.Equal(t, decision.IntentID, records[0].Metadata[runtime.AgentLoopSessionMutationIntentIDMetadataKey])
				require.Equal(t, strings.TrimSpace(decision.SuggestedPrompt), records[0].Metadata[runtime.AgentLoopSessionMutationSuggestedPromptMetadataKey])
				require.Equal(t, tc.operationLevel, records[0].Metadata[runtime.AgentLoopSessionMutationOperationLevelMetadataKey])
				require.Equal(t, decision.ActionName, records[0].Turn.Metadata[runtime.AgentLoopActionNameMetadataKey])
			case runtime.SessionMutationRequestAuthorization:
				require.Equal(t, string(runtime.SessionMutationRequestAuthorization), records[0].Turn.Origin)
				payload, ok := records[0].Payload.(runtime.AgentLoopAuthorizationRequestPayload)
				require.True(t, ok, "payload type = %T", records[0].Payload)
				require.Equal(t, tc.language, payload.Language)
				require.Equal(t, decision.ActionName, payload.ActionName)
				require.Equal(t, tc.wantRequired, payload.RequiredLevel)
				require.Equal(t, tc.wantPolicy, payload.Policy)
				require.Equal(t, tc.action, payload.Action)
				require.Equal(t, decision.ActionName, records[0].Metadata[runtime.AgentLoopActionNameMetadataKey])
				require.Equal(t, tc.wantRequired, records[0].Metadata[runtime.AgentLoopSessionMutationRequiredLevelMetadataKey])
				require.Equal(t, tc.wantPolicy, records[0].Metadata[runtime.AgentLoopSessionMutationPolicyMetadataKey])
				require.Equal(t, tc.operationLevel, records[0].Metadata[runtime.AgentLoopSessionMutationOperationLevelMetadataKey])
			default:
				t.Fatalf("unexpected mutation kind %s", tc.wantMutationKind)
			}

			got, err := mgr.GetSession(context.Background(), sessionID)
			require.NoError(t, err)
			require.Equal(t, tc.wantSessionState, got.Status)
			if tc.wantMutationKind == runtime.SessionMutationRequestAuthorization {
				require.NotNil(t, got.PendingAuthorization)
				require.Equal(t, decision.ActionName, got.PendingAuthorization.ActionName)
				require.Equal(t, tc.wantPolicy, got.PendingAuthorization.Policy)
				require.Equal(t, tc.wantRequired, got.PendingAuthorization.RequiredLevel)
			} else {
				require.Nil(t, got.PendingAuthorization)
				require.NotNil(t, got.NextAction)
				require.Equal(t, "blocked", got.NextAction.Type)
			}
		})
	}
}

func TestAgentManagerExecuteAgentPlannerActionListThreadsActionQueueMetadata(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	executed := 0
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, turn runtime.TurnContext) (runtime.TurnResult, error) {
		executed++
		payload, ok := turn.Payload.(runtime.AgentLoopActionExecutionPayload)
		require.True(t, ok, "payload type = %T", turn.Payload)
		probe, _ := payload.Action.Arguments["probe"].(string)
		return runtime.TurnResult{
			Status: runtime.TurnResultFinished,
			Payload: runtime.AgentLoopTurnResultPayload{
				Messages: []runtime.AgentLoopMessage{{
					ID:      "msg-" + probe,
					Role:    "tool",
					Content: "executed " + probe,
					ToolCalls: []runtime.AgentLoopToolCall{{
						ID:       "tool-message-" + probe,
						Name:     "daemon.health",
						Metadata: map[string]any{"phase": "message-" + probe},
					}},
				}},
				ToolCalls: []runtime.AgentLoopToolCall{{
					ID:       "tool-top-" + probe,
					Name:     "daemon.health",
					Metadata: map[string]any{"phase": "top-" + probe},
				}},
			},
		}, nil
	}))

	result := mgr.executeAgentPlannerActionBatch(
		context.Background(),
		"session-action-queue",
		agentRuntimeTurnRef{TaskID: "task-action-queue", RunID: "run-action-queue", TurnID: "turn-action-queue"},
		"daemon health twice",
		"en",
		"tester",
		"read_only",
		runtime.AgentLoopToolServiceBinding{},
		[]runtime.AgentLoopAction{
			{Name: "daemon.health", Arguments: map[string]any{"probe": "first"}},
			{Name: "daemon.health", Arguments: map[string]any{"probe": "second"}},
		},
		runtime.AgentLoopAuthorization{},
	)

	require.Equal(t, 2, executed)
	require.False(t, result.AuthorizationRequired)
	require.Equal(t, 2, result.ActionCount)
	require.Equal(t, 2, result.ProcessedActionCount)
	require.False(t, result.Truncated)
	require.Nil(t, result.StopGate)
	require.Len(t, result.ToolCalls, 2)
	require.Len(t, result.Messages, 2)

	queueID, ok := result.Metadata[runtime.AgentLoopActionQueueIDMetadataKey].(string)
	require.True(t, ok, "queue id metadata type = %T", result.Metadata[runtime.AgentLoopActionQueueIDMetadataKey])
	require.NotEmpty(t, queueID)
	require.Equal(t, 2, result.Metadata[runtime.AgentLoopActionCountMetadataKey])
	require.Equal(t, false, result.Metadata[runtime.AgentLoopActionBatchTruncatedMetadataKey])

	for idx, probe := range []string{"first", "second"} {
		wantIndex := idx + 1
		topMetadata := result.ToolCalls[idx].Metadata
		messageMetadata := result.Messages[idx].ToolCalls[0].Metadata
		require.Equal(t, "top-"+probe, topMetadata["phase"])
		require.Equal(t, "message-"+probe, messageMetadata["phase"])
		for _, metadata := range []map[string]any{topMetadata, messageMetadata} {
			require.Equal(t, queueID, metadata[runtime.AgentLoopActionQueueIDMetadataKey])
			require.Equal(t, wantIndex, metadata[runtime.AgentLoopActionIndexMetadataKey])
			require.Equal(t, 2, metadata[runtime.AgentLoopActionCountMetadataKey])
			require.Equal(t, "daemon.health", metadata[runtime.AgentLoopActionNameMetadataKey])
			require.Equal(t, "low", metadata["risk"])
		}
	}
}

func TestAgentManagerExecutePlannerActionBatchErrorFallbackUsesRuntimeResultHelper(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, _ runtime.TurnContext) (runtime.TurnResult, error) {
		err := services.NewError(services.CodeUnsupported, "unsupported action", map[string]any{
			"action":  "service-detail-action",
			"target":  "https://example.com",
			"missing": "plugin",
		}, nil)
		return runtime.TurnResult{Status: runtime.TurnResultFailed, Error: err}, err
	}))

	result := mgr.executeAgentPlannerActionBatch(
		context.Background(),
		"session-action-error-fallback",
		agentRuntimeTurnRef{TaskID: "task-action-error-fallback", RunID: "run-action-error-fallback", TurnID: "turn-action-error-fallback"},
		"run daemon health",
		"en",
		"tester",
		"read_only",
		runtime.AgentLoopToolServiceBinding{},
		[]runtime.AgentLoopAction{{Name: "daemon.health", Arguments: map[string]any{"probe": "failure"}}},
		runtime.AgentLoopAuthorization{},
	)

	require.False(t, result.AuthorizationRequired)
	require.Equal(t, 1, result.ActionCount)
	require.Equal(t, 1, result.ProcessedActionCount)
	require.Len(t, result.ToolCalls, 1)
	require.Len(t, result.Messages, 1)
	require.Len(t, result.Messages[0].ToolCalls, 1)

	queueID, ok := result.Metadata[runtime.AgentLoopActionQueueIDMetadataKey].(string)
	require.True(t, ok, "queue id metadata type = %T", result.Metadata[runtime.AgentLoopActionQueueIDMetadataKey])
	require.NotEmpty(t, queueID)
	for _, call := range []runtime.AgentLoopToolCall{result.ToolCalls[0], result.Messages[0].ToolCalls[0]} {
		require.NotNil(t, call.Error)
		require.Equal(t, services.CodeUnsupported, call.Error.Code)
		require.Equal(t, "daemon.health", call.Result[runtime.AgentLoopActionResultActionKey])
		require.Equal(t, "https://example.com", call.Result["target"])
		require.Equal(t, "plugin", call.Result["missing"])
		details, ok := call.Result[runtime.AgentLoopActionResultDetailsKey].(map[string]any)
		require.True(t, ok, "details type = %T", call.Result[runtime.AgentLoopActionResultDetailsKey])
		require.Equal(t, "service-detail-action", details["action"])
		require.Equal(t, queueID, call.Metadata[runtime.AgentLoopActionQueueIDMetadataKey])
		require.Equal(t, 1, call.Metadata[runtime.AgentLoopActionIndexMetadataKey])
		require.Equal(t, "daemon.health", call.Metadata[runtime.AgentLoopActionNameMetadataKey])
	}
}

func TestAgentManagerExecutePlannerActionBatchStopsAtGateBeforeFollowingActions(t *testing.T) {
	eng := runtime.NewEngine(runtime.NewMemStore())
	mgr := newAgentManager(t.TempDir())
	mgr.AttachEngine(eng)

	executed := 0
	attachPlannerActionExecutionActionExecutorFromTurnHandler(t, eng, runtime.TurnExecutorFunc(func(_ context.Context, _ runtime.TurnContext) (runtime.TurnResult, error) {
		executed++
		return runtime.TurnResult{Status: runtime.TurnResultFinished}, nil
	}))

	result := mgr.executeAgentPlannerActionBatch(
		context.Background(),
		"session-action-gate-stop",
		agentRuntimeTurnRef{TaskID: "task-action-gate-stop", RunID: "run-action-gate-stop", TurnID: "turn-action-gate-stop"},
		"summarize the project",
		"en",
		"tester",
		"workspace_write",
		runtime.AgentLoopToolServiceBinding{},
		[]runtime.AgentLoopAction{
			{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.md", "content": "hello"}},
			{Name: "daemon.health", Arguments: map[string]any{"probe": "should-not-run"}},
		},
		runtime.AgentLoopAuthorization{},
	)

	require.Equal(t, 0, executed)
	require.True(t, result.AuthorizationRequired)
	require.Equal(t, 2, result.ActionCount)
	require.Equal(t, 1, result.ProcessedActionCount)
	require.NotNil(t, result.StopGate)
	require.Equal(t, runtime.AgentLoopActionGateExplicitIntentRequired, result.StopGate.Decision.Kind)
	require.Len(t, result.Messages, 1)
	require.Empty(t, result.ToolCalls)
}
