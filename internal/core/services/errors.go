package services

import (
	"errors"
	"fmt"
)

// ErrorCode is a stable machine-readable service error code.
type ErrorCode string

const (
	CodeBadRequest            ErrorCode = "bad_request"
	CodeUnauthorized          ErrorCode = "unauthorized"
	CodeNotFound              ErrorCode = "not_found"
	CodeConflict              ErrorCode = "conflict"
	CodeScopeRequired         ErrorCode = "scope_required"
	CodePolicyDenied          ErrorCode = "policy_denied"
	CodeApprovalRequired      ErrorCode = "approval_required"
	CodeToolDependencyMissing ErrorCode = "tool_dependency_missing"
	CodeJobCancelled          ErrorCode = "job_cancelled"
	CodeInternal              ErrorCode = "internal_error"
	CodeUnsupported           ErrorCode = "unsupported"
	CodeRateLimited           ErrorCode = "rate_limited"
)

// ServiceError is the boundary error shape shared by CLI/API adapters.
type ServiceError struct {
	Code    ErrorCode      `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	Err     error          `json:"-"`
}

func (e *ServiceError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Message
	}
	if e.Message == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *ServiceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewError creates a service error with a stable code.
func NewError(code ErrorCode, message string, details map[string]any, err error) *ServiceError {
	if details == nil {
		details = map[string]any{}
	}
	return &ServiceError{Code: code, Message: message, Details: details, Err: err}
}

// ErrorCodeOf extracts a stable service error code from err.
func ErrorCodeOf(err error) ErrorCode {
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) && serviceErr.Code != "" {
		return serviceErr.Code
	}
	if err == nil {
		return ""
	}
	return CodeInternal
}

func badRequest(message string, err error) error {
	return NewError(CodeBadRequest, message, nil, err)
}

func notFound(message string, err error) error {
	return NewError(CodeNotFound, message, nil, err)
}

func internalError(message string, err error) error {
	return NewError(CodeInternal, message, nil, err)
}
