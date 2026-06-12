package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	runIndexSchemaVersion = "cyberagent.run_index.v1"
	runIndexSourceRunsDir = "runs_dir"
	runIndexDirName       = ".index"
	runIndexFileName      = "runs.json"
)

// RunIndex is a rebuildable cache of run summaries. The filesystem runs dir
// remains the source of truth; deleting this file should only require a rebuild.
type RunIndex struct {
	SchemaVersion string       `json:"schema_version"`
	GeneratedAt   time.Time    `json:"generated_at"`
	Source        string       `json:"source"`
	Count         int          `json:"count"`
	RunsDir       string       `json:"runs_dir"`
	Items         []RunSummary `json:"items"`
}

// RunIndexValidation compares a persisted index against a fresh runs-dir scan.
type RunIndexValidation struct {
	SchemaVersion     string              `json:"schema_version"`
	CheckedAt         time.Time           `json:"checked_at"`
	Source            string              `json:"source"`
	RunsDir           string              `json:"runs_dir"`
	Valid             bool                `json:"valid"`
	IndexedCount      int                 `json:"indexed_count"`
	ScannedCount      int                 `json:"scanned_count"`
	MissingFromIndex  []string            `json:"missing_from_index,omitempty"`
	ExtraInIndex      []string            `json:"extra_in_index,omitempty"`
	Stale             []RunIndexStaleItem `json:"stale,omitempty"`
	IndexSchema       string              `json:"index_schema,omitempty"`
	IndexGeneratedAt  time.Time           `json:"index_generated_at,omitempty"`
	IndexCount        int                 `json:"index_count"`
	IndexDeclaredPath string              `json:"index_declared_path,omitempty"`
}

// RunIndexStaleItem records a run whose indexed summary no longer matches the
// current filesystem-backed summary.
type RunIndexStaleItem struct {
	RunID   string     `json:"run_id"`
	Indexed RunSummary `json:"indexed"`
	Current RunSummary `json:"current"`
}

// RebuildIndex scans the runs directory and writes a rebuildable cache at
// <RunsDir>/.index/runs.json.
func (s RunService) RebuildIndex(ctx context.Context) (RunIndex, error) {
	if _, err := tenantContextForServiceEntry(ctx); err != nil {
		return RunIndex{}, err
	}
	summaries, err := s.scanRunSummaries(ctx, "")
	if err != nil {
		return RunIndex{}, err
	}
	if summaries == nil {
		summaries = []RunSummary{}
	}
	index := RunIndex{
		SchemaVersion: runIndexSchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Source:        runIndexSourceRunsDir,
		Count:         len(summaries),
		RunsDir:       s.RunsDir,
		Items:         summaries,
	}
	if err := s.writeRunIndex(index); err != nil {
		return RunIndex{}, err
	}
	return index, nil
}

// LoadIndex loads the current run index cache.
func (s RunService) LoadIndex(ctx context.Context) (RunIndex, error) {
	if _, err := tenantContextForServiceEntry(ctx); err != nil {
		return RunIndex{}, err
	}
	var index RunIndex
	if err := readJSONFile(s.runIndexPath(), &index); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RunIndex{}, notFound("run index not found", err)
		}
		return RunIndex{}, internalError("read run index", err)
	}
	if index.Items == nil {
		index.Items = []RunSummary{}
	}
	return index, nil
}

// ValidateIndex verifies that the cache can be reproduced from the current runs
// directory. It does not repair the index; call RebuildIndex to refresh it.
func (s RunService) ValidateIndex(ctx context.Context) (RunIndexValidation, error) {
	if _, err := tenantContextForServiceEntry(ctx); err != nil {
		return RunIndexValidation{}, err
	}
	index, err := s.LoadIndex(ctx)
	if err != nil {
		return RunIndexValidation{}, err
	}
	current, err := s.scanRunSummaries(ctx, "")
	if err != nil {
		return RunIndexValidation{}, err
	}
	if current == nil {
		current = []RunSummary{}
	}

	currentByID := make(map[string]RunSummary, len(current))
	for _, item := range current {
		currentByID[item.RunID] = item
	}
	indexedByID := make(map[string]RunSummary, len(index.Items))
	for _, item := range index.Items {
		indexedByID[item.RunID] = item
	}

	validation := RunIndexValidation{
		SchemaVersion:     runIndexSchemaVersion,
		CheckedAt:         time.Now().UTC(),
		Source:            runIndexSourceRunsDir,
		RunsDir:           s.RunsDir,
		IndexedCount:      len(index.Items),
		ScannedCount:      len(current),
		IndexSchema:       index.SchemaVersion,
		IndexGeneratedAt:  index.GeneratedAt,
		IndexCount:        index.Count,
		IndexDeclaredPath: index.RunsDir,
		MissingFromIndex:  []string{},
		ExtraInIndex:      []string{},
		Stale:             []RunIndexStaleItem{},
	}

	for _, item := range current {
		indexed, ok := indexedByID[item.RunID]
		if !ok {
			validation.MissingFromIndex = append(validation.MissingFromIndex, item.RunID)
			continue
		}
		if !runSummaryIndexEqual(indexed, item) {
			validation.Stale = append(validation.Stale, RunIndexStaleItem{RunID: item.RunID, Indexed: indexed, Current: item})
		}
	}
	for _, item := range index.Items {
		if _, ok := currentByID[item.RunID]; !ok {
			validation.ExtraInIndex = append(validation.ExtraInIndex, item.RunID)
		}
	}
	sort.Strings(validation.MissingFromIndex)
	sort.Strings(validation.ExtraInIndex)
	sort.Slice(validation.Stale, func(i, j int) bool { return validation.Stale[i].RunID < validation.Stale[j].RunID })

	validation.Valid = index.SchemaVersion == runIndexSchemaVersion &&
		index.Source == runIndexSourceRunsDir &&
		index.RunsDir == s.RunsDir &&
		index.Count == len(index.Items) &&
		len(validation.MissingFromIndex) == 0 &&
		len(validation.ExtraInIndex) == 0 &&
		len(validation.Stale) == 0
	return validation, nil
}

func (s RunService) runIndexPath() string {
	return filepath.Join(s.RunsDir, runIndexDirName, runIndexFileName)
}

func (s RunService) writeRunIndex(index RunIndex) error {
	path := s.runIndexPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return internalError("create run index directory", err)
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return internalError("encode run index", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), runIndexFileName+".*.tmp")
	if err != nil {
		return internalError("create temporary run index", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return internalError("write temporary run index", err)
	}
	if err := tmp.Close(); err != nil {
		return internalError("close temporary run index", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return internalError("replace run index", err)
	}
	return nil
}

func runSummaryIndexEqual(a, b RunSummary) bool {
	return a.RunID == b.RunID &&
		a.RunDir == b.RunDir &&
		a.Target == b.Target &&
		a.Scope == b.Scope &&
		a.Profile == b.Profile &&
		a.Status == b.Status &&
		a.CreatedAt.Equal(b.CreatedAt) &&
		a.UpdatedAt.Equal(b.UpdatedAt) &&
		a.Report == b.Report &&
		a.Counts == b.Counts
}
