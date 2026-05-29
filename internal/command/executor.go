package command

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultTimeout   = 30 * time.Second
	MaxTimeout       = 300 * time.Second
	DefaultMaxOutput = 1 * 1024 * 1024  // 1MB
	MaxMaxOutput     = 10 * 1024 * 1024 // 10MB
)

// Executor executes commands in a controlled environment.
type Executor struct {
	allowedCommands map[string]struct{}
	timeout         time.Duration
	maxOutput       int
	workDir         string
}

// NewExecutor creates a new command executor.
func NewExecutor(workDir string, timeout time.Duration, maxOutput int) *Executor {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	}
	if maxOutput <= 0 {
		maxOutput = DefaultMaxOutput
	}
	if maxOutput > MaxMaxOutput {
		maxOutput = MaxMaxOutput
	}

	return &Executor{
		allowedCommands: defaultAllowedCommands(),
		timeout:         timeout,
		maxOutput:       maxOutput,
		workDir:         workDir,
	}
}

// Result represents the result of a command execution.
type Result struct {
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMs int64  `json:"duration_ms"`
	WorkDir    string `json:"workdir"`
}

// Execute executes a command.
func (e *Executor) Execute(ctx context.Context, command string, workdir string) (*Result, error) {
	// Parse command
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	if !e.isAllowedInvocation(parts) {
		return nil, fmt.Errorf("command not allowed: %s", command)
	}
	if err := validateWorkspaceArgs(parts[1:]); err != nil {
		return nil, err
	}

	// Resolve work directory
	absWorkDir, err := e.resolveWorkDir(workdir)
	if err != nil {
		return nil, err
	}

	// Create command
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmdName := parts[0]
	cmd := exec.CommandContext(ctx, cmdName, parts[1:]...)
	cmd.Dir = absWorkDir

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start)

	// Truncate output if too large
	stdoutStr := truncateString(stdout.String(), e.maxOutput)
	stderrStr := truncateString(stderr.String(), e.maxOutput)

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("execute command: %w", err)
		}
	}

	return &Result{
		Command:    command,
		ExitCode:   exitCode,
		Stdout:     stdoutStr,
		Stderr:     stderrStr,
		DurationMs: duration.Milliseconds(),
		WorkDir:    absWorkDir,
	}, nil
}

// IsAllowed checks if a command is allowed.
func (e *Executor) IsAllowed(command string) bool {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return false
	}
	return e.isAllowedInvocation(parts) && validateWorkspaceArgs(parts[1:]) == nil
}

func (e *Executor) isAllowedInvocation(parts []string) bool {
	if len(parts) == 0 {
		return false
	}
	if _, ok := e.allowedCommands[parts[0]]; !ok {
		return false
	}
	if parts[0] != "git" {
		return true
	}
	if len(parts) < 2 {
		return false
	}
	switch parts[1] {
	case "status", "log", "show", "diff", "branch", "rev-parse", "ls-files":
		return true
	default:
		return false
	}
}

func (e *Executor) resolveWorkDir(workdir string) (string, error) {
	base, err := filepath.Abs(e.workDir)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	workdir = strings.TrimSpace(workdir)
	if workdir == "" || workdir == "." {
		return base, nil
	}
	if filepath.IsAbs(workdir) {
		return "", fmt.Errorf("workdir must be workspace-relative")
	}
	candidate, err := filepath.Abs(filepath.Join(base, filepath.Clean(workdir)))
	if err != nil {
		return "", fmt.Errorf("resolve workdir: %w", err)
	}
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve workdir relation: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("workdir escapes workspace: %s", workdir)
	}
	return candidate, nil
}

func validateWorkspaceArgs(args []string) error {
	for _, arg := range args {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		if filepath.IsAbs(arg) {
			return fmt.Errorf("command argument escapes workspace: absolute path %s", arg)
		}
		clean := filepath.Clean(arg)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("command argument escapes workspace: %s", arg)
		}
	}
	return nil
}

// defaultAllowedCommands returns the default set of low-risk read-only commands.
func defaultAllowedCommands() map[string]struct{} {
	commands := []string{
		// Workspace read/inspection.
		"pwd", "ls", "cat", "head", "tail", "wc", "grep", "find", "sort", "uniq", "cut",
		// Text/system metadata helpers.
		"echo", "printf", "date", "whoami", "which", "file", "stat", "uname", "hostname", "id", "groups",
		// Version control read-only subcommands are checked separately.
		"git",
	}

	allowed := make(map[string]struct{}, len(commands))
	for _, cmd := range commands {
		allowed[cmd] = struct{}{}
	}
	return allowed
}

// truncateString truncates a string to a maximum length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}
