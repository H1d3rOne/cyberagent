package contracts

import "time"

// Plan is a versioned list of executable steps.
type Plan struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	Version   int       `json:"version"`
	Planner   string    `json:"planner"`
	Status    Status    `json:"status"`
	Steps     []string  `json:"steps"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Rationale string    `json:"rationale,omitempty"`
	Metadata  Metadata  `json:"metadata,omitempty"`
}

// NewPlan creates a ready plan.
func NewPlan(id, jobID, planner string, stepIDs []string) Plan {
	now := Now()
	return Plan{
		ID:        id,
		JobID:     jobID,
		Version:   1,
		Planner:   planner,
		Status:    StatusReady,
		Steps:     stepIDs,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  Metadata{},
	}
}
