package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

func TestRunServiceTenantContextIsReadableButDoesNotChangeRunsDir(t *testing.T) {
	runsDir := t.TempDir()
	writeRunIndexTestWorkspace(t, runsDir, "run_local", time.Date(2026, 6, 9, 11, 0, 0, 0, time.UTC), contracts.StatusSucceeded, "https://local.example")

	service := NewRunService(runsDir)
	ctx := tenantContextWithNonLocalScope()

	page, err := service.ListRuns(ctx, RunListRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListRuns() error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].RunID != "run_local" {
		t.Fatalf("ListRuns() with tenant context should still read configured runsDir: %#v", page)
	}
	if page.Items[0].RunDir != filepath.Join(runsDir, "run_local") {
		t.Fatalf("RunDir = %q, want configured runsDir path", page.Items[0].RunDir)
	}

	detail, err := service.GetRun(ctx, "run_local")
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if detail.RunDir != filepath.Join(runsDir, "run_local") || detail.RunID != "run_local" {
		t.Fatalf("GetRun() with tenant context changed run identity/path: %#v", detail)
	}
}

func TestRunIndexTenantContextIsReadableButCacheRemainsUnderRunsDir(t *testing.T) {
	runsDir := t.TempDir()
	writeRunIndexTestWorkspace(t, runsDir, "run_indexed", time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC), contracts.StatusSucceeded, "https://indexed.example")

	service := NewRunService(runsDir)
	ctx := tenantContextWithNonLocalScope()

	index, err := service.RebuildIndex(ctx)
	if err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	if index.RunsDir != runsDir || index.Count != 1 || index.Items[0].RunID != "run_indexed" {
		t.Fatalf("RebuildIndex() should stay rooted at configured runsDir: %#v", index)
	}

	loaded, err := service.LoadIndex(ctx)
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if loaded.RunsDir != runsDir || loaded.Count != 1 || loaded.Items[0].RunID != "run_indexed" {
		t.Fatalf("LoadIndex() should stay rooted at configured runsDir: %#v", loaded)
	}
}

func TestApprovalServiceTenantContextIsReadableButDoesNotChangeRunsDir(t *testing.T) {
	runsDir := t.TempDir()
	createApprovalRun(t, runsDir, "run_approvals_tenant", []contracts.HITLRequest{
		testApproval("approval_tenant", "job_tenant", "step_tenant"),
	})

	service := NewApprovalService(runsDir)
	requests, err := service.ListApprovals(tenantContextWithNonLocalScope(), "run_approvals_tenant")
	if err != nil {
		t.Fatalf("ListApprovals() error = %v", err)
	}
	if len(requests) != 1 || requests[0].ID != "approval_tenant" {
		t.Fatalf("ListApprovals() with tenant context should still read configured runsDir: %#v", requests)
	}
}

func tenantContextWithNonLocalScope() context.Context {
	return coretenant.WithContext(context.Background(), coretenant.TenantContext{
		TenantID:    "tenant-from-future",
		WorkspaceID: "workspace-from-future",
		Mode:        coretenant.ModeMultiUser,
		Principal: coretenant.Principal{
			SubjectID:   "analyst-1",
			DisplayName: "Analyst One",
			AuthMethod:  "test",
			Roles:       []string{"analyst"},
		},
	})
}
