package contracts

import "time"

// PolicyAction is the policy engine decision.
type PolicyAction string

const (
	PolicyAllow           PolicyAction = "allow"
	PolicyAllowWithLimits PolicyAction = "allow_with_limits"
	PolicyRequireApproval PolicyAction = "require_approval"
	PolicyDeny            PolicyAction = "deny"
)

// PolicyDecision records one policy decision.
type PolicyDecision struct {
	ID            string         `json:"id"`
	JobID         string         `json:"job_id"`
	StepID        string         `json:"step_id,omitempty"`
	Decision      PolicyAction   `json:"decision"`
	Reasons       []string       `json:"reasons,omitempty"`
	AppliedLimits map[string]any `json:"applied_limits,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}
