package services

import (
	"context"

	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

// tenantContextForServiceEntry reads the effective tenant/workspace scope for a
// service call while preserving the current local-mode storage behavior. L3-A4
// wires this into key entrypoints as a no-op read: services can observe
// TenantContext, but runsDir/provider/runtime paths are not derived from it
// until a later explicit multi-user storage resolver is enabled.
func tenantContextForServiceEntry(ctx context.Context) (coretenant.TenantContext, error) {
	if err := ctxErr(ctx); err != nil {
		return coretenant.TenantContext{}, err
	}
	return coretenant.FromContextOrLocal(ctx), nil
}
