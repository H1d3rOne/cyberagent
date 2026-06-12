package daemon

import (
	"context"

	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

func daemonStorageResolverFromConfig(cfg Config) (coretenant.StorageResolver, error) {
	cfg = cfg.Normalize()
	return coretenant.NewStorageResolver(coretenant.StorageConfig{
		LocalRunsDir:           cfg.RunsDir,
		LocalProviderStorePath: defaultProviderStorePath(),
		LocalRuntimeDBPath:     agentRuntimeSQLitePath(cfg.RunsDir),
		MultiUserEnabled:       cfg.MultiUser.Enabled,
		MultiUserStorageRoot:   cfg.MultiUser.StorageRoot,
	})
}

func daemonLocalStoragePathsFromConfig(cfg Config) (coretenant.StoragePaths, coretenant.StorageResolver, error) {
	resolver, err := daemonStorageResolverFromConfig(cfg)
	if err != nil {
		return coretenant.StoragePaths{}, nil, err
	}
	paths, err := resolver.ResolveStorage(context.Background())
	if err != nil {
		return coretenant.StoragePaths{}, nil, err
	}
	return paths, resolver, nil
}
