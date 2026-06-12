package daemon

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
	"gopkg.in/yaml.v3"
)

const daemonRBACWorkspaceSelectPathTemplate = "/api/v1/workspaces/{workspace_id}/select"

type rbacPolicy struct {
	Source       string
	Requirements map[string]rbacRequirement
}

func (p rbacPolicy) Clone() rbacPolicy {
	p.Source = strings.TrimSpace(p.Source)
	if len(p.Requirements) > 0 {
		cloned := make(map[string]rbacRequirement, len(p.Requirements))
		for key, requirement := range p.Requirements {
			cloned[key] = requirement.Clone()
		}
		p.Requirements = cloned
	}
	return p
}

func (p rbacPolicy) Loaded() bool {
	return len(p.Requirements) > 0
}

func (p rbacPolicy) withDefaultRequirements() rbacPolicy {
	if len(p.Requirements) > 0 {
		return p.Clone()
	}
	return daemonDefaultRBACPolicy()
}

func daemonDefaultRBACPolicy() rbacPolicy {
	return rbacPolicy{
		Source:       "builtin",
		Requirements: daemonRBACRouteRequirements(),
	}
}

func newDaemonRBACPolicyFromConfig(cfg RBACConfig) (rbacPolicy, error) {
	if !cfg.Enabled {
		return rbacPolicy{}, nil
	}
	if strings.TrimSpace(cfg.PolicyPath) == "" {
		return daemonDefaultRBACPolicy(), nil
	}
	return loadDaemonRBACPolicyFile(cfg.PolicyPath)
}

type daemonRBACPolicyFile struct {
	Version int                     `yaml:"version"`
	Routes  []daemonRBACPolicyRoute `yaml:"routes"`
}

type daemonRBACPolicyRoute struct {
	Method string   `yaml:"method"`
	Path   string   `yaml:"path"`
	Action string   `yaml:"action"`
	Scope  string   `yaml:"scope"`
	Roles  []string `yaml:"roles"`
}

func loadDaemonRBACPolicyFile(path string) (rbacPolicy, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return rbacPolicy{}, fmt.Errorf("rbac policy path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return rbacPolicy{}, fmt.Errorf("read rbac policy %q: %w", path, err)
	}
	var raw daemonRBACPolicyFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return rbacPolicy{}, fmt.Errorf("parse rbac policy %q: %w", path, err)
	}
	if raw.Version != 0 && raw.Version != 1 {
		return rbacPolicy{}, fmt.Errorf("rbac policy %q has unsupported version %d", path, raw.Version)
	}
	if len(raw.Routes) == 0 {
		return rbacPolicy{}, fmt.Errorf("rbac policy %q must define at least one route", path)
	}
	requirements := make(map[string]rbacRequirement, len(raw.Routes))
	for idx, route := range raw.Routes {
		key, requirement, err := normalizeDaemonRBACPolicyRoute(route)
		if err != nil {
			return rbacPolicy{}, fmt.Errorf("rbac policy %q route[%d]: %w", path, idx, err)
		}
		requirements[key] = requirement
	}
	return rbacPolicy{
		Source:       path,
		Requirements: requirements,
	}, nil
}

func normalizeDaemonRBACPolicyRoute(route daemonRBACPolicyRoute) (string, rbacRequirement, error) {
	method := strings.ToUpper(strings.TrimSpace(route.Method))
	path := strings.TrimSpace(route.Path)
	action := strings.TrimSpace(route.Action)
	scope := strings.ToLower(strings.TrimSpace(route.Scope))
	if method == "" {
		return "", rbacRequirement{}, fmt.Errorf("method is required")
	}
	if path == "" || !strings.HasPrefix(path, "/") {
		return "", rbacRequirement{}, fmt.Errorf("path must be an absolute API path")
	}
	if action == "" {
		return "", rbacRequirement{}, fmt.Errorf("action is required")
	}
	if scope != "tenant" && scope != "workspace" {
		return "", rbacRequirement{}, fmt.Errorf("scope must be tenant or workspace")
	}
	if len(route.Roles) == 0 {
		return "", rbacRequirement{}, fmt.Errorf("roles are required")
	}
	roles := make([]coretenant.Role, 0, len(route.Roles))
	for _, rawRole := range route.Roles {
		role, ok := coretenant.NormalizeRole(rawRole)
		if !ok {
			return "", rbacRequirement{}, fmt.Errorf("unknown role %q", rawRole)
		}
		if role.Scope() != scope {
			return "", rbacRequirement{}, fmt.Errorf("role %q has scope %q, want %q", role, role.Scope(), scope)
		}
		roles = append(roles, role)
	}
	return daemonRBACPolicyRouteKey(method, path), rbacRequirement{
		Action: action,
		Scope:  scope,
		Roles:  roles,
	}, nil
}

func daemonRBACPolicyRouteKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}

func daemonRBACRequirementForRequestWithPolicy(policy rbacPolicy, r *http.Request) (rbacRequirement, bool) {
	if r == nil {
		return rbacRequirement{}, false
	}
	policy = policy.withDefaultRequirements()
	for _, key := range daemonRBACRequestRouteKeys(r) {
		requirement, ok := policy.Requirements[key]
		if ok {
			return requirement.Clone(), true
		}
	}
	return rbacRequirement{}, false
}

func daemonRBACRequestRouteKeys(r *http.Request) []string {
	if r == nil {
		return nil
	}
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	path := ""
	if r.URL != nil {
		path = r.URL.Path
	}
	exact := daemonRBACPolicyRouteKey(method, path)
	keys := []string{exact}
	if method == http.MethodPost && strings.HasPrefix(path, "/api/v1/workspaces/") && strings.HasSuffix(path, "/select") {
		template := daemonRBACPolicyRouteKey(method, daemonRBACWorkspaceSelectPathTemplate)
		if template != exact {
			keys = append(keys, template)
		}
	}
	return keys
}
