package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
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

func TestRunServiceRebuildsRunIndexFromRunsDir(t *testing.T) {
	runsDir := t.TempDir()
	writeRunIndexTestWorkspace(t, runsDir, "run_older", time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC), contracts.StatusSucceeded, "https://older.example")
	writeRunIndexTestWorkspace(t, runsDir, "run_newer", time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC), contracts.StatusFailed, "https://newer.example")

	service := NewRunService(runsDir)
	index, err := service.RebuildIndex(context.Background())
	if err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	if index.SchemaVersion != runIndexSchemaVersion || index.Source != runIndexSourceRunsDir || index.RunsDir != runsDir {
		t.Fatalf("unexpected index metadata: %#v", index)
	}
	if index.Count != 2 || len(index.Items) != 2 {
		t.Fatalf("unexpected index count/items: %#v", index)
	}
	if index.Items[0].RunID != "run_newer" || index.Items[1].RunID != "run_older" {
		t.Fatalf("index should preserve ListRuns ordering, got %#v", index.Items)
	}

	indexPath := filepath.Join(runsDir, ".index", "runs.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("expected run index file at %s: %v", indexPath, err)
	}
	var persisted RunIndex
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode persisted index: %v", err)
	}
	if persisted.Count != index.Count || len(persisted.Items) != len(index.Items) {
		t.Fatalf("persisted index mismatch: %#v", persisted)
	}

	loaded, err := service.LoadIndex(context.Background())
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if loaded.Count != 2 || len(loaded.Items) != 2 {
		t.Fatalf("unexpected loaded index: %#v", loaded)
	}
	validation, err := service.ValidateIndex(context.Background())
	if err != nil {
		t.Fatalf("ValidateIndex() error = %v", err)
	}
	if !validation.Valid || validation.IndexCount != 2 || validation.ScannedCount != 2 {
		t.Fatalf("expected valid rebuilt index: %#v", validation)
	}

	page, err := service.ListRuns(context.Background(), RunListRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(page.Items) != 2 || page.Items[0].RunID == ".index" {
		t.Fatalf("run index directory must not be listed as a run: %#v", page)
	}
}

func TestRunServiceValidateRunIndexDetectsMissingExtraAndStale(t *testing.T) {
	runsDir := t.TempDir()
	writeRunIndexTestWorkspace(t, runsDir, "run_keep", time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC), contracts.StatusSucceeded, "https://keep.example")
	writeRunIndexTestWorkspace(t, runsDir, "run_deleted", time.Date(2026, 6, 9, 8, 0, 0, 0, time.UTC), contracts.StatusSucceeded, "https://deleted.example")

	service := NewRunService(runsDir)
	if _, err := service.RebuildIndex(context.Background()); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(runsDir, "run_deleted")); err != nil {
		t.Fatalf("remove run workspace: %v", err)
	}
	writeRunIndexTestWorkspace(t, runsDir, "run_added", time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC), contracts.StatusSucceeded, "https://added.example")
	updateRunIndexTestJobStatus(t, runsDir, "run_keep", contracts.StatusFailed)

	validation, err := service.ValidateIndex(context.Background())
	if err != nil {
		t.Fatalf("ValidateIndex() error = %v", err)
	}
	if validation.Valid {
		t.Fatalf("validation should fail when source of truth diverges: %#v", validation)
	}
	if !reflect.DeepEqual(validation.MissingFromIndex, []string{"run_added"}) {
		t.Fatalf("MissingFromIndex = %#v, want run_added", validation.MissingFromIndex)
	}
	if !reflect.DeepEqual(validation.ExtraInIndex, []string{"run_deleted"}) {
		t.Fatalf("ExtraInIndex = %#v, want run_deleted", validation.ExtraInIndex)
	}
	if len(validation.Stale) != 1 || validation.Stale[0].RunID != "run_keep" {
		t.Fatalf("Stale = %#v, want run_keep", validation.Stale)
	}
	if validation.Stale[0].Indexed.Status != contracts.StatusSucceeded || validation.Stale[0].Current.Status != contracts.StatusFailed {
		t.Fatalf("stale status diff not captured: %#v", validation.Stale[0])
	}
}

