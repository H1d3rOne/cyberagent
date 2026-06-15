package daemon

import (
	"context"
	"encoding/json"
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

	rec := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": "tenant",
		"tenant_id": "tenant-a",
		"subject_id": "user-1",
		"roles": ["tenant_admin"]
	}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	body := decodeIdentityEnvelope(t, rec.Body.Bytes())
	if body.Error == nil || body.Error.Code != services.CodeBadRequest {
		t.Fatalf("unexpected read-only write response: %#v", body.Error)
	}
	if body.Error.Details["write_endpoints_enabled"] != false || body.Error.Details["read_only"] != true {
		t.Fatalf("read-only write response should expose disabled write metadata: %#v", body.Error.Details)
	}
	if _, found, err := store.GetTenantMembership(context.Background(), "tenant-a", "user-1"); err != nil || found {
		t.Fatalf("read-only POST should not create membership, found=%v err=%v", found, err)
	}
}

func TestMembershipAdminAPIWriteGateBootstrapUpsertsAndAuditsTenantMembership(t *testing.T) {
	store := coretenant.NewMemoryMembershipStore()
	sink := &captureDaemonAuditSink{}
	handler := newRouter(routerServices{
		admin: adminBootstrapState{
			Configured: true,
			Ready:      true,
			Token:      "bootstrap-secret",
		},
		adminConfig: AdminConfig{
			MembershipAPIEnabled:   true,
			MembershipWriteEnabled: true,
		},
		membershipStore: store,
		auditSink:       sink,
	}, "test-token")

	rec := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": " tenant ",
		"tenant_id": " tenant-a ",
		"subject_id": " user-1 ",
		"roles": [" tenant_admin ", "tenant_admin"],
		"comment": "initial bootstrap grant"
	}`, func(req *http.Request) {
		req.Header.Set(membershipAdminBootstrapHeader, "bootstrap-secret")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	data := decodeIdentityEnvelope(t, rec.Body.Bytes()).Data.(map[string]any)
	if data["status"] != "enabled_write_gate" ||
		data["read_only"] != false ||
		data["write_endpoints_enabled"] != true ||
		data["mutation"] != "upsert" ||
		data["authorized_by"] != "bootstrap_token" {
		t.Fatalf("unexpected write response metadata: %#v", data)
	}
	membership := data["membership"].(map[string]any)
	roles := membership["roles"].([]any)
	if membership["scope"] != "tenant" ||
		membership["tenant_id"] != "tenant-a" ||
		membership["subject_id"] != "user-1" ||
		len(roles) != 1 ||
		roles[0] != string(coretenant.RoleTenantAdmin) {
		t.Fatalf("unexpected written tenant membership payload: %#v", membership)
	}

	lookup := doMembershipAdminRequest(t, handler, http.MethodGet, "/api/v1/admin/memberships?scope=tenant&tenant_id=tenant-a&subject_id=user-1")
	if lookup.Code != http.StatusOK {
		t.Fatalf("GET after POST status = %d, want %d; body=%s", lookup.Code, http.StatusOK, lookup.Body.String())
	}
	lookupData := decodeIdentityEnvelope(t, lookup.Body.Bytes()).Data.(map[string]any)
	if lookupData["membership_found"] != true || lookupData["status"] != "enabled_write_gate" {
		t.Fatalf("unexpected lookup after write: %#v", lookupData)
	}

	if len(sink.events) != 1 {
		t.Fatalf("audit events = %d, want 1: %#v", len(sink.events), sink.events)
	}
	event := sink.events[0]
	if event.Type != "admin.membership.mutation" ||
		event.Outcome != "ok" ||
		event.PrincipalID != "bootstrap" ||
		event.TenantID != "tenant-a" {
		t.Fatalf("unexpected membership mutation audit event: %#v", event)
	}
	if event.Details["authorized_by"] != "bootstrap_token" ||
		event.Details["scope"] != "tenant" ||
		event.Details["subject_id"] != "user-1" ||
		event.Details["role_count"] != 1 ||
		event.Details["comment_present"] != true {
		t.Fatalf("unexpected membership mutation audit details: %#v", event.Details)
	}
	rawEvent, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal audit event: %v", err)
	}
	for _, leaked := range []string{"bootstrap-secret", "initial bootstrap grant", membershipAdminBootstrapHeader} {
		if strings.Contains(string(rawEvent), leaked) || strings.Contains(rec.Body.String(), leaked) {
			t.Fatalf("membership mutation response/audit leaked %q; response=%s audit=%s", leaked, rec.Body.String(), string(rawEvent))
		}
	}
}

func TestMembershipAdminAPIWriteGateInvalidBootstrapTokenIsUnauthorizedAndSafe(t *testing.T) {
	store := coretenant.NewMemoryMembershipStore()
	sink := &captureDaemonAuditSink{}
	handler := newRouter(routerServices{
		admin: adminBootstrapState{
			Configured: true,
			Ready:      true,
			Token:      "bootstrap-secret",
		},
		adminConfig: AdminConfig{
			MembershipAPIEnabled:   true,
			MembershipWriteEnabled: true,
		},
		membershipStore: store,
		auditSink:       sink,
	}, "test-token")

	rec := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": "tenant",
		"tenant_id": "tenant-a",
		"subject_id": "user-1",
		"roles": ["tenant_admin"],
		"comment": "should not leak"
	}`, func(req *http.Request) {
		req.Header.Set(membershipAdminBootstrapHeader, "wrong-secret")
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	body := decodeIdentityEnvelope(t, rec.Body.Bytes())
	if body.Error == nil || body.Error.Code != services.CodeUnauthorized {
		t.Fatalf("unexpected invalid-bootstrap response: %#v", body.Error)
	}
	if _, found, err := store.GetTenantMembership(context.Background(), "tenant-a", "user-1"); err != nil || found {
		t.Fatalf("invalid bootstrap token should not create membership, found=%v err=%v", found, err)
	}
	if len(sink.events) != 1 || sink.events[0].Outcome != "deny" || sink.events[0].Details["authorized_by"] != "bootstrap_token" {
		t.Fatalf("unexpected deny audit event: %#v", sink.events)
	}
	rawEvent, err := json.Marshal(sink.events[0])
	if err != nil {
		t.Fatalf("marshal audit event: %v", err)
	}
	for _, leaked := range []string{"wrong-secret", "bootstrap-secret", "should not leak", membershipAdminBootstrapHeader} {
		if strings.Contains(string(rawEvent), leaked) || strings.Contains(rec.Body.String(), leaked) {
			t.Fatalf("invalid-bootstrap response/audit leaked %q; response=%s audit=%s", leaked, rec.Body.String(), string(rawEvent))
		}
	}
}

