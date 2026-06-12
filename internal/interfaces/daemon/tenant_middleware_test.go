package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"

	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

func TestLocalTenantContextMiddlewareInjectsLocalDefaultScopeAfterBearerAuth(t *testing.T) {
	called := false
	handler := bearerAuth("test-token", withLocalTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		tc, ok := coretenant.FromContext(r.Context())
		if !ok {
			t.Fatal("tenant context missing")
		}
		if tc.TenantID != coretenant.LocalTenantID || tc.WorkspaceID != coretenant.DefaultWorkspaceID || tc.Mode != coretenant.ModeLocal {
			t.Fatalf("tenant context = %#v, want local/default local mode", tc)
		}
		if tc.Principal.SubjectID != coretenant.LocalSubjectID || tc.Principal.AuthMethod != coretenant.AuthMethodLocalBearer {
			t.Fatalf("principal = %#v, want local bearer principal", tc.Principal)
		}
		writeOK(w, http.StatusOK, map[string]any{"tenant_id": tc.TenantID, "workspace_id": tc.WorkspaceID})
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz?tenant_id=ignored", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("wrapped handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestLocalTenantContextMiddlewareDoesNotRunBeforeValidBearerAuth(t *testing.T) {
	called := false
	handler := bearerAuth("test-token", withLocalTenantContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("tenant context middleware ran for unauthorized request")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}
