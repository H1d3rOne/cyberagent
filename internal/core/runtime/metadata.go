package runtime

import (
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	pluginruntime "github.com/H1d3rOne/cyberagent/internal/plugins"
	"github.com/H1d3rOne/cyberagent/internal/runtime/artifacts"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

const runSnapshotSchemaVersion = "0.1.1"
const defaultModulePath = "github.com/H1d3rOne/cyberagent"

// BuildInfo captures binary metadata injected by the CLI/app layer.
type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Module  string `json:"module"`
}

// RunMetadataSnapshot records run provenance that should survive replay/report handoff.
type RunMetadataSnapshot struct {
	SchemaVersion string               `json:"schema_version"`
	CreatedAt     time.Time            `json:"created_at"`
	JobID         string               `json:"job_id"`
	Workspace     string               `json:"workspace"`
	Binary        BuildInfo            `json:"binary"`
	Module        string               `json:"module"`
	Command       CommandSnapshot      `json:"command"`
	Runtime       GoRuntimeSnapshot    `json:"runtime"`
	TargetID      string               `json:"target_id"`
	ScopeID       string               `json:"scope_id"`
	Profile       string               `json:"profile"`
	Request       WebScanRequestDigest `json:"request"`
}

// CommandSnapshot describes the user-facing entrypoint used for the run.
type CommandSnapshot struct {
	Name string   `json:"name"`
	Args []string `json:"args,omitempty"`
}

// GoRuntimeSnapshot captures platform details useful for debugging plugin/runtime behavior.
type GoRuntimeSnapshot struct {
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GoVersion string `json:"go_version"`
	NumCPU    int    `json:"num_cpu"`
}

// WebScanRequestDigest is a redaction-friendly subset of the scan request.
type WebScanRequestDigest struct {
	Target          string   `json:"target"`
	Scope           string   `json:"scope"`
	Profile         string   `json:"profile"`
	Out             string   `json:"out"`
	EnabledPlugins  []string `json:"enabled_plugins,omitempty"`
	NoExternalTools bool     `json:"no_external_tools"`
	Interactive     bool     `json:"interactive"`
}

// EffectiveConfigSnapshot records the post-defaulting configuration that actually drove the run.
type EffectiveConfigSnapshot struct {
	SchemaVersion   string    `json:"schema_version"`
	GeneratedAt     time.Time `json:"generated_at"`
	Profile         string    `json:"profile"`
	Target          string    `json:"target"`
	Scope           string    `json:"scope"`
	Out             string    `json:"out"`
	EnabledPlugins  []string  `json:"enabled_plugins,omitempty"`
	NoExternalTools bool      `json:"no_external_tools"`
	Interactive     bool      `json:"interactive"`
	TargetID        string    `json:"target_id"`
	ScopeID         string    `json:"scope_id"`
	JobID           string    `json:"job_id"`
}

// PluginSnapshot records plugin discovery state at run creation time.
type PluginSnapshot struct {
	SchemaVersion  string                `json:"schema_version"`
	GeneratedAt    time.Time             `json:"generated_at"`
	Enabled        []string              `json:"enabled,omitempty"`
	Disabled       []string              `json:"disabled,omitempty"`
	Discovered     []PluginSnapshotEntry `json:"discovered,omitempty"`
	DiscoveryRoots []string              `json:"discovery_roots,omitempty"`
	Errors         []string              `json:"errors,omitempty"`
}

// PluginSnapshotEntry is a compact, stable copy of plugin manifest metadata.
type PluginSnapshotEntry struct {
	ID                string                       `json:"id"`
	Name              string                       `json:"name"`
	Version           string                       `json:"version"`
	Type              contracts.PluginType         `json:"type"`
	Runtime           string                       `json:"runtime"`
	Description       string                       `json:"description,omitempty"`
	Capabilities      []string                     `json:"capabilities,omitempty"`
	Risk              contracts.RiskLevel          `json:"risk"`
	RequiresApproval  bool                         `json:"requires_approval"`
	Dependencies      []contracts.PluginDependency `json:"dependencies,omitempty"`
	InputSchema       string                       `json:"input_schema,omitempty"`
	OutputSchema      string                       `json:"output_schema,omitempty"`
	SupportedProfiles []string                     `json:"supported_profiles,omitempty"`
	Entrypoint        contracts.PluginEntrypoint   `json:"entrypoint"`
}

func writeRunSnapshots(store artifacts.Store, layout runs.Layout, job contracts.Job, target contracts.Target, scope contracts.Scope, req WebScanRequest, profile string) error {
	enabled := cloneStrings(enabledPlugins(req))
	now := contracts.Now()

	if err := store.WriteJSON("metadata.json", buildRunMetadataSnapshot(layout, job, target, scope, req, profile, enabled, now)); err != nil {
		return err
	}
	if err := store.WriteJSON("effective_config.json", buildEffectiveConfigSnapshot(layout, job, target, scope, req, profile, enabled, now)); err != nil {
		return err
	}
	return store.WriteJSON("plugins.snapshot.json", buildPluginSnapshot(enabled, pluginSearchRoots(), now))
}

