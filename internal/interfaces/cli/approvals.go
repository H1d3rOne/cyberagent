package cli

import (
	"fmt"

	approvalruntime "github.com/H1d3rOne/cyberagent/internal/runtime/approvals"
	"github.com/spf13/cobra"
)

func newApprovalsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approvals",
		Short: "List or resolve pending human approval requests",
	}

	cmd.AddCommand(newApprovalsListCommand())
	cmd.AddCommand(newApprovalsResolveCommand("approve", approvalruntime.DecisionApprove))
	cmd.AddCommand(newApprovalsResolveCommand("deny", approvalruntime.DecisionDeny))
	return cmd
}

func newApprovalsListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list <run-dir>",
		Short: "List approval requests for a run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			requests, err := approvalruntime.NewStore(args[0]).List()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(requests) == 0 {
				fmt.Fprintln(out, "No approval requests found")
				return nil
			}
			fmt.Fprintf(out, "%-42s %-36s %-10s %-14s %-32s %s\n", "ID", "STEP", "STATUS", "ACTION", "POLICY", "REASON")
			for _, request := range requests {
				fmt.Fprintf(out, "%-42s %-36s %-10s %-14s %-32s %s\n", request.ID, request.StepID, request.Status, request.RequestedAction, request.PolicyDecisionID, request.Reason)
			}
			return nil
		},
	}
}

func newApprovalsResolveCommand(name string, decision approvalruntime.Decision) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <run-dir> <approval-id-or-step-id>",
		Short: fmt.Sprintf("%s a pending approval request", titleVerb(name)),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := approvalruntime.NewStore(args[0]).Resolve(args[1], decision)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "approval %s\n", decision)
			fmt.Fprintf(out, "id: %s\n", request.ID)
			fmt.Fprintf(out, "step: %s\n", request.StepID)
			fmt.Fprintf(out, "status: %s\n", request.Status)
			fmt.Fprintf(out, "note: approval state updated only; resume execution is handled by a later command\n")
			return nil
		},
	}
}

func titleVerb(name string) string {
	switch name {
	case "approve":
		return "Approve"
	case "deny":
		return "Deny"
	default:
		return name
	}
}