func TestRunServiceRunIndexIsCacheAndRunsDirIsSourceOfTruth(t *testing.T) {
	runsDir := t.TempDir()
	writeRunIndexTestWorkspace(t, runsDir, "run_source", time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC), contracts.StatusSucceeded, "https://source.example")

	service := NewRunService(runsDir)
	if _, err := service.RebuildIndex(context.Background()); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(runsDir, ".index")); err != nil {
		t.Fatalf("remove index cache: %v", err)
	}
	if _, err := service.LoadIndex(context.Background()); err == nil || ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("LoadIndex() after cache delete error code = %s, err=%v", ErrorCodeOf(err), err)
	}

	page, err := service.ListRuns(context.Background(), RunListRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRuns() should still use runs dir source of truth: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].RunID != "run_source" {
		t.Fatalf("unexpected source-of-truth page after index delete: %#v", page)
	}
	recreated, err := service.RebuildIndex(context.Background())
	if err != nil {
		t.Fatalf("RebuildIndex() after cache delete error = %v", err)
	}
	if recreated.Count != 1 || len(recreated.Items) != 1 || recreated.Items[0].RunID != "run_source" {
		t.Fatalf("unexpected recreated index: %#v", recreated)
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

func writeRunIndexTestWorkspace(t *testing.T, runsDir, runID string, created time.Time, status contracts.Status, target string) {
	t.Helper()
	runDir := filepath.Join(runsDir, runID)
	for _, dir := range []string{
		runDir,
		filepath.Join(runDir, "artifacts"),
		filepath.Join(runDir, "reports"),
		filepath.Join(runDir, "state"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	job := contracts.Job{
		ID:        runID,
		Kind:      contracts.JobKindWebScan,
		Status:    status,
		TargetID:  "target_" + runID,
		ScopeID:   "scope_" + runID,
		Profile:   "safe",
		Workspace: runDir,
		CreatedAt: created,
	}
	writeRunIndexTestJSON(t, filepath.Join(runDir, "job.json"), job)
	writeRunIndexTestJSON(t, filepath.Join(runDir, "metadata.json"), map[string]any{
		"schema_version": "test",
		"created_at":     created,
		"job_id":         runID,
		"workspace":      runDir,
		"profile":        "safe",
		"request": map[string]any{
			"target": target,
			"scope":  target + "/scope",
		},
	})
	writeRunIndexTestJSON(t, filepath.Join(runDir, "effective_config.json"), map[string]any{
		"schema_version": "test",
		"generated_at":   created,
		"profile":        "safe",
		"target":         target,
		"scope":          target + "/scope",
		"out":            runDir,
		"job_id":         runID,
	})
	if err := os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte("{\"id\":\"event_1\"}\n{\"id\":\"event_2\"}\n"), 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}
	writeRunIndexTestJSON(t, filepath.Join(runDir, "artifacts", "index.json"), map[string]any{"schema_version": "test", "count": 1})
	writeRunIndexTestJSON(t, filepath.Join(runDir, "findings.json"), []map[string]any{{"id": "finding_1"}})
	writeRunIndexTestJSON(t, filepath.Join(runDir, "approvals.json"), []map[string]any{})
	if err := os.WriteFile(filepath.Join(runDir, "reports", "report.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write report json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "reports", "report.md"), []byte("# report\n"), 0o644); err != nil {
		t.Fatalf("write report md: %v", err)
	}
}

func updateRunIndexTestJobStatus(t *testing.T, runsDir, runID string, status contracts.Status) {
	t.Helper()
	path := filepath.Join(runsDir, runID, "job.json")
	var job contracts.Job
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read job: %v", err)
	}
	if err := json.Unmarshal(data, &job); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	job.Status = status
	writeRunIndexTestJSON(t, path, job)
}

func writeRunIndexTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
