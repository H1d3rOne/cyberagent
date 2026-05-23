package command

import (
	"bytes"
	"context"
	"errors"
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
	// MaxStdoutBytes and MaxStderrBytes cap captured output. Zero means unlimited.
	MaxStdoutBytes int64
	MaxStderrBytes int64
	// NoRecord lets higher-level runners validate and record the final call themselves.
	NoRecord bool
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
	stdout := newLimitedBuffer(req.MaxStdoutBytes)
	stderr := newLimitedBuffer(req.MaxStderrBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	stdoutExceeded := stdout.Exceeded()
	stderrExceeded := stderr.Exceeded()
	outputLimitExceeded := stdoutExceeded || stderrExceeded
	if outputLimitExceeded && err == nil {
		err = errors.New("command output limit exceeded")
	}
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
	if req.MaxStdoutBytes > 0 {
		call.Metadata["stdout_limit_bytes"] = req.MaxStdoutBytes
		call.Metadata["stdout_bytes_captured"] = int64(len(stdout.Bytes()))
		call.Metadata["stdout_truncated"] = stdoutExceeded
	}
	if req.MaxStderrBytes > 0 {
		call.Metadata["stderr_limit_bytes"] = req.MaxStderrBytes
		call.Metadata["stderr_bytes_captured"] = int64(len(stderr.Bytes()))
		call.Metadata["stderr_truncated"] = stderrExceeded
	}
	if err != nil {
		call.Metadata["error_class"] = classifyExecutionError(err, outputLimitExceeded)
		if outputLimitExceeded {
			call.Metadata["output_limit_exceeded"] = true
		}
	}
	if e.Recorder != nil && !req.NoRecord {
		if recordErr := e.Recorder.Append(call); recordErr != nil && err == nil {
			err = recordErr
		}
	}
	return ExecResult{Call: call, Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

func classifyExecutionError(err error, outputLimitExceeded bool) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if outputLimitExceeded {
		return "output_limit"
	}
	return "execution_failed"
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int64
	total     int64
	truncated bool
}

func newLimitedBuffer(limit int64) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.total += int64(len(p))
	if b.limit <= 0 {
		_, _ = b.buf.Write(p)
		return len(p), nil
	}
	remaining := b.limit - int64(b.buf.Len())
	if remaining > 0 {
		toWrite := int64(len(p))
		if toWrite > remaining {
			toWrite = remaining
		}
		_, _ = b.buf.Write(p[:toWrite])
	}
	if b.total > b.limit {
		b.truncated = true
	}
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

func (b *limitedBuffer) Exceeded() bool {
	return b.truncated
}
