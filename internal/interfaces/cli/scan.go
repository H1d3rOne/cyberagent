package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent"
	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/core/config"
	"github.com/H1d3rOne/cyberagent/internal/core/logger"
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
	"github.com/spf13/cobra"
)

func newScanCommand(build coreruntime.BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Create and run scan jobs",
	}

	web := &cobra.Command{
		Use:   "web",
		Short: "Run an authorized Web/API safe-profile scan",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, _ := cmd.Flags().GetString("target")
			scope, _ := cmd.Flags().GetString("scope")
			profile, _ := cmd.Flags().GetString("profile")
			out, _ := cmd.Flags().GetString("out")
			plugins, _ := cmd.Flags().GetStringSlice("use-plugin")
			noExternalTools, _ := cmd.Flags().GetBool("no-external-tools")
			interactive, _ := cmd.Flags().GetBool("interactive")
			configFile, _ := cmd.Flags().GetString("config")

			cfg, err := loadConfig(configFile)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			if !cmd.Flags().Changed("profile") && cfg.Defaults.Profile != "" {
				profile = cfg.Defaults.Profile
			}
			if scope == "" && cfg.Defaults.Scope != "" {
				scope = cfg.Defaults.Scope
			}
			if out == "" && cfg.Defaults.OutDir != "" {
				out = cfg.Defaults.OutDir
			}
			if len(plugins) == 0 && len(cfg.Plugins.Enabled) > 0 {
				plugins = cfg.Plugins.Enabled
			}
			profileCfg, _ := cfg.GetProfile(profile)
			if len(profileCfg.EnabledPlugins) > 0 && len(plugins) == 0 {
				plugins = profileCfg.EnabledPlugins
			}

			logger.Debug("scan web: target=%s scope=%s profile=%s", target, scope, profile)
			logger.Debug("scan web: plugins=%v no-external-tools=%v", plugins, noExternalTools)

			result, err := coreruntime.RunWebScan(cmd.Context(), coreruntime.WebScanRequest{
				Target:          target,
				Scope:           scope,
				Profile:         profile,
				Out:             out,
				EnabledPlugins:  plugins,
				NoExternalTools: noExternalTools,
				Interactive:     interactive,
				BuildInfo:       build,
				CommandName:     "scan web",
				CommandArgs:     scanWebEffectiveArgs(target, scope, profile, out, plugins, noExternalTools, interactive, configFile),
				OnProgress: func(stepName, status string) {
					logger.Info("  [%s] %s", status, stepName)
				},
			})
			if err != nil {
				return err
			}

			outWriter := cmd.OutOrStdout()
			fmt.Fprintf(outWriter, "CyberAgent scan completed\n")
			fmt.Fprintf(outWriter, "job: %s\n", result.Job.ID)
			fmt.Fprintf(outWriter, "workspace: %s\n", result.Workspace)
			fmt.Fprintf(outWriter, "findings: %d\n", len(result.Findings))
			fmt.Fprintf(outWriter, "report: %s\n", result.ReportMD)
			if written, err := writeSkippedStepSummary(outWriter, result.Workspace, result.Job.ID); err != nil {
				return fmt.Errorf("write skipped summary: %w", err)
			} else if written {
				fmt.Fprintln(outWriter)
			}
			logger.Debug("scan web: evidence=%d artifacts=%d", len(result.Evidence), len(result.Artifacts))
			return nil
		},
	}
	web.Flags().String("target", "", "target Web URL")
	web.Flags().String("scope", "", "authorized scope, comma-separated")
	web.Flags().String("profile", "safe", "scan profile")
	web.Flags().String("out", "", "run output directory")
	web.Flags().String("config", "", "path to config file (default: .cyberagent.yaml)")
	web.Flags().StringSlice("use-plugin", nil, "explicitly enable optional plugin")
	web.Flags().Bool("no-external-tools", false, "disable external tool plugins")
	web.Flags().Bool("interactive", true, "allow interactive prompts for future HITL steps")

	cmd.AddCommand(web)
	cmd.AddCommand(newScanAgentCommand())
	return cmd
}

