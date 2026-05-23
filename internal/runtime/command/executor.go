package command

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// ExecRequest describes one controlled external command invocation.
type ExecRequest struct {
	ID          string
	JobID       string
	StepID      string
	ToolName    string
	ToolVersion string
	Command     string
	Args        []string
	CWD         string
	Timeout     time.Duration
	Kind        contracts.ToolCallKind
	Stdin       []byte
}

// ExecResult contains the completed tool call and captured process output.
type ExecResult struct {
	Call   contracts.ToolCall
	Stdout []byte
	Stderr []byte
}

// Executor runs external commands through a recorder-aware boundary.
type Executor struct {
	Recorder *Recorder
	RunnerID string
}

// NewExecutor creates an external command executor.
func NewExecutor(recorder *Recorder) Executor {
	return Executor{Recorder: recorder, RunnerID: "local"}
}

// Run executes one external command and appends the completed call to commands.log.
func (e Executor) Run(ctx context.Context, req ExecRequest) (ExecResult, error) {
	if req.Command == "" {
		return ExecResult{}, fmt.Errorf("command is required")
	}
	runnerID := e.RunnerID
	if runnerID == "" {
		runnerID = "local"
	}
	kind := req.Kind
	if kind == "" {
		kind = contracts.ToolCallKindCommand
	}
	call := contracts.ToolCall{
		ID:          req.ID,
		JobID:       req.JobID,
		StepID:      req.StepID,
		ToolName:    req.ToolName,
		ToolVersion: req.ToolVersion,
		RunnerID:    runnerID,
		Kind:        kind,
		Command:     req.Command,
		Args:        append([]string(nil), req.Args...),
		CWD:         req.CWD,
		EnvPolicy:   "inherit_minimal",
		StartedAt:   contracts.Now(),
		Status:      contracts.StatusRunning,
		Metadata:    contracts.Metadata{},
	}

	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, req.Command, req.Args...)
	cmd.Dir = req.CWD
	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	status := contracts.StatusSucceeded
	exitCode := 0
	if err != nil {
		status = contracts.StatusFailed
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if runCtx.Err() != nil {
			err = runCtx.Err()
		}
	}
	finished := contracts.Now()
	call.FinishedAt = &finished
	call.Status = status
	call.ExitCode = &exitCode
	if err != nil {
		call.Error = err.Error()
	}
	if e.Recorder != nil {
		if recordErr := e.Recorder.Append(call); recordErr != nil && err == nil {
			err = recordErr
		}
	}
	return ExecResult{Call: call, Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}
