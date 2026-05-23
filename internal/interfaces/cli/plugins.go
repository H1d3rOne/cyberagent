package cli

import (
	"fmt"

	"github.com/H1d3rOne/cyberagent/internal/plugins"
	"github.com/spf13/cobra"
)

func newPluginsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "Manage CyberAgent plugins",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List known plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			manifests, err := plugins.NewManager().Discover()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(manifests) == 0 {
				fmt.Fprintln(out, "no plugins discovered")
				return nil
			}
			fmt.Fprintf(out, "%-28s %-10s %-16s %-8s %-8s %s\n", "NAME", "TYPE", "RUNTIME", "RISK", "APPROVAL", "DESCRIPTION")
			for _, manifest := range manifests {
				approval := "no"
				if manifest.RequiresApproval {
					approval = "yes"
				}
				fmt.Fprintf(out, "%-28s %-10s %-16s %-8s %-8s %s\n", manifest.Name, manifest.Type, manifest.Runtime, manifest.Risk, approval, manifest.Description)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Check plugin dependencies",
		RunE: func(cmd *cobra.Command, args []string) error {
			manifests, err := plugins.NewManager().Discover()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(manifests) == 0 {
				fmt.Fprintln(out, "no plugins discovered")
				return nil
			}
			for _, manifest := range manifests {
				health := plugins.Doctor(manifest)
				status := "ok"
				if !health.OK {
					status = "missing"
				}
				fmt.Fprintf(out, "%s (%s): %s\n", manifest.Name, manifest.Type, status)
				if len(health.Dependencies) == 0 {
					fmt.Fprintln(out, "  no dependencies")
				}
				for _, dep := range health.Dependencies {
					required := "optional"
					if dep.Required {
						required = "required"
					}
					depStatus := "ok"
					if !dep.OK {
						depStatus = "missing"
					}
					fmt.Fprintf(out, "  dep    %-14s %-8s %-8s %s\n", dep.Name, depStatus, required, dep.Detail)
				}
				if len(health.Schemas) == 0 {
					fmt.Fprintln(out, "  no schemas")
				}
				for _, schema := range health.Schemas {
					required := "optional"
					if schema.Required {
						required = "required"
					}
					schemaStatus := "ok"
					if !schema.OK {
						schemaStatus = "invalid"
					}
					fmt.Fprintf(out, "  schema %-14s %-8s %-8s %s %s\n", schema.Name, schemaStatus, required, schema.Path, schema.Detail)
				}
			}
			return nil
		},
	})

	return cmd
}
