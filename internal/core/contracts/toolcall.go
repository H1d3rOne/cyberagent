package contracts

import "time"

// ToolCallKind identifies how a tool was invoked.
type ToolCallKind string

const (
	ToolCallKindBuiltin ToolCallKind = "builtin"
	ToolCallKindCommand ToolCallKind = "command"
	ToolCallKindPlugin  ToolCallKind = "plugin"
)

// ToolCall records one concrete tool execution.
type ToolCall struct {
	ID               string       `json:"id"`
	JobID            string       `json:"job_id"`
	StepID           string       `json:"step_id"`
	ToolName         string       `json:"tool_name"`
	ToolVersion      string       `json:"tool_version,omitempty"`
	RunnerID         string       `json:"runner_id"`
	Kind             ToolCallKind `json:"kind"`
	Command          string       `json:"command,omitempty"`
	Args             []string     `json:"args,omitempty"`
	CWD              string       `json:"cwd,omitempty"`
	EnvPolicy        string       `json:"env_policy,omitempty"`
	StartedAt        time.Time    `json:"started_at"`
	FinishedAt       *time.Time   `json:"finished_at,omitempty"`
	ExitCode         *int         `json:"exit_code,omitempty"`
	StdoutArtifactID string       `json:"stdout_artifact_id,omitempty"`
	StderrArtifactID string       `json:"stderr_artifact_id,omitempty"`
	Artifacts        []string     `json:"artifacts,omitempty"`
	Status           Status       `json:"status"`
	Error            string       `json:"error,omitempty"`
	Metadata         Metadata     `json:"metadata,omitempty"`
}
