package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
			server, err := daemonapi.NewServer(cfg)
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
	return cmd
}
