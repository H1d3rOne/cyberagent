package cli

import (
	"fmt"

	"github.com/H1d3rOne/cyberagent/internal/core/logger"
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	"github.com/spf13/cobra"
)

// NewRootCommand builds the CyberAgent CLI root command.
func NewRootCommand(name, version, module string) *cobra.Command {
	return NewRootCommandWithBuildInfo(name, coreruntime.BuildInfo{Version: version, Module: module})
}

// NewRootCommandWithBuildInfo builds the CyberAgent CLI root command with full build provenance.
func NewRootCommandWithBuildInfo(name string, build coreruntime.BuildInfo) *cobra.Command {
	if build.Version == "" {
		build.Version = "unknown"
	}
	if build.Module == "" {
		build.Module = "github.com/H1d3rOne/cyberagent"
	}
	cmd := &cobra.Command{
		Use:           "cyberagent",
		Short:         "CLI/TUI-first cybersecurity agent for authorized Web/API analysis",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       build.Version,
		Long: fmt.Sprintf(`%s is a CLI/TUI-first cybersecurity agent for authorized Web/API analysis.

Module: %s
Status: MVP safe-scan runtime ready`, name, build.Module),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			verbose, _ := cmd.Flags().GetBool("verbose")
			quiet, _ := cmd.Flags().GetBool("quiet")
			switch {
			case verbose:
				logger.SetLevel(logger.LevelVerbose)
			case quiet:
				logger.SetLevel(logger.LevelQuiet)
			default:
				logger.SetLevel(logger.LevelNormal)
			}
		},
	}

	cmd.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")
	cmd.PersistentFlags().BoolP("quiet", "q", false, "suppress non-error output")
	cmd.MarkFlagsMutuallyExclusive("verbose", "quiet")

	cmd.AddCommand(newVersionCommand(name, build.Version, build.Module))
	cmd.AddCommand(newInitCommand())
	cmd.AddCommand(newDoctorCommand())
	cmd.AddCommand(newScanCommand(build))
	cmd.AddCommand(newReportCommand())
	cmd.AddCommand(newReplayCommand())
	cmd.AddCommand(newApprovalsCommand())
	cmd.AddCommand(newPluginsCommand())
	cmd.AddCommand(newTUICommand())
	cmd.AddCommand(newDashboardCommand())

	return cmd
}
