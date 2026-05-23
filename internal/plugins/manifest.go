package plugins

import (
	"fmt"
	"os"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"gopkg.in/yaml.v3"
)

// LoadManifest reads one plugin manifest YAML file.
func LoadManifest(path string) (contracts.PluginManifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return contracts.PluginManifest{}, fmt.Errorf("read plugin manifest %s: %w", path, err)
	}
	var manifest contracts.PluginManifest
	if err := yaml.Unmarshal(body, &manifest); err != nil {
		return contracts.PluginManifest{}, fmt.Errorf("decode plugin manifest %s: %w", path, err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return contracts.PluginManifest{}, fmt.Errorf("invalid plugin manifest %s: %w", path, err)
	}
	return manifest, nil
}

// ValidateManifest checks MVP required fields.
func ValidateManifest(manifest contracts.PluginManifest) error {
	if manifest.ID == "" {
		return fmt.Errorf("id is required")
	}
	if manifest.Name == "" {
		return fmt.Errorf("name is required")
	}
	if manifest.Version == "" {
		return fmt.Errorf("version is required")
	}
	if manifest.Type == "" {
		return fmt.Errorf("type is required")
	}
	if manifest.Runtime == "" {
		return fmt.Errorf("runtime is required")
	}
	if manifest.Risk == "" {
		return fmt.Errorf("risk is required")
	}
	if manifest.Entrypoint.Kind == "" || manifest.Entrypoint.Path == "" {
		return fmt.Errorf("entrypoint kind and path are required")
	}
	return nil
}
