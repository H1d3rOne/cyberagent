package tenant

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStorageResolverDefaultLocalPathsEvenWithFutureMultiUserContext(t *testing.T) {
	resolver, err := NewStorageResolver(StorageConfig{
		LocalRunsDir:           filepath.Join("local", "runs"),
		LocalProviderStorePath: filepath.Join("local", "providers.json"),
		LocalRuntimeDBPath:     filepath.Join("local", "runtime.db"),
		MultiUserStorageRoot:   filepath.Join("data", "multi"),
	})
	if err != nil {
		t.Fatalf("NewStorageResolver() error = %v", err)
	}
	ctx := WithContext(context.Background(), TenantContext{
		TenantID:    "tenant_a",
		WorkspaceID: "workspace_a",
		Mode:        ModeMultiUser,
		Principal:   LocalPrincipal(),
	})

	paths, err := resolver.ResolveStorage(ctx)
	if err != nil {
		t.Fatalf("ResolveStorage() error = %v", err)
	}
	if paths.RunsDir != filepath.Join("local", "runs") ||
		paths.ProviderStorePath != filepath.Join("local", "providers.json") ||
		paths.RuntimeDBPath != filepath.Join("local", "runtime.db") {
		t.Fatalf("disabled resolver changed local paths: %#v", paths)
	}
}

func TestStorageResolverMultiUserGateResolvesWorkspacePaths(t *testing.T) {
	root := filepath.Join("data", "multi")
	resolver, err := NewStorageResolver(StorageConfig{
		LocalRunsDir:           filepath.Join("local", "runs"),
		LocalProviderStorePath: filepath.Join("local", "providers.json"),
		LocalRuntimeDBPath:     filepath.Join("local", "runtime.db"),
		MultiUserEnabled:       true,
		MultiUserStorageRoot:   root,
	})
	if err != nil {
		t.Fatalf("NewStorageResolver() error = %v", err)
	}
	ctx := WithContext(context.Background(), TenantContext{
		TenantID:    "tenant_a",
		WorkspaceID: "workspace_b",
		Mode:        ModeMultiUser,
		Principal:   LocalPrincipal(),
	})

	paths, err := resolver.ResolveStorage(ctx)
	if err != nil {
		t.Fatalf("ResolveStorage() error = %v", err)
	}
	workspaceRoot := filepath.Join(root, "tenants", "tenant_a", "workspaces", "workspace_b")
	if paths.Mode != ModeMultiUser ||
		paths.WorkspaceRoot != workspaceRoot ||
		paths.RunsDir != filepath.Join(workspaceRoot, "runs") ||
		paths.ProviderStorePath != filepath.Join(workspaceRoot, ".agent", "providers.json") ||
		paths.RuntimeDBPath != filepath.Join(workspaceRoot, ".agent", "runtime", "runtime.db") {
		t.Fatalf("ResolveStorage() = %#v, want workspace scoped paths under %q", paths, workspaceRoot)
	}
}

func TestStorageResolverMultiUserRequiresExplicitRoot(t *testing.T) {
	if _, err := NewStorageResolver(StorageConfig{MultiUserEnabled: true}); err == nil {
		t.Fatal("NewStorageResolver() should require storage_root when multi-user is enabled")
	}
}

func TestStorageResolverRejectsUnsafeScopeSegments(t *testing.T) {
	resolver, err := NewStorageResolver(StorageConfig{
		LocalRunsDir:         "runs",
		MultiUserEnabled:     true,
		MultiUserStorageRoot: "data",
	})
	if err != nil {
		t.Fatalf("NewStorageResolver() error = %v", err)
	}
	ctx := WithContext(context.Background(), TenantContext{
		TenantID:    "../tenant",
		WorkspaceID: "workspace",
		Mode:        ModeMultiUser,
		Principal:   LocalPrincipal(),
	})
	if _, err := resolver.ResolveStorage(ctx); err == nil {
		t.Fatal("ResolveStorage() should reject unsafe tenant_id")
	}
}
