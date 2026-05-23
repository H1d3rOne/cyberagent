package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/runtime/doctor"
	"github.com/spf13/cobra"
)

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local CyberAgent runtime dependencies",
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()

			report := doctor.Run(ctx)
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "CyberAgent doctor\n")
			fmt.Fprintf(out, "platform: %s/%s\n\n", report.OS, report.Arch)
			for _, check := range report.Checks {
				required := "optional"
				if check.Required {
					required = "required"
				}
				fmt.Fprintf(out, "%-12s %-8s %-8s %s\n", check.Name, check.Status, required, check.Detail)
			}
			if report.Healthy {
				fmt.Fprintln(out, "\nrequired checks passed")
			}
		},
	}
}
