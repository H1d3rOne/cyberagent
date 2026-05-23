package contracts

import "time"

// JobKind identifies a top-level job class.
type JobKind string

const (
	JobKindWebScan JobKind = "web_scan"
)

// Job is one complete analysis run.
type Job struct {
	ID         string     `json:"id"`
	Kind       JobKind    `json:"kind"`
	Status     Status     `json:"status"`
	TargetID   string     `json:"target_id"`
	ScopeID    string     `json:"scope_id"`
	Profile    string     `json:"profile"`
	Workspace  string     `json:"workspace"`
	PlanID     string     `json:"plan_id,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Options    JobOptions `json:"options"`
	Summary    JobSummary `json:"summary"`
	Metadata   Metadata   `json:"metadata,omitempty"`
}

// JobOptions controls optional runtime behavior.
type JobOptions struct {
	Interactive        bool     `json:"interactive"`
	UseLLM             bool     `json:"use_llm"`
	AllowExternalTools bool     `json:"allow_external_tools"`
	ReportFormats      []string `json:"report_formats"`
}

// JobSummary stores fast status counters.
type JobSummary struct {
	StepsTotal     int `json:"steps_total"`
	StepsSucceeded int `json:"steps_succeeded"`
	StepsFailed    int `json:"steps_failed"`
	FindingsTotal  int `json:"findings_total"`
}

// NewWebScanJob creates a web scan job with safe defaults.
func NewWebScanJob(id, targetID, scopeID, profile, workspace string) Job {
	if profile == "" {
		profile = "safe"
	}
	return Job{
		ID:        id,
		Kind:      JobKindWebScan,
		Status:    StatusCreated,
		TargetID:  targetID,
		ScopeID:   scopeID,
		Profile:   profile,
		Workspace: workspace,
		CreatedAt: Now(),
		Options: JobOptions{
			Interactive:        true,
			UseLLM:             false,
			AllowExternalTools: true,
			ReportFormats:      []string{"markdown", "json"},
		},
		Metadata: Metadata{},
	}
}
