package daemon

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

// Server owns the local daemon HTTP adapter.
type Server struct {
	cfg     Config
	token   string
	handler http.Handler
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
	return &Server{
		cfg:   cfg,
		token: token,
		handler: newRouter(routerServices{
			version:   version,
			plugins:   services.NewPluginService(),
			runs:      runs,
			approvals: services.NewApprovalService(cfg.RunsDir),
			jobs:      jobs,
		}, token),
	}, nil
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
	if ctx == nil {
		ctx = context.Background()
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
