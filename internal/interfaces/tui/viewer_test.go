package tui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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
	if got := renderReport(""); !strings.Contains(got, "report.md not found") {
		t.Fatalf("renderReport(empty) = %q", got)
	}
	if got := tailEvents(nil, 100); len(got) != 0 {
		t.Fatalf("tailEvents(nil) len = %d, want 0", len(got))
	}
}
