package daemon

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func TestWebAuthSkeletonEndpointsRequireBearer(t *testing.T) {
	handler := newRouter(routerServices{
		auth: AuthConfig{
			WebSessions: WebSessionAuthConfig{Enabled: true, CookieName: "ca_session"},
		},
	}, "test-token")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"user@example.test"}`))
	req.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), string(services.CodeUnauthorized)) {
		t.Fatalf("body should contain %q: %s", services.CodeUnauthorized, rec.Body.String())
	}
	if gotCookie := rec.Header().Values("Set-Cookie"); len(gotCookie) != 0 {
		t.Fatalf("web auth skeleton should not issue cookies without bearer, got %v", gotCookie)
	}
}

func TestWebAuthSkeletonDisabledReturnsNotFoundAndNoCookie(t *testing.T) {
	handler := newRouter(routerServices{}, "test-token")
	for _, path := range []string{
		"/api/v1/auth/login",
		"/api/v1/auth/logout",
		"/api/v1/auth/session/refresh",
	} {
		t.Run(path, func(t *testing.T) {
			rec := doWebAuthSkeletonRequest(t, handler, path)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
			}
			if gotCookie := rec.Header().Values("Set-Cookie"); len(gotCookie) != 0 {
				t.Fatalf("disabled web auth skeleton should not issue cookies, got %v", gotCookie)
			}
			body := decodeIdentityEnvelope(t, rec.Body.Bytes())
			if body.Error == nil || body.Error.Code != services.CodeNotFound {
				t.Fatalf("error = %#v, want not_found", body.Error)
			}
			if body.Error.Details["web_sessions_enabled"] != false || body.Error.Details["status"] != "disabled" {
				t.Fatalf("disabled details should make gate explicit: %#v", body.Error.Details)
			}
		})
	}
}

func TestWebAuthSkeletonEnabledReturnsUnsupportedBoundaryAndNoCookie(t *testing.T) {
	handler := newRouter(routerServices{
		auth: AuthConfig{
			WebSessions: WebSessionAuthConfig{Enabled: true, CookieName: "ca_session"},
		},
	}, "test-token")

	for _, path := range []string{
		"/api/v1/auth/login",
		"/api/v1/auth/logout",
		"/api/v1/auth/session/refresh",
	} {
		t.Run(path, func(t *testing.T) {
			rec := doWebAuthSkeletonRequest(t, handler, path)
			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotImplemented, rec.Body.String())
			}
			if gotCookie := rec.Header().Values("Set-Cookie"); len(gotCookie) != 0 {
				t.Fatalf("enabled skeleton should not issue cookies before real login, got %v", gotCookie)
			}
			body := decodeIdentityEnvelope(t, rec.Body.Bytes())
			if body.Error == nil || body.Error.Code != services.CodeUnsupported {
				t.Fatalf("error = %#v, want unsupported", body.Error)
			}
			details := body.Error.Details
			if details["web_sessions_enabled"] != true ||
				details["cookie_name"] != "ca_session" ||
				details["csrf_required"] != true ||
				details["csrf_header"] != webAuthCSRFHeaderName ||
				details["cookie_http_only"] != true ||
				details["cookie_same_site"] != "strict" ||
				details["cookie_secure_policy"] != "secure_required_outside_loopback" ||
				details["account_adapter_configured"] != false {
				t.Fatalf("enabled details should expose only skeleton boundary, got %#v", details)
			}
		})
	}
}

func doWebAuthSkeletonRequest(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(rec, req)
	return rec
}
