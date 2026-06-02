package daemon

import (
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

const (
	approvalKindAgentAuthorization = "agent_authorization"
	approvalKindRunHITL            = "run_hitl"

	approvalCanonicalStatePending   = "pending"
	approvalCanonicalStateApproved  = "approved"
	approvalCanonicalStateDenied    = "denied"
	approvalCanonicalStateCancelled = "cancelled"
	approvalCanonicalStateModified  = "modified"
	approvalCanonicalStateExpired   = "expired"
	approvalCanonicalStateUnknown   = "unknown"
)

type approvalCanonicalState struct {
	ApprovalKind    string `json:"approval_kind"`
	State           string `json:"state"`
	Decision        string `json:"decision,omitempty"`
	ResumeAvailable bool   `json:"resume_available"`
	ApprovalID      string `json:"approval_id,omitempty"`
	RunID           string `json:"run_id,omitempty"`
	StepID          string `json:"step_id,omitempty"`
	ActionName      string `json:"action_name,omitempty"`
	RequiredLevel   string `json:"required_level,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

type runHITLApprovalResponse struct {
	contracts.HITLRequest
	ApprovalState *approvalCanonicalState `json:"approval_state,omitempty"`
}

func deriveAgentApprovalState(session *agentSession) *approvalCanonicalState {
	if session == nil {
		return nil
	}
	return canonicalAgentAuthorizationApproval(session.PendingAuthorization, "")
}

func canonicalAgentAuthorizationApproval(auth *agentAuthorizationRequest, decision string) *approvalCanonicalState {
	if auth == nil {
		return nil
	}
	decision = strings.TrimSpace(decision)
	state := canonicalApprovalState(auth.Status)
	if decision != "" {
		state = canonicalApprovalState(decision)
	}
	canonicalDecision := canonicalApprovalDecision(firstNonEmpty(decision, auth.Status))
	if state == approvalCanonicalStatePending {
		canonicalDecision = ""
	}
	if canonicalDecision == "" {
		canonicalDecision = canonicalApprovalDecision(state)
	}
	return &approvalCanonicalState{
		ApprovalKind:    approvalKindAgentAuthorization,
		State:           state,
		Decision:        canonicalDecision,
		ResumeAvailable: false,
		ApprovalID:      auth.ID,
		ActionName:      auth.ActionName,
		RequiredLevel:   auth.RequiredLevel,
		Reason:          auth.Reason,
	}
}

func runHITLApprovalResponses(runID string, requests []contracts.HITLRequest) []runHITLApprovalResponse {
	responses := make([]runHITLApprovalResponse, 0, len(requests))
	for _, request := range requests {
		responses = append(responses, runHITLApprovalResponseFor(runID, request))
	}
	return responses
}

func runHITLApprovalResponseFor(runID string, request contracts.HITLRequest) runHITLApprovalResponse {
	return runHITLApprovalResponse{
		HITLRequest:   request,
		ApprovalState: canonicalRunHITLApproval(runID, request),
	}
}

func canonicalRunHITLApproval(runID string, request contracts.HITLRequest) *approvalCanonicalState {
	state := canonicalApprovalState(string(request.Status))
	decision := canonicalApprovalDecision(approvalRuntimeResolutionString(request.Resolution, "decision"))
	if decision == "" {
		decision = canonicalApprovalDecision(state)
	}
	return &approvalCanonicalState{
		ApprovalKind:    approvalKindRunHITL,
		State:           state,
		Decision:        decision,
		ResumeAvailable: state == approvalCanonicalStateApproved,
		ApprovalID:      request.ID,
		RunID:           firstNonEmpty(strings.TrimSpace(runID), strings.TrimSpace(request.JobID)),
		StepID:          request.StepID,
		ActionName:      request.RequestedAction,
		Reason:          request.Reason,
	}
}

func canonicalApprovalState(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending", "wait", "waiting", "approval_required", "authorization_required", "waiting_authorization":
		return approvalCanonicalStatePending
	case "approve", "approved", "allow", "allowed", "authorized", "accepted":
		return approvalCanonicalStateApproved
	case "deny", "denied", "reject", "rejected", "disallow", "disallowed":
		return approvalCanonicalStateDenied
	case "cancel", "cancelled", "canceled":
		return approvalCanonicalStateCancelled
	case "modify", "modified", "edit", "edited", "edit_args", "edit-args":
		return approvalCanonicalStateModified
	case "expire", "expired":
		return approvalCanonicalStateExpired
	default:
		return approvalCanonicalStateUnknown
	}
}

func canonicalApprovalDecision(raw string) string {
	switch canonicalApprovalState(raw) {
	case approvalCanonicalStateApproved:
		return approvalCanonicalStateApproved
	case approvalCanonicalStateDenied:
		return approvalCanonicalStateDenied
	case approvalCanonicalStateCancelled:
		return approvalCanonicalStateCancelled
	case approvalCanonicalStateModified:
		return approvalCanonicalStateModified
	default:
		return ""
	}
}

func canonicalApprovalStateIsTerminal(state string) bool {
	switch state {
	case approvalCanonicalStateApproved, approvalCanonicalStateDenied, approvalCanonicalStateCancelled, approvalCanonicalStateModified, approvalCanonicalStateExpired:
		return true
	default:
		return false
	}
}
