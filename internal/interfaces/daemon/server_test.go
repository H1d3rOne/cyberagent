package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

func TestNewServerCreatesTokenFileAndNormalizesDefaults(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "config", "token")
	server, err := NewServer(Config{TokenFile: tokenFile})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	if server.Addr() != DefaultListen {
		t.Fatalf("Addr() = %q, want %q", server.Addr(), DefaultListen)
	}
	if server.TokenFile() != tokenFile {
		t.Fatalf("TokenFile() = %q, want %q", server.TokenFile(), tokenFile)
	}
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if len(data) < 32 {
		t.Fatalf("token file too short: %q", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(tokenFile)
		if err != nil {
			t.Fatalf("stat token file: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("token file permissions = %o, want 600", got)
		}
	}
}

func TestHTTPAuthEnvelopeHealthAndVersion(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("test-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	server, err := NewServer(Config{
		TokenFile: tokenFile,
		Build: services.BuildInfo{
			Name:    "CyberAgent",
			Version: "vdaemon-test",
			Commit:  "abc123",
			Date:    "2026-05-24T00:00:00Z",
			Module:  "github.com/H1d3rOne/cyberagent",
		},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	noAuth := doDaemonRequest(t, httpServer.URL+"/api/v1/version", "")
	if noAuth.StatusCode != http.StatusUnauthorized || noAuth.Body.OK || noAuth.Body.Error.Code != services.CodeUnauthorized {
		t.Fatalf("unexpected no-auth response: status=%d body=%#v", noAuth.StatusCode, noAuth.Body)
	}

	badAuth := doDaemonRequest(t, httpServer.URL+"/api/v1/version", "wrong-token")
	if badAuth.StatusCode != http.StatusUnauthorized || badAuth.Body.Error.Code != services.CodeUnauthorized {
		t.Fatalf("unexpected bad-auth response: status=%d body=%#v", badAuth.StatusCode, badAuth.Body)
	}

	version := doDaemonRequest(t, httpServer.URL+"/api/v1/version", "test-token")
	if version.StatusCode != http.StatusOK || !version.Body.OK || version.Body.Error != nil {
		t.Fatalf("unexpected version response: status=%d body=%#v", version.StatusCode, version.Body)
	}
	if version.Body.Data["version"] != "vdaemon-test" || version.Body.Data["commit"] != "abc123" {
		t.Fatalf("unexpected version data: %#v", version.Body.Data)
	}

	health := doDaemonRequest(t, httpServer.URL+"/api/v1/healthz", "test-token")
	if health.StatusCode != http.StatusOK || !health.Body.OK || health.Body.Data["status"] != "ok" {
		t.Fatalf("unexpected health response: status=%d body=%#v", health.StatusCode, health.Body)
	}

	missing := doDaemonRequest(t, httpServer.URL+"/api/v1/missing", "test-token")
	if missing.StatusCode != http.StatusNotFound || missing.Body.OK || missing.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("unexpected missing response: status=%d body=%#v", missing.StatusCode, missing.Body)
	}
}

func TestHTTPReadOnlyAPIs(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><head><script src="/app.js"></script></head><body><a href="/admin">Admin</a></body></html>`))
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch("/api/status")`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte("<urlset></urlset>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	runID := "run_readonly"
	if _, err := coreruntime.RunWebScan(context.Background(), coreruntime.WebScanRequest{
		Target:          target.URL,
		Scope:           target.URL,
		Out:             filepath.Join(runsDir, runID),
		Profile:         "safe",
		NoExternalTools: true,
		BuildInfo:       coreruntime.BuildInfo{Version: "v0.1.2-test", Commit: "abc123", Date: "2026-05-24T00:00:00Z", Module: "github.com/H1d3rOne/cyberagent"},
		CommandName:     "scan web",
		CommandArgs:     []string{"scan", "web", "--target", target.URL, "--scope", target.URL, "--no-external-tools"},
	}); err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}

	pluginRoot := filepath.Join(root, "plugins")
	writeDaemonTestPlugin(t, pluginRoot)

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vread-api"}),
		plugins:   services.NewPluginService(pluginRoot),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	doctor := doDaemonRequest(t, api.URL+"/api/v1/doctor", "test-token")
	if doctor.StatusCode != http.StatusOK || !doctor.Body.OK || doctor.Body.Data["checks"] == nil {
		t.Fatalf("unexpected doctor response: status=%d body=%#v", doctor.StatusCode, doctor.Body)
	}

	plugins := doDaemonRequest(t, api.URL+"/api/v1/plugins", "test-token")
	if plugins.StatusCode != http.StatusOK || !plugins.Body.OK || len(asList(t, plugins.Body.Data["plugins"])) != 1 {
		t.Fatalf("unexpected plugins response: status=%d body=%#v", plugins.StatusCode, plugins.Body)
	}
	pluginDoctor := doDaemonRequest(t, api.URL+"/api/v1/plugins/doctor", "test-token")
	if pluginDoctor.StatusCode != http.StatusOK || !pluginDoctor.Body.OK || pluginDoctor.Body.Data["count"].(float64) != 1 {
		t.Fatalf("unexpected plugins doctor response: status=%d body=%#v", pluginDoctor.StatusCode, pluginDoctor.Body)
	}

	runs := doDaemonRequest(t, api.URL+"/api/v1/runs?limit=10", "test-token")
	if runs.StatusCode != http.StatusOK || !runs.Body.OK {
		t.Fatalf("unexpected runs response: status=%d body=%#v", runs.StatusCode, runs.Body)
	}
	items := asList(t, runs.Body.Data["items"])
	if len(items) != 1 || asMap(t, items[0])["run_id"] != runID {
		t.Fatalf("unexpected runs items: %#v", items)
	}

	detail := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID, "test-token")
	if detail.StatusCode != http.StatusOK || !detail.Body.OK || detail.Body.Data["run_id"] != runID {
		t.Fatalf("unexpected run detail response: status=%d body=%#v", detail.StatusCode, detail.Body)
	}

	reportJSON := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/report?format=json", "test-token")
	if reportJSON.StatusCode != http.StatusOK || !reportJSON.Body.OK || reportJSON.Body.Data["format"] != "json" || reportJSON.Body.Data["report"] == nil {
		t.Fatalf("unexpected JSON report response: status=%d body=%#v", reportJSON.StatusCode, reportJSON.Body)
	}

	reportMD := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/report?format=md", "test-token")
	if reportMD.StatusCode != http.StatusOK || !reportMD.Body.OK || reportMD.Body.Data["format"] != "md" || !strings.Contains(reportMD.Body.Data["content"].(string), "CyberAgent Report") {
		t.Fatalf("unexpected markdown report response: status=%d body=%#v", reportMD.StatusCode, reportMD.Body)
	}

	artifacts := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/artifacts", "test-token")
	if artifacts.StatusCode != http.StatusOK || !artifacts.Body.OK || artifacts.Body.Data["count"].(float64) <= 0 {
		t.Fatalf("unexpected artifacts response: status=%d body=%#v", artifacts.StatusCode, artifacts.Body)
	}

	events := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/events?limit=2", "test-token")
	if events.StatusCode != http.StatusOK || !events.Body.OK || len(asList(t, events.Body.Data["items"])) != 2 || events.Body.Data["next_cursor"] == "" {
		t.Fatalf("unexpected events response: status=%d body=%#v", events.StatusCode, events.Body)
	}

	approvals := doDaemonRequest(t, api.URL+"/api/v1/runs/"+runID+"/approvals", "test-token")
	if approvals.StatusCode != http.StatusOK || !approvals.Body.OK || len(asList(t, approvals.Body.Data["approvals"])) != 0 {
		t.Fatalf("unexpected approvals response: status=%d body=%#v", approvals.StatusCode, approvals.Body)
	}

	traversal := doDaemonRequest(t, api.URL+"/api/v1/runs/../secret", "test-token")
	if traversal.StatusCode != http.StatusNotFound && traversal.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected traversal response: status=%d body=%#v", traversal.StatusCode, traversal.Body)
	}

	badLimit := doDaemonRequest(t, api.URL+"/api/v1/runs?limit=bad", "test-token")
	if badLimit.StatusCode != http.StatusBadRequest || badLimit.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected bad limit response: status=%d body=%#v", badLimit.StatusCode, badLimit.Body)
	}
}

