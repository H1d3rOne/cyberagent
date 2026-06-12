package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

func TestIdentityMeReturnsLocalTenantContext(t *testing.T) {
	handler := newRouter(routerServices{}, "test-token")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.RemoteAddr = "127.0.0.1:1000"

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := decodeIdentityEnvelope(t, rec.Body.Bytes())
	data := body.Data.(map[string]any)
	principal := data["principal"].(map[string]any)
	if principal["subject_id"] != coretenant.LocalSubjectID || principal["auth_method"] != coretenant.AuthMethodLocalBearer {
		t.Fatalf("unexpected principal payload: %#v", principal)
	}
	tenant := data["tenant"].(map[string]any)
	workspace := data["workspace"].(map[string]any)
	if tenant["id"] != coretenant.LocalTenantID || workspace["id"] != coretenant.DefaultWorkspaceID {
		t.Fatalf("unexpected scope payload: tenant=%#v workspace=%#v", tenant, workspace)
	}
	session := data["session"].(map[string]any)
	if session["mode"] != coretenant.ModeLocal || session["authenticated"] != true || session["login_required"] != false {
		t.Fatalf("unexpected session payload: %#v", session)
	}
}

func TestIdentityScopeEndpointsRequireBearer(t *testing.T) {
	handler := newRouter(routerServices{}, "test-token")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.RemoteAddr = "127.0.0.1:1000"

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), string(services.CodeUnauthorized)) {
		t.Fatalf("body should contain %q: %s", services.CodeUnauthorized, rec.Body.String())
	}
}

func TestIdentityTenantsAndWorkspacesExposeLocalDefaultOnly(t *testing.T) {
	handler := newRouter(routerServices{}, "test-token")

	tenantResp := doIdentityRequest(t, handler, http.MethodGet, "/api/v1/tenants")
	if tenantResp.Code != http.StatusOK {
		t.Fatalf("tenants status = %d, want %d; body=%s", tenantResp.Code, http.StatusOK, tenantResp.Body.String())
	}
	if !strings.Contains(tenantResp.Body.String(), `"id":"local"`) || !strings.Contains(tenantResp.Body.String(), `"selected":true`) {
		t.Fatalf("tenants should expose selected local tenant only: %s", tenantResp.Body.String())
	}

	workspaceResp := doIdentityRequest(t, handler, http.MethodGet, "/api/v1/workspaces")
	if workspaceResp.Code != http.StatusOK {
		t.Fatalf("workspaces status = %d, want %d; body=%s", workspaceResp.Code, http.StatusOK, workspaceResp.Body.String())
	}
	if !strings.Contains(workspaceResp.Body.String(), `"id":"default"`) || !strings.Contains(workspaceResp.Body.String(), `"tenant_id":"local"`) {
		t.Fatalf("workspaces should expose selected local/default workspace only: %s", workspaceResp.Body.String())
	}

	selectResp := doIdentityRequest(t, handler, http.MethodPost, "/api/v1/workspaces/default/select")
	if selectResp.Code != http.StatusOK {
		t.Fatalf("select default status = %d, want %d; body=%s", selectResp.Code, http.StatusOK, selectResp.Body.String())
	}
	if !strings.Contains(selectResp.Body.String(), `"id":"default"`) || !strings.Contains(selectResp.Body.String(), `"selected":true`) {
		t.Fatalf("select default should return selected local/default workspace: %s", selectResp.Body.String())
	}
}

func TestIdentityWorkspaceSelectRejectsUnknownWorkspace(t *testing.T) {
	handler := newRouter(routerServices{}, "test-token")
	rec := doIdentityRequest(t, handler, http.MethodPost, "/api/v1/workspaces/other/select")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), string(services.CodeNotFound)) {
		t.Fatalf("body should contain %q: %s", services.CodeNotFound, rec.Body.String())
	}
}

func TestIdentityMeKeepsLocalBearerWhenL3AuthSkeletonConfigured(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	server, err := NewServer(Config{
		RunsDir:   t.TempDir(),
		TokenFile: tokenFile,
		Auth: AuthConfig{
			WebSessions: WebSessionAuthConfig{Enabled: true},
			RBAC:        RBACConfig{Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer server.Close()
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.RemoteAddr = "127.0.0.1:1000"
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotCookie := rec.Header().Values("Set-Cookie"); len(gotCookie) != 0 {
		t.Fatalf("local bearer identity endpoint should not issue web session cookies, got %v", gotCookie)
	}
	body := decodeIdentityEnvelope(t, rec.Body.Bytes())
	data := body.Data.(map[string]any)
	session := data["session"].(map[string]any)
	if session["mode"] != coretenant.ModeLocal || session["auth_method"] != coretenant.AuthMethodLocalBearer || session["login_required"] != false {
		t.Fatalf("auth skeleton should not change local session payload: %#v", session)
	}
}

func doIdentityRequest(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeIdentityEnvelope(t *testing.T, data []byte) Envelope {
	t.Helper()
	var body Envelope
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, string(data))
	}
	return body
}
