package policy

import (
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestRiskPolicyAllowsInfoAndLow(t *testing.T) {
	for _, risk := range []contracts.RiskLevel{contracts.RiskInfo, contracts.RiskLow} {
		t.Run(string(risk), func(t *testing.T) {
			step := contracts.NewStep("step_001", "job_001", "plan_001", "test", contracts.StepTypeBuiltinTool, risk, contracts.ToolRef{Name: "builtin.test"})
			decision := NewRiskPolicy().CheckStep("policy_001", step, "safe")
			if decision.Decision != contracts.PolicyAllow {
				t.Fatalf("decision = %s, want allow; reasons=%v", decision.Decision, decision.Reasons)
			}
		})
	}
}

func TestRiskPolicyMediumRequiresApprovalInSafeProfile(t *testing.T) {
	step := contracts.NewStep("step_001", "job_001", "plan_001", "test", contracts.StepTypeExternalTool, contracts.RiskMedium, contracts.ToolRef{Name: "plugin.ffuf"})
	decision := NewRiskPolicy().CheckStep("policy_001", step, "safe")
	if decision.Decision != contracts.PolicyRequireApproval {
		t.Fatalf("decision = %s, want require_approval; reasons=%v", decision.Decision, decision.Reasons)
	}
}

func TestRiskPolicyMediumAllowedWithLimitsOutsideSafeProfile(t *testing.T) {
	step := contracts.NewStep("step_001", "job_001", "plan_001", "test", contracts.StepTypeExternalTool, contracts.RiskMedium, contracts.ToolRef{Name: "plugin.ffuf"})
	decision := NewRiskPolicy().CheckStep("policy_001", step, "standard")
	if decision.Decision != contracts.PolicyAllowWithLimits {
		t.Fatalf("decision = %s, want allow_with_limits; reasons=%v", decision.Decision, decision.Reasons)
	}
}

func TestRiskPolicyHighCriticalRequireApprovalAndForbiddenDenied(t *testing.T) {
	tests := []struct {
		risk contracts.RiskLevel
		want contracts.PolicyAction
	}{
		{contracts.RiskHigh, contracts.PolicyRequireApproval},
		{contracts.RiskCritical, contracts.PolicyRequireApproval},
		{contracts.RiskForbidden, contracts.PolicyDeny},
	}

	for _, tt := range tests {
		t.Run(string(tt.risk), func(t *testing.T) {
			step := contracts.NewStep("step_001", "job_001", "plan_001", "test", contracts.StepTypeExternalTool, tt.risk, contracts.ToolRef{Name: "plugin.test"})
			decision := NewRiskPolicy().CheckStep("policy_001", step, "standard")
			if decision.Decision != tt.want {
				t.Fatalf("decision = %s, want %s; reasons=%v", decision.Decision, tt.want, decision.Reasons)
			}
		})
	}
}

func TestRiskPolicyRequiresApprovalFlagWins(t *testing.T) {
	step := contracts.NewStep("step_001", "job_001", "plan_001", "test", contracts.StepTypeBuiltinTool, contracts.RiskLow, contracts.ToolRef{Name: "builtin.test"})
	step.RequiresApproval = true
	decision := NewRiskPolicy().CheckStep("policy_001", step, "safe")
	if decision.Decision != contracts.PolicyRequireApproval {
		t.Fatalf("decision = %s, want require_approval; reasons=%v", decision.Decision, decision.Reasons)
	}
}
