package command

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestExecutorRunRecordsSuccessfulCommand(t *testing.T) {
	recorder := NewRecorder(filepath.Join(t.TempDir(), "commands.log"))
	executor := NewExecutor(recorder)
	result, err := executor.Run(context.Background(), ExecRequest{
		ID:       "toolcall_echo_001",
		JobID:    "job_001",
		StepID:   "step_001",
		ToolName: "echo",
		Command:  "go",
		Args:     []string{"version"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Call.Status != contracts.StatusSucceeded {
		t.Fatalf("status = %s, want succeeded", result.Call.Status)
	}
	if len(result.Stdout) == 0 {
		t.Fatalf("expected stdout")
	}
	calls, err := recorder.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(calls) != 1 || calls[0].ID != "toolcall_echo_001" || calls[0].Command != "go" {
		t.Fatalf("unexpected recorded calls: %#v", calls)
	}
}

func TestExecutorRunRequiresCommand(t *testing.T) {
	_, err := NewExecutor(nil).Run(context.Background(), ExecRequest{ID: "toolcall_bad"})
	if err == nil {
		t.Fatalf("expected command required error")
	}
}
