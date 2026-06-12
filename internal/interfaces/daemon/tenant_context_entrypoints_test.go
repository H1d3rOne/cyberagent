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

func TestDaemonJobManagerTenantContextPropagatesWithoutChangingRunsDir(t *testing.T) {
	runsDir := t.TempDir()
	type capturedRequest struct {
		out string
		tc  coretenant.TenantContext
		ok  bool
	}
	captured := make(chan capturedRequest, 1)
	manager := newJobManager(jobManagerConfig{
		RunsDir:           runsDir,
		MaxConcurrentJobs: 1,
		Runner: func(ctx context.Context, req coreruntime.WebScanRequest) (coreruntime.WebScanResult, error) {
			tc, ok := coretenant.FromContext(ctx)
			captured <- capturedRequest{out: req.Out, tc: tc, ok: ok}
			return coreruntime.WebScanResult{Workspace: req.Out}, nil
		},
	})

	ctx := daemonTenantTestContext()
	response, err := manager.StartWebScan(ctx, startWebScanJobRequest{
		Target:          "https://target.example",
		Scope:           "https://target.example",
		NoExternalTools: true,
	})
	if err != nil {
		t.Fatalf("StartWebScan() error = %v", err)
	}
	if !strings.HasPrefix(response.RunDir, runsDir+string(filepath.Separator)) {
		t.Fatalf("RunDir = %q, want under configured runsDir %q", response.RunDir, runsDir)
	}

	select {
	case got := <-captured:
		if !got.ok {
			t.Fatal("runner context missing tenant context")
		}
		if got.tc.TenantID != "tenant-daemon" || got.tc.WorkspaceID != "workspace-daemon" || got.tc.Mode != coretenant.ModeMultiUser {
			t.Fatalf("runner tenant context = %#v, want propagated future scope", got.tc)
		}
		if got.out != response.RunDir {
			t.Fatalf("runner Out = %q, want response RunDir %q", got.out, response.RunDir)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for web scan runner")
	}
}

func TestScheduleManagerTenantContextDoesNotChangeStoreDir(t *testing.T) {
	runsDir := t.TempDir()
	manager := newScheduleManager(runsDir)

	created, err := manager.CreateWithContext(daemonTenantTestContext(), scheduleSpec{
		Cron:    "@hourly",
		Prompt:  "扫描 https://scheduled.example",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("CreateWithContext() error = %v", err)
	}
	path := filepath.Join(runsDir, ".agent", "schedules", created.ID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected schedule persisted under configured runsDir at %s: %v", path, err)
	}

	items := manager.ListWithContext(daemonTenantTestContext())
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("ListWithContext() = %#v, want created schedule", items)
	}
}

func TestProviderStoreTenantContextDoesNotChangeStorePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	store := newProviderStore(path)
	cfg := web.ProviderConfig{
		ActiveID: "primary",
		Providers: []web.ProviderProfile{{
			ID:    "primary",
			Name:  "Primary",
			Type:  "openai",
			Model: "gpt-test",
		}},
	}
	if err := store.SaveContext(daemonTenantTestContext(), cfg); err != nil {
		t.Fatalf("SaveContext() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected provider config persisted at configured path %s: %v", path, err)
	}
	loaded, err := store.LoadContext(daemonTenantTestContext())
	if err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	if loaded.ActiveID != "primary" || len(loaded.Providers) != 1 || loaded.Providers[0].ID != "primary" {
		t.Fatalf("LoadContext() = %#v, want saved config", loaded)
	}
}

func TestAgentManagerTenantContextDoesNotChangeLocalSessionStore(t *testing.T) {
	runsDir := t.TempDir()
	manager := newAgentManager(runsDir)
	ctx := daemonTenantTestContext()

	created, err := manager.CreateSessionWithContext(ctx, createAgentSessionRequest{Operator: "tester", Language: "zh"})
	if err != nil {
		t.Fatalf("CreateSessionWithContext() error = %v", err)
	}
	sessionID := created.Session.SessionID
	path := filepath.Join(runsDir, ".agent", "sessions", sessionID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected agent session persisted under configured runsDir at %s: %v", path, err)
	}

	list, err := manager.ListSessions(ctx, 10)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if list.Count != 1 || list.Items[0].SessionID != sessionID {
		t.Fatalf("ListSessions() = %#v, want created session", list)
	}

	got, err := manager.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got.SessionID != sessionID {
		t.Fatalf("GetSession() ID = %q, want %q", got.SessionID, sessionID)
	}
}

func daemonTenantTestContext() context.Context {
	return coretenant.WithContext(context.Background(), coretenant.TenantContext{
		TenantID:    "tenant-daemon",
		WorkspaceID: "workspace-daemon",
		Mode:        coretenant.ModeMultiUser,
		Principal: coretenant.Principal{
			SubjectID:   "daemon-user",
			DisplayName: "Daemon User",
			AuthMethod:  "test",
			Roles:       []string{"operator"},
		},
	})
}
