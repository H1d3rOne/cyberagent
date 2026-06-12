package daemon

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func TestDaemonRateLimitDefaultNoOp(t *testing.T) {
	calls := 0
	handler := withDaemonRateLimit(RateLimitConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
		req.RemoteAddr = "203.0.113.10:1000"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d; body=%s", i+1, rec.Code, http.StatusOK, rec.Body.String())
		}
	}
	if calls != 3 {
		t.Fatalf("handler calls = %d, want 3", calls)
	}
}

func TestDaemonRateLimitEnabledExhaustsBurst(t *testing.T) {
	handler := withDaemonRateLimit(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 60,
		Burst:             1,
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	firstReq.RemoteAddr = "203.0.113.10:1000"
	handler.ServeHTTP(first, firstReq)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d; body=%s", first.Code, http.StatusOK, first.Body.String())
	}

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	secondReq.RemoteAddr = "203.0.113.10:1000"
	handler.ServeHTTP(second, secondReq)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d; body=%s", second.Code, http.StatusTooManyRequests, second.Body.String())
	}
	if got := strings.TrimSpace(second.Header().Get("Retry-After")); got == "" {
		t.Fatal("Retry-After header should be set on rate limited response")
	}
	if !strings.Contains(second.Body.String(), string(services.CodeRateLimited)) {
		t.Fatalf("rate limited body should contain %q: %s", services.CodeRateLimited, second.Body.String())
	}
}

func TestDaemonHardeningDefaultNoOp(t *testing.T) {
	handler := withDaemonHardening(HardeningConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	req.RemoteAddr = "203.0.113.10:1000"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestDaemonHardeningRequireLoopback(t *testing.T) {
	handler := withDaemonHardening(HardeningConfig{RequireLoopback: true}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		remoteAddr string
		wantStatus int
		wantCode   services.ErrorCode
	}{
		{name: "rejects non loopback", remoteAddr: "203.0.113.10:1000", wantStatus: http.StatusUnprocessableEntity, wantCode: services.CodePolicyDenied},
		{name: "allows IPv4 loopback", remoteAddr: "127.0.0.1:1000", wantStatus: http.StatusOK},
		{name: "allows IPv6 loopback", remoteAddr: "[::1]:1000", wantStatus: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
			req.RemoteAddr = tc.remoteAddr
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantCode != "" && !strings.Contains(rec.Body.String(), string(tc.wantCode)) {
				t.Fatalf("body should contain %q: %s", tc.wantCode, rec.Body.String())
			}
		})
	}
}

func TestRouterSecurityGatesPreserveCORSPreflight(t *testing.T) {
	handler := newRouter(routerServices{
		rateLimit: RateLimitConfig{Enabled: true, RequestsPerMinute: 1, Burst: 1},
		hardening: HardeningConfig{RequireLoopback: true},
	}, "test-token")

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/healthz", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "authorization")
	req.RemoteAddr = "203.0.113.10:1000"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("allow origin = %q, want localhost origin", got)
	}
}

func TestRouterSecurityHardeningAppliesToAPIRequests(t *testing.T) {
	handler := newRouter(routerServices{
		hardening: HardeningConfig{RequireLoopback: true},
	}, "test-token")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.RemoteAddr = "203.0.113.10:1000"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), string(services.CodePolicyDenied)) {
		t.Fatalf("body should contain %q: %s", services.CodePolicyDenied, rec.Body.String())
	}
}

func TestRouterSecurityRateLimitAppliesToAPIRequests(t *testing.T) {
	handler := newRouter(routerServices{
		rateLimit: RateLimitConfig{Enabled: true, RequestsPerMinute: 60, Burst: 1},
	}, "test-token")

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	firstReq.Header.Set("Authorization", "Bearer test-token")
	firstReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(first, firstReq)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d; body=%s", first.Code, http.StatusOK, first.Body.String())
	}

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	secondReq.Header.Set("Authorization", "Bearer test-token")
	secondReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(second, secondReq)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d; body=%s", second.Code, http.StatusTooManyRequests, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), string(services.CodeRateLimited)) {
		t.Fatalf("body should contain %q: %s", services.CodeRateLimited, second.Body.String())
	}
}
