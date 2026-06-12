package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

type captureDaemonAuditSink struct {
	events []daemonAuditEvent
}

func (s *captureDaemonAuditSink) Record(_ context.Context, event daemonAuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *captureDaemonAuditSink) Close() error { return nil }

func TestRBACSkeletonDisabledIsNoOp(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := rbacDecisionFromContext(r.Context()); ok {
			t.Fatal("rbac decision should be absent when RBAC skeleton is disabled")
		}
		writeOK(w, http.StatusOK, map[string]any{"ok": true})
	})
	handler := withRBACSkeleton(RBACConfig{Enabled: false}, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRBACSkeletonEnabledAnnotatesWithoutEnforcing(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decision, ok := rbacDecisionFromContext(r.Context())
		if !ok {
			t.Fatal("rbac decision missing")
		}
		if !decision.Enabled || !decision.Matched || decision.Enforced || decision.Reason != "skeleton_not_enforced" {
			t.Fatalf("unexpected rbac decision: %#v", decision)
		}
		if decision.Action != "identity.read" || decision.Scope != "tenant" {
			t.Fatalf("unexpected rbac action/scope: %#v", decision)
		}
		if len(decision.RequiredRoles) == 0 || decision.RequiredRoles[0] != coretenant.RoleTenantViewer {
			t.Fatalf("required roles not populated from route table: %#v", decision.RequiredRoles)
		}
		if len(decision.PrincipalRoles) != 1 || decision.PrincipalRoles[0] != "local" {
			t.Fatalf("principal roles should preserve local bearer roles without web grants: %#v", decision.PrincipalRoles)
		}
		decision.RequiredRoles[0] = coretenant.RoleTenantOwner
		again, ok := rbacDecisionFromContext(r.Context())
		if !ok || again.RequiredRoles[0] != coretenant.RoleTenantViewer {
			t.Fatalf("rbac decision should be cloned on read, got %#v ok=%v", again, ok)
		}
		writeOK(w, http.StatusOK, map[string]any{"ok": true})
	})
	handler := withLocalTenantContext(withRBACSkeleton(RBACConfig{Enabled: true}, next))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRBACPolicyEvaluatorUsesMembershipStoreWithoutEnforcing(t *testing.T) {
	store := coretenant.NewMemoryMembershipStore()
	if _, err := store.UpsertTenantMembership(context.Background(), coretenant.TenantMembership{
		TenantID:  coretenant.LocalTenantID,
		SubjectID: "user-1",
		Roles:     []coretenant.Role{coretenant.RoleTenantViewer},
	}); err != nil {
		t.Fatalf("UpsertTenantMembership() error = %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decision, ok := rbacDecisionFromContext(r.Context())
		if !ok {
			t.Fatal("rbac decision missing")
		}
		if !decision.Evaluated || !decision.MembershipFound || !decision.Allowed || decision.Enforced {
			t.Fatalf("unexpected rbac policy decision: %#v", decision)
		}
		if decision.PolicySource != "builtin" || decision.EvaluationReason != "policy_allowed" {
			t.Fatalf("unexpected rbac policy metadata: %#v", decision)
		}
		if decision.MembershipSubjectID != "user-1" || len(decision.MembershipRoles) != 1 || decision.MembershipRoles[0] != coretenant.RoleTenantViewer {
			t.Fatalf("unexpected membership evaluation details: %#v", decision)
		}
		decision.MembershipRoles[0] = coretenant.RoleTenantOwner
		again, ok := rbacDecisionFromContext(r.Context())
		if !ok || again.MembershipRoles[0] != coretenant.RoleTenantViewer {
			t.Fatalf("rbac membership roles should be cloned on read, got %#v ok=%v", again, ok)
		}
		writeOK(w, http.StatusOK, map[string]any{"ok": true})
	})
	principal := coretenant.Principal{
		SubjectID:   "user-1",
		DisplayName: "User One",
		AuthMethod:  coretenant.AuthMethodWebSession,
		Roles:       []string{"browser_user"},
	}
	handler := withTenantContext(coretenant.LocalResolver{}, principal, withRBACPolicyEvaluator(
		RBACConfig{Enabled: true},
		store,
		rbacPolicy{},
		next,
	))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRBACPolicyEvaluatorMissingMembershipDoesNotDeny(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decision, ok := rbacDecisionFromContext(r.Context())
		if !ok {
			t.Fatal("rbac decision missing")
		}
		if !decision.Evaluated || decision.MembershipFound || decision.Allowed || decision.Enforced {
			t.Fatalf("unexpected missing-membership decision: %#v", decision)
		}
		if decision.EvaluationReason != "membership_not_found" {
			t.Fatalf("EvaluationReason = %q, want membership_not_found", decision.EvaluationReason)
		}
		writeOK(w, http.StatusOK, map[string]any{"ok": true})
	})
	principal := coretenant.Principal{SubjectID: "user-1", AuthMethod: coretenant.AuthMethodWebSession}
	handler := withTenantContext(coretenant.LocalResolver{}, principal, withRBACPolicyEvaluator(
		RBACConfig{Enabled: true},
		coretenant.NewMemoryMembershipStore(),
		rbacPolicy{},
		next,
	))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRBACEnforcementGateDeniesNonLocalMissingMembershipAndAuditsSafely(t *testing.T) {
	cfg := RBACConfig{Enabled: true, AuditEnabled: true, EnforcementEnabled: true}
	nextRan := false
	sink := &captureDaemonAuditSink{}
	policyPath := filepath.Join(t.TempDir(), "rbac.yaml")
	policy := rbacPolicy{
		Source: policyPath,
		Requirements: map[string]rbacRequirement{
			http.MethodGet + " /api/v1/me": {
				Action: "identity.read",
				Scope:  "tenant",
				Roles:  []coretenant.Role{coretenant.RoleTenantViewer},
			},
		},
	}
	principal := coretenant.Principal{SubjectID: "user-1", AuthMethod: coretenant.AuthMethodWebSession}
	handler := withTenantContext(coretenant.LocalResolver{}, principal, withRBACPolicyEvaluator(
		cfg,
		coretenant.NewMemoryMembershipStore(),
		policy,
		withRBACEnforcementGate(cfg, sink, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextRan = true
			writeOK(w, http.StatusOK, map[string]any{"ok": true})
		})),
	))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer super-secret-token")
	handler.ServeHTTP(rec, req)

	if nextRan {
		t.Fatal("next handler ran for denied RBAC request")
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
	var envelope Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rec.Body.String())
	}
	if envelope.Error == nil || envelope.Error.Code != "policy_denied" {
		t.Fatalf("unexpected denial response: %#v", envelope.Error)
	}
	if len(sink.events) != 1 {
		t.Fatalf("audit events = %d, want 1: %#v", len(sink.events), sink.events)
	}
	event := sink.events[0]
	if event.Type != "rbac.decision" || event.Outcome != "deny" {
		t.Fatalf("unexpected rbac audit event: %#v", event)
	}
	if event.Details["policy_source_type"] != "configured" ||
		event.Details["action"] != "identity.read" ||
		event.Details["evaluation_reason"] != "membership_not_found" {
		t.Fatalf("unexpected rbac audit details: %#v", event.Details)
	}
	rawEvent, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal audit event: %v", err)
	}
	for _, leaked := range []string{policyPath, "/api/v1/me", "super-secret-token"} {
		if strings.Contains(string(rawEvent), leaked) || strings.Contains(rec.Body.String(), leaked) {
			t.Fatalf("rbac deny/audit leaked %q; response=%s audit=%s", leaked, rec.Body.String(), string(rawEvent))
		}
	}
}

