package tenant

import (
	"strings"
	"time"
)

const (
	// AuthMethodWebSession is reserved for future browser login sessions backed
	// by server-side state and HttpOnly cookies. It is intentionally not wired
	// into the local daemon bearer-token chain yet.
	AuthMethodWebSession = "web_session"
	// AuthMethodPersonalAccessToken is reserved for future scoped API tokens.
	AuthMethodPersonalAccessToken = "personal_access_token"
	// AuthMethodServiceAccountToken is reserved for future automation tokens.
	AuthMethodServiceAccountToken = "service_account_token"
	// AuthMethodOIDC is reserved for future SSO/OIDC adapters.
	AuthMethodOIDC = "oidc"
	// AuthMethodPassword is reserved for a future local account adapter.
	AuthMethodPassword = "password"
)

type UserType string

const (
	UserTypeHuman          UserType = "human"
	UserTypeServiceAccount UserType = "service_account"
)

type UserStatus string

const (
	UserStatusPending  UserStatus = "pending"
	UserStatusActive   UserStatus = "active"
	UserStatusDisabled UserStatus = "disabled"
)

// Role names the future server-side RBAC roles. These constants are only a
// model skeleton for L3 and do not grant privileges in the current local daemon
// bearer-token mode.
type Role string

const (
	RoleTenantOwner    Role = "tenant_owner"
	RoleTenantAdmin    Role = "tenant_admin"
	RoleTenantOperator Role = "tenant_operator"
	RoleTenantViewer   Role = "tenant_viewer"

	RoleWorkspaceAdmin    Role = "workspace_admin"
	RoleWorkspaceOperator Role = "workspace_operator"
	RoleWorkspaceAnalyst  Role = "workspace_analyst"
	RoleWorkspaceViewer   Role = "workspace_viewer"
)

// User is a durable account subject for future multi-user deployments. Local
// daemon mode continues to use LocalPrincipal rather than this account model.
type User struct {
	ID          string
	DisplayName string
	Email       string
	Type        UserType
	Status      UserStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Session is the future HTTP/API authentication session model. It is distinct
// from Agent sessions and runtime task/run/turn sessions.
type Session struct {
	ID          string
	SubjectID   string
	AuthMethod  string
	TenantID    string
	WorkspaceID string
	Roles       []Role
	CreatedAt   time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
}

// Clone returns an isolated copy of the session roles and revocation time.
func (s Session) Clone() Session {
	s.Roles = append([]Role(nil), s.Roles...)
	if s.RevokedAt != nil {
		revokedAt := *s.RevokedAt
		s.RevokedAt = &revokedAt
	}
	return s
}

// TenantMembership binds a subject to roles at tenant scope.
type TenantMembership struct {
	TenantID  string
	SubjectID string
	Roles     []Role
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Clone returns an isolated copy of tenant membership roles.
func (m TenantMembership) Clone() TenantMembership {
	m.Roles = append([]Role(nil), m.Roles...)
	return m
}

// WorkspaceMembership binds a subject to roles at workspace scope inside a
// tenant.
type WorkspaceMembership struct {
	TenantID    string
	WorkspaceID string
	SubjectID   string
	Roles       []Role
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Clone returns an isolated copy of workspace membership roles.
func (m WorkspaceMembership) Clone() WorkspaceMembership {
	m.Roles = append([]Role(nil), m.Roles...)
	return m
}

// NormalizeRole trims and normalizes a role string. Unknown roles return an
// empty role with ok=false so future RBAC stores can reject misspellings rather
// than silently granting unintended permissions.
func NormalizeRole(raw string) (Role, bool) {
	role := Role(strings.ToLower(strings.TrimSpace(raw)))
	if !role.Valid() {
		return "", false
	}
	return role, true
}

func (r Role) Valid() bool {
	switch r {
	case RoleTenantOwner, RoleTenantAdmin, RoleTenantOperator, RoleTenantViewer,
		RoleWorkspaceAdmin, RoleWorkspaceOperator, RoleWorkspaceAnalyst, RoleWorkspaceViewer:
		return true
	default:
		return false
	}
}

func (r Role) Scope() string {
	switch r {
	case RoleTenantOwner, RoleTenantAdmin, RoleTenantOperator, RoleTenantViewer:
		return "tenant"
	case RoleWorkspaceAdmin, RoleWorkspaceOperator, RoleWorkspaceAnalyst, RoleWorkspaceViewer:
		return "workspace"
	default:
		return ""
	}
}
