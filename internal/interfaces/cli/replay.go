package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	"github.com/spf13/cobra"
)

func newReplayCommand() *cobra.Command {
	var resumeApproved bool
	cmd := &cobra.Command{
		Use:   "replay <run-dir>",
		Short: "Replay a run from events.jsonl",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if resumeApproved {
				result, err := coreruntime.ResumeApproved(cmd.Context(), coreruntime.ResumeApprovedRequest{RunDir: args[0]})
				if err != nil {
					return err
				}
				fmt.Fprintln(out, "resume-approved completed")
				if len(result.Resumed) > 0 {
					fmt.Fprintf(out, "resumed: %v\n", result.Resumed)
				}
				if len(result.Skipped) > 0 {
					fmt.Fprintf(out, "skipped: %v\n", result.Skipped)
				}
				fmt.Fprintln(out)
			}

			if written, err := writeSkippedStepSummary(out, args[0], ""); err != nil {
				return fmt.Errorf("write skipped summary: %w", err)
			} else if written {
				fmt.Fprintln(out)
			}

			eventsPath := filepath.Join(args[0], "events.jsonl")
			file, err := os.Open(eventsPath)
			if err != nil {
				return fmt.Errorf("open events %s: %w", eventsPath, err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			line := 0
			for scanner.Scan() {
				line++
				if len(scanner.Bytes()) == 0 {
					continue
				}
				var event contracts.Event
				if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
					return fmt.Errorf("decode event line %d: %w", line, err)
				}
				step := "-"
				if event.StepID != "" {
					step = event.StepID
				}
				fmt.Fprintf(out, "%s %-22s %-8s %s %s\n", event.Timestamp.Format("2006-01-02T15:04:05Z07:00"), event.Type, event.Level, step, event.Message)
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("scan events: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&resumeApproved, "resume-approved", false, "resume approved steps before replaying events")
	return cmd
}
