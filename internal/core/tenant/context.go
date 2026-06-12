// Package tenant defines the request-scoping primitives used by future
// multi-user CyberAgent deployments. The initial implementation is a no-op
// local resolver so existing daemon, dashboard, and desktop flows keep their
// current single-user behavior until L3 wires the context into routers and
// services.
package tenant

import (
	"context"
	"net/http"
)

const (
	// ModeLocal is the compatibility mode for Local Daemon, Dashboard, and
	// Desktop/Wails flows. It must not imply multi-user isolation.
	ModeLocal = "local"
	// ModeMultiUser is reserved for the explicit future Web platform mode.
	ModeMultiUser = "multi_user"

	// LocalSubjectID is the stable subject used by the no-op local resolver.
	LocalSubjectID = "local-user"
	// LocalTenantID is the stable tenant used by the no-op local resolver.
	LocalTenantID = "local"
	// DefaultWorkspaceID is the stable workspace used by the no-op local resolver.
	DefaultWorkspaceID = "default"

	// AuthMethodLocalBearer records that the principal came from the current
	// local daemon bearer-token boundary. The token value is never stored here.
	AuthMethodLocalBearer = "local_bearer"
)

// Principal is the authenticated subject snapshot attached to a request. It is
// intentionally small so it can be produced by local bearer auth today and by
// cookie/session, PAT, service-account, or OIDC adapters later.
type Principal struct {
	SubjectID   string
	DisplayName string
	AuthMethod  string
	Roles       []string
}

// Clone returns an isolated copy of the principal.
func (p Principal) Clone() Principal {
	p.Roles = append([]string(nil), p.Roles...)
	return p
}

// TenantContext is the service-layer scope for tenant/workspace owned data.
// Local mode defaults to local/default and must preserve existing runsDir and
// daemon route behavior until multi-user mode is explicitly enabled later.
type TenantContext struct {
	TenantID    string
	WorkspaceID string
	Principal   Principal
	Mode        string
}

// Clone returns an isolated copy of the tenant context.
func (tc TenantContext) Clone() TenantContext {
	tc.Principal = tc.Principal.Clone()
	return tc
}

// TenantResolver resolves the effective tenant/workspace for a request after
// authentication. The request may be nil for non-HTTP callers such as tests or
// future CLI/worker adapters.
type TenantResolver interface {
	ResolveTenant(ctx context.Context, r *http.Request, principal Principal) (TenantContext, error)
}

// LocalPrincipal returns the compatibility principal for the current local
// daemon model. It carries no token value and grants no web-admin semantics.
func LocalPrincipal() Principal {
	return Principal{
		SubjectID:   LocalSubjectID,
		DisplayName: "Local User",
		AuthMethod:  AuthMethodLocalBearer,
		Roles:       []string{"local"},
	}
}

// LocalContext returns the no-op local tenant/workspace scope used before L3
// router and service scoping are wired in.
func LocalContext() TenantContext {
	return TenantContext{
		TenantID:    LocalTenantID,
		WorkspaceID: DefaultWorkspaceID,
		Principal:   LocalPrincipal(),
		Mode:        ModeLocal,
	}
}

// LocalResolver always resolves to the local/default scope. It is the first L3
// skeleton resolver and intentionally does not read headers, cookies, URL
// params, or frontend-supplied tenant IDs.
type LocalResolver struct{}

// ResolveTenant implements TenantResolver.
func (LocalResolver) ResolveTenant(ctx context.Context, _ *http.Request, principal Principal) (TenantContext, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if principal.SubjectID == "" {
		principal = LocalPrincipal()
	} else {
		principal = principal.Clone()
	}
	return TenantContext{
		TenantID:    LocalTenantID,
		WorkspaceID: DefaultWorkspaceID,
		Principal:   principal,
		Mode:        ModeLocal,
	}, nil
}

type contextKey struct{}

// WithContext stores a cloned TenantContext in ctx. Nil ctx is normalized to
// context.Background for convenience in tests and local adapters.
func WithContext(ctx context.Context, tc TenantContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, tc.Clone())
}

// FromContext reads a cloned TenantContext from ctx.
func FromContext(ctx context.Context) (TenantContext, bool) {
	if ctx == nil {
		return TenantContext{}, false
	}
	tc, ok := ctx.Value(contextKey{}).(TenantContext)
	if !ok {
		return TenantContext{}, false
	}
	return tc.Clone(), true
}

// FromContextOrLocal reads a cloned TenantContext from ctx, falling back to the
// stable local/default compatibility scope when the caller has not been wired
// through an HTTP tenant middleware yet. It intentionally does not derive scope
// from request headers, cookies, URL params, or frontend-supplied identifiers.
func FromContextOrLocal(ctx context.Context) TenantContext {
	if tc, ok := FromContext(ctx); ok {
		return tc
	}
	return LocalContext()
}
