package daemon

import (
	"net/http"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

func withLocalTenantContext(next http.Handler) http.Handler {
	return withTenantContext(coretenant.LocalResolver{}, coretenant.LocalPrincipal(), next)
}

func withRequestTenantContext(auth WebSessionAuthConfig, next http.Handler) http.Handler {
	local := withLocalTenantContext(next)
	if !auth.Enabled || !auth.PrincipalResolverEnabled {
		return local
	}
	if next == nil {
		next = http.HandlerFunc(http.NotFound)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := webSessionFromContext(r.Context())
		if !ok {
			local.ServeHTTP(w, r)
			return
		}
		tc := tenantContextFromWebSession(session)
		next.ServeHTTP(w, r.WithContext(coretenant.WithContext(r.Context(), tc)))
	})
}

func withTenantContext(resolver coretenant.TenantResolver, principal coretenant.Principal, next http.Handler) http.Handler {
	if next == nil {
		next = http.HandlerFunc(http.NotFound)
	}
	if resolver == nil {
		resolver = coretenant.LocalResolver{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, err := resolver.ResolveTenant(r.Context(), r, principal)
		if err != nil {
			writeServiceError(w, services.NewError(services.CodeInternal, "resolve tenant context", nil, err))
			return
		}
		next.ServeHTTP(w, r.WithContext(coretenant.WithContext(r.Context(), tc)))
	})
}

func tenantContextFromWebSession(session coretenant.Session) coretenant.TenantContext {
	session = session.Clone()
	tenantID := strings.TrimSpace(session.TenantID)
	if tenantID == "" {
		tenantID = coretenant.LocalTenantID
	}
	workspaceID := strings.TrimSpace(session.WorkspaceID)
	if workspaceID == "" {
		workspaceID = coretenant.DefaultWorkspaceID
	}
	subjectID := strings.TrimSpace(session.SubjectID)
	authMethod := strings.TrimSpace(session.AuthMethod)
	if authMethod == "" {
		authMethod = coretenant.AuthMethodWebSession
	}
	return coretenant.TenantContext{
		TenantID:    tenantID,
		WorkspaceID: workspaceID,
		Principal: coretenant.Principal{
			SubjectID:   subjectID,
			DisplayName: subjectID,
			AuthMethod:  authMethod,
			Roles:       tenantRoleNamesFromSession(session.Roles),
		},
		Mode: coretenant.ModeMultiUser,
	}
}

func tenantRoleNamesFromSession(roles []coretenant.Role) []string {
	if len(roles) == 0 {
		return nil
	}
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, string(role))
	}
	return names
}
