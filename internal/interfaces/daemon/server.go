package daemon

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	agentruntime "github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
	runtimesqlite "github.com/H1d3rOne/cyberagent/internal/runtime/state/sqlite"
)

// Server owns the local daemon HTTP adapter.
type Server struct {
	cfg          Config
	token        string
	handler      http.Handler
	scheduler    *scheduleManager
	runtimeStore interface{ Close() error }
}

// NewServer creates a configured local daemon server and ensures a bearer token exists.
func NewServer(cfg Config) (*Server, error) {
	cfg = cfg.Normalize()
	token, err := loadOrCreateToken(cfg.TokenFile)
	if err != nil {
		return nil, err
	}
	version := services.NewVersionService(cfg.Build)
	runs := services.NewRunService(cfg.RunsDir)
	jobs := newJobManager(jobManagerConfig{
		RunsDir:           cfg.RunsDir,
		Build:             cfg.Build,
		MaxConcurrentJobs: cfg.MaxConcurrentJobs,
	})
	scanManager := web.NewScanManager(func(name string, cfg llm.ProviderConfig) llm.Provider {
		return buildAgentLLMProvider(name, cfg)
	})
	scheduler := newScheduleManager(cfg.RunsDir)
	providers := newProviderStore("")

	svc := routerServices{
		version:     version,
		plugins:     services.NewPluginService(),
		runs:        runs,
		approvals:   services.NewApprovalService(cfg.RunsDir),
		jobs:        jobs,
		scanManager: scanManager,
		schedules:   scheduler,
		providers:   providers,
	}
	if svc.agent == nil {
		svc.agent = newAgentManager(cfg.RunsDir)
	}
	svc.agent.AttachProviderStore(providers)
	var runtimeStore interface{ Close() error }
	if agentRuntimeEngineEnabledFromEnv(os.Getenv("CYBERAGENT_RUNTIME")) {
		store, err := runtimesqlite.Open(agentRuntimeSQLitePath(cfg.RunsDir))
		if err != nil {
			return nil, err
		}
		runtimeStore = store
		engine := agentruntime.NewEngine(store)
		svc.agent.AttachEngine(engine)
		if _, err := engine.RecoverInterruptedRuns(context.Background()); err != nil {
			_ = runtimeStore.Close()
			return nil, err
		}
	}
	scheduler.BindRunner(scheduleRunner{
		SendAgentPrompt: func(ctx context.Context, s scheduleSpec) (string, error) {
			sessionID := strings.TrimSpace(s.SessionID)
			if sessionID == "" {
				created, err := svc.agent.CreateSession(createAgentSessionRequest{
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

	return &Server{
		cfg:          cfg,
		token:        token,
		handler:      newRouter(svc, token),
		scheduler:    scheduler,
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
	if s == nil || s.runtimeStore == nil {
		return nil
	}
	err := s.runtimeStore.Close()
	s.runtimeStore = nil
	return err
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
		err := httpServer.ListenAndServe()
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
