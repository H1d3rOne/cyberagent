package command

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func newTempExecutor(t *testing.T) *Executor {
	t.Helper()
	return NewExecutor(t.TempDir(), 5*time.Second, 1<<20)
}

func TestExecuteAllowedEcho(t *testing.T) {
	executor := newTempExecutor(t)
	res, err := executor.Execute(context.Background(), "echo hello", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}
	if strings.TrimSpace(res.Stdout) != "hello" {
		t.Fatalf("expected stdout 'hello', got %q", res.Stdout)
	}
	if res.Command != "echo hello" {
		t.Fatalf("unexpected command: %s", res.Command)
	}
}

func TestExecuteRelativeWorkdir(t *testing.T) {
	base := t.TempDir()
	child := filepath.Join(base, "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdir child: %v", err)
	}
	executor := NewExecutor(base, 5*time.Second, 1<<20)
	res, err := executor.Execute(context.Background(), "pwd", "child")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(base, "child")
	if strings.TrimSpace(res.Stdout) != expected {
		t.Fatalf("expected workdir %s, got %s", expected, strings.TrimSpace(res.Stdout))
	}
}

func TestExecuteDisallowedCommand(t *testing.T) {
	executor := newTempExecutor(t)
	_, err := executor.Execute(context.Background(), "curl http://example.com", "")
	if err == nil {
		t.Fatal("expected error for disallowed command")
	}
	if !strings.Contains(err.Error(), "command not allowed") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestExecuteRejectsDestructiveCommand(t *testing.T) {
	executor := newTempExecutor(t)
	for _, cmd := range []string{"rm notes.md", "mv a b", "python -c print(1)", "git checkout main"} {
		if executor.IsAllowed(cmd) {
			t.Fatalf("expected %q to be denied by default allowlist", cmd)
		}
		_, err := executor.Execute(context.Background(), cmd, "")
		if err == nil {
			t.Fatalf("expected error for %q", cmd)
		}
		if !strings.Contains(err.Error(), "command not allowed") {
			t.Fatalf("unexpected error for %q: %v", cmd, err)
		}
	}
	if !executor.IsAllowed("git status") {
		t.Fatal("expected read-only git status to be allowed")
	}
}

func TestExecuteRejectsWorkspaceEscape(t *testing.T) {
	root := t.TempDir()
	executor := NewExecutor(filepath.Join(root, "workspace"), 5*time.Second, 1<<20)
	if err := os.MkdirAll(filepath.Join(root, "workspace"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	if _, err := executor.Execute(context.Background(), "pwd", ".."); err == nil || !strings.Contains(err.Error(), "workdir escapes workspace") {
		t.Fatalf("expected workdir escape error, got %v", err)
	}
	if _, err := executor.Execute(context.Background(), "cat ../secret.txt", ""); err == nil || !strings.Contains(err.Error(), "argument escapes workspace") {
		t.Fatalf("expected argument escape error, got %v", err)
	}
	if _, err := executor.Execute(context.Background(), "cat /etc/passwd", ""); err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
}

func TestExecuteNonZeroExit(t *testing.T) {
	executor := newTempExecutor(t)
	res, err := executor.Execute(context.Background(), "ls does-not-exist", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if strings.TrimSpace(res.Stderr) == "" {
		t.Fatal("expected stderr output for missing path")
	}
}

func TestExecuteTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep based timeout test skipped on windows")
	}
	executor := NewExecutor(t.TempDir(), time.Second, 1<<20)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := executor.Execute(ctx, "sleep 5", "")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
