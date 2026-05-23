package approvals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/events"
	"github.com/H1d3rOne/cyberagent/internal/runtime/command"
	"github.com/H1d3rOne/cyberagent/internal/runtime/state"
)

// Decision describes a user approval resolution.
type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionDeny    Decision = "deny"
)

// Store manages persisted approval requests for one run workspace.
type Store struct {
	RunDir string
}

// NewStore creates an approval store rooted at a run workspace.
func NewStore(runDir string) Store {
	return Store{RunDir: runDir}
}

// List returns approval requests from approvals.json.
func (s Store) List() ([]contracts.HITLRequest, error) {
	path := s.approvalsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read approvals %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var requests []contracts.HITLRequest
	if err := json.Unmarshal(data, &requests); err != nil {
		return nil, fmt.Errorf("decode approvals %s: %w", path, err)
	}
	return requests, nil
}

// Resolve marks one pending approval as approved or rejected.
// idOrStepID may be either the HITL request ID or the Step ID.
func (s Store) Resolve(idOrStepID string, decision Decision) (contracts.HITLRequest, error) {
	if strings.TrimSpace(idOrStepID) == "" {
		return contracts.HITLRequest{}, fmt.Errorf("approval id or step id is required")
	}
	if decision != DecisionApprove && decision != DecisionDeny {
		return contracts.HITLRequest{}, fmt.Errorf("unsupported approval decision %q", decision)
	}

	requests, err := s.List()
	if err != nil {
		return contracts.HITLRequest{}, err
	}
	idx := -1
	for i := range requests {
		if requests[i].ID == idOrStepID || requests[i].StepID == idOrStepID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return contracts.HITLRequest{}, fmt.Errorf("approval request not found: %s", idOrStepID)
	}
	if requests[idx].Status != contracts.HITLPending {
		return contracts.HITLRequest{}, fmt.Errorf("approval request %s is not pending: %s", requests[idx].ID, requests[idx].Status)
	}

	resolved := contracts.Now()
	if decision == DecisionApprove {
		requests[idx].Status = contracts.HITLApproved
	} else {
		requests[idx].Status = contracts.HITLRejected
	}
	requests[idx].ResolvedAt = &resolved
	requests[idx].Resolution = map[string]any{
		"decision":    string(decision),
		"resolved_by": "cli",
	}

	if err := s.writeAll(requests); err != nil {
		return contracts.HITLRequest{}, err
	}
	if err := s.syncState(requests[idx]); err != nil {
		return contracts.HITLRequest{}, err
	}
	if err := s.recordResolution(requests[idx], decision); err != nil {
		return contracts.HITLRequest{}, err
	}
	return requests[idx], nil
}

func (s Store) approvalsPath() string {
	return filepath.Join(s.RunDir, "approvals.json")
}

func (s Store) statePath() string {
	return filepath.Join(s.RunDir, "state", "bolt.db")
}

func (s Store) eventsPath() string {
	return filepath.Join(s.RunDir, "events.jsonl")
}

func (s Store) commandsPath() string {
	return filepath.Join(s.RunDir, "commands.log")
}

func (s Store) writeAll(requests []contracts.HITLRequest) error {
	encoded, err := json.MarshalIndent(requests, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal approvals: %w", err)
	}
	if err := os.MkdirAll(s.RunDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	return os.WriteFile(s.approvalsPath(), append(encoded, '\n'), 0o644)
}

func (s Store) syncState(request contracts.HITLRequest) error {
	store, err := state.Open(s.statePath())
	if err != nil {
		return fmt.Errorf("open state store: %w", err)
	}
	defer store.Close()
	if err := store.SaveHITLRequest(request); err != nil {
		return fmt.Errorf("save HITL request: %w", err)
	}
	return nil
}

func (s Store) recordResolution(request contracts.HITLRequest, decision Decision) error {
	eventStore := events.NewStore(s.eventsPath())
	event := contracts.NewEvent("event_approval_resolved_"+request.ID, request.JobID, request.StepID, contracts.EventApprovalResolved, contracts.EventLevelInfo, "approval "+string(decision))
	event.Data = map[string]any{
		"approval_request_id": request.ID,
		"policy_decision_id":  request.PolicyDecisionID,
		"status":              string(request.Status),
		"decision":            string(decision),
	}
	if err := eventStore.Append(event); err != nil {
		return fmt.Errorf("append approval event: %w", err)
	}

	finished := contracts.Now()
	exitCode := 0
	call := contracts.ToolCall{
		ID:         "approval_resolution_" + request.ID,
		JobID:      request.JobID,
		StepID:     request.StepID,
		ToolName:   "cyberagent.approvals",
		RunnerID:   "local",
		Kind:       contracts.ToolCallKindBuiltin,
		Command:    "cyberagent approvals " + string(decision),
		Args:       []string{s.RunDir, request.StepID},
		EnvPolicy:  "no_env",
		StartedAt:  finished,
		FinishedAt: &finished,
		ExitCode:   &exitCode,
		Status:     contracts.StatusSucceeded,
		Metadata: contracts.Metadata{
			"approval_request_id": request.ID,
			"policy_decision_id":  request.PolicyDecisionID,
			"decision":            string(decision),
			"status":              string(request.Status),
		},
	}
	if err := command.NewRecorder(s.commandsPath()).Append(call); err != nil {
		return fmt.Errorf("append approval command record: %w", err)
	}
	return nil
}
