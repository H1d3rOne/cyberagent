package daemon

import (
	"context"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
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
			case runtime.SessionMutationRequestAuthorization:
				require.Equal(t, string(runtime.SessionMutationRequestAuthorization), records[0].Turn.Origin)
				payload, ok := records[0].Payload.(runtime.AgentLoopAuthorizationRequestPayload)
				require.True(t, ok, "payload type = %T", records[0].Payload)
				require.Equal(t, tc.language, payload.Language)
				require.Equal(t, decision.ActionName, payload.ActionName)
				require.Equal(t, tc.wantRequired, payload.RequiredLevel)
				require.Equal(t, tc.wantPolicy, payload.Policy)
				require.Equal(t, tc.action, payload.Action)
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
