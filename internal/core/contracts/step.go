package contracts

import "time"

// StepType identifies the executor needed for a step.
type StepType string

const (
	StepTypeBuiltinTool  StepType = "builtin_tool"
	StepTypeExternalTool StepType = "external_tool"
	StepTypeParser       StepType = "parser"
	StepTypeAnalyzer     StepType = "analyzer"
	StepTypeReporter     StepType = "reporter"
	StepTypePolicyCheck  StepType = "policy_check"
	StepTypeHITL         StepType = "hitl"
	StepTypeReflection   StepType = "reflection"
)

// Step is the smallest schedulable unit.
type Step struct {
	ID               string         `json:"id"`
	JobID            string         `json:"job_id"`
	PlanID           string         `json:"plan_id"`
	Name             string         `json:"name"`
	Type             StepType       `json:"type"`
	Status           Status         `json:"status"`
	Risk             RiskLevel      `json:"risk"`
	RequiresApproval bool           `json:"requires_approval"`
	DependsOn        []string       `json:"depends_on,omitempty"`
	Tool             ToolRef        `json:"tool"`
	Input            map[string]any `json:"input,omitempty"`
	ExpectedOutputs  []string       `json:"expected_outputs,omitempty"`
	TimeoutSeconds   int            `json:"timeout_seconds,omitempty"`
	Retry            RetryPolicy    `json:"retry,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	StartedAt        *time.Time     `json:"started_at,omitempty"`
	FinishedAt       *time.Time     `json:"finished_at,omitempty"`
	Error            string         `json:"error,omitempty"`
	Metadata         Metadata       `json:"metadata,omitempty"`
}

// ToolRef references a built-in tool or plugin capability.
type ToolRef struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// RetryPolicy controls step retry behavior.
type RetryPolicy struct {
	MaxAttempts    int `json:"max_attempts,omitempty"`
	BackoffSeconds int `json:"backoff_seconds,omitempty"`
}

// NewStep creates a pending step.
func NewStep(id, jobID, planID, name string, stepType StepType, risk RiskLevel, tool ToolRef) Step {
	return Step{
		ID:             id,
		JobID:          jobID,
		PlanID:         planID,
		Name:           name,
		Type:           stepType,
		Status:         StatusPending,
		Risk:           risk,
		Tool:           tool,
		Input:          map[string]any{},
		TimeoutSeconds: 30,
		Retry:          RetryPolicy{MaxAttempts: 1, BackoffSeconds: 2},
		CreatedAt:      Now(),
		Metadata:       Metadata{},
	}
}
