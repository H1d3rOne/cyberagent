package daemon

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
	runtimesqlite "github.com/H1d3rOne/cyberagent/internal/runtime/state/sqlite"
)

// Server owns the local daemon HTTP adapter.
type Server struct {
	cfg          Config
	token        string
	handler      http.Handler
	scheduler    *scheduleManager
	auditSink    daemonAuditSink
	runtimeStore interface{ Close() error }
}

// NewServer creates a configured local daemon server and ensures a bearer token exists.
func NewServer(cfg Config) (*Server, error) {
	cfg = cfg.Normalize()
	storagePaths, storageResolver, err := daemonLocalStoragePathsFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	auditSink, err := newDaemonAuditSink(cfg.Audit)
	if err != nil {
		return nil, err
	}
	token, err := loadOrCreateToken(cfg.TokenFile)
	if err != nil {
		_ = auditSink.Close()
		return nil, err
	}
	adminBootstrap := loadAdminBootstrapState(cfg.Admin)
	version := services.NewVersionService(cfg.Build)
	runs := services.NewRunService(storagePaths.RunsDir)
	jobs := newJobManager(jobManagerConfig{
		RunsDir:           storagePaths.RunsDir,
		Build:             cfg.Build,
		MaxConcurrentJobs: cfg.MaxConcurrentJobs,
		StorageResolver:   storageResolver,
	})
	scanManager := web.NewScanManager(func(name string, cfg llm.ProviderConfig) llm.Provider {
		return buildAgentLLMProvider(name, cfg)
	})
	scheduler := newScheduleManager(storagePaths.RunsDir)
	providers := newProviderStoreWithStorageResolver(storagePaths.ProviderStorePath, storageResolver)
	var webSessionStore coretenant.SessionStore = coretenant.DisabledSessionStore{}
	if cfg.Auth.WebSessions.Enabled {
		webSessionStore = coretenant.NewMemorySessionStore()
	}
	var membershipStore coretenant.MembershipStore = coretenant.DisabledMembershipStore{}
	if cfg.Auth.RBAC.Enabled || cfg.Admin.MembershipAPIEnabled {
		membershipStore = coretenant.NewMemoryMembershipStore()
	}
	rbacPolicy, err := newDaemonRBACPolicyFromConfig(cfg.Auth.RBAC)
	if err != nil {
		_ = auditSink.Close()
		return nil, err
	}

	svc := routerServices{
		version:         version,
		plugins:         services.NewPluginService(),
		runs:            runs,
		approvals:       services.NewApprovalService(storagePaths.RunsDir),
		jobs:            jobs,
		scanManager:     scanManager,
		schedules:       scheduler,
		providers:       providers,
		admin:           adminBootstrap,
		adminConfig:     cfg.Admin,
		auth:            cfg.Auth,
		sessionStore:    webSessionStore,
		membershipStore: membershipStore,
		rbacPolicy:      rbacPolicy,
		auditSink:       auditSink,
		rateLimit:       cfg.RateLimit,
		hardening:       cfg.Hardening,
	}
	if svc.agent == nil {
		svc.agent = newAgentManager(storagePaths.RunsDir)
	}
	svc.agent.AttachProviderStore(providers)
	var runtimeStore interface{ Close() error }
	if agentRuntimeEngineEnabledFromEnv(os.Getenv("CYBERAGENT_RUNTIME")) {
		store, err := runtimesqlite.Open(storagePaths.RuntimeDBPath)
		if err != nil {
			_ = auditSink.Close()
			return nil, err
		}
		runtimeStore = store
		engine := agentruntime.NewEngine(store)
		svc.agent.AttachEngine(engine)
		if _, err := engine.RecoverInterruptedRuns(context.Background()); err != nil {
			_ = runtimeStore.Close()
			_ = auditSink.Close()
			return nil, err
		}
	}
	scheduler.BindRunner(scheduleRunner{
		SendAgentPrompt: func(ctx context.Context, s scheduleSpec) (string, error) {
			sessionID := strings.TrimSpace(s.SessionID)
			if sessionID == "" {
				created, err := svc.agent.CreateSessionWithContext(ctx, createAgentSessionRequest{
					Operator: s.Operator,
					Language: s.Language,
				})
				if err != nil {
					return "", err
				}
				sessionID = created.Session.SessionID
			}
			req := sendAgentMessageRequest{
				Message:        s.Prompt,
				Operator:       s.Operator,
				Language:       s.Language,
				OperationLevel: s.OpsLevel,
				Origin:         "schedule",
			}
			resp, err := svc.agent.SendMessage(ctx, sessionID, req, svc)
			if err != nil {
				return "", err
			}
			if len(resp.Messages) == 0 {
				return "", nil
			}
			return resp.Messages[len(resp.Messages)-1].ID, nil
		},
	})
	if err := auditSink.Record(context.Background(), daemonAuditEvent{
		Type:    "daemon.startup",
		Outcome: "ready",
		Details: map[string]any{
			"listen":                     cfg.Listen,
			"multi_user_enabled":         cfg.MultiUser.Enabled,
			"auth_web_sessions_enabled":  cfg.Auth.WebSessions.Enabled,
			"auth_rbac_enabled":          cfg.Auth.RBAC.Enabled,
			"auth_rbac_audit_enabled":    cfg.Auth.RBAC.AuditEnabled,
			"auth_rbac_enforcement":      cfg.Auth.RBAC.EnforcementEnabled,
			"rate_limit_enabled":         cfg.RateLimit.Enabled,
			"hardening_require_loopback": cfg.Hardening.RequireLoopback,
			"admin_bootstrap_configured": adminBootstrap.Configured,
			"admin_bootstrap_ready":      adminBootstrap.Ready,
			"admin_membership_api":       cfg.Admin.MembershipAPIEnabled,
		},
	}); err != nil {
		if runtimeStore != nil {
			_ = runtimeStore.Close()
		}
		_ = auditSink.Close()
		return nil, err
	}

	return &Server{
		cfg:          cfg,
		token:        token,
		handler:      newRouter(svc, token),
		scheduler:    scheduler,
		auditSink:    auditSink,
		runtimeStore: runtimeStore,
	}, nil
}

