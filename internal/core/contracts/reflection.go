package contracts

import "time"

// ReflectionAction is a quality-gate output.
type ReflectionAction string

const (
	ReflectionContinue          ReflectionAction = "continue"
	ReflectionRetry             ReflectionAction = "retry"
	ReflectionSkip              ReflectionAction = "skip"
	ReflectionReplan            ReflectionAction = "replan"
	ReflectionRequestApproval   ReflectionAction = "request_approval"
	ReflectionRequestCorrection ReflectionAction = "request_correction"
	ReflectionMarkUncertain     ReflectionAction = "mark_uncertain"
)

// ReflectionResult records a quality-gate decision.
type ReflectionResult struct {
	ID        string              `json:"id"`
	JobID     string              `json:"job_id"`
	Trigger   string              `json:"trigger"`
	Mode      string              `json:"mode"`
	InputRefs map[string][]string `json:"input_refs,omitempty"`
	Decision  ReflectionAction    `json:"decision"`
	Reasons   []string            `json:"reasons,omitempty"`
	CreatedAt time.Time           `json:"created_at"`
}
