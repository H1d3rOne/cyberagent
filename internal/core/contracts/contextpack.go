package contracts

// ContextPack is the redacted/cropped context passed to LLM/analyzer plugins.
type ContextPack struct {
	ID                string            `json:"id"`
	JobID             string            `json:"job_id"`
	Purpose           string            `json:"purpose"`
	ScopeSummary      string            `json:"scope_summary"`
	TargetSummary     map[string]any    `json:"target_summary,omitempty"`
	PlanSummary       map[string]any    `json:"plan_summary,omitempty"`
	EvidenceSummaries []EvidenceSummary `json:"evidence_summaries,omitempty"`
	FindingCandidates []string          `json:"finding_candidates,omitempty"`
	Policy            map[string]any    `json:"policy,omitempty"`
	Redaction         map[string]any    `json:"redaction,omitempty"`
	Metadata          Metadata          `json:"metadata,omitempty"`
}

// EvidenceSummary is a compact LLM-safe evidence reference.
type EvidenceSummary struct {
	EvidenceID string `json:"evidence_id"`
	Summary    string `json:"summary"`
}
