package daemon

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

type routerServices struct {
	version   services.VersionService
	plugins   services.PluginService
	runs      services.RunService
	approvals services.ApprovalService
}

func newRouter(svc routerServices, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/healthz", requireMethod(http.MethodGet, handleHealthz))
	mux.HandleFunc("/api/v1/version", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, http.StatusOK, svc.version.GetVersion(r.Context()))
	}))
	mux.HandleFunc("/api/v1/doctor", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeOK(w, http.StatusOK, svc.version.Doctor(r.Context()))
	}))
	mux.HandleFunc("/api/v1/plugins", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		plugins, err := svc.plugins.ListPlugins(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{"plugins": plugins})
	}))
	mux.HandleFunc("/api/v1/plugins/doctor", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		report, err := svc.plugins.Doctor(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, report)
	}))
	mux.HandleFunc("/api/v1/runs", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		limit, err := optionalInt(r.URL.Query().Get("limit"))
		if err != nil {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "invalid limit", map[string]any{"field": "limit"}, err))
			return
		}
		page, err := svc.runs.ListRuns(r.Context(), services.RunListRequest{
			Cursor: r.URL.Query().Get("cursor"),
			Limit:  limit,
			Status: contracts.Status(strings.TrimSpace(r.URL.Query().Get("status"))),
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, page)
	}))
	mux.HandleFunc("/api/v1/runs/", requireMethod(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		handleRunRead(w, r, svc)
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

func handleRunRead(w http.ResponseWriter, r *http.Request, svc routerServices) {
	runID, resource, ok := parseRunReadPath(r.URL.Path)
	if !ok {
		writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
		return
	}
	switch resource {
	case "":
		detail, err := svc.runs.GetRun(r.Context(), runID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, detail)
	case "events":
		limit, err := optionalInt(r.URL.Query().Get("limit"))
		if err != nil {
			writeServiceError(w, services.NewError(services.CodeBadRequest, "invalid limit", map[string]any{"field": "limit"}, err))
			return
		}
		page, err := svc.runs.GetEvents(r.Context(), runID, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, page)
	case "report":
		report, err := svc.runs.GetReport(r.Context(), runID, r.URL.Query().Get("format"))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, reportPayload(report))
	case "artifacts":
		index, err := svc.runs.GetArtifactIndex(r.Context(), runID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, index)
	case "approvals":
		approvals, err := svc.approvals.ListApprovals(r.Context(), runID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, map[string]any{"approvals": approvals})
	default:
		writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
	}
}

func parseRunReadPath(path string) (runID string, resource string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/runs/")
	if rest == "" || rest == path {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", "", false
	}
	if len(parts) == 1 {
		return parts[0], "", true
	}
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func optionalInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value, nil
}

type reportHTTPPayload struct {
	Format      string `json:"format"`
	ContentType string `json:"content_type"`
	Report      any    `json:"report,omitempty"`
	Content     string `json:"content,omitempty"`
}

func reportPayload(report services.RunReport) reportHTTPPayload {
	payload := reportHTTPPayload{Format: report.Format, ContentType: report.ContentType}
	if report.Format == "json" {
		var decoded any
		if err := json.Unmarshal(report.Bytes, &decoded); err == nil {
			payload.Report = decoded
			return payload
		}
	}
	payload.Content = string(report.Bytes)
	return payload
}
