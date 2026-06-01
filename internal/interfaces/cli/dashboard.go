package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/core/logger"
	daemonapi "github.com/H1d3rOne/cyberagent/internal/interfaces/daemon"
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
			daemonURL, _ := cmd.Flags().GetString("daemon-url")
			daemonTokenFile, _ := cmd.Flags().GetString("daemon-token-file")

			if !cmd.Flags().Changed("daemon-url") {
				daemonURL = "http://" + dashboardLoopbackAddr(addr)
			}

			manager := web.NewScanManager(func(name string, cfg llm.ProviderConfig) llm.Provider {
				return buildProvider(name, cfg)
			})

			apiServer := web.NewAPIServer(manager, daemonapi.NewDefaultProviderStore(""))

			daemonSrv, err := daemonapi.NewServer(daemonapi.Config{
				Listen:    addr,
				TokenFile: daemonTokenFile,
			})
			if err != nil {
				return fmt.Errorf("init embedded daemon: %w", err)
			}

			mux := http.NewServeMux()
			mux.HandleFunc("/api/local/daemon-config", handleLocalDaemonBootstrap(daemonURL, daemonTokenFile))
			mux.Handle("/api/v1/", daemonSrv.Handler())
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
	cmd.Flags().String("daemon-url", "http://"+daemonapi.DefaultListen, "default local daemon URL exposed to the dashboard bootstrap endpoint (defaults to the dashboard's own listen address)")
	cmd.Flags().String("daemon-token-file", daemonapi.DefaultTokenFile(), "local daemon bearer token file exposed only to loopback dashboard clients")
	cmd.Flags().Bool("dev", false, "development mode (frontend served by Vite)")
	return cmd
}

func handleLocalDaemonBootstrap(daemonURL, tokenFile string) http.HandlerFunc {
	type response struct {
		Available bool   `json:"available"`
		BaseURL   string `json:"base_url"`
		Token     string `json:"token,omitempty"`
		TokenFile string `json:"token_file,omitempty"`
		Message   string `json:"message,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !isLoopbackDashboardRequest(r) {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(response{
				Available: false,
				BaseURL:   strings.TrimSpace(daemonURL),
				Message:   "local daemon bootstrap is only available to loopback clients",
			})
			return
		}

		baseURL := strings.TrimSpace(daemonURL)
		tokenFile = strings.TrimSpace(tokenFile)
		data, err := osReadFileTrimmed(tokenFile)
		if err != nil {
			_ = json.NewEncoder(w).Encode(response{
				Available: false,
				BaseURL:   baseURL,
				TokenFile: tokenFile,
				Message:   err.Error(),
			})
			return
		}
		_ = json.NewEncoder(w).Encode(response{
			Available: true,
			BaseURL:   baseURL,
			Token:     data,
			TokenFile: tokenFile,
		})
	}
}

func isLoopbackDashboardRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}

func osReadFileTrimmed(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("token file is not configured")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file is empty")
	}
	return token, nil
}

// dashboardLoopbackAddr returns a loopback host:port suitable for the bootstrap
// base URL. If the listen address binds to a wildcard host (empty or 0.0.0.0),
// it is rewritten to 127.0.0.1 so that browsers on the same machine can reach
// the dashboard.
func dashboardLoopbackAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
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
