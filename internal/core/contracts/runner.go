package contracts

// RunnerType identifies execution environment.
type RunnerType string

const (
	RunnerTypeLocal  RunnerType = "local"
	RunnerTypeDocker RunnerType = "docker"
	RunnerTypeRemote RunnerType = "remote"
	RunnerTypeK8s    RunnerType = "k8s"
)

// Runner describes an execution environment.
type Runner struct {
	ID           string       `json:"id"`
	Type         RunnerType   `json:"type"`
	Status       string       `json:"status"`
	Capabilities []string     `json:"capabilities,omitempty"`
	Limits       RunnerLimits `json:"limits"`
	Metadata     Metadata     `json:"metadata,omitempty"`
}

// RunnerLimits describes runner resource constraints.
type RunnerLimits struct {
	MaxParallelSteps  int `json:"max_parallel_steps,omitempty"`
	MaxMemoryMB       int `json:"max_memory_mb,omitempty"`
	MaxRuntimeSeconds int `json:"max_runtime_seconds,omitempty"`
}
