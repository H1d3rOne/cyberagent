package services

import (
	"context"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	pluginruntime "github.com/H1d3rOne/cyberagent/internal/plugins"
)

const pluginDoctorSchemaVersion = "0.1.2"

// PluginDoctorReport is the service-layer machine-readable plugin health result.
type PluginDoctorReport struct {
	SchemaVersion string                 `json:"schema_version"`
	OK            bool                   `json:"ok"`
	Count         int                    `json:"count"`
	Plugins       []pluginruntime.Health `json:"plugins"`
}

// PluginService exposes plugin discovery and health checks.
type PluginService struct {
	manager pluginruntime.Manager
}

// NewPluginService creates a plugin service rooted at the provided plugin roots.
func NewPluginService(roots ...string) PluginService {
	return PluginService{manager: pluginruntime.NewManager(roots...)}
}

// ListPlugins discovers plugin manifests.
func (s PluginService) ListPlugins(context.Context) ([]contracts.PluginManifest, error) {
	manifests, err := s.manager.Discover()
	if err != nil {
		return nil, internalError("discover plugins", err)
	}
	return manifests, nil
}

// Doctor checks all discovered plugins.
func (s PluginService) Doctor(ctx context.Context) (PluginDoctorReport, error) {
	manifests, err := s.ListPlugins(ctx)
	if err != nil {
		return PluginDoctorReport{}, err
	}
	healths := make([]pluginruntime.Health, 0, len(manifests))
	overallOK := true
	for _, manifest := range manifests {
		health := pluginruntime.Doctor(manifest)
		if !health.OK {
			overallOK = false
		}
		healths = append(healths, health)
	}
	return PluginDoctorReport{
		SchemaVersion: pluginDoctorSchemaVersion,
		OK:            overallOK,
		Count:         len(healths),
		Plugins:       healths,
	}, nil
}
