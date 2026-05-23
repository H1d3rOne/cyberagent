package contracts

// PluginType identifies plugin behavior.
type PluginType string

const (
	PluginTypeTool           PluginType = "tool"
	PluginTypeParser         PluginType = "parser"
	PluginTypeAnalyzer       PluginType = "analyzer"
	PluginTypeReporter       PluginType = "reporter"
	PluginTypeLLMProvider    PluginType = "llm_provider"
	PluginTypeContextBuilder PluginType = "context_builder"
)

// PluginManifest describes a plugin capability and its constraints.
type PluginManifest struct {
	ID                string             `json:"id" yaml:"id"`
	Name              string             `json:"name" yaml:"name"`
	Version           string             `json:"version" yaml:"version"`
	Type              PluginType         `json:"type" yaml:"type"`
	Runtime           string             `json:"runtime" yaml:"runtime"`
	Description       string             `json:"description,omitempty" yaml:"description,omitempty"`
	Capabilities      []string           `json:"capabilities" yaml:"capabilities"`
	Risk              RiskLevel          `json:"risk" yaml:"risk"`
	RequiresApproval  bool               `json:"requires_approval" yaml:"requires_approval"`
	Dependencies      []PluginDependency `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	InputSchema       string             `json:"input_schema,omitempty" yaml:"input_schema,omitempty"`
	OutputSchema      string             `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
	ArtifactPolicy    ArtifactPolicy     `json:"artifact_policy" yaml:"artifact_policy"`
	SupportedProfiles []string           `json:"supported_profiles,omitempty" yaml:"supported_profiles,omitempty"`
	Entrypoint        PluginEntrypoint   `json:"entrypoint" yaml:"entrypoint"`
}

// PluginDependency describes an external dependency.
type PluginDependency struct {
	Name              string `json:"name" yaml:"name"`
	Kind              string `json:"kind" yaml:"kind"`
	VersionConstraint string `json:"version_constraint,omitempty" yaml:"version_constraint,omitempty"`
	Required          bool   `json:"required" yaml:"required"`
}

// ArtifactPolicy describes what plugin outputs must be retained.
type ArtifactPolicy struct {
	StoreStdout bool     `json:"store_stdout" yaml:"store_stdout"`
	StoreStderr bool     `json:"store_stderr" yaml:"store_stderr"`
	StoreFiles  []string `json:"store_files,omitempty" yaml:"store_files,omitempty"`
	Redact      bool     `json:"redact" yaml:"redact"`
}

// PluginEntrypoint describes how a plugin starts.
type PluginEntrypoint struct {
	Kind string `json:"kind" yaml:"kind"`
	Path string `json:"path" yaml:"path"`
}
