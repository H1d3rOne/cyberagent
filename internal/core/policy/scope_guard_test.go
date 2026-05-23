package policy

import (
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestScopeGuardAllowsAuthorizedURL(t *testing.T) {
	scope := contracts.NewScope("scope_001", contracts.ScopeRules{
		Hosts:   []string{"localhost"},
		Ports:   []int{8080},
		Schemes: []string{"http"},
		Paths:   []string{"/*"},
	})
	decision := NewScopeGuard().CheckURL("policy_001", "job_001", "step_001", "http://localhost:8080/index.html", scope)
	if decision.Decision != contracts.PolicyAllowWithLimits {
		t.Fatalf("decision = %s, want %s; reasons=%v", decision.Decision, contracts.PolicyAllowWithLimits, decision.Reasons)
	}
	if decision.AppliedLimits["max_requests_per_second"] != 2 {
		t.Fatalf("expected scope limits to be applied: %#v", decision.AppliedLimits)
	}
}

func TestScopeGuardDeniesWithoutScopeBoundary(t *testing.T) {
	decision := NewScopeGuard().CheckURL("policy_001", "job_001", "step_001", "http://localhost:8080/", contracts.Scope{})
	if decision.Decision != contracts.PolicyDeny {
		t.Fatalf("decision = %s, want deny", decision.Decision)
	}
	assertReason(t, decision, "scope_missing_or_empty")
}

func TestScopeGuardDeniedRulesTakePrecedence(t *testing.T) {
	scope := contracts.NewScope("scope_001", contracts.ScopeRules{
		Hosts: []string{"localhost"},
		Paths: []string{"/*"},
	})
	scope.Denied.Paths = []string{"/logout", "/admin/delete*"}

	decision := NewScopeGuard().CheckURL("policy_001", "job_001", "step_001", "http://localhost/admin/deleteUser", scope)
	if decision.Decision != contracts.PolicyDeny {
		t.Fatalf("decision = %s, want deny", decision.Decision)
	}
	assertReason(t, decision, "path_denied")
}

func TestScopeGuardDeniesOutOfScopeParts(t *testing.T) {
	scope := contracts.NewScope("scope_001", contracts.ScopeRules{
		Hosts:   []string{"localhost"},
		Ports:   []int{8080},
		Schemes: []string{"http"},
		Paths:   []string{"/app/*"},
	})

	tests := []struct {
		name   string
		raw    string
		reason string
	}{
		{name: "host", raw: "http://example.com:8080/app/", reason: "host_not_allowed"},
		{name: "scheme", raw: "https://localhost:8080/app/", reason: "scheme_not_allowed"},
		{name: "port", raw: "http://localhost:9090/app/", reason: "port_not_allowed"},
		{name: "path", raw: "http://localhost:8080/admin/", reason: "path_not_allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := NewScopeGuard().CheckURL("policy_001", "job_001", "step_001", tt.raw, scope)
			if decision.Decision != contracts.PolicyDeny {
				t.Fatalf("decision = %s, want deny", decision.Decision)
			}
			assertReason(t, decision, tt.reason)
		})
	}
}

func TestScopeGuardAllowsDomainAndCIDR(t *testing.T) {
	domainScope := contracts.NewScope("scope_domain", contracts.ScopeRules{Domains: []string{"example.test"}})
	decision := NewScopeGuard().CheckURL("policy_001", "job_001", "step_001", "https://api.example.test/v1", domainScope)
	if decision.Decision != contracts.PolicyAllowWithLimits {
		t.Fatalf("domain decision = %s, want allow; reasons=%v", decision.Decision, decision.Reasons)
	}

	cidrScope := contracts.NewScope("scope_cidr", contracts.ScopeRules{CIDRs: []string{"10.10.10.0/24"}})
	decision = NewScopeGuard().CheckURL("policy_002", "job_001", "step_001", "http://10.10.10.5/", cidrScope)
	if decision.Decision != contracts.PolicyAllowWithLimits {
		t.Fatalf("cidr decision = %s, want allow; reasons=%v", decision.Decision, decision.Reasons)
	}
}

func TestScopeGuardCheckTarget(t *testing.T) {
	target, err := contracts.NewWebTarget("target_001", "http://127.0.0.1:3000/")
	if err != nil {
		t.Fatalf("NewWebTarget() error = %v", err)
	}
	scope := contracts.NewScope("scope_001", contracts.ScopeRules{Hosts: []string{"127.0.0.1"}, Ports: []int{3000}})
	decision := NewScopeGuard().CheckTarget(target, scope)
	if decision.Decision != contracts.PolicyAllowWithLimits {
		t.Fatalf("decision = %s, want allow; reasons=%v", decision.Decision, decision.Reasons)
	}
}

func assertReason(t *testing.T, decision contracts.PolicyDecision, reason string) {
	t.Helper()
	for _, got := range decision.Reasons {
		if got == reason {
			return
		}
	}
	t.Fatalf("reason %q not found in %#v", reason, decision.Reasons)
}
