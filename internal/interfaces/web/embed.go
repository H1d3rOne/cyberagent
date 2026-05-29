package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed frontend/dist
var embeddedFrontend embed.FS

func FrontendHandler() http.Handler {
	for _, distPath := range frontendDistDirs() {
		if info, err := os.Stat(distPath); err == nil && info.IsDir() {
			return spaHandler(http.Dir(distPath))
		}
	}

	subFS, err := fs.Sub(embeddedFrontend, "frontend/dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><body><h1>Frontend not built</h1><p>Run: cd web/frontend && npm install && npm run build</p></body></html>`))
		})
	}
	return spaHandler(http.FS(subFS))
}

func frontendDistDirs() []string {
	dirs := make([]string, 0, 3)
	seen := map[string]struct{}{}
	add := func(path string) {
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		dirs = append(dirs, clean)
	}

	_, file, _, ok := runtime.Caller(0)
	if ok {
		// Vite builds into this package so `go:embed frontend/dist` can ship the
		// same assets. Prefer the source-adjacent dist directory during local
		// development so an already-built dashboard serves the latest frontend
		// files without relying on a stale embedded snapshot.
		add(filepath.Join(filepath.Dir(file), "frontend", "dist"))
	}
	add(filepath.Join("internal", "interfaces", "web", "frontend", "dist"))
	add(filepath.Join("web", "frontend", "dist"))
	return dirs
}

func spaHandler(fsys http.FileSystem) http.Handler {
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		f, err := fsys.Open(path)
		if err != nil {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()
		fileServer.ServeHTTP(w, r)
	})
}
