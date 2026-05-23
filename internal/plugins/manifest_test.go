package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.yaml")
	content := `id: plugin_test
name: test-plugin
version: 0.1.0
type: tool
runtime: external_command
description: Test plugin.
capabilities:
  - test_capability
risk: low
requires_approval: false
entrypoint:
  kind: command_builder
  path: plugins/test/plugin.yaml
artifact_policy:
  store_stdout: true
  store_stderr: true
  redact: true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if manifest.ID != "plugin_test" || manifest.Name != "test-plugin" {
		t.Fatalf("unexpected manifest identity: %#v", manifest)
	}
	if manifest.Type != contracts.PluginTypeTool {
		t.Fatalf("type = %s, want %s", manifest.Type, contracts.PluginTypeTool)
	}
	if manifest.Risk != contracts.RiskLow {
		t.Fatalf("risk = %s, want %s", manifest.Risk, contracts.RiskLow)
	}
	if len(manifest.Capabilities) != 1 || manifest.Capabilities[0] != "test_capability" {
		t.Fatalf("unexpected capabilities: %#v", manifest.Capabilities)
	}
}

func TestLoadManifestRejectsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugin.yaml")
	if err := os.WriteFile(path, []byte("id: ["), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := LoadManifest(path)
	if err == nil {
		t.Fatalf("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode plugin manifest") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateManifestRequiredFields(t *testing.T) {
	valid := contracts.PluginManifest{
		ID:      "plugin_valid",
		Name:    "valid",
		Version: "0.1.0",
		Type:    contracts.PluginTypeTool,
		Runtime: "external_command",
		Risk:    contracts.RiskLow,
		Entrypoint: contracts.PluginEntrypoint{
			Kind: "command_builder",
			Path: "plugins/valid/plugin.yaml",
		},
	}
	if err := ValidateManifest(valid); err != nil {
		t.Fatalf("ValidateManifest(valid) error = %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*contracts.PluginManifest)
		wantErr string
	}{
		{name: "id", mutate: func(m *contracts.PluginManifest) { m.ID = "" }, wantErr: "id is required"},
		{name: "name", mutate: func(m *contracts.PluginManifest) { m.Name = "" }, wantErr: "name is required"},
		{name: "version", mutate: func(m *contracts.PluginManifest) { m.Version = "" }, wantErr: "version is required"},
		{name: "type", mutate: func(m *contracts.PluginManifest) { m.Type = "" }, wantErr: "type is required"},
		{name: "runtime", mutate: func(m *contracts.PluginManifest) { m.Runtime = "" }, wantErr: "runtime is required"},
		{name: "risk", mutate: func(m *contracts.PluginManifest) { m.Risk = "" }, wantErr: "risk is required"},
		{name: "entrypoint kind", mutate: func(m *contracts.PluginManifest) { m.Entrypoint.Kind = "" }, wantErr: "entrypoint kind and path are required"},
		{name: "entrypoint path", mutate: func(m *contracts.PluginManifest) { m.Entrypoint.Path = "" }, wantErr: "entrypoint kind and path are required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := valid
			tt.mutate(&manifest)
			err := ValidateManifest(manifest)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
