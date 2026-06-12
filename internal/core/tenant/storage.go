package tenant

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// StorageConfig describes the local storage paths and the explicit multi-user
// gate. When MultiUserEnabled is false, ResolveStorage always returns the local
// paths even if the context carries a future multi-user TenantContext.
type StorageConfig struct {
	LocalRunsDir           string
	LocalProviderStorePath string
	LocalRuntimeDBPath     string

	MultiUserEnabled     bool
	MultiUserStorageRoot string
}

// StoragePaths is the resolved filesystem layout for the effective
// tenant/workspace. Local mode intentionally mirrors the existing single-user
// paths so enabling L3 skeleton code does not migrate historical data.
type StoragePaths struct {
	Mode        string
	TenantID    string
	WorkspaceID string

	RunsDir           string
	ProviderStorePath string
	RuntimeDBPath     string
	WorkspaceRoot     string
}

// StorageResolver resolves storage paths from request-scoped TenantContext.
type StorageResolver interface {
	ResolveStorage(ctx context.Context) (StoragePaths, error)
}

// PathStorageResolver is a conservative filesystem resolver. It only switches
// to scoped paths when multi-user mode is explicitly enabled and the
// TenantContext itself is multi_user.
type PathStorageResolver struct {
	cfg StorageConfig
}

// NewStorageResolver validates and builds a path-based storage resolver.
func NewStorageResolver(cfg StorageConfig) (*PathStorageResolver, error) {
	cfg.LocalRunsDir = strings.TrimSpace(cfg.LocalRunsDir)
	cfg.LocalProviderStorePath = strings.TrimSpace(cfg.LocalProviderStorePath)
	cfg.LocalRuntimeDBPath = strings.TrimSpace(cfg.LocalRuntimeDBPath)
	cfg.MultiUserStorageRoot = strings.TrimSpace(cfg.MultiUserStorageRoot)
	if cfg.MultiUserEnabled && cfg.MultiUserStorageRoot == "" {
		return nil, errors.New("multi-user storage_root is required when multi-user mode is enabled")
	}
	return &PathStorageResolver{cfg: cfg}, nil
}

// ResolveStorage resolves local/default or tenant/workspace scoped storage.
func (r *PathStorageResolver) ResolveStorage(ctx context.Context) (StoragePaths, error) {
	if r == nil {
		return StoragePaths{}, errors.New("storage resolver is nil")
	}
	tc := FromContextOrLocal(ctx)
	if !r.cfg.MultiUserEnabled || tc.Mode != ModeMultiUser {
		return r.localPaths(tc), nil
	}
	tenantID, err := safeStorageSegment("tenant_id", tc.TenantID)
	if err != nil {
		return StoragePaths{}, err
	}
	workspaceID, err := safeStorageSegment("workspace_id", tc.WorkspaceID)
	if err != nil {
		return StoragePaths{}, err
	}
	workspaceRoot := filepath.Join(r.cfg.MultiUserStorageRoot, "tenants", tenantID, "workspaces", workspaceID)
	return StoragePaths{
		Mode:              ModeMultiUser,
		TenantID:          tc.TenantID,
		WorkspaceID:       tc.WorkspaceID,
		RunsDir:           filepath.Join(workspaceRoot, "runs"),
		ProviderStorePath: filepath.Join(workspaceRoot, ".agent", "providers.json"),
		RuntimeDBPath:     filepath.Join(workspaceRoot, ".agent", "runtime", "runtime.db"),
		WorkspaceRoot:     workspaceRoot,
	}, nil
}

func (r *PathStorageResolver) localPaths(tc TenantContext) StoragePaths {
	if strings.TrimSpace(tc.TenantID) == "" {
		tc.TenantID = LocalTenantID
	}
	if strings.TrimSpace(tc.WorkspaceID) == "" {
		tc.WorkspaceID = DefaultWorkspaceID
	}
	mode := tc.Mode
	if strings.TrimSpace(mode) == "" {
		mode = ModeLocal
	}
	return StoragePaths{
		Mode:              mode,
		TenantID:          tc.TenantID,
		WorkspaceID:       tc.WorkspaceID,
		RunsDir:           r.cfg.LocalRunsDir,
		ProviderStorePath: r.cfg.LocalProviderStorePath,
		RuntimeDBPath:     r.cfg.LocalRuntimeDBPath,
	}
}

func safeStorageSegment(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required for multi-user storage", field)
	}
	if value == "." || value == ".." || strings.HasPrefix(value, ".") {
		return "", fmt.Errorf("%s contains an unsafe storage segment", field)
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return "", fmt.Errorf("%s contains an unsafe storage segment", field)
		}
	}
	return value, nil
}
