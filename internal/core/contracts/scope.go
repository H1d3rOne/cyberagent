package contracts

import "time"

// Scope describes the authorized boundary for active behavior.
type Scope struct {
	ID        string      `json:"id"`
	Allowed   ScopeRules  `json:"allowed"`
	Denied    ScopeRules  `json:"denied"`
	Limits    ScopeLimits `json:"limits"`
	Profiles  []string    `json:"profiles,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

// ScopeRules groups allow/deny matchers.
type ScopeRules struct {
	Hosts   []string `json:"hosts,omitempty"`
	Domains []string `json:"domains,omitempty"`
	CIDRs   []string `json:"cidrs,omitempty"`
	Ports   []int    `json:"ports,omitempty"`
	Schemes []string `json:"schemes,omitempty"`
	Paths   []string `json:"paths,omitempty"`
}

// ScopeLimits describes runtime constraints enforced by policy/runner layers.
type ScopeLimits struct {
	MaxRequestsPerSecond  int `json:"max_requests_per_second,omitempty"`
	MaxConcurrencyPerHost int `json:"max_concurrency_per_host,omitempty"`
	MaxTotalRequests      int `json:"max_total_requests,omitempty"`
	MaxRuntimeSeconds     int `json:"max_runtime_seconds,omitempty"`
}

// NewScope creates a scope with safe defaults.
func NewScope(id string, allowed ScopeRules) Scope {
	return Scope{
		ID:      id,
		Allowed: allowed,
		Denied:  ScopeRules{},
		Limits: ScopeLimits{
			MaxRequestsPerSecond:  2,
			MaxConcurrencyPerHost: 2,
			MaxTotalRequests:      1000,
			MaxRuntimeSeconds:     1800,
		},
		Profiles:  []string{"safe"},
		CreatedAt: Now(),
	}
}