func TestHTTPApprovalWriteAPIs(t *testing.T) {
	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	writeDaemonApprovalRun(t, runsDir, "run_approval_write", []contracts.HITLRequest{
		daemonTestApproval("approval_001", "job_001", "step_001"),
		daemonTestApproval("approval_002", "job_001", "step_002"),
	})

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vapproval-api"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	approve := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_write/approvals/approval_001/approve", "test-token", `{"operator":"local-user","comment":"approved from http"}`)
	if approve.StatusCode != http.StatusOK || !approve.Body.OK || approve.Body.Data["status"] != string(contracts.HITLApproved) {
		t.Fatalf("unexpected approve response: status=%d body=%#v", approve.StatusCode, approve.Body)
	}
	approveResolution := asMap(t, approve.Body.Data["resolution"])
	if approveResolution["resolved_by"] != "local-user" || approveResolution["comment"] != "approved from http" {
		t.Fatalf("unexpected approve resolution: %#v", approveResolution)
	}

	reapprove := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_write/approvals/approval_001/approve", "test-token", `{"operator":"local-user"}`)
	if reapprove.StatusCode != http.StatusConflict || reapprove.Body.OK || reapprove.Body.Error.Code != services.CodeConflict {
		t.Fatalf("unexpected re-approve response: status=%d body=%#v", reapprove.StatusCode, reapprove.Body)
	}

	deny := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_write/approvals/approval_002/deny", "test-token", `{"operator":"local-user","comment":"not needed"}`)
	if deny.StatusCode != http.StatusOK || !deny.Body.OK || deny.Body.Data["status"] != string(contracts.HITLRejected) {
		t.Fatalf("unexpected deny response: status=%d body=%#v", deny.StatusCode, deny.Body)
	}
	denyResolution := asMap(t, deny.Body.Data["resolution"])
	if denyResolution["resolved_by"] != "local-user" || denyResolution["comment"] != "not needed" {
		t.Fatalf("unexpected deny resolution: %#v", denyResolution)
	}

	list := doDaemonRequest(t, api.URL+"/api/v1/runs/run_approval_write/approvals", "test-token")
	if list.StatusCode != http.StatusOK || !list.Body.OK {
		t.Fatalf("unexpected approval list response: status=%d body=%#v", list.StatusCode, list.Body)
	}
	approvals := asList(t, list.Body.Data["approvals"])
	if len(approvals) != 2 || asMap(t, approvals[0])["status"] != string(contracts.HITLApproved) || asMap(t, approvals[1])["status"] != string(contracts.HITLRejected) {
		t.Fatalf("unexpected persisted approvals: %#v", approvals)
	}

	missing := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_write/approvals/missing/approve", "test-token", `{}`)
	if missing.StatusCode != http.StatusNotFound || missing.Body.OK || missing.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("unexpected missing approval response: status=%d body=%#v", missing.StatusCode, missing.Body)
	}

	badJSON := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/run_approval_write/approvals/missing/approve", "test-token", `{"operator":`)
	if badJSON.StatusCode != http.StatusBadRequest || badJSON.Body.OK || badJSON.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected bad JSON response: status=%d body=%#v", badJSON.StatusCode, badJSON.Body)
	}

	traversal := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/../secret/approvals/approval_001/approve", "test-token", `{}`)
	if traversal.StatusCode != http.StatusNotFound && traversal.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected traversal response: status=%d body=%#v", traversal.StatusCode, traversal.Body)
	}

	wrongMethod := doDaemonRequestWithBody(t, http.MethodDelete, api.URL+"/api/v1/runs/run_approval_write/approvals/approval_001/approve", "test-token", `{}`)
	if wrongMethod.StatusCode != http.StatusBadRequest || wrongMethod.Body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected wrong method response: status=%d body=%#v", wrongMethod.StatusCode, wrongMethod.Body)
	}
}

