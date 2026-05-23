package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/spf13/cobra"
)

func newReplayCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "replay <run-dir>",
		Short: "Replay a run from events.jsonl",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eventsPath := filepath.Join(args[0], "events.jsonl")
			file, err := os.Open(eventsPath)
			if err != nil {
				return fmt.Errorf("open events %s: %w", eventsPath, err)
			}
			defer file.Close()

			out := cmd.OutOrStdout()
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
}
