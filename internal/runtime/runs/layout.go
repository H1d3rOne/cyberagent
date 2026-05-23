package runs

import "path/filepath"

// Layout contains all well-known paths in one job workspace.
type Layout struct {
	Root         string
	JobID        string
	Path         string
	StateDir     string
	BoltPath     string
	EventsPath   string
	CommandsPath string

	TargetPath   string
	ScopePath    string
	JobPath      string
	PlanPath     string
	FindingsPath string

	ArtifactsDir   string
	StdoutDir      string
	StderrDir      string
	HTTPDir        string
	ToolResultsDir string
	ScreenshotsDir string

	EvidenceDir   string
	EvidenceJSONL string
	EvidenceIndex string

	ReportsDir string
	ReportMD   string
	ReportJSON string
}

// NewLayout returns the canonical workspace layout for a job.
func NewLayout(root, jobID string) Layout {
	base := filepath.Join(root, jobID)
	artifacts := filepath.Join(base, "artifacts")
	evidence := filepath.Join(base, "evidence")
	reports := filepath.Join(base, "reports")
	state := filepath.Join(base, "state")
	return Layout{
		Root:         root,
		JobID:        jobID,
		Path:         base,
		StateDir:     state,
		BoltPath:     filepath.Join(state, "bolt.db"),
		EventsPath:   filepath.Join(base, "events.jsonl"),
		CommandsPath: filepath.Join(base, "commands.log"),

		TargetPath:   filepath.Join(base, "target.json"),
		ScopePath:    filepath.Join(base, "scope.json"),
		JobPath:      filepath.Join(base, "job.json"),
		PlanPath:     filepath.Join(base, "plan.json"),
		FindingsPath: filepath.Join(base, "findings.json"),

		ArtifactsDir:   artifacts,
		StdoutDir:      filepath.Join(artifacts, "stdout"),
		StderrDir:      filepath.Join(artifacts, "stderr"),
		HTTPDir:        filepath.Join(artifacts, "http"),
		ToolResultsDir: filepath.Join(artifacts, "tool-results"),
		ScreenshotsDir: filepath.Join(artifacts, "screenshots"),

		EvidenceDir:   evidence,
		EvidenceJSONL: filepath.Join(evidence, "evidence.jsonl"),
		EvidenceIndex: filepath.Join(evidence, "index.json"),

		ReportsDir: reports,
		ReportMD:   filepath.Join(reports, "report.md"),
		ReportJSON: filepath.Join(reports, "report.json"),
	}
}

// Dirs returns all directories that must exist for the workspace.
func (l Layout) Dirs() []string {
	return []string{
		l.Path,
		l.StateDir,
		l.ArtifactsDir,
		l.StdoutDir,
		l.StderrDir,
		l.HTTPDir,
		l.ToolResultsDir,
		l.ScreenshotsDir,
		l.EvidenceDir,
		l.ReportsDir,
	}
}
