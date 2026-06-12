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

func TestWebSessionCookieResolverDisabledIsNoOp(t *testing.T) {
	store := &recordingSessionStore{}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := webSessionFromContext(r.Context()); ok {
			t.Fatal("web session context should be absent when resolver is disabled")
		}
		writeOK(w, http.StatusOK, map[string]any{"ok": true})
	})
	handler := withWebSessionCookieResolver(WebSessionAuthConfig{Enabled: false, CookieName: "ca_session"}, store, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-1"})
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.getCalls != 0 {
		t.Fatalf("disabled resolver should not read session store, get calls = %d", store.getCalls)
	}
	if gotCookie := rec.Header().Values("Set-Cookie"); len(gotCookie) != 0 {
		t.Fatalf("resolver should not issue cookies, got %v", gotCookie)
	}
}

func TestWebSessionCookieResolverAnnotatesActiveSessionWhenEnabled(t *testing.T) {
	now := time.Date(2026, 6, 9, 3, 0, 0, 0, time.UTC)
	store := coretenant.NewMemorySessionStore()
	created, err := store.CreateSession(context.Background(), coretenant.Session{
		ID:          "session-1",
		SubjectID:   "user-1",
		AuthMethod:  coretenant.AuthMethodWebSession,
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		Roles:       []coretenant.Role{coretenant.RoleWorkspaceViewer},
		CreatedAt:   now.Add(-time.Minute),
		ExpiresAt:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := webSessionFromContext(r.Context())
		if !ok {
			t.Fatal("web session context missing")
		}
		if session.ID != created.ID || session.SubjectID != "user-1" || session.AuthMethod != coretenant.AuthMethodWebSession {
			t.Fatalf("unexpected web session context: %#v", session)
		}
		session.Roles[0] = coretenant.RoleTenantOwner
		sessionAgain, ok := webSessionFromContext(r.Context())
		if !ok || sessionAgain.Roles[0] != coretenant.RoleWorkspaceViewer {
			t.Fatalf("web session context should be cloned on read, got %#v ok=%v", sessionAgain, ok)
		}
		writeOK(w, http.StatusOK, map[string]any{"session_id": session.ID})
	})
	handler := withWebSessionCookieResolverClock(
		WebSessionAuthConfig{Enabled: true, CookieName: "ca_session"},
		store,
		func() time.Time { return now },
		next,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-1"})
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotCookie := rec.Header().Values("Set-Cookie"); len(gotCookie) != 0 {
		t.Fatalf("resolver should not issue cookies, got %v", gotCookie)
	}
}

func TestNewRouterWebSessionsStillRequireBearerAndDoNotIssueCookie(t *testing.T) {
	now := time.Date(2026, 6, 9, 4, 0, 0, 0, time.UTC)
	store := coretenant.NewMemorySessionStore()
	if _, err := store.CreateSession(context.Background(), coretenant.Session{
		ID:         "session-1",
		SubjectID:  "user-1",
		AuthMethod: coretenant.AuthMethodWebSession,
		CreatedAt:  now.Add(-time.Minute),
		ExpiresAt:  now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	handler := newRouter(routerServices{
		auth: AuthConfig{
			WebSessions: WebSessionAuthConfig{Enabled: true, CookieName: "ca_session"},
		},
		sessionStore: store,
	}, "test-token")

	cookieOnly := httptest.NewRecorder()
	cookieOnlyReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	cookieOnlyReq.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-1"})
	cookieOnlyReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(cookieOnly, cookieOnlyReq)
	if cookieOnly.Code != http.StatusUnauthorized {
		t.Fatalf("cookie-only status = %d, want %d; body=%s", cookieOnly.Code, http.StatusUnauthorized, cookieOnly.Body.String())
	}
	if !strings.Contains(cookieOnly.Body.String(), string(services.CodeUnauthorized)) {
		t.Fatalf("cookie-only body should contain %q: %s", services.CodeUnauthorized, cookieOnly.Body.String())
	}

	withBearer := httptest.NewRecorder()
	withBearerReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	withBearerReq.Header.Set("Authorization", "Bearer test-token")
	withBearerReq.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-1"})
	withBearerReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(withBearer, withBearerReq)
	if withBearer.Code != http.StatusOK {
		t.Fatalf("bearer status = %d, want %d; body=%s", withBearer.Code, http.StatusOK, withBearer.Body.String())
	}
	if gotCookie := withBearer.Header().Values("Set-Cookie"); len(gotCookie) != 0 {
		t.Fatalf("local bearer API should not issue web session cookies, got %v", gotCookie)
	}
	body := decodeIdentityEnvelope(t, withBearer.Body.Bytes())
	data := body.Data.(map[string]any)
	session := data["session"].(map[string]any)
	if session["auth_method"] != coretenant.AuthMethodLocalBearer || session["mode"] != coretenant.ModeLocal {
		t.Fatalf("web session resolver should not replace local bearer identity: %#v", session)
	}
}

func TestNewRouterWebSessionPrincipalResolverHydratesTenantContextWhenExplicitlyEnabled(t *testing.T) {
	now := time.Now().UTC()
	store := coretenant.NewMemorySessionStore()
	if _, err := store.CreateSession(context.Background(), coretenant.Session{
		ID:          "session-1",
		SubjectID:   "user-1",
		AuthMethod:  coretenant.AuthMethodWebSession,
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		Roles:       []coretenant.Role{coretenant.RoleTenantViewer},
		CreatedAt:   now.Add(-time.Minute),
		ExpiresAt:   now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	handler := newRouter(routerServices{
		auth: AuthConfig{
			WebSessions: WebSessionAuthConfig{
				Enabled:                  true,
				CookieName:               "ca_session",
				PrincipalResolverEnabled: true,
			},
		},
		sessionStore: store,
	}, "test-token")

	cookieOnly := httptest.NewRecorder()
	cookieOnlyReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	cookieOnlyReq.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-1"})
	cookieOnlyReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(cookieOnly, cookieOnlyReq)
	if cookieOnly.Code != http.StatusUnauthorized {
		t.Fatalf("cookie-only status = %d, want %d; body=%s", cookieOnly.Code, http.StatusUnauthorized, cookieOnly.Body.String())
	}

	noCookie := httptest.NewRecorder()
	noCookieReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	noCookieReq.Header.Set("Authorization", "Bearer test-token")
	noCookieReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(noCookie, noCookieReq)
	if noCookie.Code != http.StatusOK {
		t.Fatalf("bearer/no-cookie status = %d, want %d; body=%s", noCookie.Code, http.StatusOK, noCookie.Body.String())
	}
	noCookieBody := decodeIdentityEnvelope(t, noCookie.Body.Bytes())
	noCookieData := noCookieBody.Data.(map[string]any)
	noCookieSession := noCookieData["session"].(map[string]any)
	if noCookieSession["auth_method"] != coretenant.AuthMethodLocalBearer || noCookieSession["mode"] != coretenant.ModeLocal {
		t.Fatalf("missing web session should fall back to local bearer identity: %#v", noCookieSession)
	}

	withBearer := httptest.NewRecorder()
	withBearerReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	withBearerReq.Header.Set("Authorization", "Bearer test-token")
	withBearerReq.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-1"})
	withBearerReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(withBearer, withBearerReq)
	if withBearer.Code != http.StatusOK {
		t.Fatalf("bearer+cookie status = %d, want %d; body=%s", withBearer.Code, http.StatusOK, withBearer.Body.String())
	}
	if gotCookie := withBearer.Header().Values("Set-Cookie"); len(gotCookie) != 0 {
		t.Fatalf("principal resolver should not issue web session cookies, got %v", gotCookie)
	}
	body := decodeIdentityEnvelope(t, withBearer.Body.Bytes())
	data := body.Data.(map[string]any)
	principal := data["principal"].(map[string]any)
	tenant := data["tenant"].(map[string]any)
	workspace := data["workspace"].(map[string]any)
	sessionPayload := data["session"].(map[string]any)
	if principal["subject_id"] != "user-1" || principal["auth_method"] != coretenant.AuthMethodWebSession {
		t.Fatalf("unexpected web session principal: %#v", principal)
	}
	roles := principal["roles"].([]any)
	if len(roles) != 1 || roles[0] != string(coretenant.RoleTenantViewer) {
		t.Fatalf("unexpected principal roles: %#v", roles)
	}
	if tenant["id"] != "tenant-a" || tenant["mode"] != coretenant.ModeMultiUser {
		t.Fatalf("unexpected tenant payload: %#v", tenant)
	}
	if workspace["id"] != "workspace-a" || workspace["tenant_id"] != "tenant-a" || workspace["mode"] != coretenant.ModeMultiUser {
		t.Fatalf("unexpected workspace payload: %#v", workspace)
	}
	if sessionPayload["auth_method"] != coretenant.AuthMethodWebSession || sessionPayload["mode"] != coretenant.ModeMultiUser {
		t.Fatalf("unexpected session payload: %#v", sessionPayload)
	}
}

func TestNewRouterWebSessionPrincipalResolverFeedsRBACEnforcement(t *testing.T) {
	now := time.Now().UTC()
	sessionStore := coretenant.NewMemorySessionStore()
	if _, err := sessionStore.CreateSession(context.Background(), coretenant.Session{
		ID:          "session-1",
		SubjectID:   "user-1",
		AuthMethod:  coretenant.AuthMethodWebSession,
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		CreatedAt:   now.Add(-time.Minute),
		ExpiresAt:   now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	membershipStore := coretenant.NewMemoryMembershipStore()
	handler := newRouter(routerServices{
		auth: AuthConfig{
			WebSessions: WebSessionAuthConfig{
				Enabled:                  true,
				CookieName:               "ca_session",
				PrincipalResolverEnabled: true,
			},
			RBAC: RBACConfig{Enabled: true, EnforcementEnabled: true},
		},
		sessionStore:    sessionStore,
		membershipStore: membershipStore,
	}, "test-token")

	denied := httptest.NewRecorder()
	deniedReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	deniedReq.Header.Set("Authorization", "Bearer test-token")
	deniedReq.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-1"})
	deniedReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(denied, deniedReq)
	if denied.Code != http.StatusUnprocessableEntity || !strings.Contains(denied.Body.String(), string(services.CodePolicyDenied)) {
		t.Fatalf("missing membership should be denied, status=%d body=%s", denied.Code, denied.Body.String())
	}

	if _, err := membershipStore.UpsertTenantMembership(context.Background(), coretenant.TenantMembership{
		TenantID:  "tenant-a",
		SubjectID: "user-1",
		Roles:     []coretenant.Role{coretenant.RoleTenantViewer},
	}); err != nil {
		t.Fatalf("UpsertTenantMembership() error = %v", err)
	}
	allowed := httptest.NewRecorder()
	allowedReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	allowedReq.Header.Set("Authorization", "Bearer test-token")
	allowedReq.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-1"})
	allowedReq.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(allowed, allowedReq)
	if allowed.Code != http.StatusOK {
		t.Fatalf("membership-backed web session should be allowed, status=%d body=%s", allowed.Code, allowed.Body.String())
	}
	body := decodeIdentityEnvelope(t, allowed.Body.Bytes())
	data := body.Data.(map[string]any)
	sessionPayload := data["session"].(map[string]any)
	if sessionPayload["auth_method"] != coretenant.AuthMethodWebSession || sessionPayload["mode"] != coretenant.ModeMultiUser {
		t.Fatalf("allowed response should preserve web session identity: %#v", sessionPayload)
	}
}

type recordingSessionStore struct {
	getCalls int
}

func (s *recordingSessionStore) CreateSession(context.Context, coretenant.Session) (coretenant.Session, error) {
	return coretenant.Session{}, nil
}

func (s *recordingSessionStore) GetSession(context.Context, string) (coretenant.Session, bool, error) {
	s.getCalls++
	return coretenant.Session{}, false, nil
}

func (s *recordingSessionStore) RevokeSession(context.Context, string, time.Time) (coretenant.Session, bool, error) {
	return coretenant.Session{}, false, nil
}
