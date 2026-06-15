package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/H1d3rOne/cyberagent/internal/core/config"
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	daemonapi "github.com/H1d3rOne/cyberagent/internal/interfaces/daemon"
	"github.com/spf13/cobra"
)

func newDaemonCommand(build coreruntime.BuildInfo) *cobra.Command {
	var cfg daemonapi.Config
	cfg.Listen = daemonapi.DefaultListen
	cfg.RunsDir = daemonapi.DefaultRunsDir
	cfg.TokenFile = daemonapi.DefaultTokenFile()
	cfg.MaxConcurrentJobs = daemonapi.DefaultMaxConcurrentJobs
	cfg.Build = services.BuildInfo{
		Name:    "CyberAgent",
		Version: build.Version,
		Commit:  build.Commit,
		Date:    build.Date,
		Module:  build.Module,
	}

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the local CyberAgent HTTP API daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			configFile, _ := cmd.Flags().GetString("config")
			fileCfg, err := loadConfig(configFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			effective := applyDaemonConfigFile(cfg, fileCfg.Daemon, func(name string) bool {
				return cmd.Flags().Changed(name)
			})
			server, err := daemonapi.NewServer(effective)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "CyberAgent daemon listening on http://%s\n", server.Addr())
			fmt.Fprintf(out, "token file: %s\n", server.TokenFile())
			fmt.Fprintln(out, "api prefix: /api/v1")
			return server.ListenAndServe(ctx)
		},
	}
	cmd.Flags().StringVar(&cfg.Listen, "listen", cfg.Listen, "listen address")
	cmd.Flags().StringVar(&cfg.RunsDir, "runs-dir", cfg.RunsDir, "run workspace root")
	cmd.Flags().StringVar(&cfg.TokenFile, "token-file", cfg.TokenFile, "bearer token file")
	cmd.Flags().IntVar(&cfg.MaxConcurrentJobs, "max-concurrent-jobs", cfg.MaxConcurrentJobs, "maximum concurrent daemon-launched jobs")
	cmd.Flags().BoolVar(&cfg.MultiUser.Enabled, "multi-user", cfg.MultiUser.Enabled, "enable explicit multi-user storage mode")
	cmd.Flags().StringVar(&cfg.MultiUser.StorageRoot, "multi-user-storage-root", cfg.MultiUser.StorageRoot, "multi-user tenant/workspace storage root")
	cmd.Flags().String("config", "", "path to config file with daemon defaults (default: .cyberagent.yaml and user config)")
	return cmd
}

func applyDaemonConfigFile(base daemonapi.Config, cfg config.DaemonConfig, flagChanged func(string) bool) daemonapi.Config {
	changed := func(name string) bool {
		return flagChanged != nil && flagChanged(name)
	}
	if cfg.Listen != "" && !changed("listen") {
		base.Listen = cfg.Listen
	}
	if cfg.RunsDir != "" && !changed("runs-dir") {
		base.RunsDir = cfg.RunsDir
	}
	if cfg.TokenFile != "" && !changed("token-file") {
		base.TokenFile = cfg.TokenFile
	}
	if cfg.MaxConcurrentJobs > 0 && !changed("max-concurrent-jobs") {
		base.MaxConcurrentJobs = cfg.MaxConcurrentJobs
	}
	if cfg.MultiUser.Enabled != nil && !changed("multi-user") {
		base.MultiUser.Enabled = *cfg.MultiUser.Enabled
	}
	if cfg.MultiUser.StorageRoot != "" && !changed("multi-user-storage-root") {
		base.MultiUser.StorageRoot = cfg.MultiUser.StorageRoot
	}
	if cfg.Auth.WebSessions.Enabled != nil {
		base.Auth.WebSessions.Enabled = *cfg.Auth.WebSessions.Enabled
	}
	if cfg.Auth.WebSessions.CookieName != "" {
		base.Auth.WebSessions.CookieName = cfg.Auth.WebSessions.CookieName
	}
	if cfg.Auth.WebSessions.TTLSeconds > 0 {
		base.Auth.WebSessions.TTLSeconds = cfg.Auth.WebSessions.TTLSeconds
	}
	if cfg.Auth.WebSessions.StoragePath != "" {
		base.Auth.WebSessions.StoragePath = cfg.Auth.WebSessions.StoragePath
	}
	if cfg.Auth.WebSessions.PrincipalResolverEnabled != nil {
		base.Auth.WebSessions.PrincipalResolverEnabled = *cfg.Auth.WebSessions.PrincipalResolverEnabled
	}
	if cfg.Auth.RBAC.Enabled != nil {
		base.Auth.RBAC.Enabled = *cfg.Auth.RBAC.Enabled
	}
	if cfg.Auth.RBAC.PolicyPath != "" {
		base.Auth.RBAC.PolicyPath = cfg.Auth.RBAC.PolicyPath
	}
	if cfg.Auth.RBAC.AuditEnabled != nil {
		base.Auth.RBAC.AuditEnabled = *cfg.Auth.RBAC.AuditEnabled
	}
	if cfg.Auth.RBAC.EnforcementEnabled != nil {
		base.Auth.RBAC.EnforcementEnabled = *cfg.Auth.RBAC.EnforcementEnabled
	}
	if cfg.Audit.Enabled != nil {
		base.Audit.Enabled = *cfg.Audit.Enabled
	}
	if cfg.Audit.Path != "" {
		base.Audit.Path = cfg.Audit.Path
	}
	if cfg.Admin.BootstrapTokenFile != "" {
		base.Admin.BootstrapTokenFile = cfg.Admin.BootstrapTokenFile
	}
	if cfg.Admin.MembershipAPIEnabled != nil {
		base.Admin.MembershipAPIEnabled = *cfg.Admin.MembershipAPIEnabled
	}
	if cfg.Admin.MembershipWriteEnabled != nil {
		base.Admin.MembershipWriteEnabled = *cfg.Admin.MembershipWriteEnabled
	}
	if cfg.RateLimit.Enabled != nil {
		base.RateLimit.Enabled = *cfg.RateLimit.Enabled
	}
	if cfg.RateLimit.RequestsPerMinute > 0 {
		base.RateLimit.RequestsPerMinute = cfg.RateLimit.RequestsPerMinute
	}
	if cfg.RateLimit.Burst > 0 {
		base.RateLimit.Burst = cfg.RateLimit.Burst
	}
	if cfg.Hardening.RequireLoopback != nil {
		base.Hardening.RequireLoopback = *cfg.Hardening.RequireLoopback
	}
	return base
}
