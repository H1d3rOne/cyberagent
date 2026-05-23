package services

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/runtime"
	approvalruntime "github.com/H1d3rOne/cyberagent/internal/runtime/approvals"
	artifactruntime "github.com/H1d3rOne/cyberagent/internal/runtime/artifacts"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

const (
	defaultRunsDir    = "runs"
	defaultListLimit  = 50
	defaultEventLimit = 100
	maxPageLimit      = 1000
)

// RunService exposes read-only run workspace operations for CLI/API adapters.
type RunService struct {
	RunsDir string
}

// NewRunService creates a run service rooted at runsDir.
func NewRunService(runsDir string) RunService {
	if strings.TrimSpace(runsDir) == "" {
		runsDir = defaultRunsDir
	}
	return RunService{RunsDir: runsDir}
}

// RunListRequest controls run listing.
type RunListRequest struct {
	Cursor string           `json:"cursor,omitempty"`
	Limit  int              `json:"limit,omitempty"`
	Status contracts.Status `json:"status,omitempty"`
}

// RunListResponse is a cursor page of run summaries.
type RunListResponse struct {
	Items      []RunSummary `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
	EOF        bool         `json:"eof"`
	Limit      int          `json:"limit"`
}

// ReportAvailability describes which report formats exist for a run.
type ReportAvailability struct {
	JSON     bool `json:"json"`
	Markdown bool `json:"markdown"`
}

// RunCounts summarizes known run artifacts.
type RunCounts struct {
	Events    int `json:"events"`
	Artifacts int `json:"artifacts"`
	Findings  int `json:"findings"`
	Approvals int `json:"approvals"`
}

// RunSummary is a lightweight view over one run workspace.
type RunSummary struct {
	RunID     string             `json:"run_id"`
	RunDir    string             `json:"run_dir"`
	Target    string             `json:"target,omitempty"`
	Scope     string             `json:"scope,omitempty"`
	Profile   string             `json:"profile,omitempty"`
	Status    contracts.Status   `json:"status,omitempty"`
	CreatedAt time.Time          `json:"created_at,omitempty"`
	UpdatedAt time.Time          `json:"updated_at,omitempty"`
	Report    ReportAvailability `json:"report"`
	Counts    RunCounts          `json:"counts"`
}

// RunDetail includes persisted snapshots for one run.
type RunDetail struct {
	RunID           string                           `json:"run_id"`
	RunDir          string                           `json:"run_dir"`
	Metadata        *runtime.RunMetadataSnapshot     `json:"metadata,omitempty"`
	EffectiveConfig *runtime.EffectiveConfigSnapshot `json:"effective_config,omitempty"`
	Job             *contracts.Job                   `json:"job,omitempty"`
	Summary         RunSummary                       `json:"summary"`
}

// RunReport is raw report content read from a run workspace.
type RunReport struct {
	Format      string `json:"format"`
	ContentType string `json:"content_type"`
	Bytes       []byte `json:"-"`
}

// EventItem wraps one event with cursor metadata.
type EventItem struct {
	Sequence int             `json:"sequence"`
	Cursor   string          `json:"cursor"`
	Event    contracts.Event `json:"event"`
}

// EventPage is a cursor page of events.
type EventPage struct {
	Items      []EventItem `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
	EOF        bool        `json:"eof"`
	Limit      int         `json:"limit"`
}

