package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/events"
	"github.com/H1d3rOne/cyberagent/internal/runtime/command"
	"github.com/H1d3rOne/cyberagent/internal/runtime/state"
)

func TestRunWebScanProducesMVPArtifacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><head><script src="/app.js"></script></head><body><a href="/admin">Admin</a><form action="/login" method="post"></form></body></html>`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte("<urlset></urlset>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "run_001")
	result, err := RunWebScan(context.Background(), WebScanRequest{Target: server.URL, Scope: server.URL, Out: out, Profile: "safe", NoExternalTools: true})
	if err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}
	if result.Job.Status != contracts.StatusSucceeded {
		t.Fatalf("job status = %s, want succeeded", result.Job.Status)
	}
	if result.Workspace != out {
		t.Fatalf("workspace = %s, want %s", result.Workspace, out)
	}
	if len(result.Findings) == 0 {
		t.Fatalf("expected header findings")
	}
	if len(result.Evidence) == 0 || len(result.Artifacts) == 0 {
		t.Fatalf("expected evidence/artifacts, got evidence=%d artifacts=%d", len(result.Evidence), len(result.Artifacts))
	}

	mustExist(t, filepath.Join(out, "target.json"))
	mustExist(t, filepath.Join(out, "scope.json"))
	mustExist(t, filepath.Join(out, "job.json"))
	mustExist(t, filepath.Join(out, "plan.json"))
	mustExist(t, filepath.Join(out, "events.jsonl"))
	mustExist(t, filepath.Join(out, "commands.log"))
	mustExist(t, filepath.Join(out, "artifacts", "http", "probe.json"))
	mustExist(t, filepath.Join(out, "artifacts", "http", "homepage.html"))
	mustExist(t, filepath.Join(out, "evidence", "index.json"))
	mustExist(t, filepath.Join(out, "findings.json"))
	mustExist(t, filepath.Join(out, "reports", "report.md"))
	mustExist(t, filepath.Join(out, "reports", "report.json"))

	calls, err := command.NewRecorder(filepath.Join(out, "commands.log")).ReadAll()
	if err != nil {
		t.Fatalf("read commands.log: %v", err)
	}
	if len(calls) != len(result.Plan.Steps) {
		t.Fatalf("commands.log calls = %d, want plan steps %d", len(calls), len(result.Plan.Steps))
	}
	seenProbe := false
	for _, call := range calls {
		if call.Kind != contracts.ToolCallKindBuiltin {
			t.Fatalf("call kind = %s, want builtin: %#v", call.Kind, call)
		}
		if call.Status != contracts.StatusSucceeded && call.Status != contracts.StatusSkipped {
			t.Fatalf("unexpected call status: %#v", call)
		}
		if call.ToolName == "builtin.http_probe" {
			seenProbe = true
		}
	}
	if !seenProbe {
		t.Fatalf("commands.log missing builtin.http_probe call: %#v", calls)
	}

	stateStore, err := state.Open(filepath.Join(out, "state", "bolt.db"))
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer stateStore.Close()
	steps, err := stateStore.ListSteps(result.Job.ID)
	if err != nil {
		t.Fatalf("ListSteps() error = %v", err)
	}
	if len(steps) != len(result.Plan.Steps) {
		t.Fatalf("state steps = %d, want %d", len(steps), len(result.Plan.Steps))
	}
	for _, step := range steps {
		if step.Status != contracts.StatusSucceeded && step.Status != contracts.StatusSkipped {
			t.Fatalf("unexpected persisted step status: %#v", step)
		}
		if step.StartedAt == nil || step.FinishedAt == nil {
			t.Fatalf("persisted step missing lifecycle timestamps: %#v", step)
		}
	}

	var findings []contracts.Finding
	readJSON(t, filepath.Join(out, "findings.json"), &findings)
	for _, finding := range findings {
		if len(finding.EvidenceIDs) == 0 {
			t.Fatalf("finding without evidence: %#v", finding)
		}
	}

	var report struct {
		Job contracts.Job `json:"job"`
	}
	readJSON(t, filepath.Join(out, "reports", "report.json"), &report)
	if report.Job.Status != contracts.StatusSucceeded {
		t.Fatalf("report job status = %s, want succeeded", report.Job.Status)
	}
	reportMD, err := os.ReadFile(filepath.Join(out, "reports", "report.md"))
	if err != nil {
		t.Fatalf("read report.md: %v", err)
	}
	if !strings.Contains(string(reportMD), "- Status: `succeeded`") {
		t.Fatalf("report.md missing succeeded status: %s", reportMD)
	}
}

func TestRunWebScanRequiresScope(t *testing.T) {
	_, err := RunWebScan(context.Background(), WebScanRequest{Target: "http://127.0.0.1:1", Scope: ""})
	if err == nil {
		t.Fatalf("expected missing scope error")
	}
}

func TestBuildScopeFromString(t *testing.T) {
	scope, err := BuildScopeFromString("scope_001", "http://localhost:8080,127.0.0.1,10.10.10.0/24,example.test")
	if err != nil {
		t.Fatalf("BuildScopeFromString() error = %v", err)
	}
	if len(scope.Allowed.Hosts) < 2 {
		t.Fatalf("expected hosts in scope: %#v", scope.Allowed.Hosts)
	}
	if len(scope.Allowed.CIDRs) != 1 || len(scope.Allowed.Domains) != 1 {
		t.Fatalf("unexpected scope: %#v", scope.Allowed)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func TestRunWebScanWithNucleiSafeMissingDependencySkipsPlugin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
	}))
	defer server.Close()

	t.Setenv("PATH", t.TempDir())
	out := filepath.Join(t.TempDir(), "run_with_plugin")
	result, err := RunWebScan(context.Background(), WebScanRequest{Target: server.URL, Scope: server.URL, Out: out, Profile: "safe", EnabledPlugins: []string{"nuclei-safe"}})
	if err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}
	if result.Job.Status != contracts.StatusSucceeded {
		t.Fatalf("job status = %s, want succeeded", result.Job.Status)
	}

	var planHasNuclei bool
	for _, stepID := range result.Plan.Steps {
		if stepID == "step_optional_nuclei_safe_001" {
			planHasNuclei = true
			break
		}
	}
	if !planHasNuclei {
		t.Fatalf("plan missing optional nuclei-safe step: %#v", result.Plan.Steps)
	}

	calls, err := command.NewRecorder(filepath.Join(out, "commands.log")).ReadAll()
	if err != nil {
		t.Fatalf("read commands.log: %v", err)
	}
	var pluginCall *contracts.ToolCall
	for i := range calls {
		if calls[i].ToolName == "plugin.nuclei-safe" {
			pluginCall = &calls[i]
			break
		}
	}
	if pluginCall == nil {
		t.Fatalf("commands.log missing plugin.nuclei-safe call: %#v", calls)
	}
	if pluginCall.Kind != contracts.ToolCallKindPlugin {
		t.Fatalf("plugin call kind = %s, want plugin: %#v", pluginCall.Kind, *pluginCall)
	}
	if pluginCall.Status != contracts.StatusSkipped {
		t.Fatalf("plugin call status = %s, want skipped: %#v", pluginCall.Status, *pluginCall)
	}
	if !strings.Contains(pluginCall.Error, "dependency nuclei not found") {
		t.Fatalf("plugin call error = %q", pluginCall.Error)
	}
}

func TestRunWebScanWithFFUFLowRateRequiresApprovalSkipsPlugin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "run_with_ffuf_plugin")
	result, err := RunWebScan(context.Background(), WebScanRequest{Target: server.URL, Scope: server.URL, Out: out, Profile: "safe", EnabledPlugins: []string{"ffuf-low-rate"}})
	if err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}
	if result.Job.Status != contracts.StatusSucceeded {
		t.Fatalf("job status = %s, want succeeded", result.Job.Status)
	}
	if !planHasStep(result.Plan, "step_optional_ffuf_low_rate_001") {
		t.Fatalf("plan missing optional ffuf-low-rate step: %#v", result.Plan.Steps)
	}

	calls, err := command.NewRecorder(filepath.Join(out, "commands.log")).ReadAll()
	if err != nil {
		t.Fatalf("read commands.log: %v", err)
	}
	pluginCall := findToolCall(calls, "plugin.ffuf-low-rate")
	if pluginCall == nil {
		t.Fatalf("commands.log missing plugin.ffuf-low-rate call: %#v", calls)
	}
	if pluginCall.Kind != contracts.ToolCallKindPlugin {
		t.Fatalf("plugin call kind = %s, want plugin: %#v", pluginCall.Kind, *pluginCall)
	}
	if pluginCall.Status != contracts.StatusSkipped {
		t.Fatalf("plugin call status = %s, want skipped: %#v", pluginCall.Status, *pluginCall)
	}
	if !strings.Contains(pluginCall.Error, "step_requires_approval") {
		t.Fatalf("plugin call error = %q, want approval reason", pluginCall.Error)
	}
	if got := pluginCall.Metadata["policy_decision"]; got != string(contracts.PolicyRequireApproval) {
		t.Fatalf("plugin call policy_decision = %#v, want require_approval", got)
	}
	if got := pluginCall.Metadata["policy_decision_id"]; got != "policy_step_optional_ffuf_low_rate_001" {
		t.Fatalf("plugin call policy_decision_id = %#v", got)
	}
	if got := pluginCall.Metadata["approval_request_id"]; got != "approval_step_optional_ffuf_low_rate_001" {
		t.Fatalf("plugin call approval_request_id = %#v", got)
	}

	var approvals []contracts.HITLRequest
	readJSON(t, filepath.Join(out, "approvals.json"), &approvals)
	if len(approvals) != 1 {
		t.Fatalf("approvals.json len = %d, want 1: %#v", len(approvals), approvals)
	}
	approval := approvals[0]
	if approval.ID != "approval_step_optional_ffuf_low_rate_001" ||
		approval.Status != contracts.HITLPending ||
		approval.PolicyDecisionID != "policy_step_optional_ffuf_low_rate_001" ||
		approval.RequestedAction != "execute_step" {
		t.Fatalf("unexpected approval request: %#v", approval)
	}
	if approval.RiskSummary["tool"] != "plugin.ffuf-low-rate" {
		t.Fatalf("approval risk summary missing tool: %#v", approval.RiskSummary)
	}

	stateStore, err := state.Open(filepath.Join(out, "state", "bolt.db"))
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer stateStore.Close()
	steps, err := stateStore.ListSteps(result.Job.ID)
	if err != nil {
		t.Fatalf("ListSteps() error = %v", err)
	}
	var ffufStep *contracts.Step
	for i := range steps {
		if steps[i].ID == "step_optional_ffuf_low_rate_001" {
			ffufStep = &steps[i]
			break
		}
	}
	if ffufStep == nil {
		t.Fatalf("persisted state missing ffuf step: %#v", steps)
	}
	if ffufStep.Status != contracts.StatusSkipped || !ffufStep.RequiresApproval || ffufStep.Risk != contracts.RiskMedium {
		t.Fatalf("unexpected persisted ffuf step: %#v", *ffufStep)
	}
	decision, err := stateStore.LoadPolicyDecision("policy_step_optional_ffuf_low_rate_001")
	if err != nil {
		t.Fatalf("LoadPolicyDecision() error = %v", err)
	}
	if decision.Decision != contracts.PolicyRequireApproval || decision.StepID != "step_optional_ffuf_low_rate_001" {
		t.Fatalf("unexpected persisted policy decision: %#v", decision)
	}
	persistedApproval, err := stateStore.LoadHITLRequest("approval_step_optional_ffuf_low_rate_001")
	if err != nil {
		t.Fatalf("LoadHITLRequest() error = %v", err)
	}
	if persistedApproval.Status != contracts.HITLPending || persistedApproval.PolicyDecisionID != decision.ID {
		t.Fatalf("unexpected persisted HITL request: %#v", persistedApproval)
	}

	eventStore := events.NewStore(filepath.Join(out, "events.jsonl"))
	allEvents, err := eventStore.ReadAll()
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if !hasEvent(allEvents, contracts.EventApprovalRequested, "step_optional_ffuf_low_rate_001") {
		t.Fatalf("events missing approval.requested: %#v", allEvents)
	}

	report, err := os.ReadFile(filepath.Join(out, "reports", "report.md"))
	if err != nil {
		t.Fatalf("read report.md: %v", err)
	}
	if !strings.Contains(string(report), "step_optional_ffuf_low_rate_001") {
		t.Fatalf("report.md missing skipped ffuf step: %s", report)
	}
}

func TestRunWebScanWithPythonJSRouteAnalyzerPlugin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><head><script src="/app.js"></script></head><body>ok</body></html>`))
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch('/api/v1/users');`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "run_with_python_plugin")
	result, err := RunWebScan(context.Background(), WebScanRequest{Target: server.URL, Scope: server.URL, Out: out, Profile: "safe", EnabledPlugins: []string{"python-js-route-analyzer"}})
	if err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}
	if result.Job.Status != contracts.StatusSucceeded {
		t.Fatalf("job status = %s, want succeeded", result.Job.Status)
	}

	if !planHasStep(result.Plan, "step_optional_python_js_route_analyzer_001") {
		t.Fatalf("plan missing optional python-js-route-analyzer step: %#v", result.Plan.Steps)
	}

	calls, err := command.NewRecorder(filepath.Join(out, "commands.log")).ReadAll()
	if err != nil {
		t.Fatalf("read commands.log: %v", err)
	}
	pluginCall := findToolCall(calls, "plugin.python-js-route-analyzer")
	if pluginCall == nil {
		t.Fatalf("commands.log missing plugin.python-js-route-analyzer call: %#v", calls)
	}
	if pluginCall.Kind != contracts.ToolCallKindPlugin {
		t.Fatalf("plugin call kind = %s, want plugin: %#v", pluginCall.Kind, *pluginCall)
	}
	if pluginCall.Status != contracts.StatusSucceeded {
		t.Fatalf("plugin call status = %s, want succeeded: %#v", pluginCall.Status, *pluginCall)
	}

	stdoutPath := filepath.Join(out, "artifacts", "tool-results", "python-js-route-analyzer.jsonl")
	mustExist(t, stdoutPath)
	stdout, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read plugin stdout artifact: %v", err)
	}
	if !strings.Contains(string(stdout), pluginCall.ID) {
		t.Fatalf("plugin stdout artifact missing request id %s: %s", pluginCall.ID, stdout)
	}
	if !hasArtifact(result.Artifacts, "artifact_python_js_route_analyzer_stdout_001") {
		t.Fatalf("result artifacts missing python plugin stdout artifact: %#v", result.Artifacts)
	}

	if !strings.Contains(string(stdout), `"route_count":1`) {
		t.Fatalf("plugin stdout artifact missing route_count=1: %s", stdout)
	}

	var foundBuiltinJSRoute bool
	var foundPluginJSRoute bool
	for _, evidence := range result.Evidence {
		if evidence.Type != contracts.EvidenceJSRoute || !strings.Contains(evidence.TargetRef, "/api/v1/users") {
			continue
		}
		if evidence.StepID == "step_js_route_extract_light_001" {
			foundBuiltinJSRoute = true
		}
		if evidence.StepID == "step_optional_python_js_route_analyzer_001" && evidence.ArtifactID == "artifact_python_js_route_analyzer_stdout_001" {
			foundPluginJSRoute = true
		}
	}
	if !foundBuiltinJSRoute {
		t.Fatalf("expected builtin JS route evidence to survive plugin execution: %#v", result.Evidence)
	}
	if !foundPluginJSRoute {
		t.Fatalf("expected plugin JS route evidence from python-js-route-analyzer: %#v", result.Evidence)
	}
}

func planHasStep(plan contracts.Plan, stepID string) bool {
	for _, id := range plan.Steps {
		if id == stepID {
			return true
		}
	}
	return false
}

func findToolCall(calls []contracts.ToolCall, toolName string) *contracts.ToolCall {
	for i := range calls {
		if calls[i].ToolName == toolName {
			return &calls[i]
		}
	}
	return nil
}

func hasArtifact(artifacts []contracts.Artifact, id string) bool {
	for _, artifact := range artifacts {
		if artifact.ID == id {
			return true
		}
	}
	return false
}

func hasEvent(all []contracts.Event, typ contracts.EventType, stepID string) bool {
	for _, event := range all {
		if event.Type == typ && event.StepID == stepID {
			return true
		}
	}
	return false
}
