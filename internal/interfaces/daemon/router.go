package daemon

import (
	"net/http"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

type routerServices struct {
	version services.VersionService
}

func newRouter(svc routerServices, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/healthz", requireMethod(http.MethodGet, handleHealthz))
	mux.HandleFunc("/api/v1/version", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, http.StatusOK, svc.version.GetVersion(r.Context()))
	}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
	})
	return bearerAuth(token, mux)
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "cyberagent-daemon",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func requireMethod(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
			return
		}
		next(w, r)
	}
}
