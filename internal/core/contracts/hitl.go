package contracts

import "time"

// HITLStatus describes human approval lifecycle.
type HITLStatus string

const (
	HITLPending   HITLStatus = "pending"
	HITLApproved  HITLStatus = "approved"
	HITLRejected  HITLStatus = "rejected"
	HITLModified  HITLStatus = "modified"
	HITLExpired   HITLStatus = "expired"
	HITLCancelled HITLStatus = "cancelled"
)

// HITLRequest records a human-in-the-loop request.
type HITLRequest struct {
	ID               string         `json:"id"`
	JobID            string         `json:"job_id"`
	StepID           string         `json:"step_id,omitempty"`
	PolicyDecisionID string         `json:"policy_decision_id,omitempty"`
	Status           HITLStatus     `json:"status"`
	Reason           string         `json:"reason"`
	RequestedAction  string         `json:"requested_action"`
	Options          []string       `json:"options,omitempty"`
	RiskSummary      map[string]any `json:"risk_summary,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	ResolvedAt       *time.Time     `json:"resolved_at,omitempty"`
	Resolution       map[string]any `json:"resolution,omitempty"`
}
