package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
)

func TestRunServiceReadsGeneratedRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer server.Close()

	runsDir := t.TempDir()
	runID := "run_001"
	runDir := filepath.Join(runsDir, runID)
	result, err := coreruntime.RunWebScan(context.Background(), coreruntime.WebScanRequest{
		Target:          server.URL,
		Scope:           server.URL,
		Out:             runDir,
		Profile:         "safe",
		NoExternalTools: true,
		BuildInfo:       coreruntime.BuildInfo{Version: "v0.1.2-test", Commit: "abc123", Date: "2026-05-24T00:00:00Z"},
		CommandName:     "scan web",
		CommandArgs:     []string{"scan", "web", "--target", server.URL, "--scope", server.URL},
	})
	if err != nil {
		t.Fatalf("RunWebScan() error = %v", err)
	}

	service := NewRunService(runsDir)
	summary, err := service.GetRunSummary(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRunSummary() error = %v", err)
	}
	if summary.RunID != runID || summary.RunDir != runDir {
		t.Fatalf("unexpected summary identity: %#v", summary)
	}
	if summary.Target != server.URL || summary.Scope != server.URL || summary.Profile != "safe" {
		t.Fatalf("unexpected summary target/scope/profile: %#v", summary)
	}
	if summary.Status != result.Job.Status {
		t.Fatalf("Status = %s, want %s", summary.Status, result.Job.Status)
	}
	if !summary.Report.JSON || !summary.Report.Markdown {
		t.Fatalf("expected both reports available: %#v", summary.Report)
	}
	if summary.Counts.Events == 0 || summary.Counts.Artifacts == 0 || summary.Counts.Findings == 0 {
		t.Fatalf("expected non-zero counts: %#v", summary.Counts)
	}

	detail, err := service.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if detail.Metadata == nil || detail.Metadata.Binary.Version != "v0.1.2-test" {
		t.Fatalf("metadata not loaded: %#v", detail.Metadata)
	}
	if detail.EffectiveConfig == nil || detail.EffectiveConfig.Target != server.URL {
		t.Fatalf("effective config not loaded: %#v", detail.EffectiveConfig)
	}
	if detail.Job == nil || detail.Job.ID != result.Job.ID {
		t.Fatalf("job not loaded: %#v", detail.Job)
	}

	page, err := service.ListRuns(context.Background(), RunListRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(page.Items) != 1 || !page.EOF || page.Items[0].RunID != runID {
		t.Fatalf("unexpected run page: %#v", page)
	}

	report, err := service.GetReport(context.Background(), runID, "json")
	if err != nil {
		t.Fatalf("GetReport(json) error = %v", err)
	}
	if report.Format != "json" || report.ContentType != "application/json" || !json.Valid(report.Bytes) {
		t.Fatalf("unexpected JSON report: format=%s type=%s bytes=%q", report.Format, report.ContentType, report.Bytes[:min(len(report.Bytes), 60)])
	}
	md, err := service.GetReport(context.Background(), runID, "md")
	if err != nil {
		t.Fatalf("GetReport(md) error = %v", err)
	}
	if md.Format != "md" || md.ContentType == "" || len(md.Bytes) == 0 {
		t.Fatalf("unexpected markdown report: %#v", md)
	}

	artifactIndex, err := service.GetArtifactIndex(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetArtifactIndex() error = %v", err)
	}
	if artifactIndex.Count != len(artifactIndex.Artifacts) || artifactIndex.Count == 0 {
		t.Fatalf("unexpected artifact index: %#v", artifactIndex)
	}

	events, err := service.GetEvents(context.Background(), runID, "", 2)
	if err != nil {
		t.Fatalf("GetEvents(first) error = %v", err)
	}
	if len(events.Items) != 2 || events.NextCursor == "" || events.EOF {
		t.Fatalf("unexpected first events page: %#v", events)
	}
	next, err := service.GetEvents(context.Background(), runID, events.NextCursor, 1000)
	if err != nil {
		t.Fatalf("GetEvents(next) error = %v", err)
	}
	if len(next.Items) == 0 || !next.EOF {
		t.Fatalf("unexpected next events page: %#v", next)
	}
}

func TestRunServiceRejectsTraversalAndMissingRun(t *testing.T) {
	service := NewRunService(t.TempDir())
	_, err := service.GetRunSummary(context.Background(), "../secret")
	if err == nil || ErrorCodeOf(err) != CodeBadRequest {
		t.Fatalf("traversal error code = %s, err=%v", ErrorCodeOf(err), err)
	}

	_, err = service.GetRunSummary(context.Background(), "missing")
	if err == nil || ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("missing error code = %s, err=%v", ErrorCodeOf(err), err)
	}
}

func TestRunServiceIgnoresInternalAgentStateDirectory(t *testing.T) {
	runsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(runsDir, ".agent", "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir agent sessions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, ".agent", "sessions", "agent_001.json"), []byte(`{"session_id":"agent_001"}`), 0o644); err != nil {
		t.Fatalf("write agent session: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(runsDir, "scratch", "reports"), 0o755); err != nil {
		t.Fatalf("mkdir scratch dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "scratch", "notes.txt"), []byte("not a run workspace"), 0o644); err != nil {
		t.Fatalf("write scratch file: %v", err)
	}

	service := NewRunService(runsDir)
	page, err := service.ListRuns(context.Background(), RunListRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(page.Items) != 0 || !page.EOF {
		t.Fatalf("internal/non-run directories must not be listed as runs: %#v", page)
	}

	_, err = service.GetRunSummary(context.Background(), ".agent")
	if err == nil || ErrorCodeOf(err) != CodeBadRequest {
		t.Fatalf("hidden internal run id error code = %s, err=%v", ErrorCodeOf(err), err)
	}
}

func TestRunServiceReportErrors(t *testing.T) {
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run_001")
	if err := os.MkdirAll(filepath.Join(runDir, "reports"), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	service := NewRunService(runsDir)
	_, err := service.GetReport(context.Background(), "run_001", "pdf")
	if err == nil || ErrorCodeOf(err) != CodeBadRequest {
		t.Fatalf("format error code = %s, err=%v", ErrorCodeOf(err), err)
	}
	_, err = service.GetReport(context.Background(), "run_001", "json")
	if err == nil || ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("missing report code = %s, err=%v", ErrorCodeOf(err), err)
	}
}

func TestServiceErrorCodeOf(t *testing.T) {
	err := NewError(CodeConflict, "conflict", nil, errors.New("busy"))
	if got := ErrorCodeOf(err); got != CodeConflict {
		t.Fatalf("ErrorCodeOf(service) = %s, want %s", got, CodeConflict)
	}
	if got := ErrorCodeOf(errors.New("plain")); got != CodeInternal {
		t.Fatalf("ErrorCodeOf(plain) = %s, want %s", got, CodeInternal)
	}
	if got := ErrorCodeOf(nil); got != "" {
		t.Fatalf("ErrorCodeOf(nil) = %s, want empty", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
