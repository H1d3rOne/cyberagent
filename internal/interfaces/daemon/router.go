package daemon

import (
	"encoding/json"
	"errors"
	"io"
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
	mux.HandleFunc("/api/v1/runs/", func(w http.ResponseWriter, r *http.Request) {
		handleRunRoute(w, r, svc)
	})
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

func handleRunRoute(w http.ResponseWriter, r *http.Request, svc routerServices) {
	switch r.Method {
	case http.MethodGet:
		handleRunRead(w, r, svc)
	case http.MethodPost:
		handleRunWrite(w, r, svc)
	default:
		writeServiceError(w, services.NewError(services.CodeBadRequest, "method not allowed", map[string]any{"method": r.Method}, nil))
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

type approvalDecisionHTTPBody struct {
	Operator string `json:"operator"`
	Comment  string `json:"comment"`
}

type resumeApprovedHTTPBody struct {
	Operator string `json:"operator"`
}

func handleRunWrite(w http.ResponseWriter, r *http.Request, svc routerServices) {
	if runID, approvalID, action, ok := parseApprovalDecisionPath(r.URL.Path); ok {
		var body approvalDecisionHTTPBody
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		req := services.ApprovalDecisionRequest{
			RunIDOrPath:        runID,
			ApprovalIDOrStepID: approvalID,
			Operator:           body.Operator,
			Comment:            body.Comment,
		}
		var (
			resolved any
			err      error
		)
		switch action {
		case "approve":
			resolved, err = svc.approvals.Approve(r.Context(), req)
		case "deny":
			resolved, err = svc.approvals.Deny(r.Context(), req)
		default:
			writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
			return
		}
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, resolved)
		return
	}

	if runID, ok := parseResumeApprovedPath(r.URL.Path); ok {
		var body resumeApprovedHTTPBody
		if err := decodeJSONBody(r, &body); err != nil {
			writeServiceError(w, err)
			return
		}
		result, err := svc.approvals.ResumeApproved(r.Context(), services.ResumeApprovedRequest{
			RunIDOrPath: runID,
			Operator:    body.Operator,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeOK(w, http.StatusOK, result)
		return
	}

	writeServiceError(w, services.NewError(services.CodeNotFound, "endpoint not found", map[string]any{"path": r.URL.Path}, nil))
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

func parseApprovalDecisionPath(path string) (runID string, approvalID string, action string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/runs/")
	if rest == "" || rest == path {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 4 {
		return "", "", "", false
	}
	if strings.TrimSpace(parts[0]) == "" ||
		parts[1] != "approvals" ||
		strings.TrimSpace(parts[2]) == "" ||
		strings.TrimSpace(parts[3]) == "" {
		return "", "", "", false
	}
	return parts[0], parts[2], parts[3], true
}

func parseResumeApprovedPath(path string) (runID string, ok bool) {
	rest := strings.TrimPrefix(path, "/api/v1/runs/")
	if rest == "" || rest == path {
		return "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || parts[1] != "resume-approved" {
		return "", false
	}
	return parts[0], true
}

func decodeJSONBody(r *http.Request, out any) error {
	if r.Body == nil {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return services.NewError(services.CodeBadRequest, "invalid JSON body", nil, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return services.NewError(services.CodeBadRequest, "invalid JSON body", nil, err)
	} else if err == nil {
		return services.NewError(services.CodeBadRequest, "invalid JSON body", map[string]any{"reason": "multiple JSON values"}, nil)
	}
	return nil
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
