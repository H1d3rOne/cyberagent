package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	approvalruntime "github.com/H1d3rOne/cyberagent/internal/runtime/approvals"
)

// ApprovalService exposes HITL approval operations for CLI/API adapters.
type ApprovalService struct {
	runs RunService
}

// NewApprovalService creates an approval service rooted at runsDir.
func NewApprovalService(runsDir string) ApprovalService {
	return ApprovalService{runs: NewRunService(runsDir)}
}

// ApprovalDecisionRequest describes one approval resolution request.
type ApprovalDecisionRequest struct {
	RunIDOrPath        string         `json:"run_id,omitempty"`
	ApprovalIDOrStepID string         `json:"approval_id,omitempty"`
	Operator           string         `json:"operator,omitempty"`
	Comment            string         `json:"comment,omitempty"`
	EditArgs           map[string]any `json:"edit_args,omitempty"`
}

// ResumeApprovedRequest describes a request to resume approved steps for a run.
type ResumeApprovedRequest struct {
	RunIDOrPath string `json:"run_id,omitempty"`
	Operator    string `json:"operator,omitempty"`
}

// ResumeApprovedResult summarizes resumed and skipped steps.
type ResumeApprovedResult struct {
	Resumed []string `json:"resumed"`
	Skipped []string `json:"skipped"`
	RunID   string   `json:"run_id"`
	RunDir  string   `json:"run_dir"`
}

// ListApprovals lists approval requests for a run.
func (s ApprovalService) ListApprovals(ctx context.Context, runIDOrPath string) ([]contracts.HITLRequest, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	layout, err := s.runs.openLayout(runIDOrPath)
	if err != nil {
		return nil, err
	}
	requests, err := approvalruntime.NewStore(layout.Path).List()
	if err != nil {
		return nil, internalError("list approvals", err)
	}
	return requests, nil
}

// Approve approves one pending approval request.
func (s ApprovalService) Approve(ctx context.Context, req ApprovalDecisionRequest) (contracts.HITLRequest, error) {
	return s.resolve(ctx, req, approvalruntime.DecisionApprove)
}

// Deny rejects one pending approval request.
func (s ApprovalService) Deny(ctx context.Context, req ApprovalDecisionRequest) (contracts.HITLRequest, error) {
	return s.resolve(ctx, req, approvalruntime.DecisionDeny)
}

// Cancel cancels one pending approval request.
func (s ApprovalService) Cancel(ctx context.Context, req ApprovalDecisionRequest) (contracts.HITLRequest, error) {
	return s.resolve(ctx, req, approvalruntime.DecisionCancel)
}

// EditArgs resolves one pending approval request with operator-edited args.
func (s ApprovalService) EditArgs(ctx context.Context, req ApprovalDecisionRequest) (contracts.HITLRequest, error) {
	return s.resolve(ctx, req, approvalruntime.DecisionEditArgs)
}

// ResumeApproved resumes approved steps in an existing run workspace.
func (s ApprovalService) ResumeApproved(ctx context.Context, req ResumeApprovedRequest) (ResumeApprovedResult, error) {
	if err := ctxErr(ctx); err != nil {
		return ResumeApprovedResult{}, err
	}
	layout, err := s.runs.openLayout(req.RunIDOrPath)
	if err != nil {
		return ResumeApprovedResult{}, err
	}
	result, err := coreruntime.ResumeApproved(ctx, coreruntime.ResumeApprovedRequest{RunDir: layout.Path})
	if err != nil {
		return ResumeApprovedResult{}, internalError("resume approved steps", err)
	}
	resumed := result.Resumed
	if resumed == nil {
		resumed = []string{}
	}
	skipped := result.Skipped
	if skipped == nil {
		skipped = []string{}
	}
	return ResumeApprovedResult{
		Resumed: resumed,
		Skipped: skipped,
		RunID:   layout.JobID,
		RunDir:  layout.Path,
	}, nil
}

func (s ApprovalService) resolve(ctx context.Context, req ApprovalDecisionRequest, decision approvalruntime.Decision) (contracts.HITLRequest, error) {
	if err := ctxErr(ctx); err != nil {
		return contracts.HITLRequest{}, err
	}
	if strings.TrimSpace(req.ApprovalIDOrStepID) == "" {
		return contracts.HITLRequest{}, badRequest("approval id or step id is required", nil)
	}
	layout, err := s.runs.openLayout(req.RunIDOrPath)
	if err != nil {
		return contracts.HITLRequest{}, err
	}
	resolved, err := approvalruntime.NewStore(layout.Path).ResolveWithMetadata(req.ApprovalIDOrStepID, decision, approvalruntime.ResolutionMetadata{
		ResolvedBy: resolvedBy(req.Operator),
		Comment:    req.Comment,
		EditArgs:   req.EditArgs,
	})
	if err != nil {
		return contracts.HITLRequest{}, mapApprovalResolveError(err)
	}
	return resolved, nil
}

func resolvedBy(operator string) string {
	if strings.TrimSpace(operator) == "" {
		return "service"
	}
	return operator
}

func mapApprovalResolveError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "approval id or step id is required"),
		strings.Contains(message, "unsupported approval decision"):
		return badRequest("invalid approval request", err)
	case strings.Contains(message, "approval request not found"):
		return notFound("approval request not found", err)
	case strings.Contains(message, "is not pending"):
		return NewError(CodeConflict, "approval request is not pending", nil, err)
	default:
		return internalError(fmt.Sprintf("resolve approval: %s", message), err)
	}
}
