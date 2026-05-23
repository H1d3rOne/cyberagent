package contracts

import "time"

// Report is the final user-facing output collection.
type Report struct {
	ID          string            `json:"id"`
	JobID       string            `json:"job_id"`
	Formats     []string          `json:"formats"`
	Paths       map[string]string `json:"paths"`
	FindingIDs  []string          `json:"finding_ids"`
	GeneratedAt time.Time         `json:"generated_at"`
	Generator   string            `json:"generator"`
	Metadata    Metadata          `json:"metadata,omitempty"`
}
