package daemon

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

// Envelope is the stable HTTP response wrapper for /api/v1.
type Envelope struct {
	OK    bool       `json:"ok"`
	Data  any        `json:"data"`
	Error *ErrorBody `json:"error"`
}

// ErrorBody is the machine-readable error shape inside Envelope.
type ErrorBody struct {
	Code    services.ErrorCode `json:"code"`
	Message string             `json:"message"`
	Details map[string]any     `json:"details"`
}

func writeOK(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{OK: true, Data: data, Error: nil})
}

func writeServiceError(w http.ResponseWriter, err error) {
	status, body := serviceErrorBody(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{OK: false, Data: nil, Error: body})
}

func serviceErrorBody(err error) (int, *ErrorBody) {
	var serviceErr *services.ServiceError
	if errors.As(err, &serviceErr) {
		return statusForCode(serviceErr.Code), &ErrorBody{
			Code:    serviceErr.Code,
			Message: serviceErr.Message,
			Details: normalizeDetails(serviceErr.Details),
		}
	}
	return http.StatusInternalServerError, &ErrorBody{
		Code:    services.CodeInternal,
		Message: "internal error",
		Details: map[string]any{},
	}
}

func statusForCode(code services.ErrorCode) int {
	switch code {
	case services.CodeBadRequest:
		return http.StatusBadRequest
	case services.CodeUnauthorized:
		return http.StatusUnauthorized
	case services.CodeNotFound:
		return http.StatusNotFound
	case services.CodeConflict:
		return http.StatusConflict
	case services.CodeScopeRequired, services.CodePolicyDenied, services.CodeApprovalRequired:
		return http.StatusUnprocessableEntity
	case services.CodeToolDependencyMissing:
		return http.StatusFailedDependency
	case services.CodeJobCancelled:
		return 499
	case services.CodeUnsupported:
		return http.StatusNotImplemented
	case services.CodeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func normalizeDetails(details map[string]any) map[string]any {
	if details == nil {
		return map[string]any{}
	}
	return details
}
