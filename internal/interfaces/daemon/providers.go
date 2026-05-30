package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
)

type providerStore struct {
	path string
	mu   sync.Mutex
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

func (s *providerStore) Load() (web.ProviderConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
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

func (s *providerStore) Save(cfg web.ProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cfg.Providers == nil {
		cfg.Providers = []web.ProviderProfile{}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
