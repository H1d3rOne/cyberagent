package cli

import (
	"fmt"
	"os"

	"github.com/H1d3rOne/cyberagent/internal/core/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a .cyberagent.yaml config file in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := config.ProjectFile
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists", path)
			}

			cfg := config.Default()
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}

			if err := os.WriteFile(path, data, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", path)
			return nil
		},
	}
}
