package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerDiscover(t *testing.T) {
	root := t.TempDir()
	writePluginManifest(t, root, "zeta", `id: plugin_zeta
name: zeta
version: 0.1.0
type: tool
runtime: external_command
risk: low
entrypoint:
  kind: command_builder
  path: plugins/zeta/plugin.yaml
`)
	writePluginManifest(t, root, "alpha", `id: plugin_alpha
name: alpha
version: 0.1.0
type: analyzer
runtime: subprocess_jsonl
risk: low
entrypoint:
  kind: subprocess
  path: plugins/alpha/analyzer.py
`)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "no-manifest"), 0o755); err != nil {
		t.Fatalf("mkdir no-manifest: %v", err)
	}

	manifests, err := NewManager(root).Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("len(manifests) = %d, want 2", len(manifests))
	}
	if manifests[0].Name != "alpha" || manifests[1].Name != "zeta" {
		t.Fatalf("manifests not sorted by name: %#v", manifests)
	}
}

func TestManagerDiscoverSkipsMissingRoot(t *testing.T) {
	manifests, err := NewManager(filepath.Join(t.TempDir(), "missing")).Discover()
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(manifests) != 0 {
		t.Fatalf("len(manifests) = %d, want 0", len(manifests))
	}
}

func TestManagerDiscoverReturnsInvalidManifestError(t *testing.T) {
	root := t.TempDir()
	writePluginManifest(t, root, "bad", `id: plugin_bad
name: bad
version: 0.1.0
`)

	_, err := NewManager(root).Discover()
	if err == nil {
		t.Fatalf("expected invalid manifest error")
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
