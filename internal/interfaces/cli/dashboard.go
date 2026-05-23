package cli

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/core/logger"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
	"github.com/spf13/cobra"
)

func newDashboardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Start the web dashboard with full scan management UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, _ := cmd.Flags().GetString("addr")
			dev, _ := cmd.Flags().GetBool("dev")

			manager := web.NewScanManager(func(name string, cfg llm.ProviderConfig) llm.Provider {
				return buildProvider(name, cfg)
			})

			apiServer := web.NewAPIServer(manager)

			mux := http.NewServeMux()
			mux.Handle("/api/", apiServer.Handler())

			if dev {
				logger.Info("Dev mode: frontend served by Vite at http://localhost:5173")
				logger.Info("API server: http://%s", addr)
			} else {
				mux.Handle("/", web.FrontendHandler())
			}

			server := &http.Server{
				Addr:         addr,
				Handler:      mux,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 0,
			}

			fmt.Fprintf(cmd.OutOrStdout(), "CyberAgent Dashboard: http://%s\n", addr)
			fmt.Fprintf(cmd.OutOrStdout(), "Press Ctrl+C to stop.\n")

			if !dev {
				go func() {
					time.Sleep(500 * time.Millisecond)
					openBrowser(fmt.Sprintf("http://%s", addr))
				}()
			}

			return server.ListenAndServe()
		},
	}

	cmd.Flags().String("addr", "127.0.0.1:8080", "listen address")
	cmd.Flags().Bool("dev", false, "development mode (frontend served by Vite)")
	return cmd
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "linux":
		cmd = "xdg-open"
	default:
		return
	}
	exec.Command(cmd, url).Start()
}
