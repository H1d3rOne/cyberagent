package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
)

func TestDaemonStorageResolverDefaultLocalPathsEvenWithMultiUserContext(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	resolver, err := daemonStorageResolverFromConfig(Config{RunsDir: runsDir})
	if err != nil {
		t.Fatalf("daemonStorageResolverFromConfig() error = %v", err)
	}

	paths, err := resolver.ResolveStorage(daemonTenantTestContext())
	if err != nil {
		t.Fatalf("ResolveStorage() error = %v", err)
	}
	if paths.RunsDir != runsDir {
		t.Fatalf("RunsDir = %q, want local runs dir %q", paths.RunsDir, runsDir)
	}
	if paths.RuntimeDBPath != agentRuntimeSQLitePath(runsDir) {
		t.Fatalf("RuntimeDBPath = %q, want %q", paths.RuntimeDBPath, agentRuntimeSQLitePath(runsDir))
	}
	if paths.ProviderStorePath != defaultProviderStorePath() {
		t.Fatalf("ProviderStorePath = %q, want default provider store path %q", paths.ProviderStorePath, defaultProviderStorePath())
	}
}

func TestDaemonStorageResolverMultiUserGateRequiresStorageRoot(t *testing.T) {
	if _, err := daemonStorageResolverFromConfig(Config{
		RunsDir:   t.TempDir(),
		MultiUser: MultiUserConfig{Enabled: true},
	}); err == nil {
		t.Fatal("daemonStorageResolverFromConfig() should require explicit storage root when multi-user is enabled")
	}
}

func TestDaemonStorageResolverMultiUserGateResolvesWorkspacePaths(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	storageRoot := filepath.Join(t.TempDir(), "data")
	resolver, err := daemonStorageResolverFromConfig(Config{
		RunsDir: runsDir,
		MultiUser: MultiUserConfig{
			Enabled:     true,
			StorageRoot: storageRoot,
		},
	})
	if err != nil {
		t.Fatalf("daemonStorageResolverFromConfig() error = %v", err)
	}
	ctx := coretenant.WithContext(context.Background(), coretenant.TenantContext{
		TenantID:    "tenant_daemon",
		WorkspaceID: "workspace_daemon",
		Mode:        coretenant.ModeMultiUser,
		Principal:   coretenant.LocalPrincipal(),
	})

	paths, err := resolver.ResolveStorage(ctx)
	if err != nil {
		t.Fatalf("ResolveStorage() error = %v", err)
	}
	workspaceRoot := filepath.Join(storageRoot, "tenants", "tenant_daemon", "workspaces", "workspace_daemon")
	if paths.RunsDir != filepath.Join(workspaceRoot, "runs") ||
		paths.ProviderStorePath != filepath.Join(workspaceRoot, ".agent", "providers.json") ||
		paths.RuntimeDBPath != filepath.Join(workspaceRoot, ".agent", "runtime", "runtime.db") {
		t.Fatalf("ResolveStorage() = %#v, want workspace paths under %q", paths, workspaceRoot)
	}
}

func TestProviderStoreMultiUserStorageResolverScopesContextPaths(t *testing.T) {
	localPath := filepath.Join(t.TempDir(), "local", "providers.json")
	storageRoot := filepath.Join(t.TempDir(), "data")
	resolver, err := coretenant.NewStorageResolver(coretenant.StorageConfig{
		LocalProviderStorePath: localPath,
		MultiUserEnabled:       true,
		MultiUserStorageRoot:   storageRoot,
	})
	if err != nil {
		t.Fatalf("NewStorageResolver() error = %v", err)
	}
	store := newProviderStoreWithStorageResolver(localPath, resolver)
	ctx := coretenant.WithContext(context.Background(), coretenant.TenantContext{
		TenantID:    "tenant_store",
		WorkspaceID: "workspace_store",
		Mode:        coretenant.ModeMultiUser,
		Principal:   coretenant.LocalPrincipal(),
	})
	cfg := web.ProviderConfig{
		ActiveID:  "primary",
		Providers: []web.ProviderProfile{{ID: "primary", Name: "Primary", Type: "openai", Model: "gpt-test"}},
	}

	if err := store.SaveContext(ctx, cfg); err != nil {
		t.Fatalf("SaveContext() error = %v", err)
	}
	scopedPath := filepath.Join(storageRoot, "tenants", "tenant_store", "workspaces", "workspace_store", ".agent", "providers.json")
	if _, err := os.Stat(scopedPath); err != nil {
		t.Fatalf("expected scoped provider config at %s: %v", scopedPath, err)
	}
	loaded, err := store.LoadContext(ctx)
	if err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	if loaded.ActiveID != "primary" || len(loaded.Providers) != 1 {
		t.Fatalf("LoadContext() = %#v, want scoped provider config", loaded)
	}
	localLoaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(localLoaded.Providers) != 0 {
		t.Fatalf("Load() should keep local provider store separate, got %#v", localLoaded)
	}
}

func TestJobManagerMultiUserStorageResolverScopesNewRunDirs(t *testing.T) {
	localRunsDir := filepath.Join(t.TempDir(), "runs")
	storageRoot := filepath.Join(t.TempDir(), "data")
	resolver, err := coretenant.NewStorageResolver(coretenant.StorageConfig{
		LocalRunsDir:         localRunsDir,
		MultiUserEnabled:     true,
		MultiUserStorageRoot: storageRoot,
	})
	if err != nil {
		t.Fatalf("NewStorageResolver() error = %v", err)
	}
	capturedOut := make(chan string, 1)
	manager := newJobManager(jobManagerConfig{
		RunsDir:           localRunsDir,
		MaxConcurrentJobs: 1,
		StorageResolver:   resolver,
		Runner: func(ctx context.Context, req coreruntime.WebScanRequest) (coreruntime.WebScanResult, error) {
			capturedOut <- req.Out
			return coreruntime.WebScanResult{Workspace: req.Out}, nil
		},
	})
	ctx := coretenant.WithContext(context.Background(), coretenant.TenantContext{
		TenantID:    "tenant_job",
		WorkspaceID: "workspace_job",
		Mode:        coretenant.ModeMultiUser,
		Principal:   coretenant.LocalPrincipal(),
	})

	response, err := manager.StartWebScan(ctx, startWebScanJobRequest{
		Target:          "https://target.example",
		Scope:           "https://target.example",
		NoExternalTools: true,
	})
	if err != nil {
		t.Fatalf("StartWebScan() error = %v", err)
	}
	wantPrefix := filepath.Join(storageRoot, "tenants", "tenant_job", "workspaces", "workspace_job", "runs") + string(filepath.Separator)
	if !strings.HasPrefix(response.RunDir, wantPrefix) {
		t.Fatalf("RunDir = %q, want scoped prefix %q", response.RunDir, wantPrefix)
	}
	select {
	case got := <-capturedOut:
		if got != response.RunDir {
			t.Fatalf("runner Out = %q, want response RunDir %q", got, response.RunDir)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for web scan runner")
	}
}