func buildRunMetadataSnapshot(layout runs.Layout, job contracts.Job, target contracts.Target, scope contracts.Scope, req WebScanRequest, profile string, enabled []string, now time.Time) RunMetadataSnapshot {
	build := normalizeBuildInfo(req.BuildInfo)
	return RunMetadataSnapshot{
		SchemaVersion: runSnapshotSchemaVersion,
		CreatedAt:     now,
		JobID:         job.ID,
		Workspace:     layout.Path,
		Binary:        build,
		Module:        build.Module,
		Command: CommandSnapshot{
			Name: commandName(req.CommandName),
			Args: cloneStrings(req.CommandArgs),
		},
		Runtime: GoRuntimeSnapshot{
			GOOS:      goruntime.GOOS,
			GOARCH:    goruntime.GOARCH,
			GoVersion: goruntime.Version(),
			NumCPU:    goruntime.NumCPU(),
		},
		TargetID: target.ID,
		ScopeID:  scope.ID,
		Profile:  profile,
		Request: WebScanRequestDigest{
			Target:          req.Target,
			Scope:           req.Scope,
			Profile:         profile,
			Out:             layout.Path,
			EnabledPlugins:  cloneStrings(enabled),
			NoExternalTools: req.NoExternalTools,
			Interactive:     req.Interactive,
		},
	}
}

func buildEffectiveConfigSnapshot(layout runs.Layout, job contracts.Job, target contracts.Target, scope contracts.Scope, req WebScanRequest, profile string, enabled []string, now time.Time) EffectiveConfigSnapshot {
	return EffectiveConfigSnapshot{
		SchemaVersion:   runSnapshotSchemaVersion,
		GeneratedAt:     now,
		Profile:         profile,
		Target:          target.Value,
		Scope:           req.Scope,
		Out:             layout.Path,
		EnabledPlugins:  cloneStrings(enabled),
		NoExternalTools: req.NoExternalTools,
		Interactive:     req.Interactive,
		TargetID:        target.ID,
		ScopeID:         scope.ID,
		JobID:           job.ID,
	}
}

func buildPluginSnapshot(enabled []string, roots []string, now time.Time) PluginSnapshot {
	snapshot := PluginSnapshot{
		SchemaVersion:  runSnapshotSchemaVersion,
		GeneratedAt:    now,
		Enabled:        cloneStrings(enabled),
		DiscoveryRoots: cloneStrings(roots),
	}
	seen := map[string]struct{}{}
	for _, root := range roots {
		manifests, err := pluginruntime.NewManager(root).Discover()
		if err != nil {
			snapshot.Errors = append(snapshot.Errors, err.Error())
			continue
		}
		for _, manifest := range manifests {
			key := manifest.ID
			if key == "" {
				key = manifest.Name
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			snapshot.Discovered = append(snapshot.Discovered, pluginSnapshotEntry(manifest))
		}
	}
	snapshot.Disabled = disabledPlugins(snapshot.Discovered, enabled)
	return snapshot
}

func pluginSnapshotEntry(manifest contracts.PluginManifest) PluginSnapshotEntry {
	return PluginSnapshotEntry{
		ID:                manifest.ID,
		Name:              manifest.Name,
		Version:           manifest.Version,
		Type:              manifest.Type,
		Runtime:           manifest.Runtime,
		Description:       manifest.Description,
		Capabilities:      cloneStrings(manifest.Capabilities),
		Risk:              manifest.Risk,
		RequiresApproval:  manifest.RequiresApproval,
		Dependencies:      append([]contracts.PluginDependency(nil), manifest.Dependencies...),
		InputSchema:       manifest.InputSchema,
		OutputSchema:      manifest.OutputSchema,
		SupportedProfiles: cloneStrings(manifest.SupportedProfiles),
		Entrypoint:        manifest.Entrypoint,
	}
}

func disabledPlugins(discovered []PluginSnapshotEntry, enabled []string) []string {
	enabledSet := map[string]struct{}{}
	for _, name := range enabled {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized != "" {
			enabledSet[normalized] = struct{}{}
		}
	}
	var disabled []string
	for _, plugin := range discovered {
		name := strings.TrimSpace(plugin.Name)
		if name == "" {
			name = strings.TrimSpace(plugin.ID)
		}
		if name == "" {
			continue
		}
		if _, ok := enabledSet[strings.ToLower(name)]; !ok {
			disabled = append(disabled, name)
		}
	}
	return disabled
}

func pluginSearchRoots() []string {
	return []string{"plugins", filepath.Join("..", "..", "..", "plugins")}
}

func normalizeBuildInfo(info BuildInfo) BuildInfo {
	if strings.TrimSpace(info.Version) == "" {
		info.Version = "unknown"
	}
	if strings.TrimSpace(info.Commit) == "" {
		info.Commit = "unknown"
	}
	if strings.TrimSpace(info.Date) == "" {
		info.Date = "unknown"
	}
	if strings.TrimSpace(info.Module) == "" {
		info.Module = defaultModulePath
	}
	return info
}

func commandName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "scan web"
	}
	return name
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return append([]string(nil), in...)
}