// ListRuns lists known run directories.
func (s RunService) ListRuns(ctx context.Context, req RunListRequest) (RunListResponse, error) {
	if err := ctxErr(ctx); err != nil {
		return RunListResponse{}, err
	}
	limit := normalizeLimit(req.Limit, defaultListLimit)
	start, err := parseCursor(req.Cursor)
	if err != nil {
		return RunListResponse{}, err
	}
	entries, err := os.ReadDir(s.RunsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RunListResponse{Items: []RunSummary{}, EOF: true, Limit: limit}, nil
		}
		return RunListResponse{}, internalError("list runs", err)
	}

	summaries := make([]RunSummary, 0, len(entries))
	for _, entry := range entries {
		if err := ctxErr(ctx); err != nil {
			return RunListResponse{}, err
		}
		if !entry.IsDir() {
			continue
		}
		summary, err := s.GetRunSummary(ctx, entry.Name())
		if err != nil {
			if ErrorCodeOf(err) == CodeNotFound || ErrorCodeOf(err) == CodeBadRequest {
				continue
			}
			return RunListResponse{}, err
		}
		if req.Status != "" && summary.Status != req.Status {
			continue
		}
		summaries = append(summaries, summary)
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if !summaries[i].CreatedAt.Equal(summaries[j].CreatedAt) {
			return summaries[i].CreatedAt.After(summaries[j].CreatedAt)
		}
		return summaries[i].RunID < summaries[j].RunID
	})

	if start > len(summaries) {
		start = len(summaries)
	}
	end := start + limit
	if end > len(summaries) {
		end = len(summaries)
	}
	page := summaries[start:end]
	eof := end >= len(summaries)
	next := ""
	if !eof {
		next = strconv.Itoa(end)
	}
	return RunListResponse{Items: page, NextCursor: next, EOF: eof, Limit: limit}, nil
}

// GetRun returns persisted snapshots and summary for one run.
func (s RunService) GetRun(ctx context.Context, runIDOrPath string) (RunDetail, error) {
	if err := ctxErr(ctx); err != nil {
		return RunDetail{}, err
	}
	layout, err := s.openLayout(runIDOrPath)
	if err != nil {
		return RunDetail{}, err
	}
	summary, err := s.summaryForLayout(layout)
	if err != nil {
		return RunDetail{}, err
	}
	detail := RunDetail{RunID: layout.JobID, RunDir: layout.Path, Summary: summary}

	var metadata runtime.RunMetadataSnapshot
	if err := readJSONIfExists(filepath.Join(layout.Path, "metadata.json"), &metadata); err != nil {
		return RunDetail{}, err
	} else if metadata.JobID != "" {
		detail.Metadata = &metadata
	}
	var effective runtime.EffectiveConfigSnapshot
	if err := readJSONIfExists(filepath.Join(layout.Path, "effective_config.json"), &effective); err != nil {
		return RunDetail{}, err
	} else if effective.JobID != "" {
		detail.EffectiveConfig = &effective
	}
	var job contracts.Job
	if err := readJSONIfExists(layout.JobPath, &job); err != nil {
		return RunDetail{}, err
	} else if job.ID != "" {
		detail.Job = &job
	}
	return detail, nil
}

// GetRunSummary returns a lightweight run summary.
func (s RunService) GetRunSummary(ctx context.Context, runIDOrPath string) (RunSummary, error) {
	if err := ctxErr(ctx); err != nil {
		return RunSummary{}, err
	}
	layout, err := s.openLayout(runIDOrPath)
	if err != nil {
		return RunSummary{}, err
	}
	return s.summaryForLayout(layout)
}

// GetReport reads an existing report file. It does not regenerate reports.
func (s RunService) GetReport(ctx context.Context, runIDOrPath, format string) (RunReport, error) {
	if err := ctxErr(ctx); err != nil {
		return RunReport{}, err
	}
	layout, err := s.openLayout(runIDOrPath)
	if err != nil {
		return RunReport{}, err
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "json"
	}
	path := layout.ReportJSON
	contentType := "application/json"
	switch format {
	case "json":
	case "md", "markdown":
		format = "md"
		path = layout.ReportMD
		contentType = "text/markdown; charset=utf-8"
	default:
		return RunReport{}, badRequest("invalid report format", fmt.Errorf("expected json or md, got %q", format))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RunReport{}, notFound("report not found", err)
		}
		return RunReport{}, internalError("read report", err)
	}
	return RunReport{Format: format, ContentType: contentType, Bytes: data}, nil
}

