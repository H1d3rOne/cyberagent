package cli

import (
	"fmt"

	"github.com/H1d3rOne/cyberagent/internal/interfaces/tui"
	"github.com/spf13/cobra"
)

func newTUICommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tui <run-dir>",
		Short: "Open the CyberAgent TUI run viewer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := tui.RunViewer(args[0]); err != nil {
				return fmt.Errorf("open TUI run viewer: %w", err)
			}
			return nil
		},
	}
}
