package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	approvalruntime "github.com/H1d3rOne/cyberagent/internal/runtime/approvals"
	"github.com/H1d3rOne/cyberagent/internal/runtime/state"
)

type skippedStepSummary struct {
	ID             string
	Name           string
	Tool           string
	Reason         string
	ApprovalID     string
	ApprovalStatus contracts.HITLStatus
}

func writeSkippedStepSummary(out io.Writer, runDir, jobID string) (bool, error) {
	items, err := loadSkippedStepSummary(runDir, jobID)
	if err != nil {
		return false, err
	}
	if len(items) == 0 {
		return false, nil
	}
	fmt.Fprintf(out, "skipped steps: %d\n", len(items))
	for _, item := range items {
		fmt.Fprintf(out, "- %s | tool=%s | reason=%s\n", item.ID, emptySummaryField(item.Tool), emptySummaryField(item.Reason))
		if item.Name != "" && item.Name != item.ID {
			fmt.Fprintf(out, "  name: %s\n", item.Name)
		}
		if item.ApprovalID != "" {
			fmt.Fprintf(out, "  approval: %s (%s)\n", item.ApprovalID, item.ApprovalStatus)
		}
	}
	writeSkippedNextActions(out, runDir, items)
	return true, nil
}

func loadSkippedStepSummary(runDir, jobID string) ([]skippedStepSummary, error) {
	if strings.TrimSpace(jobID) == "" {
		loadedJobID, err := loadRunJobID(runDir)
		if err != nil {
			return nil, err
		}
		jobID = loadedJobID
	}
	statePath := filepath.Join(runDir, "state", "bolt.db")
	if _, err := os.Stat(statePath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat state store: %w", err)
	}
	store, err := state.Open(statePath)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	steps, err := store.ListSteps(jobID)
	if err != nil {
		return nil, err
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].ID < steps[j].ID })

	approvals, err := approvalruntime.NewStore(runDir).List()
	if err != nil {
		return nil, err
	}
	approvalByStep := map[string]contracts.HITLRequest{}
	for _, approval := range approvals {
		if approval.StepID != "" {
			approvalByStep[approval.StepID] = approval
		}
	}

	items := make([]skippedStepSummary, 0)
	for _, step := range steps {
		if step.Status != contracts.StatusSkipped {
			continue
		}
		item := skippedStepSummary{
			ID:     step.ID,
			Name:   step.Name,
			Tool:   step.Tool.Name,
			Reason: step.Error,
		}
		if approval, ok := approvalByStep[step.ID]; ok {
			item.ApprovalID = approval.ID
			item.ApprovalStatus = approval.Status
		}
		items = append(items, item)
	}
	return items, nil
}

func loadRunJobID(runDir string) (string, error) {
	var job contracts.Job
	path := filepath.Join(runDir, "job.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &job); err != nil {
		return "", fmt.Errorf("decode %s: %w", path, err)
	}
	if strings.TrimSpace(job.ID) == "" {
		return "", fmt.Errorf("job id missing in %s", path)
	}
	return job.ID, nil
}

func writeSkippedNextActions(out io.Writer, runDir string, items []skippedStepSummary) {
	hasPendingApproval := false
	hasApprovedApproval := false
	for _, item := range items {
		switch item.ApprovalStatus {
		case contracts.HITLPending:
			hasPendingApproval = true
		case contracts.HITLApproved:
			hasApprovedApproval = true
		}
	}
	fmt.Fprintln(out, "next actions:")
	fmt.Fprintf(out, "- inspect events: cyberagent replay %s\n", runDir)
	if hasPendingApproval || hasApprovedApproval {
		fmt.Fprintf(out, "- list approvals: cyberagent approvals list %s\n", runDir)
	}
	if hasPendingApproval {
		fmt.Fprintf(out, "- approve pending: cyberagent approvals approve %s <approval-id-or-step-id>\n", runDir)
	}
	if hasPendingApproval || hasApprovedApproval {
		fmt.Fprintf(out, "- resume approved: cyberagent replay %s --resume-approved\n", runDir)
	}
}

func emptySummaryField(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