// GetArtifactIndex reads artifacts/index.json.
func (s RunService) GetArtifactIndex(ctx context.Context, runIDOrPath string) (artifactruntime.Index, error) {
	if err := ctxErr(ctx); err != nil {
		return artifactruntime.Index{}, err
	}
	layout, err := s.openLayout(runIDOrPath)
	if err != nil {
		return artifactruntime.Index{}, err
	}
	var index artifactruntime.Index
	if err := readJSONFile(layout.ArtifactsIndex, &index); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return artifactruntime.Index{}, notFound("artifact index not found", err)
		}
		return artifactruntime.Index{}, internalError("read artifact index", err)
	}
	return index, nil
}

// GetEvents reads events with a numeric cursor. Cursor is the number of events already consumed.
func (s RunService) GetEvents(ctx context.Context, runIDOrPath, cursor string, limit int) (EventPage, error) {
	if err := ctxErr(ctx); err != nil {
		return EventPage{}, err
	}
	layout, err := s.openLayout(runIDOrPath)
	if err != nil {
		return EventPage{}, err
	}
	start, err := parseCursor(cursor)
	if err != nil {
		return EventPage{}, err
	}
	limit = normalizeLimit(limit, defaultEventLimit)
	return readEventPage(ctx, layout.EventsPath, start, limit)
}

func (s RunService) summaryForLayout(layout runs.Layout) (RunSummary, error) {
	var metadata runtime.RunMetadataSnapshot
	if err := readJSONIfExists(filepath.Join(layout.Path, "metadata.json"), &metadata); err != nil {
		return RunSummary{}, err
	}
	var effective runtime.EffectiveConfigSnapshot
	if err := readJSONIfExists(filepath.Join(layout.Path, "effective_config.json"), &effective); err != nil {
		return RunSummary{}, err
	}
	var job contracts.Job
	if err := readJSONIfExists(layout.JobPath, &job); err != nil {
		return RunSummary{}, err
	}

	target := metadata.Request.Target
	if target == "" {
		target = effective.Target
	}
	scope := metadata.Request.Scope
	if scope == "" {
		scope = effective.Scope
	}
	profile := metadata.Profile
	if profile == "" {
		profile = effective.Profile
	}
	status := job.Status
	created := metadata.CreatedAt
	if created.IsZero() {
		created = job.CreatedAt
	}
	updated := latestModTime(layout.Path)
	if job.FinishedAt != nil && job.FinishedAt.After(updated) {
		updated = *job.FinishedAt
	}

	counts, err := s.countsForLayout(layout)
	if err != nil {
		return RunSummary{}, err
	}
	return RunSummary{
		RunID:     layout.JobID,
		RunDir:    layout.Path,
		Target:    target,
		Scope:     scope,
		Profile:   profile,
		Status:    status,
		CreatedAt: created,
		UpdatedAt: updated,
		Report: ReportAvailability{
			JSON:     fileExists(layout.ReportJSON),
			Markdown: fileExists(layout.ReportMD),
		},
		Counts: counts,
	}, nil
}

func (s RunService) countsForLayout(layout runs.Layout) (RunCounts, error) {
	eventsCount, err := countJSONLLines(layout.EventsPath)
	if err != nil {
		return RunCounts{}, err
	}
	artifactCount := 0
	var artifactIndex artifactruntime.Index
	if err := readJSONIfExists(layout.ArtifactsIndex, &artifactIndex); err != nil {
		return RunCounts{}, err
	} else {
		artifactCount = artifactIndex.Count
		if artifactCount == 0 && len(artifactIndex.Artifacts) > 0 {
			artifactCount = len(artifactIndex.Artifacts)
		}
	}
	findingCount := 0
	var findings []contracts.Finding
	if err := readJSONIfExists(layout.FindingsPath, &findings); err != nil {
		return RunCounts{}, err
	} else {
		findingCount = len(findings)
	}
	approvals, err := approvalruntime.NewStore(layout.Path).List()
	if err != nil {
		return RunCounts{}, internalError("read approvals", err)
	}
	return RunCounts{
		Events:    eventsCount,
		Artifacts: artifactCount,
		Findings:  findingCount,
		Approvals: len(approvals),
	}, nil
}

