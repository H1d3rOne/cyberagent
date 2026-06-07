package daemon

import (
	"net/url"
	"strings"
	"testing"

	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

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
	if metadata["action_queue_id"] != "queue-test" {
		t.Fatalf("action_queue_id = %#v, want queue-test", metadata["action_queue_id"])
	}
	if metadata["action_index"] != 2 {
		t.Fatalf("action_index = %#v, want 2", metadata["action_index"])
	}
	if metadata["action_count"] != 3 {
		t.Fatalf("action_count = %#v, want 3", metadata["action_count"])
	}
	if metadata["action_name"] != "daemon.health" {
		t.Fatalf("action_name = %#v, want daemon.health", metadata["action_name"])
	}
	if _, ok := metadata["action_index"].(int); !ok {
		t.Fatalf("action_index type = %T, want int", metadata["action_index"])
	}
	if _, ok := metadata["action_count"].(int); !ok {
		t.Fatalf("action_count type = %T, want int", metadata["action_count"])
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
