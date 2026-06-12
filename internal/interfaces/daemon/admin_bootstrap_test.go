package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAdminBootstrapStateLoadsTokenWithoutExposingStatusSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap-token")
	if err := os.WriteFile(path, []byte("bootstrap-secret\n"), 0o600); err != nil {
		t.Fatalf("write bootstrap token: %v", err)
	}

	state := loadAdminBootstrapState(AdminConfig{BootstrapTokenFile: path})
	if !state.Configured || !state.Ready || state.Token != "bootstrap-secret" {
		t.Fatalf("unexpected bootstrap state: %#v", state)
	}
	status := state.Status()
	if !status.Configured || !status.Ready {
		t.Fatalf("unexpected bootstrap status: %#v", status)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	if strings.Contains(string(encoded), "bootstrap-secret") || strings.Contains(string(encoded), path) {
		t.Fatalf("safe status leaked bootstrap secret/path: %s", string(encoded))
	}
}

func TestAdminBootstrapStatusRouteRequiresBearerAndRedactsSecret(t *testing.T) {
	handler := newRouter(routerServices{
		admin: adminBootstrapState{
			Configured: true,
			Ready:      true,
			Token:      "bootstrap-secret",
			Message:    "admin bootstrap token loaded",
		},
	}, "daemon-token")

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/admin/bootstrap", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/bootstrap", nil)
	req.Header.Set("Authorization", "Bearer daemon-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "bootstrap-secret") {
		t.Fatalf("bootstrap status response leaked token: %s", body)
	}
	if !strings.Contains(body, `"configured":true`) || !strings.Contains(body, `"ready":true`) {
		t.Fatalf("bootstrap status response missing readiness: %s", body)
	}
}
