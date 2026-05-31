package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentAssetRootResolvesSeedDirs(t *testing.T) {
	root := agentAssetRoot()
	for _, name := range assetDirNames {
		if info, err := os.Stat(filepath.Join(root, name)); err != nil || !info.IsDir() {
			t.Fatalf("asset root %q is missing seed dir %q: %v", root, name, err)
		}
	}
}

func TestAgentSecurityToolCatalogLoadsFromAssetRoot(t *testing.T) {
	catalog := agentSecurityToolCatalog()
	if len(catalog) == 0 {
		t.Fatalf("expected security-tool catalog to load from asset root, got empty")
	}
	for _, spec := range catalog {
		if spec.Name == "" {
			t.Fatalf("catalog entry missing name: %#v", spec)
		}
	}
}

func TestSearchUpwardForAssetsFromNestedDir(t *testing.T) {
	root := agentAssetRoot()
	nested := filepath.Join(root, "internal", "interfaces", "daemon")
	if _, err := os.Stat(nested); err != nil {
		t.Skipf("nested dir not present: %v", err)
	}
	found, ok := searchUpwardForAssets(nested)
	if !ok {
		t.Fatalf("expected upward search from %q to find asset root", nested)
	}
	if found != root {
		t.Fatalf("upward search found %q, want %q", found, root)
	}
}
