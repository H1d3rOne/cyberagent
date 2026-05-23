package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// Recorder persists auditable tool invocations as append-only JSONL.
type Recorder struct {
	path string
	mu   sync.Mutex
}

// NewRecorder creates a recorder backed by commands.log.
func NewRecorder(path string) *Recorder {
	return &Recorder{path: path}
}

// Path returns the JSONL file path backing this recorder.
func (r *Recorder) Path() string {
	return r.path
}

// Append writes one tool call as one JSON line.
func (r *Recorder) Append(call contracts.ToolCall) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("create command log dir: %w", err)
	}
	file, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open command log: %w", err)
	}
	defer file.Close()

	encoded, err := json.Marshal(call)
	if err != nil {
		return fmt.Errorf("marshal tool call: %w", err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write command log: %w", err)
	}
	return nil
}

// ReadAll reads all recorded tool calls.
func (r *Recorder) ReadAll() ([]contracts.ToolCall, error) {
	file, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open command log: %w", err)
	}
	defer file.Close()

	var calls []contracts.ToolCall
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var call contracts.ToolCall
		if err := json.Unmarshal(scanner.Bytes(), &call); err != nil {
			return nil, fmt.Errorf("decode command log line %d: %w", line, err)
		}
		calls = append(calls, call)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan command log: %w", err)
	}
	return calls, nil
}

// NewStepCall creates a pseudo command record for a built-in tool or plugin step.
func NewStepCall(id string, jobID string, step contracts.Step) contracts.ToolCall {
	if strings.HasPrefix(step.Tool.Name, "plugin.") {
		pluginName := strings.TrimPrefix(step.Tool.Name, "plugin.")
		return contracts.ToolCall{
			ID:          id,
			JobID:       jobID,
			StepID:      step.ID,
			ToolName:    step.Tool.Name,
			ToolVersion: step.Tool.Version,
			RunnerID:    "local",
			Kind:        contracts.ToolCallKindPlugin,
			Command:     pluginName,
			Args:        []string{"--step", step.ID},
			EnvPolicy:   "inherit_minimal",
			StartedAt:   contracts.Now(),
			Status:      contracts.StatusRunning,
			Metadata: contracts.Metadata{
				"step_name": step.Name,
				"step_type": step.Type,
				"risk":      step.Risk,
			},
		}
	}
	return NewBuiltinCall(id, jobID, step)
}

// NewBuiltinCall creates a completed-or-running pseudo command record for a built-in tool.
func NewBuiltinCall(id string, jobID string, step contracts.Step) contracts.ToolCall {
	return contracts.ToolCall{
		ID:          id,
		JobID:       jobID,
		StepID:      step.ID,
		ToolName:    step.Tool.Name,
		ToolVersion: step.Tool.Version,
		RunnerID:    "local",
		Kind:        contracts.ToolCallKindBuiltin,
		Command:     step.Tool.Name,
		Args:        []string{"--step", step.ID},
		EnvPolicy:   "no_env",
		StartedAt:   contracts.Now(),
		Status:      contracts.StatusRunning,
		Metadata: contracts.Metadata{
			"step_name": step.Name,
			"step_type": step.Type,
			"risk":      step.Risk,
		},
	}
}

// Finish marks a tool call as completed and records exit state.
func Finish(call contracts.ToolCall, status contracts.Status, err error) contracts.ToolCall {
	finished := contracts.Now()
	call.FinishedAt = &finished
	call.Status = status
	exitCode := 0
	if status == contracts.StatusFailed {
		exitCode = 1
	}
	call.ExitCode = &exitCode
	if err != nil {
		call.Error = err.Error()
	}
	return call
}
