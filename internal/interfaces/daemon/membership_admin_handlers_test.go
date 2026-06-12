package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

func TestMembershipAdminAPIDisabledByDefaultRequiresBearerAndReturnsNotFound(t *testing.T) {
	handler := newRouter(routerServices{}, "test-token")

	unauthorized := httptest.NewRecorder()
	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/memberships?scope=tenant&tenant_id=tenant-a&subject_id=user-1", nil)
	unauthorizedReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(unauthorized, unauthorizedReq)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d; body=%s", unauthorized.Code, http.StatusUnauthorized, unauthorized.Body.String())
	}

	disabled := httptest.NewRecorder()
	disabledReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/memberships?scope=tenant&tenant_id=tenant-a&subject_id=user-1", nil)
	disabledReq.Header.Set("Authorization", "Bearer test-token")
	disabledReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(disabled, disabledReq)
	if disabled.Code != http.StatusNotFound {
		t.Fatalf("disabled status = %d, want %d; body=%s", disabled.Code, http.StatusNotFound, disabled.Body.String())
	}
	body := decodeIdentityEnvelope(t, disabled.Body.Bytes())
	if body.Error == nil || body.Error.Code != services.CodeNotFound {
		t.Fatalf("unexpected disabled response: %#v", body.Error)
	}
	if strings.Contains(disabled.Body.String(), "test-token") {
		t.Fatalf("disabled response leaked bearer token: %s", disabled.Body.String())
	}
}

func TestMembershipAdminAPIEnabledReadsTenantAndWorkspaceMemberships(t *testing.T) {
	now := time.Date(2026, 6, 9, 8, 0, 0, 0, time.UTC)
	store := coretenant.NewMemoryMembershipStore()
	if _, err := store.UpsertTenantMembership(context.Background(), coretenant.TenantMembership{
		TenantID:  "tenant-a",
		SubjectID: "user-1",
		Roles:     []coretenant.Role{coretenant.RoleTenantAdmin},
		CreatedAt: now,
		UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("UpsertTenantMembership() error = %v", err)
	}
	if _, err := store.UpsertWorkspaceMembership(context.Background(), coretenant.WorkspaceMembership{
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		SubjectID:   "user-1",
		Roles:       []coretenant.Role{coretenant.RoleWorkspaceViewer, coretenant.RoleWorkspaceAnalyst},
		CreatedAt:   now,
		UpdatedAt:   now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertWorkspaceMembership() error = %v", err)
	}
	handler := newRouter(routerServices{
		adminConfig:     AdminConfig{MembershipAPIEnabled: true},
		membershipStore: store,
	}, "test-token")

	tenant := doMembershipAdminRequest(t, handler, http.MethodGet, "/api/v1/admin/memberships?scope=tenant&tenant_id=tenant-a&subject_id=user-1")
	if tenant.Code != http.StatusOK {
		t.Fatalf("tenant status = %d, want %d; body=%s", tenant.Code, http.StatusOK, tenant.Body.String())
	}
	tenantData := decodeIdentityEnvelope(t, tenant.Body.Bytes()).Data.(map[string]any)
	if tenantData["status"] != "enabled_read_only" || tenantData["read_only"] != true || tenantData["write_endpoints_enabled"] != false {
		t.Fatalf("unexpected tenant admin status payload: %#v", tenantData)
	}
	if tenantData["membership_found"] != true || tenantData["scope"] != "tenant" || tenantData["tenant_id"] != "tenant-a" || tenantData["subject_id"] != "user-1" {
		t.Fatalf("unexpected tenant membership lookup payload: %#v", tenantData)
	}
	tenantMembership := tenantData["membership"].(map[string]any)
	tenantRoles := tenantMembership["roles"].([]any)
	if tenantMembership["scope"] != "tenant" || len(tenantRoles) != 1 || tenantRoles[0] != string(coretenant.RoleTenantAdmin) {
		t.Fatalf("unexpected tenant membership payload: %#v", tenantMembership)
	}

	workspace := doMembershipAdminRequest(t, handler, http.MethodGet, "/api/v1/admin/memberships?scope=workspace&tenant_id=tenant-a&workspace_id=workspace-a&subject_id=user-1")
	if workspace.Code != http.StatusOK {
		t.Fatalf("workspace status = %d, want %d; body=%s", workspace.Code, http.StatusOK, workspace.Body.String())
	}
	workspaceData := decodeIdentityEnvelope(t, workspace.Body.Bytes()).Data.(map[string]any)
	if workspaceData["membership_found"] != true || workspaceData["scope"] != "workspace" || workspaceData["workspace_id"] != "workspace-a" {
		t.Fatalf("unexpected workspace membership lookup payload: %#v", workspaceData)
	}
	workspaceMembership := workspaceData["membership"].(map[string]any)
	workspaceRoles := workspaceMembership["roles"].([]any)
	if workspaceMembership["scope"] != "workspace" ||
		len(workspaceRoles) != 2 ||
		workspaceRoles[0] != string(coretenant.RoleWorkspaceViewer) ||
		workspaceRoles[1] != string(coretenant.RoleWorkspaceAnalyst) {
		t.Fatalf("unexpected workspace membership payload: %#v", workspaceMembership)
	}
}

func TestMembershipAdminAPIReadOnlyRejectsWriteMethods(t *testing.T) {
	store := coretenant.NewMemoryMembershipStore()
	handler := newRouter(routerServices{
		adminConfig:     AdminConfig{MembershipAPIEnabled: true},
		membershipStore: store,
	}, "test-token")

	rec := doMembershipAdminRequest(t, handler, http.MethodPost, "/api/v1/admin/memberships")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	body := decodeIdentityEnvelope(t, rec.Body.Bytes())
	if body.Error == nil || body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected read-only write response: %#v", body.Error)
	}
	if _, found, err := store.GetTenantMembership(context.Background(), "tenant-a", "user-1"); err != nil || found {
		t.Fatalf("read-only POST should not create membership, found=%v err=%v", found, err)
	}
}

func TestMembershipAdminAPIEnabledWithDisabledStoreReturnsUnsupported(t *testing.T) {
	handler := newRouter(routerServices{
		adminConfig: AdminConfig{MembershipAPIEnabled: true},
	}, "test-token")

	rec := doMembershipAdminRequest(t, handler, http.MethodGet, "/api/v1/admin/memberships?scope=tenant&tenant_id=tenant-a&subject_id=user-1")
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("disabled store status = %d, want %d; body=%s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	body := decodeIdentityEnvelope(t, rec.Body.Bytes())
	if body.Error == nil || body.Error.Code != services.CodeUnsupported {
		t.Fatalf("unexpected disabled store response: %#v", body.Error)
	}
}

func doMembershipAdminRequest(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(rec, req)
	return rec
}
