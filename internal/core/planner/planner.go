package planner

import (
	"fmt"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

const (
	// ProfileSafe is the MVP conservative Web/API analysis profile.
	ProfileSafe = "safe"

	// BuiltinWebSafePlanner is the planner identifier stored in Plan.Planner.
	BuiltinWebSafePlanner = "builtin.web.safe.v1"
)

// Capability describes an available tool/plugin capability.
type Capability struct {
	Name string
	Risk contracts.RiskLevel
}

// Options controls plan generation.
type Options struct {
	Profile        string
	EnabledPlugins []string
	Capabilities   []Capability
}

// Result contains the plan plus concrete step contracts.
type Result struct {
	Plan  contracts.Plan
	Steps []contracts.Step
}

// Planner builds execution plans.
type Planner struct{}

// New creates a Planner.
func New() Planner {
	return Planner{}
}

// BuildWebPlan builds a plan for an authorized web target.
func (p Planner) BuildWebPlan(job contracts.Job, target contracts.Target, scope contracts.Scope, opts Options) (Result, error) {
	profile := opts.Profile
	if strings.TrimSpace(profile) == "" {
		profile = ProfileSafe
	}
	if profile != ProfileSafe {
		return Result{}, fmt.Errorf("unsupported profile %q", profile)
	}
	if target.Type != contracts.TargetTypeWebURL {
		return Result{}, fmt.Errorf("unsupported target type %q", target.Type)
	}
	if strings.TrimSpace(scope.ID) == "" || !scopeHasBoundary(scope) {
		return Result{}, fmt.Errorf("scope is required before building active web plan")
	}

	return buildWebSafePlan(job, target, opts), nil
}

func scopeHasBoundary(scope contracts.Scope) bool {
	return len(scope.Allowed.Hosts) > 0 || len(scope.Allowed.Domains) > 0 || len(scope.Allowed.CIDRs) > 0
}

func pluginEnabled(opts Options, name string) bool {
	for _, enabled := range opts.EnabledPlugins {
		if strings.EqualFold(strings.TrimSpace(enabled), name) {
			return true
		}
	}
	return false
}
