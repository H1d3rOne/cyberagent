package desktop

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/daemon"
)

const (
	// DefaultListenHost keeps desktop-embedded daemon traffic on loopback only.
	DefaultListenHost = "127.0.0.1"

	// AuthModeBearerTokenFile means the WebView receives a bearer token that is
	// persisted in the daemon token file. The safe runtime contract records the
	// token file path but never serializes the token itself.
	AuthModeBearerTokenFile = "bearer_token_file"
)

// BootstrapConfig configures the dependency-free Desktop/Wails bootstrap seam.
// It intentionally avoids importing Wails so the same seam can be tested from
// the Go package, a Wails shell, or another local desktop host.
type BootstrapConfig struct {
	RunsDir           string
	TokenFile         string
	LogDir            string
	ListenHost        string
	MaxConcurrentJobs int
	Build             services.BuildInfo
}

// RuntimeContract is the safe-to-log desktop runtime contract. It tells a
// desktop shell where the local API lives and which local resources are in
// scope, but it does not contain the bearer token.
type RuntimeContract struct {
	APIBaseURL  string          `json:"api_base_url"`
	AuthMode    string          `json:"auth_mode"`
	TokenFile   string          `json:"token_file"`
	RunsDir     string          `json:"runs_dir"`
	LogDir      string          `json:"log_dir"`
	Permissions PermissionModel `json:"permissions"`
}

// WebViewBootstrap is the intentionally-sensitive bootstrap payload a desktop
// host injects into its local WebView. Do not log this value because it contains
// the bearer token.
type WebViewBootstrap struct {
	APIBaseURL         string `json:"api_base_url"`
	AuthorizationType  string `json:"authorization_type"`
	AuthorizationToken string `json:"authorization_token"`
}

// PermissionModel documents the desktop shell boundary that wraps the embedded
// daemon. The values are policy labels so docs/tests can pin the intended model
// before a Wails dependency is introduced.
type PermissionModel struct {
	NetworkBinding     string `json:"network_binding"`
	TokenExposure      string `json:"token_exposure"`
	FilesystemScope    string `json:"filesystem_scope"`
	ShellExecution     string `json:"shell_execution"`
	Updates            string `json:"updates"`
	ExternalNavigation string `json:"external_navigation"`
}

// EmbeddedDaemon is a running local daemon plus the desktop bootstrap contract.
type EmbeddedDaemon struct {
	contract RuntimeContract
	webView  WebViewBootstrap

	cancel   context.CancelFunc
	errCh    chan error
	stopOnce sync.Once
	stopDone chan struct{}
	stopErr  error
}

// Normalize fills desktop-safe defaults and rejects non-loopback daemon binds.
func (c BootstrapConfig) Normalize() (BootstrapConfig, error) {
	dcfg := daemon.Config{
		RunsDir:           c.RunsDir,
		TokenFile:         c.TokenFile,
		MaxConcurrentJobs: c.MaxConcurrentJobs,
		Build:             c.Build,
	}.Normalize()
	host, err := normalizeLoopbackHost(c.ListenHost)
	if err != nil {
		return BootstrapConfig{}, err
	}
	logDir := strings.TrimSpace(c.LogDir)
	if logDir == "" {
		logDir = DefaultLogDir()
	}
	return BootstrapConfig{
		RunsDir:           dcfg.RunsDir,
		TokenFile:         dcfg.TokenFile,
		LogDir:            logDir,
		ListenHost:        host,
		MaxConcurrentJobs: dcfg.MaxConcurrentJobs,
		Build:             dcfg.Build,
	}, nil
}

// DefaultLogDir returns the desktop log directory used by the bootstrap
// contract. The current spike records this path for shell integration; the
// daemon access-log implementation remains part of the Local API/Daemon line.
func DefaultLogDir() string {
	if dir, err := os.UserCacheDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "cyberagent", "logs")
	}
	return filepath.Join(".cyberagent", "logs")
}

