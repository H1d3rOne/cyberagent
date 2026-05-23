package contracts

import "time"

// Finding is a security conclusion derived from evidence.
type Finding struct {
	ID              string        `json:"id"`
	JobID           string        `json:"job_id"`
	Title           string        `json:"title"`
	Severity        Severity      `json:"severity"`
	Confidence      Confidence    `json:"confidence"`
	Category        string        `json:"category"`
	AffectedTargets []string      `json:"affected_targets"`
	EvidenceIDs     []string      `json:"evidence_ids"`
	Description     string        `json:"description"`
	Impact          string        `json:"impact"`
	Reproduction    []string      `json:"reproduction"`
	Remediation     string        `json:"remediation"`
	References      []string      `json:"references,omitempty"`
	Source          FindingSource `json:"source"`
	CreatedAt       time.Time     `json:"created_at"`
	Metadata        Metadata      `json:"metadata,omitempty"`
}

// FindingSource identifies where a finding came from.
type FindingSource struct {
	Tool   string `json:"tool"`
	StepID string `json:"step_id"`
}
