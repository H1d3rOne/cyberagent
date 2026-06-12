package daemon

import (
	"net/url"
	"strings"
	"testing"

	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

func testAgentLoopToolServices() agentruntime.AgentLoopToolServiceBinding {
	return agentLoopToolServiceBindingFromRouterServices(routerServices{})
}

func TestScheduleIntentBlocksImmediateScanSubstitution(t *testing.T) {
	reason, _, mismatch := agentPlannerActionCapabilityMismatch("每小时扫描 https://example.com", "scan.web.safe", "zh")
	if !mismatch {
		t.Fatal("expected schedule intent to block immediate scan action")
	}
	if !strings.Contains(reason, agentIntentScheduleCreate) {
		t.Fatalf("reason = %q, want schedule intent", reason)
	}

	reason, _, mismatch = agentPlannerActionCapabilityMismatch("每小时扫描 https://example.com", "schedule.create", "zh")
	if mismatch {
		t.Fatalf("schedule.create should be allowed, got reason %q", reason)
	}
}

func TestAgentPlannerActionGateDecisionOrder(t *testing.T) {
	cases := []struct {
		name          string
		prompt        string
		language      string
		operation     string
		action        agentruntime.AgentLoopAction
		wantKind      agentPlannerActionGateKind
		wantPolicy    string
		wantRequired  string
		wantIntentID  string
		wantAction    string
		wantReasonSub string
	}{
		{
			name:         "capability mismatch wins before bypass",
			prompt:       "每小时扫描 https://example.com",
			language:     "zh",
			operation:    "bypass",
			action:       agentruntime.AgentLoopAction{Name: "scan.web.safe", Arguments: map[string]any{"target": "https://example.com"}},
			wantKind:     agentPlannerActionGateCapabilityBlocked,
			wantRequired: "safe_scan",
			wantIntentID: agentIntentScheduleCreate,
			wantAction:   "scan.web.safe",
		},
		{
			name:          "explicit operator intent required before operation/manual gates",
			prompt:        "summarize the project",
			language:      "en",
			operation:     "workspace_write",
			action:        agentruntime.AgentLoopAction{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.md", "content": "hello"}},
			wantKind:      agentPlannerActionGateExplicitIntentRequired,
			wantPolicy:    "explicit_operator_intent_required",
			wantRequired:  "workspace_write",
			wantAction:    "workspace.files.write",
			wantReasonSub: "explicit operator confirmation",
		},
		{
			name:          "operation level required before manual authorization",
			prompt:        "scan https://example.com",
			language:      "en",
			operation:     "read_only",
			action:        agentruntime.AgentLoopAction{Name: "scan.web.safe", Arguments: map[string]any{"target": "https://example.com"}},
			wantKind:      agentPlannerActionGateOperationLevelRequired,
			wantPolicy:    "operation_level_required",
			wantRequired:  "safe_scan",
			wantAction:    "scan.web.safe",
			wantReasonSub: "requires safe_scan",
		},
		{
			name:          "manual authorization after explicit intent and operation level pass",
			prompt:        "write file notes.md content: hello",
			language:      "en",
			operation:     "workspace_write",
			action:        agentruntime.AgentLoopAction{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.md", "content": "hello"}},
			wantKind:      agentPlannerActionGateManualAuthorizationRequired,
			wantPolicy:    "manual_tool_authorization_required",
			wantRequired:  "workspace_write",
			wantAction:    "workspace.files.write",
			wantReasonSub: "requires human confirmation",
		},
		{
			name:         "allow read only action",
			prompt:       "daemon health",
			language:     "en",
			operation:    "read_only",
			action:       agentruntime.AgentLoopAction{Name: "daemon.health", Arguments: map[string]any{}},
			wantKind:     agentPlannerActionGateAllow,
			wantRequired: "read_only",
			wantAction:   "daemon.health",
		},
		{
			name:         "bypass skips explicit operation and manual gates",
			prompt:       "summarize the project",
			language:     "en",
			operation:    "bypass",
			action:       agentruntime.AgentLoopAction{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.md", "content": "hello"}},
			wantKind:     agentPlannerActionGateAllow,
			wantRequired: "workspace_write",
			wantAction:   "workspace.files.write",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideAgentPlannerActionGateFromAgentLoopAction(tc.prompt, tc.language, tc.operation, tc.action)
			legacyGot := decideAgentPlannerActionGate(tc.prompt, tc.language, tc.operation, agentPlannerActionFromAgentLoopAction(tc.action))
			if legacyGot != got {
				t.Fatalf("legacy gate decision = %#v, want runtime decision %#v", legacyGot, got)
			}
			if got.Kind != tc.wantKind {
				t.Fatalf("Kind = %q, want %q (decision=%#v)", got.Kind, tc.wantKind, got)
			}
			if got.Policy != tc.wantPolicy {
				t.Fatalf("Policy = %q, want %q", got.Policy, tc.wantPolicy)
			}
			if got.RequiredLevel != tc.wantRequired {
				t.Fatalf("RequiredLevel = %q, want %q", got.RequiredLevel, tc.wantRequired)
			}
			if got.IntentID != tc.wantIntentID {
				t.Fatalf("IntentID = %q, want %q", got.IntentID, tc.wantIntentID)
			}
			if got.ActionName != tc.wantAction {
				t.Fatalf("ActionName = %q, want %q", got.ActionName, tc.wantAction)
			}
			if tc.wantReasonSub != "" && !strings.Contains(got.Reason, tc.wantReasonSub) {
				t.Fatalf("Reason = %q, want substring %q", got.Reason, tc.wantReasonSub)
			}
		})
	}
}

func TestAgentPlannerActionQueueMetadata(t *testing.T) {
	metadata := agentPlannerActionQueueMetadata("queue-test", 2, 3, "daemon.health")
	if metadata[agentruntime.AgentLoopActionQueueIDMetadataKey] != "queue-test" {
		t.Fatalf("action_queue_id = %#v, want queue-test", metadata[agentruntime.AgentLoopActionQueueIDMetadataKey])
	}
	if metadata[agentruntime.AgentLoopActionIndexMetadataKey] != 2 {
		t.Fatalf("action_index = %#v, want 2", metadata[agentruntime.AgentLoopActionIndexMetadataKey])
	}
	if metadata[agentruntime.AgentLoopActionCountMetadataKey] != 3 {
		t.Fatalf("action_count = %#v, want 3", metadata[agentruntime.AgentLoopActionCountMetadataKey])
	}
	if metadata[agentruntime.AgentLoopActionNameMetadataKey] != "daemon.health" {
		t.Fatalf("action_name = %#v, want daemon.health", metadata[agentruntime.AgentLoopActionNameMetadataKey])
	}
	if _, ok := metadata[agentruntime.AgentLoopActionIndexMetadataKey].(int); !ok {
		t.Fatalf("action_index type = %T, want int", metadata[agentruntime.AgentLoopActionIndexMetadataKey])
	}
	if _, ok := metadata[agentruntime.AgentLoopActionCountMetadataKey].(int); !ok {
		t.Fatalf("action_count type = %T, want int", metadata[agentruntime.AgentLoopActionCountMetadataKey])
	}

	prepared := agentruntime.PrepareAgentLoopActionBatchWithGeneratedQueueID([]agentruntime.AgentLoopAction{
		{Name: "daemon.health", Arguments: map[string]any{"probe": "one"}},
		{Name: "plugins.list", Arguments: map[string]any{"probe": "two"}},
		{Name: "runs.list", Arguments: map[string]any{"probe": "three"}},
		{Name: "runs.context", Arguments: map[string]any{"probe": "four"}},
		{Name: "runs.approvals.list", Arguments: map[string]any{"probe": "five"}},
		{Name: "workspace.files.list", Arguments: map[string]any{"probe": "six"}},
		{Name: "workspace.search", Arguments: map[string]any{"probe": "seven"}},
	}, func() string { return "queue-batch" })
	if len(prepared.Actions) != agentruntime.AgentLoopActionBatchDefaultMaxActions {
		t.Fatalf("prepared actions = %d, want max %d", len(prepared.Actions), agentruntime.AgentLoopActionBatchDefaultMaxActions)
	}
	if !prepared.Result.Truncated || prepared.Result.ActionCount != agentruntime.AgentLoopActionBatchDefaultMaxActions {
		t.Fatalf("prepared result = %#v, want truncated effective count", prepared.Result)
	}
	if prepared.Result.Metadata[agentruntime.AgentLoopActionQueueIDMetadataKey] != "queue-batch" || prepared.Result.Metadata[agentruntime.AgentLoopActionCountMetadataKey] != agentruntime.AgentLoopActionBatchDefaultMaxActions {
		t.Fatalf("prepared metadata = %#v", prepared.Result.Metadata)
	}
}

func TestAppendAgentPlannerActionBatchResultWithQueueMetadataPreservesDefaultMetadata(t *testing.T) {
	prepared := agentruntime.PrepareAgentLoopActionBatch([]agentruntime.AgentLoopAction{
		{Name: "daemon.health", Arguments: map[string]any{"probe": "first"}},
		{Name: "plugins.list", Arguments: map[string]any{"probe": "second"}},
	}, "queue-test", 0)
	result := prepared.Result
	messages := []agentruntime.AgentLoopMessage{{
		ID:   "msg-action",
		Role: "tool",
		ToolCalls: []agentruntime.AgentLoopToolCall{{
			ID:       "tool-message",
			Name:     "daemon.health",
			Metadata: map[string]any{"phase": "message-phase"},
		}},
	}}
	toolCalls := []agentruntime.AgentLoopToolCall{{
		ID:       "tool-top",
		Name:     "daemon.health",
		Metadata: map[string]any{"phase": "top-phase"},
	}}

	appendAgentPlannerActionBatchResultWithQueueMetadata(&result, prepared, 2, "daemon.health", messages, toolCalls)
	messages[0].ToolCalls[0].Metadata["phase"] = "mutated-message"
	toolCalls[0].Metadata["phase"] = "mutated-top"

	if result.ProcessedActionCount != 2 {
		t.Fatalf("ProcessedActionCount = %d, want 2", result.ProcessedActionCount)
	}
	if len(result.Messages) != 1 || len(result.ToolCalls) != 1 || len(result.Messages[0].ToolCalls) != 1 {
		t.Fatalf("unexpected result shape: messages=%#v toolCalls=%#v", result.Messages, result.ToolCalls)
	}
	for _, metadata := range []map[string]any{result.ToolCalls[0].Metadata, result.Messages[0].ToolCalls[0].Metadata} {
		if metadata[agentruntime.AgentLoopActionQueueIDMetadataKey] != "queue-test" ||
			metadata[agentruntime.AgentLoopActionIndexMetadataKey] != 2 ||
			metadata[agentruntime.AgentLoopActionCountMetadataKey] != 2 ||
			metadata[agentruntime.AgentLoopActionNameMetadataKey] != "daemon.health" ||
			metadata["risk"] != "low" {
			t.Fatalf("metadata = %#v, want queue/default metadata", metadata)
		}
	}
	if result.ToolCalls[0].Metadata["phase"] != "top-phase" {
		t.Fatalf("top-level phase = %#v, want top-phase", result.ToolCalls[0].Metadata["phase"])
	}
	if result.Messages[0].ToolCalls[0].Metadata["phase"] != "message-phase" {
		t.Fatalf("message tool phase = %#v, want message-phase", result.Messages[0].ToolCalls[0].Metadata["phase"])
	}
	if _, ok := toolCalls[0].Metadata[agentruntime.AgentLoopActionQueueIDMetadataKey]; ok {
		t.Fatalf("original top-level call mutated: %#v", toolCalls[0].Metadata)
	}
	if _, ok := messages[0].ToolCalls[0].Metadata[agentruntime.AgentLoopActionQueueIDMetadataKey]; ok {
		t.Fatalf("original message tool call mutated: %#v", messages[0].ToolCalls[0].Metadata)
	}
}

func TestApplyAgentPlannerActionBatchStepResultPreservesDefaultMetadata(t *testing.T) {
	prepared := agentruntime.PrepareAgentLoopActionBatch([]agentruntime.AgentLoopAction{
		{Name: "daemon.health", Arguments: map[string]any{"probe": "first"}},
		{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.md"}},
	}, "queue-step", 0)
	result := prepared.Result
	executionStep := agentruntime.AgentLoopActionBatchExecutionStepResult(
		1,
		"daemon.health",
		[]agentruntime.AgentLoopMessage{{
			ID:   "msg-step",
			Role: "tool",
			ToolCalls: []agentruntime.AgentLoopToolCall{{
				ID:       "tool-message-step",
				Name:     "daemon.health",
				Metadata: map[string]any{"phase": "message-phase"},
			}},
		}},
		[]agentruntime.AgentLoopToolCall{{
			ID:       "tool-top-step",
			Name:     "daemon.health",
			Metadata: map[string]any{"phase": "top-phase"},
		}},
	)

	if stopped := applyAgentPlannerActionBatchStepResult(&result, prepared, executionStep); stopped {
		t.Fatalf("execution step stopped")
	}
	executionStep.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated-message"
	executionStep.ToolCalls[0].Metadata["phase"] = "mutated-top"

	if result.ProcessedActionCount != 1 || result.AuthorizationRequired || result.StopGate != nil {
		t.Fatalf("execution result = %#v, want processed without stop gate", result)
	}
	for _, metadata := range []map[string]any{result.ToolCalls[0].Metadata, result.Messages[0].ToolCalls[0].Metadata} {
		if metadata[agentruntime.AgentLoopActionQueueIDMetadataKey] != "queue-step" ||
			metadata[agentruntime.AgentLoopActionIndexMetadataKey] != 1 ||
			metadata[agentruntime.AgentLoopActionCountMetadataKey] != 2 ||
			metadata[agentruntime.AgentLoopActionNameMetadataKey] != "daemon.health" ||
			metadata["risk"] != "low" {
			t.Fatalf("execution metadata = %#v, want queue/default metadata", metadata)
		}
	}
	if result.ToolCalls[0].Metadata["phase"] != "top-phase" ||
		result.Messages[0].ToolCalls[0].Metadata["phase"] != "message-phase" {
		t.Fatalf("execution step metadata was not cloned: top=%#v message=%#v", result.ToolCalls[0].Metadata, result.Messages[0].ToolCalls[0].Metadata)
	}

	gate := agentruntime.AgentLoopActionGateResult{
		Decision: agentruntime.AgentLoopActionGateDecision{
			Kind:       agentruntime.AgentLoopActionGateManualAuthorizationRequired,
			ActionName: "workspace.files.write",
		},
		Messages: []agentruntime.AgentLoopMessage{{
			ID:      "msg-gate-step",
			Content: "authorization required",
			ToolCalls: []agentruntime.AgentLoopToolCall{{
				ID:       "tool-gate-message-step",
				Name:     "workspace.files.write",
				Metadata: map[string]any{"phase": "gate-message"},
			}},
		}},
		ToolCalls: []agentruntime.AgentLoopToolCall{{
			ID:       "tool-gate-top-step",
			Name:     "workspace.files.write",
			Metadata: map[string]any{"phase": "gate-top"},
		}},
		AuthorizationRequired: true,
		Handled:               true,
	}
	gateStep := agentruntime.AgentLoopActionBatchGateStepResult(2, gate)
	if stopped := applyAgentPlannerActionBatchStepResult(&result, prepared, gateStep); !stopped {
		t.Fatalf("gate step did not stop")
	}
	gateStep.StopGate.Messages[0].ToolCalls[0].Metadata["phase"] = "mutated-gate-message"
	gateStep.StopGate.ToolCalls[0].Metadata["phase"] = "mutated-gate-top"

	if !result.AuthorizationRequired || result.ProcessedActionCount != 2 || result.StopGate == nil {
		t.Fatalf("gate result = %#v, want stopped authorization", result)
	}
	if result.StopGate.Decision.ActionName != "workspace.files.write" {
		t.Fatalf("stop gate decision = %#v", result.StopGate.Decision)
	}
	if result.StopGate.Messages[0].ToolCalls[0].Metadata["phase"] != "gate-message" ||
		result.StopGate.ToolCalls[0].Metadata["phase"] != "gate-top" {
		t.Fatalf("gate step was not cloned: messages=%#v toolCalls=%#v", result.StopGate.Messages, result.StopGate.ToolCalls)
	}
	if _, ok := result.StopGate.ToolCalls[0].Metadata[agentruntime.AgentLoopActionQueueIDMetadataKey]; ok {
		t.Fatalf("gate step should not receive queue metadata: %#v", result.StopGate.ToolCalls[0].Metadata)
	}
}

func TestScheduleFallbackOverridesModelCapabilityGapText(t *testing.T) {
	plan := agentFallbackPlannerFromPrompt("每小时扫描 https://example.com", "当前工具集不支持创建定时任务/cron job", "zh")
	if plan == nil || len(plan.Actions) != 1 {
		t.Fatalf("expected one fallback action, got %#v", plan)
	}
	action := plan.Actions[0]
	if action.Name != "schedule.create" {
		t.Fatalf("action name = %q, want schedule.create", action.Name)
	}
	if action.Arguments["action"] != "agent_prompt" || action.Arguments["prompt"] != "扫描 https://example.com" {
		t.Fatalf("unexpected schedule args: %#v", action.Arguments)
	}
}

func TestScheduleFallbackCreatesGenericAgentPromptForPortScan(t *testing.T) {
	plan := agentFallbackPlannerFromPrompt("7:20执行端口扫描 example.com ports=80,443", "当前工具集不支持创建定时任务/cron job", "zh")
	if plan == nil || len(plan.Actions) != 1 {
		t.Fatalf("expected one fallback action, got %#v", plan)
	}
	action := plan.Actions[0]
	if action.Name != "schedule.create" {
		t.Fatalf("action name = %q, want schedule.create", action.Name)
	}
	if action.Arguments["cron"] != "20 7 * * *" {
		t.Fatalf("cron = %#v, want daily 07:20", action.Arguments["cron"])
	}
	if action.Arguments["action"] != "agent_prompt" {
		t.Fatalf("action args = %#v, want agent_prompt", action.Arguments)
	}
	if action.Arguments["prompt"] != "端口扫描 example.com ports=80,443" {
		t.Fatalf("prompt = %#v, want executable port scan prompt", action.Arguments["prompt"])
	}
}

func TestScheduleIntentRoutesBeforeLLMSynthesis(t *testing.T) {
	if !agentShouldRouteBeforeLLMSynthesis("每小时扫描 https://example.com") {
		t.Fatal("schedule creation should route before LLM synthesis")
	}
	if !agentShouldRouteBeforeLLMSynthesis("列出所有定时任务") {
		t.Fatal("schedule list should route before LLM synthesis")
	}
}

func TestAgentURLOutboundAllowedRejectsInternalTargets(t *testing.T) {
	cases := []string{
		"http://127.0.0.1:8080/",
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://100.64.0.1/",
		"http://224.0.0.1/",
		"http://[::1]/",
		"http://metadata.google.internal/",
	}
	for _, raw := range cases {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("url.Parse(%q) error: %v", raw, err)
		}
		if err := agentURLOutboundAllowed(parsed); err == nil {
			t.Fatalf("agentURLOutboundAllowed(%q) = nil, want error", raw)
		}
	}
}

func TestAgentURLOutboundAllowedAllowsPublicIP(t *testing.T) {
	parsed, err := url.Parse("https://93.184.216.34/")
	if err != nil {
		t.Fatalf("url.Parse error: %v", err)
	}
	if err := agentURLOutboundAllowed(parsed); err != nil {
		t.Fatalf("agentURLOutboundAllowed public IP error: %v", err)
	}
}

func TestHTMLToReadableStripsUnsafeAndPreservesText(t *testing.T) {
	title, body := htmlToReadable(`<!doctype html>
<html>
<head>
<title>Example &amp; Test</title>
<style>body { display: none; }</style>
<script>alert('x')</script>
</head>
<body>
<h1>Heading</h1>
<p>First&nbsp;paragraph &lt;ok&gt;.</p>
<div>Second <strong>paragraph</strong>.</div>
<!-- hidden -->
</body>
</html>`)
	if title != "Example & Test" {
		t.Fatalf("title = %q, want %q", title, "Example & Test")
	}
	for _, unwanted := range []string{"display: none", "alert('x')", "hidden", "<strong>"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body contains stripped content %q: %q", unwanted, body)
		}
	}
	for _, wanted := range []string{"Heading", "First paragraph <ok>.", "Second paragraph."} {
		if !strings.Contains(body, wanted) {
			t.Fatalf("body missing %q: %q", wanted, body)
		}
	}
}
