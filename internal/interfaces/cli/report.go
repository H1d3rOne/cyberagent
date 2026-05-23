package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newReportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "report <run-dir>",
		Short: "Print a run Markdown report",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reportPath := filepath.Join(args[0], "reports", "report.md")
			data, err := os.ReadFile(reportPath)
			if err != nil {
				return fmt.Errorf("read report %s: %w", reportPath, err)
			}
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}
}
