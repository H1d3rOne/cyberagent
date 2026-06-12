package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestExecuteAgentLoopActionDispatchesAttachedExecutorWithCloneIsolation(t *testing.T) {
	e, _ := newTestEngine()
	ctx := context.WithValue(context.Background(), struct{}{}, "ctx-value")
	enabled := true
	handle := &struct{ name string }{name: "legacy-handle"}
	req := AgentLoopActionExecutorRequest{
		Request: AgentLoopRequest{
			Turn: TurnContext{
				SessionID:      "session-action-executor",
				TaskID:         "task-action-executor",
				RunID:          "run-action-executor",
				TurnID:         "turn-action-executor",
				Prompt:         "run action",
				OperationLevel: "safe_scan",
			},
			Language: "en",
			Operator: "tester",
			ToolServices: AgentLoopToolServiceBinding{
				Kind:   "daemon_router_services",
				Handle: handle,
				Metadata: map[string]any{
					"adapter": "daemon_legacy_router_services",
				},
			},
			Authorization: AgentLoopAuthorization{
				AuthProfiles: []AgentLoopAuthProfile{{
					ID:      "profile-1",
					Enabled: &enabled,
				}},
				DefaultAuthProfileID: "profile-1",
			},
			Metadata: map[string]any{"source": "caller"},
		},
		Action: AgentLoopAction{
			Name:      "daemon.health",
			Arguments: map[string]any{"probe": "ok"},
		},
	}
	executorResult := AgentLoopActionExecutorResult{
		Status: TurnResultFinished,
		Reason: "executed",
		Messages: []AgentLoopMessage{{
			ID:      "msg-action-result",
			Role:    "tool",
			Content: "action ok",
			ToolCalls: []AgentLoopToolCall{{
				ID:     "tool-nested",
				Name:   "daemon.health",
				Result: map[string]any{"ok": true},
			}},
		}},
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-action-result",
			Name:     "daemon.health",
			Result:   map[string]any{"ok": true},
			Metadata: map[string]any{"source": "executor"},
		}},
		Metadata: map[string]any{"egress": "runtime"},
	}
	called := false

	e.AttachAgentLoopActionExecutor(AgentLoopActionExecutorFunc(func(gotCtx context.Context, got AgentLoopActionExecutorRequest) (AgentLoopActionExecutorResult, error) {
		called = true
		if gotCtx != ctx {
			t.Fatalf("ExecuteAgentLoopAction ctx = %#v, want original ctx", gotCtx)
		}
		if got.Request.Turn.SessionID != req.Request.Turn.SessionID ||
			got.Request.Language != req.Request.Language ||
			got.Request.Operator != req.Request.Operator ||
			got.Request.ToolServices.Handle != handle ||
			got.Action.Name != req.Action.Name ||
			got.Action.Arguments["probe"] != "ok" {
			t.Fatalf("AgentLoopActionExecutorRequest = %#v, want %#v", got, req)
		}
		got.Request.Metadata["source"] = "mutated"
		got.Request.ToolServices.Metadata["adapter"] = "mutated"
		got.Request.Authorization.AuthProfiles[0].ID = "mutated"
		*got.Request.Authorization.AuthProfiles[0].Enabled = false
		got.Action.Arguments["probe"] = "mutated"
		return executorResult, nil
	}))

	result, err := e.ExecuteAgentLoopAction(ctx, req)
	if err != nil {
		t.Fatalf("ExecuteAgentLoopAction() error = %v", err)
	}
	if !called {
		t.Fatalf("ExecuteAgentLoopAction() did not call attached executor")
	}
	if req.Request.Metadata["source"] != "caller" ||
		req.Request.ToolServices.Metadata["adapter"] != "daemon_legacy_router_services" ||
		req.Request.Authorization.AuthProfiles[0].ID != "profile-1" ||
		*req.Request.Authorization.AuthProfiles[0].Enabled != true ||
		req.Action.Arguments["probe"] != "ok" {
		t.Fatalf("ExecuteAgentLoopAction() mutated request = %#v", req)
	}

	executorResult.Messages[0].Content = "mutated"
	executorResult.Messages[0].ToolCalls[0].Result["ok"] = false
	executorResult.ToolCalls[0].Metadata["source"] = "mutated"
	executorResult.Metadata["egress"] = "mutated"
	if result.Status != TurnResultFinished ||
		result.Reason != "executed" ||
		result.Messages[0].Content != "action ok" ||
		result.Messages[0].ToolCalls[0].Result["ok"] != true ||
		result.ToolCalls[0].Metadata["source"] != "executor" ||
		result.Metadata["egress"] != "runtime" {
		t.Fatalf("ExecuteAgentLoopAction() result = %#v, want cloned executor result", result)
	}
}

func TestExecuteAgentLoopActionWithoutExecutorReturnsStableError(t *testing.T) {
	e, _ := newTestEngine()
	result, err := e.ExecuteAgentLoopAction(context.Background(), AgentLoopActionExecutorRequest{
		Request: AgentLoopRequest{Turn: TurnContext{SessionID: "session-action-missing"}},
		Action:  AgentLoopAction{Name: "daemon.health"},
	})
	if !errors.Is(err, ErrAgentLoopActionExecutorNotAttached) {
		t.Fatalf("ExecuteAgentLoopAction() error = %v, want ErrAgentLoopActionExecutorNotAttached", err)
	}
	if result.Status != TurnResultBlocked || !errors.Is(result.Error, ErrAgentLoopActionExecutorNotAttached) || result.Reason == "" {
		t.Fatalf("ExecuteAgentLoopAction() result = %#v, want blocked missing-executor result", result)
	}
}

func TestExecuteAgentLoopActionNilEngineReturnsStableError(t *testing.T) {
	var e *Engine
	result, err := e.ExecuteAgentLoopAction(context.Background(), AgentLoopActionExecutorRequest{
		Request: AgentLoopRequest{Turn: TurnContext{SessionID: "session-action-nil"}},
		Action:  AgentLoopAction{Name: "daemon.health"},
	})
	if !errors.Is(err, ErrNilEngine) {
		t.Fatalf("ExecuteAgentLoopAction() error = %v, want ErrNilEngine", err)
	}
	if result.Status != TurnResultBlocked || !errors.Is(result.Error, ErrNilEngine) || result.Reason == "" {
		t.Fatalf("ExecuteAgentLoopAction() result = %#v, want blocked nil-engine result", result)
	}
}