func TestMembershipAdminAPIWriteGateUpsertTimestampsAreStableAndClientTimestampsIgnored(t *testing.T) {
	store := coretenant.NewMemoryMembershipStore()
	handler := newRouter(routerServices{
		admin: adminBootstrapState{
			Configured: true,
			Ready:      true,
			Token:      "bootstrap-secret",
		},
		adminConfig: AdminConfig{
			MembershipAPIEnabled:   true,
			MembershipWriteEnabled: true,
		},
		membershipStore: store,
		auditSink:       &captureDaemonAuditSink{},
	}, "test-token")
	withBootstrap := func(req *http.Request) {
		req.Header.Set(membershipAdminBootstrapHeader, "bootstrap-secret")
	}
	clientCreatedAt := "2001-02-03T04:05:06Z"
	clientUpdatedAt := "2000-02-03T04:05:06Z"

	first := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": "tenant",
		"tenant_id": "tenant-a",
		"subject_id": "user-1",
		"roles": ["tenant_viewer"],
		"created_at": "2001-02-03T04:05:06Z",
		"updated_at": "2000-02-03T04:05:06Z"
	}`, withBootstrap)
	if first.Code != http.StatusOK {
		t.Fatalf("first POST status = %d, want %d; body=%s", first.Code, http.StatusOK, first.Body.String())
	}
	firstMembership := decodeIdentityEnvelope(t, first.Body.Bytes()).Data.(map[string]any)["membership"].(map[string]any)
	firstCreatedAt, _ := firstMembership["created_at"].(string)
	firstUpdatedAt, _ := firstMembership["updated_at"].(string)
	if firstCreatedAt == "" || firstUpdatedAt == "" {
		t.Fatalf("first membership response should expose non-empty timestamps: %#v", firstMembership)
	}
	if firstCreatedAt == clientCreatedAt || firstUpdatedAt == clientUpdatedAt {
		t.Fatalf("admin mutation should ignore client-provided timestamps: %#v", firstMembership)
	}

	second := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": "tenant",
		"tenant_id": "tenant-a",
		"subject_id": "user-1",
		"roles": ["tenant_admin"]
	}`, withBootstrap)
	if second.Code != http.StatusOK {
		t.Fatalf("second POST status = %d, want %d; body=%s", second.Code, http.StatusOK, second.Body.String())
	}
	secondMembership := decodeIdentityEnvelope(t, second.Body.Bytes()).Data.(map[string]any)["membership"].(map[string]any)
	if secondMembership["created_at"] != firstCreatedAt {
		t.Fatalf("upsert should preserve created_at when request omits timestamps: first=%#v second=%#v", firstMembership, secondMembership)
	}
	if secondMembership["updated_at"] == "" {
		t.Fatalf("upsert response should keep updated_at non-empty: %#v", secondMembership)
	}
	roles := secondMembership["roles"].([]any)
	if len(roles) != 1 || roles[0] != string(coretenant.RoleTenantAdmin) {
		t.Fatalf("second upsert should replace roles: %#v", secondMembership)
	}

	workspace := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": "workspace",
		"tenant_id": "tenant-a",
		"workspace_id": "workspace-a",
		"subject_id": "user-1",
		"roles": ["workspace_viewer"],
		"created_at": "2001-02-03T04:05:06Z",
		"updated_at": "2000-02-03T04:05:06Z"
	}`, withBootstrap)
	if workspace.Code != http.StatusOK {
		t.Fatalf("workspace POST status = %d, want %d; body=%s", workspace.Code, http.StatusOK, workspace.Body.String())
	}
	workspaceMembership := decodeIdentityEnvelope(t, workspace.Body.Bytes()).Data.(map[string]any)["membership"].(map[string]any)
	workspaceCreatedAt, _ := workspaceMembership["created_at"].(string)
	workspaceUpdatedAt, _ := workspaceMembership["updated_at"].(string)
	if workspaceCreatedAt == "" || workspaceUpdatedAt == "" ||
		workspaceCreatedAt == clientCreatedAt ||
		workspaceUpdatedAt == clientUpdatedAt {
		t.Fatalf("workspace membership response should expose store-owned timestamps: %#v", workspaceMembership)
	}
}

func TestMembershipAdminAPIWriteGateLocalBearerRequiresBootstrapOrWebAdmin(t *testing.T) {
	store := coretenant.NewMemoryMembershipStore()
	handler := newRouter(routerServices{
		adminConfig: AdminConfig{
			MembershipAPIEnabled:   true,
			MembershipWriteEnabled: true,
		},
		membershipStore: store,
		auditSink:       &captureDaemonAuditSink{},
	}, "test-token")

	rec := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": "tenant",
		"tenant_id": "tenant-a",
		"subject_id": "user-1",
		"roles": ["tenant_admin"]
	}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("POST status = %d, want %d; body=%s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	body := decodeIdentityEnvelope(t, rec.Body.Bytes())
	if body.Error == nil ||
		body.Error.Code != services.CodePolicyDenied ||
		body.Error.Details["reason"] != "local_bearer_requires_bootstrap_or_web_admin" {
		t.Fatalf("unexpected local-bearer denial: %#v", body.Error)
	}
	if _, found, err := store.GetTenantMembership(context.Background(), "tenant-a", "user-1"); err != nil || found {
		t.Fatalf("local bearer without bootstrap should not create membership, found=%v err=%v", found, err)
	}
}

func TestMembershipAdminAPIWriteGateTenantAdminWebSessionCanMutateTenantAndWorkspace(t *testing.T) {
	now := time.Now().UTC()
	sessionStore := coretenant.NewMemorySessionStore()
	if _, err := sessionStore.CreateSession(context.Background(), coretenant.Session{
		ID:          "session-admin",
		SubjectID:   "admin-user",
		AuthMethod:  coretenant.AuthMethodWebSession,
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		CreatedAt:   now.Add(-time.Minute),
		ExpiresAt:   now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	membershipStore := coretenant.NewMemoryMembershipStore()
	if _, err := membershipStore.UpsertTenantMembership(context.Background(), coretenant.TenantMembership{
		TenantID:  "tenant-a",
		SubjectID: "admin-user",
		Roles:     []coretenant.Role{coretenant.RoleTenantAdmin},
	}); err != nil {
		t.Fatalf("UpsertTenantMembership() error = %v", err)
	}
	handler := newRouter(routerServices{
		adminConfig: AdminConfig{
			MembershipAPIEnabled:   true,
			MembershipWriteEnabled: true,
		},
		auth: AuthConfig{
			WebSessions: WebSessionAuthConfig{
				Enabled:                  true,
				CookieName:               "ca_session",
				PrincipalResolverEnabled: true,
			},
		},
		sessionStore:    sessionStore,
		membershipStore: membershipStore,
		auditSink:       &captureDaemonAuditSink{},
	}, "test-token")

	tenant := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": "tenant",
		"tenant_id": "tenant-a",
		"subject_id": "user-tenant",
		"roles": ["tenant_viewer"]
	}`, func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-admin"})
	})
	if tenant.Code != http.StatusOK {
		t.Fatalf("tenant POST status = %d, want %d; body=%s", tenant.Code, http.StatusOK, tenant.Body.String())
	}
	tenantData := decodeIdentityEnvelope(t, tenant.Body.Bytes()).Data.(map[string]any)
	if tenantData["authorized_by"] != "tenant_owner_or_admin" {
		t.Fatalf("unexpected tenant authorized_by: %#v", tenantData)
	}

	workspace := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": "workspace",
		"tenant_id": "tenant-a",
		"workspace_id": "workspace-a",
		"subject_id": "user-workspace",
		"roles": ["workspace_viewer"]
	}`, func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-admin"})
	})
	if workspace.Code != http.StatusOK {
		t.Fatalf("workspace POST status = %d, want %d; body=%s", workspace.Code, http.StatusOK, workspace.Body.String())
	}
	workspaceData := decodeIdentityEnvelope(t, workspace.Body.Bytes()).Data.(map[string]any)
	if workspaceData["authorized_by"] != "tenant_owner_or_admin" || workspaceData["scope"] != "workspace" {
		t.Fatalf("unexpected workspace authorized_by/payload: %#v", workspaceData)
	}
}

func TestMembershipAdminAPIWriteGateWorkspaceAdminCanOnlyMutateWorkspaceScope(t *testing.T) {
	now := time.Now().UTC()
	sessionStore := coretenant.NewMemorySessionStore()
	if _, err := sessionStore.CreateSession(context.Background(), coretenant.Session{
		ID:          "session-workspace-admin",
		SubjectID:   "workspace-admin",
		AuthMethod:  coretenant.AuthMethodWebSession,
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		CreatedAt:   now.Add(-time.Minute),
		ExpiresAt:   now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	membershipStore := coretenant.NewMemoryMembershipStore()
	if _, err := membershipStore.UpsertWorkspaceMembership(context.Background(), coretenant.WorkspaceMembership{
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		SubjectID:   "workspace-admin",
		Roles:       []coretenant.Role{coretenant.RoleWorkspaceAdmin},
	}); err != nil {
		t.Fatalf("UpsertWorkspaceMembership() error = %v", err)
	}
	handler := newRouter(routerServices{
		adminConfig: AdminConfig{
			MembershipAPIEnabled:   true,
			MembershipWriteEnabled: true,
		},
		auth: AuthConfig{
			WebSessions: WebSessionAuthConfig{
				Enabled:                  true,
				CookieName:               "ca_session",
				PrincipalResolverEnabled: true,
			},
		},
		sessionStore:    sessionStore,
		membershipStore: membershipStore,
		auditSink:       &captureDaemonAuditSink{},
	}, "test-token")

	tenant := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": "tenant",
		"tenant_id": "tenant-a",
		"subject_id": "user-tenant",
		"roles": ["tenant_viewer"]
	}`, func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-workspace-admin"})
	})
	if tenant.Code != http.StatusUnprocessableEntity {
		t.Fatalf("tenant POST status = %d, want %d; body=%s", tenant.Code, http.StatusUnprocessableEntity, tenant.Body.String())
	}
	tenantBody := decodeIdentityEnvelope(t, tenant.Body.Bytes())
	if tenantBody.Error == nil || tenantBody.Error.Code != services.CodePolicyDenied {
		t.Fatalf("unexpected workspace-admin tenant denial: %#v", tenantBody.Error)
	}

	workspace := doMembershipAdminRequestWithBody(t, handler, http.MethodPost, "/api/v1/admin/memberships", `{
		"scope": "workspace",
		"tenant_id": "tenant-a",
		"workspace_id": "workspace-a",
		"subject_id": "user-workspace",
		"roles": ["workspace_analyst"]
	}`, func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: "ca_session", Value: "session-workspace-admin"})
	})
	if workspace.Code != http.StatusOK {
		t.Fatalf("workspace POST status = %d, want %d; body=%s", workspace.Code, http.StatusOK, workspace.Body.String())
	}
	workspaceData := decodeIdentityEnvelope(t, workspace.Body.Bytes()).Data.(map[string]any)
	if workspaceData["authorized_by"] != "workspace_admin" || workspaceData["scope"] != "workspace" {
		t.Fatalf("unexpected workspace-admin mutation payload: %#v", workspaceData)
	}
	if _, found, err := membershipStore.GetTenantMembership(context.Background(), "tenant-a", "user-tenant"); err != nil || found {
		t.Fatalf("workspace admin should not create tenant membership, found=%v err=%v", found, err)
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
	return doMembershipAdminRequestWithBody(t, handler, method, path, "", nil)
}

func doMembershipAdminRequestWithBody(t *testing.T, handler http.Handler, method, path, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer test-token")
	req.RemoteAddr = "127.0.0.1:1000"
	if mutate != nil {
		mutate(req)
	}
	handler.ServeHTTP(rec, req)
	return rec
}