func TestHTTPResumeApprovedAPI(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: *\n"))
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte("<urlset></urlset>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	root := t.TempDir()
	runsDir := filepath.Join(root, "runs")
	runID := "run_resume_http"
	runDir := filepath.Join(runsDir, runID)
	if _, err := coreruntime.RunWebScan(context.Background(), coreruntime.WebScanRequest{
		Target:          target.URL,
		Scope:           target.URL,
		Out:             runDir,
		Profile:         "safe",
		NoExternalTools: true,
		BuildInfo:       coreruntime.BuildInfo{Version: "v0.1.2-test"},
		CommandName:     "scan web",
	}); err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}

	handler := newRouter(routerServices{
		version:   services.NewVersionService(services.BuildInfo{Name: "CyberAgent", Version: "vresume-api"}),
		plugins:   services.NewPluginService(filepath.Join(root, "plugins")),
		runs:      services.NewRunService(runsDir),
		approvals: services.NewApprovalService(runsDir),
	}, "test-token")
	api := httptest.NewServer(handler)
	defer api.Close()

	resume := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/"+runID+"/resume-approved", "test-token", `{"operator":"local-user"}`)
	if resume.StatusCode != http.StatusOK || !resume.Body.OK || resume.Body.Data["run_id"] != runID || resume.Body.Data["run_dir"] != runDir {
		t.Fatalf("unexpected resume response: status=%d body=%#v", resume.StatusCode, resume.Body)
	}
	if len(asList(t, resume.Body.Data["resumed"])) != 0 || len(asList(t, resume.Body.Data["skipped"])) != 0 {
		t.Fatalf("unexpected resume result: %#v", resume.Body.Data)
	}

	missing := doDaemonRequestWithBody(t, http.MethodPost, api.URL+"/api/v1/runs/missing/resume-approved", "test-token", `{}`)
	if missing.StatusCode != http.StatusNotFound || missing.Body.OK || missing.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("unexpected missing resume response: status=%d body=%#v", missing.StatusCode, missing.Body)
	}
}

