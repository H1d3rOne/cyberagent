package daemon

import (
	"os"
	"path/filepath"
	"sync"
)

// assetDirNames are the seed-data directories shipped with the repository that
// the capability layer reads at runtime (security-tool catalog, MCP presets,
// skill definitions).
var assetDirNames = []string{"security-tools", "mcp", "skills"}

var (
	assetRootOnce sync.Once
	assetRootDir  string
)

// agentAssetRoot resolves the directory that contains the seed-data dirs. It
// honors the CYBERAGENT_ASSETS_DIR override, then searches upward from the
// working directory and the executable directory for a directory that holds the
// expected asset folders, and finally falls back to the working directory. The
// result is cached for the process lifetime.
func agentAssetRoot() string {
	assetRootOnce.Do(func() {
		assetRootDir = resolveAgentAssetRoot()
	})
	return assetRootDir
}

func resolveAgentAssetRoot() string {
	if override := os.Getenv("CYBERAGENT_ASSETS_DIR"); override != "" {
		return override
	}
	starts := make([]string, 0, 2)
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		if root, ok := searchUpwardForAssets(start); ok {
			return root
		}
	}
	if len(starts) > 0 {
		return starts[0]
	}
	return "."
}

func searchUpwardForAssets(start string) (string, bool) {
	dir := start
	for {
		if hasAllAssetDirs(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func hasAllAssetDirs(dir string) bool {
	for _, name := range assetDirNames {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

// agentAssetPath joins one or more path elements onto the resolved asset root.
func agentAssetPath(elem ...string) string {
	return filepath.Join(append([]string{agentAssetRoot()}, elem...)...)
}
