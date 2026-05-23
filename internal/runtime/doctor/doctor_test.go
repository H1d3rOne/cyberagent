package doctor

import (
	"context"
	"testing"
	"time"
)

func TestRunReturnsReport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report := Run(ctx)

	if report.OS == "" {
		t.Error("expected OS to be set")
	}
	if report.Arch == "" {
		t.Error("expected Arch to be set")
	}
	if len(report.Checks) == 0 {
		t.Fatal("expected at least one check")
	}
}

func TestRunWorkspaceCheckPasses(t *testing.T) {
	ctx := context.Background()
	report := Run(ctx)

	var workspaceCheck *CheckResult
	for i := range report.Checks {
		if report.Checks[i].Name == "workspace" {
			workspaceCheck = &report.Checks[i]
			break
		}
	}
	if workspaceCheck == nil {
		t.Fatal("expected workspace check to exist")
	}
	if workspaceCheck.Status != StatusOK {
		t.Errorf("expected workspace check to pass, got %s: %s", workspaceCheck.Status, workspaceCheck.Detail)
	}
	if !workspaceCheck.Required {
		t.Error("expected workspace check to be required")
	}
}

func TestRunHealthyWhenRequiredPass(t *testing.T) {
	ctx := context.Background()
	report := Run(ctx)

	allRequiredPass := true
	for _, check := range report.Checks {
		if check.Required && check.Status != StatusOK {
			allRequiredPass = false
			break
		}
	}
	if report.Healthy != allRequiredPass {
		t.Errorf("Healthy=%v but allRequiredPass=%v", report.Healthy, allRequiredPass)
	}
}

func TestRunOptionalToolsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	ctx := context.Background()
	report := Run(ctx)

	for _, check := range report.Checks {
		if check.Name == "workspace" {
			continue
		}
		if check.Required {
			t.Errorf("tool %s should not be required", check.Name)
		}
		if check.Status != StatusMissing {
			t.Errorf("tool %s should be missing with empty PATH, got %s", check.Name, check.Status)
		}
	}
	if !report.Healthy {
		t.Error("report should still be healthy when only optional tools are missing")
	}
}

func TestRunCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report := Run(ctx)
	if len(report.Checks) == 0 {
		t.Fatal("expected checks even with cancelled context")
	}
}

func TestCheckWritableDir(t *testing.T) {
	result := checkWritableDir("test", t.TempDir(), true)
	if result.Status != StatusOK {
		t.Errorf("expected OK for writable temp dir, got %s: %s", result.Status, result.Detail)
	}
}

func TestCheckWritableDirFails(t *testing.T) {
	result := checkWritableDir("test", "/nonexistent-path-xyz", true)
	if result.Status != StatusError {
		t.Errorf("expected Error for non-writable dir, got %s", result.Status)
	}
}
