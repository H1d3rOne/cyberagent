package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newReportCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "report <run-dir>",
		Short: "Print a run report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reportPath, err := reportPathForFormat(args[0], format)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(reportPath)
			if err != nil {
				return fmt.Errorf("read report %s: %w", reportPath, err)
			}
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}
	cmd.Flags().StringVarP(&format, "format", "f", "md", "report format: md or json")
	return cmd
}

func reportPathForFormat(runDir, format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "md", "markdown":
		return filepath.Join(runDir, "reports", "report.md"), nil
	case "json":
		return filepath.Join(runDir, "reports", "report.json"), nil
	default:
		return "", fmt.Errorf("invalid report format %q: expected md or json", format)
	}
}
