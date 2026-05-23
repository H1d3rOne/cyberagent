package tui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	runtime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
)

func TestLoadRunLoadsGeneratedWebScanRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><head><script src="/app.js"></script></head><body><a href="/api/users">Users</a></body></html>`))
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch('/api/users');`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runDir := filepath.Join(t.TempDir(), "run_tui")
	result, err := runtime.RunWebScan(context.Background(), runtime.WebScanRequest{Target: server.URL, Scope: server.URL, Out: runDir, Profile: "safe", NoExternalTools: true})
	if err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}

	data, err := LoadRun(result.Workspace)
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if data.RunDir != result.Workspace {
		t.Fatalf("RunDir = %s, want %s", data.RunDir, result.Workspace)
	}
	if data.Job.ID != result.Job.ID {
		t.Fatalf("Job.ID = %s, want %s", data.Job.ID, result.Job.ID)
	}
	if data.Plan.ID != result.Plan.ID || len(data.Plan.Steps) == 0 {
		t.Fatalf("unexpected plan: %#v", data.Plan)
	}
	if len(data.Events) == 0 {
		t.Fatalf("expected events")
	}
	if len(data.Approvals) != 0 {
		t.Fatalf("approvals = %d, want 0 for no-external-tools scan", len(data.Approvals))
	}
	if len(data.Evidence) == 0 {
		t.Fatalf("expected evidence")
	}
	if len(data.Findings) == 0 {
		t.Fatalf("expected findings")
	}
	if !strings.Contains(data.Report, "CyberAgent Report") {
		t.Fatalf("report does not look loaded: %.120q", data.Report)
	}
}

func TestLoadRunLoadsApprovalRequestsReadOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
	}))
	defer server.Close()

	runDir := filepath.Join(t.TempDir(), "run_tui_approvals")
	result, err := runtime.RunWebScan(context.Background(), runtime.WebScanRequest{Target: server.URL, Scope: server.URL, Out: runDir, Profile: "safe", EnabledPlugins: []string{"ffuf-low-rate"}})
	if err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}

	data, err := LoadRun(result.Workspace)
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if len(data.Approvals) != 1 {
		t.Fatalf("approvals = %d, want 1: %#v", len(data.Approvals), data.Approvals)
	}
	approval := data.Approvals[0]
	if approval.Status != contracts.HITLPending || approval.StepID != "step_optional_ffuf_low_rate_001" {
		t.Fatalf("unexpected approval request: %#v", approval)
	}

	rendered := renderApprovals(data.Approvals)
	for _, want := range []string{"read-only", "approval_step_optional_ffuf_low_rate_001", "step_optional_ffuf_low_rate_001", "pending", "cyberagent approvals approve|deny"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("renderApprovals missing %q:\n%s", want, rendered)
		}
	}

	model := newModel(data)
	if len(model.sections) != 6 || model.sections[2] != "Approvals" {
		t.Fatalf("unexpected TUI sections: %#v", model.sections)
	}
	model.tab = 2
	view := model.View()
	if !strings.Contains(view, "Approvals") || !strings.Contains(view, "approval_step_optional_ffuf_low_rate_001") {
		t.Fatalf("TUI approvals view missing approval details:\n%s", view)
	}
}

func TestLoadRunReportsMissingFiles(t *testing.T) {
	_, err := LoadRun(t.TempDir())
	if err == nil {
		t.Fatalf("expected missing file error")
	}
	if !strings.Contains(err.Error(), "job.json") {
		t.Fatalf("error = %v, want job.json context", err)
	}
}

func TestRenderHelpersHandleEmptyInputs(t *testing.T) {
	if got := renderFindings(nil); !strings.Contains(got, "No findings") {
		t.Fatalf("renderFindings(nil) = %q", got)
	}
	if got := renderApprovals(nil); !strings.Contains(got, "No approval requests") {
		t.Fatalf("renderApprovals(nil) = %q", got)
	}
	if got := renderReport(""); !strings.Contains(got, "report.md not found") {
		t.Fatalf("renderReport(empty) = %q", got)
	}
	if got := tailEvents(nil, 100); len(got) != 0 {
		t.Fatalf("tailEvents(nil) len = %d, want 0", len(got))
	}
}
