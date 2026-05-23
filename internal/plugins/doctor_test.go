package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestDoctorChecksBinaryDependency(t *testing.T) {
	manifest := contracts.PluginManifest{
		ID:      "plugin_python",
		Name:    "python-plugin",
		Version: "0.1.0",
		Type:    contracts.PluginTypeAnalyzer,
		Runtime: "subprocess_jsonl",
		Risk:    contracts.RiskLow,
		Dependencies: []contracts.PluginDependency{
			{Name: "go", Kind: "binary", Required: true},
		},
		Entrypoint: contracts.PluginEntrypoint{Kind: "subprocess", Path: "plugins/python/analyzer.py"},
	}

	health := Doctor(manifest)
	if !health.OK {
		t.Fatalf("health.OK = false, want true: %#v", health)
	}
	if len(health.Dependencies) != 1 {
		t.Fatalf("dependencies = %d, want 1", len(health.Dependencies))
	}
	if !health.Dependencies[0].OK || health.Dependencies[0].Detail == "" {
		t.Fatalf("unexpected dependency status: %#v", health.Dependencies[0])
	}
}

func TestDoctorMarksMissingRequiredDependencyUnhealthy(t *testing.T) {
	manifest := contracts.PluginManifest{
		ID:      "plugin_missing",
		Name:    "missing-plugin",
		Version: "0.1.0",
		Type:    contracts.PluginTypeTool,
		Runtime: "external_command",
		Risk:    contracts.RiskMedium,
		Dependencies: []contracts.PluginDependency{
			{Name: "definitely-not-a-real-cyberagent-binary", Kind: "binary", Required: true},
		},
		Entrypoint: contracts.PluginEntrypoint{Kind: "command_builder", Path: "plugins/missing/plugin.yaml"},
	}

	health := Doctor(manifest)
	if health.OK {
		t.Fatalf("health.OK = true, want false")
	}
	if len(health.Dependencies) != 1 || health.Dependencies[0].OK || health.Dependencies[0].Detail != "not found in PATH" {
		t.Fatalf("unexpected dependency status: %#v", health.Dependencies)
	}
}

func TestDoctorAllowsMissingOptionalUnsupportedDependency(t *testing.T) {
	manifest := contracts.PluginManifest{
		ID:      "plugin_optional",
		Name:    "optional-plugin",
		Version: "0.1.0",
		Type:    contracts.PluginTypeTool,
		Runtime: "external_command",
		Risk:    contracts.RiskLow,
		Dependencies: []contracts.PluginDependency{
			{Name: "something", Kind: "custom", Required: false},
		},
		Entrypoint: contracts.PluginEntrypoint{Kind: "command_builder", Path: "plugins/optional/plugin.yaml"},
	}

	health := Doctor(manifest)
	if !health.OK {
		t.Fatalf("health.OK = false, want true")
	}
	if len(health.Dependencies) != 1 || !health.Dependencies[0].OK || health.Dependencies[0].Detail != "unsupported dependency kind" {
		t.Fatalf("unexpected dependency status: %#v", health.Dependencies)
	}
}

func TestDoctorChecksDeclaredSchemas(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.schema.json")
	outputPath := filepath.Join(dir, "output.schema.json")
	if err := os.WriteFile(inputPath, []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatalf("write input schema: %v", err)
	}
	if err := os.WriteFile(outputPath, []byte(`{"type":"object"}`), 0o644); err != nil {
		t.Fatalf("write output schema: %v", err)
	}
	manifest := contracts.PluginManifest{
		ID:           "plugin_schema",
		Name:         "schema-plugin",
		Version:      "0.1.0",
		Type:         contracts.PluginTypeAnalyzer,
		Runtime:      "subprocess_jsonl",
		Risk:         contracts.RiskLow,
		InputSchema:  inputPath,
		OutputSchema: outputPath,
		Entrypoint:   contracts.PluginEntrypoint{Kind: "subprocess", Path: "plugins/schema/analyzer.py"},
	}

	health := Doctor(manifest)
	if !health.OK {
		t.Fatalf("health.OK = false, want true: %#v", health)
	}
	if len(health.Schemas) != 2 {
		t.Fatalf("schemas = %d, want 2: %#v", len(health.Schemas), health.Schemas)
	}
	for _, schema := range health.Schemas {
		if !schema.OK || !strings.Contains(schema.Detail, "valid JSON schema") {
			t.Fatalf("unexpected schema status: %#v", schema)
		}
	}
}

func TestDoctorMarksMissingSchemaUnhealthy(t *testing.T) {
	manifest := contracts.PluginManifest{
		ID:          "plugin_missing_schema",
		Name:        "missing-schema-plugin",
		Version:     "0.1.0",
		Type:        contracts.PluginTypeAnalyzer,
		Runtime:     "subprocess_jsonl",
		Risk:        contracts.RiskLow,
		InputSchema: filepath.Join(t.TempDir(), "missing.schema.json"),
		Entrypoint:  contracts.PluginEntrypoint{Kind: "subprocess", Path: "plugins/schema/analyzer.py"},
	}

	health := Doctor(manifest)
	if health.OK {
		t.Fatalf("health.OK = true, want false")
	}
	if len(health.Schemas) != 1 || health.Schemas[0].OK || health.Schemas[0].Detail != "schema file not found" {
		t.Fatalf("unexpected schema status: %#v", health.Schemas)
	}
}

func TestDoctorMarksInvalidSchemaUnhealthy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.schema.json")
	if err := os.WriteFile(path, []byte(`{"properties":`), 0o644); err != nil {
		t.Fatalf("write invalid schema: %v", err)
	}
	manifest := contracts.PluginManifest{
		ID:           "plugin_invalid_schema",
		Name:         "invalid-schema-plugin",
		Version:      "0.1.0",
		Type:         contracts.PluginTypeAnalyzer,
		Runtime:      "subprocess_jsonl",
		Risk:         contracts.RiskLow,
		OutputSchema: path,
		Entrypoint:   contracts.PluginEntrypoint{Kind: "subprocess", Path: "plugins/schema/analyzer.py"},
	}

	health := Doctor(manifest)
	if health.OK {
		t.Fatalf("health.OK = true, want false")
	}
	if len(health.Schemas) != 1 || health.Schemas[0].OK || !strings.Contains(health.Schemas[0].Detail, "invalid JSON schema") {
		t.Fatalf("unexpected schema status: %#v", health.Schemas)
	}
}
