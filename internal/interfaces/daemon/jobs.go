package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/planner"
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

const scanWebJobKind = "scan_web"

type webScanRunner func(context.Context, coreruntime.WebScanRequest) (coreruntime.WebScanResult, error)

type jobManagerConfig struct {
	RunsDir           string
	Build             services.BuildInfo
	MaxConcurrentJobs int
	Runner            webScanRunner
}

type jobManager struct {
	mu                sync.Mutex
	runsDir           string
	build             services.BuildInfo
	maxConcurrentJobs int
	runner            webScanRunner
	jobs              map[string]*daemonJob
	running           int
}

type startWebScanJobRequest struct {
	Target          string
	Scope           string
	Profile         string
	EnabledPlugins  []string
	NoExternalTools bool
}

type scanJobStartResponse struct {
	JobID     string           `json:"job_id"`
	Kind      string           `json:"kind"`
	Status    contracts.Status `json:"status"`
	RunID     string           `json:"run_id"`
	RunDir    string           `json:"run_dir"`
	StatusURL string           `json:"status_url"`
	RunURL    string           `json:"run_url"`
}

type daemonJob struct {
	JobID      string           `json:"job_id"`
	Kind       string           `json:"kind"`
	Status     contracts.Status `json:"status"`
	RunID      string           `json:"run_id,omitempty"`
	RunDir     string           `json:"run_dir,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
	StartedAt  *time.Time       `json:"started_at,omitempty"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
	Error      *daemonJobError  `json:"error"`

	cancel   context.CancelFunc `json:"-"`
	inFlight bool               `json:"-"`
}

type daemonJobError struct {
	Code    services.ErrorCode `json:"code"`
	Message string             `json:"message"`
}

func newJobManager(cfg jobManagerConfig) *jobManager {
	if strings.TrimSpace(cfg.RunsDir) == "" {
		cfg.RunsDir = DefaultRunsDir
	}
	if cfg.MaxConcurrentJobs <= 0 {
		cfg.MaxConcurrentJobs = DefaultMaxConcurrentJobs
	}
	if cfg.Runner == nil {
		cfg.Runner = coreruntime.RunWebScan
	}
	return &jobManager{
		runsDir:           cfg.RunsDir,
		build:             cfg.Build,
		maxConcurrentJobs: cfg.MaxConcurrentJobs,
		runner:            cfg.Runner,
		jobs:              map[string]*daemonJob{},
	}
}

func (m *jobManager) StartWebScan(ctx context.Context, req startWebScanJobRequest) (scanJobStartResponse, error) {
	if err := contextError(ctx); err != nil {
		return scanJobStartResponse{}, err
	}
	normalized, err := normalizeStartWebScanRequest(req)
	if err != nil {
		return scanJobStartResponse{}, err
	}

	now := time.Now().UTC()
	jobID := runs.NewJobID(now)
	runID := runs.NewJobID(now)
	runDir := filepath.Join(m.runsDir, runID)
	runCtx, cancel := context.WithCancel(context.Background())
	job := &daemonJob{
		JobID:     jobID,
		Kind:      scanWebJobKind,
		Status:    contracts.StatusRunning,
		RunID:     runID,
		RunDir:    runDir,
		CreatedAt: now,
		StartedAt: timePtr(now),
		cancel:    cancel,
		inFlight:  true,
	}

	m.mu.Lock()
	if m.running >= m.maxConcurrentJobs {
		m.mu.Unlock()
		cancel()
		return scanJobStartResponse{}, services.NewError(services.CodeConflict, "maximum concurrent daemon jobs reached", map[string]any{
			"max_concurrent_jobs": m.maxConcurrentJobs,
		}, nil)
	}
	for {
		if _, exists := m.jobs[jobID]; !exists {
			break
		}
		jobID = runs.NewJobID(time.Now().UTC())
		job.JobID = jobID
	}
	m.jobs[jobID] = job
	m.running++
	m.mu.Unlock()

	response := scanJobStartResponse{
		JobID:     jobID,
		Kind:      scanWebJobKind,
		Status:    contracts.StatusRunning,
		RunID:     runID,
		RunDir:    runDir,
		StatusURL: "/api/v1/jobs/" + jobID,
		RunURL:    "/api/v1/runs/" + runID,
	}

	go m.runWebScanJob(runCtx, jobID, runID, normalized)

	return response, nil
}

