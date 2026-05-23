package services

import (
	"context"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/runtime/doctor"
)

const (
	defaultName    = "CyberAgent"
	defaultVersion = "v0.1.0"
	defaultModule  = "github.com/H1d3rOne/cyberagent"
)

// BuildInfo captures binary provenance for service consumers.
type BuildInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
	Module  string `json:"module"`
}

// VersionInfo is returned by VersionService.
type VersionInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
	Module  string `json:"module"`
}

// VersionService exposes version and runtime readiness information.
type VersionService struct {
	build BuildInfo
}

// NewVersionService creates a version service with normalized defaults.
func NewVersionService(build BuildInfo) VersionService {
	if strings.TrimSpace(build.Name) == "" {
		build.Name = defaultName
	}
	if strings.TrimSpace(build.Version) == "" {
		build.Version = defaultVersion
	}
	if strings.TrimSpace(build.Module) == "" {
		build.Module = defaultModule
	}
	return VersionService{build: build}
}

// GetVersion returns binary provenance without performing I/O.
func (s VersionService) GetVersion(context.Context) VersionInfo {
	return VersionInfo{
		Name:    s.build.Name,
		Version: s.build.Version,
		Commit:  s.build.Commit,
		Date:    s.build.Date,
		Module:  s.build.Module,
	}
}

// Doctor runs local environment checks.
func (s VersionService) Doctor(ctx context.Context) doctor.Report {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	return doctor.Run(ctx)
}
