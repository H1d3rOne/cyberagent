package services

import (
	"context"
	"testing"
)

func TestVersionServiceDefaultsAndBuildInfo(t *testing.T) {
	service := NewVersionService(BuildInfo{
		Version: "v0.1.2-test",
		Commit:  "abc123",
		Date:    "2026-05-24T00:00:00Z",
	})

	info := service.GetVersion(context.Background())
	if info.Name != defaultName {
		t.Fatalf("Name = %q, want %q", info.Name, defaultName)
	}
	if info.Version != "v0.1.2-test" || info.Commit != "abc123" || info.Date != "2026-05-24T00:00:00Z" {
		t.Fatalf("unexpected version info: %#v", info)
	}
	if info.Module != defaultModule {
		t.Fatalf("Module = %q, want %q", info.Module, defaultModule)
	}
}

func TestVersionServiceDoctorRuns(t *testing.T) {
	service := NewVersionService(BuildInfo{})
	report := service.Doctor(context.Background())
	if report.OS == "" || report.Arch == "" {
		t.Fatalf("doctor report missing platform: %#v", report)
	}
	if len(report.Checks) == 0 {
		t.Fatalf("doctor report has no checks")
	}
}
