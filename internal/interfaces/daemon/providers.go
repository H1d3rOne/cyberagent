package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
)

type providerStore struct {
	path            string
	storageResolver coretenant.StorageResolver
	mu              sync.Mutex
}

func defaultProviderStorePath() string {
	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "cyberagent", "providers.json")
	}
	return filepath.Join(".cyberagent", "providers.json")
}

func newProviderStore(path string) *providerStore {
	if strings.TrimSpace(path) == "" {
		path = defaultProviderStorePath()
	}
	return &providerStore{path: path}
}

func newProviderStoreWithStorageResolver(path string, resolver coretenant.StorageResolver) *providerStore {
	store := newProviderStore(path)
	store.storageResolver = resolver
	return store
}

// NewDefaultProviderStore returns a ProviderStore backed by the per-user JSON
// file. An empty path falls back to the default location resolved by
// defaultProviderStorePath. Exposed so the CLI dashboard can share the same
// persistence as the daemon.
func NewDefaultProviderStore(path string) web.ProviderStore {
	return newProviderStore(path)
}

func (s *providerStore) Load() (web.ProviderConfig, error) {
	return s.loadFromPath(s.path)
}

func (s *providerStore) loadFromPath(path string) (web.ProviderConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(path) == "" {
		path = s.path
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return web.ProviderConfig{Providers: []web.ProviderProfile{}}, nil
		}
		return web.ProviderConfig{}, err
	}
	var cfg web.ProviderConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return web.ProviderConfig{}, err
	}
	if cfg.Providers == nil {
		cfg.Providers = []web.ProviderProfile{}
	}
	return cfg, nil
}

func (s *providerStore) LoadContext(ctx context.Context) (web.ProviderConfig, error) {
	if _, err := daemonTenantContext(ctx); err != nil {
		return web.ProviderConfig{}, err
	}
	path, err := s.pathForContext(ctx)
	if err != nil {
		return web.ProviderConfig{}, err
	}
	return s.loadFromPath(path)
}

func (s *providerStore) Save(cfg web.ProviderConfig) error {
	return s.saveToPath(s.path, cfg)
}

func (s *providerStore) saveToPath(path string, cfg web.ProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(path) == "" {
		path = s.path
	}
	if cfg.Providers == nil {
		cfg.Providers = []web.ProviderProfile{}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (s *providerStore) SaveContext(ctx context.Context, cfg web.ProviderConfig) error {
	if _, err := daemonTenantContext(ctx); err != nil {
		return err
	}
	path, err := s.pathForContext(ctx)
	if err != nil {
		return err
	}
	return s.saveToPath(path, cfg)
}

func (s *providerStore) pathForContext(ctx context.Context) (string, error) {
	if s.storageResolver == nil {
		return s.path, nil
	}
	paths, err := s.storageResolver.ResolveStorage(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(paths.ProviderStorePath) == "" {
		return s.path, nil
	}
	return paths.ProviderStorePath, nil
}
