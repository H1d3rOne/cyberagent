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

	svc := routerServices{
		version:     version,
		plugins:     services.NewPluginService(),
		runs:        runs,
		approvals:   services.NewApprovalService(cfg.RunsDir),
		jobs:        jobs,
		scanManager: scanManager,
		schedules:   scheduler,
	}
	if svc.agent == nil {
		svc.agent = newAgentManager(cfg.RunsDir)
	}
	var engine *agentruntime.Engine
	var runtimeStore interface{ Close() error }
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CYBERAGENT_RUNTIME")), "engine") {
		store, err := runtimesqlite.Open(agentRuntimeSQLitePath(cfg.RunsDir))
		if err != nil {
			return nil, err
		}
		runtimeStore = store
		engine = agentruntime.NewEngine(store)
		svc.agent.AttachEngine(engine)
	}
	scheduler.BindRunner(scheduleRunner{
		SendAgentPrompt: func(ctx context.Context, s scheduleSpec) (string, error) {
			req := sendAgentMessageRequest{
				Message:  s.Prompt,
				Operator: s.Operator,
				Language: s.Language,
				Origin:   "schedule",
			}
			resp, err := svc.agent.SendMessage(ctx, s.SessionID, req, svc)
			if err != nil {
				return "", err
			}
			if len(resp.Messages) == 0 {
				return "", nil
			}
			return resp.Messages[len(resp.Messages)-1].ID, nil
		},
		StartScanWeb: func(ctx context.Context, s scheduleSpec) (string, error) {
			profile := strings.TrimSpace(s.Profile)
			taskID := "task_" + strings.TrimSpace(s.ID)
			runID := "run_" + strings.TrimSpace(s.ID)
			if engine != nil {
				_, _ = engine.Submit(ctx, agentruntime.Task{
					ID:         taskID,
					SessionID:  s.SessionID,
					Origin:     "schedule",
					Goal:       firstNonEmpty(s.Prompt, s.Target),
					Prompt:     firstNonEmpty(s.Prompt, "scan.web.safe "+s.Target),
					ScheduleID: s.ID,
					Created:    time.Now().UTC(),
				})
				_, _ = engine.SubmitRun(ctx, agentruntime.Run{ID: runID, TaskID: taskID, State: string(agentruntime.RunRunning), Turn: 1, CreatedAt: time.Now().UTC()})
				engine.Publish(ctx, agentruntime.Event{Type: "tool_started", TaskID: taskID, RunID: runID, Data: map[string]any{"session_id": s.SessionID, "schedule_id": s.ID, "tool": "scan.web.safe", "target": s.Target}, Timestamp: time.Now().UTC()})
			}
			req := startWebScanJobRequest{
				Target:  s.Target,
				Scope:   s.Scope,
				Profile: profile,
			}
			resp, err := svc.jobs.StartWebScan(ctx, req)
			if engine != nil {
				data := map[string]any{"session_id": s.SessionID, "schedule_id": s.ID, "tool": "scan.web.safe", "target": s.Target}
				if err != nil {
					data["error"] = err.Error()
					engine.Publish(ctx, agentruntime.Event{Type: "tool_finished", TaskID: taskID, RunID: runID, Data: data, Timestamp: time.Now().UTC()})
					_ = engine.FinishRun(ctx, runID, false)
				} else {
					data["job_id"] = resp.JobID
					engine.Publish(ctx, agentruntime.Event{Type: "tool_finished", TaskID: taskID, RunID: runID, Data: data, Timestamp: time.Now().UTC()})
					_ = engine.FinishRun(ctx, runID, true)
				}
			}
			if err != nil {
				return "", err
			}
			return resp.JobID, nil
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

// Close releases resources owned by the server. ListenAndServe calls this for
// long-running daemon processes; tests that only use Handler may call it
// explicitly when CYBERAGENT_RUNTIME=engine opens the SQLite runtime store.
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
