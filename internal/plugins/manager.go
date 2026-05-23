package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// Manager discovers local plugin manifests.
type Manager struct {
	Roots []string
}

// NewManager creates a plugin manager. Empty roots default to ./plugins.
func NewManager(roots ...string) Manager {
	if len(roots) == 0 {
		roots = []string{"plugins"}
	}
	return Manager{Roots: roots}
}

// Discover returns all valid plugin manifests below configured roots.
func (m Manager) Discover() ([]contracts.PluginManifest, error) {
	var manifests []contracts.PluginManifest
	for _, root := range m.Roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read plugin root %s: %w", root, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name(), "plugin.yaml")
			if _, err := os.Stat(path); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			manifest, err := LoadManifest(path)
			if err != nil {
				return nil, err
			}
			manifests = append(manifests, manifest)
		}
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Name < manifests[j].Name })
	return manifests, nil
}
