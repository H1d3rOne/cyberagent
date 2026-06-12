package daemon

import (
	"net/http"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

type daemonIdentityPrincipalPayload struct {
	SubjectID   string   `json:"subject_id"`
	DisplayName string   `json:"display_name"`
	AuthMethod  string   `json:"auth_method"`
	Roles       []string `json:"roles"`
}

type daemonIdentityTenantPayload struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Mode     string `json:"mode"`
	Selected bool   `json:"selected"`
}

type daemonIdentityWorkspacePayload struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Label    string `json:"label"`
	Mode     string `json:"mode"`
	Selected bool   `json:"selected"`
}

type daemonIdentitySessionPayload struct {
	Mode          string `json:"mode"`
	Authenticated bool   `json:"authenticated"`
	LoginRequired bool   `json:"login_required"`
	AuthMethod    string `json:"auth_method"`
}

func handleIdentityMe(w http.ResponseWriter, r *http.Request) {
	tc := coretenant.FromContextOrLocal(r.Context())
	writeOK(w, http.StatusOK, map[string]any{
		"principal": daemonIdentityPrincipalFromTenantContext(tc),
		"tenant":    daemonIdentityTenantFromTenantContext(tc),
		"workspace": daemonIdentityWorkspaceFromTenantContext(tc),
		"session": daemonIdentitySessionPayload{
			Mode:          daemonIdentityMode(tc),
			Authenticated: true,
			LoginRequired: false,
			AuthMethod:    tc.Principal.AuthMethod,
		},
	})
}

func handleIdentityTenants(w http.ResponseWriter, r *http.Request) {
	tc := coretenant.FromContextOrLocal(r.Context())
	tenant := daemonIdentityTenantFromTenantContext(tc)
	writeOK(w, http.StatusOK, map[string]any{
		"tenants":  []daemonIdentityTenantPayload{tenant},
		"selected": tenant,
	})
}

func handleIdentityWorkspacesRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/workspaces"), "/")
	if path == "" {
		if r.Method != http.MethodGet {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		handleIdentityWorkspaces(w, r)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "select" {
		if r.Method != http.MethodPost {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		handleIdentityWorkspaceSelect(w, r, parts[0])
		return
	}

	writeServiceError(w, services.NewError(services.CodeNotFound, "workspace endpoint not found", map[string]any{"path": r.URL.Path}, nil))
}

func handleIdentityWorkspaces(w http.ResponseWriter, r *http.Request) {
	tc := coretenant.FromContextOrLocal(r.Context())
	workspace := daemonIdentityWorkspaceFromTenantContext(tc)
	writeOK(w, http.StatusOK, map[string]any{
		"workspaces": []daemonIdentityWorkspacePayload{workspace},
		"selected":   workspace,
	})
}

func handleIdentityWorkspaceSelect(w http.ResponseWriter, r *http.Request, workspaceID string) {
	tc := coretenant.FromContextOrLocal(r.Context())
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID != tc.WorkspaceID {
		writeServiceError(w, services.NewError(services.CodeNotFound, "workspace not found", map[string]any{"workspace_id": workspaceID}, nil))
		return
	}
	workspace := daemonIdentityWorkspaceFromTenantContext(tc)
	writeOK(w, http.StatusOK, map[string]any{
		"workspace": workspace,
		"selected":  workspace,
	})
}

func daemonIdentityPrincipalFromTenantContext(tc coretenant.TenantContext) daemonIdentityPrincipalPayload {
	principal := tc.Principal.Clone()
	if principal.SubjectID == "" {
		principal = coretenant.LocalPrincipal()
	}
	return daemonIdentityPrincipalPayload{
		SubjectID:   principal.SubjectID,
		DisplayName: principal.DisplayName,
		AuthMethod:  principal.AuthMethod,
		Roles:       append([]string(nil), principal.Roles...),
	}
}

func daemonIdentityTenantFromTenantContext(tc coretenant.TenantContext) daemonIdentityTenantPayload {
	tenantID := strings.TrimSpace(tc.TenantID)
	if tenantID == "" {
		tenantID = coretenant.LocalTenantID
	}
	return daemonIdentityTenantPayload{
		ID:       tenantID,
		Label:    daemonIdentityLabel(tenantID),
		Mode:     daemonIdentityMode(tc),
		Selected: true,
	}
}

func daemonIdentityWorkspaceFromTenantContext(tc coretenant.TenantContext) daemonIdentityWorkspacePayload {
	tenantID := strings.TrimSpace(tc.TenantID)
	if tenantID == "" {
		tenantID = coretenant.LocalTenantID
	}
	workspaceID := strings.TrimSpace(tc.WorkspaceID)
	if workspaceID == "" {
		workspaceID = coretenant.DefaultWorkspaceID
	}
	return daemonIdentityWorkspacePayload{
		ID:       workspaceID,
		TenantID: tenantID,
		Label:    daemonIdentityLabel(workspaceID),
		Mode:     daemonIdentityMode(tc),
		Selected: true,
	}
}

func daemonIdentityMode(tc coretenant.TenantContext) string {
	mode := strings.TrimSpace(tc.Mode)
	if mode == "" {
		return coretenant.ModeLocal
	}
	return mode
}

func daemonIdentityLabel(id string) string {
	if id == coretenant.LocalTenantID {
		return "Local"
	}
	return id
}
