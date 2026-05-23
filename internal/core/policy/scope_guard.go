package policy

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// ScopeGuard checks whether active behavior stays inside the authorized scope.
type ScopeGuard struct{}

// NewScopeGuard creates a ScopeGuard.
func NewScopeGuard() ScopeGuard {
	return ScopeGuard{}
}

// CheckTarget checks a normalized target against scope.
func (g ScopeGuard) CheckTarget(target contracts.Target, scope contracts.Scope) contracts.PolicyDecision {
	raw := target.Normalized.BaseURL
	if target.Normalized.Path != "" {
		raw += target.Normalized.Path
	}
	return g.CheckURL("policy_scope_target", "", "", raw, scope)
}

// CheckURL checks one URL against scope and returns a policy decision.
func (g ScopeGuard) CheckURL(id, jobID, stepID, raw string, scope contracts.Scope) contracts.PolicyDecision {
	decision := contracts.PolicyDecision{
		ID:            id,
		JobID:         jobID,
		StepID:        stepID,
		Decision:      contracts.PolicyDeny,
		Reasons:       []string{},
		AppliedLimits: map[string]any{},
		CreatedAt:     contracts.Now(),
	}

	if strings.TrimSpace(scope.ID) == "" || !hasHostBoundary(scope.Allowed) {
		decision.Reasons = append(decision.Reasons, "scope_missing_or_empty")
		return decision
	}

	u, err := url.Parse(raw)
	if err != nil {
		decision.Reasons = append(decision.Reasons, "url_parse_error")
		return decision
	}
	if u.Scheme == "" || u.Hostname() == "" {
		decision.Reasons = append(decision.Reasons, "url_missing_scheme_or_host")
		return decision
	}

	ref, err := newURLRef(u)
	if err != nil {
		decision.Reasons = append(decision.Reasons, "url_normalize_error")
		return decision
	}

	if denied, reasons := matchesDenied(ref, scope.Denied); denied {
		decision.Reasons = append(decision.Reasons, reasons...)
		return decision
	}

	if ok, reasons := matchesAllowed(ref, scope.Allowed); !ok {
		decision.Reasons = append(decision.Reasons, reasons...)
		return decision
	}

	decision.Decision = contracts.PolicyAllowWithLimits
	decision.Reasons = append(decision.Reasons, "scope_allowed")
	if scope.Limits.MaxRequestsPerSecond > 0 {
		decision.AppliedLimits["max_requests_per_second"] = scope.Limits.MaxRequestsPerSecond
	}
	if scope.Limits.MaxConcurrencyPerHost > 0 {
		decision.AppliedLimits["max_concurrency_per_host"] = scope.Limits.MaxConcurrencyPerHost
	}
	if scope.Limits.MaxTotalRequests > 0 {
		decision.AppliedLimits["max_total_requests"] = scope.Limits.MaxTotalRequests
	}
	if scope.Limits.MaxRuntimeSeconds > 0 {
		decision.AppliedLimits["max_runtime_seconds"] = scope.Limits.MaxRuntimeSeconds
	}
	return decision
}

type urlRef struct {
	Scheme string
	Host   string
	Port   int
	Path   string
	IP     netip.Addr
	HasIP  bool
}

func newURLRef(u *url.URL) (urlRef, error) {
	port := defaultPort(u.Scheme)
	if p := u.Port(); p != "" {
		parsed, err := strconv.Atoi(p)
		if err != nil || parsed <= 0 || parsed > 65535 {
			return urlRef{}, fmt.Errorf("invalid port %q", p)
		}
		port = parsed
	}
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	ref := urlRef{Scheme: strings.ToLower(u.Scheme), Host: host, Port: port, Path: p}
	if ip, err := netip.ParseAddr(host); err == nil {
		ref.IP = ip.Unmap()
		ref.HasIP = true
	}
	return ref, nil
}

func defaultPort(scheme string) int {
	if strings.EqualFold(scheme, "https") {
		return 443
	}
	return 80
}

func hasHostBoundary(r contracts.ScopeRules) bool {
	return len(r.Hosts) > 0 || len(r.Domains) > 0 || len(r.CIDRs) > 0
}

func matchesAllowed(ref urlRef, r contracts.ScopeRules) (bool, []string) {
	var missing []string
	if !matchesAnyHostBoundary(ref, r) {
		missing = append(missing, "host_not_allowed")
	}
	if len(r.Schemes) > 0 && !containsFold(r.Schemes, ref.Scheme) {
		missing = append(missing, "scheme_not_allowed")
	}
	if len(r.Ports) > 0 && !containsInt(r.Ports, ref.Port) {
		missing = append(missing, "port_not_allowed")
	}
	if len(r.Paths) > 0 && !matchesAnyPath(r.Paths, ref.Path) {
		missing = append(missing, "path_not_allowed")
	}
	return len(missing) == 0, missing
}

func matchesDenied(ref urlRef, r contracts.ScopeRules) (bool, []string) {
	var reasons []string
	if len(r.Hosts) > 0 && matchesHost(ref.Host, r.Hosts) {
		reasons = append(reasons, "host_denied")
	}
	if len(r.Domains) > 0 && matchesDomain(ref.Host, r.Domains) {
		reasons = append(reasons, "domain_denied")
	}
	if len(r.CIDRs) > 0 && matchesCIDR(ref, r.CIDRs) {
		reasons = append(reasons, "cidr_denied")
	}
	if len(r.Schemes) > 0 && containsFold(r.Schemes, ref.Scheme) {
		reasons = append(reasons, "scheme_denied")
	}
	if len(r.Ports) > 0 && containsInt(r.Ports, ref.Port) {
		reasons = append(reasons, "port_denied")
	}
	if len(r.Paths) > 0 && matchesAnyPath(r.Paths, ref.Path) {
		reasons = append(reasons, "path_denied")
	}
	return len(reasons) > 0, reasons
}

func matchesAnyHostBoundary(ref urlRef, r contracts.ScopeRules) bool {
	return matchesHost(ref.Host, r.Hosts) || matchesDomain(ref.Host, r.Domains) || matchesCIDR(ref, r.CIDRs)
}

func matchesHost(host string, allowed []string) bool {
	for _, item := range allowed {
		candidate := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(item), "."))
		if candidate == "" {
			continue
		}
		if strings.EqualFold(host, candidate) {
			return true
		}
	}
	return false
}

func matchesDomain(host string, domains []string) bool {
	for _, item := range domains {
		domain := strings.ToLower(strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(item), "."), "."))
		if domain == "" || net.ParseIP(host) != nil {
			continue
		}
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func matchesCIDR(ref urlRef, cidrs []string) bool {
	if !ref.HasIP {
		return false
	}
	for _, item := range cidrs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			continue
		}
		if prefix.Contains(ref.IP) {
			return true
		}
	}
	return false
}

func matchesAnyPath(patterns []string, target string) bool {
	if target == "" {
		target = "/"
	}
	for _, raw := range patterns {
		pattern := strings.TrimSpace(raw)
		if pattern == "" {
			continue
		}
		if pattern == "/*" || pattern == "*" {
			return true
		}
		if !strings.HasPrefix(pattern, "/") {
			pattern = "/" + pattern
		}
		if ok, _ := path.Match(pattern, target); ok {
			return true
		}
		if strings.HasSuffix(pattern, "*") && strings.HasPrefix(target, strings.TrimSuffix(pattern, "*")) {
			return true
		}
		if target == pattern {
			return true
		}
	}
	return false
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func containsInt(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