func (m *jobManager) GetJob(ctx context.Context, jobID string) (daemonJob, error) {
	if err := contextError(ctx); err != nil {
		return daemonJob{}, err
	}
	jobID, err := normalizeDaemonJobID(jobID)
	if err != nil {
		return daemonJob{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return daemonJob{}, services.NewError(services.CodeNotFound, "job not found", map[string]any{"job_id": jobID}, nil)
	}
	return cloneDaemonJob(job), nil
}

func (m *jobManager) CancelJob(ctx context.Context, jobID string) (daemonJob, error) {
	if err := contextError(ctx); err != nil {
		return daemonJob{}, err
	}
	jobID, err := normalizeDaemonJobID(jobID)
	if err != nil {
		return daemonJob{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return daemonJob{}, services.NewError(services.CodeNotFound, "job not found", map[string]any{"job_id": jobID}, nil)
	}
	if job.Status == contracts.StatusCancelled {
		return daemonJob{}, services.NewError(services.CodeConflict, "job is already cancelled", map[string]any{"job_id": jobID, "status": job.Status}, nil)
	}
	if !job.inFlight || job.cancel == nil || job.Status != contracts.StatusRunning {
		return daemonJob{}, services.NewError(services.CodeConflict, "job is not running", map[string]any{"job_id": jobID, "status": job.Status}, nil)
	}
	job.cancel()
	now := time.Now().UTC()
	job.Status = contracts.StatusCancelled
	job.FinishedAt = timePtr(now)
	job.Error = &daemonJobError{Code: services.CodeJobCancelled, Message: "job cancellation requested"}
	return cloneDaemonJob(job), nil
}

func (m *jobManager) runWebScanJob(ctx context.Context, jobID, runID string, req startWebScanJobRequest) {
	result, err := m.runner(ctx, coreruntime.WebScanRequest{
		Target:          req.Target,
		Scope:           req.Scope,
		Profile:         req.Profile,
		Out:             filepath.Join(m.runsDir, runID),
		EnabledPlugins:  cloneStrings(req.EnabledPlugins),
		NoExternalTools: req.NoExternalTools,
		Interactive:     false,
		BuildInfo: coreruntime.BuildInfo{
			Version: m.build.Version,
			Commit:  m.build.Commit,
			Date:    m.build.Date,
			Module:  m.build.Module,
		},
		CommandName: "daemon scan web",
		CommandArgs: daemonScanCommandArgs(req),
	})
	m.finishJob(jobID, result, err)
}

func (m *jobManager) finishJob(jobID string, result coreruntime.WebScanResult, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return
	}
	if job.inFlight {
		m.running--
		job.inFlight = false
	}
	job.cancel = nil
	finished := time.Now().UTC()
	if job.FinishedAt == nil {
		job.FinishedAt = timePtr(finished)
	}
	if job.Status == contracts.StatusCancelled {
		if job.Error == nil {
			job.Error = &daemonJobError{Code: services.CodeJobCancelled, Message: "job cancelled"}
		}
		return
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			job.Status = contracts.StatusCancelled
			job.Error = &daemonJobError{Code: services.CodeJobCancelled, Message: "job cancelled"}
			return
		}
		job.Status = contracts.StatusFailed
		job.Error = &daemonJobError{Code: services.CodeInternal, Message: err.Error()}
		return
	}
	job.Status = contracts.StatusSucceeded
	job.Error = nil
	if result.Workspace != "" {
		job.RunDir = result.Workspace
	}
}

func normalizeStartWebScanRequest(req startWebScanJobRequest) (startWebScanJobRequest, error) {
	req.Target = strings.TrimSpace(req.Target)
	req.Scope = strings.TrimSpace(req.Scope)
	req.Profile = strings.TrimSpace(req.Profile)
	if req.Target == "" {
		return startWebScanJobRequest{}, services.NewError(services.CodeBadRequest, "target is required", map[string]any{"field": "target"}, nil)
	}
	if req.Scope == "" {
		return startWebScanJobRequest{}, services.NewError(services.CodeScopeRequired, "scope is required for active scanning", map[string]any{"field": "scope"}, nil)
	}
	if req.Profile == "" {
		req.Profile = planner.ProfileSafe
	}
	if req.Profile != planner.ProfileSafe {
		return startWebScanJobRequest{}, services.NewError(services.CodeBadRequest, "unsupported scan profile", map[string]any{
			"field":              "profile",
			"profile":            req.Profile,
			"supported_profiles": []string{planner.ProfileSafe},
		}, nil)
	}
	if req.NoExternalTools {
		req.EnabledPlugins = nil
	} else {
		req.EnabledPlugins = cloneStrings(req.EnabledPlugins)
	}
	return req, nil
}

func normalizeDaemonJobID(raw string) (string, error) {
	jobID := strings.TrimSpace(raw)
	if jobID == "" {
		return "", services.NewError(services.CodeBadRequest, "job id is required", map[string]any{"field": "job_id"}, nil)
	}
	if strings.Contains(jobID, "\x00") || strings.ContainsAny(jobID, `/\`) || strings.Contains(jobID, "..") {
		return "", services.NewError(services.CodeBadRequest, "invalid job id", map[string]any{"field": "job_id"}, nil)
	}
	return jobID, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return services.NewError(services.CodeInternal, "context error", nil, ctx.Err())
}

func daemonScanCommandArgs(req startWebScanJobRequest) []string {
	args := []string{"daemon", "scan", "web", "--target", req.Target, "--scope", req.Scope, "--profile", req.Profile}
	if req.NoExternalTools {
		args = append(args, "--no-external-tools")
	}
	if len(req.EnabledPlugins) > 0 {
		for _, plugin := range req.EnabledPlugins {
			args = append(args, "--enable-plugin", plugin)
		}
	}
	return args
}

func cloneDaemonJob(job *daemonJob) daemonJob {
	if job == nil {
		return daemonJob{}
	}
	cloned := *job
	cloned.cancel = nil
	if job.StartedAt != nil {
		cloned.StartedAt = timePtr(*job.StartedAt)
	}
	if job.FinishedAt != nil {
		cloned.FinishedAt = timePtr(*job.FinishedAt)
	}
	if job.Error != nil {
		cloned.Error = &daemonJobError{Code: job.Error.Code, Message: job.Error.Message}
	}
	return cloned
}

func timePtr(t time.Time) *time.Time {
	copy := t
	return &copy
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