func (s RunService) openLayout(runIDOrPath string) (runs.Layout, error) {
	runID, err := s.resolveRunID(runIDOrPath)
	if err != nil {
		return runs.Layout{}, err
	}
	layout, err := runs.NewManager(s.RunsDir).Open(runID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return runs.Layout{}, notFound("run not found", err)
		}
		if strings.Contains(err.Error(), "invalid job ID") || strings.Contains(err.Error(), "job ID is empty") {
			return runs.Layout{}, badRequest("invalid run id", err)
		}
		return runs.Layout{}, internalError("open run", err)
	}
	return layout, nil
}

func (s RunService) resolveRunID(runIDOrPath string) (string, error) {
	identifier := strings.TrimSpace(runIDOrPath)
	if identifier == "" {
		return "", badRequest("run id is required", nil)
	}
	if strings.Contains(identifier, "\x00") {
		return "", badRequest("invalid run id", nil)
	}
	if filepath.IsAbs(identifier) || strings.ContainsAny(identifier, `/\`) {
		if strings.Contains(identifier, "..") {
			return "", badRequest("invalid run path", nil)
		}
		root, err := filepath.Abs(s.RunsDir)
		if err != nil {
			return "", internalError("resolve runs dir", err)
		}
		candidate, err := filepath.Abs(filepath.Clean(identifier))
		if err != nil {
			return "", badRequest("invalid run path", err)
		}
		rel, err := filepath.Rel(root, candidate)
		if err != nil {
			return "", badRequest("invalid run path", err)
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || strings.ContainsAny(rel, `/\`) {
			return "", badRequest("invalid run path", nil)
		}
		return rel, nil
	}
	return identifier, nil
}

func readEventPage(ctx context.Context, path string, start, limit int) (EventPage, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return EventPage{Items: []EventItem{}, EOF: true, Limit: limit}, nil
		}
		return EventPage{}, internalError("open events", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	sequence := 0
	items := make([]EventItem, 0, limit)
	for scanner.Scan() {
		if err := ctxErr(ctx); err != nil {
			return EventPage{}, err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		sequence++
		if sequence <= start {
			continue
		}
		if len(items) >= limit {
			return EventPage{Items: items, NextCursor: strconv.Itoa(sequence - 1), EOF: false, Limit: limit}, nil
		}
		var event contracts.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return EventPage{}, internalError("decode event", fmt.Errorf("line %d: %w", sequence, err))
		}
		items = append(items, EventItem{Sequence: sequence, Cursor: strconv.Itoa(sequence), Event: event})
	}
	if err := scanner.Err(); err != nil {
		return EventPage{}, internalError("scan events", err)
	}
	return EventPage{Items: items, EOF: true, Limit: limit}, nil
}

func readJSONIfExists(path string, out any) error {
	err := readJSONFile(path, out)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func readJSONFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func countJSONLLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, internalError("open jsonl", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, internalError("scan jsonl", err)
	}
	return count, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func latestModTime(path string) time.Time {
	var latest time.Time
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil && info.ModTime().After(latest) {
			latest = info.ModTime().UTC()
		}
		return nil
	})
	return latest
}

func normalizeLimit(limit, fallback int) int {
	if limit <= 0 {
		limit = fallback
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	return limit
}

func parseCursor(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(cursor)
	if err != nil || value < 0 {
		return 0, badRequest("invalid cursor", err)
	}
	return value, nil
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return NewError(CodeInternal, "context error", nil, err)
	}
	return nil
}
