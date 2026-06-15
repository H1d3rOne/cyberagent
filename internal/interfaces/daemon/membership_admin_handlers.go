package daemon

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

const membershipAdminEndpoint = "/api/v1/admin/memberships"
const membershipAdminBootstrapHeader = "X-CyberAgent-Admin-Bootstrap-Token"

type membershipAdminResponse struct {
	Status                string                  `json:"status"`
	ReadOnly              bool                    `json:"read_only"`
	WriteEndpointsEnabled bool                    `json:"write_endpoints_enabled"`
	Mutation              string                  `json:"mutation,omitempty"`
	AuthorizedBy          string                  `json:"authorized_by,omitempty"`
	Scope                 string                  `json:"scope"`
	TenantID              string                  `json:"tenant_id"`
	WorkspaceID           string                  `json:"workspace_id,omitempty"`
	SubjectID             string                  `json:"subject_id"`
	MembershipFound       bool                    `json:"membership_found"`
	Membership            *membershipAdminPayload `json:"membership"`
}

type membershipAdminPayload struct {
	Scope       string   `json:"scope"`
	TenantID    string   `json:"tenant_id"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
	SubjectID   string   `json:"subject_id"`
	Roles       []string `json:"roles"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

type membershipAdminMutationRequest struct {
	Scope       string   `json:"scope"`
	TenantID    string   `json:"tenant_id"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
	SubjectID   string   `json:"subject_id"`
	Roles       []string `json:"roles"`
	Comment     string   `json:"comment,omitempty"`
}

func handleMembershipAdminRoute(w http.ResponseWriter, r *http.Request, admin AdminConfig, bootstrap adminBootstrapState, store coretenant.MembershipStore, auditSink daemonAuditSink) {
	if !admin.MembershipAPIEnabled {
		writeServiceError(w, membershipAdminDisabledError())
		return
	}
	switch r.Method {
	case http.MethodGet:
		handleMembershipAdminLookupRoute(w, r, admin, store)
	case http.MethodPost:
		handleMembershipAdminMutationRoute(w, r, admin, bootstrap, store, auditSink)
	default:
		writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{
			"method":                  r.Method,
			"endpoint":                membershipAdminEndpoint,
			"allowed_methods":         membershipAdminAllowedMethods(admin),
			"read_only":               !admin.MembershipWriteEnabled,
			"write_endpoints_enabled": admin.MembershipWriteEnabled,
		}, nil))
	}
}

func handleMembershipAdminLookupRoute(w http.ResponseWriter, r *http.Request, admin AdminConfig, store coretenant.MembershipStore) {
	if store == nil {
		store = coretenant.DisabledMembershipStore{}
	}

	query := r.URL.Query()
	scope := strings.ToLower(strings.TrimSpace(query.Get("scope")))
	if scope != "tenant" && scope != "workspace" {
		writeServiceError(w, services.NewError(services.CodeBadRequest, "invalid membership scope", map[string]any{
			"field":          "scope",
			"allowed_scopes": []string{"tenant", "workspace"},
			"endpoint":       membershipAdminEndpoint,
			"read_only":      true,
		}, nil))
		return
	}
	tenantID := strings.TrimSpace(query.Get("tenant_id"))
	subjectID := strings.TrimSpace(query.Get("subject_id"))
	if tenantID == "" || subjectID == "" {
		writeServiceError(w, services.NewError(services.CodeBadRequest, "membership lookup requires tenant_id and subject_id", map[string]any{
			"required_fields": []string{"scope", "tenant_id", "subject_id"},
			"endpoint":        membershipAdminEndpoint,
			"read_only":       true,
		}, nil))
		return
	}

	switch scope {
	case "tenant":
		handleTenantMembershipLookup(w, r.Context(), store, admin.MembershipWriteEnabled, tenantID, subjectID)
	case "workspace":
		workspaceID := strings.TrimSpace(query.Get("workspace_id"))
		if workspaceID == "" {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "workspace membership lookup requires workspace_id", map[string]any{
				"required_fields": []string{"scope", "tenant_id", "workspace_id", "subject_id"},
				"endpoint":        membershipAdminEndpoint,
				"read_only":       true,
			}, nil))
			return
		}
		handleWorkspaceMembershipLookup(w, r.Context(), store, admin.MembershipWriteEnabled, tenantID, workspaceID, subjectID)
	}
}

func handleTenantMembershipLookup(w http.ResponseWriter, ctx context.Context, store coretenant.MembershipStore, writeEnabled bool, tenantID, subjectID string) {
	membership, found, err := store.GetTenantMembership(ctx, tenantID, subjectID)
	if err != nil {
		writeServiceError(w, membershipAdminLookupError(err))
		return
	}
	var payload *membershipAdminPayload
	if found {
		payload = tenantMembershipAdminPayload(membership)
	}
	writeOK(w, http.StatusOK, membershipAdminResponse{
		Status:                membershipAdminStatus(writeEnabled),
		ReadOnly:              !writeEnabled,
		WriteEndpointsEnabled: writeEnabled,
		Scope:                 "tenant",
		TenantID:              tenantID,
		SubjectID:             subjectID,
		MembershipFound:       found,
		Membership:            payload,
	})
}

func handleWorkspaceMembershipLookup(w http.ResponseWriter, ctx context.Context, store coretenant.MembershipStore, writeEnabled bool, tenantID, workspaceID, subjectID string) {
	membership, found, err := store.GetWorkspaceMembership(ctx, tenantID, workspaceID, subjectID)
	if err != nil {
		writeServiceError(w, membershipAdminLookupError(err))
		return
	}
	var payload *membershipAdminPayload
	if found {
		payload = workspaceMembershipAdminPayload(membership)
	}
	writeOK(w, http.StatusOK, membershipAdminResponse{
		Status:                membershipAdminStatus(writeEnabled),
		ReadOnly:              !writeEnabled,
		WriteEndpointsEnabled: writeEnabled,
		Scope:                 "workspace",
		TenantID:              tenantID,
		WorkspaceID:           workspaceID,
		SubjectID:             subjectID,
		MembershipFound:       found,
		Membership:            payload,
	})
}

func handleMembershipAdminMutationRoute(w http.ResponseWriter, r *http.Request, admin AdminConfig, bootstrap adminBootstrapState, store coretenant.MembershipStore, auditSink daemonAuditSink) {
	if !admin.MembershipWriteEnabled {
		writeServiceError(w, services.NewError(services.CodeBadRequest, "membership admin write endpoint is disabled", map[string]any{
			"endpoint":                membershipAdminEndpoint,
			"allowed_methods":         []string{http.MethodGet},
			"read_only":               true,
			"write_endpoints_enabled": false,
			"required_gate":           "daemon.admin.membership_write_enabled",
		}, nil))
		return
	}
	if store == nil {
		store = coretenant.DisabledMembershipStore{}
	}
	if auditSink == nil {
		auditSink = noopDaemonAuditSink{}
	}
	var body membershipAdminMutationRequest
	if err := decodeJSONBody(r, &body); err != nil {
		writeServiceError(w, err)
		return
	}
	body = normalizeMembershipAdminMutationRequest(body)
	if err := validateMembershipAdminMutationRequest(body); err != nil {
		writeServiceError(w, err)
		return
	}
	authorization, err := authorizeMembershipAdminMutation(r.Context(), r, bootstrap, store, body)
	if err != nil {
		_ = recordMembershipAdminMutationAudit(r.Context(), auditSink, body, authorization, "deny")
		writeServiceError(w, err)
		return
	}

	switch body.Scope {
	case "tenant":
		roles, err := membershipAdminRoles(body.Roles, "tenant")
		if err != nil {
			writeServiceError(w, err)
			return
		}
		membership, err := store.UpsertTenantMembership(r.Context(), coretenant.TenantMembership{
			TenantID:  body.TenantID,
			SubjectID: body.SubjectID,
			Roles:     roles,
		})
		if err != nil {
			writeServiceError(w, membershipAdminMutationError(err))
			return
		}
		if err := recordMembershipAdminMutationAudit(r.Context(), auditSink, body, authorization, "ok"); err != nil {
			writeServiceError(w, services.NewError(services.CodeInternal, "record membership mutation audit event", nil, err))
			return
		}
		writeOK(w, http.StatusOK, membershipAdminResponse{
			Status:                membershipAdminStatus(true),
			ReadOnly:              false,
			WriteEndpointsEnabled: true,
			Mutation:              "upsert",
			AuthorizedBy:          authorization.Method,
			Scope:                 "tenant",
			TenantID:              body.TenantID,
			SubjectID:             body.SubjectID,
			MembershipFound:       true,
			Membership:            tenantMembershipAdminPayload(membership),
		})
	case "workspace":
		roles, err := membershipAdminRoles(body.Roles, "workspace")
		if err != nil {
			writeServiceError(w, err)
			return
		}
		membership, err := store.UpsertWorkspaceMembership(r.Context(), coretenant.WorkspaceMembership{
			TenantID:    body.TenantID,
			WorkspaceID: body.WorkspaceID,
			SubjectID:   body.SubjectID,
			Roles:       roles,
		})
		if err != nil {
			writeServiceError(w, membershipAdminMutationError(err))
			return
		}
		if err := recordMembershipAdminMutationAudit(r.Context(), auditSink, body, authorization, "ok"); err != nil {
			writeServiceError(w, services.NewError(services.CodeInternal, "record membership mutation audit event", nil, err))
			return
		}
		writeOK(w, http.StatusOK, membershipAdminResponse{
			Status:                membershipAdminStatus(true),
			ReadOnly:              false,
			WriteEndpointsEnabled: true,
			Mutation:              "upsert",
			AuthorizedBy:          authorization.Method,
			Scope:                 "workspace",
			TenantID:              body.TenantID,
			WorkspaceID:           body.WorkspaceID,
			SubjectID:             body.SubjectID,
			MembershipFound:       true,
			Membership:            workspaceMembershipAdminPayload(membership),
		})
	}
}

type membershipAdminMutationAuthorization struct {
	Method      string
	PrincipalID string
}

func membershipAdminAllowedMethods(admin AdminConfig) []string {
	if admin.MembershipWriteEnabled {
		return []string{http.MethodGet, http.MethodPost}
	}
	return []string{http.MethodGet}
}

func membershipAdminStatus(writeEnabled bool) string {
	if writeEnabled {
		return "enabled_write_gate"
	}
	return "enabled_read_only"
}

func normalizeMembershipAdminMutationRequest(req membershipAdminMutationRequest) membershipAdminMutationRequest {
	req.Scope = strings.ToLower(strings.TrimSpace(req.Scope))
	req.TenantID = strings.TrimSpace(req.TenantID)
	req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
	req.SubjectID = strings.TrimSpace(req.SubjectID)
	req.Comment = strings.TrimSpace(req.Comment)
	for i := range req.Roles {
		req.Roles[i] = strings.TrimSpace(req.Roles[i])
	}
	return req
}

func validateMembershipAdminMutationRequest(req membershipAdminMutationRequest) error {
	if req.Scope != "tenant" && req.Scope != "workspace" {
		return services.NewError(services.CodeBadRequest, "invalid membership scope", map[string]any{
			"field":          "scope",
			"allowed_scopes": []string{"tenant", "workspace"},
			"endpoint":       membershipAdminEndpoint,
		}, nil)
	}
	required := []string{"scope", "tenant_id", "subject_id", "roles"}
	if req.Scope == "workspace" {
		required = []string{"scope", "tenant_id", "workspace_id", "subject_id", "roles"}
	}
	if req.TenantID == "" || req.SubjectID == "" || (req.Scope == "workspace" && req.WorkspaceID == "") || len(req.Roles) == 0 {
		return services.NewError(services.CodeBadRequest, "membership mutation missing required fields", map[string]any{
			"required_fields": required,
			"endpoint":        membershipAdminEndpoint,
		}, nil)
	}
	_, err := membershipAdminRoles(req.Roles, req.Scope)
	return err
}

func membershipAdminRoles(raw []string, scope string) ([]coretenant.Role, error) {
	if len(raw) == 0 {
		return nil, services.NewError(services.CodeBadRequest, "membership roles are required", map[string]any{
			"field":    "roles",
			"endpoint": membershipAdminEndpoint,
		}, nil)
	}
	roles := make([]coretenant.Role, 0, len(raw))
	seen := make(map[coretenant.Role]struct{}, len(raw))
	for _, value := range raw {
		role, ok := coretenant.NormalizeRole(value)
		if !ok {
			return nil, services.NewError(services.CodeBadRequest, "membership role is unknown", map[string]any{
				"field":    "roles",
				"role":     strings.TrimSpace(value),
				"endpoint": membershipAdminEndpoint,
			}, nil)
		}
		if role.Scope() != scope {
			return nil, services.NewError(services.CodeBadRequest, "membership role scope mismatch", map[string]any{
				"field":      "roles",
				"role":       string(role),
				"role_scope": role.Scope(),
				"want_scope": scope,
				"endpoint":   membershipAdminEndpoint,
			}, nil)
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles, nil
}

func authorizeMembershipAdminMutation(ctx context.Context, r *http.Request, bootstrap adminBootstrapState, store coretenant.MembershipStore, req membershipAdminMutationRequest) (membershipAdminMutationAuthorization, error) {
	if token := strings.TrimSpace(r.Header.Get(membershipAdminBootstrapHeader)); token != "" {
		if bootstrap.Ready && constantTimeTokenEqual(token, bootstrap.Token) {
			return membershipAdminMutationAuthorization{Method: "bootstrap_token", PrincipalID: "bootstrap"}, nil
		}
		return membershipAdminMutationAuthorization{Method: "bootstrap_token"}, services.NewError(services.CodeUnauthorized, "invalid admin bootstrap token", map[string]any{
			"endpoint": membershipAdminEndpoint,
		}, nil)
	}

	tc := coretenant.FromContextOrLocal(r.Context())
	principal := tc.Principal.Clone()
	if strings.TrimSpace(principal.AuthMethod) == coretenant.AuthMethodLocalBearer {
		return membershipAdminMutationAuthorization{Method: "none", PrincipalID: principal.SubjectID}, membershipAdminMutationDeniedError("local_bearer_requires_bootstrap_or_web_admin")
	}
	subjectID := strings.TrimSpace(principal.SubjectID)
	if subjectID == "" {
		return membershipAdminMutationAuthorization{Method: "none"}, membershipAdminMutationDeniedError("principal_missing")
	}
	if store == nil {
		store = coretenant.DisabledMembershipStore{}
	}
	if allowedByTenantAdmin(ctx, store, req.TenantID, subjectID) {
		return membershipAdminMutationAuthorization{Method: "tenant_owner_or_admin", PrincipalID: subjectID}, nil
	}
	if req.Scope == "workspace" && allowedByWorkspaceAdmin(ctx, store, req.TenantID, req.WorkspaceID, subjectID) {
		return membershipAdminMutationAuthorization{Method: "workspace_admin", PrincipalID: subjectID}, nil
	}
	return membershipAdminMutationAuthorization{Method: "membership", PrincipalID: subjectID}, membershipAdminMutationDeniedError("missing_owner_or_admin_membership")
}

func constantTimeTokenEqual(got, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func allowedByTenantAdmin(ctx context.Context, store coretenant.MembershipStore, tenantID, subjectID string) bool {
	membership, found, err := store.GetTenantMembership(ctx, tenantID, subjectID)
	if err != nil || !found {
		return false
	}
	for _, role := range membership.Roles {
		if role == coretenant.RoleTenantOwner || role == coretenant.RoleTenantAdmin {
			return true
		}
	}
	return false
}

func allowedByWorkspaceAdmin(ctx context.Context, store coretenant.MembershipStore, tenantID, workspaceID, subjectID string) bool {
	membership, found, err := store.GetWorkspaceMembership(ctx, tenantID, workspaceID, subjectID)
	if err != nil || !found {
		return false
	}
	for _, role := range membership.Roles {
		if role == coretenant.RoleWorkspaceAdmin {
			return true
		}
	}
	return false
}

func membershipAdminMutationDeniedError(reason string) error {
	return services.NewError(services.CodePolicyDenied, "membership mutation requires bootstrap token or owner/admin membership", map[string]any{
		"endpoint":       membershipAdminEndpoint,
		"required":       []string{"admin_bootstrap_token", "tenant_owner", "tenant_admin", "workspace_admin_for_workspace_scope"},
		"reason":         reason,
		"bootstrap_gate": "daemon.admin.bootstrap_token_file",
		"write_gate":     "daemon.admin.membership_write_enabled",
	}, nil)
}

func membershipAdminMutationError(err error) error {
	if errors.Is(err, coretenant.ErrMembershipStoreDisabled) {
		return services.NewError(services.CodeUnsupported, "membership store is disabled", map[string]any{
			"endpoint":                membershipAdminEndpoint,
			"status":                  "store_disabled",
			"read_only":               false,
			"write_endpoints_enabled": true,
		}, err)
	}
	return services.NewError(services.CodeInternal, "mutate membership", map[string]any{
		"endpoint":                membershipAdminEndpoint,
		"write_endpoints_enabled": true,
	}, err)
}

func recordMembershipAdminMutationAudit(ctx context.Context, sink daemonAuditSink, req membershipAdminMutationRequest, authorization membershipAdminMutationAuthorization, outcome string) error {
	if sink == nil {
		return nil
	}
	roleCount := len(req.Roles)
	if roles, err := membershipAdminRoles(req.Roles, req.Scope); err == nil {
		roleCount = len(roles)
	}
	return sink.Record(ctx, daemonAuditEvent{
		Type:        "admin.membership.mutation",
		Outcome:     outcome,
		PrincipalID: strings.TrimSpace(authorization.PrincipalID),
		TenantID:    strings.TrimSpace(req.TenantID),
		WorkspaceID: strings.TrimSpace(req.WorkspaceID),
		Details: map[string]any{
			"mutation":        "upsert",
			"scope":           req.Scope,
			"subject_id":      req.SubjectID,
			"role_count":      roleCount,
			"authorized_by":   authorization.Method,
			"comment_present": req.Comment != "",
		},
	})
}

func membershipAdminDisabledError() error {
	return services.NewError(services.CodeNotFound, "membership admin API is disabled", map[string]any{
		"endpoint":                membershipAdminEndpoint,
		"status":                  "disabled",
		"membership_api_enabled":  false,
		"read_only":               true,
		"write_endpoints_enabled": false,
		"required_query":          []string{"scope", "tenant_id", "subject_id"},
	}, nil)
}

func membershipAdminLookupError(err error) error {
	if errors.Is(err, coretenant.ErrMembershipStoreDisabled) {
		return services.NewError(services.CodeUnsupported, "membership store is disabled", map[string]any{
			"endpoint":                membershipAdminEndpoint,
			"status":                  "store_disabled",
			"read_only":               true,
			"write_endpoints_enabled": false,
		}, err)
	}
	return services.NewError(services.CodeInternal, "lookup membership", map[string]any{
		"endpoint":  membershipAdminEndpoint,
		"read_only": true,
	}, err)
}

func tenantMembershipAdminPayload(m coretenant.TenantMembership) *membershipAdminPayload {
	m = m.Clone()
	return &membershipAdminPayload{
		Scope:     "tenant",
		TenantID:  strings.TrimSpace(m.TenantID),
		SubjectID: strings.TrimSpace(m.SubjectID),
		Roles:     membershipRoleNames(m.Roles),
		CreatedAt: membershipAdminTime(m.CreatedAt),
		UpdatedAt: membershipAdminTime(m.UpdatedAt),
	}
}

func workspaceMembershipAdminPayload(m coretenant.WorkspaceMembership) *membershipAdminPayload {
	m = m.Clone()
	return &membershipAdminPayload{
		Scope:       "workspace",
		TenantID:    strings.TrimSpace(m.TenantID),
		WorkspaceID: strings.TrimSpace(m.WorkspaceID),
		SubjectID:   strings.TrimSpace(m.SubjectID),
		Roles:       membershipRoleNames(m.Roles),
		CreatedAt:   membershipAdminTime(m.CreatedAt),
		UpdatedAt:   membershipAdminTime(m.UpdatedAt),
	}
}

func membershipRoleNames(roles []coretenant.Role) []string {
	if len(roles) == 0 {
		return nil
	}
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, string(role))
	}
	return names
}

func membershipAdminTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
