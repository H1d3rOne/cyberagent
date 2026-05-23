package command

import (
	"path/filepath"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestRecorderAppendAndReadAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commands.log")
	recorder := NewRecorder(path)
	call := contracts.ToolCall{
		ID:        "toolcall_001",
		JobID:     "job_001",
		StepID:    "step_001",
		ToolName:  "builtin.http_probe",
		RunnerID:  "local",
		Kind:      contracts.ToolCallKindBuiltin,
		Command:   "builtin.http_probe",
		StartedAt: contracts.Now(),
		Status:    contracts.StatusSucceeded,
	}
	if err := recorder.Append(call); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	calls, err := recorder.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1", len(calls))
	}
	if calls[0].ID != call.ID || calls[0].ToolName != call.ToolName || calls[0].Kind != contracts.ToolCallKindBuiltin {
		t.Fatalf("unexpected call: %#v", calls[0])
	}
}

func TestNewBuiltinCallAndFinish(t *testing.T) {
	step := contracts.NewStep("step_http_probe_001", "job_001", "plan_001", "HTTP Probe", contracts.StepTypeBuiltinTool, contracts.RiskLow, contracts.ToolRef{Name: "builtin.http_probe", Version: "v1"})
	call := NewBuiltinCall("toolcall_001", "job_001", step)
	if call.Status != contracts.StatusRunning {
		t.Fatalf("status = %s, want running", call.Status)
	}
	if call.Kind != contracts.ToolCallKindBuiltin || call.Command != "builtin.http_probe" {
		t.Fatalf("unexpected call: %#v", call)
	}

	finished := Finish(call, contracts.StatusSucceeded, nil)
	if finished.Status != contracts.StatusSucceeded {
		t.Fatalf("status = %s, want succeeded", finished.Status)
	}
	if finished.FinishedAt == nil {
		t.Fatalf("FinishedAt is nil")
	}
	if finished.ExitCode == nil || *finished.ExitCode != 0 {
		t.Fatalf("ExitCode = %#v, want 0", finished.ExitCode)
	}
}