func TestRBACEnforcementGateAllowsMembershipAndAnnotatesDecision(t *testing.T) {
	cfg := RBACConfig{Enabled: true, EnforcementEnabled: true}
	store := coretenant.NewMemoryMembershipStore()
	if _, err := store.UpsertTenantMembership(context.Background(), coretenant.TenantMembership{
		TenantID:  coretenant.LocalTenantID,
		SubjectID: "user-1",
		Roles:     []coretenant.Role{coretenant.RoleTenantViewer},
	}); err != nil {
		t.Fatalf("UpsertTenantMembership() error = %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decision, ok := rbacDecisionFromContext(r.Context())
		if !ok {
			t.Fatal("rbac decision missing")
		}
		if !decision.Enforced || !decision.Allowed || decision.Reason != "enforced_allowed" {
			t.Fatalf("unexpected enforced allow decision: %#v", decision)
		}
		writeOK(w, http.StatusOK, map[string]any{"ok": true})
	})
	principal := coretenant.Principal{SubjectID: "user-1", AuthMethod: coretenant.AuthMethodWebSession}
	handler := withTenantContext(coretenant.LocalResolver{}, principal, withRBACPolicyEvaluator(
		cfg,
		store,
		rbacPolicy{},
		withRBACEnforcementGate(cfg, noopDaemonAuditSink{}, next),
	))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRBACEnforcementGateExemptsLocalBearerFromDeny(t *testing.T) {
	cfg := RBACConfig{Enabled: true, EnforcementEnabled: true}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decision, ok := rbacDecisionFromContext(r.Context())
		if !ok {
			t.Fatal("rbac decision missing")
		}
		if decision.Enforced || decision.Allowed || decision.Reason != "local_bearer_exempt" {
			t.Fatalf("unexpected local bearer exemption decision: %#v", decision)
		}
		if decision.EvaluationReason != "membership_not_found" {
			t.Fatalf("EvaluationReason = %q, want membership_not_found", decision.EvaluationReason)
		}
		writeOK(w, http.StatusOK, map[string]any{"ok": true})
	})
	handler := withLocalTenantContext(withRBACPolicyEvaluator(
		cfg,
		coretenant.NewMemoryMembershipStore(),
		rbacPolicy{},
		withRBACEnforcementGate(cfg, noopDaemonAuditSink{}, next),
	))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("local bearer should be exempt from RBAC deny, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRBACRequirementTableUsesKnownRoles(t *testing.T) {
	for key, requirement := range daemonRBACRouteRequirements() {
		if requirement.Action == "" || requirement.Scope == "" {
			t.Fatalf("%s missing action/scope: %#v", key, requirement)
		}
		for _, role := range requirement.Roles {
			if !role.Valid() {
				t.Fatalf("%s contains invalid role %q", key, role)
			}
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/default/select", nil)
	requirement, ok := daemonRBACRequirementForRequest(req)
	if !ok {
		t.Fatal("dynamic workspace select requirement missing")
	}
	if requirement.Action != "workspace.select" || requirement.Scope != "workspace" {
		t.Fatalf("unexpected workspace select requirement: %#v", requirement)
	}
	for _, role := range requirement.Roles {
		if !role.Valid() || role.Scope() != "workspace" {
			t.Fatalf("workspace select role should be valid workspace role, got %q", role)
		}
	}
}

func TestLoadDaemonRBACPolicyFileOverridesRouteRequirements(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rbac.yaml")
	content := `
version: 1
routes:
  - method: get
    path: /api/v1/me
    action: identity.read.custom
    scope: tenant
    roles:
      - tenant_owner
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	policy, err := loadDaemonRBACPolicyFile(path)
	if err != nil {
		t.Fatalf("loadDaemonRBACPolicyFile() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	requirement, ok := daemonRBACRequirementForRequestWithPolicy(policy, req)
	if !ok {
		t.Fatal("policy requirement missing")
	}
	if requirement.Action != "identity.read.custom" || len(requirement.Roles) != 1 || requirement.Roles[0] != coretenant.RoleTenantOwner {
		t.Fatalf("unexpected loaded policy requirement: %#v", requirement)
	}
}

func TestLoadDaemonRBACPolicyFileRejectsUnknownOrWrongScopeRoles(t *testing.T) {
	tests := map[string]string{
		"unknown": `
routes:
  - method: GET
    path: /api/v1/me
    action: identity.read
    scope: tenant
    roles: [not_a_role]
`,
		"wrong_scope": `
routes:
  - method: GET
    path: /api/v1/me
    action: identity.read
    scope: tenant
    roles: [workspace_viewer]
`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rbac.yaml")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadDaemonRBACPolicyFile(path); err == nil {
				t.Fatal("loadDaemonRBACPolicyFile() error = nil, want validation error")
			}
		})
	}
}

func TestNewServerLoadsExplicitRBACPolicyPath(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "rbac.yaml")
	if err := os.WriteFile(policyPath, []byte(`
routes:
  - method: GET
    path: /api/v1/me
    action: identity.read
    scope: tenant
    roles: [tenant_viewer]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(Config{
		RunsDir:   filepath.Join(dir, "runs"),
		TokenFile: filepath.Join(dir, "token"),
		Auth: AuthConfig{
			RBAC: RBACConfig{
				Enabled:    true,
				PolicyPath: "  " + policyPath + "  ",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	defer server.Close()
	token, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	req.RemoteAddr = "127.0.0.1:1000"
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("local bearer should not be denied by RBAC policy skeleton, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewServerRBACPolicyPathIsIgnoredWhenDisabledAndFailsWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	missingPolicyPath := filepath.Join(dir, "missing-rbac.yaml")
	server, err := NewServer(Config{
		RunsDir:   filepath.Join(dir, "disabled-runs"),
		TokenFile: filepath.Join(dir, "disabled-token"),
		Auth: AuthConfig{
			RBAC: RBACConfig{
				Enabled:    false,
				PolicyPath: missingPolicyPath,
			},
		},
	})
	if err != nil {
		t.Fatalf("disabled RBAC should not load policy path, NewServer() error = %v", err)
	}
	defer server.Close()

	_, err = NewServer(Config{
		RunsDir:   filepath.Join(dir, "enabled-runs"),
		TokenFile: filepath.Join(dir, "enabled-token"),
		Auth: AuthConfig{
			RBAC: RBACConfig{
				Enabled:    true,
				PolicyPath: missingPolicyPath,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "read rbac policy") {
		t.Fatalf("enabled RBAC should fail fast on missing explicit policy, err=%v", err)
	}
}

func TestNewRouterRBACSkeletonEnabledDoesNotChangeLocalBearer(t *testing.T) {
	handler := newRouter(routerServices{
		auth: AuthConfig{
			RBAC: RBACConfig{Enabled: true},
		},
	}, "test-token")

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
	session := data["session"].(map[string]any)
	if session["auth_method"] != coretenant.AuthMethodLocalBearer || session["mode"] != coretenant.ModeLocal {
		t.Fatalf("RBAC skeleton should not replace local bearer identity: %#v", session)
	}
}

func TestNewRouterRBACEnforcementGateKeepsLocalBearerExempt(t *testing.T) {
	handler := newRouter(routerServices{
		auth: AuthConfig{
			RBAC: RBACConfig{Enabled: true, AuditEnabled: true, EnforcementEnabled: true},
		},
	}, "test-token")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.RemoteAddr = "127.0.0.1:1000"
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("local bearer should not be denied by RBAC enforcement gate, status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := decodeIdentityEnvelope(t, rec.Body.Bytes())
	data := body.Data.(map[string]any)
	session := data["session"].(map[string]any)
	if session["auth_method"] != coretenant.AuthMethodLocalBearer || session["mode"] != coretenant.ModeLocal {
		t.Fatalf("RBAC enforcement should preserve local bearer identity: %#v", session)
	}
}
