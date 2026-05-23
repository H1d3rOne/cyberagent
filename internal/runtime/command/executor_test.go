package command

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestExecutorRunOutputLimitRecordsFailure(t *testing.T) {
	recorder := NewRecorder(filepath.Join(t.TempDir(), "commands.log"))
	result, err := NewExecutor(recorder).Run(context.Background(), ExecRequest{
		ID:             "toolcall_limit_001",
		JobID:          "job_001",
		StepID:         "step_001",
		ToolName:       "printf",
		Command:        "sh",
		Args:           []string{"-c", "printf '1234567890'"},
		MaxStdoutBytes: 4,
	})
	if err == nil || !strings.Contains(err.Error(), "output limit") {
		t.Fatalf("expected output limit error, got %v", err)
	}
	if result.Call.Status != contracts.StatusFailed || result.Call.Metadata["error_class"] != "output_limit" || result.Call.Metadata["stdout_truncated"] != true {
		t.Fatalf("unexpected limited call: %#v", result.Call)
	}
	if string(result.Stdout) != "1234" {
		t.Fatalf("stdout = %q, want first 4 bytes", result.Stdout)
	}
	calls, err := recorder.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(calls) != 1 || calls[0].Metadata["error_class"] != "output_limit" {
		t.Fatalf("unexpected recorded calls: %#v", calls)
	}
}

func TestExecutorRunTimeoutClassifiesFailure(t *testing.T) {
	recorder := NewRecorder(filepath.Join(t.TempDir(), "commands.log"))
	result, err := NewExecutor(recorder).Run(context.Background(), ExecRequest{
		ID:       "toolcall_timeout_001",
		JobID:    "job_001",
		StepID:   "step_001",
		ToolName: "sleep",
		Command:  "sh",
		Args:     []string{"-c", "sleep 1"},
		Timeout:  20 * time.Millisecond,
	})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if result.Call.Status != contracts.StatusFailed || result.Call.Metadata["error_class"] != "timeout" {
		t.Fatalf("unexpected timeout call: %#v", result.Call)
	}
}

func TestExecutorRunNoRecordSkipsRecorderAppend(t *testing.T) {
	recorder := NewRecorder(filepath.Join(t.TempDir(), "commands.log"))
	result, err := NewExecutor(recorder).Run(context.Background(), ExecRequest{
		ID:       "toolcall_no_record_001",
		JobID:    "job_001",
		StepID:   "step_001",
		ToolName: "echo",
		Command:  "sh",
		Args:     []string{"-c", "printf ok"},
		NoRecord: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Call.Status != contracts.StatusSucceeded || string(result.Stdout) != "ok" {
		t.Fatalf("unexpected no-record result: %#v stdout=%q", result.Call, result.Stdout)
	}
	calls, err := recorder.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("expected no recorded calls, got %#v", calls)
	}
}
