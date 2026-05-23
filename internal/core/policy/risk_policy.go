package policy

import (
	"fmt"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// RiskPolicy decides whether a step can run automatically.
type RiskPolicy struct{}

// NewRiskPolicy creates a RiskPolicy.
func NewRiskPolicy() RiskPolicy {
	return RiskPolicy{}
}

// CheckStep returns the risk decision for one step.
func (p RiskPolicy) CheckStep(id string, step contracts.Step, profile string) contracts.PolicyDecision {
	decision := contracts.PolicyDecision{
		ID:            id,
		JobID:         step.JobID,
		StepID:        step.ID,
		Decision:      contracts.PolicyDeny,
		Reasons:       []string{},
		AppliedLimits: map[string]any{},
		CreatedAt:     contracts.Now(),
	}

	if step.RequiresApproval {
		decision.Decision = contracts.PolicyRequireApproval
		decision.Reasons = append(decision.Reasons, "step_requires_approval")
		return decision
	}

	switch step.Risk {
	case contracts.RiskInfo, contracts.RiskLow:
		decision.Decision = contracts.PolicyAllow
		decision.Reasons = append(decision.Reasons, fmt.Sprintf("risk_%s_allowed", step.Risk))
	case contracts.RiskMedium:
		if profile == "safe" || profile == "" {
			decision.Decision = contracts.PolicyRequireApproval
			decision.Reasons = append(decision.Reasons, "risk_medium_requires_approval_in_safe_profile")
		} else {
			decision.Decision = contracts.PolicyAllowWithLimits
			decision.Reasons = append(decision.Reasons, "risk_medium_allowed_with_limits")
		}
	case contracts.RiskHigh, contracts.RiskCritical:
		decision.Decision = contracts.PolicyRequireApproval
		decision.Reasons = append(decision.Reasons, fmt.Sprintf("risk_%s_requires_approval", step.Risk))
	case contracts.RiskForbidden:
		decision.Decision = contracts.PolicyDeny
		decision.Reasons = append(decision.Reasons, "risk_forbidden_denied")
	default:
		decision.Decision = contracts.PolicyDeny
		decision.Reasons = append(decision.Reasons, "unknown_risk_denied")
	}

	return decision
}
