package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func TestHTTPApprovalDecisionPublishesRuntimeEvents(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	store := agentruntime.NewMemStore()
	engine := agentruntime.NewEngine(store)
	agent := newAgentManager(runsDir)
	agent.AttachEngine(engine)

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vapproval-runtime-events"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		agent:     agent,
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	tests := []struct {
		name           string
		action         string
		body           string
		wantDecision   string
		wantStatus     contracts.HITLStatus
		wantEditArgs   bool
		wantTaskFailed bool
	}{
		{
			name:         "approve",
			action:       "approve",
			body:         `{"operator":"local-user","comment":"approved from runtime test"}`,
			wantDecision: "approve",
			wantStatus:   contracts.HITLApproved,
		},
		{
			name:         "deny",
			action:       "deny",
			body:         `{"operator":"local-user","comment":"denied from runtime test"}`,
			wantDecision: "deny",
			wantStatus:   contracts.HITLRejected,
		},
		{
			name:           "edit_args",
			action:         "edit-args",
			body:           `{"operator":"local-user","edit_args":{"target":"https://example.com/edited","method":"POST"}}`,
			wantDecision:   "edit_args",
			wantStatus:     contracts.HITLModified,
			wantEditArgs:   true,
			wantTaskFailed: false,
		},
		{
			name:           "cancel",
			action:         "cancel",
			body:           `{"operator":"local-user","comment":"cancel from runtime test"}`,
			wantDecision:   "cancel",
			wantStatus:     contracts.HITLCancelled,
			wantTaskFailed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runID := "run_runtime_" + tt.name
			approvalID := "approval_runtime_" + tt.name
			stepID := "step_runtime_" + tt.name
			sessionID := "agent_runtime_" + tt.name
			taskID := "task_runtime_" + tt.name
			writeDaemonApprovalRun(t, runsDir, runID, []contracts.HITLRequest{
				daemonTestApproval(approvalID, runID, stepID),
			})
			addApprovalRuntimeTestSession(t, agent, sessionID, taskID, runID)

			resp := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/"+runID+"/approvals/"+approvalID+"/"+tt.action, "test-token", tt.body)
			if resp.StatusCode != http.StatusOK || !resp.Body.OK || resp.Body.Data["status"] != string(tt.wantStatus) {
				t.Fatalf("unexpected approval response: status=%d body=%#v", resp.StatusCode, resp.Body)
			}

			events, err := store.ListEvents(context.Background(), agentruntime.EventFilter{RunID: runID}, 10)
			if err != nil {
				t.Fatalf("ListEvents(%s) error = %v", runID, err)
			}
			resolved := firstApprovalRuntimeEvent(events, "approval_resolved")
			if resolved == nil {
				t.Fatalf("missing approval_resolved event in %#v", events)
			}
			if resolved.RunID != runID || resolved.SessionID != sessionID || resolved.TaskID != taskID {
				t.Fatalf("unexpected approval_resolved envelope: %#v", *resolved)
			}
			if resolved.Data["approval_id"] != approvalID ||
				resolved.Data["step_id"] != stepID ||
				resolved.Data["run_id"] != runID ||
				resolved.Data["decision"] != tt.wantDecision ||
				resolved.Data["status"] != string(tt.wantStatus) ||
				resolved.Data["requested_action"] != "execute_step" ||
				resolved.Data["resolved_by"] != "local-user" {
				t.Fatalf("unexpected approval_resolved data: %#v", resolved.Data)
			}
			if _, ok := resolved.Data["comment"]; !ok {
				t.Fatalf("approval_resolved data should include comment key: %#v", resolved.Data)
			}
			if tt.wantEditArgs {
				editArgs, ok := resolved.Data["edit_args"].(map[string]any)
				if !ok || editArgs["target"] != "https://example.com/edited" || editArgs["method"] != "POST" {
					t.Fatalf("unexpected edit_args data: %#v", resolved.Data["edit_args"])
				}
			}

			failed := firstApprovalRuntimeEvent(events, "task_failed")
			if !tt.wantTaskFailed {
				if failed != nil {
					t.Fatalf("unexpected task_failed event: %#v", *failed)
				}
				return
			}
			if failed == nil {
				t.Fatalf("missing cancel task_failed event in %#v", events)
			}
			if failed.RunID != runID || failed.SessionID != sessionID || failed.TaskID != taskID {
				t.Fatalf("unexpected task_failed envelope: %#v", *failed)
			}
			if failed.Data["reason"] != "approval_cancelled" ||
				failed.Data["approval_id"] != approvalID ||
				failed.Data["step_id"] != stepID ||
				failed.Data["run_id"] != runID ||
				failed.Data["status"] != string(contracts.HITLCancelled) {
				t.Fatalf("unexpected task_failed data: %#v", failed.Data)
			}
		})
	}
}

func TestHTTPApprovalDecisionRuntimeEngineAbsentNoOp(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	runID := "run_runtime_absent"
	approvalID := "approval_runtime_absent"
	writeDaemonApprovalRun(t, runsDir, runID, []contracts.HITLRequest{
		daemonTestApproval(approvalID, runID, "step_runtime_absent"),
	})

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vapproval-runtime-absent"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
		agent:     newAgentManager(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	resp := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/"+runID+"/approvals/"+approvalID+"/approve", "test-token", `{"operator":"local-user","comment":"approve without engine"}`)
	if resp.StatusCode != http.StatusOK || !resp.Body.OK || resp.Body.Data["status"] != string(contracts.HITLApproved) {
		t.Fatalf("unexpected approval response without engine: status=%d body=%#v", resp.StatusCode, resp.Body)
	}
	resolution := asMap(t, resp.Body.Data["resolution"])
	if resolution["decision"] != "approve" || resolution["resolved_by"] != "local-user" || resolution["comment"] != "approve without engine" {
		t.Fatalf("unexpected approval resolution without engine: %#v", resolution)
	}
}

func addApprovalRuntimeTestSession(t *testing.T, agent *agentManager, sessionID, taskID, runID string) {
	t.Helper()
	now := time.Now().UTC()
	session := &agentSession{
		SessionID: sessionID,
		CreatedAt: now,
		UpdatedAt: now,
		Operator:  "tester",
		Language:  "zh",
		Status:    "running",
		ActiveTask: &agentTask{
			ID:          taskID,
			Goal:        "approval runtime event test",
			Kind:        "web_scan",
			Status:      "running",
			ActiveRunID: runID,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		NextAction: &agentNextAction{Type: "approval_required", Reason: "waiting approval"},
	}
	session.normalize()
	agent.mu.Lock()
	defer agent.mu.Unlock()
	agent.sessions[sessionID] = session
}

func firstApprovalRuntimeEvent(events []agentruntime.Event, eventType string) *agentruntime.Event {
	for idx := range events {
		if events[idx].Type == eventType {
			return &events[idx]
		}
	}
	return nil
}
