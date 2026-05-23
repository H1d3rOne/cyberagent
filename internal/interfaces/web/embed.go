package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed frontend/dist
var embeddedFrontend embed.FS

func FrontendHandler() http.Handler {
	distPath := filepath.Join("web", "frontend", "dist")
	if info, err := os.Stat(distPath); err == nil && info.IsDir() {
		return spaHandler(http.Dir(distPath))
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
