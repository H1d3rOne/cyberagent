package services

import (
	"context"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// ScanService exposes scan creation operations for future daemon/API adapters.
//
// v0.1.2 Slice 2 intentionally provides request/response validation only.
// Actual async execution is owned by the later daemon job manager slice.
type ScanService struct{}

// NewScanService creates a scan service skeleton.
func NewScanService() ScanService {
	return ScanService{}
}

// StartWebScanRequest describes a safe web scan request.
type StartWebScanRequest struct {
	Target          string   `json:"target"`
	Scope           string   `json:"scope"`
	Profile         string   `json:"profile,omitempty"`
	EnabledPlugins  []string `json:"enabled_plugins,omitempty"`
	NoExternalTools bool     `json:"no_external_tools"`
	Interactive     bool     `json:"interactive,omitempty"`
}

// StartWebScanResult is the future async scan job response.
type StartWebScanResult struct {
	JobID     string           `json:"job_id"`
	Kind      string           `json:"kind"`
	Status    contracts.Status `json:"status"`
	RunID     string           `json:"run_id,omitempty"`
	RunDir    string           `json:"run_dir,omitempty"`
	StatusURL string           `json:"status_url,omitempty"`
	RunURL    string           `json:"run_url,omitempty"`
}

// StartWebScan validates a web scan request. Execution is deferred until job manager implementation.
func (s ScanService) StartWebScan(ctx context.Context, req StartWebScanRequest) (StartWebScanResult, error) {
	if err := ctxErr(ctx); err != nil {
		return StartWebScanResult{}, err
	}
	if strings.TrimSpace(req.Target) == "" {
		return StartWebScanResult{}, badRequest("target is required", nil)
	}
	if strings.TrimSpace(req.Scope) == "" {
		return StartWebScanResult{}, NewError(CodeScopeRequired, "scope is required for active scanning", map[string]any{"field": "scope"}, nil)
	}
	return StartWebScanResult{}, NewError(CodeUnsupported, "async scan job manager is not implemented yet", nil, nil)
}