// StartEmbeddedDaemon starts the daemon on 127.0.0.1:0 and returns the runtime
// contract a Wails shell can inject into its WebView.
func StartEmbeddedDaemon(parent context.Context, cfg BootstrapConfig) (*EmbeddedDaemon, error) {
	if parent == nil {
		parent = context.Background()
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	normalized, err := cfg.Normalize()
	if err != nil {
		return nil, err
	}
	listenAddr := net.JoinHostPort(normalized.ListenHost, "0")
	server, err := daemon.NewServer(daemon.Config{
		Listen:            listenAddr,
		RunsDir:           normalized.RunsDir,
		TokenFile:         normalized.TokenFile,
		MaxConcurrentJobs: normalized.MaxConcurrentJobs,
		Build:             normalized.Build,
	})
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		_ = server.Close()
		return nil, err
	}
	tokenData, err := os.ReadFile(server.TokenFile())
	if err != nil {
		_ = ln.Close()
		_ = server.Close()
		return nil, fmt.Errorf("read desktop daemon token file: %w", err)
	}
	baseURL := listenerBaseURL(ln)
	ctx, cancel := context.WithCancel(parent)
	runtime := &EmbeddedDaemon{
		contract: RuntimeContract{
			APIBaseURL:  baseURL,
			AuthMode:    AuthModeBearerTokenFile,
			TokenFile:   server.TokenFile(),
			RunsDir:     normalized.RunsDir,
			LogDir:      normalized.LogDir,
			Permissions: DefaultPermissionModel(),
		},
		webView: WebViewBootstrap{
			APIBaseURL:         baseURL,
			AuthorizationType:  "Bearer",
			AuthorizationToken: strings.TrimSpace(string(tokenData)),
		},
		cancel:   cancel,
		errCh:    make(chan error, 1),
		stopDone: make(chan struct{}),
	}
	go func() {
		runtime.errCh <- server.Serve(ctx, ln)
	}()
	return runtime, nil
}

// Contract returns the safe-to-log runtime contract.
func (e *EmbeddedDaemon) Contract() RuntimeContract {
	if e == nil {
		return RuntimeContract{}
	}
	return e.contract
}

// WebViewBootstrap returns the sensitive WebView injection payload.
func (e *EmbeddedDaemon) WebViewBootstrap() WebViewBootstrap {
	if e == nil {
		return WebViewBootstrap{}
	}
	return e.webView
}

// Stop cancels the embedded daemon and waits for shutdown.
func (e *EmbeddedDaemon) Stop(ctx context.Context) error {
	if e == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	e.stopOnce.Do(func() {
		e.cancel()
		go func() {
			e.stopErr = <-e.errCh
			close(e.stopDone)
		}()
	})
	select {
	case <-e.stopDone:
		return e.stopErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DefaultPermissionModel pins the intended Desktop/Wails boundary for the
// spike: local-only networking, explicit token injection, local workspace/data
// directories, no silent shell escalation, and updater actions initiated by the
// desktop shell rather than by the daemon HTTP API.
func DefaultPermissionModel() PermissionModel {
	return PermissionModel{
		NetworkBinding:     "loopback_ephemeral_only",
		TokenExposure:      "inject_to_local_webview_only_never_log",
		FilesystemScope:    "configured_runs_token_log_dirs_only",
		ShellExecution:     "reuse_daemon_policy_gates_no_desktop_bypass",
		Updates:            "desktop_shell_owned_explicit_user_action",
		ExternalNavigation: "open_external_browser_with_confirmed_url",
	}
}

func normalizeLoopbackHost(raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return DefaultListenHost, nil
	}
	if strings.EqualFold(host, "localhost") {
		return DefaultListenHost, nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("desktop daemon listen host must be loopback, got %q", raw)
	}
	return host, nil
}

func listenerBaseURL(ln net.Listener) string {
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return "http://" + ln.Addr().String()
	}
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, port)}).String()
}
