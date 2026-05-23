package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand(name, version, module string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n%s\n", name, version, module)
		},
	}
}