func agentRuntimeSQLitePath(runsDir string) string {
	base := strings.TrimSpace(runsDir)
	if base == "" {
		base = DefaultRunsDir
	}
	return filepath.Join(base, ".agent", "runtime", "runtime.db")
}

// agentRuntimeEngineEnabledFromEnv controls the compatibility switch for the
// production Agent runtime. Engine is now the default path; set
// CYBERAGENT_RUNTIME=legacy/off/disabled/false/0 to opt out for rollback.
func agentRuntimeEngineEnabledFromEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "legacy", "off", "disabled", "disable", "false", "0", "none":
		return false
	default:
		return true
	}
}

// Close releases resources owned by the server. ListenAndServe calls this for
// long-running daemon processes; tests that only use Handler may call it
// explicitly because the SQLite runtime store is opened by default.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var closeErr error
	if s.runtimeStore != nil {
		closeErr = errors.Join(closeErr, s.runtimeStore.Close())
		s.runtimeStore = nil
	}
	if s.auditSink != nil {
		closeErr = errors.Join(closeErr, s.auditSink.Close())
		s.auditSink = nil
	}
	return closeErr
}

// Handler returns the daemon HTTP handler for tests and embedding.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// Addr returns the normalized listen address.
func (s *Server) Addr() string {
	return s.cfg.Listen
}

// TokenFile returns the token file path used by this daemon.
func (s *Server) TokenFile() string {
	return s.cfg.TokenFile
}

// ListenAndServe runs the daemon until the context is cancelled or the server fails.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		_ = s.Close()
		return err
	}
	return s.Serve(ctx, ln)
}

// Serve runs the daemon on a caller-owned listener until the context is
// cancelled or the server fails. It is intended for embedded hosts (dashboard,
// desktop/Wails shells, tests) that need to bind an ephemeral loopback port and
// surface the concrete URL to a WebView without changing daemon API behavior.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	defer s.Close()
	if ctx == nil {
		ctx = context.Background()
	}
	if s.scheduler != nil {
		s.scheduler.Start()
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.scheduler.Stop(stopCtx)
		}()
	}
	httpServer := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		err := httpServer.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}
