package tenant

import (
	"testing"
	"time"
)

func TestL3AuthModelRolesAreExplicitAndScoped(t *testing.T) {
	tests := []struct {
		raw       string
		want      Role
		wantScope string
	}{
		{raw: " tenant_owner ", want: RoleTenantOwner, wantScope: "tenant"},
		{raw: "TENANT_VIEWER", want: RoleTenantViewer, wantScope: "tenant"},
		{raw: "workspace_admin", want: RoleWorkspaceAdmin, wantScope: "workspace"},
		{raw: "workspace_analyst", want: RoleWorkspaceAnalyst, wantScope: "workspace"},
	}
	for _, tc := range tests {
		role, ok := NormalizeRole(tc.raw)
		if !ok {
			t.Fatalf("NormalizeRole(%q) ok=false, want true", tc.raw)
		}
		if role != tc.want || role.Scope() != tc.wantScope || !role.Valid() {
			t.Fatalf("NormalizeRole(%q) = %q scope=%q valid=%v, want %q/%q/true", tc.raw, role, role.Scope(), role.Valid(), tc.want, tc.wantScope)
		}
	}

	for _, raw := range []string{"", "local", "owner", "tenant_root", "workspace/superuser"} {
		if role, ok := NormalizeRole(raw); ok || role != "" {
			t.Fatalf("NormalizeRole(%q) = %q, %v; want empty/false", raw, role, ok)
		}
	}
}

func TestL3AuthModelCloneIsolatesMutableFields(t *testing.T) {
	revokedAt := time.Now().UTC()
	wantRevokedAt := revokedAt
	session := Session{
		ID:          "session-1",
		SubjectID:   "user-1",
		AuthMethod:  AuthMethodWebSession,
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		Roles:       []Role{RoleTenantAdmin, RoleWorkspaceOperator},
		RevokedAt:   &revokedAt,
	}
	clonedSession := session.Clone()
	session.Roles[0] = RoleTenantViewer
	*session.RevokedAt = revokedAt.Add(time.Hour)
	if clonedSession.Roles[0] != RoleTenantAdmin {
		t.Fatalf("Session.Clone() roles aliased caller slice: %#v", clonedSession.Roles)
	}
	if !clonedSession.RevokedAt.Equal(wantRevokedAt) {
		t.Fatalf("Session.Clone() revoked_at pointer aliased caller value: %v", clonedSession.RevokedAt)
	}

	tenantMember := TenantMembership{TenantID: "tenant-a", SubjectID: "user-1", Roles: []Role{RoleTenantOwner}}
	clonedTenantMember := tenantMember.Clone()
	tenantMember.Roles[0] = RoleTenantViewer
	if clonedTenantMember.Roles[0] != RoleTenantOwner {
		t.Fatalf("TenantMembership.Clone() roles aliased caller slice: %#v", clonedTenantMember.Roles)
	}

	workspaceMember := WorkspaceMembership{TenantID: "tenant-a", WorkspaceID: "workspace-a", SubjectID: "user-1", Roles: []Role{RoleWorkspaceAnalyst}}
	clonedWorkspaceMember := workspaceMember.Clone()
	workspaceMember.Roles[0] = RoleWorkspaceViewer
	if clonedWorkspaceMember.Roles[0] != RoleWorkspaceAnalyst {
		t.Fatalf("WorkspaceMembership.Clone() roles aliased caller slice: %#v", clonedWorkspaceMember.Roles)
	}
}

func TestL3AuthModelLocalPrincipalDoesNotGrantWebRBACRoles(t *testing.T) {
	principal := LocalPrincipal()
	if principal.AuthMethod != AuthMethodLocalBearer {
		t.Fatalf("LocalPrincipal().AuthMethod = %q, want %q", principal.AuthMethod, AuthMethodLocalBearer)
	}
	for _, raw := range principal.Roles {
		if role, ok := NormalizeRole(raw); ok || role != "" {
			t.Fatalf("local bearer role %q should not normalize to future RBAC role %q", raw, role)
		}
	}
}
