package daemon

import (
	"net/url"
	"strings"
	"testing"
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
