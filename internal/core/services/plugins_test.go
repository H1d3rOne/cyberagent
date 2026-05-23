package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestPluginServiceListAndDoctor(t *testing.T) {
	root := t.TempDir()
	writePluginManifest(t, root, "alpha", `id: plugin_alpha
name: alpha
version: 0.1.0
type: tool
runtime: external_command
risk: low
requires_approval: false
dependencies:
  - name: definitely-missing-cyberagent-test-binary
    kind: binary
    required: false
entrypoint:
  kind: command_builder
  path: plugins/alpha/plugin.yaml
artifact_policy:
  store_stdout: true
  store_stderr: true
  redact: true
`)

	service := NewPluginService(root)
	manifests, err := service.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("ListPlugins() error = %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("manifests = %d, want 1", len(manifests))
	}
	if manifests[0].Name != "alpha" || manifests[0].Risk != contracts.RiskLow {
		t.Fatalf("unexpected manifest: %#v", manifests[0])
	}

	report, err := service.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.SchemaVersion != pluginDoctorSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", report.SchemaVersion, pluginDoctorSchemaVersion)
	}
	if report.Count != 1 || len(report.Plugins) != 1 {
		t.Fatalf("unexpected doctor report count: %#v", report)
	}
	if !report.OK || !report.Plugins[0].OK {
		t.Fatalf("optional missing dependency should not make plugin unhealthy: %#v", report)
	}
}

func writePluginManifest(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
}