type daemonHTTPResponse struct {
	StatusCode int
	Body       daemonTestEnvelope
}

type daemonTestEnvelope struct {
	OK    bool           `json:"ok"`
	Data  map[string]any `json:"data"`
	Error *ErrorBody     `json:"error"`
}

func doDaemonRequest(t *testing.T, url, token string) daemonHTTPResponse {
	t.Helper()
	return doDaemonRequestWithBody(t, http.MethodGet, url, token, "")
}

func doDaemonRequestWithBody(t *testing.T, method, url, token, body string) daemonHTTPResponse {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	var decoded daemonTestEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return daemonHTTPResponse{StatusCode: resp.StatusCode, Body: decoded}
}

func writeDaemonApprovalRun(t *testing.T, root, runID string, requests []contracts.HITLRequest) {
	t.Helper()
	layout, err := runs.NewManager(root).Create(runID)
	if err != nil {
		t.Fatalf("create approval run: %v", err)
	}
	encoded, err := json.MarshalIndent(requests, "", "  ")
	if err != nil {
		t.Fatalf("marshal approvals: %v", err)
	}
	if err := os.WriteFile(layout.ApprovalsPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write approvals: %v", err)
	}
}

func daemonTestApproval(id, jobID, stepID string) contracts.HITLRequest {
	return contracts.HITLRequest{
		ID:               id,
		JobID:            jobID,
		StepID:           stepID,
		PolicyDecisionID: "policy_" + stepID,
		Status:           contracts.HITLPending,
		Reason:           "requires approval",
		RequestedAction:  "execute_step",
		Options:          []string{"approve", "deny"},
		CreatedAt:        contracts.Now(),
	}
}

func writeDaemonTestPlugin(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	manifest := `id: plugin_alpha
name: alpha
version: 0.1.0
type: tool
runtime: external_command
risk: low
requires_approval: false
dependencies:
  - name: definitely-missing-cyberagent-test-binary
    kind: binary
    required: false
entrypoint:
  kind: command_builder
  path: plugins/alpha/plugin.yaml
artifact_policy:
  store_stdout: true
  store_stderr: true
  redact: true
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
}

func asList(t *testing.T, value any) []any {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value is %T, want []any: %#v", value, value)
	}
	return items
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	item, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value is %T, want map[string]any: %#v", value, value)
	}
	return item
}
