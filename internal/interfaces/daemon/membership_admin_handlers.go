package daemon

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

const membershipAdminEndpoint = "/api/v1/admin/memberships"

type membershipAdminResponse struct {
	Status                string                  `json:"status"`
	ReadOnly              bool                    `json:"read_only"`
	WriteEndpointsEnabled bool                    `json:"write_endpoints_enabled"`
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

func handleMembershipAdminRoute(w http.ResponseWriter, r *http.Request, admin AdminConfig, store coretenant.MembershipStore) {
	if !admin.MembershipAPIEnabled {
		writeServiceError(w, membershipAdminDisabledError())
		return
	}
	if r.Method != http.MethodGet {
		writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{
			"method":                  r.Method,
			"endpoint":                membershipAdminEndpoint,
			"allowed_methods":         []string{http.MethodGet},
			"read_only":               true,
			"write_endpoints_enabled": false,
		}, nil))
		return
	}
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
		handleTenantMembershipLookup(w, r.Context(), store, tenantID, subjectID)
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
		handleWorkspaceMembershipLookup(w, r.Context(), store, tenantID, workspaceID, subjectID)
	}
}

func handleTenantMembershipLookup(w http.ResponseWriter, ctx context.Context, store coretenant.MembershipStore, tenantID, subjectID string) {
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
		Status:                "enabled_read_only",
		ReadOnly:              true,
		WriteEndpointsEnabled: false,
		Scope:                 "tenant",
		TenantID:              tenantID,
		SubjectID:             subjectID,
		MembershipFound:       found,
		Membership:            payload,
	})
}

func handleWorkspaceMembershipLookup(w http.ResponseWriter, ctx context.Context, store coretenant.MembershipStore, tenantID, workspaceID, subjectID string) {
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
		Status:                "enabled_read_only",
		ReadOnly:              true,
		WriteEndpointsEnabled: false,
		Scope:                 "workspace",
		TenantID:              tenantID,
		WorkspaceID:           workspaceID,
		SubjectID:             subjectID,
		MembershipFound:       found,
		Membership:            payload,
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
