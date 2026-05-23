package policy

import "github.com/H1d3rOne/cyberagent/internal/core/contracts"

// Engine combines scope and risk policy checks.
type Engine struct {
	Scope ScopeGuard
	Risk  RiskPolicy
}

// NewEngine creates a policy engine with default policies.
func NewEngine() Engine {
	return Engine{Scope: NewScopeGuard(), Risk: NewRiskPolicy()}
}

// CheckStep checks a step risk policy. Network scope checks are explicit via CheckURL/CheckTarget.
func (e Engine) CheckStep(id string, step contracts.Step, profile string) contracts.PolicyDecision {
	return e.Risk.CheckStep(id, step, profile)
}

// CheckURL checks a URL against scope.
func (e Engine) CheckURL(id, jobID, stepID, raw string, scope contracts.Scope) contracts.PolicyDecision {
	return e.Scope.CheckURL(id, jobID, stepID, raw, scope)
}

// CheckTarget checks a normalized target against scope.
func (e Engine) CheckTarget(target contracts.Target, scope contracts.Scope) contracts.PolicyDecision {
	return e.Scope.CheckTarget(target, scope)
}
