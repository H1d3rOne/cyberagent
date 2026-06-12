package daemon

import (
	"context"

	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

// daemonTenantContext reads the effective tenant/workspace scope for daemon
// service entrypoints. In local mode this is a no-op compatibility read: daemon
// storage continues to use the configured runsDir/provider store/runtime DB
// until a later explicit multi-user storage resolver is introduced.
func daemonTenantContext(ctx context.Context) (coretenant.TenantContext, error) {
	if err := contextError(ctx); err != nil {
		return coretenant.TenantContext{}, err
	}
	return coretenant.FromContextOrLocal(ctx), nil
}

func daemonContextWithTenant(parent context.Context, tc coretenant.TenantContext) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	return coretenant.WithContext(parent, tc)
}