func scanWebEffectiveArgs(target, scope, profile, out string, plugins []string, noExternalTools, interactive bool, configFile string) []string {
	args := []string{"scan", "web"}
	if target != "" {
		args = append(args, "--target", target)
	}
	if scope != "" {
		args = append(args, "--scope", scope)
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if out != "" {
		args = append(args, "--out", out)
	}
	if configFile != "" {
		args = append(args, "--config", configFile)
	}
	for _, plugin := range plugins {
		if strings.TrimSpace(plugin) != "" {
			args = append(args, "--use-plugin", plugin)
		}
	}
	if noExternalTools {
		args = append(args, "--no-external-tools")
	}
	if !interactive {
		args = append(args, "--interactive=false")
	}
	return args
}

func loadConfig(path string) (config.Config, error) {
	if path != "" {
		return config.LoadFrom(path)
	}
	return config.Load()
}

func newScanAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run an LLM-driven autonomous security assessment",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, _ := cmd.Flags().GetString("target")
			scope, _ := cmd.Flags().GetString("scope")
			providerName, _ := cmd.Flags().GetString("provider")
			model, _ := cmd.Flags().GetString("model")
			apiKey, _ := cmd.Flags().GetString("api-key")
			baseURL, _ := cmd.Flags().GetString("base-url")
			instructions, _ := cmd.Flags().GetString("instructions")
			maxTurns, _ := cmd.Flags().GetInt("max-turns")
			configFile, _ := cmd.Flags().GetString("config")
			noConfirm, _ := cmd.Flags().GetBool("no-confirm")
			resume, _ := cmd.Flags().GetString("resume")
			interactive, _ := cmd.Flags().GetBool("interactive")
			multiAgent, _ := cmd.Flags().GetBool("multi-agent")
			dashboardAddr, _ := cmd.Flags().GetString("dashboard")

			cfg, _ := loadConfig(configFile)

			if !cmd.Flags().Changed("provider") && cfg.Agent.Provider != "" {
				providerName = cfg.Agent.Provider
			}
			if !cmd.Flags().Changed("model") && cfg.Agent.Model != "" {
				model = cfg.Agent.Model
			}
			if !cmd.Flags().Changed("base-url") && cfg.Agent.BaseURL != "" {
				baseURL = cfg.Agent.BaseURL
			}
			if !cmd.Flags().Changed("max-turns") && cfg.Agent.MaxTurns > 0 {
				maxTurns = cfg.Agent.MaxTurns
			}
			if instructions == "" && cfg.Agent.Instructions != "" {
				instructions = cfg.Agent.Instructions
			}
			if scope == "" && cfg.Defaults.Scope != "" {
				scope = cfg.Defaults.Scope
			}

			if target == "" {
				return fmt.Errorf("--target is required")
			}

			scopeList := []string{target}
			if scope != "" {
				scopeList = strings.Split(scope, ",")
			}

			if apiKey == "" {
				apiKey = resolveAPIKey(providerName)
			}

			provider := buildProvider(providerName, llm.ProviderConfig{
				APIKey:  apiKey,
				BaseURL: baseURL,
				Model:   model,
			})

			outWriter := cmd.OutOrStdout()

			out, _ := cmd.Flags().GetString("out")
			if out == "" {
				out = "."
			}
			eventLogger, _ := agent.NewEventLogger(out, "agent_"+fmt.Sprintf("%d", time.Now().Unix()))

			var dashboard *web.Server
			if dashboardAddr != "" {
				dashboard = web.NewServer(target, maxTurns)
				go func() {
					logger.Info("Dashboard: http://%s", dashboardAddr)
					http.ListenAndServe(dashboardAddr, dashboard.Handler())
				}()
			}

			a := agent.New(agent.Config{
				Provider:     provider,
				Target:       target,
				Scope:        scopeList,
				Instructions: instructions,
				MaxTurns:     maxTurns,
				NoConfirm:    noConfirm,
				OutDir:       out,
				OnConfirm: func(toolName string, input json.RawMessage) bool {
					fmt.Fprintf(outWriter, "\n[CONFIRM] High-risk tool %q requested. Input: %s\n", toolName, string(input))
					fmt.Fprintf(outWriter, "Allow? [y/N]: ")
					var answer string
					fmt.Fscanln(cmd.InOrStdin(), &answer)
					return answer == "y" || answer == "Y" || answer == "yes"
				},
				OnEvent: func(e agent.Event) {
					if eventLogger != nil {
						eventLogger.Log(e)
					}
					if dashboard != nil {
						dashboard.OnEvent(e)
					}
					switch e.Type {
					case agent.EventProgress:
						logger.Debug("  [turn %d/%d] elapsed=%s", e.Turn, e.MaxTurns, e.Elapsed.Round(time.Second))
					case agent.EventTextDelta:
						fmt.Fprint(outWriter, e.Message)
					case agent.EventToolCall:
						fmt.Fprintf(outWriter, "\n")
						logger.Info("  [tool] %s", e.ToolName)
					case agent.EventFinding:
						logger.Info("  [finding] %s (%s)", e.Message, e.Finding.Severity)
					case agent.EventError:
						logger.Info("  [error] %s: %s", e.ToolName, e.Message)
					case agent.EventDone:
						fmt.Fprintf(outWriter, "\n")
					}
				},
			})

			if len(cfg.Agent.CustomTools) > 0 {
				a.Registry().RegisterCustomTools(cfg.Agent.CustomTools)
			}

			logger.Info("Starting agent scan: target=%s provider=%s", target, providerName)

			ctx, cancel := context.WithCancel(cmd.Context())
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				logger.Info("Interrupt received, saving session...")
				a.SaveSession(out)
				cancel()
			}()
			defer signal.Stop(sigCh)

			if resume != "" {
				if err := a.LoadSession(resume); err != nil {
					return fmt.Errorf("resume session: %w", err)
				}
				logger.Info("Resumed session from %s", resume)
			}

			var result *agent.Result
			if multiAgent {
				fmt.Fprintf(outWriter, "Multi-agent mode: recon → deep analysis\n\n")
				orch := agent.NewOrchestrator(agent.OrchestratorConfig{
					Provider:     provider,
					Target:       target,
					Scope:        scopeList,
					Instructions: instructions,
					MaxTurns:     maxTurns,
					NoConfirm:    noConfirm,
					OutDir:       out,
					OnEvent: func(e agent.Event) {
						if eventLogger != nil {
							eventLogger.Log(e)
						}
						switch e.Type {
						case agent.EventProgress:
							logger.Info("  %s", e.Message)
						case agent.EventTextDelta:
							fmt.Fprint(outWriter, e.Message)
						case agent.EventToolCall:
							fmt.Fprintf(outWriter, "\n")
							logger.Info("  [tool] %s", e.ToolName)
						case agent.EventFinding:
							logger.Info("  [finding] %s (%s)", e.Message, e.Finding.Severity)
						case agent.EventError:
							logger.Info("  [error] %s", e.Message)
						case agent.EventDone:
							fmt.Fprintf(outWriter, "\n")
						}
					},
					OnConfirm: func(toolName string, input json.RawMessage) bool {
						fmt.Fprintf(outWriter, "\n[CONFIRM] High-risk tool %q requested. Input: %s\n", toolName, string(input))
						fmt.Fprintf(outWriter, "Allow? [y/N]: ")
						var answer string
						fmt.Fscanln(cmd.InOrStdin(), &answer)
						return answer == "y" || answer == "Y" || answer == "yes"
					},
				})
				orchResult, err := orch.Run(ctx)
				if err != nil {
					return fmt.Errorf("multi-agent scan: %w", err)
				}
				result = orchResult.Combined
			} else if interactive {
				fmt.Fprintf(outWriter, "Interactive agent mode. Type 'exit' or 'quit' to end.\n\n")
				var err error
				result, err = a.RunInteractive(ctx, cmd.InOrStdin(), outWriter)
				if err != nil {
					return fmt.Errorf("agent interactive: %w", err)
				}
			} else {
				var err error
				result, err = a.Run(ctx)
				if err != nil {
					return fmt.Errorf("agent scan: %w", err)
				}
			}

			fmt.Fprintf(outWriter, "\nAgent scan completed\n")
			fmt.Fprintf(outWriter, "turns: %d\n", result.Turns)
			fmt.Fprintf(outWriter, "duration: %s\n", result.Duration.Round(100*1000000))
			fmt.Fprintf(outWriter, "tokens: %d in / %d out\n", result.TokensIn, result.TokensOut)
			if result.EstCostUSD > 0 {
				fmt.Fprintf(outWriter, "est. cost: $%.4f\n", result.EstCostUSD)
			}
			fmt.Fprintf(outWriter, "findings: %d\n", len(result.Findings))
			for _, f := range result.Findings {
				fmt.Fprintf(outWriter, "  [%s] %s\n", f.Severity, f.Title)
			}
			fmt.Fprintf(outWriter, "\nSummary:\n%s\n", result.Summary)

			report, err := agent.GenerateReport(out, result, target)
			if err != nil {
				logger.Info("warning: report generation failed: %s", err)
			} else {
				fmt.Fprintf(outWriter, "\nReports:\n  %s\n  %s\n", report.MDPath, report.JSONPath)
			}

			sessionPath, err := a.SaveSession(out)
			if err != nil {
				logger.Debug("warning: session save failed: %s", err)
			} else {
				logger.Debug("session saved: %s", sessionPath)
			}
			return nil
		},
	}

	cmd.Flags().String("target", "", "target URL to assess")
	cmd.Flags().String("scope", "", "authorized scope (comma-separated domains/IPs)")
	cmd.Flags().String("provider", "anthropic", "LLM provider: anthropic, openai, ollama, anthropic-compatible, openai-compatible")
	cmd.Flags().String("model", "", "model name (provider-specific)")
	cmd.Flags().String("api-key", "", "API key for the LLM provider")
	cmd.Flags().String("base-url", "", "custom base URL for the LLM API")
	cmd.Flags().String("instructions", "", "additional instructions for the agent")
	cmd.Flags().String("out", "", "output directory for reports (default: current directory)")
	cmd.Flags().String("config", "", "path to config file (default: .cyberagent.yaml)")
	cmd.Flags().Int("max-turns", 50, "maximum number of agent turns")
	cmd.Flags().Bool("no-confirm", false, "skip confirmation for high-risk tools")
	cmd.Flags().String("resume", "", "path to session.json to resume a previous scan")
	cmd.Flags().Bool("interactive", false, "run in interactive multi-turn mode")
	cmd.Flags().Bool("multi-agent", false, "run multi-agent mode (recon + deep analysis)")
	cmd.Flags().String("dashboard", "", "start web dashboard on addr (e.g. :8080)")
	return cmd
}

func buildProvider(name string, cfg llm.ProviderConfig) llm.Provider {
	var provider llm.Provider
	switch name {
	case "openai", "openai-compatible":
		provider = llm.NewOpenAI(cfg)
	case "ollama":
		provider = llm.NewOllama(cfg)
	case "anthropic", "anthropic-compatible":
		provider = llm.NewAnthropic(cfg)
	default:
		if cfg.BaseURL == "" {
			provider = llm.NewAnthropic(cfg)
		} else {
			provider = llm.NewOpenAI(cfg)
		}
	}
	return llm.WithRetry(provider, llm.DefaultRetryConfig())
}

func resolveAPIKey(provider string) string {
	switch provider {
	case "openai", "openai-compatible":
		return os.Getenv("OPENAI_API_KEY")
	case "anthropic", "anthropic-compatible":
		return os.Getenv("ANTHROPIC_API_KEY")
	default:
		if key := os.Getenv("LLM_API_KEY"); key != "" {
			return key
		}
		return os.Getenv("OPENAI_API_KEY")
	}
}
